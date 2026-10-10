package netx

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Detect loss of transport security bounds when cloud/CDN constructors share it.
func TestSharedTransportPreservesCloudContract(t *testing.T) {
	for _, timeout := range []time.Duration{-1, 0, 2*time.Minute + 1} {
		if tr, err := NewPublicTransport(timeout); err == nil || tr != nil {
			t.Fatalf("invalid timeout admitted: %s", timeout)
		}
	}
	for _, timeout := range []time.Duration{time.Nanosecond, 30 * time.Second, 2 * time.Minute} {
		tr, err := NewPublicTransport(timeout)
		if err != nil || tr == nil {
			t.Fatalf("valid timeout refused: %s: %v", timeout, err)
		}
		t.Cleanup(tr.CloseIdleConnections)
		assertTransportPolicy(t, tr, timeout)
	}
	c, err := New("https://api.deepseek.com/v1", []string{"api.deepseek.com"}, 30*time.Second, 1024)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := c.http.Transport.(*http.Transport)
	if !ok {
		t.Fatal("cloud constructor omitted public transport")
	}
	t.Cleanup(tr.CloseIdleConnections)
	assertTransportPolicy(t, tr, 30*time.Second)
	if c.http.Timeout != 30*time.Second || c.http.Jar != nil || c.max != 1024 {
		t.Fatal("cloud request lifetime, cookie or response bounds changed")
	}
	if err := c.http.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("cloud redirect policy changed: %v", err)
	}
	for _, tc := range []struct {
		base string
		max  int64
	}{
		{"http://api.deepseek.com", 1024},
		{"https://api.deepseek.com:444", 1024},
		{"https://evil.example", 1024},
		{"https://u:p@api.deepseek.com", 1024},
		{"https://api.deepseek.com?token=synthetic", 1024},
		{"https://api.deepseek.com", 0},
		{"https://api.deepseek.com", (8 << 20) + 1},
	} {
		if client, err := New(tc.base, []string{"api.deepseek.com"}, 30*time.Second, tc.max); err == nil || client != nil {
			t.Fatal("cloud endpoint or response bounds bypassed")
		}
	}
}

func assertTransportPolicy(t *testing.T, tr *http.Transport, timeout time.Duration) {
	t.Helper()
	if tr.Proxy != nil || tr.DialContext == nil || tr.DialTLSContext != nil || !tr.DisableCompression {
		t.Fatal("proxy, unguarded TLS dial or compression enabled")
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 || tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("minimum TLS or certificate verification policy missing")
	}
	if tr.TLSHandshakeTimeout != 10*time.Second || tr.ResponseHeaderTimeout != timeout || tr.IdleConnTimeout != 60*time.Second || tr.MaxResponseHeaderBytes != 32<<10 {
		t.Fatal("transport lifetime/header bounds changed")
	}
	if tr.MaxIdleConns != 4 || tr.MaxIdleConnsPerHost != 2 || tr.MaxConnsPerHost != 2 {
		t.Fatal("connection resource bounds changed")
	}
}

