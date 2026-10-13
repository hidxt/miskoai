package librarybundle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func envelope(h string, p []byte) []byte {
	b := []byte("MISKOAI-LIBRARY-1\n")
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(h)))
	b = append(b, n[:]...)
	b = append(b, []byte(h)...)
	return append(b, p...)
}
func rawHeader(db []byte, a []Asset) string {
	if a == nil {
		a = []Asset{}
	}
	b, _ := json.Marshal(Header{File{int64(len(db)), digest(db)}, a})
	return string(b)
}
func TestHeaderStrictBeforeCallback(t *testing.T) {
	good := rawHeader([]byte("d"), nil)
	cases := map[string]string{
		"null": "null", "array": "[]", "missing_assets": strings.Replace(good, `,"assets":[]`, "", 1), "null_assets": strings.Replace(good, `"assets":[]`, `"assets":null`, 1), "duplicate_root": strings.Replace(good, `"assets":[]`, `"assets":[],"assets":[]`, 1), "duplicate_db": strings.Replace(good, `"bytes":1`, `"bytes":1,"bytes":1`, 1), "unknown_root": strings.Replace(good, `"assets":[]`, `"assets":[],"other":1`, 1), "unknown_db": strings.Replace(good, `"bytes":1`, `"bytes":1,"other":1`, 1), "null_db": `{"database":null,"assets":[]}`, "trailing": good + ` {}`, "fraction": strings.Replace(good, `"bytes":1`, `"bytes":1.0`, 1), "exponent": strings.Replace(good, `"bytes":1`, `"bytes":1e0`, 1), "overflow": strings.Replace(good, `"bytes":1`, `"bytes":9223372036854775808`, 1), "negative": strings.Replace(good, `"bytes":1`, `"bytes":-1`, 1), "zero": strings.Replace(good, `"bytes":1`, `"bytes":0`, 1), "uppercase": strings.Replace(good, digest([]byte("d")), strings.ToUpper(digest([]byte("d"))), 1), "missing_sha": `{"database":{"bytes":1},"assets":[]}`,
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			got, err := Decode(context.Background(), bytes.NewReader(envelope(h, []byte("d"))), func(Record, io.Reader) error { calls++; return nil })
			if !errors.Is(err, ErrInvalid) || calls != 0 || got.Database.Bytes != 0 {
				t.Fatalf("invalid header admitted: calls=%d header=%+v err=%v", calls, got, err)
			}
		})
	}
}
func TestHeaderQuotaAdmission(t *testing.T) {
	sha := strings.Repeat("a", 64)
	mk := func(n int, size int64) Header {
		h := Header{File{256 << 20, sha}, make([]Asset, n)}
		for i := range h.Assets {
			h.Assets[i] = Asset{fmt.Sprintf("%064x", i), size, sha}
		}
		return h
	}
	cases := []struct {
		name  string
		h     Header
		valid bool
	}{{"empty", mk(0, 1), true}, {"128", mk(128, 1), true}, {"129", mk(129, 1), false}, {"sum_exact", mk(16, 4<<20), true}, {"sum_over", mk(17, 4<<20), false}, {"asset_exact", mk(1, 4<<20), true}, {"asset_over", mk(1, (4<<20)+1), false}, {"db_over", Header{File{(256 << 20) + 1, sha}, []Asset{}}, false}, {"db_zero", Header{File{0, sha}, []Asset{}}, false}, {"asset_zero", mk(1, 0), false}, {"asset_negative", mk(1, -1), false}, {"db_overflow", Header{File{1 << 62, sha}, []Asset{}}, false}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			err := Encode(context.Background(), io.Discard, c.h, func(Record) (io.Reader, func() error, error) { calls++; return nil, nil, errors.New("open canary") })
			if c.valid {
				if calls != 1 || !errors.Is(err, ErrIO) {
					t.Fatalf("valid rejected calls=%d err=%v", calls, err)
				}
			} else if calls != 0 || !errors.Is(err, ErrInvalid) {
				t.Fatalf("bad admitted calls=%d err=%v", calls, err)
			}
			encoded, marshalErr := json.Marshal(c.h)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			calls = 0
			_, err = Decode(context.Background(), bytes.NewReader(envelope(string(encoded), nil)), func(Record, io.Reader) error { calls++; return errors.New("callback canary") })
			if c.valid {
				if calls != 1 || !errors.Is(err, ErrIO) {
					t.Fatalf("decode valid calls=%d err=%v", calls, err)
				}
			} else if calls != 0 || !errors.Is(err, ErrInvalid) {
				t.Fatalf("decode invalid calls=%d err=%v", calls, err)
			}
		})
	}
}
func TestHeaderAssetStrict(t *testing.T) {
	sha := digest([]byte("x"))
	a := Asset{strings.Repeat("1", 64), 1, sha}
	good := rawHeader([]byte("d"), []Asset{a})
	cases := map[string]string{"null_record": strings.Replace(good, `[{"key"`, `[null,{"key"`, 1), "duplicate_key": strings.Replace(good, `"key":`, `"key":"`+a.Key+`","key":`, 1), "unknown": strings.Replace(good, `"key":`, `"other":1,"key":`, 1), "missing_key": strings.Replace(good, `"key":"`+a.Key+`",`, "", 1), "bad_key": strings.Replace(good, a.Key, "bad", 1), "duplicate_record": rawHeader([]byte("d"), []Asset{a, a}), "unsorted": rawHeader([]byte("d"), []Asset{{strings.Repeat("2", 64), 1, sha}, a})}
	for n, h := range cases {
		t.Run(n, func(t *testing.T) {
			calls := 0
			_, err := Decode(context.Background(), bytes.NewReader(envelope(h, nil)), func(Record, io.Reader) error { calls++; return nil })
			if !errors.Is(err, ErrInvalid) || calls != 0 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
func TestHeaderFraming(t *testing.T) {
	good := envelope(rawHeader([]byte("d"), nil), []byte("d"))
	cases := [][]byte{nil, []byte("wrong"), good[:18], append([]byte("MISKOAI-LIBRARY-1\n"), 0, 0, 0, 0), append([]byte("MISKOAI-LIBRARY-1\n"), 1, 0, 1, 0)}
	for i, b := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			calls := 0
			_, err := Decode(context.Background(), bytes.NewReader(b), func(Record, io.Reader) error { calls++; return nil })
			if err == nil || calls != 0 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestHeaderAdditionalGrammar(t *testing.T) {
	good := rawHeader([]byte("d"), []Asset{{strings.Repeat("a", 64), 1, digest([]byte("a"))}})
	cases := []string{
		strings.Replace(good, `"bytes":1`, `"bytes":"1"`, 1),
		strings.Replace(good, `"bytes":1`, `"bytes":true`, 1),
		strings.Replace(good, `"sha256":"`+digest([]byte("d"))+`"`, `"sha256":null`, 1),
		strings.Replace(good, `"assets":[`, `"assets":{`, 1),
		strings.Replace(good, `"bytes":1`, `"bytes":1,"b\u0079tes":1`, 1),
		strings.Replace(good, `"assets":[`, `"assets":[],"\u0061ssets":[`, 1),
		strings.Replace(good, `"key":"`+strings.Repeat("a", 64)+`"`, `"key":null`, 1),
		strings.Replace(good, `"key":"`+strings.Repeat("a", 64)+`"`, `"key":1`, 1),
		good + ` CANARY`,
	}
	for i, h := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			calls := 0
			_, err := Decode(context.Background(), bytes.NewReader(envelope(h, nil)), func(Record, io.Reader) error { calls++; return nil })
			if !errors.Is(err, ErrInvalid) || calls != 0 || strings.Contains(err.Error(), "CANARY") {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestHeaderLengthBounds(t *testing.T) {
	h := rawHeader([]byte("d"), nil)
	for _, n := range []int{65536, 65537} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			padded := h + strings.Repeat(" ", n-len(h))
			calls := 0
			_, err := Decode(context.Background(), bytes.NewReader(envelope(padded, []byte("d"))), func(_ Record, r io.Reader) error { calls++; _, e := io.Copy(io.Discard, r); return e })
			if n == 65536 {
				if err != nil || calls != 1 {
					t.Fatalf("exact header rejected: %v", err)
				}
			} else if !errors.Is(err, ErrInvalid) || calls != 0 {
				t.Fatalf("over header admitted calls=%d err=%v", calls, err)
			}
		})
	}
}
