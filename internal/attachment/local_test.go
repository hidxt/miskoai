package attachment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/document"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/provider"
)

type localInputFunc func([]byte) (int, error)

func (f localInputFunc) Read(b []byte) (int, error) { return f(b) }
func localProcessor(t *testing.T) *ImageProcessor {
	t.Helper()
	p := processor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
		t.Error("local downloaded")
		return nil, ErrDownload
	})
	p.dataDir = filepath.Join(t.TempDir(), "private")
	p.parser = document.New()
	return p
}
func TestLocalImageFormatsDigestAndCleanup(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			p := localProcessor(t)
			raw := tinyImage(t, format)
			sum := sha256.Sum256(raw)
			calls := 0
			var borrowed Artifact
			e := p.WithLocalImage(context.Background(), bytes.NewReader(raw), int64(len(raw)), func(info LocalImage, a Artifact) error {
				calls++
				borrowed = a
				if info.Info.Format != format || info.Info.Width != 2 || info.Info.Height != 2 || info.Info.Frames != 1 || info.Bytes != int64(len(raw)) || info.SHA256 != hex.EncodeToString(sum[:]) {
					t.Fatalf("metadata %+v", info)
				}
				got := make([]byte, len(raw))
				if n, e := a.ReadAt(got, 0); n != len(got) || e != nil || !bytes.Equal(got, raw) {
					t.Fatal("borrowed bytes")
				}
				return nil
			})
			if e != nil || calls != 1 {
				t.Fatalf("local %v calls%d", e, calls)
			}
			if _, e = borrowed.ReadAt(make([]byte, 1), 0); !errors.Is(e, media.ErrIO) {
				t.Fatal("borrowed artifact retained")
			}
			entries, e := os.ReadDir(p.dataDir)
			if e != nil || len(entries) != 0 {
				t.Fatal("spool remains")
			}
		})
	}
}
func TestLocalImageValidationAndRedaction(t *testing.T) {
	for _, name := range []string{"unsupported", "trailing-image", "corrupt", "truncated-stream", "oversized", "reader-canary", "consumer-canary", "cancel-consumer"} {
		t.Run(name, func(t *testing.T) {
			p := localProcessor(t)
			raw := tinyImage(t, "png")
			want := media.ErrInvalid
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			switch name {
			case "unsupported":
				raw = []byte("not image")
				want = media.ErrUnsupported
			case "trailing-image":
				raw = append(raw, 1)
			case "corrupt":
				raw[len(raw)/2] ^= 1
			case "oversized":
				want = media.ErrLimit
			case "reader-canary":
				want = media.ErrIO
			case "consumer-canary":
				want = ErrConsume
			case "cancel-consumer":
				want = context.Canceled
			}
			input := io.Reader(bytes.NewReader(raw))
			size := int64(len(raw))
			if name == "truncated-stream" {
				size++
			}
			if name == "oversized" {
				size = 4<<20 + 1
			}
			if name == "reader-canary" {
				input = localInputFunc(func([]byte) (int, error) { return 0, errors.New("RAW-LOCAL-CANARY") })
			}
			e := p.WithLocalImage(ctx, input, size, func(LocalImage, Artifact) error {
				calls++
				if name == "cancel-consumer" {
					cancel()
					return nil
				}
				return errors.New("RAW-LOCAL-CANARY")
			})
			if !errors.Is(e, want) || strings.Contains(e.Error(), "CANARY") {
				t.Fatalf("error %v want %v", e, want)
			}
			if name != "consumer-canary" && name != "cancel-consumer" && calls != 0 {
				t.Fatal("invalid consumed")
			}
			if files, e := os.ReadDir(p.dataDir); e == nil && len(files) != 0 {
				t.Fatal("partial spool remains")
			}
		})
	}
}

