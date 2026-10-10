package web

import (
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/maintenance"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type faultDownload struct {
	reads, closed int
	fail          bool
	size          int64
	closeError    bool
	done          chan struct{}
}

func (d *faultDownload) metadata() (string, int64) {
	if d.size != 0 {
		return "miskoai-memory.json", d.size
	}
	return "miskoai-memory.json", 4
}
func (d *faultDownload) Read(p []byte) (int, error) {
	d.reads++
	if d.fail {
		return 0, errors.New("synthetic private path")
	}
	if d.reads == 1 {
		return copy(p, "data"), nil
	}
	return 0, io.EOF
}
func (d *faultDownload) Close() error {
	d.closed++
	if d.done != nil {
		close(d.done)
	}
	if d.closeError {
		return errors.New("close private canary")
	}
	return nil
}

type systemFixture struct {
	managementBackend
	download      *faultDownload
	restoreBytes  int64
	resume, clear int
}

func (f *systemFixture) prepareDownload(ctx context.Context, _ bool) (managementDownload, error) {
	return f.download, nil
}
func (f *systemFixture) Restore(ctx context.Context, r io.Reader) (maintenance.RestoreResult, error) {
	n, e := io.Copy(io.Discard, r)
	f.restoreBytes = n
	return maintenance.RestoreResult{PreviousRetained: true, ChannelPaused: true}, e
}

type repeatedReader struct {
	remaining int64
	read      int64
}

func (r *repeatedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	for i := 0; i < n; i++ {
		p[i] = 'a'
	}
	r.remaining -= int64(n)
	r.read += int64(n)
	return n, nil
}
func TestRestoreStreamBoundsAndTypes(t *testing.T) {
	for _, tc := range []struct {
		name, kind, method string
		size, declared     int64
		want               int
	}{{"wrong-type", "application/json", "POST", 10, -1, 400}, {"wrong-method", "application/octet-stream", "GET", 10, -1, 405}, {"known-oversize", "application/octet-stream", "POST", 256<<20 + 1, 256<<20 + 1, 400}, {"stream-oversize", "application/octet-stream", "POST", 256<<20 + 10, -1, 400}, {"accepted", "application/octet-stream", "POST", 10, -1, 200}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &systemFixture{}
			s := newTestServer(t, newApplication(f))
			cookie, csrf := login(t, s)
			input := &repeatedReader{remaining: tc.size}
			r := httptest.NewRequest(tc.method, "/api/restore", input)
			r.ContentLength = tc.declared
			r.Host = testHost
			r.AddCookie(cookie)
			r.Header.Set("Origin", testOrigin)
			r.Header.Set("X-MiskoAI-CSRF", csrf)
			r.Header.Set("Content-Type", tc.kind)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc.name != "stream-oversize" && tc.name != "accepted" && input.read != 0 {
				t.Fatal("read before validation")
			}
			if input.read > 256<<20+1 {
				t.Fatal("unbounded read", input.read)
			}
		})
	}
}
func (f *systemFixture) ResumeAfterReconciliation(context.Context) error { f.resume++; return nil }
func (f *systemFixture) ClearMemory(context.Context) error               { f.clear++; return nil }

