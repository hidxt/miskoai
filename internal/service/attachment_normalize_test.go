package service

import (
	"encoding/base64"
	"errors"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/storage"
	"reflect"
	"strings"
	"testing"
	"time"
)

var attachmentScope = storage.Scope{Account: "bot", User: "alice"}
var attachmentReceipt = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

const attachmentHex = "00112233445566778899aabbccddeeff"

func attachmentFrame(items string) []byte {
	return []byte(`{"get_updates_buf":"cursor","msgs":[{"message_id":"18446744073709551615","from_user_id":"alice","to_user_id":"bot","message_type":1,"message_state":2,"context_token":"context","item_list":[` + items + `]}]}`)
}
func attachmentImage() string {
	return `{"type":2,"image_item":{"aeskey":"` + attachmentHex + `","media":{"aes_key":"unused-invalid-key","encrypt_query_param":"query","full_url":"https://example.test/full","encrypt_type":1}}}`
}
func attachmentOnly(t *testing.T, raw []byte) storage.InboxEntry {
	t.Helper()
	entries, cursor, err := normalizeAttachments(raw, attachmentScope, attachmentReceipt)
	if err != nil || cursor != "cursor" || len(entries) != 1 {
		t.Fatalf("normalization: entries=%d cursor=%q error=%v", len(entries), cursor, err)
	}
	return entries[0]
}

// These assertions catch lost attachment identity, key precedence and lossless IDs.
func TestAuthorizedAttachmentNormalization(t *testing.T) {
	t.Run("image", func(t *testing.T) {
		entry := attachmentOnly(t, attachmentFrame(attachmentImage()))
		want := storage.Attachment{Kind: "image", ImageHex: attachmentHex, Query: "query", FullURL: "https://example.test/full"}
		if entry.Attachment == nil || *entry.Attachment != want || entry.MessageID != "18446744073709551615" || entry.Scope != attachmentScope || entry.ContextToken != "context" || !entry.ReceivedAt.Equal(attachmentReceipt) {
			t.Fatalf("wrong identity or descriptor: %#v", entry)
		}
	})
	for _, key := range []string{base64.StdEncoding.EncodeToString([]byte("0123456789abcdef")), base64.StdEncoding.EncodeToString([]byte(attachmentHex))} {
		t.Run("file"+key[:3], func(t *testing.T) {
			entry := attachmentOnly(t, attachmentFrame(`{"type":4,"file_item":{"media":{"aes_key":"`+key+`","encrypt_query_param":"q"},"file_name":"notes.txt","len":"4194304","md5":"`+attachmentHex+`"}}`))
			if entry.Attachment == nil || entry.Attachment.Kind != "file" || entry.Attachment.MediaKey != key || entry.Attachment.Length != "4194304" || entry.Attachment.FileName != "notes.txt" || entry.Attachment.MD5 != attachmentHex {
				t.Fatalf("file descriptor wrong: %#v", entry.Attachment)
			}
		})
	}
	t.Run("authorization-before-key", func(t *testing.T) {
		for _, change := range [][2]string{{`"alice"`, `"foreign"`}, {`"bot"`, `"foreign"`}, {`"message_type":1`, `"message_type":2`}, {`"message_state":2`, `"message_state":1`}, {`"context_token"`, `"group_id":"group","context_token"`}} {
			raw := strings.Replace(string(attachmentFrame(strings.Replace(attachmentImage(), attachmentHex, "invalid", 1))), change[0], change[1], 1)
			entries, cursor, err := normalizeAttachments([]byte(raw), attachmentScope, attachmentReceipt)
			if err != nil || cursor != "cursor" || len(entries) != 0 {
				t.Fatalf("unauthorized metadata was consumed: %v %d", err, len(entries))
			}
		}
	})
}

