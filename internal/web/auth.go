package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"time"
)

const sessionCookie = "MiskoAI_session"

type session struct {
	csrf    [32]byte
	expires time.Time
}

// Caller holds mu, including entropy reads, so test seams and table state have
// the same protected lifetime as production authentication state.
func (s *Server) randomToken() (string, [32]byte, error) {
	var bytes [32]byte
	_, err := io.ReadFull(s.random, bytes[:])
	if err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes[:])
	return token, sha256.Sum256([]byte(token)), nil
}
func tokenDigest(token string) ([32]byte, bool) {
	if len(token) != 43 {
		return [32]byte{}, false
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(data) != 32 {
		return [32]byte{}, false
	}
	return sha256.Sum256([]byte(token)), true
}

func (s *Server) cleanup(now time.Time) {
	for key, expires := range s.nonces {
		if !now.Before(expires) {
			delete(s.nonces, key)
		}
	}
	for key, value := range s.sessions {
		if !now.Before(value.expires) {
			delete(s.sessions, key)
		}
	}
}

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	site := r.Header.Values("Sec-Fetch-Site")
	if len(site) > 1 || (len(site) == 1 && site[0] != "same-origin" && site[0] != "none") {
		safeError(w, 403, "web_forbidden")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.cleanup(now)
	if len(s.nonces) >= 64 {
		safeError(w, 503, "web_busy")
		return
	}
	token, key, err := s.randomToken()
	if err != nil {
		safeError(w, 503, "web_random")
		return
	}
	if _, exists := s.nonces[key]; exists {
		safeError(w, 503, "web_random")
		return
	}
	s.nonces[key] = now.Add(time.Minute)
	tokenResponse(w, "nonce", token)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	// Every same-origin login attempt spends the shared budget, including malformed
	// bodies; an attacker cannot bypass admission by varying parsing failures.
	s.mu.Lock()
	now := s.now()
	s.cleanup(now)
	if !s.admitLogin(now) {
		s.mu.Unlock()
		safeError(w, 429, "web_rate")
		return
	}
	s.mu.Unlock()
	values := r.Header.Values("Content-Type")
	if len(values) != 1 {
		safeError(w, 400, "web_json")
		return
	}
	media, _, err := mime.ParseMediaType(values[0])
	if err != nil || media != "application/json" {
		safeError(w, 400, "web_json")
		return
	}
	body, err := strictRequestObject(r.Body, 2048, "password", "nonce")
	if err != nil {
		safeError(w, 400, "web_json")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now = s.now()
	s.cleanup(now)
	nonce, valid := tokenDigest(body["nonce"])
	expires, exists := s.nonces[nonce]
	delete(s.nonces, nonce)
	password := sha256.Sum256([]byte(body["password"]))
	match := subtle.ConstantTimeCompare(password[:], s.password[:])
	if !valid || !exists || !now.Before(expires) || match != 1 {
		safeError(w, 401, "web_auth")
		return
	}
	if len(s.sessions) >= 64 {
		safeError(w, 503, "web_busy")
		return
	}
	token, key, err := s.randomToken()
	if err != nil {
		safeError(w, 503, "web_random")
		return
	}
	if _, exists := s.sessions[key]; exists {
		safeError(w, 503, "web_random")
		return
	}
	csrf, csrfKey, err := s.randomToken()
	if err != nil {
		safeError(w, 503, "web_random")
		return
	}
	expires = now.Add(20 * time.Minute)
	s.sessions[key] = session{csrf: csrfKey, expires: expires}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 1200, Expires: expires})
	tokenResponse(w, "csrf", csrf)
}

// Caller holds mu. A fixed ten-entry ring bounds the rolling-minute budget;
// a backward clock refuses conservatively until the last observed time returns.
func (s *Server) admitLogin(now time.Time) bool {
	if s.rateKnown && now.Before(s.rateLast) {
		return false
	}
	s.rateKnown = true
	s.rateLast = now
	for s.rateCount > 0 && !now.Before(s.rateTimes[s.rateHead].Add(time.Minute)) {
		s.rateTimes[s.rateHead] = time.Time{}
		s.rateHead = (s.rateHead + 1) % len(s.rateTimes)
		s.rateCount--
	}
	if s.rateCount == len(s.rateTimes) {
		return false
	}
	s.rateTimes[(s.rateHead+s.rateCount)%len(s.rateTimes)] = now
	s.rateCount++
	return true
}

func clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) ([32]byte, bool) {
	count := 0
	token := ""
	for _, cookie := range r.Cookies() {
		if cookie.Name == sessionCookie {
			count++
			token = cookie.Value
		}
	}
	key, valid := tokenDigest(token)
	s.mu.Lock()
	now := s.now()
	s.cleanup(now)
	_, exists := s.sessions[key]
	s.mu.Unlock()
	if count != 1 || !valid || !exists {
		if count > 0 {
			clearCookie(w)
		}
		safeError(w, 401, "web_auth")
		return [32]byte{}, false
	}
	return key, true
}

func (s *Server) checkCSRF(key [32]byte, r *http.Request) bool {
	values := r.Header.Values("X-MiskoAI-CSRF")
	if len(values) != 1 {
		return false
	}
	digest, valid := tokenDigest(values[0])
	if !valid {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, exists := s.sessions[key]
	return exists && s.now().Before(value.expires) && subtle.ConstantTimeCompare(digest[:], value.csrf[:]) == 1
}

// Refresh mints a new CSRF value; the session table retains only its digest.
// Concurrent tabs must refresh/retry after another tab rotates the token.
func (s *Server) refreshCSRF(w http.ResponseWriter, key [32]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, exists := s.sessions[key]
	if !exists || !s.now().Before(value.expires) {
		delete(s.sessions, key)
		clearCookie(w)
		safeError(w, 401, "web_auth")
		return
	}
	token, digest, err := s.randomToken()
	if err != nil {
		safeError(w, 503, "web_random")
		return
	}
	value.csrf = digest
	s.sessions[key] = value
	tokenResponse(w, "csrf", token)
}
