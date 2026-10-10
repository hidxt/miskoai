package web

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testPassword = "synthetic-password-canary"
const testHost = "127.0.0.1:17432"
const testOrigin = "http://127.0.0.1:17432"

// Swallowing ErrAbortHandler would append a JSON error to committed download
// bytes and turn an interrupted chunked response into an apparently complete one.
func TestHTTPAbortPreservesInterruptedResponse(t *testing.T) {
	s := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, "synthetic-partial")
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}))
	cookie, _ := login(t, s)
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	client := server.Client()
	client.Timeout = 2 * time.Second
	r, err := http.NewRequest("GET", server.URL+"/api/download", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Host = testHost
	r.AddCookie(cookie)
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != io.ErrUnexpectedEOF || closeErr != nil || response.StatusCode != 200 || string(body) != "synthetic-partial" {
		t.Fatalf("abort must interrupt without appended JSON: status=%d body=%q read=%v close=%v", response.StatusCode, body, readErr, closeErr)
	}
	s.joined.Wait()
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if active != 0 {
		t.Fatal("aborted handler retained admission")
	}
}

func TestHTTPAbortSentinelPropagatesAndReleasesAdmission(t *testing.T) {
	s := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) }))
	cookie, _ := login(t, s)
	var caught any
	func() {
		defer func() { caught = recover() }()
		request(s, "GET", "/api/download", "", cookie, "", "")
	}()
	if caught != http.ErrAbortHandler {
		t.Fatal("HTTP abort sentinel was swallowed")
	}
	s.joined.Wait()
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if active != 0 {
		t.Fatal("aborted handler retained admission")
	}
}

func newTestServer(t *testing.T, app http.Handler) *Server {
	t.Helper()
	s, e := New(Options{Listen: testHost, Password: testPassword}, app, nil)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func request(s *Server, method, path, body string, cookie *http.Cookie, origin, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = s.host
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if csrf != "" {
		r.Header.Set("X-MiskoAI-CSRF", csrf)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestHostAndOriginRefusal(t *testing.T) {
	calls := 0
	s := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	for _, host := range []string{"attacker.test:17432", "localhost:17432", "127.0.0.1", "127.0.0.1:9999", "[::1]:17432"} {
		r := httptest.NewRequest("GET", "/api/bootstrap", nil)
		r.Host = host
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Errorf("host %q: %d", host, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("refusal reached application")
	}
	if w := request(s, "POST", "/api/login", `{"password":"x","nonce":"x"}`, nil, "http://attacker.test", ""); w.Code != 403 {
		t.Fatalf("foreign origin: %d", w.Code)
	}
}

func TestURLQueryAndOverrideRefusal(t *testing.T) {
	calls := 0
	s := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	cookie, csrf := login(t, s)
	for _, path := range []string{"/api/write?password=secret-canary", "/api/write?%74oken=x", "/api/write?CSRF=x", "/api/write?session_token=x", "/api/write?admin_password=x", "/api/write?api_key=x", "/api/write?bot_token=x", "/api/write?nonce=x", "/api/write?session=x", "/api/write?MiskoAI_session=x", "/api/write?x=%zz", "/api/write?x=a;b", "/api/session?q=x", "/api/bootstrap?q=x", "/api/logout?q=x", "/api/login?q=x", "/api%2fwrite"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Host = testHost
		r.AddCookie(cookie)
		r.Header.Set("Origin", testOrigin)
		r.Header.Set("X-MiskoAI-CSRF", csrf)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 && w.Code != 400 {
			t.Errorf("URL %q status %d", path, w.Code)
		}
	}
	csrf = decodeToken(t, request(s, "GET", "/api/session", "", cookie, "", ""), "csrf")
	for _, header := range []string{"X-HTTP-Method-Override", "X-Method-Override", "X-HTTP-Method"} {
		r := httptest.NewRequest("POST", "/api/write", nil)
		r.Host = testHost
		r.AddCookie(cookie)
		r.Header.Set("Origin", testOrigin)
		r.Header.Set("X-MiskoAI-CSRF", csrf)
		r.Header.Set(header, "DELETE")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Errorf("override %s: %d", header, w.Code)
		}
	}
	r := httptest.NewRequest("GET", testOrigin+"/api/write", nil)
	r.Host = testHost
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Errorf("absolute request status %d", w.Code)
	}
	if calls != 0 {
		t.Fatal("unsafe URL reached application")
	}
}

func TestAssetsWhitelistAndPanicSafety(t *testing.T) {
	assets := 0
	s, e := New(Options{Listen: testHost, Password: testPassword}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("secret-canary-password-token-path") }), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { assets++; _, _ = io.WriteString(w, "synthetic-asset") }))
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/", "/app.js", "/style.css"} {
		for _, method := range []string{"GET", "HEAD"} {
			w := request(s, method, path, "", nil, "", "")
			if w.Code != 200 || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Referrer-Policy") != "no-referrer" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
				t.Fatalf("asset %s %s status %d", method, path, w.Code)
			}
		}
	}
	for _, path := range []string{"/index.html", "/favicon.ico", "/unknown", "/secrets", "/app.js/extra"} {
		if w := request(s, "GET", path, "", nil, "", ""); w.Code != 404 {
			t.Errorf("unknown asset %s status %d", path, w.Code)
		}
	}
	if assets != 6 {
		t.Errorf("asset whitelist bypass %d", assets)
	}
	cookie, _ := login(t, s)
	w := request(s, "GET", "/api/panic?path=secret-canary-password-token-path", "", cookie, "", "")
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret-canary") || strings.Contains(w.Header().Get("Content-Security-Policy"), "unsafe-inline") {
		t.Fatalf("unsafe panic response %d %q", w.Code, w.Body.String())
	}
}
func TestOptionsRefuseUnsafeBinding(t *testing.T) {
	for _, listen := range []string{"localhost:17432", "0.0.0.0:17432", "127.0.0.1:0", "127.0.0.1:65536", "[::]:17432", "[::ffff:127.0.0.1]:17432", "127.0.0.1:017432"} {
		if _, e := New(Options{Listen: listen, Password: testPassword}, nil, nil); e == nil {
			t.Errorf("accepted %q", listen)
		}
	}
	for _, pw := range []string{"short", strings.Repeat("x", 257), string([]byte{255}), strings.Repeat(" ", 16), "synthetic-pass\x00word", "synthetic-pass\nword"} {
		if _, e := New(Options{Listen: testHost, Password: pw}, nil, nil); e == nil {
			t.Error("accepted invalid password")
		}
	}
}

