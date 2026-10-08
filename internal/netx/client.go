// Package netx owns bounded cloud HTTP calls. Remote response bodies never enter errors.
package netx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const MaxRequest = 8 << 20

var ErrLimit = errors.New("remote payload limit exceeded")

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string {
	code := "remote_failure"
	switch e.Status {
	case 400, 422:
		code = "invalid_request"
	case 401, 403:
		code = "authentication"
	case 402:
		code = "balance"
	case 429:
		code = "rate_limit"
	case 500, 502, 503, 504:
		code = "temporarily_unavailable"
	}
	return fmt.Sprintf("%s (HTTP %d)", code, e.Status)
}

type Client struct {
	http    *http.Client
	base    *url.URL
	timeout time.Duration
	max     int64
	sem     chan struct{}
}

func New(base string, allowed []string, timeout time.Duration, max int64) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" || u.Port() != "" && u.Port() != "443" || u.Path != "" && u.Path != "/" && u.Path != "/v1" {
		return nil, errors.New("invalid cloud endpoint")
	}
	approved := false
	for _, host := range allowed {
		if strings.EqualFold(u.Hostname(), host) {
			approved = true
		}
	}
	if !approved {
		return nil, errors.New("unapproved cloud endpoint")
	}
	if timeout <= 0 || timeout > 2*time.Minute || max <= 0 || max > 8<<20 {
		return nil, errors.New("invalid transport bounds")
	}
	transport := &http.Transport{DialContext: safeDial, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: timeout, MaxIdleConns: 4, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2, IdleConnTimeout: 60 * time.Second, DisableCompression: true, MaxResponseHeaderBytes: 32 << 10}
	return &Client{http: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, base: u, timeout: timeout, max: max, sem: make(chan struct{}, 2)}, nil
}

// SetTransport supplies an application-owned transport for deterministic offline tests.
// Configure it before concurrent use. It cannot be reached from model or HTTP input.
func (c *Client) SetTransport(t http.RoundTripper) { c.http.Transport = t }

func safeDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("invalid remote address")
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, errors.New("remote DNS failure")
	}
	if len(addresses) == 0 {
		return nil, errors.New("remote DNS empty")
	}
	for _, ip := range addresses {
		if !PublicIP(ip) {
			return nil, errors.New("private remote address rejected")
		}
	}
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	for _, ip := range addresses {
		conn, e := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return conn, nil
		}
	}
	return nil, errors.New("remote connection failure")
}

func PublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, block := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16"} {
		if netip.MustParsePrefix(block).Contains(ip) {
			return false
		}
	}
	return true
}

func (c *Client) request(ctx context.Context, method, path, key string, input any, consume func(io.Reader) error) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#") || strings.Contains(path, "..") {
		return errors.New("invalid cloud path")
	}
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return errors.New("invalid request encoding")
		}
		if len(body) > MaxRequest {
			return ErrLimit
		}
	}
	u := *c.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid request")
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("remote transport failure")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{resp.StatusCode}
	}
	if resp.ContentLength > c.max {
		return ErrLimit
	}
	limited := &io.LimitedReader{R: resp.Body, N: c.max + 1}
	err = consume(limited)
	if limited.N <= 0 {
		return ErrLimit
	}
	return err
}

func (c *Client) JSON(ctx context.Context, method, path, key string, input, output any) error {
	return c.request(ctx, method, path, key, input, func(r io.Reader) error {
		data, err := io.ReadAll(r)
		if err != nil {
			return errors.New("remote response read failure")
		}
		if int64(len(data)) > c.max {
			return ErrLimit
		}
		if err = json.Unmarshal(data, output); err != nil {
			return errors.New("invalid remote JSON")
		}
		return nil
	})
}

// SSE requires an explicit DONE marker; a truncated stream must not look successful.
func (c *Client) SSE(ctx context.Context, path, key string, input any, consume func([]byte) error) error {
	return c.request(ctx, "POST", path, key, input, func(r io.Reader) error {
		scan := bufio.NewScanner(r)
		scan.Buffer(make([]byte, 4096), 64<<10)
		for scan.Scan() {
			line := scan.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				return nil
			}
			if payload == "" {
				continue
			}
			if err := consume([]byte(payload)); err != nil {
				return err
			}
		}
		if scan.Err() != nil {
			return errors.New("remote stream read failure")
		}
		return errors.New("remote stream ended before DONE")
	})
}
