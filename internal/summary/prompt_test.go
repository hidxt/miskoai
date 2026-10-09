package summary

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

func TestPromptBoundsAndVisibleEvidence(t *testing.T) {
	h := []storage.Turn{}
	for i := 0; i < 16; i++ {
		id := fmt.Sprintf("%03d-%s", i, strings.Repeat("i", 508))
		h = append(h, storage.Turn{ID: id, Role: "user", Content: strings.Repeat("\x01", 1024)}, storage.Turn{ID: id, Role: "assistant", Content: strings.Repeat("好", 1000)})
	}
	m, visible := buildPrompt(storage.Summary{Text: strings.Repeat("好", 3000)}, h)
	if len(m) > 40 || len(m) != 2 || m[0].Role != "system" || m[1].Role != "user" {
		t.Fatal(m)
	}
	size := 0
	for _, v := range m {
		size += len(v.Content.(string))
	}
	if size > 24576 {
		t.Fatal(size)
	}
	var data struct {
		Previous string        `json:"previous_summary"`
		Turns    []visibleTurn `json:"turns"`
	}
	if e := json.Unmarshal([]byte(m[1].Content.(string)), &data); e != nil {
		t.Fatal(e)
	}
	if len(data.Previous) > 2048 || !utf8.ValidString(data.Previous) || len(data.Turns)%2 != 0 || len(data.Turns) >= 32 {
		t.Fatal("whole-pair eviction")
	}
	for _, v := range data.Turns {
		if len(v.Content) > 1024 || !utf8.ValidString(v.Content) {
			t.Fatal("turn bound")
		}
	}
	if len(visible) == 0 {
		t.Fatal("no user evidence")
	}
	if data.Turns[0].ID == h[0].ID || data.Turns[len(data.Turns)-1].ID != h[len(h)-1].ID {
		t.Fatal("did not evict oldest pairs")
	}
	for i := 0; i < len(data.Turns); i += 2 {
		if data.Turns[i].Role != "user" || data.Turns[i+1].Role != "assistant" || data.Turns[i].ID != data.Turns[i+1].ID {
			t.Fatal("role/ID pairing")
		}
	}
}
func TestStrictReplies(t *testing.T) {
	visible := map[string]string{"id": "I like tea"}
	good := `{"summary":"","candidates":[{"content":"likes tea","message_id":"id","quote":"tea"}]}`
	if _, _, e := parseReply(provider.Reply{Text: good, FinishReason: "stop"}, visible); e != nil {
		t.Fatal(e)
	}
	cases := []string{`{}`, `{"summary":"x"}`, `{"summary":null,"candidates":[]}`, `{"summary":"x","candidates":null}`, `{"summary":"x","summary":"y","candidates":[]}`, `{"summary":"x","candidates":[],"extra":1}`, good + ` {}`, `{"summary":"x","candidates":[null]}`, `{"summary":"x","candidates":[{"content":"x","message_id":"id","quote":"tea","quote":"tea"}]}`, strings.Replace(good, `"tea"`, `"assistant"`, 1), strings.Replace(good, `"id"`, `"other"`, 1), strings.Replace(good, `"likes tea"`, `"\u0001"`, 1), `{"summary":"\ud800","candidates":[]}`, `{"summary":"x","candidates":[` + strings.TrimSuffix(strings.Repeat(`{"content":"x","message_id":"id","quote":"tea"},`, 9), ",") + `]}`, `{"summary":"` + strings.Repeat("x", 8193) + `","candidates":[]}`, `{"summary":"x","candidates":[]}` + strings.Repeat(" ", 16384), string([]byte{'{', '"', 0xff, '"', ':', '0', '}'})}
	for i, s := range cases {
		t.Run(fmt.Sprintf("invalid-%02d", i), func(t *testing.T) {
			if _, _, e := parseReply(provider.Reply{Text: s, FinishReason: "stop"}, visible); e == nil {
				t.Fatal("accepted", i)
			}
		})
	}
	for _, finish := range []string{"", "length", "tool_calls"} {
		if _, _, e := parseReply(provider.Reply{Text: good, FinishReason: finish}, visible); e == nil {
			t.Fatal(finish)
		}
	}
}
func TestCommandsExcludedAndAssistantCannotProve(t *testing.T) {
	commands := []string{"/remember x", "/remember", "请记住：x", "/memory clear", "/memory candidates", "/memory confirm 1", "/export-memory", "/forget 1", "/search x", "搜索：x"}
	for _, c := range commands {
		_, v := buildPrompt(storage.Summary{}, []storage.Turn{{ID: "id", Role: "user", Content: c}, {ID: "id", Role: "assistant", Content: "tea"}})
		if len(v) != 0 {
			t.Fatal("command evidence", c)
		}
	}
	_, v := buildPrompt(storage.Summary{}, []storage.Turn{{ID: "id", Role: "user", Content: strings.Repeat("a", 1024) + "tea"}, {ID: "id", Role: "assistant", Content: "tea"}})
	if _, _, e := parseReply(provider.Reply{Text: `{"summary":"x","candidates":[{"content":"x","message_id":"id","quote":"tea"}]}`, FinishReason: "stop"}, v); e == nil {
		t.Fatal("invisible/assistant evidence")
	}
}

func TestStrictBoundaryFieldsAndSurrogates(t *testing.T) {
	mk := func(content, id, quote string) string {
		b, _ := json.Marshal(map[string]any{"summary": "s", "candidates": []map[string]string{{"content": content, "message_id": id, "quote": quote}}})
		return string(b)
	}
	id := strings.Repeat("i", 512)
	quote := strings.Repeat("q", 1024)
	visible := map[string]string{id: quote}
	if _, _, e := parseReply(provider.Reply{Text: mk(strings.Repeat("c", 1024), id, quote), FinishReason: "stop"}, visible); e != nil {
		t.Fatal("valid maximum", e)
	}
	for _, s := range []string{mk(strings.Repeat("c", 1025), id, quote), mk("c", id+"i", quote), mk("c", id, quote+"q"), mk("", id, quote), mk("c", id, ""), mk("c", id, "\x7f"), `{"summary":"x","candidates":{}}`, `{"summary":"x","candidates":[{"content":4,"message_id":"id","quote":"q"}]}`, `{"summary":"x","candidates":[{"content":"c","message_id":"id"}]}`} {
		if _, _, e := parseReply(provider.Reply{Text: s, FinishReason: "stop"}, visible); e == nil {
			t.Fatal("invalid boundary accepted")
		}
	}
	for _, s := range []string{`{"summary":"\udc00","candidates":[]}`, `{"summary":"\ud800x","candidates":[]}`, `{"summary":"\ud800\u0041","candidates":[]}`} {
		if _, _, e := parseReply(provider.Reply{Text: s, FinishReason: "stop"}, visible); e == nil {
			t.Fatal("surrogate accepted")
		}
	}
	for _, s := range []string{`{"summary":"\ud83d\ude00","candidates":[]}`, `{"summary":"literal \\ud800","candidates":[]}`, "{\"summary\":\"\\r\\n\\t\",\"candidates\":[]}"} {
		if _, _, e := parseReply(provider.Reply{Text: s, FinishReason: "stop"}, visible); e != nil {
			t.Fatal("valid escape rejected", e)
		}
	}
}
