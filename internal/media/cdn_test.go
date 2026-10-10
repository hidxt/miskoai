package media

import (
	"bytes"
	"context"
	"crypto/aes"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type cdnRT func(*http.Request) (*http.Response, error)

func (f cdnRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type cdnBody struct {
	io.Reader
	closeErr error
	closed   atomic.Bool
}

func (b *cdnBody) Close() error { b.closed.Store(true); return b.closeErr }
func cdnClient(t *testing.T, f cdnRT) *CDN {
	t.Helper()
	c, e := NewCDN()
	if e != nil {
		t.Fatal(e)
	}
	c.client.Transport = f
	t.Cleanup(func() { c.Close() })
	return c
}
func response(status int, b io.ReadCloser, n int64) *http.Response {
	return &http.Response{StatusCode: status, Body: b, ContentLength: n, Header: make(http.Header)}
}
func cipherFixture(p []byte) []byte {
	n := 16 - len(p)%16
	b := append([]byte(nil), p...)
	b = append(b, bytes.Repeat([]byte{byte(n)}, n)...)
	a, _ := aes.NewCipher(make([]byte, 16))
	for i := 0; i < len(b); i += 16 {
		a.Encrypt(b[i:i+16], b[i:i+16])
	}
	return b
}

func TestCDNNoCredentialForwarding(t *testing.T) {
	calls := 0
	c := cdnClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Accept-Encoding") != "" || len(r.Header) != 0 {
			t.Errorf("unexpected request headers: %v", r.Header)
		}
		d, ok := r.Context().Deadline()
		if !ok || time.Until(d) > 30*time.Second || time.Until(d) < 29*time.Second {
			t.Error("missing whole-operation deadline")
		}
		return response(200, &cdnBody{Reader: bytes.NewReader(cipherFixture([]byte("hello")))}, 16), nil
	})
	a, e := c.Download(context.Background(), mediaDir(t), "opaque", "", [16]byte{})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(a)
	if string(b) != "hello" {
		t.Fatal("wrong plaintext")
	}
	a.Close()
	if _, e = c.Download(context.Background(), mediaDir(t), "fallback", "https://evil.invalid/c2c/download?a=b", [16]byte{}); e == nil || calls != 1 {
		t.Fatal("invalid preferred URL reached transport")
	}
	if c.client.Jar != nil || c.client.CheckRedirect(&http.Request{}, nil) != http.ErrUseLastResponse {
		t.Fatal("redirect/cookie policy")
	}
}

func TestCDNTransportNoReplayPolicy(t *testing.T) {
	c, e := NewCDN()
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	transport, ok := c.client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || !transport.DisableKeepAlives || !transport.DisableCompression {
		t.Fatal("unsafe transport policy")
	}
	if transport.Protocols == nil || !transport.Protocols.HTTP1() || transport.Protocols.HTTP2() || transport.Protocols.UnencryptedHTTP2() {
		t.Fatal("explicit HTTP1-only policy required for zero replay")
	}
}

type cdnReadHeld struct{ entered, release chan struct{} }

