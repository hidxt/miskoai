package media

import (
	"bufio"
	"bytes"
	"compress/lzw"
	"compress/zlib"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func imageFixture(t *testing.T, format string) []byte {
	t.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	m.Set(1, 1, color.NRGBA{R: 17, G: 32, B: 83, A: 255})
	var b bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&b, m)
	} else {
		err = jpeg.Encode(&b, m, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Structural-only fixtures are never passed to the real decoder. Their tiny
// payloads let the per-call seam prove refusal before allocation.
func gifHeader(w, h uint16) []byte {
	b := []byte("GIF89a")
	b = append(b, byte(w), byte(w>>8), byte(h), byte(h>>8), 0x80, 0, 0, 0, 0, 0, 255, 255, 255)
	return b
}
func gifDescriptor(x, y, w, h uint16, interlaced bool) []byte {
	b := []byte{0x2c, byte(x), byte(x >> 8), byte(y), byte(y >> 8), byte(w), byte(w >> 8), byte(h), byte(h >> 8), 0}
	if interlaced {
		b[9] = 0x40
	}
	return append(b, 2, 0)
}
func gifStructure(w, h uint16, frames int, interlaced bool) []byte {
	b := gifHeader(w, h)
	for i := 0; i < frames; i++ {
		b = append(b, gifDescriptor(0, 0, w, h, interlaced)...)
	}
	return append(b, 0x3b)
}
func TestImageGIFInterlacedCumulativeEstimate(t *testing.T) {
	// Each canvas satisfies the old visible-pixel cap. 48M retained pixels
	// fit the old estimate, but the additional 48M interlace buffers do not.
	requireBeforeDecode(t, gifStructure(4000, 4000, 3, true), ErrLimit)
}
func tinyInterlacedGIF(t *testing.T) []byte {
	t.Helper()
	b := gifHeader(2, 5)
	d := gifDescriptor(0, 0, 2, 5, true)
	b = append(b, d[:10]...)
	var compressed bytes.Buffer
	w := lzw.NewWriter(&compressed, lzw.LSB, 2)
	// Genuine interlaced order: rows 0,4,2,1,3, with two palette indexes/row.
	_, err := w.Write([]byte{0, 1, 0, 0, 1, 1, 1, 0, 0, 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	b = append(b, 2, byte(compressed.Len()))
	b = append(b, compressed.Bytes()...)
	return append(b, 0, 0x3b)
}
func TestImageTinyInterlacedGIFCompleteDecode(t *testing.T) {
	b := tinyInterlacedGIF(t)
	g, err := gif.DecodeAll(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(g.Image[0].Pix, []byte{0, 1, 1, 0, 1, 1, 0, 1, 0, 0}) {
		t.Fatalf("interlaced fixture did not decode correct rows: %v", g.Image[0].Pix)
	}
	info, err := ValidateImage(context.Background(), bytes.NewReader(b), int64(len(b)))
	if err != nil || info.EstimatedDecodeBytes != imageFixedBytes+16*1024+20 || info.Width != 2 || info.Height != 5 {
		t.Fatalf("interlaced: %+v %v", info, err)
	}
}

func jpegSegment(marker byte, payload []byte) []byte {
	n := len(payload) + 2
	return append([]byte{255, marker, byte(n >> 8), byte(n)}, payload...)
}

// Independently constructed 1x1 progressive grayscale JPEG: zero DC and one
// AC EOB, one-symbol Huffman tables, and unit quantization. No decoder source.
func tinyProgressiveJPEG() []byte {
	b := jpegHeader(1, 1, 0xc2, 0x11)
	q := append([]byte{0}, bytes.Repeat([]byte{1}, 64)...)
	b = append(b, jpegSegment(0xdb, q)...)
	for _, table := range []byte{0, 0x10} {
		p := make([]byte, 18)
		p[0] = table
		p[1] = 1
		b = append(b, jpegSegment(0xc4, p)...)
	}
	b = append(b, jpegSegment(0xda, []byte{1, 1, 0, 0, 0, 0})...)
	b = append(b, 0x7f)
	b = append(b, jpegSegment(0xda, []byte{1, 1, 0, 1, 63, 0})...)
	return append(b, 0x7f, 255, 217)
}
func TestImageProgressiveCompleteDecode(t *testing.T) {
	b := tinyProgressiveJPEG()
	m, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if m.Bounds() != image.Rect(0, 0, 1, 1) {
		t.Fatal(m.Bounds())
	}
	info, err := ValidateImage(context.Background(), bytes.NewReader(b), int64(len(b)))
	if err != nil || info.EstimatedDecodeBytes != imageFixedBytes+256+4+256 {
		t.Fatalf("progressive: %+v %v", info, err)
	}
}
func tinyPNG16Interlaced(t *testing.T) []byte {
	t.Helper()
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	// 1x1 Adam7 has only pass 0. Filter byte followed by RGBA16 samples.
	if _, err := w.Write([]byte{0, 0, 1, 0, 2, 0, 3, 255, 255}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b := pngHeader(1, 1, 16, 6, 1)
	b = append(b, pngChunk("IDAT", compressed.Bytes())...)
	return append(b, pngChunk("IEND", nil)...)
}
func TestImagePNG16InterlacedCompleteDecode(t *testing.T) {
	b := tinyPNG16Interlaced(t)
	info, err := ValidateImage(context.Background(), bytes.NewReader(b), int64(len(b)))
	if err != nil || info.EstimatedDecodeBytes != imageFixedBytes+16+18 {
		t.Fatalf("16bit interlace: %+v %v", info, err)
	}
}

func TestImageGIFLaterFramesAndMalformedStructures(t *testing.T) {
	cases := []struct {
		name string
		b    []byte
		want error
	}{
		{"cumulative", gifStructure(4000, 4000, 5, false), ErrLimit},
		{"zero", append(gifHeader(2, 2), gifDescriptor(0, 0, 0, 1, false)...), ErrInvalid},
		{"offsetOverflow", append(gifHeader(2, 2), gifDescriptor(65535, 0, 1, 1, false)...), ErrInvalid},
		{"noFrame", append(gifHeader(2, 2), 0x3b), ErrInvalid},
		{"trailing", append(gifFixture(t, 1), 0), ErrInvalid},
		{"extension", append(gifHeader(2, 2), 0x21, 0x01), ErrUnsupported},
		{"truncatedGCT", gifHeader(2, 2)[:17], ErrInvalid},
		{"truncatedSubblock", append(gifHeader(2, 2), 0x21, 0xfe, 255, 1), ErrInvalid},
		{"badGCE", append(gifHeader(2, 2), 0x21, 0xf9, 5, 0, 0, 0, 0, 0), ErrInvalid},
		{"badApplicationSize", append(gifHeader(2, 2), 0x21, 0xff, 255), ErrInvalid},
	}
	b := gifHeader(4000, 4000)
	b = append(b, gifDescriptor(0, 0, 1, 1, false)...)
	for i := 0; i < 5; i++ {
		b = append(b, gifDescriptor(0, 0, 4000, 4000, false)...)
	}
	b = append(b, 0x3b)
	cases = append(cases, struct {
		name string
		b    []byte
		want error
	}{"smallFirstLargeLater", b, ErrLimit})
	b = append(gifHeader(2, 2), gifDescriptor(0, 0, 1, 1, false)...)
	b = append(b, gifDescriptor(0, 0, 65535, 65535, false)...)
	cases = append(cases, struct {
		name string
		b    []byte
		want error
	}{"hugeLater", b, ErrInvalid})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { requireBeforeDecode(t, tc.b, tc.want) })
	}
	for _, b := range [][]byte{gifFixture(t, 1), tinyInterlacedGIF(t)} {
		for cut := 1; cut < len(b); cut++ {
			_, err := ValidateImage(context.Background(), bytes.NewReader(b[:cut]), int64(cut))
			if err == nil {
				t.Fatalf("truncated GIF accepted at %d", cut)
			}
		}
	}
}

func TestImagePNGStructureAndCompressedErrors(t *testing.T) {
	base := imageFixture(t, "png")
	badCRC := bytes.Clone(base)
	badCRC[len(badCRC)-1] ^= 1
	cases := []struct {
		name string
		b    []byte
		want error
	}{
		{"CRC", badCRC, ErrInvalid},
		{"duplicateIHDR", append(bytes.Clone(base[:33]), base[8:]...), ErrInvalid},
		{"firstChunk", append([]byte("\x89PNG\r\n\x1a\n"), pngChunk("IDAT", nil)...), ErrInvalid},
		{"missingIDAT", append(pngHeader(1, 1, 8, 6, 0), pngChunk("IEND", nil)...), ErrInvalid},
		{"trailing", append(bytes.Clone(base), 0), ErrInvalid},
		{"unknownCritical", append(pngHeader(1, 1, 8, 6, 0), pngChunk("ABCD", nil)...), ErrUnsupported},
		{"reservedBit", append(pngHeader(1, 1, 8, 6, 0), pngChunk("aaab", nil)...), ErrInvalid},
		{"hugeChunk", append(pngHeader(1, 1, 8, 6, 0), 255, 255, 255, 255, 'I', 'D', 'A', 'T', 0, 0, 0, 0), ErrInvalid},
	}
	for _, kind := range []string{"acTL", "fcTL", "fdAT"} {
		cases = append(cases, struct {
			name string
			b    []byte
			want error
		}{kind, append(pngHeader(1, 1, 8, 6, 0), pngChunk(kind, nil)...), ErrUnsupported})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { requireBeforeDecode(t, tc.b, tc.want) })
	}
	t.Run("noncontiguousIDAT", func(t *testing.T) {
		b := pngHeader(1, 1, 8, 6, 0)
		for _, kind := range []string{"IDAT", "tEXt", "IDAT", "IEND"} {
			b = append(b, pngChunk(kind, nil)...)
		}
		requireBeforeDecode(t, b, ErrInvalid)
	})
	t.Run("chunkCap", func(t *testing.T) {
		b := pngHeader(1, 1, 8, 6, 0)
		for i := 0; i < 4096; i++ {
			b = append(b, pngChunk("tEXt", nil)...)
		}
		requireBeforeDecode(t, b, ErrLimit)
	})
	// Correct chunk CRC does not make corrupt compressed data valid.
	t.Run("validCRCInvalidZlib", func(t *testing.T) {
		b := pngHeader(1, 1, 8, 6, 0)
		b = append(b, pngChunk("IDAT", []byte("synthetic invalid zlib"))...)
		b = append(b, pngChunk("IEND", nil)...)
		_, err := ValidateImage(context.Background(), bytes.NewReader(b), int64(len(b)))
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("compressed body accepted: %v", err)
		}
	})
	for cut := 1; cut < len(base); cut++ {
		_, err := ValidateImage(context.Background(), bytes.NewReader(base[:cut]), int64(cut))
		if err == nil {
			t.Fatalf("truncated PNG accepted at %d", cut)
		}
	}
}

func TestImageJPEGStructureAndEntropyErrors(t *testing.T) {
	b := imageFixture(t, "jpeg")
	duplicate := append(jpegHeader(1, 1, 0xc0, 0x11), jpegHeader(1, 1, 0xc0, 0x11)[2:]...)
	badID := jpegHeader(1, 1, 0xc0, 0x11, 0x11, 0x11)
	badID[15] = badID[12]
	for _, tc := range []struct {
		name string
		b    []byte
		want error
	}{
		{"duplicate", duplicate, ErrInvalid}, {"componentID", badID, ErrInvalid},
		{"lossless", jpegHeader(1, 1, 0xc3, 0x11), ErrUnsupported},
		{"zeroSampling", jpegHeader(1, 1, 0xc0, 0), ErrUnsupported},
		{"tooManyComponents", jpegHeader(1, 1, 0xc0, 0x11, 0x11), ErrUnsupported},
		{"trailing", append(bytes.Clone(b), 0), ErrInvalid},
		{"shortSegment", []byte{255, 216, 255, 224, 0, 1, 255, 217}, ErrInvalid},
		{"hugeSegment", []byte{255, 216, 255, 224, 255, 255, 255, 217}, ErrInvalid},
		{"noSOF", []byte{255, 216, 255, 217}, ErrInvalid},
		{"earlyRestart", []byte{255, 216, 255, 208, 255, 217}, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) { requireBeforeDecode(t, tc.b, tc.want) })
	}
	t.Run("markerCap", func(t *testing.T) {
		b := []byte{255, 216}
		for i := 0; i < 4096; i++ {
			b = append(b, 255, 254, 0, 2)
		}
		requireBeforeDecode(t, b, ErrLimit)
	})
	t.Run("truncatedEntropyExactEOI", func(t *testing.T) {
		b := tinyProgressiveJPEG()
		start := bytes.Index(b, []byte{255, 218})
		b = append(bytes.Clone(b[:start+10]), 255, 217)
		_, err := ValidateImage(context.Background(), bytes.NewReader(b), int64(len(b)))
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("truncated entropy accepted: %v", err)
		}
	})
	for cut := 1; cut < len(b); cut++ {
		_, err := ValidateImage(context.Background(), bytes.NewReader(b[:cut]), int64(cut))
		if err == nil {
			t.Fatalf("truncated JPEG accepted at %d", cut)
		}
	}
}

