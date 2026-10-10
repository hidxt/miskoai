package service

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/storage"
)

// normalizeAttachments is not wired into the operational service.
func normalizeAttachments(raw []byte, scope storage.Scope, receipt time.Time) ([]storage.InboxEntry, string, error) {
	if len(raw) > 2<<20 || !utf8.Valid(raw) {
		return nil, "", weixin.ErrProtocol
	}
	var entries []storage.InboxEntry
	cursor := ""
	count := 0
	auth, badStatus := false, false
	sawMedia := false
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
			if len(v) > 6*16384+2 || json.Unmarshal(v, &cursor) != nil || len(cursor) > 16384 || !validText(cursor) {
				return weixin.ErrProtocol
			}
		case strings.EqualFold(k, "msgs"):
			entries = nil
			return rawArray(v, &count, func(m json.RawMessage) error {
				e, err := attachmentMessage(m, scope, receipt, &sawMedia)
				if err != nil {
					return err
				}
				entries = append(entries, e...)
				return nil
			})
		}
		return nil
	})
	if !sawMedia {
		// Preserve the complete existing text-only envelope behavior, including
		// duplicate outer fields, decoder limits and advisory timestamp handling.
		return normalize(raw, scope, receipt)
	}
	if auth {
		return nil, "", weixin.ErrAuthExpired
	}
	if err != nil {
		return nil, "", err
	}
	if badStatus {
		return nil, "", weixin.ErrService
	}
	// Match the existing normalizer's non-nil empty result.
	if entries == nil {
		entries = []storage.InboxEntry{}
	}
	return entries, cursor, nil
}

// consumedObject checks original key spellings before maps or typed decoding can
// collapse duplicate, case-folded or escaped aliases. Unknown fields remain inert
// bounded raw values; they are never decoded into media metadata.
func consumedObject(raw []byte, names []string, valueLimit int) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return nil, weixin.ErrProtocol
	}
	fields := make(map[string]json.RawMessage, len(names))
	for d.More() {
		before := d.InputOffset()
		token, err := d.Token()
		if err != nil {
			return nil, weixin.ErrProtocol
		}
		name, ok := token.(string)
		if !ok {
			return nil, weixin.ErrProtocol
		}
		spelling := bytes.TrimSpace(raw[before:d.InputOffset()])
		spelling = bytes.TrimSpace(bytes.TrimPrefix(spelling, []byte(",")))
		var known string
		for _, candidate := range names {
			if strings.EqualFold(name, candidate) {
				known = candidate
				break
			}
		}
		if known != "" {
			canonical, _ := json.Marshal(known)
			if !bytes.Equal(spelling, canonical) {
				return nil, weixin.ErrProtocol
			}
			if _, exists := fields[known]; exists {
				return nil, weixin.ErrProtocol
			}
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || len(value) > valueLimit || known == "" && len(value) > 16384 {
			return nil, weixin.ErrProtocol
		}
		if known != "" {
			fields[known] = value
		}
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') {
		return nil, weixin.ErrProtocol
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, weixin.ErrProtocol
	}
	return fields, nil
}

func attachmentString(fields map[string]json.RawMessage, name string) (string, error) {
	raw, exists := fields[name]
	if !exists {
		return "", nil
	}
	var s string
	if len(raw) > 16384 || len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return "", weixin.ErrProtocol
	}
	return s, nil
}