func (r *cdnReadHeld) Read([]byte) (int, error) { close(r.entered); <-r.release; return 0, io.EOF }
func TestCDNUploadBorrowCloseJoinsReader(t *testing.T) {
	held := &cdnReadHeld{entered: make(chan struct{}), release: make(chan struct{})}
	release := cdnRelease(t, held.release)
	b := &cdnUploadBody{reader: held}
	readDone := make(chan struct{})
	t.Cleanup(func() { release(); awaitCDN(t, readDone) })
	go func() { b.Read(make([]byte, 16)); close(readDone) }()
	awaitCDN(t, held.entered)
	closed := make(chan struct{})
	go func() { b.Close(); close(closed) }()
	select {
	case <-closed:
		release()
		awaitCDN(t, readDone)
		t.Fatal("borrow close abandoned actual Read")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	awaitCDN(t, readDone)
	awaitCDN(t, closed)
	if n, e := b.Read(make([]byte, 16)); n != 0 || e == nil {
		t.Fatal("closed borrow still permits reads")
	}
}

type cdnCloseSignalBody struct {
	io.Reader
	closed chan struct{}
}

func (b *cdnCloseSignalBody) Close() error { close(b.closed); return nil }
func TestCDNUploadCancellationDuringBorrowClose(t *testing.T) {
	a, e := Encrypt(context.Background(), mediaDir(t), strings.NewReader("borrow"), [16]byte{})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	held := &cdnReadHeld{entered: make(chan struct{}), release: make(chan struct{})}
	readDone := make(chan struct{})
	responseClosed := make(chan struct{})
	c := cdnClient(t, func(r *http.Request) (*http.Response, error) {
		// A narrow per-request synthetic reader forces an outstanding request-body
		// read at the transport return boundary; the real client/admission run.
		borrowed := r.Body.(*cdnUploadBody)
		borrowed.reader = held
		go func() { r.Body.Read(make([]byte, 16)); close(readDone) }()
		awaitCDN(t, held.entered)
		resp := response(200, &cdnCloseSignalBody{Reader: strings.NewReader(""), closed: responseClosed}, 0)
		resp.Header.Set("x-encrypted-param", "opaque")
		return resp, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	release := cdnRelease(t, held.release)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		v, e := c.Upload(ctx, a, "x", "", strings.Repeat("a", 32))
		if v != "" && e != nil {
			t.Error("failure returned parameter")
		}
		done <- e
	}()
	awaitCDN(t, responseClosed)
	// Let the response-close defer return and the request-borrow Close block
	// before cancellation; cancellation before response close is another path.
	select {
	case <-done:
		release()
		awaitCDN(t, readDone)
		t.Fatal("borrowed read abandoned")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
		release()
		awaitCDN(t, readDone)
		t.Fatal("borrowed read abandoned")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	awaitCDN(t, readDone)
	if e := cdnResult(t, done); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancellation after actual borrow close lost: %v", e)
	}
}

func TestCDNInvalidUploadNeverReachesTransport(t *testing.T) {
	calls := 0
	c := cdnClient(t, func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected") })
	for _, a := range []*Artifact{nil, {size: 0}, {size: 15}, {size: maxCiphertext + 16}} {
		if _, e := c.Upload(context.Background(), a, "x", "", strings.Repeat("a", 32)); e == nil {
			t.Error("invalid artifact accepted")
		}
	}
	if _, e := c.Upload(context.Background(), &Artifact{size: 16}, "x", "https://evil.invalid/c2c/upload?a=b", strings.Repeat("a", 32)); e == nil {
		t.Fatal("unsafe upload endpoint accepted")
	}
	if calls != 0 {
		t.Fatal("invalid input reached transport")
	}
}

func TestCDNDownloadReadAndTransportErrorCanaries(t *testing.T) {
	for _, tc := range []struct {
		name         string
		reader       io.Reader
		transportErr bool
	}{
		{"read", cdnFailReader{}, false}, {"transport", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := mediaDir(t)
			calls := 0
			var body *cdnBody
			c := cdnClient(t, func(*http.Request) (*http.Response, error) {
				calls++
				if tc.transportErr {
					return nil, errors.New("https://URL_SECRET/?QUERY_SECRET&KEY_SECRET")
				}
				body = &cdnBody{Reader: tc.reader}
				return response(200, body, -1), nil
			})
			a, e := c.Download(context.Background(), dir, "QUERY_SECRET", "", [16]byte{})
			if a != nil || e == nil || strings.Contains(e.Error(), "SECRET") || strings.Contains(e.Error(), dir) {
				t.Fatalf("unsafe result: %v", e)
			}
			if calls != 1 {
				t.Fatal("reattempt")
			}
			if body != nil && !body.closed.Load() {
				t.Fatal("body leaked")
			}
			noSpools(t, dir)
		})
	}
}

type cdnCloseHeldBody struct {
	io.Reader
	entered, release chan struct{}
}

func (b *cdnCloseHeldBody) Close() error { close(b.entered); <-b.release; return nil }
func TestCDNAdmissionAndCloseJoinCheckedBodyClose(t *testing.T) {
	body := &cdnCloseHeldBody{Reader: bytes.NewReader(cipherFixture([]byte("completed"))), entered: make(chan struct{}), release: make(chan struct{})}
	dir := mediaDir(t)
	var calls atomic.Int32
	c := cdnClient(t, func(*http.Request) (*http.Response, error) { calls.Add(1); return response(200, body, 16), nil })
	release := cdnRelease(t, body.release)
	result := make(chan error, 1)
	go func() {
		a, e := c.Download(context.Background(), dir, "x", "", [16]byte{})
		if a != nil {
			a.Close()
		}
		result <- e
	}()
	awaitCDN(t, body.entered)
	wait, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, e := c.Download(wait, mediaDir(t), "x", "", [16]byte{}); !errors.Is(e, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatal("admission released before checked close")
	}
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	// stop is the closed-state publication; neither Close nor Download may
	// complete until the actual response Close returns.
	awaitCDN(t, c.stop)
	select {
	case <-closed:
		t.Fatal("Close abandoned body.Close")
	default:
	}
	select {
	case <-result:
		t.Fatal("Download abandoned body.Close")
	default:
	}
	release()
	awaitCDN(t, closed)
	if e := cdnResult(t, result); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	noSpools(t, dir)
}

func TestCDNActualDownloadBoundsAndPadding(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      []byte
		decl     int64
		encoding string
		closeErr error
		want     error
	}{
		{"unknown-valid", cipherFixture([]byte("ok")), -1, "", nil, nil},
		{"maximum-valid", cipherFixture(bytes.Repeat([]byte{'x'}, maxPlaintext)), maxCiphertext, "identity", nil, nil},
		{"known-excess", nil, maxCiphertext + 1, "", nil, ErrLimit},
		{"unknown-excess", make([]byte, maxCiphertext+1), -1, "", nil, ErrLimit},
		{"dishonest-excess", make([]byte, maxCiphertext+1), 16, "", nil, ErrLimit},
		{"truncated", make([]byte, 15), -1, "", nil, ErrInvalid},
		{"bad-padding", make([]byte, 16), -1, "", nil, ErrInvalid},
		{"compressed", cipherFixture([]byte("ok")), 16, "gzip", nil, ErrInvalid},
		{"close-failure", cipherFixture([]byte("ok")), 16, "", errors.New("BODY_SECRET"), ErrIO},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := mediaDir(t)
			body := &cdnBody{Reader: bytes.NewReader(tc.raw), closeErr: tc.closeErr}
			c := cdnClient(t, func(*http.Request) (*http.Response, error) {
				r := response(200, body, tc.decl)
				if tc.encoding != "" {
					r.Header.Set("Content-Encoding", tc.encoding)
				}
				return r, nil
			})
			a, e := c.Download(context.Background(), dir, "opaque", "", [16]byte{})
			if tc.want == nil {
				if e != nil || a == nil {
					t.Fatalf("success: %v", e)
				}
				a.Close()
			} else if a != nil || !errors.Is(e, tc.want) {
				t.Fatalf("refusal: artifact=%v error=%v", a, e)
			}
			if !body.closed.Load() {
				t.Error("body not closed")
			}
			noSpools(t, dir)
		})
	}
}

