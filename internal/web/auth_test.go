package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func bootstrap(t *testing.T, s *Server) string {
	t.Helper()
	return decodeToken(t, request(s, "GET", "/api/bootstrap", "", nil, "", ""), "nonce")
}
func loginBody(password, nonce string) string {
	b, _ := json.Marshal(map[string]string{"password": password, "nonce": nonce})
	return string(b)
}
func login(t *testing.T, s *Server) (*http.Cookie, string) {
	t.Helper()
	nonce := bootstrap(t, s)
	w := request(s, "POST", "/api/login", loginBody(testPassword, nonce), nil, "http://"+s.host, "")
	csrf := decodeToken(t, w, "csrf")
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing unique cookie")
	}
	return cookies[0], csrf
}
func TestLoginNonceAndSessionFixation(t *testing.T) {
	s := newTestServer(t, nil)
	nonce := bootstrap(t, s)
	supplied := &http.Cookie{Name: "MiskoAI_session", Value: strings.Repeat("x", 43)}
	w := request(s, "POST", "/api/login", loginBody(testPassword, nonce), supplied, testOrigin, "")
	csrf := decodeToken(t, w, "csrf")
	cookie := w.Result().Cookies()[0]
	if cookie.Name != "MiskoAI_session" || cookie.Value == supplied.Value || len(cookie.Value) != 43 || cookie.Path != "/" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" || cookie.Secure || cookie.MaxAge != 1200 {
		t.Fatalf("invalid session cookie: %+v", cookie)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unsafe cache/CORS")
	}
	if next := request(s, "POST", "/api/login", loginBody(testPassword, nonce), nil, testOrigin, ""); next.Code != 401 {
		t.Fatalf("replay status %d", next.Code)
	}
	if next := request(s, "GET", "/api/session", "", supplied, "", ""); next.Code != 401 {
		t.Fatalf("fixation status %d", next.Code)
	}
	next := request(s, "GET", "/api/session", "", cookie, "", "")
	fresh := decodeToken(t, next, "csrf")
	if fresh == csrf {
		t.Fatal("CSRF did not rotate")
	}
	r := httptest.NewRequest("GET", "/api/bootstrap", nil)
	r.Host = testHost
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross site bootstrap allowed")
	}
}
func TestCSRFAndLogout(t *testing.T) {
	calls := 0
	s := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	cookie, old := login(t, s)
	fresh := decodeToken(t, request(s, "GET", "/api/session", "", cookie, "", ""), "csrf")
	for _, tc := range []struct{ method, origin, token string }{{"POST", testOrigin, old}, {"POST", testOrigin, "wrong"}, {"POST", "", fresh}, {"POST", "http://attacker.test", fresh}, {"TRACE", testOrigin, fresh}, {"OPTIONS", testOrigin, fresh}} {
		if w := request(s, tc.method, "/api/write", "", cookie, tc.origin, tc.token); w.Code != 403 && w.Code != 405 {
			t.Errorf("refusal %v status %d", tc, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("refused request reached app")
	}
	if w := request(s, "POST", "/api/write", "", cookie, testOrigin, fresh); w.Code != 204 || calls != 1 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authorized write failed")
	}
	w := request(s, "POST", "/api/logout", "", cookie, testOrigin, fresh)
	if w.Code != 200 || w.Body.String() != `{"ok":true}` || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout did not clear")
	}
	if w = request(s, "GET", "/api/write", "", cookie, "", ""); w.Code != 401 || calls != 1 {
		t.Fatal("logout session still usable")
	}
}
func TestSessionExpiryAndCaps(t *testing.T) {
	s := newTestServer(t, nil)
	clock := time.Unix(1800000000, 0)
	s.now = func() time.Time { return clock }
	first, _ := login(t, s)
	for i := 1; i < 64; i++ {
		if i%9 == 0 {
			clock = clock.Add(time.Minute + time.Second)
		}
		login(t, s)
	}
	clock = clock.Add(time.Minute + time.Second)
	nonce := bootstrap(t, s)
	if w := request(s, "POST", "/api/login", loginBody(testPassword, nonce), nil, testOrigin, ""); w.Code != 503 {
		t.Fatalf("full session table status %d", w.Code)
	}
	clock = time.Unix(1800000000, 0).Add(20 * time.Minute)
	clock = clock.Add(-time.Second)
	decodeToken(t, request(s, "GET", "/api/session", "", first, "", ""), "csrf")
	clock = clock.Add(time.Second)
	w := request(s, "GET", "/api/session", "", first, "", "")
	if w.Code != 401 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("expired session accepted/not cleared")
	}
	n := newTestServer(t, nil)
	n.now = func() time.Time { return clock }
	for i := 0; i < 64; i++ {
		bootstrap(t, n)
	}
	if w := request(n, "GET", "/api/bootstrap", "", nil, "", ""); w.Code != 503 {
		t.Fatal("nonce cap unbounded")
	}
	clock = clock.Add(time.Minute)
	bootstrap(t, n)
	expired := bootstrap(t, n)
	clock = clock.Add(time.Minute)
	if w := request(n, "POST", "/api/login", loginBody(testPassword, expired), nil, testOrigin, ""); w.Code != 401 {
		t.Fatal("expired nonce accepted")
	}
}

