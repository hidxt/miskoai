package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hidxt/miskoai/internal/config"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const statusBodyLimit = 2 << 20

var errStatus = errors.New("cli_status")
var errStatusUnreachable = errors.New("cli_status_unreachable: MiskoAI stopped or unreachable; run miskoai serve")

func canonicalListen(address string) (string, bool) {
	h, p, e := net.SplitHostPort(address)
	n, ne := strconv.Atoi(p)
	return h, e == nil && ne == nil && (h == "127.0.0.1" || h == "::1") && n > 0 && n <= 65535 && strconv.Itoa(n) == p && net.JoinHostPort(h, p) == address
}
func status(ctx context.Context, cfg config.Config, out io.Writer) (result error) {
	if ctx == nil {
		return errors.New("cli_context")
	}
	if ctx.Err() != nil {
		return errStatus
	}
	host, ok := canonicalListen(cfg.Listen)
	if !ok || len(cfg.AdminPassword) < 16 || len(cfg.AdminPassword) > 256 {
		return errStatus
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	network := "tcp4"
	if host == "::1" {
		network = "tcp6"
	}
	dialer := net.Dialer{Timeout: 2 * time.Second, KeepAlive: -1}
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
		if address != cfg.Listen {
			return nil, errStatus
		}
		return dialer.DialContext(ctx, network, cfg.Listen)
	}, ResponseHeaderTimeout: 3 * time.Second, MaxResponseHeaderBytes: 8 << 10, DisableCompression: true, DisableKeepAlives: true, MaxConnsPerHost: 1, MaxIdleConns: 1, IdleConnTimeout: time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	origin := "http://" + cfg.Listen
	request := func(method, path string, body []byte, cookie, csrf string) ([]byte, []*http.Cookie, error) {
		r, e := http.NewRequestWithContext(ctx, method, origin+path, bytes.NewReader(body))
		if e != nil {
			return nil, nil, errStatus
		}
		if method == http.MethodPost {
			r.Header.Set("Origin", origin)
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "MiskoAI_session", Value: cookie})
		}
		if csrf != "" {
			r.Header.Set("X-MiskoAI-CSRF", csrf)
		}
		response, e := client.Do(r)
		if e != nil {
			return nil, nil, errStatusUnreachable
		}
		defer response.Body.Close()
		data, e := io.ReadAll(io.LimitReader(response.Body, statusBodyLimit+1))
		if e != nil || len(data) > statusBodyLimit || response.StatusCode != http.StatusOK {
			return nil, nil, errStatus
		}
		values := response.Header.Values("Content-Type")
		if len(values) != 1 {
			return nil, nil, errStatus
		}
		kind, params, e := mime.ParseMediaType(values[0])
		if e != nil || kind != "application/json" {
			return nil, nil, errStatus
		}
		for k, v := range params {
			if k != "charset" || !strings.EqualFold(v, "utf-8") {
				return nil, nil, errStatus
			}
		}
		return data, response.Cookies(), nil
	}
	data, _, e := request("GET", "/api/bootstrap", nil, "", "")
	if e != nil {
		return e
	}
	nonce, e := statusToken(data, "nonce")
	if e != nil {
		return errStatus
	}
	body, _ := json.Marshal(struct {
		Password string `json:"password"`
		Nonce    string `json:"nonce"`
	}{cfg.AdminPassword, nonce})
	data, cookies, e := request("POST", "/api/login", body, "", "")
	if e != nil {
		return e
	}
	csrf, e := statusToken(data, "csrf")
	if e != nil {
		return errStatus
	}
	cookie := ""
	for _, c := range cookies {
		if c.Name == "MiskoAI_session" {
			if cookie != "" || !validStatusToken(c.Value) || c.Path != "/" || c.Domain != "" || c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.MaxAge < 0 {
				return errStatus
			}
			cookie = c.Value
		}
	}
	if cookie == "" {
		return errStatus
	}
	defer func() {
		data, _, e := request("POST", "/api/logout", []byte(`{}`), cookie, csrf)
		if e == nil {
			object, parseErr := statusObject(data)
			if parseErr != nil || len(object) != 1 || string(bytes.TrimSpace(object["ok"])) != "true" {
				e = errStatus
			}
		}
		if result == nil && e != nil {
			result = errStatus
		}
	}()
	data, _, e = request("GET", "/api/status", nil, cookie, "")
	if e != nil {
		return e
	}
	summary, e := parseStatus(data)
	if e != nil {
		return errStatus
	}
	if _, e = io.WriteString(out, summary); e != nil {
		return errStatus
	}
	return nil
}