type cdnFailReader struct{}

func (cdnFailReader) Read([]byte) (int, error) { return 0, errors.New("BODY_SECRET") }
func TestCDNUploadExactlyOnce(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		headers      []string
		reader       io.Reader
		closeErr     error
		transportErr bool
		success      bool
	}{
		{"success", 200, []string{"returned-opaque"}, strings.NewReader("ok"), nil, false, true},
		{"maximum-response", 200, []string{"returned-opaque"}, strings.NewReader(strings.Repeat("x", 65536)), nil, false, true},
		{"429", 429, nil, strings.NewReader(""), nil, false, false}, {"503", 503, nil, strings.NewReader(""), nil, false, false},
		{"uncertain", 0, nil, nil, nil, true, false}, {"missing-header", 200, nil, strings.NewReader(""), nil, false, false},
		{"duplicate-header", 200, []string{"x", "y"}, strings.NewReader(""), nil, false, false},
		{"large-header", 200, []string{strings.Repeat("x", 4097)}, strings.NewReader(""), nil, false, false},
		{"control-header", 200, []string{"x\ny"}, strings.NewReader(""), nil, false, false},
		{"empty-header", 200, []string{""}, strings.NewReader(""), nil, false, false},
		{"invalid-utf8-header", 200, []string{string([]byte{255})}, strings.NewReader(""), nil, false, false},
		{"body-read", 200, []string{"x"}, cdnFailReader{}, nil, false, false},
		{"body-close", 200, []string{"x"}, strings.NewReader(""), errors.New("BODY_SECRET"), false, false},
		{"body-limit", 200, []string{"x"}, strings.NewReader(strings.Repeat("x", 65537)), nil, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, e := Encrypt(context.Background(), mediaDir(t), strings.NewReader("caller-owned"), [16]byte{})
			if e != nil {
				t.Fatal(e)
			}
			defer a.Close()
			calls := 0
			var body *cdnBody
			var borrowed io.ReadCloser
			c := cdnClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				borrowed = r.Body
				if r.Method != "POST" || r.ContentLength != 16 || r.GetBody != nil || r.Header.Get("Content-Type") != "application/octet-stream" || len(r.Header) != 1 {
					t.Error("upload request contract")
				}
				raw, e := io.ReadAll(r.Body)
				if e != nil || !bytes.Equal(raw, cipherFixture([]byte("caller-owned"))) {
					t.Error("wrong ciphertext")
				}
				if tc.transportErr {
					return nil, errors.New("URL_SECRET")
				}
				body = &cdnBody{Reader: tc.reader, closeErr: tc.closeErr}
				resp := response(tc.status, body, -1)
				for _, v := range tc.headers {
					resp.Header.Add("x-encrypted-param", v)
				}
				return resp, nil
			})
			v, e := c.Upload(context.Background(), a, "UPLOAD_SECRET", "", strings.Repeat("a", 32))
			if tc.success {
				if e != nil || v != "returned-opaque" {
					t.Fatalf("upload: %q %v", v, e)
				}
			} else if e == nil || v != "" {
				t.Fatal("false success")
			}
			if calls != 1 {
				t.Fatalf("attempts %d", calls)
			}
			if n, e := borrowed.Read(make([]byte, 16)); n != 0 || e == nil {
				t.Fatal("returned transfer left borrow readable")
			}
			if e != nil && (strings.Contains(e.Error(), "SECRET") || strings.Contains(e.Error(), a.path)) {
				t.Fatal("secret leaked")
			}
			if body != nil && !body.closed.Load() {
				t.Error("body not closed")
			}
			raw := make([]byte, 16)
			if n, e := a.ReadAt(raw, 0); n != 16 || e != nil {
				t.Fatal("caller ownership lost")
			}
			if _, e = os.Stat(a.path); e != nil {
				t.Fatal("caller file removed")
			}
		})
	}
}

