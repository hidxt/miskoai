// Adapted from Tencent/openclaw-weixin (MIT), commit
// 24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c. See retained license.
package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://ilinkai.weixin.qq.com"
	// ProtocolVersion identifies the upstream wire implementation this PoC follows.
	// Independent client-version acceptance has not been verified with the service.
	ProtocolVersion  = "2.4.9"
	maxResponseBytes = 2 * 1024 * 1024
	maxTextBytes     = 16 * 1024
	maxOpaqueBytes   = 16 * 1024
)

var (
	ErrInvalidInput   = errors.New("weixin: invalid input")
	ErrEndpoint       = errors.New("weixin: endpoint rejected")
	ErrHTTP           = errors.New("weixin: HTTP status rejected")
	ErrProtocol       = errors.New("weixin: invalid service response")
	ErrResponseLimit  = errors.New("weixin: response size limit exceeded")
	ErrAuthExpired    = errors.New("weixin: account authorization expired")
	ErrUnauthorized   = errors.New("weixin: unauthorized")
	ErrRateLimited    = errors.New("weixin: rate limited")
	ErrService        = errors.New("weixin: service rejected request")
	ErrTransport      = errors.New("weixin: transport failed")
	ErrOutcomeUnknown = errors.New("weixin: send outcome unknown; reconcile before resending")
)

type Client struct {
	base       *url.URL
	token      string
	httpClient *http.Client
	slot       chan struct{}
	pollSlot   chan struct{}
}

// New creates a production TLS client. Redirects and environment proxies are
// disabled. DNS answers are validated and pinned for each TCP connection.
func New(baseURL, token string) (*Client, error) {
	u, e := validateBase(baseURL)
	if e != nil {
		return nil, e
	}
	if len(token) > maxOpaqueBytes || strings.ContainsAny(token, "\r\n") {
		return nil, ErrInvalidInput
	}
	tr := &http.Transport{Proxy: nil, DialContext: dialPublic, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 40 * time.Second, MaxResponseHeaderBytes: 32 * 1024, MaxConnsPerHost: 2, MaxIdleConns: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 60 * time.Second}
	return &Client{base: u, token: strings.TrimSpace(token), slot: make(chan struct{}, 1), pollSlot: make(chan struct{}, 1), httpClient: &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func approvedHost(h string) bool {
	h = strings.ToLower(h)
	return strings.HasSuffix(h, ".weixin.qq.com") && !strings.ContainsAny(h, "/@\\:")
}
func validateBase(s string) (*url.URL, error) {
	u, e := url.Parse(s)
	if e != nil || u.Scheme != "https" || u.User != nil || !approvedHost(u.Hostname()) || (u.Port() != "" && u.Port() != "443") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return nil, ErrEndpoint
	}
	return u, nil
}
func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// Restrict IPv6 to the global allocation. In particular, do not accept
	// NAT64 translation prefixes that could encode a private IPv4 destination.
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	// Teredo and 6to4 can tunnel traffic to embedded IPv4 destinations;
	// being inside the IPv6 global allocation does not make them acceptable.
	for _, p := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001::/32", "2001:db8::/32", "2002::/16"} {
		if netip.MustParsePrefix(p).Contains(ip) {
			return false
		}
	}
	return true
}
func dialPublic(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil || port != "443" || !approvedHost(host) {
		return nil, ErrEndpoint
	}
	ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if e != nil || len(ips) == 0 {
		return nil, ErrTransport
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, ErrEndpoint
		}
	}
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	for _, ip := range ips {
		conn, e := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, ErrTransport
}

type baseInfo struct {
	Version string `json:"channel_version"`
	Agent   string `json:"bot_agent"`
}

func metadata() baseInfo { return baseInfo{ProtocolVersion, "MiskoAI/0.1.0"} }

// call preserves the typed login/send contract over the same bounded exchange.
func (c *Client) call(ctx context.Context, base *url.URL, method, path string, body any, authenticated, send bool, timeout time.Duration, out any) error {
	raw, err := c.exchange(ctx, base, method, path, body, authenticated, send, timeout)
	if err != nil {
		return err
	}
	if err = responseStatus(raw); err != nil {
		if send && errors.Is(err, ErrProtocol) {
			return ErrOutcomeUnknown
		}
		return err
	}
	if err = json.Unmarshal(raw, out); err != nil {
		if send {
			return ErrOutcomeUnknown
		}
		return ErrProtocol
	}
	return nil
}

