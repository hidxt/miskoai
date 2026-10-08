package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/netx"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

func TestExplicitStatusRetryBudget(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		status, failures, calls int
		success                 bool
	}{
		{"rate limit then success", 429, 1, 2, true}, {"rate limit exhausted", 429, 99, 3, false}, {"authentication", 401, 99, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := NewDeepSeek("https://api.deepseek.com", "synthetic", "model")
			calls := 0
			d.transport.SetTransport(transportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if calls <= tc.failures {
					return fixture(tc.status, `{}`), nil
				}
				return fixture(200, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
			}))
			_, err := d.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, 16)
			if calls != tc.calls || (err == nil) != tc.success {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if !tc.success {
				var status *netx.HTTPError
				if !errors.As(err, &status) || status.Status != tc.status {
					t.Fatalf("wrong error: %v", err)
				}
			}
		})
	}
}

func TestSearchDropsUnsafeSources(t *testing.T) {
	s, _ := NewSearch("synthetic")
	urls := []string{"https://example.com/source", "https://localhost/a", "https://127.0.0.1/a", "https://10.0.0.1/a", "https://[::1]/a", "http://example.com/a", "javascript:alert(1)", "https://user:pass@example.com/a", "https://service.local/a"}
	results := make([]SearchResult, len(urls))
	for i, u := range urls {
		results[i] = SearchResult{Title: "source", URL: u, Content: "snippet"}
	}
	payload, _ := json.Marshal(map[string]any{"results": results})
	s.transport.SetTransport(transportFunc(func(*http.Request) (*http.Response, error) { return fixture(200, string(payload)), nil }))
	got, err := s.Search(context.Background(), "query", 10)
	if err != nil || len(got) != 1 || got[0].URL != urls[0] {
		t.Fatalf("results=%v err=%v", got, err)
	}
}

func TestInlineImagePayload(t *testing.T) {
	var buf bytes.Buffer
	err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = png.Decode(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatal("invalid synthetic PNG", err)
	}
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	imageURL := "data:image/png;base64," + encoded
	d, _ := NewDeepSeek("https://api.deepseek.com", "synthetic", "vision-model")
	called := false
	d.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string        `json:"role"`
				Content []ContentPart `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "vision-model" || len(body.Messages) != 1 || body.Messages[0].Role != "user" {
			t.Fatalf("bad envelope: %+v", body)
		}
		parts := body.Messages[0].Content
		if len(parts) != 2 || parts[0].Type != "text" || parts[0].Text != "Describe" || parts[1].Type != "image_url" || parts[1].ImageURL == nil || parts[1].ImageURL.URL != imageURL {
			t.Fatalf("bad parts: %+v", parts)
		}
		return fixture(200, `{"choices":[{"message":{"content":"pixel"},"finish_reason":"stop"}]}`), nil
	}))
	_, err = d.Chat(context.Background(), []Message{{Role: "user", Content: []ContentPart{{Type: "text", Text: "Describe"}, {Type: "image_url", ImageURL: &ImageURL{URL: imageURL}}}}}, 16)
	if err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
}

func TestContextByteBoundary(t *testing.T) {
	d, _ := NewDeepSeek("https://api.deepseek.com", "synthetic", "model")
	for _, n := range []int{65536, 65537} {
		_, err := d.request([]Message{{Role: "system", Content: strings.Repeat("a", 32768)}, {Role: "user", Content: strings.Repeat("b", n-32768)}}, 16, false)
		if (err == nil) != (n == 65536) {
			t.Fatalf("bytes=%d err=%v", n, err)
		}
	}
	// UTF-8 bytes, rather than rune count, consume the same aggregate budget.
	if _, err := d.request([]Message{{Role: "user", Content: strings.Repeat("界", 21846)}}, 16, false); err == nil {
		t.Fatal("multibyte oversized context accepted")
	}
}

func TestStreamConsumerCancellation(t *testing.T) {
	d, _ := NewDeepSeek("https://api.deepseek.com", "synthetic", "model")
	calls := 0
	d.transport.SetTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return fixture(200, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"second\"}}]}\n\ndata: [DONE]\n\n"), nil
	}))
	consumed := 0
	_, err := d.Stream(context.Background(), []Message{{Role: "user", Content: "hello"}}, 16, func(string) error { consumed++; return context.Canceled })
	if !errors.Is(err, context.Canceled) || consumed != 1 || calls != 1 {
		t.Fatalf("consumed=%d calls=%d err=%v", consumed, calls, err)
	}
}
