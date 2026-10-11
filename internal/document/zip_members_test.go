package document

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func admittedFixture(t *testing.T, b []byte) *zipManifest {
	t.Helper()
	m, err := admitZIP(context.Background(), bytes.NewReader(b), int64(len(b)))
	if err != nil || m == nil {
		t.Fatalf("fixture admission: %v", err)
	}
	return m
}
func expectMemberError(t *testing.T, b []byte, want error) {
	t.Helper()
	m := admittedFixture(t, b)
	got, err := validateZIPMembers(context.Background(), bytes.NewReader(b), m, map[string]bool{"word/document.xml": true})
	if !errors.Is(err, want) || got != nil {
		t.Fatalf("want safe %v and nil selected bytes, got %v", want, err)
	}
	if strings.Contains(err.Error(), "PRIVATE-CANARY") {
		t.Fatal("member error leaked payload/path")
	}
}

// Close-only validation, CRC-zero skips, or buffered compressed over-read miss these corruptions.
func TestZIPCRCZeroAndCompressedExhaustion(t *testing.T) {
	for _, e := range []zipFixtureEntry{{name: "word/document.xml", data: []byte("<doc>中</doc>")}, {name: "word/document.xml", data: []byte("<doc>中</doc>"), method: 8, flags: 0x808}, {name: "word/document.xml"}} {
		b, _ := buildZIP(t, e)
		m := admittedFixture(t, b)
		got, err := validateZIPMembers(context.Background(), bytes.NewReader(b), m, map[string]bool{"word/document.xml": true})
		if err != nil || string(got["word/document.xml"]) != string(e.data) || len(got) != 1 {
			t.Fatalf("complete Store/Deflate/empty validation: %v", err)
		}
	}
	for _, method := range []uint16{0, 8} {
		b, p := buildZIP(t, zipFixtureEntry{name: "word/document.xml", data: []byte("PRIVATE-CANARY"), method: method})
		put32(b, p[0].local+14, 0)
		put32(b, p[0].central+16, 0)
		expectMemberError(t, b, ErrInvalid)
	}
	// Empty decoded output must still consume a valid complete stream and compare CRC.
	b, p := buildZIP(t, zipFixtureEntry{name: "word/document.xml", method: 8})
	put32(b, p[0].local+14, 1)
	put32(b, p[0].central+16, 1)
	expectMemberError(t, b, ErrInvalid)
	for _, raw := range [][]byte{{3, 0, 0}, {3}, {0xff}, {0}} {
		b, _ := buildZIP(t, zipFixtureEntry{name: "word/document.xml", method: 8, compressed: raw})
		expectMemberError(t, b, ErrInvalid)
	}
	b, _ = buildZIP(t, zipFixtureEntry{name: "word/document.xml", data: []byte("x"), compressed: []byte("xy")})
	expectZIPError(t, b, ErrInvalid)
	b, p = buildZIP(t, zipFixtureEntry{name: "word/document.xml", data: []byte("x"), method: 8})
	put32(b, p[0].local+22, 2)
	put32(b, p[0].central+24, 2)
	expectMemberError(t, b, ErrInvalid)
	// A selected valid prefix must be discarded if any unused member has a bad CRC.
	b, p = buildZIP(t, zipFixtureEntry{name: "word/document.xml", data: []byte("ok")}, zipFixtureEntry{name: "unused.bin", data: []byte("bad")})
	put32(b, p[1].local+14, 0)
	put32(b, p[1].central+16, 0)
	expectMemberError(t, b, ErrInvalid)
}

