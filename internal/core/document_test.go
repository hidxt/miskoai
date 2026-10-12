package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/document"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

type documentProcessorFunc func(context.Context, storage.Attachment, func(document.Result) error) error

func (f documentProcessorFunc) WithDocument(ctx context.Context, a storage.Attachment, k func(document.Result) error) error {
	return f(ctx, a, k)
}
func syntheticDocumentResult() document.Result {
	return document.Result{Text: "synthetic external document", Pages: 1, Format: document.TXT}
}

func documentDependencies(m modelFunc, p agent.DocumentProcessor, ch *fakeChannel) Dependencies {
	return Dependencies{Model: m, Documents: p, ChannelFactory: func(config.Authorization) (Channel, error) { return ch, nil }}
}

func TestCoreDocumentCleanupFailureRetainsTextUsage(t *testing.T) {
	var sends atomic.Int32
	c := documentController(t, modelFunc(reply), documentProcessorFunc(func(ctx context.Context, _ storage.Attachment, k func(document.Result) error) error {
		if e := k(syntheticDocumentResult()); e != nil {
			return e
		}
		return errors.New("synthetic cleanup failed")
	}), &fakeChannel{poll: idle, send: func(context.Context, string, string, string, string) error { sends.Add(1); return nil }})
	documentInbox(t, c, "cleanup-document", "", "fixture.txt")
	cancel, done := runCore(t, c)
	eventually(t, func() bool { return c.Status().Channel.Processed == 1 })
	cancel()
	waitSignal(t, done)
	m := c.Status().Metrics
	if m.ChatAttempts != 1 || m.VisionAttempts != 0 || m.SuccessfulUsage.TotalTokens != 3 || sends.Load() != 1 {
		t.Fatal("postmodel cleanup lost counted usage or sent twice", m, sends.Load())
	}
}

func TestCoreDocumentUnsupportedSingleReply(t *testing.T) {
	var modelCalls, sends atomic.Int32
	c := documentController(t, modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		modelCalls.Add(1)
		return provider.Reply{}, nil
	}), documentProcessorFunc(func(_ context.Context, a storage.Attachment, _ func(document.Result) error) error {
		if a.FileName != "fixture.exe" {
			t.Error("unsupported descriptor changed")
		}
		return document.ErrUnsupported
	}), &fakeChannel{poll: idle, send: func(_ context.Context, _, _, _, text string) error {
		sends.Add(1)
		if !strings.Contains(text, "暂不支持") {
			t.Error("unsupported reply not honest")
		}
		return nil
	}})
	documentInbox(t, c, "unsupported-document", "", "fixture.exe")
	cancel, done := runCore(t, c)
	eventually(t, func() bool { return c.Status().Channel.Processed == 1 })
	cancel()
	waitSignal(t, done)
	if modelCalls.Load() != 0 || sends.Load() != 1 || c.Status().Metrics.ChatAttempts != 0 {
		t.Fatal("unsupported file modeled or sent twice")
	}
}

func TestCoreDocumentDatabaseReopenDoesNotReplay(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(map[bool]string{false: "ack", true: "ambiguous"}[ambiguous], func(t *testing.T) {
			var effects, polls atomic.Int32
			ch := &fakeChannel{poll: func(ctx context.Context, s string) ([]byte, error) { polls.Add(1); return idle(ctx, s) }, send: func(context.Context, string, string, string, string) error {
				effects.Add(1)
				if ambiguous {
					return errors.New("synthetic uncertain send")
				}
				return nil
			}}
			model := modelFunc(func(ctx context.Context, m []provider.Message, n int) (provider.Reply, error) {
				effects.Add(1)
				return reply(ctx, m, n)
			})
			processor := documentProcessorFunc(func(ctx context.Context, _ storage.Attachment, k func(document.Result) error) error {
				effects.Add(1)
				return k(syntheticDocumentResult())
			})
			c := documentController(t, model, processor, ch)
			documentInbox(t, c, "reopen-document", "genuine caption", "fixture.txt")
			cancel, done := runCore(t, c)
			eventually(t, func() bool { return c.Status().Channel.Processed == 1 })
			cancel()
			waitSignal(t, done)
			if !ambiguous {
				h, e := c.store.History(context.Background(), c.scope, 16)
				if e != nil || len(h) != 2 || h[0].Content != "genuine caption" {
					t.Fatal("caption provenance changed", h, e)
				}
			}
			cfg := c.cfg
			if e := c.Close(); e != nil {
				t.Fatal(e)
			}
			next, e := New(cfg, documentDependencies(model, processor, ch))
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = next.Close() })
			documentInbox(t, next, "reopen-document", "genuine caption", "fixture.txt")
			prior := polls.Load()
			nextCancel, nextDone := runCore(t, next)
			eventually(t, func() bool { return polls.Load() > prior })
			nextCancel()
			waitSignal(t, nextDone)
			pending, e := next.store.PendingInbox(context.Background(), next.scope, 1)
			if e != nil || len(pending) != 0 || next.Status().Channel.Processed != 0 {
				t.Fatal("duplicate entered reopened handler", pending, e)
			}
			claimed, e := next.store.ClaimMessage(context.Background(), next.scope, "reopen-document", "genuine caption")
			if e != nil || claimed || effects.Load() != 3 {
				t.Fatal("reopened claim/effects changed", claimed, e, effects.Load())
			}
		})
	}
}