func TestCDNUploadEarlySuccessRefusesIncompleteRequest(t *testing.T) {
	for _, mode := range []string{"unconsumed", "partial", "readerror", "closed-artifact", "truncated-artifact", "exact-complete"} {
		t.Run(mode, func(t *testing.T) {
			a, e := Encrypt(context.Background(), mediaDir(t), strings.NewReader("caller-owned"), [16]byte{})
			if e != nil {
				t.Fatal(e)
			}
			defer a.Close()
			if mode == "closed-artifact" {
				if e := a.file.Close(); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "truncated-artifact" {
				if e := os.Truncate(a.path, 8); e != nil {
					t.Fatal(e)
				}
			}
			calls := 0
			body := &cdnBody{Reader: strings.NewReader("")}
			c := cdnClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				switch mode {
				case "partial":
					r.Body.Read(make([]byte, 8))
				case "readerror":
					r.Body.(*cdnUploadBody).reader = cdnBytesErrorReader{}
					r.Body.Read(make([]byte, 16))
				case "closed-artifact", "truncated-artifact":
					io.ReadAll(r.Body)
				case "exact-complete":
					if _, e := io.ReadFull(r.Body, make([]byte, 16)); e != nil {
						t.Error(e)
					}
				}
				resp := response(200, body, 0)
				resp.Header.Set("x-encrypted-param", "opaque")
				return resp, nil
			})
			v, e := c.Upload(context.Background(), a, "x", "", strings.Repeat("a", 32))
			if mode == "exact-complete" {
				if e != nil || v != "opaque" {
					t.Fatalf("exact request refused: %q %v", v, e)
				}
			} else if v != "" || !errors.Is(e, ErrIO) {
				t.Fatalf("incomplete/failed request accepted: %q %v", v, e)
			}
			if calls != 1 || !body.closed.Load() {
				t.Fatal("reattempt or response body leak")
			}
			if _, e := os.Stat(a.path); e != nil {
				t.Fatal("caller artifact removed")
			}
			if mode != "closed-artifact" && mode != "truncated-artifact" {
				if n, e := a.ReadAt(make([]byte, 16), 0); n != 16 || e != nil {
					t.Fatal("caller ownership lost")
				}
			}
		})
	}
}

