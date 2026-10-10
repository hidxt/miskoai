package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/web"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cliFixture(t *testing.T) config.Config {
	t.Helper()
	for _, k := range []string{"MISKOAI_LISTEN", "MISKOAI_DEEPSEEK_URL", "MISKOAI_MODEL", "MISKOAI_VISION_MODEL", "MISKOAI_WEIXIN_URL", "MISKOAI_MAX_OUTPUT_TOKENS", "MISKOAI_CONTEXT_BYTES", "MISKOAI_CONTEXT_TOKENS", "MISKOAI_CREDENTIALS_FILE"} {
		t.Setenv(k, "")
		if e := os.Unsetenv(k); e != nil {
			t.Fatal(e)
		}
	}
	d := filepath.Join(t.TempDir(), "private")
	if e := privatefs.EnsureDir(d); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MISKOAI_DATA_DIR", d)
	t.Setenv("MISKOAI_ADMIN_PASSWORD", "synthetic-password-123")
	t.Setenv("DEEPSEEK_API_KEY", "synthetic-deepseek-key")
	t.Setenv("OLLAMA_API_KEY", "synthetic-search-key")
	c, e := config.Load()
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func waitCLI(t *testing.T, c <-chan error) error {
	t.Helper()
	select {
	case e := <-c:
		return e
	case <-time.After(10 * time.Second):
		t.Fatal("synthetic shutdown watchdog")
		return nil
	}
}
func waitEvent(t *testing.T, c <-chan struct{}) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(10 * time.Second):
		t.Fatal("synthetic event watchdog")
	}
}

type lifetimeCore struct {
	run   func(context.Context) error
	close func() error
}

func TestServePreCanceledAndCanceledConstruction(t *testing.T) {
	for _, pre := range []bool{true, false} {
		t.Run(fmt.Sprint(pre), func(t *testing.T) {
			cfg := cliFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var ran, closed atomic.Int32
			ready := make(chan struct{})
			close(ready)
			if pre {
				cancel()
			}
			e := serveWith(ctx, cfg, io.Discard, func(config.Config) (serveCore, serveWeb, error) {
				cancel()
				return lifetimeCore{run: func(ctx context.Context) error { ran.Add(1); <-ctx.Done(); return nil }, close: func() error { closed.Add(1); return nil }}, lifetimeWeb{ready: ready, run: func(ctx context.Context) error { <-ctx.Done(); return nil }}, nil
			})
			if pre {
				if !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
			} else if e != nil {
				t.Fatal(e)
			}
			if ran.Load() != 0 || (!pre && closed.Load() != 1) {
				t.Fatal("canceled worker or ownership leak", ran.Load(), closed.Load())
			}
		})
	}
}

type completedReadyWeb struct {
	done     chan struct{}
	returned chan struct{}
	run      func(context.Context) error
}

func (w completedReadyWeb) Run(ctx context.Context) error { defer close(w.returned); return w.run(ctx) }
func (w completedReadyWeb) Ready() <-chan struct{}        { <-w.returned; return w.done }
func TestServeCompletedReadyPreventsWorkers(t *testing.T) {
	cfg := cliFixture(t)
	ready := make(chan struct{})
	close(ready)
	var ran, closed atomic.Int32
	e := serveWith(context.Background(), cfg, io.Discard, func(config.Config) (serveCore, serveWeb, error) {
		return lifetimeCore{run: func(context.Context) error { ran.Add(1); return nil }, close: func() error { closed.Add(1); return nil }}, completedReadyWeb{done: ready, returned: make(chan struct{}), run: func(context.Context) error { return errors.New("synthetic-canary") }}, nil
	})
	if e == nil || e.Error() != "cli_serve" || ran.Load() != 0 || closed.Load() != 1 {
		t.Fatal("historical readiness started workers", e, ran.Load(), closed.Load())
	}
}
func TestServeSiblingResultsAndConstructionFailures(t *testing.T) {
	for _, kind := range []string{"core_error", "core_nil", "web_error", "web_nil", "construct_error", "close_error", "output_error"} {
		t.Run(kind, func(t *testing.T) {
			cfg := cliFixture(t)
			var coreJoined, webJoined, closed atomic.Int32
			ready := make(chan struct{})
			close(ready)
			coreStarted := make(chan struct{})
			out := io.Writer(io.Discard)
			if kind == "output_error" {
				out = maintenanceOutputFailure{}
			}
			e := serveWith(context.Background(), cfg, out, func(config.Config) (serveCore, serveWeb, error) {
				c := lifetimeCore{run: func(ctx context.Context) error {
					defer coreJoined.Add(1)
					close(coreStarted)
					if kind == "core_error" {
						return errors.New("synthetic-canary")
					}
					if kind == "core_nil" {
						return nil
					}
					<-ctx.Done()
					return nil
				}, close: func() error {
					closed.Add(1)
					if kind == "close_error" {
						return errors.New("synthetic-canary")
					}
					return nil
				}}
				if kind == "construct_error" {
					return c, nil, errors.New("synthetic-canary")
				}
				w := lifetimeWeb{ready: ready, run: func(ctx context.Context) error {
					defer webJoined.Add(1)
					if kind == "output_error" {
						<-ctx.Done()
						return nil
					}
					<-coreStarted
					if kind == "web_error" {
						return errors.New("synthetic-canary")
					}
					if kind == "web_nil" || kind == "close_error" {
						return nil
					}
					<-ctx.Done()
					return nil
				}}
				return c, w, nil
			})
			if e == nil || e.Error() != "cli_serve" || closed.Load() != 1 {
				t.Fatal("safe failure/close lost", e, closed.Load())
			}
			if kind != "construct_error" && webJoined.Load() != 1 {
				t.Fatal("Web not joined")
			}
			if kind != "construct_error" && kind != "output_error" && coreJoined.Load() != 1 {
				t.Fatal("Core not joined")
			}
		})
	}
}

