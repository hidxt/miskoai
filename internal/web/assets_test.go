package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAssetsExactResources(t *testing.T) {
	for _, tc := range []struct{ path, mime string }{{"/", "text/html; charset=utf-8"}, {"/app.js", "text/javascript; charset=utf-8"}, {"/style.css", "text/css; charset=utf-8"}} {
		t.Run(tc.path, func(t *testing.T) {
			get := httptest.NewRecorder()
			Assets().ServeHTTP(get, httptest.NewRequest("GET", tc.path, nil))
			if get.Code != 200 || get.Header().Get("Content-Type") != tc.mime || get.Body.Len() == 0 {
				t.Fatalf("resource: status=%d type=%q bytes=%d", get.Code, get.Header().Get("Content-Type"), get.Body.Len())
			}
			if get.Header().Get("Content-Length") != strconv.Itoa(get.Body.Len()) {
				t.Fatal("missing exact size")
			}
			head := httptest.NewRecorder()
			Assets().ServeHTTP(head, httptest.NewRequest("HEAD", tc.path, nil))
			if head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Length") != get.Header().Get("Content-Length") || head.Header().Get("Content-Type") != tc.mime {
				t.Fatal("HEAD representation differs")
			}
		})
	}
}

func TestAssetsRefuseFallbackAndMutation(t *testing.T) {
	for _, path := range []string{"/index.html", "/assets/app.js", "/missing", "/app.js/", "/../LICENSE", "/assets/../index.html"} {
		w := httptest.NewRecorder()
		Assets().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("fallback %q: %d", path, w.Code)
		}
	}
	for _, method := range []string{"POST", "PUT", "DELETE", "OPTIONS"} {
		w := httptest.NewRecorder()
		Assets().ServeHTTP(w, httptest.NewRequest(method, "/", nil))
		if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
			t.Fatalf("method %s: %d", method, w.Code)
		}
	}
}

func TestAssetsComposeWithAuthenticatedServer(t *testing.T) {
	calls := 0
	s, err := New(Options{Listen: testHost, Password: testPassword}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = io.WriteString(w, "private-synthetic") }), Assets())
	if err != nil {
		t.Fatal(err)
	}
	public := request(s, "GET", "/", "", nil, "", "")
	if public.Code != 200 || !strings.Contains(public.Header().Get("Content-Security-Policy"), "script-src 'self'") || strings.Contains(public.Header().Get("Content-Security-Policy"), "unsafe-inline") {
		t.Fatal("public panel/CSP contract")
	}
	wrongHost := httptest.NewRequest("GET", "/app.js", nil)
	wrongHost.Host = "attacker.invalid"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, wrongHost)
	if w.Code != 403 {
		t.Fatalf("Host guard: %d", w.Code)
	}
	if request(s, "GET", "/api/status", "", nil, "", "").Code != 401 || calls != 0 {
		t.Fatal("anonymous application reached")
	}
	cookie, csrf := login(t, s)
	if request(s, "GET", "/api/status", "", cookie, "", "").Code != 200 || calls != 1 {
		t.Fatal("authenticated application refused")
	}
	if request(s, "POST", "/api/settings", "{}", cookie, testOrigin, "").Code != 403 || calls != 1 {
		t.Fatal("missing CSRF reached application")
	}
	if request(s, "POST", "/api/settings", "{}", cookie, testOrigin, csrf).Code != 200 || calls != 2 {
		t.Fatal("valid CSRF application refused")
	}
}