type cdnBytesErrorReader struct{}

func (cdnBytesErrorReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 1
	}
	return len(p), errors.New("REQUEST_SECRET")
}

type cdnHeldFailureReader struct{ entered, release chan struct{} }

func (r *cdnHeldFailureReader) Read(p []byte) (int, error) {
	close(r.entered)
	<-r.release
	return cdnBytesErrorReader{}.Read(p)
}
func TestCDNUploadEarlyResponseJoinsHeldReadFailure(t *testing.T) {
	a, e := Encrypt(context.Background(), mediaDir(t), strings.NewReader("caller-owned"), [16]byte{})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	held := &cdnHeldFailureReader{entered: make(chan struct{}), release: make(chan struct{})}
	readDone := make(chan struct{})
	responseClosed := make(chan struct{})
	c := cdnClient(t, func(r *http.Request) (*http.Response, error) {
		r.Body.(*cdnUploadBody).reader = held
		go func() { r.Body.Read(make([]byte, 16)); close(readDone) }()
		awaitCDN(t, held.entered)
		resp := response(200, &cdnCloseSignalBody{Reader: strings.NewReader(""), closed: responseClosed}, 0)
		resp.Header.Set("x-encrypted-param", "opaque")
		return resp, nil
	})
	release := cdnRelease(t, held.release)
	done := make(chan error, 1)
	go func() {
		v, e := c.Upload(context.Background(), a, "x", "", strings.Repeat("a", 32))
		if v != "" {
			t.Error("failed held read returned parameter")
		}
		done <- e
	}()
	awaitCDN(t, responseClosed)
	select {
	case <-done:
		t.Fatal("actual borrowed read abandoned")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	awaitCDN(t, readDone)
	select {
	case e := <-done:
		if !errors.Is(e, ErrIO) || strings.Contains(e.Error(), "SECRET") {
			t.Fatalf("held error not safely refused: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("upload join did not end")
	}
	if n, e := a.ReadAt(make([]byte, 16), 0); n != 16 || e != nil {
		t.Fatal("caller ownership lost")
	}
}

// Register after cdnClient so LIFO cleanup releases before blocking Close.
// The normal path uses the same idempotent release as failure cleanup.
func cdnRelease(t *testing.T, ch chan struct{}) func() {
	t.Helper()
	var once sync.Once
	release := func() { once.Do(func() { close(ch) }) }
	t.Cleanup(release)
	return release
}

func TestCDNHeldFixtureCleanupReleasesBeforeClientClose(t *testing.T) {
	done := make(chan error, 1)
	t.Run("cleanup", func(t *testing.T) {
		body := &cdnHeldBody{entered: make(chan struct{}), observed: make(chan struct{}), release: make(chan struct{})}
		dir := mediaDir(t)
		c := cdnClient(t, func(r *http.Request) (*http.Response, error) {
			body.ctx = r.Context()
			return response(200, body, -1), nil
		})
		cdnRelease(t, body.release)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go func() { _, e := c.Download(ctx, dir, "x", "", [16]byte{}); done <- e }()
		awaitCDN(t, body.entered)
		// Returning exercises the same registered cleanup path as an assertion
		// failure: release must run before the client's blocking Close cleanup.
	})
	if e := cdnResult(t, done); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func cdnResult(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(3 * time.Second):
		t.Fatal("fixture result timed out")
		return nil
	}
}

func TestCDNRedirectAndPreCancelledNeverReplayed(t *testing.T) {
	calls := 0
	body := &cdnBody{Reader: strings.NewReader("BODY_SECRET")}
	c := cdnClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		r := response(302, body, -1)
		r.Header.Set("Location", "https://evil.invalid/QUERY_SECRET")
		return r, nil
	})
	if a, e := c.Download(context.Background(), mediaDir(t), "x", "", [16]byte{}); a != nil || !errors.Is(e, ErrHTTP) || calls != 1 || !body.closed.Load() {
		t.Fatal("redirect followed or body leaked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.Download(ctx, mediaDir(t), "x", "", [16]byte{}); !errors.Is(e, context.Canceled) || calls != 1 {
		t.Fatal("cancelled operation dispatched")
	}
	if _, e := c.Download(nil, mediaDir(t), "x", "", [16]byte{}); !errors.Is(e, ErrInvalid) || calls != 1 {
		t.Fatal("nil context accepted")
	}
}

type cdnHeldBody struct {
	ctx                        context.Context
	entered, observed, release chan struct{}
	closed                     atomic.Bool
}

func (b *cdnHeldBody) Read([]byte) (int, error) {
	close(b.entered)
	<-b.ctx.Done()
	close(b.observed)
	<-b.release
	return 0, b.ctx.Err()
}
func (b *cdnHeldBody) Close() error { b.closed.Store(true); return nil }
func awaitCDN(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("fixture timed out")
	}
}
func TestCDNAdmissionCancellationJoinsBody(t *testing.T) {
	body := &cdnHeldBody{entered: make(chan struct{}), observed: make(chan struct{}), release: make(chan struct{})}
	dir := mediaDir(t)
	var calls atomic.Int32
	c := cdnClient(t, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		body.ctx = r.Context()
		return response(200, body, -1), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := cdnRelease(t, body.release)
	done := make(chan error, 1)
	go func() { _, e := c.Download(ctx, dir, "x", "", [16]byte{}); done <- e }()
	awaitCDN(t, body.entered)
	cancel()
	awaitCDN(t, body.observed)
	waiter, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	_, e := c.Download(waiter, mediaDir(t), "x", "", [16]byte{})
	if !errors.Is(e, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatal("actual admission released early")
	}
	select {
	case <-done:
		t.Fatal("abandoned reader")
	default:
	}
	release()
	if e := cdnResult(t, done); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if !body.closed.Load() {
		t.Fatal("close missing")
	}
}
func TestCDNErrorCanariesAndClose(t *testing.T) {
	body := &cdnHeldBody{entered: make(chan struct{}), observed: make(chan struct{}), release: make(chan struct{})}
	dir := mediaDir(t)
	c := cdnClient(t, func(r *http.Request) (*http.Response, error) {
		body.ctx = r.Context()
		return response(200, body, -1), nil
	})
	release := cdnRelease(t, body.release)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { _, e := c.Download(ctx, dir, "QUERY_SECRET", "", [16]byte{}); done <- e }()
	awaitCDN(t, body.entered)
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	awaitCDN(t, body.observed)
	select {
	case <-closed:
		t.Fatal("Close did not join actual body")
	default:
	}
	if _, e := c.Download(context.Background(), dir, "x", "", [16]byte{}); e == nil {
		t.Fatal("closed client accepted work")
	}
	release()
	awaitCDN(t, closed)
	if e := cdnResult(t, done); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	noSpools(t, dir)
}
