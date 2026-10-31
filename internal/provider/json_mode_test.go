package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type jsonChatter interface {
	ChatJSON(context.Context, []Message, int) (Reply, error)
}

func TestJSONModeWireAndDefaultPaths(t *testing.T) {
	for _, mode := range []string{"json", "plain", "stream", "vision"} {
		t.Run(mode, func(t *testing.T) {
			d, err := NewDeepSeek("https://api.deepseek.com", "synthetic", "deepseek-flash")
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			d.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				var b map[string]any
				if e := json.NewDecoder(r.Body).Decode(&b); e != nil {
					t.Fatal(e)
				}
				if r.URL.Path != "/chat/completions" || b["model"] != "deepseek-flash" || b["max_tokens"] != float64(64) || b["reasoning_effort"] != "none" {
					t.Fatal("request contract changed")
				}
				if _, ok := r.Context().Deadline(); !ok {
					t.Fatal("missing deadline")
				}
				if mode == "json" {
					f, ok := b["response_format"].(map[string]any)
					if !ok || len(f) != 1 || f["type"] != "json_object" || b["stream"] != false {
						t.Fatal("JSON wire format absent")
					}
				} else if _, ok := b["response_format"]; ok {
					t.Fatal("default path acquired JSON mode")
				}
				if mode == "stream" {
					return fixture(200, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"), nil
				}
				return fixture(200, `{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"}],"usage":{"total_tokens":7}}`), nil
			}))
			msgs := []Message{{Role: "user", Content: "JSON is merely user text"}}
			if mode == "vision" {
				msgs[0].Content = []ContentPart{{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,AQ==", Detail: "low"}}}
			}
			if mode == "json" {
				j, ok := any(d).(jsonChatter)
				if !ok {
					t.Fatal("missing explicit JSON capability")
				}
				r, e := j.ChatJSON(context.Background(), msgs, 64)
				if e != nil || r.Usage.TotalTokens != 7 {
					t.Fatal(r, e)
				}
			} else if mode == "stream" {
				_, err = d.Stream(context.Background(), msgs, 64, func(string) error { return nil })
			} else {
				_, err = d.Chat(context.Background(), msgs, 64)
			}
			if err != nil || calls != 1 {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestJSONModeFailureBudgets(t *testing.T) {
	for _, mode := range []string{"blank", "400", "transport", "429", "503", "cancel", "length", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			d, _ := NewDeepSeek("https://api.deepseek.com", "synthetic", "deepseek-flash")
			j, ok := any(d).(jsonChatter)
			if !ok {
				t.Fatal("missing explicit JSON capability")
			}
			calls := 0
			d.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				var b map[string]any
				if e := json.NewDecoder(r.Body).Decode(&b); e != nil {
					t.Fatal(e)
				}
				f, ok := b["response_format"].(map[string]any)
				if !ok || f["type"] != "json_object" {
					t.Fatal("retry lost JSON mode")
				}
				switch mode {
				case "400":
					return fixture(400, `{}`), nil
				case "429":
					return fixture(429, `{}`), nil
				case "503":
					return fixture(503, `{}`), nil
				case "transport":
					return nil, errors.New("synthetic transport error")
				}
				content, finish := "   ", "stop"
				if mode == "length" {
					content, finish = `{"reply":"ok"}`, "length"
				}
				if mode == "oversize" {
					content = strings.Repeat("x", (32<<10)+1)
				}
				bts, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": finish}}})
				return fixture(200, string(bts)), nil
			}))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			r, err := j.ChatJSON(ctx, []Message{{Role: "user", Content: "Return JSON"}}, 64)
			want := 1
			if mode == "429" || mode == "503" {
				want = 3
			}
			if mode == "cancel" {
				want = 0
			}
			if calls != want {
				t.Fatalf("calls=%d want=%d", calls, want)
			}
			if mode == "length" {
				if err != nil || r.FinishReason != "length" {
					t.Fatal(r, err)
				}
			} else if err == nil {
				t.Fatal("failure accepted")
			}
		})
	}
}