type borrowedDocuments struct {
	process documentProcessorFunc
	closes  atomic.Int32
}

func (p *borrowedDocuments) WithDocument(ctx context.Context, a storage.Attachment, k func(document.Result) error) error {
	return p.process(ctx, a, k)
}
func (p *borrowedDocuments) Close() error { p.closes.Add(1); return nil }

func TestCoreDocumentMaintenanceJoinsActualCleanup(t *testing.T) {
	for _, operation := range []string{"close", "replace", "restore"} {
		t.Run(operation, func(t *testing.T) {
			entered, canceled, release, operationDone := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			var launched, dbAvailable atomic.Bool
			var calls atomic.Int32
			var heldStore *storage.Store
			p := &borrowedDocuments{process: documentProcessorFunc(func(ctx context.Context, _ storage.Attachment, k func(document.Result) error) error {
				if e := k(syntheticDocumentResult()); e != nil {
					return e
				}
				if calls.Add(1) != 1 {
					return nil
				}
				close(entered)
				<-ctx.Done()
				close(canceled)
				<-release
				_, e := heldStore.History(context.Background(), storage.Scope{Account: "fixture-account", User: "fixture-user"}, 1)
				dbAvailable.Store(e == nil)
				return ctx.Err()
			})}
			c := documentController(t, modelFunc(reply), p, &fakeChannel{poll: idle})
			heldStore = c.store
			var backup []byte
			if operation == "restore" {
				d, e := c.PrepareBackup(context.Background())
				if e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() { _ = d.Close() })
				backup, e = io.ReadAll(d)
				closeErr := d.Close()
				if e != nil || closeErr != nil {
					t.Fatal(e, closeErr)
				}
			}
			documentInbox(t, c, "held-document", "", "fixture.txt")
			cancel, runDone := runCore(t, c)
			// Register release/cancel/join before any assertion on the held operation.
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				cancel()
				waitSignal(t, runDone)
				if launched.Load() {
					waitSignal(t, operationDone)
				}
			})
			waitSignal(t, entered)
			launched.Store(true)
			go func() {
				defer close(operationDone)
				switch operation {
				case "close":
					if e := c.Close(); e != nil {
						t.Error(e)
					}
				case "restore":
					if _, e := c.Restore(context.Background(), bytes.NewReader(backup)); e != nil {
						t.Error(e)
					}
				case "replace":
					if e := acquire(context.Background(), c.maintenance); e != nil {
						t.Error(e)
						return
					}
					defer func() { <-c.maintenance }()
					c.stopGeneration()
					if e := c.startGeneration(); e != nil {
						t.Error(e)
					}
				}
			}()
			waitSignal(t, canceled)
			select {
			case <-operationDone:
				t.Fatal("maintenance abandoned actual document cleanup")
			default:
			}
			if _, e := heldStore.History(context.Background(), c.scope, 1); e != nil {
				t.Fatal("database closed while cleanup held", e)
			}
			if l, e := maintenance.Acquire(c.cfg.DataDir); e == nil {
				_ = l.Close()
				t.Fatal("ownership lock released while cleanup held")
			}
			if operation == "replace" {
				// Persist before the next worker's initial durable inbox scan.
				documentInbox(t, c, "next-document", "", "fixture.txt")
			}
			releaseOnce.Do(func() { close(release) })
			waitSignal(t, operationDone)
			if !dbAvailable.Load() || p.closes.Load() != 0 {
				t.Fatal("database or borrowed ownership closed before join", dbAvailable.Load(), p.closes.Load())
			}
			if operation == "close" {
				waitSignal(t, runDone)
				l, e := maintenance.Acquire(c.cfg.DataDir)
				if e != nil {
					t.Fatal("lock unavailable after actual close", e)
				}
				if e = l.Close(); e != nil {
					t.Fatal(e)
				}
			} else {
				if operation == "restore" {
					if c.store == heldStore {
						t.Fatal("restore failed to replace store")
					}
					documentInbox(t, c, "next-document", "", "fixture.txt")
					if e := c.ResumeAfterReconciliation(context.Background()); e != nil {
						t.Fatal(e)
					}
				}
				if c.documents != p || c.cdn != nil {
					t.Fatal("replacement replaced borrowed processor or created downloader")
				}
				eventually(t, func() bool { return calls.Load() == 2 })
				cancel()
				waitSignal(t, runDone)
			}
		})
	}
}

