package document

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"runtime"
	"strings"
	"testing"
)

// Fixtures serialize ZIP records independently of admission's parsing helpers.
type zipFixtureEntry struct {
	name                       string
	data                       []byte
	method, flags              uint16
	compressed                 []byte
	extra, localExtra, comment []byte
	creator                    uint16
	attrs                      uint32
	descriptor                 int
}
type zipFixtureOffsets struct{ local, central, body, descriptor int }

func put16(b []byte, at int, v uint16) { binary.LittleEndian.PutUint16(b[at:], v) }
func put32(b []byte, at int, v uint32) { binary.LittleEndian.PutUint32(b[at:], v) }
func get32(b []byte, at int) uint32    { return binary.LittleEndian.Uint32(b[at:]) }

func buildZIP(t *testing.T, entries ...zipFixtureEntry) ([]byte, []zipFixtureOffsets) {
	t.Helper()
	var out []byte
	positions := make([]zipFixtureOffsets, len(entries))
	compressed := make([][]byte, len(entries))
	for i, e := range entries {
		compressed[i] = e.compressed
		if compressed[i] == nil {
			compressed[i] = e.data
			if e.method == 8 {
				var b bytes.Buffer
				w, err := flate.NewWriter(&b, flate.DefaultCompression)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write(e.data); err != nil {
					t.Fatal(err)
				}
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
				compressed[i] = b.Bytes()
			}
		}
		positions[i].local = len(out)
		h := make([]byte, 30)
		put32(h, 0, 0x04034b50)
		put16(h, 4, 20)
		put16(h, 6, e.flags)
		put16(h, 8, e.method)
		if e.flags&8 == 0 {
			put32(h, 14, crc32.ChecksumIEEE(e.data))
			put32(h, 18, uint32(len(compressed[i])))
			put32(h, 22, uint32(len(e.data)))
		}
		put16(h, 26, uint16(len(e.name)))
		put16(h, 28, uint16(len(e.localExtra)))
		out = append(out, h...)
		out = append(out, e.name...)
		out = append(out, e.localExtra...)
		positions[i].body = len(out)
		out = append(out, compressed[i]...)
		positions[i].descriptor = len(out)
		if e.flags&8 != 0 {
			if e.descriptor != 12 {
				out = binary.LittleEndian.AppendUint32(out, 0x08074b50)
			}
			out = binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(e.data))
			out = binary.LittleEndian.AppendUint32(out, uint32(len(compressed[i])))
			out = binary.LittleEndian.AppendUint32(out, uint32(len(e.data)))
		}
	}
	centralStart := len(out)
	for i, e := range entries {
		positions[i].central = len(out)
		h := make([]byte, 46)
		put32(h, 0, 0x02014b50)
		put16(h, 4, e.creator)
		put16(h, 6, 20)
		put16(h, 8, e.flags)
		put16(h, 10, e.method)
		put32(h, 16, crc32.ChecksumIEEE(e.data))
		put32(h, 20, uint32(len(compressed[i])))
		put32(h, 24, uint32(len(e.data)))
		put16(h, 28, uint16(len(e.name)))
		put16(h, 30, uint16(len(e.extra)))
		put16(h, 32, uint16(len(e.comment)))
		put32(h, 38, e.attrs)
		put32(h, 42, uint32(positions[i].local))
		out = append(out, h...)
		out = append(out, e.name...)
		out = append(out, e.extra...)
		out = append(out, e.comment...)
	}
	end := make([]byte, 22)
	put32(end, 0, 0x06054b50)
	put16(end, 8, uint16(len(entries)))
	put16(end, 10, uint16(len(entries)))
	put32(end, 12, uint32(len(out)-centralStart))
	put32(end, 16, uint32(centralStart))
	return append(out, end...), positions
}

func expectZIPError(t *testing.T, in []byte, want error) {
	t.Helper()
	m, err := admitZIP(context.Background(), bytes.NewReader(in), int64(len(in)))
	if !errors.Is(err, want) || m != nil {
		t.Fatalf("want safe %v and nil manifest, got %v", want, err)
	}
	if strings.Contains(err.Error(), "PRIVATE-CANARY") {
		t.Fatal("ZIP name leaked")
	}
}

