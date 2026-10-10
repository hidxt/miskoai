package media

import (
	"strings"
	"testing"
)

func TestCDNURLPrecedenceAndRefusal(t *testing.T) {
	const good = "https://novac2c.cdn.weixin.qq.com/c2c/download?a=%2f&z=hello+world"
	got, e := cdnURL("download", "unused", good, "")
	if e != nil || got != good {
		t.Fatalf("preferred bytes not preserved: %q %v", got, e)
	}
	got, e = cdnURL("download", "a&b+ c", "", "")
	if e != nil || got != "https://novac2c.cdn.weixin.qq.com/c2c/download?encrypted_query_param=a%26b%2B+c" {
		t.Fatalf("fallback: %q %v", got, e)
	}
	for _, bad := range []string{
		"http://novac2c.cdn.weixin.qq.com/c2c/download?a=b", "https://evil.invalid/c2c/download?a=b",
		"https://u@novac2c.cdn.weixin.qq.com/c2c/download?a=b", "https://novac2c.cdn.weixin.qq.com:444/c2c/download?a=b",
		"https://novac2c.cdn.weixin.qq.com/c2c/upload?a=b", "https://novac2c.cdn.weixin.qq.com/c2c/%64ownload?a=b",
		good + "#", good + "#fragment", "https://novac2c.cdn.weixin.qq.com/c2c/download?", good + "&a=c",
		good + "&bad=%xz", good + ";foo=bar", good + "&control=%0a", good + "&=x", good + "&x=" + strings.Repeat("x", 4097),
		good + " ", "https://NOVAC2C.cdn.weixin.qq.com/c2c/download?a=b", good + "&" + strings.Repeat("k", 65) + "=v",
	} {
		if _, e := cdnURL("download", "valid-fallback", bad, ""); e == nil {
			t.Errorf("accepted unsafe preferred %q", bad)
		}
	}
	if _, e := cdnURL("download", "", "", ""); e == nil {
		t.Fatal("empty parameter accepted")
	}
	for _, f := range []string{"", strings.Repeat("A", 32), strings.Repeat("a", 31), strings.Repeat("g", 32)} {
		if _, e := cdnURL("upload", "x", "https://novac2c.cdn.weixin.qq.com/c2c/upload?a=b", f); e == nil {
			t.Errorf("bad filekey accepted %q", f)
		}
	}
	if _, e := cdnURL("upload", "x", "", strings.Repeat("a", 32)); e != nil {
		t.Fatal(e)
	}
}

func TestCDNURLMetadataBounds(t *testing.T) {
	const prefix = "https://novac2c.cdn.weixin.qq.com/c2c/download?"
	for _, raw := range []string{
		prefix + "a=" + strings.Repeat("x", 16384), prefix + "a=%ff", prefix + "a=%C2%85",
		"https://novac2c.cdn.weixin.qq.com:0443/c2c/download?a=b",
		"https://novac2c.cdn.weixin.qq.com./c2c/download?a=b",
		"https://novac2c.cdn.weixin.qq.com/c2c/./download?a=b",
	} {
		if _, e := cdnURL("download", "valid", raw, ""); e == nil {
			t.Errorf("unsafe accepted %q", raw)
		}
	}
	many := []string{}
	for _, k := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q"} {
		many = append(many, k+"=x")
	}
	if _, e := cdnURL("download", "valid", prefix+strings.Join(many, "&"), ""); e == nil {
		t.Fatal("17 query keys accepted")
	}
	for _, s := range []string{"", strings.Repeat("x", 4097), "x\n", string([]byte{255})} {
		if _, e := cdnURL("download", s, "", ""); e == nil {
			t.Fatal("bad fallback accepted")
		}
	}
	if got, e := cdnURL("download", "unused", "https://novac2c.cdn.weixin.qq.com:443/c2c/download?a=", ""); e != nil || got != "https://novac2c.cdn.weixin.qq.com:443/c2c/download?a=" {
		t.Fatal("valid explicit443 query rejected")
	}
}