func TestHTTPHandlerAdmissionAndShutdown(t *testing.T) {
	entered := make(chan struct{}, 32)
	release := make(chan struct{})
	done := make(chan struct{}, 32)
	s := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { entered <- struct{}{}; <-release; w.WriteHeader(204) }))
	cookie, csrf := login(t, s)
	for i := 0; i < 32; i++ {
		go func() { request(s, "GET", "/api/held", "", cookie, "", csrf); done <- struct{}{} }()
	}
	for i := 0; i < 32; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			close(release)
			t.Fatal("handler not admitted")
		}
	}
	if w := request(s, "GET", "/api/held", "", cookie, "", csrf); w.Code != 503 {
		t.Errorf("busy %d", w.Code)
	}
	close(release)
	for i := 0; i < 32; i++ {
		<-done
	}

	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := listener.Addr().String()
	if e = listener.Close(); e != nil {
		t.Fatal(e)
	}
	started := make(chan struct{})
	cancelSeen := make(chan struct{})
	allowReturn := make(chan struct{})
	joined := make(chan struct{})
	actual, e := New(Options{Listen: addr, Password: testPassword}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(cancelSeen)
		<-allowReturn
		close(joined)
	}), nil)
	if e != nil {
		t.Fatal(e)
	}
	c, _ := login(t, actual)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- actual.Run(ctx) }()
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	ready := false
	for i := 0; i < 100; i++ {
		resp, err := client.Get("http://" + addr + "/")
		if err == nil {
			_ = resp.Body.Close()
			ready = true
			break
		}
		select {
		case err := <-runDone:
			t.Fatalf("run failed: %v", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		cancel()
		t.Fatal("listener never ready")
	}
	// Hold a legitimately admitted TCP connection with no request header.
	// The later started HTTP handler proves the listener accepted both.
	idleConnection, err := net.Dial("tcp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer idleConnection.Close()
	req, _ := http.NewRequest("GET", "http://"+addr+"/api/held", nil)
	req.AddCookie(c)
	requestDone := make(chan struct{})
	go func() {
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		close(requestDone)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("actual handler never started")
	}
	cancel()
	select {
	case <-cancelSeen:
	case <-time.After(3 * time.Second):
		close(allowReturn)
		t.Fatal("shutdown did not cancel handler")
	}
	select {
	case err := <-runDone:
		close(allowReturn)
		t.Fatalf("returned before handler joined: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	close(allowReturn)
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	// A new TCP connection may legitimately remain in net/http shutdown
	// until ReadHeaderTimeout (5s) or StateNew quiescence (>5s), plus polling.
	// This is only a test watchdog; held-handler return is asserted above.
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return")
	}
	<-joined
	<-requestDone
	if err := idleConnection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var probe [1]byte
	if n, err := idleConnection.Read(probe[:]); n != 0 || err != io.EOF {
		t.Fatalf("Run returned without closing idle TCP connection: bytes=%d error=%v", n, err)
	}
}

func TestApplicationDeadlinesAndSafeFallback(t *testing.T) {
	s := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Error("missing deadline")
		}
		remaining := time.Until(deadline)
		want := 10 * time.Second
		if r.URL.Path == "/api/backup" || r.URL.Path == "/api/restore" {
			want = 30 * time.Second
		}
		if remaining > want || remaining < want-time.Second {
			t.Errorf("%s deadline %s", r.URL.Path, remaining)
		}
		w.WriteHeader(204)
	}))
	cookie, csrf := login(t, s)
	for _, path := range []string{"/api/read", "/api/backup", "/api/restore"} {
		if w := request(s, "POST", path, "", cookie, testOrigin, csrf); w.Code != 204 {
			t.Fatal("authenticated deadline dispatch failed")
		}
	}
	empty := newTestServer(t, nil)
	if w := request(empty, "GET", "/unknown-secret-canary", "", nil, "", ""); w.Code != 404 || strings.Contains(w.Body.String(), "canary") {
		t.Fatal("unsafe asset fallback")
	}
	c, _ := login(t, empty)
	if w := request(empty, "GET", "/api/unknown-secret-canary", "", c, "", ""); w.Code != 404 || strings.Contains(w.Body.String(), "canary") {
		t.Fatal("unsafe application fallback")
	}
}