func TestCoreDocumentProductionResourcesSurviveRestoreAndGeneration(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg.DeepSeekKey = "synthetic-key"
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	c, e := New(cfg, Dependencies{ChannelFactory: func(config.Authorization) (Channel, error) { return &fakeChannel{poll: idle}, nil }})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close() })
	processor, cdn, text, vision := c.images, c.cdn, c.model, c.vision
	parser := reflect.ValueOf(processor).Elem().FieldByName("parser").Pointer()
	if parser == 0 || any(c.documents) != any(processor) {
		t.Fatal("production persistent resources missing")
	}
	d, e := c.PrepareBackup(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = d.Close() })
	data, e := io.ReadAll(d)
	ce := d.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	cancel, done := runCore(t, c)
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.generation != nil })
	desired, _, _ := c.Settings()
	desired.MaxOutput++
	if e = c.SaveSettings(context.Background(), desired); e != nil {
		t.Fatal(e)
	}
	if e = acquire(context.Background(), c.maintenance); e != nil {
		t.Fatal(e)
	}
	c.stopGeneration()
	e = c.startGeneration()
	<-c.maintenance
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Restore(context.Background(), bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	if e = c.ResumeAfterReconciliation(context.Background()); e != nil {
		t.Fatal(e)
	}
	if c.images != processor || any(c.documents) != any(processor) || c.cdn != cdn || c.model != text || c.vision != vision || reflect.ValueOf(c.images).Elem().FieldByName("parser").Pointer() != parser {
		t.Fatal("replacement recreated media/parser/model resource")
	}
	cancel()
	waitSignal(t, done)
}

func TestCoreDocumentInjectionCompatibility(t *testing.T) {
	p := &borrowedDocuments{process: documentProcessorFunc(func(context.Context, storage.Attachment, func(document.Result) error) error { return nil })}
	for _, withModel := range []bool{false, true} {
		t.Run(map[bool]string{false: "production-text", true: "borrowed-text"}[withModel], func(t *testing.T) {
			cfg := fixtureConfig(t)
			d := Dependencies{Documents: p}
			if withModel {
				d.Model = modelFunc(reply)
			}
			c, e := New(cfg, d)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = c.Close() })
			if c.documents != p {
				t.Fatal("explicit Documents replaced")
			}
			if withModel && (c.cdn != nil || c.vision != nil || c.images != nil) {
				t.Fatal("text-only injection created unintended downloader")
			}
			if e = c.Close(); e != nil {
				t.Fatal(e)
			}
			if p.closes.Load() != 0 {
				t.Fatal("borrowed Documents closed")
			}
		})
	}
	cfg := fixtureConfig(t)
	c, e := New(cfg, Dependencies{Model: modelFunc(reply)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close() })
	if c.documents != nil || c.cdn != nil {
		t.Fatal("legacy text-only dependency gained downloader")
	}
}

