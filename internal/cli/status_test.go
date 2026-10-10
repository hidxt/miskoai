package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/web"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func actualStatusWeb(t *testing.T) (string, func()) {
	t.Helper()
	cfg := cliFixture(t)
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	address := l.Addr().String()
	l.Close()
	c, e := core.New(cfg, core.Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	w, e := web.New(web.Options{Listen: address, Password: cfg.AdminPassword}, web.Application(c), web.Assets())
	if e != nil {
		c.Close()
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	waitEvent(t, w.Ready())
	return address, func() {
		cancel()
		if e := waitCLI(t, done); e != nil {
			t.Error(e)
		}
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	}
}

const safeStatusFixture = `{"uptime_seconds":4,"heap_bytes":9007199254740993,"goroutines":3,"gc_count":2,"rss_available":false,"channel_state":"unconfigured","logical_attempts":{"chat":9007199254740993,"summary":0,"vision":0,"search":1},"events":[{"kind":"synthetic-canary"}]}`

func statusHTTP(t *testing.T, handler http.Handler) (config.Config, func()) {
	t.Helper()
	cfg := cliFixture(t)
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	cfg.Listen = l.Addr().String()
	server := &http.Server{Handler: handler}
	done := make(chan error, 1)
	go func() { done <- server.Serve(l) }()
	return cfg, func() {
		server.Close()
		e := waitCLI(t, done)
		if e != nil && e != http.ErrServerClosed {
			t.Error(e)
		}
	}
}
func syntheticToken() string { return base64.RawURLEncoding.EncodeToString(make([]byte, 32)) }
func TestStatusProtocolBoundsAndLogout(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
		bad        bool
	}{
		{"valid", safeStatusFixture, 200, false},
		{"status_error", `{"error":"synthetic-canary"}`, 500, true},
		{"oversize", strings.Repeat("x", statusBodyLimit+1), 200, true},
		{"malformed", `{"broken":`, 200, true},
		{"trailing", safeStatusFixture + `{}`, 200, true},
		{"duplicate", strings.Replace(safeStatusFixture, `"heap_bytes":`, `"heap_bytes":1,"heap_bytes":`, 1), 200, true},
		{"float", strings.Replace(safeStatusFixture, `"goroutines":3`, `"goroutines":3.5`, 1), 200, true},
		{"negative", strings.Replace(safeStatusFixture, `"gc_count":2`, `"gc_count":-1`, 1), 200, true},
		{"null", strings.Replace(safeStatusFixture, `"heap_bytes":9007199254740993`, `"heap_bytes":null`, 1), 200, true},
		{"unknown_state", strings.Replace(safeStatusFixture, `"unconfigured"`, `"synthetic-canary"`, 1), 200, true},
		{"rss_claim", strings.Replace(safeStatusFixture, `"rss_available":false`, `"rss_available":true`, 1), 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bootstrap, login, fetched, logout atomic.Int32
			var address string
			token := syntheticToken()
			cfg, cleanup := statusHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.RawQuery != "" || r.Host != address {
					t.Error("unexpected origin or query")
				}
				switch r.URL.Path {
				case "/api/bootstrap":
					bootstrap.Add(1)
					if r.Method != "GET" {
						t.Error("bootstrap method")
					}
					fmt.Fprintf(w, `{"nonce":%q}`, token)
				case "/api/login":
					login.Add(1)
					if r.Method != "POST" || r.Header.Get("Origin") != "http://"+address {
						t.Error("login origin/method")
					}
					var p map[string]string
					json.NewDecoder(r.Body).Decode(&p)
					if p["password"] != "synthetic-password-123" || p["nonce"] != token {
						t.Error("login fields")
					}
					http.SetCookie(w, &http.Cookie{Name: "MiskoAI_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
					fmt.Fprintf(w, `{"csrf":%q}`, token)
				case "/api/status":
					fetched.Add(1)
					c, e := r.Cookie("MiskoAI_session")
					if e != nil || c.Value != token || r.Method != "GET" {
						t.Error("status authentication")
					}
					w.WriteHeader(tc.code)
					io.WriteString(w, tc.body)
				case "/api/logout":
					logout.Add(1)
					c, e := r.Cookie("MiskoAI_session")
					if e != nil || c.Value != token || r.Method != "POST" || r.Header.Get("Origin") != "http://"+address || r.Header.Get("X-MiskoAI-CSRF") != token || r.Header.Get("X-CSRF-Token") != "" {
						t.Error("logout authentication/CSRF")
					}
					b, _ := io.ReadAll(r.Body)
					if string(b) != "{}" {
						t.Error("logout body")
					}
					io.WriteString(w, `{"ok":true}`)
				default:
					t.Error("unexpected route")
				}
			}))
			address = cfg.Listen
			defer cleanup()
			var out bytes.Buffer
			e := status(context.Background(), cfg, &out)
			if (e != nil) != tc.bad {
				t.Fatal("response acceptance", e)
			}
			if e != nil && e.Error() != errStatus.Error() {
				t.Fatal("unsafe error", e)
			}
			if strings.Contains(out.String(), "canary") {
				t.Fatal("nested payload leak")
			}
			if !tc.bad && !strings.Contains(out.String(), "9007199254740993") {
				t.Fatal("integer precision lost")
			}
			if bootstrap.Load() != 1 || login.Load() != 1 || fetched.Load() != 1 || logout.Load() != 1 {
				t.Fatal("unexpected/repeated flow", bootstrap.Load(), login.Load(), fetched.Load(), logout.Load())
			}
		})
	}
}
func TestStatusRefusesHostileBootstrap(t *testing.T) {
	token := syntheticToken()
	for _, tc := range []struct {
		name, body, ctype string
		code              int
	}{
		{"duplicate", fmt.Sprintf(`{"nonce":%q,"nonce":%q}`, token, token), "application/json", 200},
		{"alias", fmt.Sprintf(`{"Nonce":%q}`, token), "application/json", 200},
		{"escaped_alias", fmt.Sprintf(`{"\u006eonce":%q}`, token), "application/json", 200},
		{"trailing", fmt.Sprintf(`{"nonce":%q}null`, token), "application/json", 200},
		{"invalid_token", `{"nonce":"synthetic-canary"}`, "application/json", 200},
		{"oversized", strings.Repeat("x", statusBodyLimit+1), "application/json", 200},
		{"non2xx_oversized", strings.Repeat("x", statusBodyLimit+1), "application/json", 401},
		{"html", fmt.Sprintf(`{"nonce":%q}`, token), "text/html", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			cfg, cleanup := statusHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", tc.ctype)
				w.WriteHeader(tc.code)
				io.WriteString(w, tc.body)
			}))
			defer cleanup()
			var out bytes.Buffer
			e := status(context.Background(), cfg, &out)
			if e == nil || e.Error() != errStatus.Error() || out.Len() != 0 || calls.Load() != 1 {
				t.Fatal("hostile bootstrap admitted", e, out.String(), calls.Load())
			}
		})
	}
}
func TestStatusRedirectAndProxyRefusal(t *testing.T) {
	var forwarded atomic.Int32
	target, targetCleanup := statusHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1); t.Error("credential forwarded") }))
	defer targetCleanup()
	cfg, cleanup := statusHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://"+target.Listen+"/api/login?password=synthetic-canary")
		w.WriteHeader(307)
	}))
	defer cleanup()
	t.Setenv("HTTP_PROXY", "http://"+target.Listen)
	t.Setenv("HTTPS_PROXY", "http://"+target.Listen)
	t.Setenv("ALL_PROXY", "http://"+target.Listen)
	t.Setenv("NO_PROXY", "")
	var out bytes.Buffer
	e := status(context.Background(), cfg, &out)
	if e == nil || forwarded.Load() != 0 || out.Len() != 0 || strings.Contains(e.Error(), "canary") {
		t.Fatal("redirect/proxy accepted", e)
	}
}
func TestStatusWrongPasswordAndMissingPassword(t *testing.T) {
	address, cleanup := actualStatusWeb(t)
	defer cleanup()
	cfg := cliFixture(t)
	cfg.Listen = address
	t.Setenv("MISKOAI_LISTEN", address)
	cfg.AdminPassword = "synthetic-wrong-password"
	if e := status(context.Background(), cfg, io.Discard); e == nil || e.Error() != errStatus.Error() {
		t.Fatal(e)
	}
	cfg.AdminPassword = ""
	if e := status(context.Background(), cfg, io.Discard); e == nil || e.Error() != errStatus.Error() {
		t.Fatal(e)
	}
}
func TestStatusDeadlineCancellationAndUnreachable(t *testing.T) {
	cfg, cleanup := statusHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if e := status(ctx, cfg, io.Discard); e == nil || time.Since(start) > time.Second {
		t.Fatal("deadline lost", e)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if e := status(canceled, cfg, io.Discard); e == nil {
		t.Fatal("canceled status accepted")
	}
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	cfg.Listen = l.Addr().String()
	l.Close()
	if e = status(context.Background(), cfg, io.Discard); e == nil || e.Error() != errStatusUnreachable.Error() {
		t.Fatal("unreachable error", e)
	}
}

func TestStatusMissingDBAndMalformedLogout(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprint(malformed), func(t *testing.T) {
			token := syntheticToken()
			var logout atomic.Int32
			cfg, cleanup := statusHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/bootstrap":
					fmt.Fprintf(w, `{"nonce":%q}`, token)
				case "/api/login":
					http.SetCookie(w, &http.Cookie{Name: "MiskoAI_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
					fmt.Fprintf(w, `{"csrf":%q}`, token)
				case "/api/status":
					io.WriteString(w, safeStatusFixture)
				case "/api/logout":
					logout.Add(1)
					if malformed {
						io.WriteString(w, `{"ok":true}synthetic-canary`)
					} else {
						io.WriteString(w, `{"ok":true}`)
					}
				}
			}))
			defer cleanup()
			e := status(context.Background(), cfg, io.Discard)
			if (e != nil) != malformed || logout.Load() != 1 {
				t.Fatal("logout acceptance", e, logout.Load())
			}
			entries, e := os.ReadDir(cfg.DataDir)
			if e != nil || len(entries) != 0 {
				t.Fatal("status created DB/lock/journal", e, len(entries))
			}
		})
	}
}
func TestStatusTokensStrictForBothResponses(t *testing.T) {
	for _, key := range []string{"nonce", "csrf"} {
		for _, body := range []string{fmt.Sprintf(`{"%s":%q,"%s":%q}`, key, syntheticToken(), key, syntheticToken()), fmt.Sprintf(`{"%s":%q}`, strings.ToUpper(key), syntheticToken()), fmt.Sprintf(`{"%s":%q}null`, key, syntheticToken()), fmt.Sprintf(`{"%s":null}`, key), fmt.Sprintf(`{"%s":%q}`, key, syntheticToken()+"=")} {
			if _, e := statusToken([]byte(body), key); e == nil {
				t.Fatal("ambiguous token admitted")
			}
		}
	}
}

