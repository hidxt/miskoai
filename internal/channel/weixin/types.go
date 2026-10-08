// Package weixin implements the pinned Tencent iLink client wire protocol.
// Adapted from Tencent/openclaw-weixin (MIT); see docs/research/tencent-LICENSE.txt.
package weixin

import (
	"bytes"
	"encoding/json"
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