func (c *Client) exchange(ctx context.Context, base *url.URL, method, path string, body any, authenticated, send bool, timeout time.Duration) ([]byte, error) {
	if ctx == nil {
		return nil, ErrInvalidInput
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// One long poll can coexist with one outbound/login operation. Repeated
	// polls remain serialized, without delaying a conversation reply.
	slot := c.slot
	if path == "/ilink/bot/getupdates" {
		slot = c.pollSlot
	}
	select {
	case slot <- struct{}{}:
		defer func() { <-slot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var data []byte
	var e error
	if body != nil {
		data, e = json.Marshal(body)
		if e != nil || len(data) > maxResponseBytes {
			return nil, ErrInvalidInput
		}
	}
	u := base.ResolveReference(&url.URL{Path: strings.SplitN(path, "?", 2)[0]})
	if parts := strings.SplitN(path, "?", 2); len(parts) == 2 {
		u.RawQuery = parts[1]
	}
	req, e := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(data))
	if e != nil {
		return nil, ErrInvalidInput
	}
	req.Header.Set("iLink-App-Id", "bot")
	req.Header.Set("iLink-App-ClientVersion", strconv.Itoa(2<<16|4<<8|9))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("AuthorizationType", "ilink_bot_token")
		var random [4]byte
		if _, e = rand.Read(random[:]); e != nil {
			return nil, ErrTransport
		}
		req.Header.Set("X-WECHAT-UIN", base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(binary.BigEndian.Uint32(random[:])), 10))))
	}
	if authenticated {
		if c.token == "" {
			return nil, ErrUnauthorized
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, e := c.httpClient.Do(req)
	if e != nil {
		if send {
			return nil, ErrOutcomeUnknown
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrTransport
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		switch res.StatusCode {
		case 401, 403:
			return nil, ErrUnauthorized
		case 429:
			return nil, ErrRateLimited
		}
		if send && res.StatusCode >= 500 {
			return nil, ErrOutcomeUnknown
		}
		return nil, ErrHTTP
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if e != nil {
		if send {
			return nil, ErrOutcomeUnknown
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrTransport
	}
	if len(raw) > maxResponseBytes {
		if send {
			return nil, ErrOutcomeUnknown
		}
		return nil, ErrResponseLimit
	}
	return raw, nil
}

// GetUpdates performs one bounded poll. The caller durably ingests messages and
// cursor together; this adapter does not acknowledge/persist or retry them.
func (c *Client) GetUpdates(ctx context.Context, cursor string) (Updates, error) {
	raw, err := c.RawUpdates(ctx, cursor)
	if err != nil {
		return Updates{}, err
	}
	return DecodeUpdates(raw)
}

// RawUpdates performs one bounded poll for durable ingestion.
func (c *Client) RawUpdates(ctx context.Context, cursor string) ([]byte, error) {
	if len(cursor) > maxOpaqueBytes {
		return nil, ErrInvalidInput
	}
	body := struct {
		Cursor string   `json:"get_updates_buf"`
		Base   baseInfo `json:"base_info"`
	}{cursor, metadata()}
	raw, err := c.exchange(ctx, c.base, "POST", "/ilink/bot/getupdates", body, true, false, 40*time.Second)
	if err != nil {
		return nil, err
	}
	// Malformed successes must reach private durable quarantine unchanged.
	if err = responseStatus(raw); err != nil && !errors.Is(err, ErrProtocol) {
		return nil, err
	}
	return raw, nil
}

// SendText makes exactly one request. Context and clientID must come from the
// authorized conversation and persisted outbox respectively.
func (c *Client) SendText(ctx context.Context, to, contextToken, clientID, text string) error {
	if to == "" || contextToken == "" || clientID == "" || text == "" || len(text) > maxTextBytes || len(to) > maxOpaqueBytes || len(contextToken) > maxOpaqueBytes || len(clientID) > maxOpaqueBytes {
		return ErrInvalidInput
	}
	msg := Message{ToUserID: to, ContextToken: contextToken, ClientID: clientID, MessageType: 2, MessageState: 2, Items: []Item{{Type: 1, Text: &TextItem{Text: text}}}}
	// The official send builder explicitly sends an empty from_user_id.
	body := struct {
		Msg  any      `json:"msg"`
		Base baseInfo `json:"base_info"`
	}{struct {
		Message
		From string `json:"from_user_id"`
	}{msg, ""}, metadata()}
	raw, err := c.exchange(ctx, c.base, "POST", "/ilink/bot/sendmessage", body, true, true, 15*time.Second)
	if err != nil {
		return err
	}
	return sendAcknowledgement(raw)
}

// StartQR always uses the official fixed bootstrap endpoint. No token is sent.
func (c *Client) StartQR(ctx context.Context) (QR, error) {
	var out QR
	u, _ := url.Parse(DefaultBaseURL)
	body := struct {
		Tokens []string `json:"local_token_list"`
	}{[]string{}}
	e := c.call(ctx, u, "POST", "/ilink/bot/get_bot_qrcode?bot_type=3", body, false, false, 15*time.Second, &out)
	if e == nil && (out.Code == "" || out.DisplayContent == "") {
		e = ErrProtocol
	}
	return out, e
}

// QRStatus performs one fixed-host status poll; it never follows redirect_host.
func (c *Client) QRStatus(ctx context.Context, code, verificationCode string) (QRStatus, error) {
	return c.QRStatusAt(ctx, DefaultBaseURL, code, verificationCode)
}

// QRStatusAt permits an explicit IDC continuation only after the same TLS host
// and public-IP validation as all production connections. It sends no bearer.
func (c *Client) QRStatusAt(ctx context.Context, baseURL, code, verificationCode string) (QRStatus, error) {
	var out QRStatus
	u, e := validateBase(baseURL)
	if e != nil {
		return out, e
	}
	if code == "" || len(code) > maxOpaqueBytes || len(verificationCode) > 64 {
		return out, ErrInvalidInput
	}
	query := url.Values{"qrcode": []string{code}}
	if verificationCode != "" {
		query.Set("verify_code", verificationCode)
	}
	e = c.call(ctx, u, "GET", "/ilink/bot/get_qrcode_status?"+query.Encode(), nil, false, false, 40*time.Second, &out)
	if e == nil {
		switch out.Status {
		case "wait", "scaned", "expired", "need_verifycode", "verify_code_blocked", "scaned_but_redirect", "binded_redirect":
		case "confirmed":
			if out.BotID == "" || out.BotToken == "" {
				e = ErrProtocol
			}
			if out.BaseURL != "" {
				if _, err := validateBase(out.BaseURL); err != nil {
					e = ErrEndpoint
				}
			}
		default:
			e = ErrProtocol
		}
	}
	return out, e
}
