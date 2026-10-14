// Wire construction adapted from Tencent/openclaw-weixin (MIT), commit
// 24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c. See retained Tencent license.
package weixin

import (
	"context"
	"encoding/base64"
	"time"
)

// ImageSend describes already uploaded ciphertext. Key text is lowercase ASCII
// hex32, which the pinned source base64-encodes as text, not raw AES bytes.
type ImageSend struct {
	EncryptQueryParam, AESKeyHex string
	CiphertextBytes              int64
}

// SendImage sends exactly one image item once. Conversation context and stable
// clientID are caller-authorized values. Uncertain sends require reconciliation.
func (c *Client) SendImage(ctx context.Context, to, contextToken, clientID string, image ImageSend) error {
	if !imageWireOpaque(to, true) || !imageWireOpaque(contextToken, true) || !imageWireOpaque(clientID, true) || !imageWireOpaque(image.EncryptQueryParam, true) || !imageWireHex(image.AESKeyHex) || image.CiphertextBytes < 16 || image.CiphertextBytes > (4<<20)+16 || image.CiphertextBytes%16 != 0 {
		return ErrInvalidInput
	}
	type media struct {
		Param string `json:"encrypt_query_param"`
		AES   string `json:"aes_key"`
		Type  int    `json:"encrypt_type"`
	}
	type imageItem struct {
		Media media `json:"media"`
		Size  int64 `json:"mid_size"`
	}
	type item struct {
		Type  int       `json:"type"`
		Image imageItem `json:"image_item"`
	}
	type message struct {
		From     string  `json:"from_user_id"`
		To       string  `json:"to_user_id"`
		Context  string  `json:"context_token"`
		ClientID string  `json:"client_id"`
		Type     int     `json:"message_type"`
		State    int     `json:"message_state"`
		Items    [1]item `json:"item_list"`
	}
	body := struct {
		Msg  message  `json:"msg"`
		Base baseInfo `json:"base_info"`
	}{message{"", to, contextToken, clientID, 2, 2, [1]item{{2, imageItem{media{image.EncryptQueryParam, base64.StdEncoding.EncodeToString([]byte(image.AESKeyHex)), 1}, image.CiphertextBytes}}}}, metadata()}
	raw, err := c.exchange(ctx, c.base, "POST", "/ilink/bot/sendmessage", body, true, true, 15*time.Second)
	if err != nil {
		return err
	}
	return sendAcknowledgement(raw)
}
