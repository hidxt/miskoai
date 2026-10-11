package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

type imageProcessorFunc func(context.Context, storage.Attachment, func(provider.ContentPart) error) error

func (f imageProcessorFunc) WithImage(c context.Context, a storage.Attachment, k func(provider.ContentPart) error) error {
	return f(c, a, k)
}
func syntheticImagePart() provider.ContentPart {
	return provider.ContentPart{Type: "image_url", ImageURL: &provider.ImageURL{URL: "data:image/png;base64,AAAA"}}
}
func imageController(t *testing.T, vision modelFunc, images imageProcessorFunc, ch *fakeChannel) *Controller {
	t.Helper()
	cfg := fixtureConfig(t)
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	c, e := New(cfg, Dependencies{Model: modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		t.Error("image routed to text model")
		return provider.Reply{}, errors.New("text refused")
	}), Vision: vision, Images: images, ChannelFactory: func(config.Authorization) (Channel, error) { return ch, nil }})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	})
	return c
}
func imageInbox(t *testing.T, c *Controller, id string, kind string, caption string) {
	t.Helper()
	ctx := context.Background()
	cursor, e := c.store.Cursor(ctx, c.scope)
	if e != nil {
		t.Fatal(e)
	}
	f, e := c.store.RecordPoll(ctx, c.scope, cursor, []byte(`{"synthetic":true}`))
	if e != nil {
		t.Fatal(e)
	}
	a := storage.Attachment{Kind: kind}
	if kind == "image" {
		a.ImageHex = "00112233445566778899aabbccddeeff"
		a.Query = "query"
	}
	e = c.store.ResolvePoll(ctx, c.scope, f, "cursor", []storage.InboxEntry{{Scope: c.scope, MessageID: id, Text: caption, ContextToken: "synthetic-context", ReceivedAt: time.Now().UTC(), Attachment: &a}})
	if e != nil {
		t.Fatal(e)
	}
}
func TestCoreImageDurableDeliveryAndMetrics(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "ack", true: "ambiguous"}[unknown], func(t *testing.T) {
			var downloads, visions, sends atomic.Int32
			ch := &fakeChannel{poll: idle, send: func(context.Context, string, string, string, string) error {
				sends.Add(1)
				if unknown {
					return errors.New("synthetic unknown")
				}
				return nil
			}}
			c := imageController(t, modelFunc(func(ctx context.Context, m []provider.Message, n int) (provider.Reply, error) {
				visions.Add(1)
				parts, ok := m[len(m)-1].Content.([]provider.ContentPart)
				if !ok || len(parts) != 2 || parts[0].Text != "genuine caption" {
					t.Error("current caption/image missing")
				}
				return reply(ctx, m, n)
			}), imageProcessorFunc(func(ctx context.Context, a storage.Attachment, k func(provider.ContentPart) error) error {
				downloads.Add(1)
				return k(syntheticImagePart())
			}), ch)
			imageInbox(t, c, "image-1", "image", "genuine caption")
			cancel, done := runCore(t, c)
			eventually(t, func() bool { return c.Status().Channel.Processed == 1 })
			cancel()
			waitSignal(t, done)
			if downloads.Load() != 1 || visions.Load() != 1 || sends.Load() != 1 {
				t.Fatalf("effects downloads=%d vision=%d sends=%d", downloads.Load(), visions.Load(), sends.Load())
			}
			m := c.Status().Metrics
			if m.VisionAttempts != 1 || m.ChatAttempts != 0 || m.SuccessfulUsage.TotalTokens != 3 {
				t.Fatalf("wrong accounting %+v", m)
			}
			claimed, e := c.store.ClaimMessage(context.Background(), c.scope, "image-1", "genuine caption")
			if e != nil || claimed {
				t.Fatal("durable claim disappeared", claimed, e)
			}
			history, e := c.store.History(context.Background(), c.scope, 16)
			if e != nil {
				t.Fatal(e)
			}
			if !unknown && (len(history) != 2 || history[0].Content != "genuine caption") {
				t.Fatal("history provenance changed", history)
			}
		})
	}
}
func TestCoreUnsupportedAttachmentNoEffects(t *testing.T) {
	var effects atomic.Int32
	c := imageController(t, modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		effects.Add(1)
		return provider.Reply{}, nil
	}), imageProcessorFunc(func(context.Context, storage.Attachment, func(provider.ContentPart) error) error {
		effects.Add(1)
		return nil
	}), &fakeChannel{poll: idle})
	imageInbox(t, c, "unsupported-1", "unsupported", "")
	cancel, done := runCore(t, c)
	eventually(t, func() bool { return c.Status().Channel.Processed == 1 })
	cancel()
	waitSignal(t, done)
	if effects.Load() != 0 {
		t.Fatal("unsupported attachment had model/media effects")
	}
	h, e := c.store.History(context.Background(), c.scope, 16)
	if e != nil || len(h) != 0 {
		t.Fatal("empty USER became evidence", h, e)
	}
}
func TestCoreVisionAccountingSurvivesCleanupFailure(t *testing.T) {
	c := imageController(t, modelFunc(reply), imageProcessorFunc(func(ctx context.Context, _ storage.Attachment, k func(provider.ContentPart) error) error {
		if e := k(syntheticImagePart()); e != nil {
			return e
		}
		return errors.New("synthetic cleanup failed")
	}), &fakeChannel{poll: idle})
	imageInbox(t, c, "cleanup-1", "image", "")
	cancel, done := runCore(t, c)
	eventually(t, func() bool { return c.Status().Channel.Processed == 1 })
	cancel()
	waitSignal(t, done)
	m := c.Status().Metrics
	if m.VisionAttempts != 1 || m.SuccessfulUsage.TotalTokens != 3 {
		t.Fatal("successful vision usage lost", m)
	}
}
func TestCoreCloseJoinsImageCleanupBeforeDatabase(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var dbAvailable atomic.Bool
	var c *Controller
	c = imageController(t, modelFunc(reply), imageProcessorFunc(func(ctx context.Context, _ storage.Attachment, k func(provider.ContentPart) error) error {
		if e := k(syntheticImagePart()); e != nil {
			return e
		}
		close(entered)
		<-ctx.Done()
		<-release
		_, e := c.store.History(context.Background(), c.scope, 1)
		dbAvailable.Store(e == nil)
		return ctx.Err()
	}), &fakeChannel{poll: idle})
	imageInbox(t, c, "held-1", "image", "")
	ctx, cancel := context.WithCancel(context.Background())
	runDone, closeDone := make(chan struct{}), make(chan struct{})
	var launchedClose atomic.Bool
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		cancel()
		waitSignal(t, runDone)
		if launchedClose.Load() {
			waitSignal(t, closeDone)
		}
	})
	go func() { defer close(runDone); _ = c.Run(ctx) }()
	waitSignal(t, entered)
	launchedClose.Store(true)
	go func() {
		defer close(closeDone)
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	}()
	eventually(t, func() bool { return c.Status().ChannelState == "closing" })
	select {
	case <-closeDone:
		t.Fatal("Close abandoned actual image cleanup")
	default:
	}
	once.Do(func() { close(release) })
	waitSignal(t, closeDone)
	waitSignal(t, runDone)
	if !dbAvailable.Load() {
		t.Fatal("database closed before borrowed image cleanup")
	}
}
func TestCoreProductionVisionSharesTransportAndMediaLifetime(t *testing.T) {
	cfg := fixtureConfig(t)
	c, e := New(cfg, Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close() })
	text, ok := c.model.(*provider.DeepSeek)
	vision, vok := c.vision.(*provider.DeepSeek)
	if !ok || !vok || text == vision || c.images == nil || c.cdn == nil {
		t.Fatal("production dependencies missing or not cloned")
	}
	a := reflect.ValueOf(text).Elem().FieldByName("transport").Pointer()
	b := reflect.ValueOf(vision).Elem().FieldByName("transport").Pointer()
	if a == 0 || a != b {
		t.Fatal("vision created independent admission/transport")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	_, e = c.cdn.Download(context.Background(), cfg.DataDir, "", "", [16]byte{})
	if e == nil {
		t.Fatal("owned CDN not closed")
	}
}
func TestCoreImageDependencyPairRequired(t *testing.T) {
	for _, visionOnly := range []bool{true, false} {
		t.Run(map[bool]string{true: "vision", false: "images"}[visionOnly], func(t *testing.T) {
			cfg := fixtureConfig(t)
			d := Dependencies{Model: modelFunc(reply)}
			if visionOnly {
				d.Vision = modelFunc(reply)
			} else {
				d.Images = imageProcessorFunc(func(context.Context, storage.Attachment, func(provider.ContentPart) error) error { return nil })
			}
			c, e := New(cfg, d)
			if e == nil {
				_ = c.Close()
				t.Fatal("partial image dependency accepted")
			}
			if _, e := os.Lstat(filepath.Join(cfg.DataDir, "miskoai.db")); !os.IsNotExist(e) {
				t.Fatal("partial dependency caused database effect", e)
			}
		})
	}
}