func TestRunBindFailureJoinsAdmittedWork(t *testing.T) {
	occupied, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer occupied.Close()
	entered := make(chan struct{})
	cancelSeen := make(chan struct{})
	release := make(chan struct{})
	returned := make(chan struct{})
	s, e := New(Options{Listen: occupied.Addr().String(), Password: testPassword}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(cancelSeen)
		<-release
		close(returned)
	}), nil)
	if e != nil {
		t.Fatal(e)
	}
	cookie, _ := login(t, s)
	handlerDone := make(chan struct{})
	go func() { request(s, "GET", "/api/held", "", cookie, "", ""); close(handlerDone) }()
	<-entered
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(context.Background()) }()
	select {
	case <-cancelSeen:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("bind error did not cancel admitted work")
	}
	select {
	case err := <-runDone:
		close(release)
		t.Fatalf("bind error returned before join: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-runDone:
		if err == nil || err.Error() != "web_bind" {
			t.Fatalf("unsafe bind error %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bind lifetime never joined")
	}
	<-returned
	<-handlerDone
	if w := request(s, "GET", "/api/bootstrap", "", nil, "", ""); w.Code != 503 {
		t.Fatal("admission continued after failed lifetime")
	}
	if e := s.Run(context.Background()); e == nil {
		t.Fatal("server lifetime reused")
	}
	pre := newTestServer(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := pre.Run(ctx); e != nil {
		t.Fatal(e)
	}
	if w := request(pre, "GET", "/api/bootstrap", "", nil, "", ""); w.Code != 503 {
		t.Fatal("admission continued after canceled lifetime")
	}
}
func decodeToken(t *testing.T, w *httptest.ResponseRecorder, key string) string {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("%s status %d", key, w.Code)
	}
	var obj map[string]string
	if e := json.Unmarshal(w.Body.Bytes(), &obj); e != nil {
		t.Fatal(e)
	}
	token := obj[key]
	if len(token) != 43 {
		t.Fatalf("%s length %d", key, len(token))
	}
	return token
}
