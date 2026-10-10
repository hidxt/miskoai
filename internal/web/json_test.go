package web

import (
	"strings"
	"testing"
)

func TestStrictRequestObject(t *testing.T) {
	good := `{"password":"synthetic","nonce":"ok"}`
	got, e := strictRequestObject(strings.NewReader(good), 2048, "password", "nonce")
	if e != nil || got["nonce"] != "ok" {
		t.Errorf("valid object: %v", e)
	}
	for _, body := range []string{`{"password":"\ud834\udd1e","nonce":"ok"}`, `{"password":"quote\"slash\\end","nonce":"ok"}`, `{"password":"\ufffd","nonce":"ok"}`} {
		if _, e := strictRequestObject(strings.NewReader(body), 2048, "password", "nonce"); e != nil {
			t.Errorf("valid escaped string refused: %v", e)
		}
	}
	for _, body := range []string{`{"password":"a","password":"b","nonce":"x"}`, `{"password":"a","\u0070assword":"b","nonce":"x"}`, `{"password":null,"nonce":"x"}`, `{"password":1,"nonce":"x"}`, `{"password":{},"nonce":"x"}`, `{"password":"a","nonce":"x","extra":"x"}`, `{"password":"a"}`, `[]`, good + ` {}`, good + `x`, "{\"password\":\"" + string([]byte{255}) + "\",\"nonce\":\"x\"}", `{"password":"\ud800","nonce":"x"}`, strings.Repeat(" ", 2049) + good} {
		if _, e := strictRequestObject(strings.NewReader(body), 2048, "password", "nonce"); e == nil {
			t.Errorf("accepted invalid input %q", body)
		}
	}
}
