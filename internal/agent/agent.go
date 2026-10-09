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
type Searcher interface {
	Search(context.Context, string, int) ([]provider.SearchResult, error)
}
type Sender interface {
	SendText(context.Context, string, string, string, string) error
}
type Incoming struct{ Account, User, ID, Text, ContextToken string }
type Options struct {
	Account, User, Profile                 string
	MaxOutput, ContextBytes, ContextTokens int
}
type Result struct {
	State string
	Usage provider.Usage
	Code  string
}
type Agent struct {
	store     *storage.Store
	model     Model
	search    Searcher
	sender    Sender
	opts      Options
	scope     storage.Scope
	admission chan struct{}
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
func (a *Agent) Handle(parent context.Context, in Incoming) (Result, error) {
	if parent == nil {
		return failure("rejected", "input")
	}
	if in.Account != a.opts.Account || in.User != a.opts.User {
		return failure("rejected", "scope")
	}
	if !validField(in.Account, 256) || !validField(in.User, 256) || !validField(in.ID, 512) || !validField(in.ContextToken, 16384) || !validText(in.Text) || strings.TrimSpace(in.Text) == "" || len(in.Text) > 16384 {
		return failure("rejected", "input")
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
	text, code, handled := a.command(ctx, in.Text)
	var usage provider.Usage
	if !handled {
		if len(in.Text) > 8192 {
			text, code = limitReply, "limit"
		} else {
			text, usage, code = a.answer(ctx, in.Text)
		}
	}
	if ctx.Err() != nil {
		a.setState(ctx, in.ID, "failed")
		return Result{State: "failed", Usage: usage, Code: "canceled"}, errors.New("canceled")
	}
	text, valid := validatedReply(text, maxReply)
	if !valid {
		text, code = modelReply, "output"
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
	var empty provider.Usage
	snapshot, err := a.store.ChatContextWithProfile(ctx, a.scope, factQuery(current), 16, 8, a.opts.Profile)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrInvalid) {
			return modelReply, empty, "input"
		}
		return modelReply, empty, storageCode(err)
	}
	var sources []provider.SearchResult
	if query, explicit := searchCommand(current); explicit {
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
	msgs, ok := buildSnapshotContext(current, snapshot, sources, a.opts)
	if !ok {
		return limitReply, empty, "limit"
	}
	if ctx.Err() != nil {
		return modelReply, empty, "canceled"
	}
	reply, err := a.model.Chat(ctx, msgs, a.opts.MaxOutput)
	if err != nil {
		return modelReply, empty, "model"
	}
	if reply.FinishReason != "stop" && reply.FinishReason != "length" {
		return modelReply, reply.Usage, "output"
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