func attachmentMessage(raw json.RawMessage, scope storage.Scope, receipt time.Time, sawMedia *bool) ([]storage.InboxEntry, error) {
	// Original array/object counts are checked even for ignored senders. No typed
	// []Item or nested key decoding is performed in this authorization preflight.
	count := 0
	err := rawObject(raw, func(k string, v json.RawMessage) error {
		for _, name := range []string{"from_user_id", "to_user_id", "group_id"} {
			if strings.EqualFold(k, name) && len(v) > 6*16384+2 {
				return weixin.ErrProtocol
			}
		}
		if strings.EqualFold(k, "item_list") {
			return rawArray(v, &count, func(item json.RawMessage) error {
				return rawObject(item, func(string, json.RawMessage) error { return nil })
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var head struct {
		From  string `json:"from_user_id"`
		To    string `json:"to_user_id"`
		Group string `json:"group_id"`
		Type  int    `json:"message_type"`
		State int    `json:"message_state"`
	}
	if json.Unmarshal(raw, &head) != nil || len(head.From) > 16384 || len(head.To) > 16384 || len(head.Group) > 16384 {
		return nil, weixin.ErrProtocol
	}
	if head.From != scope.User || head.Group != "" || head.Type != 1 || (head.State != 0 && head.State != 2) || (head.To != "" && head.To != scope.Account) {
		return nil, nil
	}
	var items []json.RawMessage
	count = 0
	nontext := 0
	// Type inspection does not consume media objects or keys. Text-only messages
	// delegate to the unchanged normalizer, preserving its existing byte behavior.
	// Visit every original matching list before strict head admission: a later
	// duplicate list must not erase evidence that this is a media message.
	err = rawObject(raw, func(k string, v json.RawMessage) error {
		if !strings.EqualFold(k, "item_list") {
			return nil
		}
		items = items[:0]
		return rawArray(v, &count, func(item json.RawMessage) error {
			itemHasMedia := false
			typ := 0
			err := rawObject(item, func(k string, v json.RawMessage) error {
				if strings.EqualFold(k, "type") {
					if json.Unmarshal(v, &typ) != nil {
						return weixin.ErrProtocol
					}
					if typ != 1 {
						itemHasMedia = true
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			if typ != 1 || itemHasMedia {
				nontext++
				*sawMedia = true
			}
			items = append(items, item)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	if nontext == 0 {
		frame := append([]byte(`{"msgs":[`), raw...)
		frame = append(frame, []byte(`]}`)...)
		e, _, err := normalize(frame, scope, receipt)
		return e, err
	}
	fields, err := consumedObject(raw, []string{"message_id", "from_user_id", "to_user_id", "group_id", "message_type", "message_state", "context_token", "create_time_ms", "item_list", "client_id", "session_id"}, 2<<20)
	if err != nil {
		return nil, err
	}
	// Bound identity fields at their original raw boundary before typed decoding.
	for _, name := range []string{"message_id", "from_user_id", "to_user_id", "group_id", "context_token", "client_id", "session_id"} {
		if len(fields[name]) > 6*16384+2 {
			return nil, weixin.ErrProtocol
		}
	}
	cleaned := make([]json.RawMessage, 0, len(items))
	var attachment *storage.Attachment
	for _, item := range items {
		f, err := consumedObject(item, []string{"type", "msg_id", "text_item", "image_item", "file_item"}, 6*16384+1024)
		if err != nil {
			return nil, err
		}
		var typ int
		if json.Unmarshal(f["type"], &typ) != nil {
			return nil, weixin.ErrProtocol
		}
		for _, name := range []string{"image_item", "file_item"} {
			if len(f[name]) > 16384 {
				return nil, weixin.ErrProtocol
			}
		}
		if typ == 1 {
			_, err := consumedObject(f["text_item"], []string{"text"}, 6*16384+2)
			if err != nil {
				return nil, err
			}
			// The existing decoder validates text type/length; this step only protects
			// original consumed aliases before the sanitized object is materialized.
		} else if nontext != 1 || (typ != 2 && typ != 4) {
			attachment = &storage.Attachment{Kind: "unsupported"}
		} else {
			attachment, err = attachmentMetadata(f, typ)
			if err != nil {
				return nil, err
			}
		}
		kept := map[string]json.RawMessage{"type": f["type"]}
		if v, ok := f["msg_id"]; ok {
			if len(v) > 6*16384+2 {
				return nil, weixin.ErrProtocol
			}
			kept["msg_id"] = v
		}
		if typ == 1 {
			kept["text_item"] = f["text_item"]
		}
		b, err := json.Marshal(kept)
		if err != nil {
			return nil, weixin.ErrProtocol
		}
		cleaned = append(cleaned, b)
	}
	if attachment == nil || attachment.Validate() != nil {
		return nil, weixin.ErrProtocol
	}
	fields["item_list"], err = json.Marshal(cleaned)
	if err != nil {
		return nil, weixin.ErrProtocol
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil, weixin.ErrProtocol
	}
	b, err = timestampFallback(b)
	if err != nil {
		return nil, err
	}
	frame := append([]byte(`{"msgs":[`), b...)
	frame = append(frame, []byte(`]}`)...)
	updates, err := weixin.DecodeUpdates(frame)
	if err != nil {
		return nil, weixin.ErrProtocol
	}
	m := updates.Messages[0]
	id := m.MessageID.String()
	if id != "" {
		if _, err := strconv.ParseUint(id, 10, 64); err != nil {
			return nil, weixin.ErrProtocol
		}
		for _, r := range id {
			if r < '0' || r > '9' {
				return nil, weixin.ErrProtocol
			}
		}
	} else {
		for _, item := range m.Items {
			if item.MessageID != "" {
				id = item.MessageID.String()
				break
			}
		}
	}
	var texts []string
	for _, item := range m.Items {
		if item.Type == 1 {
			if item.Text == nil {
				return nil, weixin.ErrProtocol
			}
			texts = append(texts, item.Text.Text)
		}
	}
	text, fits := joinText(texts)
	if !fits || !validText(text) || !validField(id, 512) || !validField(m.ContextToken, 16384) {
		return nil, weixin.ErrProtocol
	}
	received := receipt
	if m.CreateTimeMS > 0 {
		candidate := time.UnixMilli(m.CreateTimeMS).UTC()
		if candidate.Year() >= 1970 && candidate.Year() <= 9999 {
			received = candidate
		}
	}
	return []storage.InboxEntry{{Scope: scope, MessageID: id, Text: text, ContextToken: m.ContextToken, ReceivedAt: received, Attachment: attachment}}, nil
}

func attachmentMetadata(item map[string]json.RawMessage, typ int) (*storage.Attachment, error) {
	kind, field := "image", "image_item"
	names := []string{"media", "aeskey"}
	if typ == 4 {
		kind, field = "file", "file_item"
		names = []string{"media", "file_name", "md5", "len"}
	}
	payload, err := consumedObject(item[field], names, 16384)
	if err != nil {
		return nil, err
	}
	a := &storage.Attachment{Kind: kind}
	if typ == 2 {
		a.ImageHex, err = attachmentString(payload, "aeskey")
		if err != nil {
			return nil, err
		}
	}
	mediaNames := []string{"encrypt_query_param", "full_url", "encrypt_type"}
	if a.ImageHex == "" {
		mediaNames = append(mediaNames, "aes_key")
	}
	md, err := consumedObject(payload["media"], mediaNames, 16384)
	if err != nil {
		return nil, err
	}
	if v, ok := md["encrypt_type"]; ok && !bytes.Equal(bytes.TrimSpace(v), []byte("1")) {
		return nil, weixin.ErrProtocol
	}
	for _, dest := range []struct {
		name string
		dst  *string
	}{{"encrypt_query_param", &a.Query}, {"full_url", &a.FullURL}, {"aes_key", &a.MediaKey}} {
		*dest.dst, err = attachmentString(md, dest.name)
		if err != nil {
			return nil, err
		}
	}
	if typ == 4 {
		for _, dest := range []struct {
			name string
			dst  *string
		}{{"file_name", &a.FileName}, {"md5", &a.MD5}, {"len", &a.Length}} {
			*dest.dst, err = attachmentString(payload, dest.name)
			if err != nil {
				return nil, err
			}
		}
	}
	// The preferred key is exclusive: unused fallback bytes are neither decoded
	// nor persisted. ParseKey supplies the existing strict wire compatibility.
	if _, err := media.ParseKey(a.ImageHex, a.MediaKey); err != nil {
		return nil, weixin.ErrProtocol
	}
	return a, nil
}
