package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

// Local fixture delegates implement the complete channel independently. Every
// remote-shaped method stays synchronous and synthetic; no transport is called.
type coreReplyChannel struct {
	text   func(context.Context, string, string, string, string) error
	image  func(context.Context, string, string, string, weixin.ImageSend) error
	upload func(context.Context, weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error)
}

func (d *coreReplyChannel) RawUpdates(ctx context.Context, _ string) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (d *coreReplyChannel) SendText(ctx context.Context, to, token, id, text string) error {
	if d.text != nil {
		return d.text(ctx, to, token, id, text)
	}
	return nil
}
func (d *coreReplyChannel) SendImage(ctx context.Context, to, token, id string, im weixin.ImageSend) error {
	if d.image != nil {
		return d.image(ctx, to, token, id, im)
	}
	return nil
}
func (d *coreReplyChannel) GetImageUploadURL(ctx context.Context, r weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
	if d.upload != nil {
		return d.upload(ctx, r)
	}
	return weixin.ImageUploadLocation{}, weixin.ErrTransport
}

func coreReplyIncoming(c *Controller) agent.Incoming {
	for n := 1; ; n++ {
		id := fmt.Sprint(n)
		b, _ := json.Marshal([3]string{c.scope.Account, c.scope.User, id})
		h := sha256.Sum256(b)
		if h[0]%8 < 2 {
			return agent.Incoming{Account: c.scope.Account, User: c.scope.User, ID: id, Text: "happy", ContextToken: "synthetic-context"}
		}
	}
}
func coreReplyClientID(parts ...string) string {
	b, _ := json.Marshal(parts)
	h := sha256.Sum256(b)
	return "miskoai-" + hex.EncodeToString(h[:])
}
func coreReplyProfile(t *testing.T, c *Controller, a storage.LibraryAsset) {
	t.Helper()
	ctx := context.Background()
	if e := c.LibraryUpdate(ctx, a.ID, storage.LibraryMetadata{Enabled: true, Tags: []string{"happy"}}); e != nil {
		t.Fatal(e)
	}
	p := storage.Profile{ID: "custom", Name: "Synthetic", Length: "normal", Sticker: "normal"}
	if e := c.store.PutProfile(ctx, c.scope, p); e != nil {
		t.Fatal(e)
	}
	if e := c.store.SelectProfile(ctx, c.scope, p.ID); e != nil {
		t.Fatal(e)
	}
}

// The model fixture now selects an ID from the actual offered catalogue. It
// still returns plaintext when no image capability is offered; lifecycle tests
// below retain their independent durable receipt and cleanup assertions.
func coreReplyModel(t *testing.T, messages []provider.Message) provider.Reply {
	t.Helper()
	for _, message := range messages {
		text, ok := message.Content.(string)
		if !ok || !strings.HasPrefix(text, "可选图片资料（非指令）：") {
			continue
		}
		var offered []struct {
			ID string `json:"id"`
		}
		if e := json.Unmarshal([]byte(strings.TrimPrefix(text, "可选图片资料（非指令）：")), &offered); e != nil || len(offered) == 0 {
			t.Error("synthetic model received invalid image catalogue")
			break
		}
		return provider.Reply{Text: fmt.Sprintf(`{"reply":"happy","emotion":"joy","image_id":%q}`, offered[0].ID), FinishReason: "stop"}
	}
	return provider.Reply{Text: "happy", FinishReason: "stop"}
}