func TestRandomFailureAndCollisionRefuseSafely(t *testing.T) {
	s := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	cookie, csrf := login(t, s)
	s.random = strings.NewReader("")
	if w := request(s, "GET", "/api/session", "", cookie, "", ""); w.Code != 503 || w.Body.String() != `{"error":"web_random"}` {
		t.Fatal("random refresh error unsafe")
	}
	if w := request(s, "POST", "/api/write", "", cookie, testOrigin, csrf); w.Code != 204 {
		t.Fatal("random refresh failure changed session")
	}
	if w := request(s, "GET", "/api/bootstrap", "", nil, "", ""); w.Code != 503 {
		t.Fatal("random bootstrap failure accepted")
	}
	n := newTestServer(t, nil)
	n.random = strings.NewReader(strings.Repeat("\x00", 64))
	nonce := bootstrap(t, n)
	if w := request(n, "GET", "/api/bootstrap", "", nil, "", ""); w.Code != 503 {
		t.Fatal("nonce collision overwrote prior nonce")
	}
	if w := request(n, "POST", "/api/login", loginBody(testPassword, nonce), nil, testOrigin, ""); w.Code != 503 || len(w.Result().Cookies()) != 0 {
		t.Fatal("entropy failure created a partial session")
	}
}
func TestLoginGlobalRateBound(t *testing.T) {
	s := newTestServer(t, nil)
	clock := time.Unix(1800000000, 0)
	s.now = func() time.Time { return clock }
	for i := 0; i < 10; i++ {
		w := request(s, "POST", "/api/login", loginBody("wrong", bootstrap(t, s)), nil, testOrigin, "")
		if w.Code != 401 {
			t.Fatalf("attempt %d status %d", i, w.Code)
		}
	}
	if w := request(s, "POST", "/api/login", loginBody(testPassword, bootstrap(t, s)), nil, testOrigin, ""); w.Code != 429 {
		t.Fatalf("rate cap %d", w.Code)
	}
	clock = clock.Add(time.Minute)
	login(t, s)
}

func TestMalformedLoginUsesSharedRateBudget(t *testing.T) {
	s := newTestServer(t, nil)
	for i := 0; i < 10; i++ {
		if w := request(s, "POST", "/api/login", `{"password":null}`, nil, testOrigin, ""); w.Code != 400 {
			t.Fatalf("malformed attempt %d status %d", i, w.Code)
		}
	}
	if w := request(s, "POST", "/api/login", `{}`, nil, testOrigin, ""); w.Code != 429 {
		t.Fatalf("malformed login bypassed global cap: %d", w.Code)
	}
}