// Omitting unused members from quotas or returning a prefix would accept the bomb.
func TestZIPDecodedAggregateAndCancel(t *testing.T) {
	exact := bytes.Repeat([]byte{'x'}, 4*1024*1024)
	b, _ := buildZIP(t, zipFixtureEntry{name: "word/document.xml", data: exact, method: 8}, zipFixtureEntry{name: "unused.bin", data: exact, method: 8})
	m := admittedFixture(t, b)
	got, err := validateZIPMembers(context.Background(), bytes.NewReader(b), m, map[string]bool{"word/document.xml": true})
	if err != nil || len(got) != 1 || len(got["word/document.xml"]) != 4*1024*1024 || !bytes.Equal(got["word/document.xml"], exact) {
		t.Fatalf("aggregate exactly 8MiB: %v", err)
	}
	tooBig, p := buildZIP(t, zipFixtureEntry{name: "word/document.xml", data: []byte("ok")}, zipFixtureEntry{name: "unused.bin", data: bytes.Repeat([]byte{'x'}, 8*1024*1024), method: 8})
	expectZIPError(t, tooBig, ErrLimit)
	put32(tooBig, p[1].local+22, 1)
	put32(tooBig, p[1].central+24, 1)
	expectMemberError(t, tooBig, ErrLimit)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &countReader{}
	if got, err := validateZIPMembers(ctx, r, m, nil); !errors.Is(err, ErrCanceled) || !errors.Is(err, context.Canceled) || got != nil || r.calls.Load() != 0 {
		t.Fatalf("decode cancel before read: %v", err)
	}
}

// Arbitrary selected keys must refuse before I/O, and no nested member is decoded recursively.
func TestZIPFixedSelectionAndSafeBoundary(t *testing.T) {
	b, _ := buildZIP(t, zipFixtureEntry{name: "[Content_Types].xml", data: []byte("types")}, zipFixtureEntry{name: "_rels/.rels", data: []byte("rels")}, zipFixtureEntry{name: "word/document.xml", data: []byte("doc")}, zipFixtureEntry{name: "nested.zip", data: []byte("not a zip")})
	m := admittedFixture(t, b)
	got, err := validateZIPMembers(context.Background(), bytes.NewReader(b), m, map[string]bool{"[Content_Types].xml": true, "_rels/.rels": true, "word/document.xml": true})
	if err != nil || len(got) != 3 || string(got["[Content_Types].xml"]) != "types" || string(got["_rels/.rels"]) != "rels" || string(got["word/document.xml"]) != "doc" {
		t.Fatalf("fixed parts: %v", err)
	}
	for _, selected := range []map[string]bool{{"PRIVATE-CANARY/path": true}, {"nested.zip": false}} {
		r := &countReader{}
		got, err := validateZIPMembers(context.Background(), r, m, selected)
		if !errors.Is(err, ErrInvalid) || got != nil || r.calls.Load() != 0 || strings.Contains(err.Error(), "PRIVATE-CANARY") {
			t.Fatalf("invalid selected key: %v", err)
		}
	}
	for _, tc := range []struct {
		ctx context.Context
		r   io.ReaderAt
		m   *zipManifest
	}{{nil, bytes.NewReader(b), m}, {context.Background(), nil, m}, {context.Background(), bytes.NewReader(b), nil}} {
		if got, err := validateZIPMembers(tc.ctx, tc.r, tc.m, nil); !errors.Is(err, ErrInvalid) || got != nil {
			t.Fatalf("invalid decode boundary: %v", err)
		}
	}
	if got, err := validateZIPMembers(context.Background(), zipFailureReader{}, m, nil); !errors.Is(err, ErrInvalid) || got != nil || strings.Contains(err.Error(), "PRIVATE-CANARY") {
		t.Fatalf("reader safe class: %v", err)
	}
	changed := bytes.Clone(b)
	changed[30] = 'X'
	if got, err := validateZIPMembers(context.Background(), bytes.NewReader(changed), m, nil); !errors.Is(err, ErrInvalid) || got != nil {
		t.Fatalf("manifest/local crosscheck: %v", err)
	}
}

type heldZIPReader struct {
	input            io.ReaderAt
	at               int64
	entered, release chan struct{}
	once             sync.Once
}

func (r *heldZIPReader) ReadAt(p []byte, off int64) (int, error) {
	if off == r.at && len(p) == 1 {
		r.once.Do(func() { close(r.entered) })
		<-r.release
	}
	return r.input.ReadAt(p, off)
}

