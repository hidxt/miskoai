// Package web supplies the loopback management HTTP security boundary.
package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Options contains private configuration supplied by the process owner.
type Options struct{ Listen, Password string }

// Server owns HTTP admission and ephemeral authentication, never application storage.
// New constructs a single-use Run lifetime. Handler can also serve synthetic requests.
type Server struct {
	host, origin, network string
	password              [32]byte
	application, assets   http.Handler
	now                   func() time.Time
	random                io.Reader
	mu                    sync.Mutex
	nonces                map[[32]byte]time.Time
	sessions              map[[32]byte]session
	rateTimes             [10]time.Time
	rateHead, rateCount   int
	rateLast              time.Time
	rateKnown             bool
	active                int
	closing, runStarted   bool
	joined                sync.WaitGroup
	workCtx               context.Context
	cancelWork            context.CancelFunc
	ready                 chan struct{}
}

// New accepts only canonical literal loopback addresses and a bounded UTF-8 password.
func New(o Options, application, assets http.Handler) (*Server, error) {
	host, port, err := net.SplitHostPort(o.Listen)
	if err != nil || (host != "127.0.0.1" && host != "::1") {
		return nil, errors.New("web_options")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 || strconv.Itoa(p) != port || net.JoinHostPort(host, port) != o.Listen || len(o.Password) < 16 || len(o.Password) > 256 || !utf8.ValidString(o.Password) || strings.TrimSpace(o.Password) == "" || strings.IndexFunc(o.Password, unicode.IsControl) >= 0 {
		return nil, errors.New("web_options")
	}
	network := "tcp4"
	if host == "::1" {
		network = "tcp6"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{host: o.Listen, origin: "http://" + o.Listen, network: network, password: sha256.Sum256([]byte(o.Password)), application: application, assets: assets, now: time.Now, random: rand.Reader, nonces: make(map[[32]byte]time.Time), sessions: make(map[[32]byte]session), workCtx: ctx, cancelWork: cancel, ready: make(chan struct{})}, nil
}

// Ready reports the completed successful-bind startup event, not ongoing health.
// Consumers must also observe their context and the actual Run result.
func (s *Server) Ready() <-chan struct{} { return s.ready }

// Handler enforces admission, exact Host, authentication and same-origin mutations.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serveHTTP) }

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	s.mu.Lock()
	if s.closing || s.active >= 32 {
		s.mu.Unlock()
		safeError(w, 503, "web_busy")
		return
	}
	s.active++
	s.joined.Add(1)
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.active--; s.mu.Unlock(); s.joined.Done() }()
	defer func() {
		if value := recover(); value != nil {
			// A committed download cannot become a JSON error response. Let
			// net/http interrupt it without logging a caller-controlled panic.
			if value == http.ErrAbortHandler {
				panic(value)
			}
			safeError(w, 500, "web_internal")
		}
	}()
	if r.Host != s.host || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-Host") != "" || r.Header.Get("X-Forwarded-Proto") != "" {
		safeError(w, 403, "web_forbidden")
		return
	}
	if !safeRequestTarget(r) {
		safeError(w, 403, "web_forbidden")
		return
	}
	switch r.Method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE":
	default:
		safeError(w, 405, "web_method")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" && !s.sameOrigin(r) {
		safeError(w, 403, "web_forbidden")
		return
	}
	switch r.URL.Path {
	case "/api/bootstrap":
		if r.Method != "GET" {
			safeError(w, 405, "web_method")
			return
		}
		s.bootstrap(w, r)
		return
	case "/api/login":
		if r.Method != "POST" {
			safeError(w, 405, "web_method")
			return
		}
		s.login(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		key, ok := s.authenticate(w, r)
		if !ok {
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && !s.checkCSRF(key, r) {
			safeError(w, 403, "web_forbidden")
			return
		}
		switch r.URL.Path {
		case "/api/session":
			if r.Method != "GET" {
				safeError(w, 405, "web_method")
				return
			}
			s.refreshCSRF(w, key)
			return
		case "/api/logout":
			if r.Method != "POST" {
				safeError(w, 405, "web_method")
				return
			}
			s.mu.Lock()
			delete(s.sessions, key)
			s.mu.Unlock()
			clearCookie(w)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
		if s.application == nil {
			safeError(w, 404, "web_not_found")
			return
		}
		timeout := 10 * time.Second
		if r.URL.Path == "/api/backup" || r.URL.Path == "/api/restore" {
			timeout = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		stop := context.AfterFunc(s.workCtx, cancel)
		defer stop()
		defer cancel()
		if s.workCtx.Err() != nil {
			cancel()
		}
		s.application.ServeHTTP(w, r.WithContext(ctx))
		return
	}
	if (r.Method == "GET" || r.Method == "HEAD") && s.assets != nil && (r.URL.Path == "/" || r.URL.Path == "/app.js" || r.URL.Path == "/style.css") {
		s.assets.ServeHTTP(w, r)
		return
	}
	safeError(w, 404, "web_not_found")
}

func safeRequestTarget(r *http.Request) bool {
	if r.URL == nil || r.URL.IsAbs() || r.URL.Opaque != "" || r.URL.User != nil || r.URL.Fragment != "" || r.URL.RawPath != "" || !strings.HasPrefix(r.RequestURI, "/") || strings.HasPrefix(r.RequestURI, "//") {
		return false
	}
	if r.Header.Get("X-HTTP-Method-Override") != "" || r.Header.Get("X-Method-Override") != "" || r.Header.Get("X-HTTP-Method") != "" {
		return false
	}
	switch r.URL.Path {
	case "/api/bootstrap", "/api/login", "/api/session", "/api/logout":
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			return false
		}
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return false
	}
	for name := range query {
		switch strings.ToLower(name) {
		case "password", "nonce", "token", "csrf", "session", "miskoai_session", "session_token", "admin_password", "api_key", "bot_token":
			return false
		}
	}
	return true
}

func (s *Server) sameOrigin(r *http.Request) bool {
	values := r.Header.Values("Origin")
	return len(values) == 1 && values[0] == s.origin
}

func securityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
}

// Run binds the configured literal loopback address, cancels work and joins every
// admitted handler before returning. A deadline never stands in for that join.
func (s *Server) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("web_context")
	}
	s.mu.Lock()
	if s.runStarted || s.closing {
		s.mu.Unlock()
		return errors.New("web_lifetime")
	}
	s.runStarted = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.closing = true; s.mu.Unlock(); s.cancelWork(); s.joined.Wait() }()
	if ctx.Err() != nil {
		return nil
	}
	listener, err := net.Listen(s.network, s.host)
	if err != nil {
		return errors.New("web_bind")
	}
	httpServer := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8 << 10, ErrorLog: log.New(safeLogWriter{}, "", 0)}
	served := make(chan error, 1)
	go func() { served <- httpServer.Serve(listener) }()
	close(s.ready)
	select {
	case err = <-served:
	case <-ctx.Done():
		s.mu.Lock()
		s.closing = true
		s.mu.Unlock()
		s.cancelWork()
		// Background is intentional: actual handler return is a required lifetime join.
		if shutdownErr := httpServer.Shutdown(context.Background()); shutdownErr != nil {
			_ = httpServer.Close()
		}
		err = <-served
	}
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	s.cancelWork()
	_ = httpServer.Close()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.New("web_serve")
	}
	return nil
}

// net/http diagnostics can include caller-controlled text. Emit only a fixed code.
type safeLogWriter struct{}

func (safeLogWriter) Write(p []byte) (int, error) { log.Print("web_http_error"); return len(p), nil }