func TestLoginRollingMinuteAndBackwardClock(t *testing.T) {
	s := newTestServer(t, nil)
	clock := time.Unix(1800000000, 0)
	s.now = func() time.Time { return clock }
	attempt := func() int {
		return request(s, "POST", "/api/login", `{"password":"wrong","nonce":"x"}`, nil, testOrigin, "").Code
	}
	if got := attempt(); got != 401 {
		t.Fatal(got)
	}
	clock = clock.Add(59 * time.Second)
	for i := 0; i < 9; i++ {
		if got := attempt(); got != 401 {
			t.Fatalf("near boundary attempt %d: %d", i, got)
		}
	}
	clock = clock.Add(time.Second)
	if got := attempt(); got != 401 {
		t.Fatalf("expired oldest slot not reclaimed: %d", got)
	}
	if got := attempt(); got != 429 {
		t.Fatalf("fixed-window boundary burst bypass: %d", got)
	}
	clock = clock.Add(-time.Minute)
	if got := attempt(); got != 429 {
		t.Fatalf("backward clock replenished budget: %d", got)
	}
	clock = time.Unix(1800000000, 0).Add(119 * time.Second)
	for i := 0; i < 9; i++ {
		if got := attempt(); got != 401 {
			t.Fatalf("expired near-boundary slot %d not reclaimed: %d", i, got)
		}
	}
	if got := attempt(); got != 429 {
		t.Fatalf("rolling slots exceeded: %d", got)
	}
	fresh := newTestServer(t, nil)
	fresh.now = func() time.Time { return clock }
	if w := request(fresh, "POST", "/api/login", `{}`, nil, testOrigin, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	clock = clock.Add(-time.Second)
	if w := request(fresh, "POST", "/api/login", `{}`, nil, testOrigin, ""); w.Code != 429 {
		t.Fatal("backward clock admitted despite available slots")
	}
}

func TestLoginBodyRefusalAndNonceConsumption(t *testing.T) {
	s := newTestServer(t, nil)
	nonce := bootstrap(t, s)
	for _, body := range []string{`{"password":"a","password":"b","nonce":"x"}`, `{"password":null,"nonce":"x"}`, `{"password":"a","nonce":"x"} {}`, strings.Repeat(" ", 2049), "{\"password\":\"" + string([]byte{255}) + "\",\"nonce\":\"x\"}"} {
		w := request(s, "POST", "/api/login", body, nil, testOrigin, "")
		if w.Code != 400 || strings.Contains(w.Body.String(), testPassword) {
			t.Fatalf("invalid login status/body %d %q", w.Code, w.Body.String())
		}
	}
	wrong := request(s, "POST", "/api/login", loginBody("wrong-canary", nonce), nil, testOrigin, "")
	if wrong.Code != 401 || strings.Contains(wrong.Body.String(), "canary") {
		t.Fatal("unsafe password failure")
	}
	if w := request(s, "POST", "/api/login", loginBody(testPassword, nonce), nil, testOrigin, ""); w.Code != 401 {
		t.Fatal("wrong password did not consume nonce")
	}
}

func TestAuthHeadersAndCookieAmbiguity(t *testing.T) {
	calls := 0
	s := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	cookie, csrf := login(t, s)
	for _, kind := range []string{"origin-duplicate", "csrf-duplicate", "cookie-duplicate", "forwarded", "forwarded-host", "forwarded-proto"} {
		r := httptest.NewRequest("POST", "/api/write", nil)
		r.Host = testHost
		r.AddCookie(cookie)
		r.Header.Set("Origin", testOrigin)
		r.Header.Set("X-MiskoAI-CSRF", csrf)
		switch kind {
		case "origin-duplicate":
			r.Header.Add("Origin", testOrigin)
		case "csrf-duplicate":
			r.Header.Add("X-MiskoAI-CSRF", csrf)
		case "cookie-duplicate":
			r.AddCookie(cookie)
		case "forwarded":
			r.Header.Set("Forwarded", "host="+testHost)
		case "forwarded-host":
			r.Header.Set("X-Forwarded-Host", testHost)
		case "forwarded-proto":
			r.Header.Set("X-Forwarded-Proto", "http")
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 401 && w.Code != 403 {
			t.Errorf("ambiguity %s status %d", kind, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("ambiguous auth reached app")
	}
	if w := request(s, "POST", "/api/write", "", cookie, testOrigin, csrf); w.Code != 204 || calls != 1 {
		t.Fatal("valid auth not admitted")
	}
}
