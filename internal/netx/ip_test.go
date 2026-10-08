package netx

import (
	"net/netip"
	"testing"
)

func TestPublicIPRejectsPrivateAndTransitionRanges(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1", "fe80::1", "::ffff:192.168.1.1", "64:ff9b::a00:1", "64:ff9b:1::a00:1", "2002:0a00:0001::1", "2001:0000:4136:e378:8000:63bf:3fff:fdd2"} {
		t.Run(raw, func(t *testing.T) {
			if PublicIP(netip.MustParseAddr(raw)) {
				t.Fatal("unsafe address accepted")
			}
		})
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !PublicIP(netip.MustParseAddr(raw)) {
			t.Errorf("public address rejected: %s", raw)
		}
	}
	if PublicIP(netip.Addr{}) {
		t.Fatal("invalid address accepted")
	}
}