// Removing preallocation gates permits attacker counts and metadata allocation.
func TestZIPMetadataAdmissionBeforeAllocation(t *testing.T) {
	base, _ := buildZIP(t, zipFixtureEntry{name: "word/document.xml", data: []byte("x")})
	for _, tc := range []struct {
		name   string
		mutate func([]byte)
		want   error
	}{
		{"zip64-count", func(b []byte) { put16(b, len(b)-12, 65535) }, ErrUnsupported},
		{"zip64-size", func(b []byte) { put32(b, len(b)-10, 0xffffffff) }, ErrUnsupported},
		{"zip64-offset", func(b []byte) { put32(b, len(b)-6, 0xffffffff) }, ErrUnsupported},
		{"disk", func(b []byte) { put16(b, len(b)-18, 1) }, ErrUnsupported},
		{"count-mismatch", func(b []byte) { put16(b, len(b)-14, 0) }, ErrInvalid},
		{"count-wrap", func(b []byte) { put16(b, len(b)-14, 0); put16(b, len(b)-12, 0) }, ErrInvalid},
		{"central-range", func(b []byte) { put32(b, len(b)-6, 1) }, ErrInvalid},
		{"central-too-large", func(b []byte) { put32(b, len(b)-10, 256*1024+1) }, ErrLimit},
		{"trailing-byte", func(b []byte) { put16(b, len(b)-2, 1) }, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) { b := bytes.Clone(base); tc.mutate(b); expectZIPError(t, b, tc.want) })
	}
	entries := make([]zipFixtureEntry, 257)
	for i := range entries {
		entries[i].name = strings.Repeat("x", i+1)
	}
	tooMany, _ := buildZIP(t, entries...)
	expectZIPError(t, tooMany, ErrLimit)
	ambiguous := bytes.Clone(base)
	put16(ambiguous, len(ambiguous)-2, 22)
	ambiguous = append(ambiguous, base[len(base)-22:]...)
	expectZIPError(t, ambiguous, ErrInvalid)
	prepended := append([]byte("PRIVATE-CANARY"), base...)
	expectZIPError(t, prepended, ErrInvalid)
	for _, tc := range []struct {
		name string
		e    zipFixtureEntry
		want error
	}{
		{"long-name", zipFixtureEntry{name: strings.Repeat("x", 513)}, ErrLimit},
		{"large-extra", zipFixtureEntry{name: "a", extra: make([]byte, 4097)}, ErrLimit},
		{"large-comment", zipFixtureEntry{name: "a", comment: make([]byte, 1025)}, ErrLimit},
		{"zip64-extra", zipFixtureEntry{name: "a", extra: []byte{1, 0, 0, 0}}, ErrUnsupported},
		{"zip64-local-extra", zipFixtureEntry{name: "a", localExtra: []byte{1, 0, 0, 0}}, ErrUnsupported},
		{"broken-extra", zipFixtureEntry{name: "a", extra: []byte{2, 0, 1, 0}}, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) { b, _ := buildZIP(t, tc.e); expectZIPError(t, b, tc.want) })
	}
}

type sparseZIPReader struct {
	size           int64
	regions        map[int64][]byte
	calls, largest int
}

func (r *sparseZIPReader) ReadAt(p []byte, off int64) (int, error) {
	r.calls++
	if len(p) > r.largest {
		r.largest = len(p)
	}
	clear(p)
	for start, b := range r.regions {
		for i := range p {
			at := off + int64(i) - start
			if at >= 0 && at < int64(len(b)) {
				p[i] = b[at]
			}
		}
	}
	return len(p), nil
}

// Sparse fields must refuse without materializing the advertised field or size.
func TestZIPMetadataSparseReadAndAllocationBound(t *testing.T) {
	const size int64 = 4 * 1024 * 1024
	end := make([]byte, 22)
	put32(end, 0, 0x06054b50)
	put16(end, 8, 1)
	put16(end, 10, 1)
	central := size - 22 - (46 + 65535)
	put32(end, 12, 46+65535)
	put32(end, 16, uint32(central))
	h := make([]byte, 46)
	put32(h, 0, 0x02014b50)
	put16(h, 28, 65535)
	r := &sparseZIPReader{size: size, regions: map[int64][]byte{size - 22: end, central: h}}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	m, err := admitZIP(context.Background(), r, size)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrLimit) || m != nil {
		t.Fatalf("oversize sparse name: %v", err)
	}
	if r.calls != 2 || r.largest > 65557 {
		t.Fatalf("metadata read budget calls=%d largest=%d", r.calls, r.largest)
	}
	if after.TotalAlloc-before.TotalAlloc > 1024*1024 {
		t.Fatal("preflight allocated above conservative 1MiB test ceiling")
	}
	t.Logf("sparse preflight: %d reads, largest %d, TotalAlloc delta %d bytes", r.calls, r.largest, after.TotalAlloc-before.TotalAlloc)
	noRead := &countReader{}
	if m, err := admitZIP(context.Background(), noRead, size+1); !errors.Is(err, ErrLimit) || m != nil || noRead.calls.Load() != 0 {
		t.Fatalf("input cap before read: %v", err)
	}
}

