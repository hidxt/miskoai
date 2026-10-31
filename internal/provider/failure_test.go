package provider

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/netx"
	"io"
	"net/http"
	"strings"
	"testing"
)

type failureDiagnosticStatusBody struct{}

func (failureDiagnosticStatusBody) Read([]byte) (int, error) { return 0, &netx.HTTPError{Status: 503} }

func TestFailureDiagnosticOnlyReceivedHTTPStatusRetries(t *testing.T) {
	for _, origin := range []string{"headers", "body"} {
		t.Run(origin, func(t *testing.T) {
			d, _ := NewDeepSeek("https://api.deepseek.com", "synthetic", "deepseek-flash")
			calls := 0
			d.transport.SetTransport(transportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if origin == "headers" {
					return nil, &netx.HTTPError{Status: 503}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(failureDiagnosticStatusBody{})}, nil
			}))
			_, err := d.ChatJSON(context.Background(), []Message{{Role: "user", Content: "Return JSON"}}, 64)
			if calls != 1 || netx.FailureCode(err) != "transport" {
				t.Fatal("transport/body error broadened received-status retry")
			}
		})
	}
}

func TestFailureDiagnosticProviderOriginsAndBudget(t *testing.T) {
	for _, mode := range []string{"preflight", "no-key", "400", "401", "402", "403", "422", "429", "500", "503", "transport", "json", "empty", "finish", "limit", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			d, _ := NewDeepSeek("https://api.deepseek.com", "synthetic", "deepseek-flash")
			calls := 0
			d.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Fatal("payload")
				}
				if body["model"] != "deepseek-flash" || body["max_tokens"] != float64(64) || body["stream"] != false || body["reasoning_effort"] != "none" {
					t.Fatal("request contract changed")
				}
				format, ok := body["response_format"].(map[string]any)
				if !ok || format["type"] != "json_object" || len(format) != 1 {
					t.Fatal("JSON parameter changed")
				}
				switch mode {
				case "400":
					return fixture(400, "private-canary"), nil
				case "401":
					return fixture(401, "private-canary"), nil
				case "402":
					return fixture(402, "private-canary"), nil
				case "403":
					return fixture(403, "private-canary"), nil
				case "422":
					return fixture(422, "private-canary"), nil
				case "429":
					return fixture(429, "private-canary"), nil
				case "500":
					return fixture(500, "private-canary"), nil
				case "503":
					return fixture(503, "private-canary"), nil
				case "transport":
					return nil, errors.New("private-canary")
				case "json":
					return fixture(200, "private-canary"), nil
				case "empty":
					return fixture(200, `{"choices":[]}`), nil
				case "finish":
					return fixture(200, `{"choices":[{"message":{"content":"safe"},"finish_reason":"private-canary"}]}`), nil
				}
				text := strings.Repeat("x", (32<<10)+1)
				encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": text}, "finish_reason": "stop"}}})
				return fixture(200, string(encoded)), nil
			}))
			msgs := []Message{{Role: "user", Content: "Return JSON"}}
			if mode == "preflight" {
				msgs = nil
			}
			if mode == "no-key" {
				d.key = ""
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			_, err := d.ChatJSON(ctx, msgs, 64)
			want := map[string]string{"preflight": "request_invalid", "no-key": "request_invalid", "400": "request_invalid", "401": "authentication", "402": "balance", "403": "authentication", "422": "request_invalid", "429": "rate_limit", "500": "unavailable", "503": "unavailable", "transport": "transport", "json": "response_invalid", "empty": "response_empty", "finish": "response_finish", "limit": "payload_limit", "cancel": "canceled"}[mode]
			attempts := 1
			if mode == "429" || mode == "503" {
				attempts = 3
			}
			if mode == "preflight" || mode == "no-key" || mode == "cancel" {
				attempts = 0
			}
			if netx.FailureCode(err) != want || calls != attempts || strings.Contains(err.Error(), "private-canary") {
				t.Fatalf("code=%s calls=%d", netx.FailureCode(err), calls)
			}
		})
	}
}
