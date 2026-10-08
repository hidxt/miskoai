// Package weixin implements the pinned Tencent iLink client wire protocol.
// Adapted from Tencent/openclaw-weixin (MIT); see docs/research/tencent-LICENSE.txt.
package weixin

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Message identifiers remain lossless whether the service encodes them as
// decimal JSON numbers or strings. Unknown content is retained for later stages.
type Message struct {
	MessageID    json.Number `json:"message_id,omitempty"`
	FromUserID   string      `json:"from_user_id,omitempty"`
	ToUserID     string      `json:"to_user_id,omitempty"`
	ClientID     string      `json:"client_id,omitempty"`
	ContextToken string      `json:"context_token,omitempty"`
	SessionID    string      `json:"session_id,omitempty"`
	GroupID      string      `json:"group_id,omitempty"`
	MessageType  int         `json:"message_type,omitempty"`
	MessageState int         `json:"message_state,omitempty"`
	CreateTimeMS int64       `json:"create_time_ms,omitempty"`
	Items        []Item      `json:"item_list,omitempty"`
}
type Item struct {
	Type      int             `json:"type"`
	MessageID ItemMessageID   `json:"msg_id,omitempty"`
	Text      *TextItem       `json:"text_item,omitempty"`
	Image     json.RawMessage `json:"image_item,omitempty"`
	Voice     json.RawMessage `json:"voice_item,omitempty"`
	File      json.RawMessage `json:"file_item,omitempty"`
	Video     json.RawMessage `json:"video_item,omitempty"`
	Reference json.RawMessage `json:"ref_msg,omitempty"`
}

// ItemMessageID matches the pinned MessageItem.msg_id string contract. The
// upstream parser also losslessly converts integer JSON IDs into strings;
// existing opaque strings are retained unchanged.
type ItemMessageID string

func (id ItemMessageID) String() string { return string(id) }

func (id *ItemMessageID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*id = ""
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*id = ItemMessageID(value)
		return nil
	}
	digits := data
	if len(digits) > 0 && digits[0] == '-' {
		digits = digits[1:]
	}
	if len(digits) == 0 {
		return ErrProtocol
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return ErrProtocol
		}
	}
	*id = ItemMessageID(string(data))
	return nil
}

type TextItem struct {
	Text string `json:"text"`
}
type Updates struct {
	Messages          []Message `json:"msgs"`
	Cursor            string    `json:"get_updates_buf"`
	LongPollTimeoutMS int64     `json:"longpolling_timeout_ms"`
}

// DecodeUpdates decodes a poll response without network access.
func DecodeUpdates(raw []byte) (Updates, error) {
	if len(raw) > maxResponseBytes {
		return Updates{}, ErrResponseLimit
	}
	if err := responseStatus(raw); err != nil {
		return Updates{}, err
	}
	if err := preflightObject(raw, "updates"); err != nil {
		return Updates{}, ErrProtocol
	}
	var out Updates
	if err := json.Unmarshal(raw, &out); err != nil {
		return Updates{}, ErrProtocol
	}
	return out, nil
}

// responseStatus never returns remote error text or materializes message arrays.
func responseStatus(raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if !utf8.Valid(raw) || len(trimmed) == 0 || trimmed[0] != '{' {
		return ErrProtocol
	}
	var status struct {
		Ret  *int `json:"ret"`
		Code *int `json:"errcode"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return ErrProtocol
	}
	// Either documented expiry signal must stop account activity, even when
	// the other status field reports a generic service failure.
	for _, code := range []*int{status.Ret, status.Code} {
		if code != nil && *code == -14 {
			return ErrAuthExpired
		}
	}
	for _, code := range []*int{status.Ret, status.Code} {
		if code != nil && *code != 0 {
			return ErrService
		}
	}
	return nil
}

// Preflight uses raw fields/elements, never an unbounded []Message or []Item.
// Every duplicate key is visited; case folding mirrors encoding/json's matching.
func preflightObject(raw []byte, kind string) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return ErrProtocol
	}
	arrayCount := 0
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return ErrProtocol
		}
		name, ok := key.(string)
		if !ok {
			return ErrProtocol
		}
		// encoding/json also folds Unicode variants of ASCII letters (for
		// example long-s). Plain ToLower would leave an allocation bypass.
		for _, known := range []string{"msgs", "item_list", "text_item", "get_updates_buf", "message_id", "from_user_id", "to_user_id", "client_id", "context_token", "session_id", "group_id", "msg_id", "text", "image_item", "voice_item", "file_item", "video_item", "ref_msg"} {
			if strings.EqualFold(name, known) {
				name = known
				break
			}
		}
		var field json.RawMessage
		if err := dec.Decode(&field); err != nil {
			return ErrProtocol
		}
		switch {
		case kind == "updates" && name == "msgs":
			if err := preflightArray(field, "message", &arrayCount); err != nil {
				return err
			}
		case kind == "message" && name == "item_list":
			if err := preflightArray(field, "item", &arrayCount); err != nil {
				return err
			}
		case kind == "item" && name == "text_item":
			if !bytes.Equal(field, []byte("null")) {
				if err := preflightObject(field, "text"); err != nil {
					return err
				}
			}
		case (kind == "updates" && name == "get_updates_buf") ||
			(kind == "message" && (name == "message_id" || name == "from_user_id" || name == "to_user_id" || name == "client_id" || name == "context_token" || name == "session_id" || name == "group_id")) ||
			(kind == "item" && name == "msg_id") || (kind == "text" && name == "text"):
			// IDs also allow lossless JSON numbers. Typed decoding checks their syntax.
			if len(field) > 0 && field[0] == '"' {
				var value string
				if json.Unmarshal(field, &value) != nil || len(value) > maxOpaqueBytes {
					return ErrProtocol
				}
			} else if len(field) > maxOpaqueBytes {
				return ErrProtocol
			}
		case kind == "item" && (name == "image_item" || name == "voice_item" || name == "file_item" || name == "video_item" || name == "ref_msg"):
			if len(field) > maxOpaqueBytes {
				return ErrProtocol
			}
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') {
		return ErrProtocol
	}
	return nil
}

func preflightArray(raw []byte, elementKind string, count *int) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	start, err := dec.Token()
	if err != nil || start != json.Delim('[') {
		return ErrProtocol
	}
	for dec.More() {
		*count++
		if *count > 256 {
			return ErrProtocol
		}
		var element json.RawMessage
		if err := dec.Decode(&element); err != nil {
			return ErrProtocol
		}
		if err := preflightObject(element, elementKind); err != nil {
			return err
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim(']') {
		return ErrProtocol
	}
	return nil
}

type QR struct {
	Code           string `json:"qrcode"`
	DisplayContent string `json:"qrcode_img_content"`
}

// QRStatus contains sensitive authentication material. Never log or expose this
// object wholesale; only display the status and QR content to the login user.
type QRStatus struct {
	Status       string `json:"status"`
	BotToken     string `json:"bot_token,omitempty"`
	BotID        string `json:"ilink_bot_id,omitempty"`
	UserID       string `json:"ilink_user_id,omitempty"`
	BaseURL      string `json:"baseurl,omitempty"`
	RedirectHost string `json:"redirect_host,omitempty"`
}