type serveModel struct{ calls atomic.Int32 }

func (m *serveModel) Chat(context.Context, []provider.Message, int) (provider.Reply, error) {
	m.calls.Add(1)
	return provider.Reply{}, errors.New("synthetic-only")
}

type serveChannel struct {
	entered, canceled, release chan struct{}
	calls, send                atomic.Int32
}

func (c *serveChannel) RawUpdates(ctx context.Context, _ string) ([]byte, error) {
	c.calls.Add(1)
	close(c.entered)
	<-ctx.Done()
	close(c.canceled)
	<-c.release
	return nil, ctx.Err()
}
func (c *serveChannel) SendText(context.Context, string, string, string, string) error {
	c.send.Add(1)
	return errors.New("synthetic-only")
}
func TestServeActualWorkerAndHandlerJoin(t *testing.T) {
	cfg := cliFixture(t)
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	address := l.Addr().String()
	l.Close()
	t.Setenv("MISKOAI_LISTEN", address)
	cfg, e = config.Load()
	if e != nil {
		t.Fatal(e)
	}
	f, e := privatefs.Create(filepath.Join(cfg.DataDir, "weixin-auth.json"))
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.WriteString(`{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	ce := f.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	model := &serveModel{}
	channel := &serveChannel{entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	c, e := core.New(cfg, core.Dependencies{Model: model, ChannelFactory: func(config.Authorization) (core.Channel, error) { return channel, nil }})
	if e != nil {
		t.Fatal(e)
	}
	admitted := make(chan struct{})
	httpRelease := make(chan struct{})
	closed := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(channel.release); close(httpRelease) }) }
	w, e := web.New(web.Options{Listen: address, Password: cfg.AdminPassword}, web.Application(c), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(admitted)
		<-httpRelease
		io.WriteString(w, "synthetic")
	}))
	if e != nil {
		c.Close()
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	requestDone := make(chan struct{})
	requested := false
	t.Cleanup(func() {
		cancel()
		release()
		if e := waitCLI(t, done); e != nil {
			t.Error(e)
		}
		if requested {
			waitEvent(t, requestDone)
		}
	})
	go func() {
		done <- serveWith(ctx, cfg, io.Discard, func(config.Config) (serveCore, serveWeb, error) {
			return lifetimeCore{run: c.Run, close: func() error { close(closed); return c.Close() }}, w, nil
		})
	}()
	waitEvent(t, channel.entered)
	requested = true
	go func() {
		defer close(requestDone)
		client := http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 10 * time.Second}
		response, e := client.Get("http://" + address + "/")
		if e == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		client.CloseIdleConnections()
	}()
	waitEvent(t, admitted)
	cancel()
	waitEvent(t, channel.canceled)
	select {
	case <-closed:
		t.Fatal("closed before actual worker/HTTP handler returned")
	case <-time.After(30 * time.Millisecond):
	}
	release()
	e = waitCLI(t, done)
	done <- e
	if e != nil {
		t.Fatal(e)
	}
	waitEvent(t, closed)
	if model.calls.Load() != 0 || channel.send.Load() != 0 || channel.calls.Load() != 1 {
		t.Fatal("unexpected synthetic delegate calls")
	}
	t.Log("external_calls=0 synthetic_poll_calls=1 model_calls=0 send_calls=0 actual_worker_join=true actual_handler_join=true")
}

func (c lifetimeCore) Run(ctx context.Context) error { return c.run(ctx) }
func (c lifetimeCore) Close() error                  { return c.close() }

type lifetimeWeb struct {
	run   func(context.Context) error
	ready <-chan struct{}
}

func (w lifetimeWeb) Run(ctx context.Context) error { return w.run(ctx) }
func (w lifetimeWeb) Ready() <-chan struct{}        { return w.ready }

func TestServeBindFailureUnwinds(t *testing.T) {
	cfg := cliFixture(t)
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	t.Setenv("MISKOAI_LISTEN", l.Addr().String())
	cfg, e = config.Load()
	if e != nil {
		t.Fatal(e)
	}
	var ran atomic.Int32
	constructed := false
	e = serveWith(context.Background(), cfg, io.Discard, func(c config.Config) (serveCore, serveWeb, error) {
		constructed = true
		real, e := core.New(c, core.Dependencies{})
		if e != nil {
			return nil, nil, e
		}
		w, e := web.New(web.Options{Listen: c.Listen, Password: c.AdminPassword}, web.Application(real), web.Assets())
		if e != nil {
			real.Close()
			return nil, nil, e
		}
		return lifetimeCore{run: func(ctx context.Context) error { ran.Add(1); return real.Run(ctx) }, close: real.Close}, w, nil
	})
	if e == nil || e.Error() != "cli_serve" || !constructed || ran.Load() != 0 {
		t.Fatalf("binding must construct/unwind without Core.Run: %v %v %d", e, constructed, ran.Load())
	}
	lock, e := maintenance.Acquire(cfg.DataDir)
	if e != nil {
		t.Fatal("lock retained", e)
	}
	if e = lock.Close(); e != nil {
		t.Fatal(e)
	}
	real, e := core.New(cfg, core.Dependencies{})
	if e != nil {
		t.Fatal("store not reusable", e)
	}
	if e = real.Close(); e != nil {
		t.Fatal(e)
	}
}

func TestServeCommandOccupiedPort(t *testing.T) {
	cfg := cliFixture(t)
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	t.Setenv("MISKOAI_LISTEN", l.Addr().String())
	var out bytes.Buffer
	e = RunContext(context.Background(), []string{"serve"}, &out)
	if e == nil || e.Error() != "cli_serve" || out.Len() != 0 {
		t.Fatal("occupied production command", e, out.String())
	}
	lock, e := maintenance.Acquire(cfg.DataDir)
	if e != nil {
		t.Fatal("production command retained lock", e)
	}
	if e = lock.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestServeSignalJoins(t *testing.T) {
	cfg := cliFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	close(ready)
	started := make(chan struct{})
	held := make(chan struct{})
	closed := make(chan struct{})
	result := make(chan error, 1)
	release := func() {
		select {
		case <-held:
		default:
			close(held)
		}
	}
	t.Cleanup(func() { cancel(); release(); waitCLI(t, result) })
	go func() {
		result <- serveWith(ctx, cfg, io.Discard, func(config.Config) (serveCore, serveWeb, error) {
			return lifetimeCore{run: func(ctx context.Context) error { close(started); <-ctx.Done(); <-held; return nil }, close: func() error { close(closed); return nil }}, lifetimeWeb{ready: ready, run: func(ctx context.Context) error { <-ctx.Done(); return nil }}, nil
		})
	}()
	waitEvent(t, started)
	cancel()
	select {
	case <-closed:
		t.Fatal("Close before actual worker return")
	case <-time.After(30 * time.Millisecond):
	}
	release()
	e := waitCLI(t, result)
	result <- e
	if e != nil {
		t.Fatal(e)
	}
	waitEvent(t, closed)
}
func TestServeReadinessOrdering(t *testing.T) {
	cfg := cliFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- serveWith(ctx, cfg, io.Discard, func(config.Config) (serveCore, serveWeb, error) {
			return lifetimeCore{run: func(ctx context.Context) error { close(started); <-ctx.Done(); return nil }, close: func() error { return nil }}, lifetimeWeb{ready: ready, run: func(ctx context.Context) error { <-ctx.Done(); return nil }}, nil
		})
	}()
	t.Cleanup(func() { cancel(); waitCLI(t, result) })
	select {
	case <-started:
		t.Fatal("worker before bind")
	case <-time.After(30 * time.Millisecond):
	}
	close(ready)
	waitEvent(t, started)
}
func TestRunContextRejectsInvalidContextBeforeConfig(t *testing.T) {
	t.Setenv("MISKOAI_LISTEN", "invalid-synthetic-address")
	if e := RunContext(nil, []string{"version"}, io.Discard); e == nil || e.Error() != "cli_context" {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := RunContext(ctx, []string{"init"}, io.Discard); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	var b bytes.Buffer
	if e := RunContext(context.Background(), []string{"help"}, &b); e != nil {
		t.Fatal(e)
	}
}