// Accepting dangerous names/types/flags broadens the archive trust boundary.
func TestZIPCanonicalNamesTypesAndFlags(t *testing.T) {
	for _, name := range []string{"", "/a", "C:/a", "a\\b", "a//b", "a/../b", "./a", "a/./b", "a\x00b", string([]byte{0xff}), "PRIVATE-CANARY/../a"} {
		t.Run("name", func(t *testing.T) { b, _ := buildZIP(t, zipFixtureEntry{name: name}); expectZIPError(t, b, ErrInvalid) })
	}
	dup, _ := buildZIP(t, zipFixtureEntry{name: "a"}, zipFixtureEntry{name: "a"})
	expectZIPError(t, dup, ErrInvalid)
	for _, attrs := range []uint32{0120777 << 16, 0010777 << 16, 0020777 << 16, 0060777 << 16, 0140777 << 16, 0030777 << 16} {
		b, _ := buildZIP(t, zipFixtureEntry{name: "a", creator: 3 << 8, attrs: attrs})
		expectZIPError(t, b, ErrUnsupported)
	}
	for _, flags := range []uint16{1, 0x10, 0x20, 0x40, 0x100, 0x2000, 2} {
		b, _ := buildZIP(t, zipFixtureEntry{name: "a", flags: flags})
		expectZIPError(t, b, ErrUnsupported)
	}
	b, _ := buildZIP(t, zipFixtureEntry{name: "a", method: 12})
	expectZIPError(t, b, ErrUnsupported)
	b, _ = buildZIP(t, zipFixtureEntry{name: "dir/", data: []byte("x")})
	expectZIPError(t, b, ErrInvalid)
	b, _ = buildZIP(t, zipFixtureEntry{name: "dir", creator: 3 << 8, attrs: 0040755 << 16})
	expectZIPError(t, b, ErrInvalid)
	for _, e := range []zipFixtureEntry{{name: "中.xml", flags: 0x800}, {name: "a", method: 8, flags: 6}, {name: "dir/", creator: 3 << 8, attrs: 0040755 << 16}} {
		b, _ := buildZIP(t, e)
		if m, err := admitZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err != nil || m == nil {
			t.Fatalf("valid canonical archive: %v", err)
		}
	}
}

