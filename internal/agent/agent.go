// Package agent orchestrates authorized, bounded text work with durable claims.
package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

type Model interface {
	Chat(context.Context, []provider.Message, int) (provider.Reply, error)
}

// JSONModel is optional: only a trusted, admitted image-advice context selects
// this capability. A failed JSON call must never fall back to another request.
type JSONModel interface {
	ChatJSON(context.Context, []provider.Message, int) (provider.Reply, error)
}
type Searcher interface {
	Search(context.Context, string, int) ([]provider.SearchResult, error)
}
type Sender interface {
	SendText(context.Context, string, string, string, string) error
}
type Incoming struct {
	Account, User, ID, Text, ContextToken string
	Attachment                            *storage.Attachment
}
type Options struct {
	Account, User, Profile                 string
	MaxOutput, ContextBytes, ContextTokens int
}
type Result struct {
	State      string
	ImageState string
	// ImageDecision is a fixed, process-local diagnostic, never model text.
	ImageDecision string
	Usage         provider.Usage
	Code          string
}
type Agent struct {
	store             *storage.Store
	model             Model
	vision            Model
	images            ImageProcessor
	documents         DocumentProcessor
	search            Searcher
	sender            Sender
	opts              Options
	scope             storage.Scope
	replyImageBorrow  ReplyImageBorrow
	replyImagePrepare ReplyImagePrepare
	admission         chan struct{}
}

const maxReply = 16384
const limitReply = "内容超过当前限制，请缩短后再试。"
const modelReply = "暂时无法生成回复，请稍后发送新消息再试。"
const memoryReply = "暂时无法处理记忆操作，请稍后发送新消息再试。"
const searchReply = "暂时无法完成搜索，请稍后发送新消息再试。"

func New(s *storage.Store, m Model, q Searcher, send Sender, o Options) (*Agent, error) {
	if s == nil || m == nil || send == nil || !validField(o.Account, 256) || !validField(o.User, 256) {
		return nil, errors.New("invalid agent configuration")
	}
	if o.Profile != "" && !validProfileID(o.Profile) {
		return nil, errors.New("invalid agent configuration")
	}
	if o.MaxOutput == 0 {
		o.MaxOutput = 512
	}
	if o.ContextBytes == 0 {
		o.ContextBytes = 24576
	}
	if o.ContextTokens == 0 {
		o.ContextTokens = 32768
	}
	if o.MaxOutput < 1 || o.MaxOutput > 4096 || o.ContextBytes < 16384 || o.ContextBytes > 49152 || o.ContextTokens < 4096 || o.ContextTokens > 65536 {
		return nil, errors.New("invalid agent configuration")
	}
	if o.Profile != "" {
		if _, builtin := profiles[o.Profile]; !builtin {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := s.ChatContextWithProfile(ctx, storage.Scope{Account: o.Account, User: o.User}, "", 1, 1, o.Profile); err != nil {
				return nil, errors.New("invalid agent configuration")
			}
		}
	}
	return &Agent{store: s, model: m, search: q, sender: send, opts: o, scope: storage.Scope{Account: o.Account, User: o.User}, admission: make(chan struct{}, 1)}, nil
}