func TestCoreDocumentDoesNotRelaxImageDependencyPair(t *testing.T) {
	for _, visionOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "images", true: "vision"}[visionOnly], func(t *testing.T) {
			cfg := fixtureConfig(t)
			d := Dependencies{Model: modelFunc(reply), Documents: documentProcessorFunc(func(context.Context, storage.Attachment, func(document.Result) error) error { return nil })}
			if visionOnly {
				d.Vision = modelFunc(reply)
			} else {
				d.Images = imageProcessorFunc(func(context.Context, storage.Attachment, func(provider.ContentPart) error) error { return nil })
			}
			c, e := New(cfg, d)
			if e == nil {
				_ = c.Close()
				t.Fatal("Documents allowed partial image dependency")
			}
			if _, e = os.Lstat(filepath.Join(cfg.DataDir, "miskoai.db")); !os.IsNotExist(e) {
				t.Fatal("invalid dependency created database", e)
			}
		})
	}
}
func documentController(t *testing.T, m modelFunc, p agent.DocumentProcessor, ch *fakeChannel) *Controller {
	t.Helper()
	cfg := fixtureConfig(t)
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	c, e := New(cfg, documentDependencies(m, p, ch))
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
func documentInbox(t *testing.T, c *Controller, id, caption, name string) {
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
	a := storage.Attachment{Kind: "file", FileName: name, Query: "synthetic-query", MediaKey: "AAAAAAAAAAAAAAAAAAAAAA=="}
	e = c.store.ResolvePoll(ctx, c.scope, f, "cursor", []storage.InboxEntry{{Scope: c.scope, MessageID: id, Text: caption, ContextToken: "synthetic-context", ReceivedAt: time.Now().UTC(), Attachment: &a}})
	if e != nil {
		t.Fatal(e)
	}
}
func TestCoreDocumentDurableEmptyCaptionAndMetrics(t *testing.T) {
	var processors, models, sends atomic.Int32
	ch := &fakeChannel{poll: idle, send: func(_ context.Context, _, _, _, text string) error {
		sends.Add(1)
		if !strings.HasPrefix(text, "文档资料：使用提取文本 UTF-8 字节 [0,") {
			t.Error("visible excerpt label lost")
		}
		return nil
	}}
	var c *Controller
	c = documentController(t, modelFunc(func(ctx context.Context, m []provider.Message, n int) (provider.Reply, error) {
		models.Add(1)
		return reply(ctx, m, n)
	}), documentProcessorFunc(func(ctx context.Context, a storage.Attachment, k func(document.Result) error) error {
		processors.Add(1)
		claimed, e := c.store.ClaimMessage(ctx, c.scope, "18446744073709551615", "")
		if e != nil || claimed {
			t.Error("document processing before durable claim", claimed, e)
		}
		if a.FileName != "fixture.txt" {
			t.Error("descriptor lost")
		}
		return k(syntheticDocumentResult())
	}), ch)
	documentInbox(t, c, "18446744073709551615", "", "fixture.txt")
	cancel, done := runCore(t, c)
	eventually(t, func() bool { return c.Status().Channel.Processed == 1 })
	cancel()
	waitSignal(t, done)
	if processors.Load() != 1 || models.Load() != 1 || sends.Load() != 1 {
		t.Fatalf("effects processor=%d model=%d sends=%d", processors.Load(), models.Load(), sends.Load())
	}
	metrics := c.Status().Metrics
	if metrics.ChatAttempts != 1 || metrics.VisionAttempts != 0 || metrics.SuccessfulUsage.TotalTokens != 3 {
		t.Fatal("document accounting changed", metrics)
	}
	h, e := c.store.History(context.Background(), c.scope, 16)
	if e != nil || len(h) != 0 {
		t.Fatal("empty caption became USER evidence", h, e)
	}
}
func TestCoreDocumentProductionSharedProcessor(t *testing.T) {
	c, e := New(fixtureConfig(t), Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close() })
	p, ok := c.images.(*attachment.ImageProcessor)
	if !ok {
		t.Fatal("missing production processor")
	}
	// Observe pointer identity only; private parser state remains private.
	parser := reflect.ValueOf(p).Elem().FieldByName("parser").Pointer()
	if parser == 0 {
		t.Fatal("production processor has no persistent document parser")
	}
	f := reflect.ValueOf(c).Elem().FieldByName("documents")
	if !f.IsValid() || f.IsNil() {
		t.Fatal("missing production document interface")
	}
	if f.Elem().Pointer() != reflect.ValueOf(p).Pointer() {
		t.Fatal("documents and images do not share the same outer processor")
	}
}
