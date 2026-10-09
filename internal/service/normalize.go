package service

import (
	"bytes"
	"encoding/json"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func validField(v string, max int) bool {
	if strings.TrimSpace(v) == "" || len(v) > max || !utf8.ValidString(v) {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func validText(v string) bool {
	if !utf8.ValidString(v) {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

// rawObject visits every key in order; EqualFold matches encoding/json, including
// duplicate and Unicode-folded keys. Raw input has already been persisted.
func rawObject(raw []byte, visit func(string, json.RawMessage) error) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return weixin.ErrProtocol
	}
	var visitError error
	for d.More() {
		k, e := d.Token()
		if e != nil {
			return weixin.ErrProtocol
		}
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return weixin.ErrProtocol
		}
		if e = visit(k.(string), v); e != nil {
			// Continue visiting bounded original keys so a later explicit auth
			// expiry cannot disappear behind an invalid message shape.
			visitError = e
		}
	}
	if t, e = d.Token(); e != nil || t != json.Delim('}') {
		return weixin.ErrProtocol
	}
	if _, e = d.Token(); e != io.EOF {
		return weixin.ErrProtocol
	}
	return visitError
}
func rawArray(raw []byte, count *int, visit func(json.RawMessage) error) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	t, e := d.Token()
	if e != nil || t != json.Delim('[') {
		return weixin.ErrProtocol
	}
	for d.More() {
		*count++
		if *count > 256 {
			return weixin.ErrProtocol
		}
		var v json.RawMessage
		if d.Decode(&v) != nil {
			return weixin.ErrProtocol
		}
		if e = visit(v); e != nil {
			return e
		}
	}
	if t, e = d.Token(); e != nil || t != json.Delim(']') {
		return weixin.ErrProtocol
	}
	if _, e = d.Token(); e != io.EOF {
		return weixin.ErrProtocol
	}
	return nil
}
func normalize(raw []byte, scope storage.Scope, receipt time.Time) ([]storage.InboxEntry, string, error) {
	if len(raw) > 2<<20 || !utf8.Valid(raw) {
		return nil, "", weixin.ErrProtocol
	}
	var selected []json.RawMessage
	cursor := ""
	count := 0
	auth := false
	badStatus := false
	err := rawObject(raw, func(k string, v json.RawMessage) error {
		switch {
		case strings.EqualFold(k, "ret") || strings.EqualFold(k, "errcode"):
			var code *int
			if json.Unmarshal(v, &code) != nil {
				return weixin.ErrProtocol
			}
			if code != nil {
				if *code == -14 {
					auth = true
				} else if *code != 0 {
					badStatus = true
				}
			}
		case strings.EqualFold(k, "get_updates_buf"):
			var c string
			if json.Unmarshal(v, &c) != nil || len(c) > 16384 || !validText(c) {
				return weixin.ErrProtocol
			}
			cursor = c
		case strings.EqualFold(k, "msgs"):
			selected = nil
			return rawArray(v, &count, func(m json.RawMessage) error {
				// Original array/item shape and count are bounded even for ignored senders.
				n := 0
				if e := rawObject(m, func(key string, value json.RawMessage) error {
					if strings.EqualFold(key, "item_list") {
						return rawArray(value, &n, func(item json.RawMessage) error {
							return rawObject(item, func(string, json.RawMessage) error { return nil })
						})
					}
					return nil
				}); e != nil {
					return e
				}
				var head struct {
					From  string `json:"from_user_id"`
					To    string `json:"to_user_id"`
					Group string `json:"group_id"`
					Type  int    `json:"message_type"`
					State int    `json:"message_state"`
				}
				if json.Unmarshal(m, &head) != nil {
					return weixin.ErrProtocol
				}
				if len(head.From) > 16384 || len(head.To) > 16384 || len(head.Group) > 16384 {
					return weixin.ErrProtocol
				}
				if head.From != scope.User || head.Group != "" || head.Type != 1 || (head.State != 0 && head.State != 2) || (head.To != "" && head.To != scope.Account) {
					return nil
				}
				m, err := timestampFallback(m)
				if err != nil {
					return err
				}
				selected = append(selected, m)
				return nil
			})
		}
		return nil
	})
	if auth {
		return nil, "", weixin.ErrAuthExpired
	}
	if err != nil {
		return nil, "", err
	}
	if badStatus {
		return nil, "", weixin.ErrService
	}
	// The reviewed decoder enforces bounded typed fields/items for allowed entries.
	filtered, err := json.Marshal(struct {
		Messages []json.RawMessage `json:"msgs"`
		Cursor   string            `json:"get_updates_buf"`
	}{selected, cursor})
	if err != nil {
		return nil, "", weixin.ErrProtocol
	}
	u, err := weixin.DecodeUpdates(filtered)
	if err != nil {
		return nil, "", err
	}
	entries := make([]storage.InboxEntry, 0, len(u.Messages))
	for _, m := range u.Messages {
		id := m.MessageID.String()
		if id != "" {
			if _, err := strconv.ParseUint(id, 10, 64); err != nil {
				return nil, "", weixin.ErrProtocol
			}
			for _, r := range id {
				if r < '0' || r > '9' {
					return nil, "", weixin.ErrProtocol
				}
			}
		} else {
			for _, i := range m.Items {
				if i.MessageID != "" {
					id = i.MessageID.String()
					break
				}
			}
		}
		texts := []string{}
		unsupported := false
		for _, i := range m.Items {
			if i.Type != 1 {
				unsupported = true
				continue
			}
			if i.Text == nil {
				return nil, "", weixin.ErrProtocol
			}
			texts = append(texts, i.Text.Text)
		}
		// Text-only scope: media envelopes carry no claimed read/seen side effect.
		if unsupported {
			continue
		}
		text, fits := joinText(texts)
		if !fits || !validField(id, 512) || !validField(m.ContextToken, 16384) || strings.TrimSpace(text) == "" || !validText(text) {
			return nil, "", weixin.ErrProtocol
		}
		received := receipt
		if m.CreateTimeMS > 0 {
			candidate := time.UnixMilli(m.CreateTimeMS).UTC()
			if candidate.Year() >= 1970 && candidate.Year() <= 9999 {
				received = candidate
			}
		}
		entries = append(entries, storage.InboxEntry{Scope: scope, MessageID: id, Text: text, ContextToken: m.ContextToken, ReceivedAt: received})
	}
	return entries, cursor, nil
}

func joinText(texts []string) (string, bool) {
	remaining := 16384
	for i, text := range texts {
		if i != 0 {
			if remaining == 0 {
				return "", false
			}
			remaining-- // Joining inserts one newline between adjacent items.
		}
		// Subtract from a nonnegative budget, never add attacker-controlled
		// lengths. Reject before allocating the aggregate joined string.
		if len(text) > remaining {
			return "", false
		}
		remaining -= len(text)
	}
	return strings.Join(texts, "\n"), true
}

// Timestamp metadata is advisory. Invalid numbers/types use receipt time rather
// than turning an otherwise authorized, valid text envelope into lost work.
func timestampFallback(raw json.RawMessage) (json.RawMessage, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	first := true
	err := rawObject(raw, func(k string, value json.RawMessage) error {
		if strings.EqualFold(k, "create_time_ms") {
			var ms int64
			if json.Unmarshal(value, &ms) != nil || ms <= 0 || time.UnixMilli(ms).Year() > 9999 {
				value = json.RawMessage("0")
			}
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		key, _ := json.Marshal(k)
		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
		return nil
	})
	b.WriteByte('}')
	return b.Bytes(), err
}