// Every consumed spelling is checked in original JSON rather than a collapsed map.
func TestAttachmentConsumedAliasesRefuse(t *testing.T) {
	base := string(attachmentFrame(attachmentImage()))
	for name, change := range map[string][2]string{
		"duplicate-message": {`"from_user_id":"alice"`, `"from_user_id":"alice","from_user_id":"alice"`},
		"case-message":      {`"from_user_id":"alice"`, `"FROM_USER_ID":"alice"`},
		"escaped-message":   {`"from_user_id":"alice"`, `"from_\u0075ser_id":"alice"`},
		"duplicate-type":    {`"type":2`, `"type":2,"type":2`},
		"case-image":        {`"image_item"`, `"IMAGE_ITEM"`},
		"escaped-key":       {`"aeskey"`, `"aes\u006bey"`},
		"duplicate-media":   {`"media":{`, `"media":{},"media":{`},
		"fold-query":        {`"encrypt_query_param"`, `"ENCRYPT_QUERY_PARAM"`},
		"duplicate-encrypt": {`"encrypt_type":1`, `"encrypt_type":1,"encrypt_type":1`},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := normalizeAttachments([]byte(strings.Replace(base, change[0], change[1], 1)), attachmentScope, attachmentReceipt)
			if !errors.Is(err, weixin.ErrProtocol) {
				t.Fatalf("ambiguous consumed field accepted: %v", err)
			}
		})
	}
}

func TestAttachmentCaptionOnlyProvenance(t *testing.T) {
	for _, caption := range []string{"", `{"type":1,"text_item":{"text":"genuine caption"}},`} {
		entry := attachmentOnly(t, attachmentFrame(caption+attachmentImage()))
		want := ""
		if caption != "" {
			want = "genuine caption"
		}
		if entry.Text != want {
			t.Fatalf("non-user provenance in caption: %q", entry.Text)
		}
	}
	raw := attachmentFrame(`{"type":1,"msg_id":"opaque-item-id","text_item":{"text":"first"}},{"type":1,"text_item":{"text":"second\nline"}}`)
	before, bc, be := normalize(raw, attachmentScope, attachmentReceipt)
	after, ac, ae := normalizeAttachments(raw, attachmentScope, attachmentReceipt)
	if !reflect.DeepEqual(before, after) || bc != ac || be != ae {
		t.Fatal("text-only behavior changed")
	}
	_, _, err := normalizeAttachments(attachmentFrame(`{"type":1,"text_item":{"text":""}}`), attachmentScope, attachmentReceipt)
	if !errors.Is(err, weixin.ErrProtocol) {
		t.Fatalf("empty text became fabricated message: %v", err)
	}
}

func TestMultipleUnsupportedItemsAreExplicit(t *testing.T) {
	for _, items := range []string{strings.Replace(attachmentImage(), attachmentHex, "invalid-preferred", 1) + "," + attachmentImage(), `{"type":3,"voice_item":{"aeskey":"invalid"}}`, attachmentImage() + `,{"type":5,"video_item":{}}`, `{"type":4,"file_item":{"media":{"aes_key":null}}},{"type":4,"file_item":{}}`} {
		entry := attachmentOnly(t, attachmentFrame(`{"type":1,"text_item":{"text":"caption"}},`+items))
		if entry.Attachment == nil || *entry.Attachment != (storage.Attachment{Kind: "unsupported"}) || entry.Text != "caption" {
			t.Fatalf("unsupported metadata/provenance wrong: %#v", entry)
		}
	}
}

func TestAttachmentMetadataAndKeysBounded(t *testing.T) {
	base := string(attachmentFrame(attachmentImage()))
	for name, raw := range map[string]string{
		"invalid-preferred": strings.Replace(base, attachmentHex, "broken", 1),
		"null-encryption":   strings.Replace(base, `"encrypt_type":1`, `"encrypt_type":null`, 1),
		"float-encryption":  strings.Replace(base, `"encrypt_type":1`, `"encrypt_type":1.0`, 1),
		"wrong-encryption":  strings.Replace(base, `"encrypt_type":1`, `"encrypt_type":2`, 1),
		"oversized-query":   strings.Replace(base, `"query"`, `"`+strings.Repeat("x", 4097)+`"`, 1),
		"oversized-nested":  strings.Replace(base, `"unused-invalid-key"`, `"`+strings.Repeat("x", 16385)+`"`, 1),
		"oversized-frame":   strings.Repeat(" ", 2<<20) + base,
		"too-many-items":    string(attachmentFrame(strings.Repeat(`{"type":3},`, 256) + `{"type":3}`)),
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := normalizeAttachments([]byte(raw), attachmentScope, attachmentReceipt)
			if !errors.Is(err, weixin.ErrProtocol) {
				t.Fatalf("invalid metadata accepted: %v", err)
			}
			if strings.Contains(err.Error(), attachmentHex) || strings.Contains(err.Error(), "example.test") {
				t.Fatal("metadata leaked in error")
			}
		})
	}
	raw := []byte(`{"msgs":false,"get_updates_buf":{},"ret":-14}`)
	_, _, err := normalizeAttachments(raw, attachmentScope, attachmentReceipt)
	if !errors.Is(err, weixin.ErrAuthExpired) {
		t.Fatalf("expiry lost behind shape failure: %v", err)
	}
}

