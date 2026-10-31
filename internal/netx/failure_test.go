package netx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFailureDiagnosticClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"success", nil, ""}, {"unknown", errors.New("private-canary"), "unknown"},
		{"wrapped-timeout", fmt.Errorf("private-canary: %w", context.DeadlineExceeded), "timeout"},
		{"wrapped-cancel", fmt.Errorf("private-canary: %w", context.Canceled), "canceled"},
		{"limit", fmt.Errorf("private-canary: %w", ErrLimit), "payload_limit"},
		{"400", &HTTPError{400}, "request_invalid"}, {"422", &HTTPError{422}, "request_invalid"},
		{"401", &HTTPError{401}, "authentication"}, {"403", &HTTPError{403}, "authentication"},
		{"402", &HTTPError{402}, "balance"}, {"429", &HTTPError{429}, "rate_limit"},
		{"500", &HTTPError{500}, "unavailable"}, {"502", &HTTPError{502}, "unavailable"},
		{"503", &HTTPError{503}, "unavailable"}, {"504", &HTTPError{504}, "unavailable"},
		{"redirect", &HTTPError{302}, "unknown"}, {"other-status", &HTTPError{418}, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FailureCode(tc.err); got != tc.want {
				t.Fatalf("code=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestFailureDiagnosticSafeWrapper(t *testing.T) {
	secret := errors.New("private-canary")
	for _, code := range []string{"request_invalid", "authentication", "balance", "rate_limit", "unavailable", "timeout", "canceled", "transport", "payload_limit", "response_invalid", "response_empty", "response_finish", "unknown", "private-canary", ""} {
		t.Run(code, func(t *testing.T) {
			err := WithFailure(code, secret)
			want := code
			if code == "" || code == "private-canary" {
				want = "unknown"
			}
			if FailureCode(err) != want || strings.Contains(err.Error(), "private-canary") || !errors.Is(err, secret) {
				t.Fatal("unsafe wrapper or lost identity")
			}
		})
	}
	if WithFailure("transport", nil) != nil || NormalizeFailureCode("") != "" || NormalizeFailureCode("private-canary") != "unknown" {
		t.Fatal("nil/reset/normalization")
	}
}

type failureDiagnosticReader struct{}

type failureDiagnosticContextReader struct{ cause error }

type failureDiagnosticCancelReader struct{ cancel context.CancelFunc }

func (r failureDiagnosticCancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, errors.New("private-canceled-read-canary")
}

func TestFailureDiagnosticCanceledBodyGenericRead(t *testing.T) {
	c, _ := New("https://api.deepseek.com", []string{"api.deepseek.com"}, time.Second, 1024)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.SetTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(failureDiagnosticCancelReader{cancel})}, nil
	}))
	var out any
	err := c.JSON(ctx, "POST", "/chat/completions", "synthetic", nil, &out)
	if FailureCode(err) != "canceled" || !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private-") {
		t.Fatal("body request cancellation identity lost")
	}
}

func (r failureDiagnosticContextReader) Read([]byte) (int, error) {
	return 0, fmt.Errorf("private-body-canary: %w", r.cause)
}

func TestFailureDiagnosticWrappedContextOrigins(t *testing.T) {
	for _, origin := range []string{"headers", "body"} {
		for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
			t.Run(origin+"/"+cause.Error(), func(t *testing.T) {
				c, _ := New("https://api.deepseek.com", []string{"api.deepseek.com"}, time.Second, 1024)
				calls := 0
				c.SetTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					if origin == "headers" {
						return nil, fmt.Errorf("private-header-canary: %w", cause)
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(failureDiagnosticContextReader{cause})}, nil
				}))
				var out any
				err := c.JSON(context.Background(), "POST", "/chat/completions", "synthetic", nil, &out)
				want := "canceled"
				if errors.Is(cause, context.DeadlineExceeded) {
					want = "timeout"
				}
				if FailureCode(err) != want || !errors.Is(err, cause) || calls != 1 || strings.Contains(err.Error(), "private-") {
					t.Fatalf("lost identity/privacy/budget: code=%s calls=%d", FailureCode(err), calls)
				}
			})
		}
	}
}

func (failureDiagnosticReader) Read([]byte) (int, error) { return 0, errors.New("private-read-canary") }
func TestFailureDiagnosticTransportOrigins(t *testing.T) {
	for _, mode := range []string{"transport", "json", "read", "limit", "path", "encoding"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := New("https://api.deepseek.com", []string{"api.deepseek.com"}, time.Second, 16)
			calls := 0
			c.SetTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if mode == "transport" {
					return nil, errors.New("private-canary")
				}
				var body io.Reader = strings.NewReader("{")
				if mode == "read" {
					body = failureDiagnosticReader{}
				}
				if mode == "limit" {
					body = strings.NewReader(strings.Repeat("x", 17))
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(body)}, nil
			}))
			path := "/chat/completions"
			var input any = map[string]string{"safe": "data"}
			if mode == "path" {
				path = "bad"
			}
			if mode == "encoding" {
				input = make(chan int)
			}
			var out any
			err := c.JSON(context.Background(), "POST", path, "synthetic", input, &out)
			want := map[string]string{"transport": "transport", "json": "response_invalid", "read": "transport", "limit": "payload_limit", "path": "request_invalid", "encoding": "request_invalid"}[mode]
			expectedCalls := 1
			if mode == "path" || mode == "encoding" {
				expectedCalls = 0
			}
			if FailureCode(err) != want || calls != expectedCalls || strings.Contains(err.Error(), "private-") {
				t.Fatalf("wrong classification/calls code=%s calls=%d", FailureCode(err), calls)
			}
		})
	}
}
