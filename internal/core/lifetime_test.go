package core

import (
	"context"
	"errors"
	"fmt"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/provider"
	"sync/atomic"
	"testing"
	"time"
)

type modelFunc func(context.Context, []provider.Message, int) (provider.Reply, error)

func (f modelFunc) Chat(c context.Context, m []provider.Message, n int) (provider.Reply, error) {
	return f(c, m, n)
}

type fakeChannel struct {
	poll func(context.Context, string) ([]byte, error)
	send func(context.Context, string, string, string, string) error
}

func (f *fakeChannel) RawUpdates(c context.Context, s string) ([]byte, error) { return f.poll(c, s) }
func (f *fakeChannel) SendText(c context.Context, a, b, d, e string) error {
	if f.send == nil {
		return nil
	}
	return f.send(c, a, b, d, e)
}
func authorized(t *testing.T, m modelFunc, ch *fakeChannel) *Controller {
	t.Helper()
	cfg := fixtureConfig(t)
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	c, e := New(cfg, Dependencies{Model: m, ChannelFactory: func(config.Authorization) (Channel, error) { return ch, nil }})
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
func idle(ctx context.Context, _ string) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }
func reply(context.Context, []provider.Message, int) (provider.Reply, error) {
	return provider.Reply{Text: "synthetic answer", FinishReason: "stop", Usage: provider.Usage{TotalTokens: 3}}, nil
}
func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("actual job boundary timed out")
	}
}
func eventually(t *testing.T, f func() bool) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if f() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition timed out")
}
func runCore(t *testing.T, c *Controller) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if e := c.Run(ctx); e != nil && !errors.Is(e, context.Canceled) {
			t.Error(e)
		}
	}()
	t.Cleanup(func() { cancel(); waitSignal(t, done) })
	return cancel, done
}
func seed(t *testing.T, c *Controller, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		id := fmt.Sprint("seed-", i)
		if ok, e := c.store.ClaimMessage(ctx, c.scope, id, "synthetic user"); e != nil || !ok {
			t.Fatal(ok, e)
		}
		if e := c.store.SetMessageState(ctx, c.scope, id, "sending"); e != nil {
			t.Fatal(e)
		}
		if e := c.store.CompleteMessage(ctx, c.scope, id, "synthetic assistant"); e != nil {
			t.Fatal(e)
		}
	}
}
func inbox(t *testing.T, c *Controller) {
	t.Helper()
	_, e := c.store.RecordPoll(context.Background(), c.scope, "", []byte(`{"ret":0,"msgs":[{"message_id":123,"from_user_id":"fixture-user","to_user_id":"fixture-account","message_type":1,"message_state":2,"context_token":"synthetic-context","item_list":[{"type":1,"text_item":{"text":"hello"}}]}],"get_updates_buf":"next"}`))
	if e != nil {
		t.Fatal(e)
	}
}
func TestCoreSharedModelAndSummaryObserver(t *testing.T) {
	var chats, summaries atomic.Int32
	c := authorized(t, modelFunc(func(ctx context.Context, m []provider.Message, n int) (provider.Reply, error) {
		if n == 1024 {
			summaries.Add(1)
			return provider.Reply{Text: `{"summary":"synthetic saved","candidates":[]}`, FinishReason: "stop"}, nil
		}
		chats.Add(1)
		return reply(ctx, m, n)
	}), &fakeChannel{poll: idle})
	seed(t, c, 15)
	inbox(t, c)
	cancel, done := runCore(t, c)
	eventually(t, func() bool { return c.Status().Summary.Completed == 1 })
	cancel()
	waitSignal(t, done)
	if chats.Load() != 1 || summaries.Load() != 1 {
		t.Fatal("shared delegate/observer not invoked", chats.Load(), summaries.Load())
	}
	if c.Status().Metrics.ChatAttempts != 1 || c.Status().Metrics.SummaryAttempts != 1 {
		t.Fatal("attempt labels incorrect")
	}
}
func TestCoreCancelJoinsActualJob(t *testing.T) {
	entered, observed, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	c := authorized(t, modelFunc(func(ctx context.Context, _ []provider.Message, _ int) (provider.Reply, error) {
		close(entered)
		<-ctx.Done()
		close(observed)
		<-release
		return provider.Reply{}, ctx.Err()
	}), &fakeChannel{poll: idle})
	inbox(t, c)
	cancel, done := runCore(t, c)
	waitSignal(t, entered)
	cancel()
	waitSignal(t, observed)
	select {
	case <-done:
		t.Fatal("returned before actual admitted model joined")
	default:
	}
	close(release)
	waitSignal(t, done)
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestCoreAuthExpiryPausesGenerationAndManagement(t *testing.T) {
	for _, mode := range []string{"poll", "send"} {
		t.Run(mode, func(t *testing.T) {
			summaryEntered, summaryJoined, expire := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var polls atomic.Int32
			ch := &fakeChannel{poll: func(ctx context.Context, _ string) ([]byte, error) {
				polls.Add(1)
				if mode == "poll" {
					select {
					case <-expire:
						return nil, weixin.ErrAuthExpired
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				return idle(ctx, "")
			}}
			if mode == "send" {
				ch.send = func(ctx context.Context, _, _, _, _ string) error {
					select {
					case <-expire:
						return weixin.ErrAuthExpired
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
			c := authorized(t, modelFunc(func(ctx context.Context, m []provider.Message, n int) (provider.Reply, error) {
				if n == 1024 {
					close(summaryEntered)
					<-ctx.Done()
					close(summaryJoined)
					return provider.Reply{}, ctx.Err()
				}
				return reply(ctx, m, n)
			}), ch)
			seed(t, c, 16)
			if mode == "send" {
				inbox(t, c)
			}
			cancel, done := runCore(t, c)
			eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.generation != nil })
			c.mu.Lock()
			g := c.generation
			c.mu.Unlock()
			g.summary.Wake()
			waitSignal(t, summaryEntered)
			close(expire)
			waitSignal(t, summaryJoined)
			waitSignal(t, g.done)
			eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.generation == nil })
			if c.Status().ChannelState != "authorization_expired" {
				t.Fatal("expiry reason lost")
			}
			if _, e := c.FactsPage(context.Background(), "", 0, 8); e != nil {
				t.Fatal("management unavailable", e)
			}
			before := polls.Load()
			if e := c.ClearMemory(context.Background()); e != nil {
				t.Fatal(e)
			}
			if polls.Load() != before {
				t.Fatal("expired remote work restarted")
			}
			select {
			case <-done:
				t.Fatal("management parent ended on expiry")
			default:
			}
			cancel()
			waitSignal(t, done)
		})
	}
}
func TestRunDoesNotCancelReplacementGeneration(t *testing.T) {
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
	cancel, done := runCore(t, c)
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.generation != nil })
	if e := acquire(context.Background(), c.maintenance); e != nil {
		t.Fatal(e)
	}
	c.mu.Lock()
	old := c.generation
	c.mu.Unlock()
	c.stopGeneration()
	if e := c.startGeneration(); e != nil {
		t.Fatal(e)
	}
	c.mu.Lock()
	fresh := c.generation
	c.mu.Unlock()
	<-c.maintenance
	waitSignal(t, old.done)
	time.Sleep(30 * time.Millisecond)
	select {
	case <-fresh.done:
		t.Fatal("Run's old-generation observer canceled replacement")
	default:
	}
	cancel()
	waitSignal(t, done)
}

func TestCanceledBeforeRunStartsNoGeneration(t *testing.T) {
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: func(context.Context, string) ([]byte, error) {
		t.Error("poll started after prior cancellation")
		return nil, ErrUnavailable
	}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := c.Run(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal("canceled admission must return cancellation", e)
	}
	c.mu.Lock()
	started, g := c.started, c.generation
	c.mu.Unlock()
	if started || g != nil {
		t.Fatal("canceled admission created a worker lifetime")
	}
}
