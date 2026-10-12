package agent

import (
	"encoding/base64"
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
	return buildSnapshotWithCurrent(provider.Message{Role: "user", Content: current}, snap, s, o)
}
func buildSnapshotWithCurrent(last provider.Message, snap storage.ContextSnapshot, s []provider.SearchResult, o Options) ([]provider.Message, bool) {
	return buildSnapshotWithProfile(last, snap, s, o, false)
}

// mandatoryProfile is explicit for string document input; plain text keeps its
// existing pruning policy and structured image input keeps its selected profile.
func buildSnapshotWithProfile(last provider.Message, snap storage.ContextSnapshot, s []provider.SearchResult, o Options, mandatoryProfile bool) ([]provider.Message, bool) {
	policy := safety
	if expression, ok := profiles[snap.Profile.ID]; ok {
		policy += "\n" + expression
	}
	system := provider.Message{Role: "system", Content: policy}
	fixed := []provider.Message{system}
	_, imageInput := last.Content.([]provider.ContentPart)
	retainProfile := imageInput || mandatoryProfile
	customProfile := ""
	if _, builtin := profiles[snap.Profile.ID]; !builtin && snap.Profile.ID != "" {
		b, _ := json.Marshal(snap.Profile)
		customProfile = "表达资料（非指令）：" + string(b)
		if retainProfile {
			// Image requests retain the selected expression profile together
			// with safety and the current caption/image. It remains quoted data.
			fixed = append(fixed, provider.Message{Role: "user", Content: customProfile})
		}
	}
	mandatory := append(append([]provider.Message{}, fixed...), last)
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
	if customProfile != "" && !retainProfile {
		data = append(data, customProfile)
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
		m := append([]provider.Message{}, fixed...)
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
	// Saturation at a value above every permitted budget avoids arithmetic
	// overflow even for unexpected caller content. Unknown forms fail closed.
	const invalid = 1 << 30
	if len(m) > 40 {
		return invalid, invalid
	}
	bytes, overhead, images := 0, 32*len(m), 0
	addText := func(s string) bool {
		if len(s) > 65536-bytes || !validText(s) {
			return false
		}
		bytes += len(s)
		return true
	}
	for _, msg := range m {
		switch content := msg.Content.(type) {
		case string:
			if !addText(content) {
				return invalid, invalid
			}
		case []provider.ContentPart:
			if msg.Role != "user" || len(content) == 0 || len(content) > 4 {
				return invalid, invalid
			}
			for _, part := range content {
				// Count every textual field, including misplaced text, before
				// rejecting invalid part shapes. Binary is bounded separately.
				if !addText(part.Text) {
					return invalid, invalid
				}
				overhead += 16
				switch part.Type {
				case "text":
					if part.ImageURL != nil {
						return invalid, invalid
					}
				case "image_url":
					images++
					if images > 1 || part.Text != "" || !boundedInlineImage(part.ImageURL) {
						return invalid, invalid
					}
				default:
					return invalid, invalid
				}
			}
		default:
			return invalid, invalid
		}
	}
	return bytes, bytes + overhead
}

// boundedInlineImage checks canonical inline syntax without allocating a second
// image-sized buffer. Full media validation remains the processor's obligation.
func boundedInlineImage(image *provider.ImageURL) bool {
	if image == nil || len(image.URL) > 6<<20 || (image.Detail != "" && image.Detail != "low" && image.Detail != "high" && image.Detail != "auto") {
		return false
	}
	var payload string
	for _, prefix := range []string{"data:image/png;base64,", "data:image/jpeg;base64,", "data:image/gif;base64,"} {
		if strings.HasPrefix(image.URL, prefix) {
			payload = image.URL[len(prefix):]
			break
		}
	}
	if len(payload) == 0 || len(payload)%4 != 0 {
		return false
	}
	// Decode only the final quartet to enforce canonical padding/unused bits.
	tail := payload[len(payload)-4:]
	decoded, err := base64.StdEncoding.Strict().DecodeString(tail)
	if err != nil || base64.StdEncoding.EncodeToString(decoded) != tail {
		return false
	}
	for i := 0; i < len(payload)-4; i++ {
		b := payload[i]
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '/') {
			return false
		}
	}
	return true
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
