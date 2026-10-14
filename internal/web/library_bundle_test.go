package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/maintenance"
)

// These fixtures exercise the real HTTP boundary; they do not install a database.
type bundleHTTPFixture struct {
	managementBackend
	d                  managementDownload
	prepared, restored int
	result             maintenance.RestoreResult
	err                error
	observe            func(context.Context)
	consume            func(io.Reader) error
}

func (f *bundleHTTPFixture) prepareLibraryBundle(ctx context.Context) (managementDownload, error) {
	f.prepared++
	if f.observe != nil {
		f.observe(ctx)
	}
	return f.d, f.err
}
func (f *bundleHTTPFixture) RestoreLibraryBundle(ctx context.Context, r io.Reader) (maintenance.RestoreResult, error) {
	f.restored++
	if f.observe != nil {
		f.observe(ctx)
	}
	if f.err != nil {
		return maintenance.RestoreResult{}, f.err
	}
	if f.consume != nil {
		return f.result, f.consume(r)
	}
	_, e := io.Copy(io.Discard, r)
	return f.result, e
}

func TestLibraryBundleHTTPMaintenanceDeadline(t *testing.T) {
	for _, path := range []string{"/api/library/backup", "/api/library/restore"} {
		t.Run(path, func(t *testing.T) {
			var remaining time.Duration
			f := &bundleHTTPFixture{err: core.ErrBusy, observe: func(ctx context.Context) {
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("missing deadline")
				}
				remaining = time.Until(deadline)
			}}
			s, c, x := bundleHTTPServer(t, f)
			body, kind := "{}", "application/json"
			if strings.HasSuffix(path, "restore") {
				body, kind = "data", "application/octet-stream"
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, bundleHTTPRequest(path, body, kind, c, x))
			if w.Code != 409 || remaining < 25*time.Second || remaining > 30*time.Second {
				t.Fatalf("deadline class status=%d remaining=%v", w.Code, remaining)
			}
		})
	}
}

func TestLibraryBundleHTTPSafeErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"content", library.ErrContent, 400, "web_content"}, {"recovery", library.ErrRecovery, 409, "web_recovery"}, {"cleanup", library.ErrCleanup, 500, "web_operation"}, {"capacity", library.ErrCapacity, 409, "web_busy"}, {"closed", library.ErrClosed, 503, "web_unavailable"}, {"maintenance", core.ErrMaintenance, 400, "web_maintenance"}, {"cancel", context.Canceled, 408, "web_timeout"}, {"deadline", context.DeadlineExceeded, 408, "web_timeout"}, {"private", errors.New("private path credential canary"), 500, "web_operation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/api/library/backup", "/api/library/restore"} {
				f := &bundleHTTPFixture{err: errors.Join(tc.err, errors.New("private credential canary"))}
				s, c, x := bundleHTTPServer(t, f)
				body, kind := "{}", "application/json"
				if strings.HasSuffix(path, "restore") {
					body, kind = "data", "application/octet-stream"
				}
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, bundleHTTPRequest(path, body, kind, c, x))
				if w.Code != tc.status || w.Body.String() != `{"error":"`+tc.code+`"}` {
					t.Fatal(w.Code, w.Body.String())
				}
			}
		})
	}
}

type bundleHTTPHeld struct {
	readEntered, readRelease, closeEntered, closeRelease chan struct{}
	cancel                                               context.CancelFunc
	closeOnce                                            sync.Once
}

