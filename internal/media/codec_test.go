package media

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mediaDir(t *testing.T) string { t.Helper(); return filepath.Join(t.TempDir(), "private") }
func decodeHex(s string) []byte {
	b, e := hex.DecodeString(s)
	if e != nil {
		panic(e)
	}
	return b
}
func artifactBytes(t *testing.T, a *Artifact) []byte {
	t.Helper()
	b, e := io.ReadAll(a)
	if e != nil {
		t.Fatal(e)
	}
	if int64(len(b)) != a.Size() {
		t.Fatal("size mismatch")
	}
	return b
}
func noSpools(t *testing.T, dir string) {
	t.Helper()
	entries, e := os.ReadDir(dir)
	if e != nil && !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if len(entries) != 0 {
		t.Fatalf("leaked %d spools", len(entries))
	}
}

func TestMediaKeyFormsAndPrecedence(t *testing.T) {
	raw := decodeHex("000102030405060708090a0b0c0d0e0f")
	var want [16]byte
	copy(want[:], raw)
	forms := [][2]string{{hex.EncodeToString(raw), "ignored invalid fallback"}, {strings.ToUpper(hex.EncodeToString(raw)), ""}, {"", base64.StdEncoding.EncodeToString(raw)}, {"", base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(raw)))}}
	for i, p := range forms {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			got, e := ParseKey(p[0], p[1])
			if e != nil || got != want {
				t.Fatalf("valid key: %v", e)
			}
		})
	}
	invalid := [][2]string{{"", ""}, {"bad", forms[2][1]}, {strings.Repeat("g", 32), ""}, {"", forms[2][1] + "\n"}, {"", " " + forms[2][1]}, {"", forms[2][1][:22] + "B="}, {"", base64.StdEncoding.EncodeToString(make([]byte, 17))}, {"", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("z", 32)))}, {"", strings.Repeat("A", 1024)}}
	for i, p := range invalid {
		t.Run("invalid"+string(rune('a'+i)), func(t *testing.T) {
			_, e := ParseKey(p[0], p[1])
			if !errors.Is(e, ErrInvalid) {
				t.Fatalf("want invalid, got %v", e)
			}
		})
	}
}

func TestMediaECBKnownVectorAndPadding(t *testing.T) {
	var key [16]byte
	copy(key[:], decodeHex("000102030405060708090a0b0c0d0e0f"))
	// FIPS-197 first AES block plus independently specified full PKCS7 block.
	plain := decodeHex("00112233445566778899aabbccddeeff")
	dir := mediaDir(t)
	a, e := Encrypt(context.Background(), dir, bytes.NewReader(plain), key)
	if e != nil {
		t.Fatal(e)
	}
	got := artifactBytes(t, a)
	if len(got) != 32 || !bytes.Equal(got[:16], decodeHex("69c4e0d86a7b0430d8cdb78070b4c55a")) {
		t.Fatal("known block mismatch")
	}
	block, _ := aes.NewCipher(key[:])
	padded := make([]byte, 16)
	block.Decrypt(padded, got[16:])
	if !bytes.Equal(padded, bytes.Repeat([]byte{16}, 16)) {
		t.Fatal("full padding missing")
	}
	if e = a.Close(); e != nil {
		t.Fatal(e)
	}
	noSpools(t, dir)
	for n := 0; n < 32; n++ {
		a, e := Encrypt(context.Background(), dir, bytes.NewReader(bytes.Repeat([]byte{0x42}, n)), key)
		if e != nil {
			t.Fatal(e)
		}
		b := artifactBytes(t, a)
		a.Close()
		p, e := Decrypt(context.Background(), dir, bytes.NewReader(b), key)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(artifactBytes(t, p), bytes.Repeat([]byte{0x42}, n)) {
			t.Fatal("padding length mismatch")
		}
		p.Close()
	}
}

type generatedReader struct {
	remaining int64
	maxRead   int
}