// Handle serializes one scope's work. The total deadline includes admission
// wait. Every persisted claim is terminal for automatic replay, in every state.
func (a *Agent) Handle(parent context.Context, in Incoming) (result Result, handleErr error) {
	if parent == nil {
		return failure("rejected", "input")
	}
	if in.Account != a.opts.Account || in.User != a.opts.User {
		return failure("rejected", "scope")
	}
	if !validField(in.Account, 256) || !validField(in.User, 256) || !validField(in.ID, 512) || !validField(in.ContextToken, 16384) || !validText(in.Text) || len(in.Text) > 16384 || in.Attachment == nil && strings.TrimSpace(in.Text) == "" {
		return failure("rejected", "input")
	}
	// Descriptor access follows exact scope and genuine caption validation.
	// Copy validated metadata so subsequent work uses this admitted descriptor.
	if in.Attachment != nil {
		descriptor := *in.Attachment
		if descriptor.Validate() != nil {
			return failure("rejected", "input")
		}
		in.Attachment = &descriptor
	}
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return failure("failed", "canceled")
	}
	select {
	case a.admission <- struct{}{}:
		defer func() { <-a.admission }()
	case <-ctx.Done():
		return failure("failed", "canceled")
	}
	if ctx.Err() != nil {
		return failure("failed", "canceled")
	}
	claimed, err := a.store.ClaimMessage(ctx, a.scope, in.ID, in.Text)
	if err != nil {
		return failure("failed", storageCode(err))
	}
	if !claimed {
		return Result{State: "duplicate"}, nil
	}
	var text, code string
	var usage provider.Usage
	var captured storage.Profile
	var advice imageAdvice
	advice.decision = "not_applicable"
	defer func() {
		if advice.decision == "suggested" && ctx.Err() != nil {
			advice.decision = "canceled"
		}
		if advice.decision == "selected" {
			advice.decision = "image_pipeline"
			if result.ImageState == "acked" {
				advice.decision = "image_acked"
			}
		}
		result.ImageDecision = advice.decision
	}()
	ordinary := false
	if in.Attachment != nil {
		if len(in.Text) > 8192 {
			text, code = limitReply, "limit"
		} else if in.Attachment.Kind == "file" {
			text, usage, code = a.answerDocument(ctx, in.Text, *in.Attachment)
		} else {
			text, usage, code = a.answerImage(ctx, in.Text, *in.Attachment)
		}
	} else {
		var handled bool
		text, code, handled = a.command(ctx, in.Text)
		if !handled {
			if len(in.Text) > 8192 {
				text, code = limitReply, "limit"
			} else {
				text, usage, code = a.answerWithAdvice(ctx, in.Text, &captured, &advice)
				_, searching := searchCommand(in.Text)
				ordinary = code == "" && !searching
			}
		}
	}
	if advice.err != nil {
		a.setState(ctx, in.ID, "failed")
		return Result{State: "failed", Usage: usage, Code: storageCode(advice.err)}, errors.New(storageCode(advice.err))
	}
	if ctx.Err() != nil {
		a.setState(ctx, in.ID, "failed")
		return Result{State: "failed", Usage: usage, Code: "canceled"}, errors.New("canceled")
	}
	text, valid := validatedReply(text, maxReply)
	if !valid {
		text, code = modelReply, "output"
	}
	if ordinary && valid && advice.id != "" && a.replyImageBorrow != nil && a.replyImagePrepare != nil && imageSelectionProfile(captured) && captured.Sticker != "off" {
		chosen, selected := selectReplyImage(imageSelectionInput{Scope: a.scope, InboundID: in.ID, UserText: in.Text, Reply: text, SuggestedID: advice.id, Profile: captured, Candidates: advice.candidates, NowMillis: time.Now().UTC().UnixMilli(), LastConfirmedMillis: advice.last, OrdinarySuccess: true})
		if selected {
			advice.decision = "selected"
			return a.sendImageReply(ctx, in, text, usage, chosen)
		}
		advice.decision = refusedImageDecision(advice, captured, time.Now().UTC().UnixMilli())
	}
	if err = a.store.SetMessageState(ctx, a.scope, in.ID, "sending"); err != nil {
		a.setState(ctx, in.ID, "failed")
		return Result{State: "failed", Usage: usage, Code: storageCode(err)}, errors.New(storageCode(err))
	}
	// Once sending is durable, any uncertainty is ambiguous. There is one call,
	// no send retry, even if the sender returns an acknowledgment after cancel.
	if ctx.Err() != nil {
		a.setState(ctx, in.ID, "ambiguous")
		return Result{State: "ambiguous", Usage: usage, Code: "canceled"}, errors.New("canceled")
	}
	err = a.sender.SendText(ctx, in.User, in.ContextToken, stableClientID(in), text)
	if err != nil || ctx.Err() != nil {
		a.setState(ctx, in.ID, "ambiguous")
		code = "send"
		if ctx.Err() != nil {
			code = "canceled"
		}
		return Result{State: "ambiguous", Usage: usage, Code: code}, errors.New(code)
	}
	if err = a.store.CompleteMessage(ctx, a.scope, in.ID, text); err != nil {
		a.setState(ctx, in.ID, "ambiguous")
		return Result{State: "ambiguous", Usage: usage, Code: "storage"}, errors.New("storage")
	}
	return Result{State: "sent", Usage: usage, Code: code}, nil
}