type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header       { return w.header }
func (w *failingWriter) WriteHeader(int)           {}
func (w *failingWriter) Write([]byte) (int, error) { return 0, errors.New("write canary") }
func TestDownloadCancelCleanup(t *testing.T) {
	for _, mode := range []string{"success", "read_error", "write_error", "canceled", "close_error"} {
		t.Run(mode, func(t *testing.T) {
			d := &faultDownload{fail: mode == "read_error", closeError: mode == "close_error"}
			f := &systemFixture{download: d}
			s := newTestServer(t, newApplication(f))
			cookie, csrf := login(t, s)
			r := httptest.NewRequest("POST", "/api/export", strings.NewReader(`{}`))
			r.Host = testHost
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", testOrigin)
			r.Header.Set("X-MiskoAI-CSRF", csrf)
			r.AddCookie(cookie)
			if mode == "canceled" {
				ctx, cancel := context.WithCancel(r.Context())
				cancel()
				r = r.WithContext(ctx)
			}
			aborted := false
			if mode == "write_error" {
				aborted = serveForAbort(s.Handler(), &failingWriter{header: make(http.Header)}, r)
			} else {
				w := httptest.NewRecorder()
				aborted = serveForAbort(s.Handler(), w, r)
				if mode == "success" && (w.Code != 200 || w.Body.String() != "data" || w.Header().Get("Content-Disposition") != `attachment; filename="miskoai-memory.json"`) {
					t.Fatal(w.Code, w.Body.String(), w.Header())
				}
			}
			if mode == "read_error" || mode == "write_error" || mode == "close_error" {
				if !aborted {
					t.Fatal("committed stream failure was not aborted")
				}
			}
			if d.closed != 1 {
				t.Fatalf("close count %d", d.closed)
			}
		})
	}
}
func serveForAbort(h http.Handler, w http.ResponseWriter, r *http.Request) (aborted bool) {
	defer func() {
		if p := recover(); p != nil {
			if p != http.ErrAbortHandler {
				panic(p)
			}
			aborted = true
		}
	}()
	h.ServeHTTP(w, r)
	return false
}
func TestDownloadHTTPTruncationAndPreCanceledContext(t *testing.T) {
	for _, mode := range []string{"truncated", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			d := &faultDownload{size: 8, done: make(chan struct{})}
			f := &systemFixture{download: d}
			s := newTestServer(t, newApplication(f))
			cookie, csrf := login(t, s)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			handler := s.Handler()
			if mode == "canceled" {
				handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					handlerRequest := r.WithContext(ctx)
					cancel()
					s.Handler().ServeHTTP(w, handlerRequest)
				})
			}
			ts := httptest.NewUnstartedServer(handler)
			ts.Config.ErrorLog = log.New(io.Discard, "", 0)
			ts.Start()
			defer ts.Close()
			r, e := http.NewRequest("POST", ts.URL+"/api/export", strings.NewReader(`{}`))
			if e != nil {
				t.Fatal(e)
			}
			r.Host = testHost
			r.AddCookie(cookie)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", testOrigin)
			r.Header.Set("X-MiskoAI-CSRF", csrf)
			client := ts.Client()
			client.Timeout = 5 * time.Second
			resp, e := client.Do(r)
			if e != nil {
				if mode != "truncated" {
					t.Fatal(e)
				}
			} else {
				body, readErr := io.ReadAll(resp.Body)
				ce := resp.Body.Close()
				if ce != nil {
					t.Fatal(ce)
				}
				if mode == "truncated" {
					if readErr == nil || string(body) != "data" {
						t.Fatal("non-truncated failure", string(body), readErr)
					}
				} else {
					if resp.StatusCode != 408 || readErr != nil {
						t.Fatal(resp.StatusCode, readErr)
					}
				}
			}
			select {
			case <-d.done:
			case <-time.After(time.Second):
				t.Fatal("download not closed")
			}
			if d.closed != 1 {
				t.Fatal(d.closed)
			}
		})
	}
}

type cancelDownload struct {
	ctx      context.Context
	sent     bool
	done     chan struct{}
	canceled chan struct{}
	release  chan struct{}
	closed   atomic.Int32
}

func (d *cancelDownload) metadata() (string, int64) { return "miskoai-memory.json", 16384 }
func (d *cancelDownload) Read(p []byte) (int, error) {
	if !d.sent {
		d.sent = true
		n := 8192
		if len(p) < n {
			n = len(p)
		}
		for i := 0; i < n; i++ {
			p[i] = 'x'
		}
		return n, nil
	}
	<-d.ctx.Done()
	close(d.canceled)
	<-d.release
	return 0, d.ctx.Err()
}
func (d *cancelDownload) Close() error { d.closed.Add(1); close(d.done); return nil }

type cancelStreamFixture struct {
	managementBackend
	d *cancelDownload
}

func (f cancelStreamFixture) prepareDownload(ctx context.Context, _ bool) (managementDownload, error) {
	f.d.ctx = ctx
	return f.d, nil
}

// Force observation of the partial response without relying on net/http's
// optional ReaderFrom buffering in this held-read failure fixture.
type flushingResponseWriter struct{ http.ResponseWriter }

