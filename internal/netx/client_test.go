package netx

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRejectEndpointAndRedirect(t *testing.T) {
	for _, u := range []string{"http://api.deepseek.com", "https://api.deepseek.com.evil.example", "https://127.0.0.1", "https://u:p@api.deepseek.com", "https://api.deepseek.com/path?token=secret"} {
		if _, err := New(u, []string{"api.deepseek.com"}, time.Second, 1024); err == nil {
			t.Fatalf("accepted %s", u)
		}
	}
	c, _ := New("https://api.deepseek.com", []string{"api.deepseek.com"}, time.Second, 1024)
	calls := 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://evil.example"}}, Body: io.NopCloser(strings.NewReader("secret remote error"))}, nil
	})
	var out any
	err := c.JSON(context.Background(), "POST", "/chat/completions", "synthetic", map[string]string{"a": "b"}, &out)
	if err == nil || strings.Contains(err.Error(), "secret") || calls != 1 {
		t.Fatalf("unsafe redirect handling: %v calls=%d", err, calls)
	}
}

func TestBoundedResponseAndCancellation(t *testing.T) {
	c, _ := New("https://api.deepseek.com", []string{"api.deepseek.com"}, time.Second, 16)
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 100)))}, nil
	})
	var out any
	if err := c.JSON(context.Background(), "GET", "/models", "", nil, &out); err == nil {
		t.Fatal("oversized response accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.JSON(ctx, "GET", "/models", "", nil, &out); err == nil {
		t.Fatal("cancelled call accepted")
	}
}