// Trusting local headers alone or central offsets alone misses disagreements/aliasing.
func TestZIPLocalCentralAndOverlap(t *testing.T) {
	base, p := buildZIP(t, zipFixtureEntry{name: "a", data: []byte("x")}, zipFixtureEntry{name: "b", data: []byte("y")})
	for _, tc := range []struct {
		name   string
		mutate func([]byte)
	}{
		{"local-name", func(b []byte) { b[p[0].local+30] = 'z' }},
		{"local-method", func(b []byte) { put16(b, p[0].local+8, 8) }},
		{"local-flags", func(b []byte) { put16(b, p[0].local+6, 8) }},
		{"local-crc", func(b []byte) { put32(b, p[0].local+14, 0) }},
		{"local-size", func(b []byte) { put32(b, p[0].local+22, 0) }},
		{"local-compressed-size", func(b []byte) { put32(b, p[0].local+18, 0) }},
		{"alias-offset", func(b []byte) { put32(b, p[1].central+42, 0) }},
		{"overlap", func(b []byte) { put32(b, p[0].central+20, 33); put32(b, p[0].local+18, 33) }},
		{"past-central", func(b []byte) { put32(b, p[0].central+42, uint32(p[0].central)) }},
		{"disk-start", func(b []byte) { put16(b, p[0].central+34, 1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bytes.Clone(base)
			tc.mutate(b)
			want := ErrInvalid
			if tc.name == "disk-start" {
				want = ErrUnsupported
			}
			expectZIPError(t, b, want)
		})
	}
	for _, n := range []int{12, 16} {
		b, ps := buildZIP(t, zipFixtureEntry{name: "a", data: []byte("abc"), method: 8, flags: 8, descriptor: n})
		if m, err := admitZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err != nil || m == nil {
			t.Fatalf("valid %d descriptor: %v", n, err)
		}
		for _, delta := range []int{0, 4, 8} {
			bad := bytes.Clone(b)
			at := ps[0].descriptor
			if n == 16 {
				at += 4
			}
			bad[at+delta] ^= 1
			expectZIPError(t, bad, ErrInvalid)
		}
		bad := bytes.Clone(b)
		put32(bad, ps[0].local+14, 123)
		expectZIPError(t, bad, ErrInvalid)
	}
}

type zipFailureReader struct{}

func (zipFailureReader) ReadAt([]byte, int64) (int, error) {
	return 0, errors.New("PRIVATE-CANARY/path/archive")
}

// Reader errors and canceled/invalid entry points must return no manifest or payload.
func TestZIPAdmissionSafeErrorsAndCancel(t *testing.T) {
	for _, tc := range []struct {
		ctx  context.Context
		r    io.ReaderAt
		size int64
		want error
	}{
		{nil, bytes.NewReader(nil), 0, ErrInvalid}, {context.Background(), nil, 0, ErrInvalid},
		{context.Background(), bytes.NewReader(nil), -1, ErrInvalid}, {context.Background(), zipFailureReader{}, 22, ErrInvalid},
	} {
		m, err := admitZIP(tc.ctx, tc.r, tc.size)
		if !errors.Is(err, tc.want) || m != nil || strings.Contains(err.Error(), "PRIVATE-CANARY") {
			t.Fatalf("safe error: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &countReader{}
	if m, err := admitZIP(ctx, r, 22); !errors.Is(err, context.Canceled) || !errors.Is(err, ErrCanceled) || m != nil || r.calls.Load() != 0 {
		t.Fatalf("cancel before read: %v", err)
	}
}

// A directory suffix must not bypass canonical duplicate admission.
func TestZIPFileDirectoryCanonicalDuplicate(t *testing.T) {
	for _, entries := range [][]zipFixtureEntry{
		{{name: "a"}, {name: "a/"}},
		{{name: "a/"}, {name: "a"}},
	} {
		b, _ := buildZIP(t, entries...)
		expectZIPError(t, b, ErrInvalid)
	}
}

// Exact metadata bounds remain usable; a central span above its cap refuses.
func TestZIPExactMetadataBounds(t *testing.T) {
	extra := make([]byte, 4096)
	put16(extra, 0, 2)
	put16(extra, 2, 4092)
	entries := make([]zipFixtureEntry, 256)
	for i := range entries {
		entries[i].name = strings.Repeat("x", i+1)
	}
	for _, fixture := range [][]zipFixtureEntry{
		entries,
		{{name: strings.Repeat("x", 512), extra: extra, localExtra: extra, comment: bytes.Repeat([]byte{'x'}, 1024)}},
	} {
		b, _ := buildZIP(t, fixture...)
		if m, err := admitZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err != nil || m == nil {
			t.Fatalf("exact metadata bound: %v", err)
		}
	}
	// 256 * (46 header + 4 name + 974 comment) == 256KiB exactly.
	for i := range entries {
		entries[i].name = string([]byte{'a', byte('A' + i/26), byte('A' + i%26), 'z'})
		entries[i].comment = bytes.Repeat([]byte{'c'}, 974)
	}
	b, p := buildZIP(t, entries...)
	if len(b)-22-p[0].central != 256*1024 {
		t.Fatal("fixture central boundary is not exactly 256KiB")
	}
	if m, err := admitZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err != nil || m == nil {
		t.Fatalf("exact central bound: %v", err)
	}
	entries[0].comment = append(entries[0].comment, 'c')
	b, _ = buildZIP(t, entries...)
	expectZIPError(t, b, ErrLimit)
	// An EOCD comment at its 65535-byte bound still uses one bounded tail read.
	b, _ = buildZIP(t, zipFixtureEntry{name: "a"})
	put16(b, len(b)-2, 65535)
	b = append(b, bytes.Repeat([]byte{'c'}, 65535)...)
	if m, err := admitZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err != nil || m == nil {
		t.Fatalf("exact EOCD comment bound: %v", err)
	}
}
