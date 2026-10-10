package document

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func expectText(t *testing.T, f Format, in, want []byte) {
	t.Helper()
	got, err := New().Extract(context.Background(), f, bytes.NewReader(in), int64(len(in)))
	if err != nil || got.Text != string(want) || got.Pages != 1 || got.Format != f {
		t.Fatalf("extraction mismatch: error=%v pages=%d format=%q length=%d", err, got.Pages, got.Format, len(got.Text))
	}
}
func expectError(t *testing.T, p *Parser, ctx context.Context, f Format, r io.ReaderAt, size int64, want error) {
	t.Helper()
	got, err := p.Extract(ctx, f, r, size)
	if !errors.Is(err, want) || got != (Result{}) {
		t.Fatalf("want safe %v and empty result, got %v length=%d", want, err, len(got.Text))
	}
	if err != nil && strings.Contains(err.Error(), "PRIVATE-CANARY") {
		t.Fatal("reader payload leaked")
	}
}

// Removing carry handling, BOM position checks, or preservation breaks these cases.
func TestTextUTF8SplitAndBOM(t *testing.T) {
	for _, f := range []Format{TXT, Markdown} {
		t.Run(string(f), func(t *testing.T) {
			for _, width := range []int{2, 3, 4} {
				runeText := map[int]string{2: "é", 3: "中", 4: "😀"}[width]
				for split := 1; split < width; split++ {
					in := []byte(strings.Repeat("x", 4096-split) + runeText + "\r\n <script>&amp;</script>  \t")
					expectText(t, f, in, in)
				}
			}
			expectText(t, f, []byte("\xef\xbb\xbf中\n"), []byte("中\n"))
			expectText(t, f, []byte("\xef\xbb\xbf\xef\xbb\xbfx"), []byte("\xef\xbb\xbfx"))
			expectText(t, f, []byte("x\xef\xbb\xbf"), []byte("x\xef\xbb\xbf"))
		})
	}
	for name, in := range map[string][]byte{
		"utf16le": {0xff, 0xfe, 'a', 0}, "utf16be": {0xfe, 0xff, 0, 'a'}, "utf32le": {0xff, 0xfe, 0, 0}, "utf32be": {0, 0, 0xfe, 0xff},
		"invalid": {0xff}, "overlong": {0xc0, 0xaf}, "surrogate": {0xed, 0xa0, 0x80}, "truncated": {0xe4, 0xb8}, "nul": {'x', 0},
		"splitinvalid": append(bytes.Repeat([]byte{'x'}, 4095), 0xe4, 'x'),
	} {
		t.Run(name, func(t *testing.T) {
			expectError(t, New(), context.Background(), TXT, bytes.NewReader(in), int64(len(in)), ErrInvalid)
		})
	}
}

// Relaxing limits, accepting whitespace, or silently returning a prefix breaks this test.
func TestTextExactBounds(t *testing.T) {
	for _, f := range []Format{TXT, Markdown} {
		exact := bytes.Repeat([]byte{'x'}, 128*1024)
		expectText(t, f, exact, exact)
		expectText(t, f, append([]byte{0xef, 0xbb, 0xbf}, exact...), exact)
		over := append(exact, 'x')
		expectError(t, New(), context.Background(), f, bytes.NewReader(over), int64(len(over)), ErrLimit)
	}
	for _, s := range []string{"", " \t\r\n", "\u2003\u3000", "\xef\xbb\xbf"} {
		expectError(t, New(), context.Background(), TXT, strings.NewReader(s), int64(len(s)), ErrEmpty)
	}
	full := bytes.Repeat([]byte{'x'}, 4*1024*1024)
	expectError(t, New(), context.Background(), TXT, bytes.NewReader(full), int64(len(full)), ErrLimit)
}

type hostileReader struct{ mode string }

func (r hostileReader) ReadAt(p []byte, off int64) (int, error) {
	switch r.mode {
	case "short":
		copy(p, "ok")
		return 2, io.EOF
	case "shortnil":
		copy(p, "ok")
		return 2, nil
	case "negative":
		return -1, nil
	case "oversized":
		return len(p) + 1, nil
	case "error":
		copy(p, "ok")
		return len(p), errors.New("PRIVATE-CANARY/path")
	default:
		copy(p, "ok")
		return len(p), io.EOF
	}
}

// Accepting dishonest counts or reader errors as a complete prefix breaks these cases.
func TestParserNoSuccessfulTruncation(t *testing.T) {
	for _, mode := range []string{"short", "shortnil", "negative", "oversized", "error"} {
		t.Run(mode, func(t *testing.T) {
			expectError(t, New(), context.Background(), TXT, hostileReader{mode}, 10, ErrInvalid)
		})
	}
	expectText(t, TXT, []byte("ok"), []byte("ok"))
	got, err := New().Extract(context.Background(), TXT, hostileReader{"fulleof"}, 2)
	if err != nil || got.Text != "ok" {
		t.Fatalf("complete final EOF rejected: %v", err)
	}
}