func failure(state, code string) (Result, error) {
	return Result{State: state, Code: code}, errors.New(code)
}
func storageCode(err error) string {
	if errors.Is(err, storage.ErrCapacity) {
		return "capacity"
	}
	return "storage"
}
func (a *Agent) setState(ctx context.Context, id, state string) {
	// Detached cleanup is exclusively a bounded state write: never network work.
	if ctx.Err() != nil {
		detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = a.store.SetMessageState(detached, a.scope, id, state)
		return
	}
	_ = a.store.SetMessageState(ctx, a.scope, id, state)
}
func validText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func validField(s string, max int) bool {
	if strings.TrimSpace(s) == "" || len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func stableClientID(in Incoming) string {
	b, _ := json.Marshal([3]string{in.Account, in.User, in.ID})
	h := sha256.Sum256(b)
	return "miskoai-" + hex.EncodeToString(h[:])
}
func prefixBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 0 {
		return ""
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
func validatedReply(s string, max int) (string, bool) {
	if !validText(s) || strings.TrimSpace(s) == "" {
		return "", false
	}
	if len(s) > max {
		suffix := "\n[内容已截断]"
		s = prefixBytes(s, max-len(suffix)) + suffix
	}
	return s, true
}

func (a *Agent) answer(ctx context.Context, current string) (string, provider.Usage, string) {
	return a.answerWithProfile(ctx, current, nil)
}

func (a *Agent) answerWithProfile(ctx context.Context, current string, captured *storage.Profile) (string, provider.Usage, string) {
	return a.answerWithAdvice(ctx, current, captured, nil)
}

func (a *Agent) answerWithAdvice(ctx context.Context, current string, captured *storage.Profile, advice *imageAdvice) (string, provider.Usage, string) {
	var empty provider.Usage
	snapshot, err := a.store.ChatContextWithProfile(ctx, a.scope, factQuery(current), 16, 8, a.opts.Profile)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrInvalid) {
			return modelReply, empty, "input"
		}
		return modelReply, empty, storageCode(err)
	}
	if captured != nil {
		*captured = snapshot.Profile
	}
	var sources []provider.SearchResult
	query, explicit := searchCommand(current)
	if explicit {
		if query == "" || len(query) > 512 {
			return limitReply, empty, "limit"
		}
		if a.search == nil {
			return searchReply, empty, "search"
		}
		got, err := a.search.Search(ctx, query, 3)
		if err != nil || len(got) > 3 {
			return searchReply, empty, "search"
		}
		for _, r := range got {
			if provider.SafeSourceURL(r.URL) && len(r.URL) <= 2048 && validText(r.URL) && validText(r.Title) && validText(r.Content) {
				r.Title = prefixBytes(r.Title, 256)
				r.Content = prefixBytes(r.Content, 1600)
				sources = append(sources, r)
			}
		}
	}
	var msgs []provider.Message
	var ok bool
	structured := false
	if advice != nil && !explicit {
		switch {
		case a.replyImageBorrow == nil || a.replyImagePrepare == nil:
			advice.decision = "unavailable"
		case !imageSelectionProfile(snapshot.Profile):
			advice.decision = "profile_invalid"
		case snapshot.Profile.Sticker == "off":
			advice.decision = "profile_off"
		default:
			advice.decision = "empty_catalogue"
		}
	}
	if advice != nil && !explicit && a.replyImageBorrow != nil && a.replyImagePrepare != nil && imageSelectionProfile(snapshot.Profile) && snapshot.Profile.Sticker != "off" {
		candidates, last, snapshotErr := a.store.ImageSelectionSnapshot(ctx, a.scope)
		if snapshotErr != nil {
			advice.decision = "snapshot_error"
			advice.err = snapshotErr
			return modelReply, empty, storageCode(snapshotErr)
		}
		catalogue, offered := imageCatalogue(a.scope, candidates, time.Now().UTC().UnixMilli())
		if len(offered) > 0 {
			advice.decision = "context_limit"
			msgs, ok = buildImageAdviceContext(current, snapshot, sources, a.opts, catalogue)
			if ok {
				structured = true
				advice.candidates, advice.last = offered, last
			}
		}
	}
	if !structured {
		msgs, ok = buildSnapshotContext(current, snapshot, sources, a.opts)
	}
	if !ok {
		return limitReply, empty, "limit"
	}
	if ctx.Err() != nil {
		return modelReply, empty, "canceled"
	}
	chat := a.model.Chat
	if structured {
		if model, ok := a.model.(JSONModel); ok {
			chat = model.ChatJSON
		}
	}
	reply, err := chat(ctx, msgs, a.opts.MaxOutput)
	if err != nil {
		if advice != nil {
			advice.decision = "model_error"
		}
		return modelReply, empty, "model"
	}
	if reply.FinishReason != "stop" && reply.FinishReason != "length" {
		if advice != nil {
			advice.decision = "invalid_output"
		}
		return modelReply, reply.Usage, "output"
	}
	if structured {
		text, id, decision, valid := parseImageAdvice(reply.Text)
		advice.decision = decision
		if !valid {
			return modelReply, reply.Usage, "output"
		}
		// A complete text inside a length-limited response is useful, but a
		// truncated response never grants optional image advice.
		if reply.FinishReason == "stop" {
			advice.id = id
		} else {
			advice.decision = "output_limited"
		}
		return text, reply.Usage, ""
	}
	citation := ""
	if len(sources) > 0 {
		citation = "\n\n搜索来源："
		for _, r := range sources {
			citation += "\n" + r.Title + "\n" + r.URL
		}
	}
	text, valid := validatedReply(reply.Text, maxReply-len(citation))
	if !valid {
		return modelReply, reply.Usage, "output"
	}
	return text + citation, reply.Usage, ""
}