func TestAttachmentOriginalTypeCannotHideMedia(t *testing.T) {
	// A later text type must not erase an original media type before alias checks.
	raw := attachmentFrame(`{"type":2,"type":1,"text_item":{"text":"caption"},"image_item":{}}`)
	_, _, err := normalizeAttachments(raw, attachmentScope, attachmentReceipt)
	if !errors.Is(err, weixin.ErrProtocol) {
		t.Fatalf("duplicate type hid media: %v", err)
	}
}

func TestAttachmentLosslessCaptionIdentityAndTimestamp(t *testing.T) {
	for _, timestamp := range []string{`"invalid"`, `-1`, `1.5`, `null`, `{}`, `253402300800000`} {
		raw := strings.Replace(string(attachmentFrame(attachmentImage())), `"context_token"`, `"create_time_ms":`+timestamp+`,"context_token"`, 1)
		entry := attachmentOnly(t, []byte(raw))
		if !entry.ReceivedAt.Equal(attachmentReceipt) {
			t.Fatalf("invalid advisory timestamp did not use receipt: %s", timestamp)
		}
	}
	raw := strings.Replace(string(attachmentFrame(`{"type":1,"msg_id":"opaque/id","text_item":{"text":" first\n"}},{"type":1,"text_item":{"text":"second\t"}},`+attachmentImage())), `"message_id":"18446744073709551615",`, "", 1)
	raw = strings.Replace(raw, `"context_token"`, `"create_time_ms":1000,"context_token"`, 1)
	entry := attachmentOnly(t, []byte(raw))
	if entry.MessageID != "opaque/id" || entry.Text != " first\n\nsecond\t" || !entry.ReceivedAt.Equal(time.UnixMilli(1000).UTC()) {
		t.Fatalf("lossless caption/identity/time changed: %#v", entry)
	}
	before, _, err := normalize(attachmentFrame(attachmentImage()), attachmentScope, attachmentReceipt)
	if err != nil || len(before) != 0 {
		t.Fatal("existing default media behavior changed")
	}
}

func TestAttachmentFallbackAndOptionalMetadata(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	for _, typ := range []int{2, 4} {
		field := "image_item"
		kind := "image"
		if typ == 4 {
			field = "file_item"
			kind = "file"
		}
		item := `{"type":` + string(rune('0'+typ)) + `,"` + field + `":{"media":{"aes_key":"` + key + `","full_url":"https://example.test/full"}}}`
		entry := attachmentOnly(t, attachmentFrame(item))
		if entry.Attachment == nil || *entry.Attachment != (storage.Attachment{Kind: kind, MediaKey: key, FullURL: "https://example.test/full"}) {
			t.Fatalf("optional metadata changed: %#v", entry.Attachment)
		}
	}
	// Unused fallback shape is inert under a valid preferred hex spelling.
	raw := strings.Replace(string(attachmentFrame(attachmentImage())), `"unused-invalid-key"`, `{"arbitrary":"bounded"}`, 1)
	entry := attachmentOnly(t, []byte(raw))
	if entry.Attachment.MediaKey != "" {
		t.Fatal("unused fallback persisted")
	}
	raw = strings.Replace(string(attachmentFrame(attachmentImage())), attachmentHex, "invalid-preferred", 1)
	raw = strings.Replace(raw, "unused-invalid-key", key, 1)
	_, _, err := normalizeAttachments([]byte(raw), attachmentScope, attachmentReceipt)
	if !errors.Is(err, weixin.ErrProtocol) {
		t.Fatalf("invalid preferred key fell back: %v", err)
	}
}