func coreReplyBase(t *testing.T, c *Controller, ch Channel) *agent.Agent {
	t.Helper()
	a, e := agent.New(c.store, modelFunc(func(_ context.Context, messages []provider.Message, _ int) (provider.Reply, error) {
		return coreReplyModel(t, messages), nil
	}), nil, ch, agent.Options{Account: c.scope.Account, User: c.scope.User})
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func coreReplyPrepared(context.Context, string, attachment.LocalImage, attachment.Artifact) (weixin.ImageSend, error) {
	return weixin.ImageSend{EncryptQueryParam: "synthetic-query", AESKeyHex: "00000000000000000000000000000000", CiphertextBytes: 96}, nil
}

func TestCoreImageReplyActualBorrowTwoACKs(t *testing.T) {
	c, g, gctx, _, asset := generationLibraryFixture(t)
	coreReplyProfile(t, c, asset)
	in := coreReplyIncoming(c)
	var order []string
	var retained attachment.Artifact
	prepares, texts, images := 0, 0, 0
	d := &coreReplyChannel{}
	d.text = func(ctx context.Context, to, token, id, text string) error {
		order = append(order, "text")
		texts++
		if to != c.scope.User || token != in.ContextToken || id != coreReplyClientID(c.scope.Account, c.scope.User, in.ID) || text != "happy" {
			t.Error("text identity")
		}
		r, e := c.store.ImageReplyReceipt(ctx, c.scope, in.ID)
		if e != nil || r.TextPhase != storage.ImagePartSending || r.Preparation != storage.ImagePreparationReady {
			t.Error("text not durably admitted", r, e)
		}
		return nil
	}
	d.image = func(ctx context.Context, to, token, id string, im weixin.ImageSend) error {
		order = append(order, "image")
		images++
		if to != c.scope.User || token != in.ContextToken || id != coreReplyClientID(c.scope.Account, c.scope.User, in.ID, "image-v1") || im.CiphertextBytes != 96 {
			t.Error("image identity")
		}
		r, e := c.store.ImageReplyReceipt(ctx, c.scope, in.ID)
		if e != nil || r.TextPhase != storage.ImagePartAcknowledged || r.ImagePhase != storage.ImagePartSending {
			t.Error("independent image admission", r, e)
		}
		return nil
	}
	ch := newObservedChannel(c, g, d)
	base := coreReplyBase(t, c, ch)
	configured, e := c.bindGenerationReplyAgent(gctx, g, ch, base, func(ctx context.Context, to string, im attachment.LocalImage, art attachment.Artifact) (weixin.ImageSend, error) {
		order = append(order, "prepare")
		prepares++
		retained = art
		c.mu.Lock()
		held := c.leases
		c.mu.Unlock()
		if held != 1 || to != c.scope.User || im.SHA256 != asset.SHA256 || art.Size() != asset.Bytes {
			t.Error("actual held identities", held)
		}
		b := make([]byte, 8)
		if _, e := art.ReadAt(b, 0); e != nil || string(b) != "\x89PNG\r\n\x1a\n" {
			t.Error("genuine validated PNG", e)
		}
		r, e := c.store.ImageReplyReceipt(ctx, c.scope, in.ID)
		if e != nil || r.Preparation != storage.ImagePreparationExternal {
			t.Error("preparation admission", r, e)
		}
		if e := c.store.PutProfile(ctx, c.scope, storage.Profile{ID: "callback", Name: "Synthetic", Length: "normal", Sticker: "off"}); e != nil {
			t.Error("transaction spans consumer", e)
		}
		return coreReplyPrepared(ctx, to, im, art)
	})
	if e != nil {
		t.Fatal(e)
	}
	r, e := configured.Handle(context.Background(), in)
	if e != nil || r.State != "sent" || r.ImageState != "acked" {
		t.Fatal("two-part result", r, e)
	}
	receipt, e := c.store.ImageReplyReceipt(context.Background(), c.scope, in.ID)
	if e != nil || receipt.TextPhase != "acked" || receipt.ImagePhase != "acked" || receipt.AssetID != asset.ID || receipt.SHA256 != asset.SHA256 || receipt.TextClientID == receipt.ImageClientID {
		t.Fatal("two durable ACKs", receipt, e)
	}
	row, e := c.store.LibraryAsset(context.Background(), c.scope, asset.ID)
	if e != nil || row.Uses != 1 {
		t.Fatal("usage once", row, e)
	}
	if fmt.Sprint(order) != "[prepare text image]" || prepares != 1 || texts != 1 || images != 1 {
		t.Fatal("once and order", order)
	}
	if _, e := retained.ReadAt(make([]byte, 1), 0); e == nil {
		t.Fatal("plaintext retained after lifetime")
	}
	c.mu.Lock()
	held := c.leases
	c.mu.Unlock()
	if held != 0 {
		t.Fatal("lease leaked")
	}
	// The configured and original Agent share admission and persistent claims.
	for _, a := range []*agent.Agent{configured, base} {
		r, e := a.Handle(context.Background(), in)
		if e != nil || r.State != "duplicate" {
			t.Fatal("duplicate", r, e)
		}
	}
	if texts != 1 || images != 1 || prepares != 1 {
		t.Fatal("duplicate sent")
	}
}

func TestCoreImageReplyCapturedIdentityRefuses(t *testing.T) {
	for _, name := range []string{"generation", "store", "scope", "manager", "exclusive", "canceled"} {
		t.Run(name, func(t *testing.T) {
			c, g, gctx, cancel, asset := generationLibraryFixture(t)
			coreReplyProfile(t, c, asset)
			in := coreReplyIncoming(c)
			calls := 0
			d := &coreReplyChannel{text: func(context.Context, string, string, string, string) error { calls++; return nil }, image: func(context.Context, string, string, string, weixin.ImageSend) error { calls++; return nil }}
			ch := newObservedChannel(c, g, d)
			a, e := c.bindGenerationReplyAgent(gctx, g, ch, coreReplyBase(t, c, ch), func(ctx context.Context, to string, im attachment.LocalImage, art attachment.Artifact) (weixin.ImageSend, error) {
				calls++
				return coreReplyPrepared(ctx, to, im, art)
			})
			if e != nil {
				t.Fatal(e)
			}
			store, scope, manager := c.store, c.scope, c.library
			// The model's own snapshot must complete before mutating captured Core state.
			c.mu.Lock()
			switch name {
			case "generation":
				c.generation = &generation{}
			case "store":
				c.store = &storage.Store{}
			case "scope":
				c.scope.User = "replacement"
			case "manager":
				c.library = &library.Manager{}
			case "exclusive":
				c.exclusive = true
			case "canceled":
				cancel()
			}
			c.mu.Unlock()
			defer func() {
				c.mu.Lock()
				c.store = store
				c.scope = scope
				c.library = manager
				c.generation = g
				c.exclusive = false
				c.mu.Unlock()
			}()
			done := make(chan error, 1)
			go func() { _, e := a.Handle(context.Background(), in); done <- e }()
			select {
			case e := <-done:
				if e == nil || calls != 0 {
					t.Fatal("captured identity accepted", calls, e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("refusal waited for maintenance")
			}
			row, e := store.LibraryAsset(context.Background(), scope, asset.ID)
			if e != nil || row.Uses != 0 {
				t.Fatal("refused usage", row, e)
			}
		})
	}
}

func TestCoreImageReplyManagerRecheckedAfterValidation(t *testing.T) {
	c, g, gctx, _, asset := generationLibraryFixture(t)
	coreReplyProfile(t, c, asset)
	original := c.library
	manager, e := library.New(c.cfg.DataDir, generationLibraryProcessor{delegate: c.images.(library.Processor), before: func() { c.mu.Lock(); c.library = original; c.mu.Unlock() }})
	if e != nil {
		t.Fatal(e)
	}
	c.library = manager
	defer func() {
		c.library = original
		if e := manager.Close(); e != nil {
			t.Error(e)
		}
	}()
	calls := 0
	d := &coreReplyChannel{text: func(context.Context, string, string, string, string) error { calls++; return nil }}
	ch := newObservedChannel(c, g, d)
	a, e := c.bindGenerationReplyAgent(gctx, g, ch, coreReplyBase(t, c, ch), func(ctx context.Context, to string, im attachment.LocalImage, art attachment.Artifact) (weixin.ImageSend, error) {
		calls++
		return coreReplyPrepared(ctx, to, im, art)
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := a.Handle(context.Background(), coreReplyIncoming(c)); e == nil || calls != 0 {
		t.Fatal("manager replacement entered consumer", calls, e)
	}
}

func TestCoreImageReplyCancelAndCloseJoin(t *testing.T) {
	for _, name := range []string{"generation", "close"} {
		t.Run(name, func(t *testing.T) {
			c, g, gctx, cancel, asset := generationLibraryFixture(t)
			coreReplyProfile(t, c, asset)
			in := coreReplyIncoming(c)
			store, sc := c.store, c.scope
			entered, observed, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			sends := 0
			d := &coreReplyChannel{text: func(context.Context, string, string, string, string) error { sends++; return nil }}
			ch := newObservedChannel(c, g, d)
			a, e := c.bindGenerationReplyAgent(gctx, g, ch, coreReplyBase(t, c, ch), func(ctx context.Context, _ string, _ attachment.LocalImage, art attachment.Artifact) (weixin.ImageSend, error) {
				close(entered)
				<-ctx.Done()
				close(observed)
				<-release
				c.mu.Lock()
				held := c.leases
				c.mu.Unlock()
				if held != 1 {
					t.Error("lease released before actual preparation returned")
				}
				if _, e := art.ReadAt(make([]byte, 1), 0); e != nil {
					t.Error("artifact closed early", e)
				}
				return weixin.ImageSend{}, ctx.Err()
			})
			if e != nil {
				t.Fatal(e)
			}
			done := make(chan error, 1)
			go func() { _, e := a.Handle(context.Background(), in); close(g.done); done <- e }()
			waitSignal(t, entered)
			closeDone := make(chan error, 1)
			if name == "close" {
				go func() { closeDone <- c.Close() }()
			} else {
				cancel()
			}
			waitSignal(t, observed)
			if _, e := store.LibraryAsset(context.Background(), sc, asset.ID); e != nil {
				t.Error("Store closed early", e)
			}
			select {
			case e := <-done:
				t.Error("consumer returned early", e)
			case <-time.After(20 * time.Millisecond):
			}
			if name == "close" {
				select {
				case e := <-closeDone:
					t.Error("Close returned before actual preparation", e)
				case <-time.After(20 * time.Millisecond):
				}
			}
			unblock()
			select {
			case e := <-done:
				if e == nil || sends != 0 {
					t.Error("canceled send", sends, e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("consumer did not join")
			}
			if name == "close" {
				select {
				case e := <-closeDone:
					if e != nil {
						t.Error(e)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("Close did not join")
				}
			}
		})
	}
}

func TestCoreImageReplyNoOuterLeaseWhileMediaWaits(t *testing.T) {
	c, g, gctx, cancel, asset := generationLibraryFixture(t)
	coreReplyProfile(t, c, asset)
	original := c.library
	entry := make(chan struct{})
	m, e := library.New(c.cfg.DataDir, generationLibraryProcessor{delegate: c.images.(library.Processor), entry: func() { close(entry) }})
	if e != nil {
		t.Fatal(e)
	}
	c.library = m
	defer func() {
		c.library = original
		if e := m.Close(); e != nil {
			t.Error(e)
		}
	}()
	hold, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	mediaDone := make(chan error, 1)
	raw := libraryPNG(t, 88)
	go func() {
		mediaDone <- c.images.(library.Processor).WithLocalImage(context.Background(), bytes.NewReader(raw), int64(len(raw)), func(attachment.LocalImage, attachment.Artifact) error { close(hold); <-release; return nil })
	}()
	waitSignal(t, hold)
	calls := 0
	d := &coreReplyChannel{text: func(context.Context, string, string, string, string) error { calls++; return nil }}
	ch := newObservedChannel(c, g, d)
	a, e := c.bindGenerationReplyAgent(gctx, g, ch, coreReplyBase(t, c, ch), func(ctx context.Context, to string, im attachment.LocalImage, art attachment.Artifact) (weixin.ImageSend, error) {
		calls++
		return coreReplyPrepared(ctx, to, im, art)
	})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	in := coreReplyIncoming(c)
	go func() { _, e := a.Handle(context.Background(), in); done <- e }()
	waitSignal(t, entry)
	c.mu.Lock()
	held := c.leases
	c.mu.Unlock()
	if held != 0 {
		t.Error("outer Store lease held before media admission", held)
	}
	cancel()
	select {
	case e := <-done:
		if e == nil || calls != 0 {
			t.Error("canceled media wait consumed", calls, e)
		}
	case <-time.After(5 * time.Second):
		t.Error("generation cancellation did not leave media wait")
	}
	unblock()
	if e := <-mediaDone; e != nil {
		t.Error(e)
	}
}

func TestCoreImageReplyOptionalComposition(t *testing.T) {
	c, g, gctx, _, asset := generationLibraryFixture(t)
	coreReplyProfile(t, c, asset)
	for _, name := range []string{"text", "upload-only", "send-only", "missing-cdn", "missing-manager"} {
		t.Run(name, func(t *testing.T) {
			var d Channel
			switch name {
			case "text":
				d = &observedImageTextDelegate{}
			case "upload-only":
				d = &observedImageUploadDelegate{observedImageTextDelegate: &observedImageTextDelegate{}}
			case "send-only":
				d = &observedImageSendDelegate{&observedImageTextDelegate{}}
			default:
				d = &coreReplyChannel{}
			}
			ch := newObservedChannel(c, g, d)
			base := coreReplyBase(t, c, ch)
			cdn, m := c.cdn, c.library
			if name == "missing-cdn" {
				c.cdn = nil
			}
			if name == "missing-manager" {
				c.library = nil
			}
			a, e := c.generationReplyAgent(gctx, g, ch, base)
			c.cdn, c.library = cdn, m
			if e != nil || a != base {
				t.Fatal("optional dependency changed legacy Agent", e)
			}
		})
	}
}

func TestCoreImageReplyInvalidComposition(t *testing.T) {
	c, g, gctx, _, _ := generationLibraryFixture(t)
	ch := newObservedChannel(c, g, &coreReplyChannel{})
	base := coreReplyBase(t, c, ch)
	for _, tc := range []struct {
		name       string
		controller *Controller
		ctx        context.Context
		g          *generation
		a          *agent.Agent
		prepare    agent.ReplyImagePrepare
	}{
		{"nil-controller", nil, gctx, g, base, coreReplyPrepared}, {"nil-context", c, nil, g, base, coreReplyPrepared}, {"nil-generation", c, gctx, nil, base, coreReplyPrepared}, {"nil-agent", c, gctx, g, nil, coreReplyPrepared}, {"nil-prepare", c, gctx, g, base, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, e := tc.controller.bindGenerationReplyAgent(tc.ctx, tc.g, ch, tc.a, tc.prepare)
			if e != ErrConfiguration || a != nil {
				t.Fatal("invalid binding", a, e)
			}
		})
	}
	legacy := coreReplyBase(t, c, &observedImageTextDelegate{})
	if a, e := c.bindGenerationReplyAgent(gctx, g, ch, legacy, coreReplyPrepared); e != ErrConfiguration || a != nil {
		t.Fatal("Agent sender mismatch did not refuse safely", e)
	}
}

// This exercises the actual startGeneration callsite and production Preparer.
// Upload-location expiry ends before CDN upload, so no external call is possible.
func TestCoreImageReplyStartGenerationObservedPreparationExpiry(t *testing.T) {
	c := libraryController(t, true)
	asset := libraryUploaded(t, c, libraryPNG(t, 91))
	coreReplyProfile(t, c, asset)
	in := coreReplyIncoming(c)
	entered := make(chan struct{})
	sends := 0
	c.channel = &coreReplyChannel{upload: func(ctx context.Context, r weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
		if r.ToUserID != c.scope.User || r.RawBytes != asset.Bytes {
			t.Error("production preparation lost image identity")
		}
		close(entered)
		return weixin.ImageUploadLocation{}, weixin.ErrAuthExpired
	}, text: func(context.Context, string, string, string, string) error { sends++; return nil }, image: func(context.Context, string, string, string, weixin.ImageSend) error { sends++; return nil }}
	c.model = modelFunc(func(_ context.Context, messages []provider.Message, _ int) (provider.Reply, error) {
		return coreReplyModel(t, messages), nil
	})
	c.providerReady = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.parent = ctx
	body := fmt.Sprintf(`{"ret":0,"msgs":[{"message_id":%s,"from_user_id":"fixture-user","to_user_id":"fixture-account","message_type":1,"message_state":2,"context_token":"synthetic-context","item_list":[{"type":1,"text_item":{"text":"happy"}}]}],"get_updates_buf":"next"}`, in.ID)
	if _, e := c.store.RecordPoll(ctx, c.scope, "", []byte(body)); e != nil {
		t.Fatal(e)
	}
	if e := c.startGeneration(); e != nil {
		t.Fatal(e)
	}
	defer c.stopGeneration()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual generation did not preserve image capability / configure real preparation")
	}
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.expired })
	c.mu.Lock()
	g := c.generation
	c.mu.Unlock()
	waitSignal(t, g.done)
	receipt, e := c.store.ImageReplyReceipt(context.Background(), c.scope, in.ID)
	if e != nil || receipt.Preparation != storage.ImagePreparationUnknown || receipt.TextPhase == storage.ImagePartAcknowledged || sends != 0 {
		t.Fatal("upload expiry receipt / no send", receipt, sends, e)
	}
	if files, e := filepath.Glob(filepath.Join(c.cfg.DataDir, "media-*")); e != nil || len(files) != 0 {
		t.Fatal("production plaintext/ciphertext cleanup", files, e)
	}
}
