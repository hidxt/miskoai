package media

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Route names and parameter spellings follow Tencent/openclaw-weixin,
// commit 24de5c9eb0dd5e595d7e2d090ed8a3f82870d42c (MIT; internal/notices/notices.txt).
// The admission policy is intentionally narrower than the reference builder.
const cdnOrigin = "https://novac2c.cdn.weixin.qq.com"

func cleanMetadata(s string, max int, whitespace bool) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || (whitespace && unicode.IsSpace(r)) {
			return false
		}
	}
	return true
}
func cdnURL(route, parameter, fullURL, filekey string) (string, error) {
	if route != "download" && route != "upload" {
		return "", ErrInvalid
	}
	if route == "upload" {
		if len(filekey) != 32 {
			return "", ErrInvalid
		}
		for _, r := range filekey {
			if !(r >= 'a' && r <= 'f') && !(r >= '0' && r <= '9') {
				return "", ErrInvalid
			}
		}
	}
	raw := fullURL
	if raw == "" {
		if parameter == "" || !cleanMetadata(parameter, 4096, false) {
			return "", ErrInvalid
		}
		q := url.Values{"encrypted_query_param": {parameter}}
		if route == "upload" {
			q.Set("filekey", filekey)
		}
		raw = cdnOrigin + "/c2c/" + route + "?" + q.Encode()
	}
	if !cleanMetadata(raw, 16<<10, true) || strings.Contains(raw, "#") {
		return "", ErrEndpoint
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || (u.Host != "novac2c.cdn.weixin.qq.com" && u.Host != "novac2c.cdn.weixin.qq.com:443") || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.ForceQuery || u.RawPath != "" || u.Path != "/c2c/"+route {
		return "", ErrEndpoint
	}
	if u.RawQuery == "" || len(u.RawQuery) > 12<<10 {
		return "", ErrEndpoint
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil || len(q) == 0 || len(q) > 16 {
		return "", ErrEndpoint
	}
	for k, v := range q {
		if k == "" || !cleanMetadata(k, 64, false) || len(v) != 1 || !cleanMetadata(v[0], 4096, false) {
			return "", ErrEndpoint
		}
	}
	return raw, nil
}