func TestCoreImageResourcesSurviveRestoreAndGeneration(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg.DeepSeekKey = "synthetic-key"
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	c, e := New(cfg, Dependencies{ChannelFactory: func(config.Authorization) (Channel, error) { return &fakeChannel{poll: idle}, nil }})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close() })
	images, cdn, text, vision, oldStore := c.images, c.cdn, c.model, c.vision, c.store
	d, e := c.PrepareBackup(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = d.Close() })
	data, e := io.ReadAll(d)
	if e != nil {
		t.Fatal(e)
	}
	if e = d.Close(); e != nil {
		t.Fatal(e)
	}
	cancel, done := runCore(t, c)
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.generation != nil })
	if _, e = c.Restore(context.Background(), bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	if c.store == oldStore {
		t.Fatal("restore did not reopen database")
	}
	if e = c.ResumeAfterReconciliation(context.Background()); e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.generation != nil })
	if c.images != images || c.cdn != cdn || c.model != text || c.vision != vision {
		t.Fatal("restore replaced shared media/model resources")
	}
	cancel()
	waitSignal(t, done)
}

func TestCoreImageDatabaseReopenDoesNotReplay(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "ack", true: "ambiguous"}[unknown], func(t *testing.T) {
			var effects atomic.Int32
			var polls atomic.Int32
			ch := &fakeChannel{poll: func(ctx context.Context, s string) ([]byte, error) { polls.Add(1); return idle(ctx, s) }, send: func(context.Context, string, string, string, string) error {
				effects.Add(1)
				if unknown {
					return errors.New("synthetic uncertain send")
				}
				return nil
			}}
			vision := modelFunc(func(ctx context.Context, m []provider.Message, n int) (provider.Reply, error) {
				effects.Add(1)
				return reply(ctx, m, n)
			})
			images := imageProcessorFunc(func(ctx context.Context, _ storage.Attachment, k func(provider.ContentPart) error) error {
				effects.Add(1)
				return k(syntheticImagePart())
			})
			c := imageController(t, vision, images, ch)
			imageInbox(t, c, "restart-image", "image", "")
			cancel, done := runCore(t, c)
			eventually(t, func() bool { return c.Status().Channel.Processed == 1 })
			cancel()
			waitSignal(t, done)
			cfg := c.cfg
			if e := c.Close(); e != nil {
				t.Fatal(e)
			}
			next, e := New(cfg, Dependencies{Model: modelFunc(reply), Vision: vision, Images: images, ChannelFactory: func(config.Authorization) (Channel, error) { return ch, nil }})
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = next.Close() })
			imageInbox(t, next, "restart-image", "image", "")
			priorPolls := polls.Load()
			nextCancel, nextDone := runCore(t, next)
			eventually(t, func() bool { return polls.Load() > priorPolls })
			nextCancel()
			waitSignal(t, nextDone)
			pending, e := next.store.PendingInbox(context.Background(), next.scope, 1)
			if e != nil || len(pending) != 0 || next.Status().Channel.Processed != 0 {
				t.Fatal("durable receive duplicate entered handler", pending, e)
			}
			claimed, e := next.store.ClaimMessage(context.Background(), next.scope, "restart-image", "")
			if e != nil || claimed {
				t.Fatal("reopened message claim lost", claimed, e)
			}
			if effects.Load() != 3 {
				t.Fatal("database reopen repeated attachment side effects", effects.Load())
			}
		})
	}
}