func TestLocalImageFullSizeDigestAndAnimation(t *testing.T) {
	for _, name := range []string{"png4MiB", "gif32", "gif33"} {
		t.Run(name, func(t *testing.T) {
			var raw []byte
			frames := 1
			if name == "png4MiB" {
				base := tinyImage(t, "png")
				padding := make([]byte, (4<<20)-len(base)-12)
				for i := range padding {
					padding[i] = byte(i)
				}
				chunk := make([]byte, 12+len(padding))
				binary.BigEndian.PutUint32(chunk, uint32(len(padding)))
				copy(chunk[4:], "tEXt")
				copy(chunk[8:], padding)
				binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
				raw = append(bytes.Clone(base[:33]), chunk...)
				raw = append(raw, base[33:]...)
			} else {
				frames = 32
				if name == "gif33" {
					frames = 33
				}
				g := gif.GIF{Config: image.Config{ColorModel: color.Palette{color.Black, color.White}, Width: 2, Height: 2}}
				for i := 0; i < frames; i++ {
					g.Image = append(g.Image, image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White}))
					g.Delay = append(g.Delay, 1)
				}
				var b bytes.Buffer
				if e := gif.EncodeAll(&b, &g); e != nil {
					t.Fatal(e)
				}
				raw = b.Bytes()
			}
			sum := sha256.Sum256(raw)
			calls := 0
			p := localProcessor(t)
			e := p.WithLocalImage(context.Background(), bytes.NewReader(raw), int64(len(raw)), func(info LocalImage, a Artifact) error {
				calls++
				if info.SHA256 != hex.EncodeToString(sum[:]) || info.Bytes != int64(len(raw)) || info.Info.Frames != frames || info.Info.Animated != (frames > 1) {
					t.Fatalf("full metadata %+v", info)
				}
				return nil
			})
			if name == "gif33" {
				if !errors.Is(e, media.ErrLimit) || calls != 0 {
					t.Fatalf("frame bound %v calls%d", e, calls)
				}
			} else if e != nil || calls != 1 {
				t.Fatalf("full content %v calls%d", e, calls)
			}
		})
	}
}

func TestLocalImagePreflightNoEffects(t *testing.T) {
	for _, name := range []string{"nil-context", "nil-input", "nil-consumer", "nil-processor", "zero-processor", "zero-size", "negative-size", "oversized", "canceled"} {
		t.Run(name, func(t *testing.T) {
			p := localProcessor(t)
			ctx := context.Background()
			size := int64(1)
			want := ErrInvalid
			input := io.Reader(localInputFunc(func([]byte) (int, error) { t.Fatal("preflight read"); return 0, io.EOF }))
			consume := func(LocalImage, Artifact) error { t.Fatal("preflight consumer"); return nil }
			switch name {
			case "nil-context":
				ctx = nil
			case "nil-input":
				input = nil
			case "nil-consumer":
				consume = nil
			case "nil-processor":
				p = nil
			case "zero-processor":
				p = &ImageProcessor{}
			case "zero-size":
				size = 0
				want = media.ErrInvalid
			case "negative-size":
				size = -1
				want = media.ErrInvalid
			case "oversized":
				size = 4<<20 + 1
				want = media.ErrLimit
			case "canceled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
				want = context.Canceled
			}
			if e := p.WithLocalImage(ctx, input, size, consume); !errors.Is(e, want) {
				t.Fatalf("preflight %v want %v", e, want)
			}
		})
	}
}

func TestLocalImageTransferredArtifactFailures(t *testing.T) {
	for _, name := range []string{"stage-error", "nil-artifact", "size-mismatch", "validate-reader-error", "hash-reader-error", "close-error"} {
		t.Run(name, func(t *testing.T) {
			raw := tinyImage(t, "png")
			a, _ := completed(t, raw)
			w := &wrappedArtifact{Artifact: a}
			p := localProcessor(t)
			want := media.ErrIO
			calls := 0
			if name == "size-mismatch" {
				size := int64(1)
				w.sizeOverride = &size
				want = media.ErrInvalid
			}
			if name == "validate-reader-error" {
				w.readFn = func([]byte, int64) (int, error) { return 0, errors.New("RAW-LOCAL-CANARY") }
			}
			if name == "hash-reader-error" {
				reads := 0
				w.readFn = func(b []byte, o int64) (int, error) { reads++; return a.ReadAt(b, o) }
				if _, e := media.ValidateImage(context.Background(), w, int64(len(raw))); e != nil {
					t.Fatal(e)
				}
				validationReads := reads
				reads = 0
				w.readFn = func(b []byte, o int64) (int, error) {
					reads++
					if reads > validationReads {
						return 0, errors.New("RAW-LOCAL-CANARY")
					}
					return a.ReadAt(b, o)
				}
			}
			if name == "close-error" {
				want = ErrCleanup
				w.closeFn = func() error { _ = a.Close(); return errors.New("RAW-LOCAL-CANARY") }
			}
			e := p.withLocalImage(context.Background(), bytes.NewReader(raw), int64(len(raw)), func(LocalImage, Artifact) error { calls++; return nil }, func(context.Context, string, io.Reader, int64) (Artifact, error) {
				if name == "stage-error" {
					return w, errors.New("RAW-LOCAL-CANARY")
				}
				if name == "nil-artifact" {
					return nil, nil
				}
				return w, nil
			})
			if !errors.Is(e, want) || strings.Contains(e.Error(), "CANARY") {
				t.Fatalf("failure %v want %v", e, want)
			}
			expectedCalls := 0
			if name == "close-error" {
				expectedCalls = 1
			}
			if calls != expectedCalls {
				t.Fatalf("consumer %d", calls)
			}
			expectedCloses := int32(1)
			if name == "nil-artifact" {
				expectedCloses = 0
			}
			if w.closes.Load() != expectedCloses {
				t.Fatalf("closes %d", w.closes.Load())
			}
		})
	}
}

