// Package librarybundle provides the bounded version 1 library envelope.
package librarybundle

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
)

const (
	magic             = "MISKOAI-LIBRARY-1\n"
	maxHeader         = 65536
	maxDatabase int64 = 256 << 20
	maxAsset    int64 = 4 << 20
	maxAssets         = 128
	maxAssetSum int64 = 64 << 20
	maxEnvelope int64 = 321 << 20
)

type File struct {
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type Asset struct {
	Key    string `json:"key"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type Header struct {
	Database File    `json:"database"`
	Assets   []Asset `json:"assets"`
}

func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range []byte(s) {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// admit copies caller-owned records before any callback can mutate them.
func admit(h Header) (Header, error) {
	if h.Database.Bytes < 1 || h.Database.Bytes > maxDatabase || !validDigest(h.Database.SHA256) || len(h.Assets) > maxAssets {
		return Header{}, ErrInvalid
	}
	copyH := Header{Database: h.Database, Assets: make([]Asset, len(h.Assets))}
	var sum int64
	var previous string
	for i, a := range h.Assets {
		if a.Bytes < 1 || a.Bytes > maxAsset || !validDigest(a.Key) || !validDigest(a.SHA256) || (i > 0 && a.Key <= previous) || a.Bytes > maxAssetSum-sum {
			return Header{}, ErrInvalid
		}
		sum += a.Bytes
		previous = a.Key
		copyH.Assets[i] = a
	}
	// Header framing is included conservatively even before canonical serialization.
	if h.Database.Bytes > maxEnvelope-int64(len(magic)+4+maxHeader)-sum {
		return Header{}, ErrInvalid
	}
	return copyH, nil
}

// parseHeader reads the fixed grammar directly. Token-based field admission
// rejects duplicates (including escaped aliases), nulls and unknown fields.
func parseHeader(b []byte) (Header, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if !delim(d, '{') {
		return Header{}, ErrInvalid
	}
	var h Header
	seen := uint8(0)
	for d.More() {
		k, e := d.Token()
		if e != nil {
			return Header{}, ErrInvalid
		}
		key, ok := k.(string)
		if !ok {
			return Header{}, ErrInvalid
		}
		switch key {
		case "database":
			if seen&1 != 0 {
				return Header{}, ErrInvalid
			}
			seen |= 1
			a, e := parseFile(d, false)
			if e != nil {
				return Header{}, e
			}
			h.Database = File{a.Bytes, a.SHA256}
		case "assets":
			if seen&2 != 0 {
				return Header{}, ErrInvalid
			}
			seen |= 2
			if !delim(d, '[') {
				return Header{}, ErrInvalid
			}
			h.Assets = make([]Asset, 0)
			for d.More() {
				if len(h.Assets) == maxAssets {
					return Header{}, ErrInvalid
				}
				a, e := parseFile(d, true)
				if e != nil {
					return Header{}, e
				}
				h.Assets = append(h.Assets, a)
			}
			if !delim(d, ']') {
				return Header{}, ErrInvalid
			}
		default:
			return Header{}, ErrInvalid
		}
	}
	if seen != 3 || !delim(d, '}') {
		return Header{}, ErrInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return Header{}, ErrInvalid
	}
	return admit(h)
}
func delim(d *json.Decoder, want json.Delim) bool { t, e := d.Token(); return e == nil && t == want }
func parseFile(d *json.Decoder, asset bool) (Asset, error) {
	if !delim(d, '{') {
		return Asset{}, ErrInvalid
	}
	var a Asset
	seen := uint8(0)
	for d.More() {
		t, e := d.Token()
		if e != nil {
			return Asset{}, ErrInvalid
		}
		k, ok := t.(string)
		if !ok {
			return Asset{}, ErrInvalid
		}
		var bit uint8
		switch k {
		case "bytes":
			bit = 1
		case "sha256":
			bit = 2
		case "key":
			if !asset {
				return Asset{}, ErrInvalid
			}
			bit = 4
		default:
			return Asset{}, ErrInvalid
		}
		if seen&bit != 0 {
			return Asset{}, ErrInvalid
		}
		seen |= bit
		v, e := d.Token()
		if e != nil {
			return Asset{}, ErrInvalid
		}
		switch k {
		case "bytes":
			n, ok := v.(json.Number)
			if !ok {
				return Asset{}, ErrInvalid
			}
			s := string(n)
			for _, c := range []byte(s) {
				if c < '0' || c > '9' {
					return Asset{}, ErrInvalid
				}
			}
			a.Bytes, e = strconv.ParseInt(s, 10, 64)
			if e != nil {
				return Asset{}, ErrInvalid
			}
		case "sha256":
			var ok bool
			a.SHA256, ok = v.(string)
			if !ok {
				return Asset{}, ErrInvalid
			}
		case "key":
			var ok bool
			a.Key, ok = v.(string)
			if !ok {
				return Asset{}, ErrInvalid
			}
		}
	}
	required := uint8(3)
	if asset {
		required = 7
	}
	if seen != required || !delim(d, '}') {
		return Asset{}, ErrInvalid
	}
	return a, nil
}