func TestCoreImageGenerationReplacementJoinsActualCleanup(t *testing.T) {
	entered, canceled, release, replaced := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	var calls atomic.Int32
	var replacementLaunched atomic.Bool
	images := imageProcessorFunc(func(ctx context.Context, _ storage.Attachment, k func(provider.ContentPart) error) error {
		if e := k(syntheticImagePart()); e != nil {
			return e
		}
		if calls.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			close(canceled)
			<-release
			return ctx.Err()
		}
		return nil
	})
	c := imageController(t, modelFunc(reply), images, &fakeChannel{poll: idle})
	imageInbox(t, c, "replace-held", "image", "")
	cancel, done := runCore(t, c)
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		cancel()
		waitSignal(t, done)
		if replacementLaunched.Load() {
			waitSignal(t, replaced)
		}
	})
	waitSignal(t, entered)
	replacementLaunched.Store(true)
	go func() {
		defer close(replaced)
		if e := acquire(context.Background(), c.maintenance); e != nil {
			t.Error(e)
			return
		}
		defer func() { <-c.maintenance }()
		c.stopGeneration()
		if e := c.startGeneration(); e != nil {
			t.Error(e)
		}
	}()
	waitSignal(t, canceled)
	select {
	case <-replaced:
		t.Fatal("replacement abandoned image cleanup")
	default:
	}
	if _, e := c.store.History(context.Background(), c.scope, 1); e != nil {
		t.Fatal("database unavailable during cleanup", e)
	}
	imageInbox(t, c, "replace-next", "image", "")
	once.Do(func() { close(release) })
	waitSignal(t, replaced)
	eventually(t, func() bool { return calls.Load() == 2 })
	cancel()
	waitSignal(t, done)
}
