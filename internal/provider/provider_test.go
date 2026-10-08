package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(code int, s string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(s))}
}

func TestChatStreamAndSearch(t *testing.T) {
	d, err := NewDeepSeek("https://api.deepseek.com", "synthetic", "configured-model")
	if err != nil {
		t.Fatal(err)
	}
	d.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "configured-model" || r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Fatal("wrong request contract")
		}
		if body["stream"] == true {
			return fixture(200, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":3}}\n\ndata: [DONE]\n\n"), nil
		}
		return fixture(200, `{"choices":[{"message":{"content":"Hi"},"finish_reason":"stop"}],"usage":{"total_tokens":3}}`), nil
	}))
	out, err := d.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, 64)
	if err != nil || out.Text != "Hi" || out.Usage.TotalTokens != 3 {
		t.Fatalf("chat: %#v %v", out, err)
	}
	var text strings.Builder
	usage, err := d.Stream(context.Background(), []Message{{Role: "user", Content: "hello"}}, 64, func(s string) error { _, e := text.WriteString(s); return e })
	if err != nil || text.String() != "Hi" || usage.TotalTokens != 3 {
		t.Fatalf("stream: %q %#v %v", text.String(), usage, err)
	}
	s, _ := NewSearch("synthetic")
	s.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://ollama.com/api/web_search" {
			t.Fatal(r.URL)
		}
		return fixture(200, `{"results":[{"title":"fixture","url":"https://example.com/source","content":"summary"}]}`), nil
	}))
	results, err := s.Search(context.Background(), "latest info", 2)
	if err != nil || len(results) != 1 || results[0].URL != "https://example.com/source" {
		t.Fatalf("search: %#v %v", results, err)
	}
}

func TestProviderErrorsAndIncompleteStream(t *testing.T) {
	d, _ := NewDeepSeek("https://api.deepseek.com", "synthetic", "model")
	d.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		return fixture(401, `{"error":"sensitive fixture"}`), nil
	}))
	_, err := d.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, 64)
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("unsafe remote error", err)
	}
	d.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		return fixture(200, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"), nil
	}))
	if _, err = d.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}}, 64, func(string) error { return nil }); err == nil {
		t.Fatal("truncated stream accepted")
	}
}