func (r *generatedReader) Read(p []byte) (int, error) {
	if len(p) > r.maxRead {
		r.maxRead = len(p)
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	for i := 0; i < n; i++ {
		p[i] = 0x42
	}
	r.remaining -= int64(n)
	return n, nil
}
func TestMediaMaximumAlignedRoundTrip(t *testing.T) {
	dir := mediaDir(t)
	r := &generatedReader{remaining: 4 << 20}
	a, e := Encrypt(context.Background(), dir, r, [16]byte{})
	if e != nil {
		t.Fatal(e)
	}
	if a.Size() != (4<<20)+16 || r.maxRead > 32768 {
		t.Fatal("stream bound")
	}
	p, e := Decrypt(context.Background(), dir, a, [16]byte{})
	if e != nil {
		t.Fatal(e)
	}
	if p.Size() != 4<<20 {
		t.Fatal("maximum rejected")
	}
	buf := make([]byte, 32768)
	var total int64
	for {
		n, e := p.Read(buf)
		for _, b := range buf[:n] {
			if b != 0x42 {
				t.Fatal("content")
			}
		}
		total += int64(n)
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	if total != 4<<20 {
		t.Fatal("truncated")
	}
	p.Close()
	a.Close()
	noSpools(t, dir)
	a, e = Encrypt(context.Background(), dir, &generatedReader{remaining: (4 << 20) + 1}, [16]byte{})
	if a != nil || !errors.Is(e, ErrLimit) {
		t.Fatalf("plus one: %v", e)
	}
	noSpools(t, dir)
	a, e = Decrypt(context.Background(), dir, &generatedReader{remaining: (4 << 20) + 17}, [16]byte{})
	if a != nil || !errors.Is(e, ErrLimit) {
		t.Fatalf("cipher plus one: %v", e)
	}
	noSpools(t, dir)
}

func TestMediaRefusesInvalidCipherBeforeExposure(t *testing.T) {
	b, _ := aes.NewCipher(make([]byte, 16))
	cases := [][]byte{nil, {1}, make([]byte, 15), make([]byte, 17)}
	for _, pad := range []byte{0, 17, 2} {
		p := bytes.Repeat([]byte{1}, 16)
		p[15] = pad
		c := make([]byte, 32)
		b.Encrypt(c[:16], make([]byte, 16))
		b.Encrypt(c[16:], p)
		cases = append(cases, c)
	}
	for i, c := range cases {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			dir := mediaDir(t)
			a, e := Decrypt(context.Background(), dir, bytes.NewReader(c), [16]byte{})
			if a != nil || !errors.Is(e, ErrInvalid) {
				t.Fatalf("exposed invalid: %v", e)
			}
			noSpools(t, dir)
		})
	}
}

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	copy(p, bytes.Repeat([]byte{7}, 32))
	return 32, errors.New("sensitive reader detail")
}

type cancelReader struct{ cancel context.CancelFunc }

func (r cancelReader) Read(p []byte) (int, error) {
	r.cancel()
	copy(p, bytes.Repeat([]byte{7}, 32))
	return 32, nil
}
func TestMediaIOAndCancellationCleanup(t *testing.T) {
	for _, decrypt := range []bool{false, true} {
		for _, name := range []string{"read", "write", "short", "sync", "close", "cancel", "deadline"} {
			t.Run(name+map[bool]string{true: "decrypt", false: "encrypt"}[decrypt], func(t *testing.T) {
				dir := mediaDir(t)
				ctx := context.Background()
				input := io.Reader(bytes.NewReader(make([]byte, 32)))
				ops := spoolOps{}
				want := ErrIO
				switch name {
				case "read":
					input = errorReader{}
				case "write":
					ops.write = func(*os.File, []byte) (int, error) { return 0, errors.New("sensitive write detail") }
				case "short":
					ops.write = func(_ *os.File, p []byte) (int, error) { return len(p) - 1, nil }
				case "sync":
					ops.sync = func(*os.File) error { return errors.New("sync") }
				case "close":
					ops.close = func(f *os.File) error { f.Close(); return errors.New("close") }
				case "cancel":
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					input = cancelReader{cancel}
					want = context.Canceled
				case "deadline":
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, testingDeadlinePast())
					defer cancel()
					want = context.DeadlineExceeded
				}
				if decrypt && name != "read" && name != "cancel" {
					b, _ := aes.NewCipher(make([]byte, 16))
					c := make([]byte, 32)
					b.Encrypt(c[:16], make([]byte, 16))
					b.Encrypt(c[16:], bytes.Repeat([]byte{16}, 16))
					input = bytes.NewReader(c)
				}
				a, e := transform(ctx, dir, input, [16]byte{}, decrypt, ops)
				if a != nil || !errors.Is(e, want) {
					t.Fatalf("want %v got %v", want, e)
				}
				noSpools(t, dir)
			})
		}
	}
}

// This finite allocation probe generates maximum input without a whole-payload
// fixture. Its allocation result is host evidence, not an RSS or VPS claim.
func BenchmarkMediaMaximumEncrypt(b *testing.B) {
	dir := filepath.Join(b.TempDir(), "private")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		a, e := Encrypt(context.Background(), dir, &generatedReader{remaining: 4 << 20}, [16]byte{})
		if e != nil {
			b.Fatal(e)
		}
		if a.Size() != (4<<20)+16 {
			b.Fatal("size")
		}
		if e = a.Close(); e != nil {
			b.Fatal(e)
		}
	}
}
