package weixin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func imageUploadFixture() ImageUploadRequest {
	return ImageUploadRequest{ToUserID: "synthetic-user", FileKey: strings.Repeat("a", 32), RawMD5: strings.Repeat("b", 32), AESKeyHex: strings.Repeat("c", 32), RawBytes: 4 << 20}
}

func TestImageUploadWire(t *testing.T) {
	calls := 0
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/ilink/bot/getuploadurl" || r.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Error("incorrect authenticated route")
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 15*time.Second {
			t.Error("deadline")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"filekey": strings.Repeat("a", 32), "to_user_id": "synthetic-user", "rawsize": float64(4 << 20), "rawfilemd5": strings.Repeat("b", 32), "filesize": float64((4 << 20) + 16), "media_type": float64(1), "no_need_thumb": true, "aeskey": strings.Repeat("c", 32)}
		if len(body) != len(want)+1 {
			t.Error("extra or missing fields")
		}
		for k, v := range want {
			if body[k] != v {
				t.Errorf("field %s mismatch", k)
			}
		}
		base := body["base_info"].(map[string]any)
		if base["channel_version"] != ProtocolVersion {
			t.Error("base info")
		}
		return response(`{"upload_param":"synthetic-param"}`), nil
	})
	loc, err := c.GetImageUploadURL(context.Background(), imageUploadFixture())
	if err != nil || loc.UploadParam != "synthetic-param" || calls != 1 {
		t.Fatalf("location/calls error: %v %d", err, calls)
	}
}

func TestImageUploadLocationStrict(t *testing.T) {
	cases := []struct {
		name, body string
		want       error
	}{
		{"retzero", `{"ret":0,"upload_param":"p"}`, nil},
		{"url", `{"upload_full_url":"https://untrusted.example/path"}`, nil},
		{"both", `{"upload_param":"p","upload_full_url":"https://untrusted.example","thumb_upload_param":"t","extra":{"ret":null}}`, nil},
		{"empty", `{}`, ErrProtocol}, {"nullret", `{"ret":null,"upload_param":"p"}`, ErrProtocol},
		{"stringret", `{"ret":"0","upload_param":"p"}`, ErrProtocol}, {"fraction", `{"ret":0.0,"upload_param":"p"}`, ErrProtocol},
		{"duplicate", `{"upload_param":"p","UPLOAD_PARAM":"q"}`, ErrProtocol},
		{"fold", `{"upload_full_url":"p","upload_full_urL":"q"}`, ErrProtocol},
		{"duplicateret", `{"ret":0,"RET":0,"upload_param":"p"}`, ErrProtocol},
		{"thumbnull", `{"upload_param":"p","thumb_upload_param":null}`, ErrProtocol},
		{"control", `{"upload_param":"p\n"}`, ErrProtocol}, {"whitespace", `{"upload_full_url":" "}`, ErrProtocol},
		{"oversize", `{"upload_param":"` + strings.Repeat("p", maxOpaqueBytes+1) + `"}`, ErrProtocol},
		{"trailing", `{"upload_param":"p"}{}`, ErrProtocol}, {"array", `[]`, ErrProtocol},
		{"utf8", "{\"upload_param\":\"\xff\"}", ErrProtocol},
		{"service", `{"ret":9,"upload_param":"p"}`, ErrService},
		{"expired", `{"ret":9,"errcode":-14,"upload_param":null}`, ErrAuthExpired},
		{"expiredduplicate", `{"ret":0,"RET":-14,"upload_param":"p"}`, ErrAuthExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t, func(*http.Request) (*http.Response, error) { return response(tc.body), nil })
			loc, err := c.GetImageUploadURL(context.Background(), imageUploadFixture())
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if err != nil && loc != (ImageUploadLocation{}) {
				t.Error("partial location")
			}
		})
	}
}

func TestImageUploadAdmission(t *testing.T) {
	for _, mutate := range []func(*ImageUploadRequest){func(q *ImageUploadRequest) { q.RawBytes = 0 }, func(q *ImageUploadRequest) { q.RawBytes = (4 << 20) + 1 }, func(q *ImageUploadRequest) { q.FileKey = "A" + q.FileKey[1:] }, func(q *ImageUploadRequest) { q.RawMD5 = "x" + q.RawMD5[1:] }, func(q *ImageUploadRequest) { q.AESKeyHex = "" }, func(q *ImageUploadRequest) { q.ToUserID = "" }, func(q *ImageUploadRequest) { q.ToUserID = "bad\n" }, func(q *ImageUploadRequest) { q.ToUserID = "\xff" }, func(q *ImageUploadRequest) { q.ToUserID = strings.Repeat("u", maxOpaqueBytes+1) }} {
		q := imageUploadFixture()
		mutate(&q)
		c := fixture(t, func(*http.Request) (*http.Response, error) {
			t.Error("dispatched invalid input")
			return response(`{}`), nil
		})
		if _, err := c.GetImageUploadURL(context.Background(), q); !errors.Is(err, ErrInvalidInput) {
			t.Fatal(err)
		}
	}
}

func TestImageUploadPaddedSizes(t *testing.T) {
	for _, size := range []int64{1, 15, 16, 17, 4 << 20} {
		q := imageUploadFixture()
		q.RawBytes = size
		c := fixture(t, func(r *http.Request) (*http.Response, error) {
			var body struct {
				Raw    int64 `json:"rawsize"`
				Cipher int64 `json:"filesize"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Raw != size || body.Cipher != 16*(size/16+1) {
				t.Error("padding boundary")
			}
			return response(`{"upload_param":"p"}`), nil
		})
		if _, err := c.GetImageUploadURL(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
}