func (d *bundleHTTPHeld) Read(p []byte) (int, error) {
	if d.readEntered != nil {
		close(d.readEntered)
		<-d.readRelease
	}
	if d.cancel != nil {
		d.cancel()
	}
	return copy(p, "data"), nil
}
func (d *bundleHTTPHeld) metadata() (string, int64) { return "miskoai-library.bundle", 4 }
func (d *bundleHTTPHeld) Close() error {
	d.closeOnce.Do(func() {
		if d.closeEntered != nil {
			close(d.closeEntered)
			<-d.closeRelease
		}
	})
	return nil
}
func TestLibraryBundleHTTPJoinedReadAndClose(t *testing.T) {
	for _, phase := range []string{"read", "close"} {
		t.Run(phase, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			d := &bundleHTTPHeld{}
			if phase == "read" {
				d.readEntered, d.readRelease = entered, release
			} else {
				d.closeEntered, d.closeRelease = entered, release
			}
			f := &bundleHTTPFixture{d: d}
			s, c, x := bundleHTTPServer(t, f)
			done := make(chan struct{})
			w := httptest.NewRecorder()
			go func() {
				defer close(done)
				s.Handler().ServeHTTP(w, bundleHTTPRequest("/api/library/backup", "{}", "application/json", c, x))
			}()
			defer func() {
				unblock()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("handler not joined")
				}
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("not entered")
			}
			select {
			case <-done:
				t.Fatal("returned with held operation")
			default:
			}
			unblock()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("did not finish")
			}
			if w.Code != 200 || w.Body.String() != "data" {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
func TestLibraryBundleHTTPReadCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := &bundleHTTPHeld{cancel: cancel}
	f := &bundleHTTPFixture{d: d}
	s, c, x := bundleHTTPServer(t, f)
	w := httptest.NewRecorder()
	r := bundleHTTPRequest("/api/library/backup", "{}", "application/json", c, x).WithContext(ctx)
	if !serveForAbort(s.Handler(), w, r) || w.Body.Len() != 0 {
		t.Fatal("canceled read must abort before writing", w.Body.String())
	}
}
func TestLibraryBundleHTTPActualTruncation(t *testing.T) {
	d := &bundleHTTPDownload{input: strings.NewReader("data"), name: "miskoai-library.bundle", size: 8, done: make(chan struct{})}
	f := &bundleHTTPFixture{d: d}
	s, c, x := bundleHTTPServer(t, f)
	ts := httptest.NewUnstartedServer(s.Handler())
	ts.Config.ErrorLog = log.New(io.Discard, "", 0)
	ts.Start()
	defer ts.Close()
	r := bundleHTTPRequest("/api/library/backup", "{}", "application/json", c, x)
	r.URL.Scheme = "http"
	r.URL.Host = strings.TrimPrefix(ts.URL, "http://")
	r.RequestURI = ""
	client := ts.Client()
	client.Timeout = 3 * time.Second
	response, e := client.Do(r)
	if e == nil {
		defer response.Body.Close()
		data, readErr := io.ReadAll(response.Body)
		if readErr == nil || strings.Contains(string(data), "error") {
			t.Fatal("stream completed instead of truncated", string(data), readErr)
		}
	}
	select {
	case <-d.done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close not joined")
	}
	if d.closed != 1 {
		t.Fatal("not synchronously closed", d.closed)
	}
}

type bundleHTTPDownload struct {
	input    io.Reader
	name     string
	size     int64
	closed   int
	closeErr error
	done     chan struct{}
}

func (d *bundleHTTPDownload) metadata() (string, int64)  { return d.name, d.size }
func (d *bundleHTTPDownload) Read(p []byte) (int, error) { return d.input.Read(p) }
func (d *bundleHTTPDownload) Close() error {
	d.closed++
	if d.done != nil {
		close(d.done)
	}
	return d.closeErr
}
func bundleHTTPServer(t *testing.T, f managementBackend) (*Server, *http.Cookie, string) {
	t.Helper()
	s := newTestServer(t, newApplication(f))
	c, x := login(t, s)
	// Prove actual auth prerequisites before route assertions.
	w := request(s, "GET", "/api/session", "", c, "", "")
	if w.Code != 200 {
		t.Fatalf("auth prerequisite %d", w.Code)
	}
	var session struct {
		CSRF string `json:"csrf"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &session); e != nil || session.CSRF == "" {
		t.Fatal("session prerequisite", e)
	}
	x = session.CSRF
	return s, c, x
}
func bundleHTTPRequest(path, body, kind string, c *http.Cookie, csrf string) *http.Request {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Host = testHost
	r.AddCookie(c)
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("X-MiskoAI-CSRF", csrf)
	r.Header.Set("Content-Type", kind)
	return r
}
func TestLibraryBundleHTTPFixedBackup(t *testing.T) {
	d := &bundleHTTPDownload{input: strings.NewReader("data"), name: "miskoai-library.bundle", size: 4}
	f := &bundleHTTPFixture{d: d}
	s, c, x := bundleHTTPServer(t, f)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, bundleHTTPRequest("/api/library/backup", "{}", "application/json", c, x))
	if w.Code != 200 || w.Body.String() != "data" || w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("Content-Length") != "4" || w.Header().Get("Content-Disposition") != `attachment; filename="miskoai-library.bundle"` || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("backup %d %s %v", w.Code, w.Body.String(), w.Header())
	}
	if f.prepared != 1 || d.closed != 1 {
		t.Fatal("transfer lifecycle", f.prepared, d.closed)
	}
}
func TestLibraryBundleHTTPFixedRestore(t *testing.T) {
	for _, flags := range []bool{false, true} {
		t.Run(map[bool]string{false: "false", true: "true"}[flags], func(t *testing.T) {
			f := &bundleHTTPFixture{result: maintenance.RestoreResult{PreviousRetained: flags, ChannelPaused: flags}}
			s, c, x := bundleHTTPServer(t, f)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, bundleHTTPRequest("/api/library/restore", "data", "application/octet-stream", c, x))
			want := `{"prior_retained":false,"channel_paused":false}`
			if flags {
				want = `{"prior_retained":true,"channel_paused":true}`
			}
			if w.Code != 200 || w.Body.String() != want || f.restored != 1 {
				t.Fatalf("restore %d %s calls=%d", w.Code, w.Body.String(), f.restored)
			}
		})
	}
}
func TestLibraryBundleHTTPAdmission(t *testing.T) {
	for _, path := range []string{"/api/library/backup", "/api/library/restore"} {
		t.Run(path, func(t *testing.T) {
			for _, mode := range []string{"method", "query", "auth", "csrf", "origin", "host", "encoding", "nil-body", "type", "duplicate-type", "params", "body", "trailing", "oversize", "unknown", "zero", "negative"} {
				if strings.HasSuffix(path, "restore") && (mode == "body" || mode == "trailing") || strings.HasSuffix(path, "backup") && (mode == "unknown" || mode == "zero" || mode == "negative") {
					continue
				}
				t.Run(mode, func(t *testing.T) {
					f := &bundleHTTPFixture{}
					s, c, x := bundleHTTPServer(t, f)
					body, kind := "{}", "application/json"
					if strings.HasSuffix(path, "restore") {
						body, kind = "data", "application/octet-stream"
					}
					r := bundleHTTPRequest(path, body, kind, c, x)
					want := 400
					switch mode {
					case "method":
						r.Method = "GET"
						want = 405
					case "query":
						r.URL.RawQuery = "path=private"
					case "auth":
						r.Header.Del("Cookie")
						want = 401
					case "csrf":
						r.Header.Del("X-MiskoAI-CSRF")
						want = 403
					case "origin":
						r.Header.Set("Origin", "http://evil.example")
						want = 403
					case "host":
						r.Host = "evil.example"
						want = 403
					case "encoding":
						r.Header.Set("Content-Encoding", "")
					case "nil-body":
						r.Body = nil
					case "type":
						r.Header.Set("Content-Type", "text/plain")
					case "duplicate-type":
						r.Header.Add("Content-Type", kind)
					case "params":
						if strings.HasSuffix(path, "backup") {
							r.Header.Set("Content-Type", kind+"; boundary=x")
						} else {
							r.Header.Set("Content-Type", kind+"; charset=utf-8")
						}
					case "body":
						r.Body = io.NopCloser(strings.NewReader(`{"path":"private"}`))
					case "trailing":
						r.Body = io.NopCloser(strings.NewReader("{} {}"))
					case "oversize":
						r.ContentLength = 321<<20 + 1
					case "unknown":
						r.ContentLength = -1
					case "zero":
						r.ContentLength = 0
					case "negative":
						r.ContentLength = -2
					}
					w := httptest.NewRecorder()
					s.Handler().ServeHTTP(w, r)
					if w.Code != want || f.prepared != 0 || f.restored != 0 {
						t.Fatalf("%s: status=%d body=%s effects=%d/%d", mode, w.Code, w.Body.String(), f.prepared, f.restored)
					}
				})
			}
		})
	}
}
func TestLibraryBundleHTTPUnsupported(t *testing.T) {
	s, c, x := bundleHTTPServer(t, &managementFixture{})
	for _, path := range []string{"/api/library/backup", "/api/library/restore"} {
		w := httptest.NewRecorder()
		r := bundleHTTPRequest(path, "{}", "application/json", c, x)
		s.Handler().ServeHTTP(w, r)
		if w.Code != 503 || w.Body.String() != `{"error":"web_unavailable"}` {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestLibraryBundleHTTPTransferAbortAndMetadata(t *testing.T) {
	for _, mode := range []string{"short", "read-error", "write-error", "close-error", "bad-name", "zero", "negative", "oversize", "max"} {
		t.Run(mode, func(t *testing.T) {
			d := &bundleHTTPDownload{input: strings.NewReader("data"), name: "miskoai-library.bundle", size: 4}
			want, abort := 200, true
			switch mode {
			case "short":
				d.size = 8
			case "read-error":
				d.input = errorReader{}
			case "close-error":
				d.closeErr = errors.New("private close path")
			case "bad-name":
				d.name = "private/path"
				want, abort = 500, false
			case "zero":
				d.size = 0
				want, abort = 500, false
			case "negative":
				d.size = -1
				want, abort = 500, false
			case "oversize":
				d.size = 321<<20 + 1
				want, abort = 500, false
			case "max":
				d.size = 321 << 20
			}
			f := &bundleHTTPFixture{d: d}
			s, c, x := bundleHTTPServer(t, f)
			r := bundleHTTPRequest("/api/library/backup", "{}", "application/json", c, x)
			w := httptest.NewRecorder()
			var writer http.ResponseWriter = w
			if mode == "write-error" {
				writer = &failingWriter{header: make(http.Header)}
			}
			if got := serveForAbort(s.Handler(), writer, r); got != abort {
				t.Fatalf("abort %v want %v", got, abort)
			}
			if mode != "write-error" && (w.Code != want || strings.Contains(w.Body.String(), "private") || abort && strings.Contains(w.Body.String(), "error")) {
				t.Fatal(w.Code, w.Body.String())
			}
			if d.closed != 1 {
				t.Fatal("close", d.closed)
			}
		})
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("private read path") }
func TestLibraryBundleHTTPRestoreExactLength(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		declared   int64
		want       int
	}{{"short", "abc", 4, 400}, {"extra", "abcde", 4, 400}, {"exact", "abcd", 4, 200}, {"max-admitted-short", "a", 321 << 20, 400}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &bundleHTTPFixture{}
			s, c, x := bundleHTTPServer(t, f)
			r := bundleHTTPRequest("/api/library/restore", tc.body, "application/octet-stream", c, x)
			r.ContentLength = tc.declared
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != tc.want || f.restored != 1 {
				t.Fatal(w.Code, w.Body.String(), f.restored)
			}
		})
	}
}

func TestLibraryBundleHTTPErrorPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"maintenance-canceled", errors.Join(core.ErrMaintenance, context.Canceled), 408},
		{"maintenance-cleanup", errors.Join(core.ErrMaintenance, library.ErrCleanup), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &bundleHTTPFixture{err: tc.err}
			s, c, x := bundleHTTPServer(t, f)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, bundleHTTPRequest("/api/library/restore", "data", "application/octet-stream", c, x))
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
func TestLibraryBundleHTTPRestoreRequiresConsumedEOF(t *testing.T) {
	for _, mode := range []string{"no-consumption", "no-probe", "ignored-overflow"} {
		t.Run(mode, func(t *testing.T) {
			f := &bundleHTTPFixture{consume: func(r io.Reader) error {
				switch mode {
				case "no-probe":
					var b [4]byte
					_, _ = io.ReadFull(r, b[:])
				case "ignored-overflow":
					_, _ = io.Copy(io.Discard, r)
					_, _ = io.Copy(io.Discard, r)
				}
				return nil
			}}
			s, c, x := bundleHTTPServer(t, f)
			w := httptest.NewRecorder()
			r := bundleHTTPRequest("/api/library/restore", "abcde", "application/octet-stream", c, x)
			r.ContentLength = 4
			s.Handler().ServeHTTP(w, r)
			if w.Code != 400 || w.Body.String() != `{"error":"web_input"}` {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
func TestLibraryBundleHTTPRestoreCancelAndReadFailure(t *testing.T) {
	for _, mode := range []string{"cancel-before", "cancel-read", "read-error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &bundleHTTPFixture{}
			s, c, x := bundleHTTPServer(t, f)
			r := bundleHTTPRequest("/api/library/restore", "data", "application/octet-stream", c, x).WithContext(ctx)
			want, calls := 408, 1
			switch mode {
			case "cancel-before":
				cancel()
				calls = 0
			case "cancel-read":
				r.Body = io.NopCloser(&bundleHTTPHeld{cancel: cancel})
			case "read-error":
				r.Body = io.NopCloser(errorReader{})
				want = 500
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != want || f.restored != calls || strings.Contains(w.Body.String(), "private") {
				t.Fatal(w.Code, w.Body.String(), f.restored)
			}
		})
	}
}

func TestLibraryBundleHTTPBackupPreparationLifetime(t *testing.T) {
	for _, mode := range []string{"nil-download", "error-with-download", "canceled-before", "canceled-preparation", "canceled-preparation-close-error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d := &bundleHTTPDownload{input: strings.NewReader("data"), name: "miskoai-library.bundle", size: 4}
			f := &bundleHTTPFixture{d: d}
			want, calls, closes := 500, 1, 1
			switch mode {
			case "nil-download":
				f.d = nil
				closes = 0
			case "error-with-download":
				f.err = errors.New("private preparation canary")
			case "canceled-before":
				cancel()
				want, calls, closes = 408, 0, 0
			case "canceled-preparation":
				f.observe = func(context.Context) { cancel() }
				want = 408
			case "canceled-preparation-close-error":
				f.observe = func(context.Context) { cancel() }
				d.closeErr = errors.New("private close canary")
			}
			s, c, x := bundleHTTPServer(t, f)
			w := httptest.NewRecorder()
			r := bundleHTTPRequest("/api/library/backup", "{}", "application/json", c, x).WithContext(ctx)
			s.Handler().ServeHTTP(w, r)
			if w.Code != want || f.prepared != calls || d.closed != closes || strings.Contains(w.Body.String(), "private") || w.Header().Get("Content-Disposition") != "" {
				t.Fatal(w.Code, w.Body.String(), f.prepared, d.closed)
			}
		})
	}
}

// A final Read may return the complete requested bytes and a non-EOF error.
// The HTTP transfer must abort before exposing those bytes; ordinary final EOF
// with all declared bytes remains a valid completion control.
type bundleHTTPFinalRead struct {
	left   int64
	err    error
	cancel context.CancelFunc
}

func (r *bundleHTTPFinalRead) Read(p []byte) (int, error) {
	n := len(p)
	if int64(n) > r.left {
		n = int(r.left)
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.left -= int64(n)
	if r.left == 0 {
		if r.cancel != nil {
			r.cancel()
		}
		return n, r.err
	}
	return n, nil
}
func TestLibraryBundleHTTPFinalReadError(t *testing.T) {
	for _, size := range []int64{4, 4096, 4097} {
		t.Run(strconv.FormatInt(size, 10), func(t *testing.T) {
			for _, mode := range []string{"non-eof-error", "cancel-and-error", "final-eof", "final-no-error"} {
				t.Run(mode, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					input := &bundleHTTPFinalRead{left: size}
					abort := false
					switch mode {
					case "non-eof-error":
						input.err = io.ErrUnexpectedEOF
						abort = true
					case "cancel-and-error":
						input.err = errors.New("private final read error")
						input.cancel = cancel
						abort = true
					case "final-eof":
						input.err = io.EOF
					}
					d := &bundleHTTPDownload{input: input, name: "miskoai-library.bundle", size: size}
					f := &bundleHTTPFixture{d: d}
					s, c, x := bundleHTTPServer(t, f)
					w := httptest.NewRecorder()
					r := bundleHTTPRequest("/api/library/backup", "{}", "application/json", c, x).WithContext(ctx)
					gotAbort := serveForAbort(s.Handler(), w, r)
					wantBytes := int(size)
					if abort {
						wantBytes = int((size-1)/4096) * 4096
					}
					if gotAbort != abort || w.Body.Len() != wantBytes || w.Body.String() != strings.Repeat("x", wantBytes) || w.Code != 200 || d.closed != 1 || f.prepared != 1 {
						t.Fatalf("abort=%v want=%v bytes=%d want=%d code=%d closes=%d preparations=%d", gotAbort, abort, w.Body.Len(), wantBytes, w.Code, d.closed, f.prepared)
					}
					if strings.Contains(w.Body.String(), "error") || strings.Contains(w.Body.String(), "private") {
						t.Fatal("error leaked into binary response")
					}
				})
			}
		})
	}
}

type bundleHTTPInvalidRead struct{ count int }

func (r bundleHTTPInvalidRead) Read(p []byte) (int, error) {
	if r.count == 2 {
		return len(p) + 1, nil
	}
	return r.count, nil
}

type bundleHTTPShortRead struct{ left int }

func (r *bundleHTTPShortRead) Read(p []byte) (int, error) {
	if r.left == 0 {
		return 0, io.EOF
	}
	p[0] = 'x'
	r.left--
	return 1, nil
}

type bundleHTTPShortWriter struct{ *httptest.ResponseRecorder }

func (w bundleHTTPShortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestLibraryBundleHTTPDirectReadBounds(t *testing.T) {
	for _, mode := range []string{"negative-read", "oversized-read", "no-progress", "short-success", "short-write"} {
		t.Run(mode, func(t *testing.T) {
			var input io.Reader = &bundleHTTPFinalRead{left: 4}
			abort, want := true, 0
			switch mode {
			case "negative-read":
				input = bundleHTTPInvalidRead{-1}
			case "oversized-read":
				input = bundleHTTPInvalidRead{2}
			case "no-progress":
				input = bundleHTTPInvalidRead{0}
			case "short-success":
				input = &bundleHTTPShortRead{left: 4}
				abort, want = false, 4
			}
			d := &bundleHTTPDownload{input: input, name: "miskoai-library.bundle", size: 4}
			f := &bundleHTTPFixture{d: d}
			s, c, x := bundleHTTPServer(t, f)
			w := httptest.NewRecorder()
			var output http.ResponseWriter = w
			if mode == "short-write" {
				output = bundleHTTPShortWriter{w}
			}
			got := serveForAbort(s.Handler(), output, bundleHTTPRequest("/api/library/backup", "{}", "application/json", c, x))
			if got != abort || w.Body.Len() != want || d.closed != 1 || f.prepared != 1 {
				t.Fatal("direct read/write refusal", got, w.Body.Len(), d.closed, f.prepared)
			}
		})
	}
}
