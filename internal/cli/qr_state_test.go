package cli

import (
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"testing"
)

func TestQRStateDoesNotRedirectAlreadyBoundAndClearsCode(t *testing.T) {
	if _, _, err := nextQRState(weixin.QRStatus{Status: "binded_redirect"}, weixin.DefaultBaseURL, ""); err == nil {
		t.Fatal("already-bound state treated as login success or redirect")
	}
	base, code, err := nextQRState(weixin.QRStatus{Status: "scaned"}, weixin.DefaultBaseURL, "123456")
	if err != nil || base != weixin.DefaultBaseURL || code != "" {
		t.Fatal("scanned state retained stale verification code", err)
	}
}