func (w flushingResponseWriter) Write(p []byte) (int, error) {
	n, e := w.ResponseWriter.Write(p)
	if e == nil {
		e = http.NewResponseController(w.ResponseWriter).Flush()
	}
	return n, e
}
func TestDownloadHTTPClientCancelsDuringTransfer(t *testing.T) {
	d := &cancelDownload{done: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	s := newTestServer(t, newApplication(cancelStreamFixture{d: d}))
	cookie, csrf := login(t, s)
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.Handler().ServeHTTP(flushingResponseWriter{w}, r) }))
	ts.Config.ErrorLog = log.New(io.Discard, "", 0)
	ts.Start()
	defer ts.Close()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(d.release) }) }
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, e := http.NewRequestWithContext(ctx, "POST", ts.URL+"/api/export", strings.NewReader(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	r.Host = testHost
	r.AddCookie(cookie)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("X-MiskoAI-CSRF", csrf)
	client := ts.Client()
	client.Timeout = 5 * time.Second
	resp, e := client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	bytes := make([]byte, 4096)
	if _, e := io.ReadFull(resp.Body, bytes); e != nil {
		t.Fatal(e)
	}
	cancel()
	select {
	case <-d.canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("server reader did not observe actual client cancellation")
	}
	select {
	case <-d.done:
		t.Fatal("artifact closed before actual read returned")
	default:
	}
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if active != 1 {
		t.Fatal("handler admission released before actual return", active)
	}
	joined := make(chan struct{})
	go func() { s.joined.Wait(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("handler joined before held read release")
	default:
	}
	release()
	select {
	case <-d.done:
	case <-time.After(5 * time.Second):
		t.Fatal("actual canceled HTTP transfer did not close/join")
	}
	select {
	case <-joined:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not actually join")
	}
	if d.closed.Load() != 1 {
		t.Fatal("close count", d.closed.Load())
	}
}

type statusFixture struct{ managementBackend }

func (statusFixture) Status() core.Status {
	v := core.Status{HeapBytes: 1234, RSSAvailable: true, ChannelState: "private-state-canary", Events: []core.Event{{Kind: "private-event-canary"}, {Kind: "created"}}}
	v.Channel.Channel = "private-channel-canary"
	v.Channel.LastCode = "private-error-canary"
	v.Summary.LastCode = "private-summary-canary"
	v.Metrics.ChatAttempts = 12
	return v
}
func TestDiagnosticsAndStatusRedaction(t *testing.T) {
	s := newTestServer(t, newApplication(statusFixture{}))
	cookie, _ := login(t, s)
	for _, path := range []string{"/api/status", "/api/diagnostics"} {
		w := request(s, "GET", path, "", cookie, "", "")
		if w.Code != 200 || strings.Contains(w.Body.String(), "canary") || !strings.Contains(w.Body.String(), `"rss_available":false`) || !strings.Contains(w.Body.String(), `"heap_bytes":1234`) || !strings.Contains(w.Body.String(), `"logical_attempts":{"chat":12`) {
			t.Fatal(w.Code, w.Body.String())
		}
		if path == "/api/diagnostics" && !strings.Contains(w.Body.String(), `"external_probe":false`) {
			t.Fatal("external probe claim")
		}
	}
}
func TestRestorePreservesAndPauses(t *testing.T) {
	c := privateCore(t, false)
	s := newTestServer(t, Application(c))
	cookie, csrf := login(t, s)
	if w := request(s, "POST", "/api/facts", `{"content":"preserved","category":"","importance":1,"expires_at":null}`, cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code)
	}
	snapshot := request(s, "POST", "/api/backup", `{}`, cookie, testOrigin, csrf)
	if snapshot.Code != 200 {
		t.Fatal(snapshot.Code)
	}
	restore := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/restore", strings.NewReader(body))
		r.Host = testHost
		r.AddCookie(cookie)
		r.Header.Set("Origin", testOrigin)
		r.Header.Set("X-MiskoAI-CSRF", csrf)
		r.Header.Set("Content-Type", "application/octet-stream")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := restore("invalid snapshot"); w.Code == 200 {
		t.Fatal("invalid restore accepted")
	}
	if w := request(s, "GET", "/api/facts", "", cookie, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "preserved") {
		t.Fatal("invalid restore lost data")
	}
	if w := restore(snapshot.Body.String()); w.Code != 200 || w.Body.String() != `{"prior_retained":true,"channel_paused":true}` {
		t.Fatal(w.Code, w.Body.String())
	}
	if c.Status().ChannelState != "restore_paused" {
		t.Fatal(c.Status().ChannelState)
	}
	if w := request(s, "POST", "/api/channel/resume", `{"acknowledge":"wrong"}`, cookie, testOrigin, csrf); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if c.Status().ChannelState != "restore_paused" {
		t.Fatal("bad ack resumed")
	}
	if w := request(s, "POST", "/api/channel/resume", `{"acknowledge":"reconciled_restored_history"}`, cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