func TestStatusObjectFieldCountBound(t *testing.T) {
	var body strings.Builder
	body.WriteString("{")
	for i := 0; i < 33; i++ {
		if i != 0 {
			body.WriteString(",")
		}
		fmt.Fprintf(&body, `"synthetic_%d":0`, i)
	}
	body.WriteString("}")
	if _, e := statusObject([]byte(body.String())); e == nil {
		t.Fatal("oversized object field count admitted")
	}
}

func TestStatusLoginRedirectNeverForwardsPassword(t *testing.T) {
	var forwarded, login atomic.Int32
	token := syntheticToken()
	target, targetCleanup := statusHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer targetCleanup()
	cfg, cleanup := statusHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/bootstrap" {
			fmt.Fprintf(w, `{"nonce":%q}`, token)
			return
		}
		login.Add(1)
		w.Header().Set("Location", "http://"+target.Listen+"/api/login")
		w.WriteHeader(307)
		io.WriteString(w, `{"error":"synthetic-canary"}`)
	}))
	defer cleanup()
	var out bytes.Buffer
	e := status(context.Background(), cfg, &out)
	if e == nil || e.Error() != errStatus.Error() || forwarded.Load() != 0 || login.Load() != 1 || out.Len() != 0 {
		t.Fatal("login redirect forwarded", e, forwarded.Load(), login.Load())
	}
}
func TestStatusLogoutUsesRemainingDeadline(t *testing.T) {
	var logout atomic.Int32
	token := syntheticToken()
	cfg, cleanup := statusHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/bootstrap":
			fmt.Fprintf(w, `{"nonce":%q}`, token)
		case "/api/login":
			http.SetCookie(w, &http.Cookie{Name: "MiskoAI_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			fmt.Fprintf(w, `{"csrf":%q}`, token)
		case "/api/status":
			time.Sleep(30 * time.Millisecond)
			io.WriteString(w, safeStatusFixture)
		case "/api/logout":
			logout.Add(1)
			<-r.Context().Done()
		}
	}))
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	e := status(ctx, cfg, io.Discard)
	if e == nil || logout.Load() != 1 || time.Since(start) > 500*time.Millisecond {
		t.Fatal("logout extended/restarted deadline", e, logout.Load(), time.Since(start))
	}
}
func TestStatusAuthenticatedFlow(t *testing.T) {
	address, cleanup := actualStatusWeb(t)
	defer cleanup()
	cfg := cliFixture(t)
	cfg.Listen = address
	t.Setenv("MISKOAI_LISTEN", address)
	sentinel := []byte("synthetic invalid DB sentinel")
	path := filepath.Join(cfg.DataDir, "miskoai.db")
	if e := os.WriteFile(path, sentinel, 0600); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e := RunContext(context.Background(), []string{"status"}, &out); e != nil {
		t.Fatal("authenticated status failed", e)
	}
	for _, want := range []string{"channel: unconfigured", "Go heap:", "RSS: unavailable", "logical attempts:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal("missing summary", want, out.String())
		}
	}
	b, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(b, sentinel) {
		t.Fatal("status touched database", e)
	}
	for _, name := range []string{"miskoai.db-wal", "miskoai.db-shm", ".miskoai.lock"} {
		if _, e := os.Stat(filepath.Join(cfg.DataDir, name)); !os.IsNotExist(e) {
			t.Fatal("status created storage artifact", name, e)
		}
	}
}
func TestStatusNoCredentialLogging(t *testing.T) {
	cfg := cliFixture(t)
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	cfg.Listen = l.Addr().String()
	s := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"nonce":"synthetic-secret-canary"}`)
	})}
	defer s.Close()
	go s.Serve(l)
	var out bytes.Buffer
	e = status(context.Background(), cfg, &out)
	if e == nil || e.Error() != "cli_status" || strings.Contains(out.String()+e.Error(), "synthetic") {
		t.Fatal("unsafe status error", e, out.String())
	}
}
