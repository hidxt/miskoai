// Wire construction adapted from Tencent/openclaw-weixin (MIT), commit
// 24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c. See retained Tencent license.
package weixin

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ImageUploadRequest describes actual validated plaintext. The caller supplies
// random file/key values and its actual MD5; MD5 is wire metadata, not authentication.
type ImageUploadRequest struct {
	ToUserID, FileKey, RawMD5, AESKeyHex string
	RawBytes                             int64
}

// ImageUploadLocation is untrusted metadata, never an authorization to fetch a
// URL. Pass it through media.CDN.Upload's existing URL/IP policy before egress.
type ImageUploadLocation struct{ UploadParam, UploadFullURL string }

// GetImageUploadURL performs one authenticated metadata request, without reading
// files, encrypting, uploading, retries or thumbnail work.
func (c *Client) GetImageUploadURL(ctx context.Context, request ImageUploadRequest) (ImageUploadLocation, error) {
	if !imageWireOpaque(request.ToUserID, true) || !imageWireHex(request.FileKey) || !imageWireHex(request.RawMD5) || !imageWireHex(request.AESKeyHex) || request.RawBytes < 1 || request.RawBytes > 4<<20 {
		return ImageUploadLocation{}, ErrInvalidInput
	}
	body := struct {
		To          string   `json:"to_user_id"`
		FileKey     string   `json:"filekey"`
		Type        int      `json:"media_type"`
		RawBytes    int64    `json:"rawsize"`
		MD5         string   `json:"rawfilemd5"`
		CipherBytes int64    `json:"filesize"`
		NoThumb     bool     `json:"no_need_thumb"`
		AES         string   `json:"aeskey"`
		Base        baseInfo `json:"base_info"`
	}{request.ToUserID, request.FileKey, 1, request.RawBytes, request.RawMD5, 16 * (request.RawBytes/16 + 1), true, request.AESKeyHex, metadata()}
	raw, err := c.exchange(ctx, c.base, "POST", "/ilink/bot/getuploadurl", body, true, false, 15*time.Second)
	if err != nil {
		return ImageUploadLocation{}, err
	}
	return imageUploadLocation(raw)
}

func imageWireHex(value string) bool {
	if len(value) != 32 {
		return false
	}
	for i := range value {
		if !(value[i] >= '0' && value[i] <= '9' || value[i] >= 'a' && value[i] <= 'f') {
			return false
		}
	}
	return true
}

func imageWireOpaque(value string, required bool) bool {
	if len(value) > maxOpaqueBytes || !utf8.ValidString(value) || required && strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Scan recognized fields before typed consumption. Unknown values stay slices
// of the capped body, and case-fold aliases cannot override earlier metadata.
func imageUploadLocation(raw []byte) (ImageUploadLocation, error) {
	if len(raw) > maxResponseBytes || !utf8.Valid(raw) || !json.Valid(raw) {
		return ImageUploadLocation{}, ErrProtocol
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return ImageUploadLocation{}, ErrProtocol
	}
	var out ImageUploadLocation
	var seen [5]bool
	invalid, expired, rejected := false, false, false
	known := [5]string{"ret", "errcode", "upload_param", "upload_full_url", "thumb_upload_param"}
	for pos := ackWhitespace(raw, 1); raw[pos] != '}'; {
		end := ackStringEnd(raw, pos)
		var name string
		_ = json.Unmarshal(raw[pos:end], &name)
		pos = ackWhitespace(raw, end)
		pos = ackWhitespace(raw, pos+1)
		end = ackValueEnd(raw, pos)
		value := bytes.TrimSpace(raw[pos:end])
		for i, key := range known {
			if !strings.EqualFold(name, key) {
				continue
			}
			invalid = invalid || seen[i]
			seen[i] = true
			if i < 2 {
				code, err := strconv.ParseInt(string(value), 10, 64)
				if err != nil {
					invalid = true
				} else {
					expired = expired || code == -14
					rejected = rejected || code != 0
				}
			} else {
				var decoded string
				if len(value) > maxOpaqueBytes*6+2 || len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &decoded) != nil || !imageWireOpaque(decoded, false) {
					invalid = true
				} else {
					if decoded != "" && strings.TrimSpace(decoded) == "" {
						invalid = true
					}
					switch i {
					case 2:
						out.UploadParam = decoded
					case 3:
						out.UploadFullURL = decoded
					}
				}
			}
			break
		}
		pos = ackWhitespace(raw, end)
		if raw[pos] == ',' {
			pos = ackWhitespace(raw, pos+1)
		}
	}
	if expired {
		return ImageUploadLocation{}, ErrAuthExpired
	}
	if invalid {
		return ImageUploadLocation{}, ErrProtocol
	}
	if rejected {
		return ImageUploadLocation{}, ErrService
	}
	if out.UploadParam == "" && out.UploadFullURL == "" {
		return ImageUploadLocation{}, ErrProtocol
	}
	return out, nil
}