func validStatusToken(token string) bool {
	b, e := base64.RawURLEncoding.Strict().DecodeString(token)
	return e == nil && len(token) == 43 && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == token
}

// Authentication responses have one exact key and one canonical token. Checking
// raw spelling also refuses escaped aliases which ordinary struct decoding accepts.
func statusToken(data []byte, key string) (string, error) {
	object, e := statusObject(data)
	if e != nil || len(object) != 1 {
		return "", errStatus
	}
	raw, ok := object[key]
	if !ok {
		return "", errStatus
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !validStatusToken(value) {
		return "", errStatus
	}
	return value, nil
}

func statusObject(data []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return nil, errStatus
	}
	result := make(map[string]json.RawMessage)
	for d.More() {
		// The reviewed status DTO has thirteen top-level fields; authentication
		// and attempt objects are smaller. Bound unknown-field work as well as bytes.
		if len(result) >= 32 {
			return nil, errStatus
		}
		start := d.InputOffset()
		tok, e = d.Token()
		key, ok := tok.(string)
		if e != nil || !ok || len(key) > 64 {
			return nil, errStatus
		}
		spelling := bytes.TrimSpace(data[start:d.InputOffset()])
		spelling = bytes.TrimSpace(bytes.TrimPrefix(spelling, []byte(",")))
		if string(spelling) != `"`+key+`"` {
			return nil, errStatus
		}
		if _, ok = result[key]; ok {
			return nil, errStatus
		}
		for prior := range result {
			if strings.EqualFold(prior, key) {
				return nil, errStatus
			}
		}
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return nil, errStatus
		}
		result[key] = v
	}
	tok, e = d.Token()
	if e != nil || tok != json.Delim('}') {
		return nil, errStatus
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errStatus
	}
	return result, nil
}
func parseStatus(data []byte) (string, error) {
	obj, e := statusObject(data)
	if e != nil {
		return "", e
	}
	var uptime int64
	var heap uint64
	var goroutines int
	var gc uint32
	var rss bool
	var state string
	required := map[string]any{"uptime_seconds": &uptime, "heap_bytes": &heap, "goroutines": &goroutines, "gc_count": &gc, "rss_available": &rss, "channel_state": &state}
	for k, dst := range required {
		v, ok := obj[k]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) || json.Unmarshal(v, dst) != nil {
			return "", errStatus
		}
	}
	if uptime < 0 || goroutines < 1 || rss {
		return "", errStatus
	}
	switch state {
	case "closed", "closing", "unavailable", "authorization_expired", "restore_paused", "unconfigured", "provider_unconfigured", "stopped", "receiving", "backoff", "paused", "quarantined":
	default:
		return "", errStatus
	}
	attempts, e := statusObject(obj["logical_attempts"])
	if e != nil {
		return "", errStatus
	}
	var totals [4]uint64
	for i, k := range []string{"chat", "summary", "vision", "search"} {
		v, ok := attempts[k]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) || json.Unmarshal(v, &totals[i]) != nil {
			return "", errStatus
		}
	}
	return fmt.Sprintf("MiskoAI %s\nchannel: %s\nuptime: %d seconds\nGo heap: %d bytes\ngoroutines: %d\nGC count: %d\nRSS: unavailable\nlogical attempts: chat=%d summary=%d vision=%d search=%d\n", Version, state, uptime, heap, goroutines, gc, totals[0], totals[1], totals[2], totals[3]), nil
}
