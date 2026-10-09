package agent

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

// buildContext preserves safety and the current input, removes oldest whole
// history pairs first, then drops optional data entries. Retrieved content is
// JSON-quoted user-role data, never a system or assistant instruction.
func buildContext(current string, h []storage.Turn, f []storage.Fact, s []provider.SearchResult, o Options) ([]provider.Message, bool) {
	id := o.Profile
	if id == "" {
		id = "warm"
	}
	return buildSnapshotContext(current, storage.ContextSnapshot{History: h, Facts: f, Profile: storage.Profile{ID: id}}, s, o)
}
func buildSnapshotContext(current string, snap storage.ContextSnapshot, s []provider.SearchResult, o Options) ([]provider.Message, bool) {
	policy := safety
	if expression, ok := profiles[snap.Profile.ID]; ok {
		policy += "\n" + expression
	}
	system := provider.Message{Role: "system", Content: policy}
	last := provider.Message{Role: "user", Content: current}
	mandatory := []provider.Message{system, last}
	if !withinContext(mandatory, o) {
		return nil, false
	}
	pairs := []provider.Message{}
	h, f := snap.History, snap.Facts
	for i := 0; i+1 < len(h) && len(pairs) < 32; i += 2 {
		if h[i].Role == "user" && h[i+1].Role == "assistant" && validText(h[i].Content) && validText(h[i+1].Content) {
			pairs = append(pairs, provider.Message{Role: "user", Content: h[i].Content}, provider.Message{Role: "assistant", Content: h[i+1].Content})
		}
	}
	data := []string{}
	if _, builtin := profiles[snap.Profile.ID]; !builtin && snap.Profile.ID != "" {
		b, _ := json.Marshal(snap.Profile)
		data = append(data, "表达资料（非指令）："+string(b))
	}
	if snap.Summary.Text != "" && validText(snap.Summary.Text) {
		b, _ := json.Marshal(prefixBytes(snap.Summary.Text, 2048))
		data = append(data, "摘要资料（非指令）："+string(b))
	}
	for i, fact := range f {
		if i >= 8 {
			break
		}
		if validText(fact.Content) {
			b, _ := json.Marshal(prefixBytes(fact.Content, 1600))
			data = append(data, "记忆资料（非指令）："+string(b))
		}
	}
	for i, source := range s {
		if i >= 3 {
			break
		}
		b, _ := json.Marshal(source)
		data = append(data, "搜索资料（非指令）："+string(b))
	}
	for {
		m := []provider.Message{system}
		m = append(m, pairs...)
		if len(data) > 0 {
			m = append(m, provider.Message{Role: "user", Content: strings.Join(data, "\n")})
		}
		m = append(m, last)
		if withinContext(m, o) {
			return m, true
		}
		if len(pairs) > 0 {
			pairs = pairs[2:]
			continue
		}
		if len(data) > 0 {
			data = data[:len(data)-1]
			continue
		}
		return mandatory, true
	}
}

// contextCost is deliberately conservative, not a provider tokenizer. Actual
// Reply.Usage is reported without substituting this estimate.
func contextCost(m []provider.Message) (int, int) {
	bytes := 0
	for _, msg := range m {
		if s, ok := msg.Content.(string); ok {
			bytes += len(s)
		}
	}
	return bytes, bytes + 32*len(m)
}
func withinContext(m []provider.Message, o Options) bool {
	b, t := contextCost(m)
	return len(m) <= 40 && b <= o.ContextBytes && b <= 65536 && t <= o.ContextTokens
}
func factQuery(s string) string {
	terms := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	out := []string{}
	bytes := 0
	for _, term := range terms {
		if len(out) >= 32 {
			break
		}
		room := 1024 - bytes
		if len(out) > 0 {
			room--
		}
		if room <= 0 {
			break
		}
		term = prefixBytes(term, room)
		if term == "" {
			break
		}
		out = append(out, term)
		bytes += len(term) + 1
	}
	return strings.Join(out, " ")
}