// Catch a refactor that moves the shared client admission to each request.
func TestSharedTransportCloudAdmissionRemainsTwo(t *testing.T) {
	c, err := New("https://api.deepseek.com", []string{"api.deepseek.com"}, time.Second, 1024)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	entered := make(chan struct{}, 3)
	var calls atomic.Int32
	c.SetTransport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
	}))
	done := make(chan error, 2)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for i := 0; i < 2; i++ {
		go func() {
			var out any
			done <- c.JSON(ctx, "GET", "/models", "", nil, &out)
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("two requests did not enter transport")
		}
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer waitCancel()
	var out any
	if err := c.JSON(waitCtx, "GET", "/models", "", nil, &out); !errors.Is(err, context.DeadlineExceeded) {
		cancel()
		t.Fatalf("third request bypassed actual admission: %v", err)
	}
	if calls.Load() != 2 {
		cancel()
		t.Fatal("waiting request reached transport")
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

// Catch checking only the first DNS answer or dialing before all are validated.
func TestPinnedDialRejectsMixedAnswers(t *testing.T) {
	for _, answers := range [][]netip.Addr{
		{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("1.1.1.1")},
		{netip.MustParseAddr("2606:4700:4700::1111"), netip.MustParseAddr("::ffff:192.168.1.1")},
		{netip.MustParseAddr("8.8.8.8"), {}},
	} {
		lookups, dials := 0, 0
		conn, err := dialPublicAddress(context.Background(), "tcp", "fixture.example:443",
			func(context.Context, string, string) ([]netip.Addr, error) { lookups++; return answers, nil },
			func(context.Context, string, string) (net.Conn, error) {
				dials++
				return nil, errors.New("synthetic dial")
			})
		if conn != nil || err == nil || err.Error() != "private remote address rejected" || lookups != 1 || dials != 0 {
			t.Fatalf("mixed DNS set was not refused before dial: conn=%v err=%v lookups=%d dials=%d", conn, err, lookups, dials)
		}
	}
}

// Catch a hostname re-resolution or failure to pin the validated IPv4/IPv6 port.
func TestPinnedDialUsesValidatedAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, tc := range []struct {
		address string
		ip      string
		want    string
	}{
		{"fixture.example:443", "8.8.8.8", "8.8.8.8:443"},
		{"fixture.example:443", "2606:4700:4700::1111", "[2606:4700:4700::1111]:443"},
	} {
		lookups, dials := 0, 0
		a, b := net.Pipe()
		conn, err := dialPublicAddress(ctx, "tcp", tc.address,
			func(gotCtx context.Context, network, host string) ([]netip.Addr, error) {
				lookups++
				if gotCtx != ctx || network != "ip" || host != "fixture.example" {
					t.Fatal("lookup did not retain context or source hostname")
				}
				return []netip.Addr{netip.MustParseAddr(tc.ip)}, nil
			},
			func(gotCtx context.Context, network, address string) (net.Conn, error) {
				dials++
				if gotCtx != ctx || network != "tcp" || address != tc.want {
					t.Fatalf("unguarded address dialed: %s", address)
				}
				return a, nil
			})
		_ = a.Close()
		_ = b.Close()
		if err != nil || conn != a || lookups != 1 || dials != 1 {
			t.Fatalf("pinned direct connection failed: %v lookups=%d dials=%d", err, lookups, dials)
		}
	}
}

func TestPinnedDialSafeFailuresAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		address string
		answers []netip.Addr
		lookup  error
		want    string
		dials   int
	}{
		{"address", "secret-canary", nil, nil, "invalid remote address", 0},
		{"dns", "fixture.example:443", nil, errors.New("secret-canary"), "remote DNS failure", 0},
		{"empty", "fixture.example:443", nil, nil, "remote DNS empty", 0},
		{"connect", "fixture.example:443", []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("1.1.1.1")}, nil, "remote connection failure", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dials := 0
			conn, err := dialPublicAddress(context.Background(), "tcp", tc.address,
				func(context.Context, string, string) ([]netip.Addr, error) { return tc.answers, tc.lookup },
				func(context.Context, string, string) (net.Conn, error) {
					dials++
					return nil, errors.New("secret-canary")
				})
			if conn != nil || err == nil || err.Error() != tc.want || strings.Contains(err.Error(), "secret-canary") || dials != tc.dials {
				t.Fatalf("unsafe failure: %v dials=%d", err, dials)
			}
		})
	}
	lookups, dials := 0, 0
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	conn, err := dialPublicAddress(context.Background(), "tcp", "fixture.example:443",
		func(context.Context, string, string) ([]netip.Addr, error) {
			lookups++
			return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("1.1.1.1")}, nil
		},
		func(_ context.Context, _ string, address string) (net.Conn, error) {
			dials++
			if dials == 1 && address == "8.8.8.8:443" {
				return nil, errors.New("secret-canary")
			}
			if dials == 2 && address == "1.1.1.1:443" {
				return a, nil
			}
			t.Fatal("fallback used an unvalidated endpoint")
			return nil, errors.New("synthetic unexpected")
		})
	if err != nil || conn != a || lookups != 1 || dials != 2 {
		t.Fatalf("validated fallback failed: %v lookups=%d dials=%d", err, lookups, dials)
	}
}