type imageFailAt struct {
	data  []byte
	reads int
	after int
}

func (r *imageFailAt) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	if r.reads > r.after {
		return 0, errors.New("synthetic-secret-canary filename body token")
	}
	return bytes.NewReader(r.data).ReadAt(p, off)
}
func TestImageReaderErrorsAreSafe(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		b := imageFixture(t, "png")
		if format == "jpeg" {
			b = imageFixture(t, "jpeg")
		}
		if format == "gif" {
			b = gifFixture(t, 1)
		}
		for _, after := range []int{0, 1} {
			t.Run(format+string(rune('0'+after)), func(t *testing.T) {
				r := &imageFailAt{data: b, after: after}
				info, err := ValidateImage(context.Background(), r, int64(len(b)))
				if !errors.Is(err, ErrIO) || info != (ImageInfo{}) || strings.Contains(err.Error(), "canary") {
					t.Fatalf("unsafe or misclassified IO: %+v %v", info, err)
				}
			})
		}
	}
}

func TestImageArithmeticAndExactBoundaries(t *testing.T) {
	for _, pair := range [][2]int64{{math.MaxInt64, 2}, {-1, 1}, {1, -1}} {
		if _, err := imageMul(pair[0], pair[1]); !errors.Is(err, ErrLimit) {
			t.Fatal("multiply overflow", pair, err)
		}
	}
	if _, err := imageAdd(math.MaxInt64, 1); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if n, err := imageEstimate(imageDecodeLimit-1, 1); err != nil || n != imageDecodeLimit {
		t.Fatal(n, err)
	}
	if _, err := imageEstimate(imageDecodeLimit, 1); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	for _, dim := range [][2]int64{{8192, 1}, {4000, 4000}} {
		if err := imageDimensions(dim[0], dim[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, dim := range [][2]int64{{0, 1}, {1, 0}, {-1, 1}} {
		if !errors.Is(imageDimensions(dim[0], dim[1]), ErrInvalid) {
			t.Fatal(dim)
		}
	}
	if !errors.Is(imageDimensions(8192, 8192), ErrLimit) {
		t.Fatal("pixel cap")
	}
	for _, size := range []int64{0, -1} {
		if _, err := ValidateImage(context.Background(), bytes.NewReader(nil), size); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, err := ValidateImage(nil, bytes.NewReader(nil), 1); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := ValidateImage(context.Background(), nil, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	// Exact input cap can carry inert comment padding, with valid image decoding.
	base := imageFixture(t, "png")
	padding := make([]byte, maxPlaintext-len(base)-12)
	b := append(bytes.Clone(base[:33]), pngChunk("tEXt", padding)...)
	b = append(b, base[33:]...)
	if info, err := ValidateImage(context.Background(), bytes.NewReader(b), int64(len(b))); err != nil || info.Width != 3 {
		t.Fatalf("exact4MiB: %+v %v", info, err)
	}
}

func TestImageMetadataMustMatchDecoder(t *testing.T) {
	b := imageFixture(t, "png")
	for _, actual := range []ImageInfo{{Format: "jpeg", Width: 3, Height: 2, Frames: 1}, {Format: "png", Width: 4, Height: 2, Frames: 1}, {Format: "png", Width: 3, Height: 2, Frames: 2}} {
		info, err := validateImage(context.Background(), bytes.NewReader(b), int64(len(b)), func(string, io.Reader) (ImageInfo, error) { return actual, nil })
		if !errors.Is(err, ErrInvalid) || info != (ImageInfo{}) {
			t.Fatal(info, err)
		}
	}
}

func inspectImageFixture(b []byte, scan func(*imageScanner) (ImageInfo, error)) (ImageInfo, error) {
	return scan(&imageScanner{ctx: context.Background(), r: bufio.NewReaderSize(bytes.NewReader(b), 4096), remaining: int64(len(b))})
}
func jpegStructuralEnd(b []byte) []byte {
	b = append(b, jpegSegment(0xda, []byte{1, 1, 0, 0, 63, 0})...)
	return append(b, 0x7f, 255, 217)
}
func TestImageExactStructuralEstimates(t *testing.T) {
	// PNG has two-byte rounding residue, so its closest dimension boundary
	// is the greatest admitted row count rather than exactly64MiB.
	const w int64 = 8192
	h := (imageDecodeLimit - imageFixedBytes - 2*(8*w+1)) / (16 * w)
	for _, height := range []int64{h, h + 1} {
		b := pngHeader(uint32(w), uint32(height), 16, 6, 1)
		b = append(b, pngChunk("IDAT", nil)...)
		b = append(b, pngChunk("IEND", nil)...)
		info, err := inspectImageFixture(b, scanPNG)
		if height == h {
			want := 16*w*h + 2*(8*w+1) + imageFixedBytes
			if err != nil || info.EstimatedDecodeBytes != want {
				t.Fatalf("PNG boundary: %+v %v want%d", info, err, want)
			}
		} else if !errors.Is(err, ErrLimit) {
			t.Fatal("PNG overboundary", err)
		}
	}
	for _, tc := range []struct {
		name     string
		w, h     uint16
		marker   byte
		sampling []byte
		want     int64
	}{
		{"baselineRounded", 33, 17, 0xc0, []byte{0x22, 0x11, 0x11}, 4*48*32 + 4*33*17 + imageFixedBytes},
		{"flexMaxNotLuma", 33, 17, 0xc2, []byte{0x11, 0x41, 0x14}, 4*64*32 + 4*33*17 + 256*2*1*9 + imageFixedBytes},
		{"fourComponents", 33, 17, 0xc2, []byte{0x22, 0x11, 0x11, 0x22}, 4*48*32 + 4*33*17 + 256*3*2*10 + imageFixedBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info, err := inspectImageFixture(jpegStructuralEnd(jpegHeader(tc.w, tc.h, tc.marker, tc.sampling...)), scanJPEG)
			if err != nil || info.EstimatedDecodeBytes != tc.want {
				t.Fatalf("JPEG estimate: %+v %v want%d", info, err, tc.want)
			}
		})
	}
	// Exactly64MiB with five bounded descriptors and real cumulative pixels;
	// seam-free structural inspection allocates no attacker-sized images.
	remaining := imageDecodeLimit - imageFixedBytes - 5*(16<<10)
	b := gifHeader(4000, 4000)
	for i := 0; i < 4; i++ {
		b = append(b, gifDescriptor(0, 0, 4000, 4000, false)...)
		remaining -= 16000000
	}
	if remaining <= 0 || remaining > 16000000 || remaining%1024 != 0 {
		t.Fatal("fixture arithmetic", remaining)
	}
	b = append(b, gifDescriptor(0, 0, 1024, uint16(remaining/1024), false)...)
	b = append(b, 0x3b)
	info, err := inspectImageFixture(b, scanGIF)
	if err != nil || info.EstimatedDecodeBytes != imageDecodeLimit {
		t.Fatalf("GIF exactboundary: %+v %v", info, err)
	}
	// One additional frame makes fixed per-frame overhead exceed the cap.
	b = append(b[:len(b)-1], gifDescriptor(0, 0, 1, 1, false)...)
	b = append(b, 0x3b)
	requireBeforeDecode(t, b, ErrLimit)
}

func TestImageEntropyFramingAndFullDecode(t *testing.T) {
	b := jpegHeader(1, 1, 0xc2, 0x11)
	b = append(b, jpegSegment(0xda, []byte{1, 1, 0, 0, 0, 0})...)
	b = append(b, 0x01, 255, 0, 0x02, 255, 208, 0x03, 255, 255, 217)
	if _, err := inspectImageFixture(b, scanJPEG); err != nil {
		t.Fatal("stuffed/restart/fill framing", err)
	}
	// Structurally framed bytes still need tables and compressed validity.
	if _, err := ValidateImage(context.Background(), bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("framing-only JPEG accepted")
	}
	for _, b := range [][]byte{gifStructure(1, 1, 1, false), append(pngHeader(1, 1, 8, 6, 0), append(pngChunk("IDAT", nil), pngChunk("IEND", nil)...)...)} {
		if _, err := ValidateImage(context.Background(), bytes.NewReader(b), int64(len(b))); !errors.Is(err, ErrInvalid) {
			t.Fatalf("empty compressed stream accepted: %v", err)
		}
	}
}

type imageHeldAt struct {
	input            io.ReaderAt
	entered, release chan struct{}
	reads            int
}

func (r *imageHeldAt) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	if r.reads == 2 {
		close(r.entered)
		<-r.release
	}
	return r.input.ReadAt(p, off)
}
func TestImageBlockedReadCancellationJoins(t *testing.T) {
	b := imageFixture(t, "png")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &imageHeldAt{input: bytes.NewReader(b), entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() { _, err := ValidateImage(ctx, r, int64(len(b))); done <- err }()
	select {
	case <-r.entered:
	case <-done:
		t.Fatal("held decode read not entered")
	case <-time.After(time.Second):
		t.Fatal("entry timeout")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned while ReaderAt still blocked")
	case <-time.After(20 * time.Millisecond):
	}
	close(r.release)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("read lifetime not joined")
	}
}

func TestImageLateCancellationAndDeadline(t *testing.T) {
	b := imageFixture(t, "png")
	ctx, cancel := context.WithCancel(context.Background())
	info, err := validateImage(ctx, bytes.NewReader(b), int64(len(b)), func(format string, r io.Reader) (ImageInfo, error) {
		actual, e := decodeImage(format, r)
		cancel()
		return actual, e
	})
	if !errors.Is(err, context.Canceled) || info != (ImageInfo{}) {
		t.Fatal(info, err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = ValidateImage(ctx, bytes.NewReader(b), int64(len(b)))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	s := &imageScanner{ctx: ctx, r: bufio.NewReader(bytes.NewReader(b)), remaining: int64(len(b))}
	if err = s.read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal("buffered scanner ignored cancellation", err)
	}
}

func TestImageFiniteAdversarialMutations(t *testing.T) {
	for _, b := range [][]byte{imageFixture(t, "png"), imageFixture(t, "jpeg"), gifFixture(t, 2), tinyInterlacedGIF(t), tinyProgressiveJPEG(), tinyPNG16Interlaced(t)} {
		for i := range b {
			mutated := bytes.Clone(b)
			mutated[i] ^= 0xff
			info, err := validateImage(context.Background(), bytes.NewReader(mutated), int64(len(mutated)), func(string, io.Reader) (ImageInfo, error) { return ImageInfo{}, ErrInvalid })
			if err == nil || info != (ImageInfo{}) {
				t.Fatal("mutation yielded partial acceptance")
			}
		}
	}
}

func TestImageCallerFileOwnership(t *testing.T) {
	b := imageFixture(t, "png")
	p := filepath.Join(t.TempDir(), "synthetic-only.png")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.Seek(7, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	original, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ValidateImage(context.Background(), f, int64(len(b))); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = ValidateImage(ctx, f, int64(len(b))); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil || pos != 7 {
		t.Fatalf("caller offset changed: %d %v", pos, err)
	}
	after, err := os.Stat(p)
	if err != nil || !os.SameFile(original, after) {
		t.Fatal("caller file removed or replaced")
	}
	got, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(got, b) {
		t.Fatal("caller data changed")
	}
}

func FuzzImageValidation(f *testing.F) {
	for _, b := range [][]byte{nil, []byte("GIF89a"), pngHeader(4000, 4000, 16, 6, 1), tinyProgressiveJPEG(), gifStructure(4000, 4000, 3, true), {255, 216, 255, 224, 255, 255}, {137, 80, 78, 71, 13, 10, 26, 10}} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 8192 {
			t.Skip("finite fuzz input bound")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		// The decoder seam deliberately refuses, so arbitrary mutation never
		// allocates attacker-sized pixels while structural scanners are fuzzed.
		info, err := validateImage(ctx, bytes.NewReader(b), int64(len(b)), func(string, io.Reader) (ImageInfo, error) { return ImageInfo{}, ErrInvalid })
		if err == nil || info != (ImageInfo{}) {
			t.Fatal("unexpected partial success")
		}
		if !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrLimit) && !errors.Is(err, ErrUnsupported) && !errors.Is(err, ErrIO) && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("non-fixed error", err)
		}
	})
}

func pngChunk(kind string, data []byte) []byte {
	b := make([]byte, len(data)+12)
	binary.BigEndian.PutUint32(b, uint32(len(data)))
	copy(b[4:], kind)
	copy(b[8:], data)
	binary.BigEndian.PutUint32(b[8+len(data):], crc32.ChecksumIEEE(b[4:8+len(data)]))
	return b
}
func pngHeader(w, h uint32, depth, color, interlace byte) []byte {
	hdr := make([]byte, 13)
	binary.BigEndian.PutUint32(hdr, w)
	binary.BigEndian.PutUint32(hdr[4:], h)
	hdr[8] = depth
	hdr[9] = color
	hdr[12] = interlace
	return append([]byte("\x89PNG\r\n\x1a\n"), pngChunk("IHDR", hdr)...)
}
func jpegHeader(w, h uint16, marker byte, sampling ...byte) []byte {
	p := []byte{8, byte(h >> 8), byte(h), byte(w >> 8), byte(w), byte(len(sampling))}
	for i, s := range sampling {
		p = append(p, byte(i+1), s, 0)
	}
	n := len(p) + 2
	return append([]byte{255, 216, 255, marker, byte(n >> 8), byte(n)}, p...)
}
func requireBeforeDecode(t *testing.T, data []byte, want error) {
	t.Helper()
	called := false
	info, err := validateImage(context.Background(), bytes.NewReader(data), int64(len(data)), func(string, io.Reader) (ImageInfo, error) { called = true; return ImageInfo{}, nil })
	if called || !errors.Is(err, want) || info != (ImageInfo{}) {
		t.Fatalf("predecode: called=%v info=%+v err=%v want=%v", called, info, err, want)
	}
}
func TestImageProgressiveSamplingEstimate(t *testing.T) {
	for _, s := range [][]byte{{0x11}, {0x11, 0x41, 0x14}, {0x11, 0x11, 0x11, 0x11}} {
		requireBeforeDecode(t, jpegHeader(4000, 4000, 0xc2, s...), ErrLimit)
	}
}
func TestImagePNG16BitInterlaceEstimate(t *testing.T) {
	requireBeforeDecode(t, pngHeader(4000, 4000, 16, 6, 1), ErrLimit)
	apng := append(pngHeader(1, 1, 8, 6, 0), pngChunk("acTL", make([]byte, 8))...)
	requireBeforeDecode(t, apng, ErrUnsupported)
}
func gifFixture(t *testing.T, n int) []byte {
	t.Helper()
	g := &gif.GIF{Config: image.Config{Width: 2, Height: 2, ColorModel: color.Palette{color.Black, color.White}}}
	for i := 0; i < n; i++ {
		g.Image = append(g.Image, image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White}))
		g.Delay = append(g.Delay, 1)
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestImageGIFAllFramesBounded(t *testing.T) {
	for _, n := range []int{1, 2, 32} {
		b := gifFixture(t, n)
		info, err := ValidateImage(context.Background(), bytes.NewReader(b), int64(len(b)))
		if err != nil || info.Frames != n || info.Animated != (n > 1) {
			t.Fatalf("frames %d: %+v %v", n, info, err)
		}
	}
	requireBeforeDecode(t, gifFixture(t, 33), ErrLimit)
}
func TestImageCheckedArithmeticAndLimits(t *testing.T) {
	_, err := ValidateImage(context.Background(), bytes.NewReader(nil), maxPlaintext+1)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("input cap: %v", err)
	}
	requireBeforeDecode(t, pngHeader(8193, 1, 8, 6, 0), ErrLimit)
	requireBeforeDecode(t, pngHeader(4001, 4000, 8, 6, 0), ErrLimit)
}
func TestImageCancellationAndOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := imageFixture(t, "png")
	_, err := ValidateImage(ctx, bytes.NewReader(b), int64(len(b)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, e := validateImage(ctx, bytes.NewReader(b), int64(len(b)), func(_ string, r io.Reader) (ImageInfo, error) {
			close(entered)
			<-release
			_, e := io.Copy(io.Discard, r)
			return ImageInfo{}, e
		})
		done <- e
	}()
	select {
	case <-entered:
	case <-done:
		t.Fatal("decoder never entered")
	case <-time.After(time.Second):
		t.Fatal("decoder entry timeout")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned before actual decoder joined")
	default:
	}
	close(release)
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("decoder did not join")
	}
}

func TestImagePNGAndJPEGCompleteValidity(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			data := imageFixture(t, format)
			info, err := ValidateImage(context.Background(), bytes.NewReader(data), int64(len(data)))
			if err != nil || info.Format != format || info.Width != 3 || info.Height != 2 || info.Frames != 1 || info.Animated {
				t.Fatalf("complete valid image refused: info=%+v err=%v", info, err)
			}
		})
	}
}