type zipValidationResult struct {
	parts map[string][]byte
	err   error
}

// Cancellation cannot detach synchronous compressed input; cleanup must release and join.
func TestZIPHeldDecodeCancellationJoinsActualRead(t *testing.T) {
	b, p := buildZIP(t, zipFixtureEntry{name: "word/document.xml", data: []byte("abc"), method: 8})
	m := admittedFixture(t, b)
	r := &heldZIPReader{input: bytes.NewReader(b), at: int64(p[0].body), entered: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan zipValidationResult, 1)
	var releaseOnce sync.Once
	joined := false
	t.Cleanup(func() {
		cancel()
		releaseOnce.Do(func() { close(r.release) })
		if !joined {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("ZIP decode cleanup did not join")
			}
		}
	})
	go func() {
		parts, err := validateZIPMembers(ctx, r, m, map[string]bool{"word/document.xml": true})
		done <- zipValidationResult{parts, err}
	}()
	select {
	case <-r.entered:
	case got := <-done:
		joined = true
		t.Fatalf("decode did not reach held read: %v", got.err)
	case <-time.After(5 * time.Second):
		t.Fatal("compressed read never entered")
	}
	cancel()
	select {
	case got := <-done:
		joined = true
		t.Fatalf("decode returned while held: %v", got.err)
	default:
	}
	releaseOnce.Do(func() { close(r.release) })
	select {
	case got := <-done:
		joined = true
		if !errors.Is(got.err, ErrCanceled) || !errors.Is(got.err, context.Canceled) || got.parts != nil {
			t.Fatalf("held canceled decode: %v", got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("decode did not join")
	}
}

// Real stdlib-produced descriptors/extras must survive the application's guard.
func TestZIPStandardWriterCompatibleSubset(t *testing.T) {
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	for _, method := range []uint16{zip.Store, zip.Deflate} {
		name := "[Content_Types].xml"
		if method == zip.Deflate {
			name = "word/document.xml"
		}
		h := &zip.FileHeader{Name: name, Method: method}
		h.SetModTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
		member, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write([]byte("valid 中")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.SetComment("synthetic archive"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b := archive.Bytes()
	m := admittedFixture(t, b)
	parts, err := validateZIPMembers(context.Background(), bytes.NewReader(b), m, map[string]bool{"[Content_Types].xml": true, "word/document.xml": true})
	if err != nil || len(parts) != 2 || string(parts["[Content_Types].xml"]) != "valid 中" || string(parts["word/document.xml"]) != "valid 中" {
		t.Fatalf("stdlib-writer subset: %v", err)
	}
}

type observedBodyReader struct {
	input         io.ReaderAt
	body, end     int64
	bodyReadBytes int
}

func (r *observedBodyReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= r.body && off < r.end {
		r.bodyReadBytes += len(p)
	}
	return r.input.ReadAt(p, off)
}

// A nearly exhausted aggregate permits only the remaining byte and one EOF
// probe, rather than another full decoded chunk beyond the shared quota.
func TestZIPDecodeQuotaBoundsNextRead(t *testing.T) {
	b, positions := buildZIP(t, zipFixtureEntry{name: "word/document.xml", data: bytes.Repeat([]byte{'x'}, 8192)})
	m := admittedFixture(t, b)
	r := &observedBodyReader{input: bytes.NewReader(b), body: int64(positions[0].body), end: int64(positions[0].body + 8192)}
	zr, err := zip.NewReader(zipContextReaderAt{ctx: context.Background(), reader: checkedReaderAt{input: r, size: int64(len(b))}}, int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	r.bodyReadBytes = 0
	total := uint64(8*1024*1024 - 1)
	part, err := decodeZIPMember(context.Background(), zr.File[0], &m.members[0], true, &total)
	if !errors.Is(err, ErrLimit) || part != nil {
		t.Fatalf("nearly spent shared quota: %v", err)
	}
	if r.bodyReadBytes != 2 {
		t.Fatalf("quota permitted %d input bytes, want one remaining byte plus one probe", r.bodyReadBytes)
	}
}