func TestAttachmentExtendedRefusals(t *testing.T) {
	base := string(attachmentFrame(attachmentImage()))
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	file := string(attachmentFrame(`{"type":4,"file_item":{"media":{"aes_key":"` + key + `","encrypt_query_param":"query"},"file_name":"file.txt","len":"1","md5":"` + attachmentHex + `"}}`))
	for name, raw := range map[string]string{
		"unicode-fold":                strings.Replace(base, `"aeskey"`, `"aeſkey"`, 1),
		"null-key":                    strings.Replace(base, `"`+attachmentHex+`"`, `null`, 1),
		"no-locator":                  strings.Replace(strings.Replace(base, `"query"`, `""`, 1), `"https://example.test/full"`, `""`, 1),
		"file-key-alias":              strings.Replace(file, `"aes_key"`, `"aes_\u006bey"`, 1),
		"length-leading-zero":         strings.Replace(file, `"len":"1"`, `"len":"01"`, 1),
		"length-too-large":            strings.Replace(file, `"len":"1"`, `"len":"4194305"`, 1),
		"length-null":                 strings.Replace(file, `"len":"1"`, `"len":null`, 1),
		"bad-md5":                     strings.Replace(file, attachmentHex, "bad-md5", 1),
		"filename-too-large":          strings.Replace(file, "file.txt", strings.Repeat("x", 513), 1),
		"caption-alias":               string(attachmentFrame(`{"type":1,"text_item":{"TEXT":"caption"}},` + attachmentImage())),
		"join-overflow":               string(attachmentFrame(`{"type":1,"text_item":{"text":"` + strings.Repeat("x", 16384) + `"}},{"type":1,"text_item":{"text":""}},` + attachmentImage())),
		"id-overflow":                 strings.Replace(base, "18446744073709551615", "18446744073709551616", 1),
		"ignored-ref-too-large":       strings.Replace(base, `"type":2`, `"type":2,"ref_msg":"`+strings.Repeat("x", 16385)+`"`, 1),
		"canonical-dto-overflow":      strings.Replace(base, "https://example.test/full", "https://example.test/"+strings.Repeat("<", 2800), 1),
		"unsupported-voice-too-large": string(attachmentFrame(`{"type":3,"voice_item":"` + strings.Repeat("x", 16385) + `"}`)),
		"too-many-messages":           `{"msgs":[` + strings.Repeat(`{"from_user_id":"foreign"},`, 256) + `{"from_user_id":"foreign"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := normalizeAttachments([]byte(raw), attachmentScope, attachmentReceipt)
			if !errors.Is(err, weixin.ErrProtocol) {
				t.Fatalf("invalid field accepted: %v", err)
			}
		})
	}
}

func TestAttachmentEscapedCaptionBudget(t *testing.T) {
	// Escaped wire bytes may exceed the decoded caption budget; the genuine
	// decoded caption still fits exactly and retains every character.
	entry := attachmentOnly(t, attachmentFrame(`{"type":1,"text_item":{"text":"`+strings.Repeat(`\u0061`, 16384)+`"}},`+attachmentImage()))
	if entry.Text != strings.Repeat("a", 16384) {
		t.Fatal("escaped caption was truncated or rewritten")
	}
}

func TestAttachmentTextOnlyByteBehavior(t *testing.T) {
	for _, raw := range [][]byte{
		attachmentFrame(`{"type":1,"text_item":{"text":"text"}}`),
		[]byte(`{"get_updates_buf":null,"msgs":null}`),
		[]byte(`{"msgs":[],"MSGS":[],"ret":0}`),
		[]byte(`{"ret":-14,"msgs":false}`),
		[]byte(`{"ret":1,"errcode":-14,"msgs":[]}`),
		[]byte(strings.Replace(string(attachmentFrame(`{"type":1,"text_item":{"text":"text"}}`)), `"message_id":"18446744073709551615"`, `"message_id":"123","message_id":"456"`, 1)),
		[]byte(strings.Replace(string(attachmentFrame(`{"type":1,"text_item":{"text":"text"}}`)), `"from_user_id"`, `"from_\u0075ser_id"`, 1)),
	} {
		before, bc, be := normalize(raw, attachmentScope, attachmentReceipt)
		after, ac, ae := normalizeAttachments(raw, attachmentScope, attachmentReceipt)
		if !reflect.DeepEqual(before, after) || bc != ac || be != ae {
			t.Fatalf("text behavior changed: before=%v,%q,%v after=%v,%q,%v", before, bc, be, after, ac, ae)
		}
	}
}

func attachmentDuplicateLists(first, second, spelling string) []byte {
	return []byte(strings.TrimSuffix(string(attachmentFrame(first)), `]}]}`) + `],` + spelling + `:[` + second + `]}]}`)
}

func TestAttachmentOriginalItemListsCannotHideMedia(t *testing.T) {
	// Earlier media must survive detection even when a later matching list would
	// overwrite it during typed decoding. Strict original head admission refuses.
	for kind, first := range map[string]string{
		"image":       attachmentImage(),
		"file":        `{"type":4,"file_item":{"media":{"aes_key":null}}}`,
		"unsupported": `{"type":3,"voice_item":{"key":"unused"}}`,
	} {
		for suffix, second := range map[string]string{"text": `{"type":1,"text_item":{"text":"later caption"}}`, "empty": ""} {
			for alias, spelling := range map[string]string{"duplicate": `"item_list"`, "folded": `"ITEM_LIST"`, "escaped": `"item_\u006cist"`} {
				t.Run(kind+"/"+suffix+"/"+alias, func(t *testing.T) {
					_, _, err := normalizeAttachments(attachmentDuplicateLists(first, second, spelling), attachmentScope, attachmentReceipt)
					if !errors.Is(err, weixin.ErrProtocol) {
						t.Fatalf("later item_list hid original %s: %v", kind, err)
					}
				})
			}
		}
	}
}

func TestAttachmentOriginalItemListsPreserveTextAndAuthorization(t *testing.T) {
	first := `{"type":1,"text_item":{"text":"earlier"}}`
	for _, second := range []string{`{"type":1,"text_item":{"text":"later"}}`, ""} {
		for _, spelling := range []string{`"item_list"`, `"ITEM_LIST"`, `"item_\u006cist"`} {
			raw := attachmentDuplicateLists(first, second, spelling)
			before, bc, be := normalize(raw, attachmentScope, attachmentReceipt)
			after, ac, ae := normalizeAttachments(raw, attachmentScope, attachmentReceipt)
			if !reflect.DeepEqual(before, after) || bc != ac || be != ae {
				t.Fatalf("genuine text-only duplicate behavior changed for %s", spelling)
			}
		}
	}
	for _, change := range [][2]string{{`"alice"`, `"foreign"`}, {`"bot"`, `"foreign"`}, {`"context_token"`, `"group_id":"group","context_token"`}} {
		raw := attachmentDuplicateLists(strings.Replace(attachmentImage(), attachmentHex, "invalid-preferred", 1), first, `"ITEM_LIST"`)
		raw = []byte(strings.Replace(string(raw), change[0], change[1], 1))
		entries, cursor, err := normalizeAttachments(raw, attachmentScope, attachmentReceipt)
		if err != nil || len(entries) != 0 || cursor != "cursor" {
			t.Fatalf("unauthorized original list consumed media: %v", err)
		}
	}
	// The count is shared across original lists, rather than reset per field.
	raw := attachmentDuplicateLists(strings.TrimSuffix(strings.Repeat(`{"type":3},`, 255), ","), `{"type":1},{"type":1}`, `"item_list"`)
	_, _, err := normalizeAttachments(raw, attachmentScope, attachmentReceipt)
	if !errors.Is(err, weixin.ErrProtocol) {
		t.Fatalf("original cumulative item bound lost: %v", err)
	}
}