// A successor of each supported kind must wait throughout the same admission.
func localSuccessorsBlocked(t *testing.T, p *ImageProcessor) {
	t.Helper()
	for _, kind := range []string{"local", "image", "document"} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		var e error
		switch kind {
		case "local":
			e = p.WithLocalImage(ctx, localInputFunc(func([]byte) (int, error) { t.Error("successor read"); return 0, io.EOF }), 1, func(LocalImage, Artifact) error { t.Error("successor consumed"); return nil })
		case "image":
			e = p.WithImage(ctx, descriptor(), func(provider.ContentPart) error { t.Error("successor image"); return nil })
		case "document":
			e = p.WithDocument(ctx, fileDescriptor("a.txt"), func(document.Result) error { t.Error("successor document"); return nil })
		}
		cancel()
		if !errors.Is(e, context.DeadlineExceeded) {
			t.Fatalf("%s entered early: %v", kind, e)
		}
	}
}
func TestLocalImageReaderConsumerAndCloseJoin(t *testing.T) {
	for _, stage := range []string{"reader", "consumer", "close", "error-artifact-close"} {
		t.Run(stage, func(t *testing.T) {
			p := localProcessor(t)
			raw := tinyImage(t, "png")
			ctx, cancel := context.WithCancel(context.Background())
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			var once sync.Once
			t.Cleanup(func() {
				cancel()
				once.Do(func() { close(release) })
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("worker not joined")
				}
			})
			input := io.Reader(bytes.NewReader(raw))
			consume := func(LocalImage, Artifact) error { return nil }
			if stage == "reader" {
				r := bytes.NewReader(raw)
				input = localInputFunc(func(b []byte) (int, error) { close(entered); <-release; return r.Read(b) })
			}
			if stage == "consumer" {
				consume = func(LocalImage, Artifact) error { close(entered); <-release; return nil }
			}
			if stage == "close" || stage == "error-artifact-close" {
				a, _ := completed(t, raw)
				w := &wrappedArtifact{Artifact: a, closeFn: func() error { close(entered); <-release; _ = a.Close(); return errors.New("RAW-CLOSE-CANARY") }}
				go func() {
					done <- p.withLocalImage(ctx, input, int64(len(raw)), consume, func(context.Context, string, io.Reader, int64) (Artifact, error) {
						if stage == "error-artifact-close" {
							return w, errors.New("RAW-STAGE-CANARY")
						}
						return w, nil
					})
				}()
			} else {
				go func() { done <- p.WithLocalImage(ctx, input, int64(len(raw)), consume) }()
			}
			select {
			case <-entered:
			case e := <-done:
				done <- e
				t.Fatalf("returned before entering %s: %v", stage, e)
			case <-time.After(2 * time.Second):
				t.Fatal("operation did not enter")
			}
			localSuccessorsBlocked(t, p)
			cancel()
			localSuccessorsBlocked(t, p)
			select {
			case e := <-done:
				done <- e
				t.Fatal("returned before actual operation joined")
			default:
			}
			once.Do(func() { close(release) })
			select {
			case e := <-done:
				if !errors.Is(e, context.Canceled) {
					t.Fatalf("cancel %v", e)
				}
				if strings.Contains(e.Error(), "CANARY") {
					t.Fatal("raw error")
				}
				if (stage == "close" || stage == "error-artifact-close") && !errors.Is(e, ErrCleanup) {
					t.Fatal("cleanup failure lost")
				}
				done <- e
			case <-time.After(2 * time.Second):
				t.Fatal("join")
			}
			if e := p.WithLocalImage(context.Background(), bytes.NewReader(raw), int64(len(raw)), func(LocalImage, Artifact) error { return nil }); e != nil {
				t.Fatalf("slot reuse %v", e)
			}
		})
	}
}
