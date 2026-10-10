package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxAttachmentBytes = 16 << 10

// Attachment is private encrypted protocol metadata, never genuine USER text.
// URL validation here is structural only; it does not authorize a network fetch.
type Attachment struct {
	Kind     string `json:"kind"`
	ImageHex string `json:"image_hex"`
	MediaKey string `json:"media_key"`
	Query    string `json:"query"`
	FullURL  string `json:"full_url"`
	FileName string `json:"file_name"`
	MD5      string `json:"md5"`
	Length   string `json:"length"`
}

func attachmentText(s string, max int) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func attachmentHex(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range []byte(s) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
func (a *Attachment) Validate() error {
	if a == nil {
		return ErrInvalid
	}
	if !attachmentText(a.Query, 4096) || !attachmentText(a.FullURL, 16384) || !attachmentText(a.FileName, 512) {
		return ErrInvalid
	}
	if a.Kind == "unsupported" {
		if a.ImageHex != "" || a.MediaKey != "" || a.Query != "" || a.FullURL != "" || a.FileName != "" || a.MD5 != "" || a.Length != "" {
			return ErrInvalid
		}
	} else {
		if a.Kind != "image" && a.Kind != "file" {
			return ErrInvalid
		}
		if strings.TrimSpace(a.FullURL) != a.FullURL || a.Query == "" && a.FullURL == "" {
			return ErrInvalid
		}
		if a.Kind == "image" && (a.FileName != "" || a.MD5 != "" || a.Length != "") {
			return ErrInvalid
		}
		if a.ImageHex != "" {
			if a.Kind != "image" || !attachmentHex(a.ImageHex) || a.MediaKey != "" {
				return ErrInvalid
			}
		} else {
			if len(a.MediaKey) != 24 && len(a.MediaKey) != 44 {
				return ErrInvalid
			}
			key, e := base64.StdEncoding.Strict().DecodeString(a.MediaKey)
			if e != nil || base64.StdEncoding.EncodeToString(key) != a.MediaKey || !(len(key) == 16 || len(key) == 32 && attachmentHex(string(key))) {
				return ErrInvalid
			}
		}
		if a.MD5 != "" && !attachmentHex(a.MD5) {
			return ErrInvalid
		}
		if a.Length != "" {
			if len(a.Length) > 7 {
				return ErrInvalid
			}
			n, e := strconv.ParseUint(a.Length, 10, 32)
			if e != nil || n < 1 || n > 4194304 || strconv.FormatUint(n, 10) != a.Length {
				return ErrInvalid
			}
		}
	}
	b, e := json.Marshal(a)
	if e != nil {
		return ErrInvalid
	}
	if len(b) > maxAttachmentBytes {
		return ErrCapacity
	}
	return nil
}
func encodeAttachment(a *Attachment) ([]byte, error) {
	if a == nil {
		return nil, nil
	}
	if e := a.Validate(); e != nil {
		return nil, e
	}
	b, e := json.Marshal(a)
	if e != nil {
		return nil, ErrInvalid
	}
	return b, nil
}
func decodeAttachment(b []byte) (*Attachment, error) {
	if len(b) > maxAttachmentBytes {
		return nil, ErrCapacity
	}
	var a Attachment
	if json.Unmarshal(b, &a) != nil {
		return nil, ErrInvalid
	}
	canonical, e := encodeAttachment(&a)
	if e != nil {
		return nil, e
	}
	// Equality admits exactly known string fields, canonical JSON, no duplicate,
	// escaped alias, null, omitted field, unknown field or trailing value.
	if !bytes.Equal(b, canonical) {
		return nil, ErrInvalid
	}
	return &a, nil
}
func receiveQueueBudget(ctx context.Context, q rowQuery) (int, int, error) {
	var v, count, size int
	if e := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); e != nil {
		return 0, 0, storageError(ctx, e)
	}
	query := `SELECT count(*),coalesce(sum(length(CAST(text AS BLOB))+length(CAST(context_token AS BLOB))),0) FROM inbox`
	if v == 4 {
		query = `SELECT (SELECT count(*) FROM inbox),(SELECT coalesce(sum(length(CAST(text AS BLOB))+length(CAST(context_token AS BLOB))),0) FROM inbox)+(SELECT coalesce(sum(length(body)),0) FROM inbox_attachments)`
	}
	if e := q.QueryRowContext(ctx, query).Scan(&count, &size); e != nil {
		return 0, 0, storageError(ctx, e)
	}
	if count > 1024 || size > 8<<20 {
		return 0, 0, ErrCapacity
	}
	return count, size, nil
}
func validateAttachments(ctx context.Context, q rowQuery) error {
	var v int
	if e := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); e != nil {
		return storageError(ctx, e)
	}
	if v != 4 {
		return nil
	}
	if e := validateRowShape(ctx, q, "inbox_attachments", []boundedColumn{{"body", maxAttachmentBytes, "blob"}}, `typeof(sequence)<>'integer' OR sequence<=0 OR NOT EXISTS(SELECT 1 FROM inbox WHERE inbox.sequence=inbox_attachments.sequence)`); e != nil {
		return e
	}
	if e := noUnsafe(ctx, q, `SELECT (SELECT count(*) FROM inbox_attachments)>(SELECT count(*) FROM inbox)`); e != nil {
		return e
	}
	rows, e := q.QueryContext(ctx, `SELECT body FROM inbox_attachments`)
	if e != nil {
		return storageError(ctx, e)
	}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return storageError(ctx, e)
		}
		if _, e = decodeAttachment(b); e != nil {
			rows.Close()
			return e
		}
	}
	return closeValidatedRows(ctx, rows)
}
