package attachment

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/netx"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

func descriptor() storage.Attachment {
	return storage.Attachment{Kind: "image", Query: "synthetic-query", ImageHex: strings.Repeat("0", 32)}
}

func tinyImage(t *testing.T, format string) []byte {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, 2, 2))
	m.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&b, m)
	case "jpeg":
		err = jpeg.Encode(&b, m, nil)
	case "gif":
		err = gif.Encode(&b, m, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func completed(t *testing.T, raw []byte) (*media.Artifact, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	var key [16]byte
	cipher, err := media.Encrypt(context.Background(), dir, bytes.NewReader(raw), key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cipher.Close() })
	a, err := media.Decrypt(context.Background(), dir, ioReader(cipher), key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if err = cipher.Close(); err != nil {
		t.Fatal(err)
	}
	return a, dir
}

func ioReader(a Artifact) *bytes.Reader {
	// Fixture preparation only: production never copies whole plaintext.
	b := make([]byte, a.Size())
	if n, _ := a.ReadAt(b, 0); n != len(b) {
		panic("fixture artifact read")
	}
	return bytes.NewReader(b)
}

type wrappedArtifact struct {
	Artifact
	closes       atomic.Int32
	closeFn      func() error
	readFn       func([]byte, int64) (int, error)
	sizeOverride *int64
}

func (a *wrappedArtifact) Close() error {
	a.closes.Add(1)
	if a.closeFn != nil {
		return a.closeFn()
	}
	return a.Artifact.Close()
}
func (a *wrappedArtifact) ReadAt(p []byte, o int64) (int, error) {
	if a.readFn != nil {
		return a.readFn(p, o)
	}
	return a.Artifact.ReadAt(p, o)
}
func (a *wrappedArtifact) Size() int64 {
	if a.sizeOverride != nil {
		return *a.sizeOverride
	}
	return a.Artifact.Size()
}

func processor(t *testing.T, d imageDownloader) *ImageProcessor {
	t.Helper()
	p, e := newImageProcessor("synthetic-admitted-dir", d)
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestImageFormatsAndPrivateCleanup(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			raw := tinyImage(t, format)
			a, dir := completed(t, raw)
			w := &wrappedArtifact{Artifact: a}
			calls := 0
			p := processor(t, func(_ context.Context, d, q, u string, k [16]byte) (Artifact, error) {
				if d != "synthetic-admitted-dir" || q != descriptor().Query || u != "" || k != [16]byte{} {
					t.Error("adapter arguments")
				}
				return w, nil
			})
			err := p.WithImage(context.Background(), descriptor(), func(part provider.ContentPart) error {
				calls++
				if part.Type != "image_url" || part.Text != "" || part.ImageURL == nil || part.ImageURL.Detail != "low" {
					t.Error("noncanonical part")
					return nil
				}
				prefix := "data:image/" + format + ";base64,"
				if !strings.HasPrefix(part.ImageURL.URL, prefix) {
					t.Error("format")
				}
				decoded, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(part.ImageURL.URL, prefix))
				if e != nil || !bytes.Equal(decoded, raw) {
					t.Error("bytes")
				}
				return nil
			})
			if err != nil || calls != 1 || w.closes.Load() != 1 {
				t.Fatalf("err=%v calls=%d closes=%d", err, calls, w.closes.Load())
			}
			entries, e := os.ReadDir(dir)
			if e != nil || len(entries) != 0 {
				t.Fatalf("private spool remains: %v count=%d", e, len(entries))
			}
		})
	}
}

func TestImagePreflightNoEffects(t *testing.T) {
	for _, name := range []string{"invalid-kind", "unsupported", "file", "missing-query", "bad-key", "preferred-no-fallback", "image-metadata", "nil-context", "nil-consumer", "canceled"} {
		t.Run(name, func(t *testing.T) {
			d := descriptor()
			ctx := context.Background()
			consume := func(provider.ContentPart) error { t.Error("consumer effect"); return nil }
			switch name {
			case "invalid-kind":
				d.Kind = "other"
			case "unsupported":
				d = storage.Attachment{Kind: "unsupported"}
			case "file":
				d.Kind = "file"
				d.ImageHex = ""
				d.MediaKey = base64.StdEncoding.EncodeToString(make([]byte, 16))
			case "missing-query":
				d.Query = ""
			case "bad-key":
				d.ImageHex = "bad"
			case "preferred-no-fallback":
				d.ImageHex = "bad"
				d.MediaKey = base64.StdEncoding.EncodeToString(make([]byte, 16))
			case "image-metadata":
				d.FileName = "private-name"
			case "nil-context":
				ctx = nil
			case "nil-consumer":
				consume = nil
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			p := processor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
				t.Error("download effect")
				return nil, nil
			})
			if err := p.WithImage(ctx, d, consume); err == nil {
				t.Fatal("expected refusal")
			}
		})
	}
}

func TestImageDownstreamFailuresSafeAndClosed(t *testing.T) {
	for _, name := range []string{"download", "download-artifact", "nil", "invalid", "truncated", "decode-invalid", "dimensions", "empty", "oversize", "reader", "encode-reader", "consumer", "close", "cancel-consumer", "cancel-download"} {
		t.Run(name, func(t *testing.T) {
			raw := tinyImage(t, "png")
			if name == "invalid" {
				raw = []byte("untrusted bytes")
			}
			if name == "truncated" {
				raw = raw[:len(raw)-5]
			}
			if name == "dimensions" {
				binary.BigEndian.PutUint32(raw[16:20], 8193)
			}
			if name == "decode-invalid" {
				// Keep every structural chunk and CRC valid, corrupt zlib contents.
				for off := 8; off+12 <= len(raw); {
					n := int(binary.BigEndian.Uint32(raw[off : off+4]))
					if string(raw[off+4:off+8]) == "IDAT" {
						raw[off+8] = 0
						binary.BigEndian.PutUint32(raw[off+8+n:off+12+n], crc32.ChecksumIEEE(raw[off+4:off+8+n]))
						break
					}
					off += 12 + n
				}
			}
			a, _ := completed(t, raw)
			w := &wrappedArtifact{Artifact: a}
			canary := errors.New("PRIVATE_CANARY_KEY_URL_FILENAME")
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			if name == "empty" || name == "oversize" {
				n := int64(0)
				if name == "oversize" {
					n = 4<<20 + 1
				}
				w.sizeOverride = &n
			}
			if name == "reader" {
				w.readFn = func([]byte, int64) (int, error) { return 0, canary }
			}
			if name == "encode-reader" {
				zeroReads := 0
				w.readFn = func(b []byte, o int64) (int, error) {
					if o == 0 {
						zeroReads++
					}
					if zeroReads >= 3 {
						return 0, canary
					}
					return a.ReadAt(b, o)
				}
			}
			if name == "close" {
				w.closeFn = func() error { _ = a.Close(); return canary }
			}
			p := processor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
				switch name {
				case "download":
					return nil, canary
				case "nil":
					return nil, nil
				case "download-artifact":
					return w, canary
				case "cancel-download":
					cancel()
					return w, canary
				}
				return w, nil
			})
			calls := 0
			err := p.WithImage(ctx, descriptor(), func(provider.ContentPart) error {
				calls++
				switch name {
				case "consumer":
					return canary
				case "cancel-consumer":
					cancel()
				}
				return nil
			})
			if err == nil || strings.Contains(err.Error(), "PRIVATE_CANARY") {
				t.Fatalf("unsafe or missing error: %v", err)
			}
			want := 0
			if name == "consumer" || name == "close" || name == "cancel-consumer" {
				want = 1
			}
			if calls != want {
				t.Fatalf("calls %d want %d", calls, want)
			}
			closed := int32(1)
			if name == "download" || name == "nil" {
				closed = 0
			}
			if w.closes.Load() != closed {
				t.Fatalf("closes %d want %d", w.closes.Load(), closed)
			}
			if strings.HasPrefix(name, "cancel-") && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
		})
	}
}

func TestImageCanonicalMediaKeys(t *testing.T) {
	for _, name := range []string{"raw16", "asciihex32"} {
		t.Run(name, func(t *testing.T) {
			d := descriptor()
			d.ImageHex = ""
			raw := bytes.Repeat([]byte{0x12}, 16)
			encoded := raw
			if name == "asciihex32" {
				encoded = []byte(strings.Repeat("12", 16))
			}
			d.MediaKey = base64.StdEncoding.EncodeToString(encoded)
			a, _ := completed(t, tinyImage(t, "png"))
			p := processor(t, func(_ context.Context, _, _, _ string, key [16]byte) (Artifact, error) {
				if !bytes.Equal(key[:], raw) {
					t.Error("incorrect parsed key")
				}
				return a, nil
			})
			if e := p.WithImage(context.Background(), d, func(provider.ContentPart) error { return nil }); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not reach barrier")
	}
}

func TestImageAdmissionRetainedThroughConsumerAndClose(t *testing.T) {
	a, _ := completed(t, tinyImage(t, "png"))
	consumeEntered := make(chan struct{})
	consumeRelease := make(chan struct{})
	closeEntered := make(chan struct{})
	closeRelease := make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	var consumeOnce, closeOnce sync.Once
	// Release and join are registered before launching work or any assertion.
	t.Cleanup(func() {
		cancel()
		consumeOnce.Do(func() { close(consumeRelease) })
		closeOnce.Do(func() { close(closeRelease) })
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("worker not joined")
		}
	})
	w := &wrappedArtifact{Artifact: a}
	w.closeFn = func() error { close(closeEntered); <-closeRelease; return a.Close() }
	var downloads atomic.Int32
	p := processor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
		downloads.Add(1)
		return w, nil
	})
	go func() {
		done <- p.WithImage(ctx, descriptor(), func(provider.ContentPart) error { close(consumeEntered); <-consumeRelease; return nil })
	}()
	await(t, consumeEntered)
	cancel()
	checkWaiter := func() {
		wait, c := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer c()
		e := p.WithImage(wait, descriptor(), func(provider.ContentPart) error { t.Error("waiter consumed"); return nil })
		if !errors.Is(e, context.DeadlineExceeded) || downloads.Load() != 1 {
			t.Fatalf("admission released early: %v downloads=%d", e, downloads.Load())
		}
	}
	checkWaiter()
	consumeOnce.Do(func() { close(consumeRelease) })
	await(t, closeEntered)
	checkWaiter()
	closeOnce.Do(func() { close(closeRelease) })
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatalf("cancel result %v", e)
		}
		done <- e
	case <-time.After(5 * time.Second):
		t.Fatal("worker not complete")
	}
	// A fresh completed artifact proves reuse after actual Close, not cancellation.
	a2, _ := completed(t, tinyImage(t, "gif"))
	p.download = func(context.Context, string, string, string, [16]byte) (Artifact, error) {
		downloads.Add(1)
		return a2, nil
	}
	if e := p.WithImage(context.Background(), descriptor(), func(provider.ContentPart) error { return nil }); e != nil || downloads.Load() != 2 {
		t.Fatalf("slot not reusable: %v", e)
	}
}

func TestImageCanceledWaiterDoesNotDownload(t *testing.T) {
	p := processor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
		t.Error("download effect")
		return nil, nil
	})
	p.slot <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := p.WithImage(ctx, descriptor(), func(provider.ContentPart) error { return nil }); !errors.Is(e, context.Canceled) {
		t.Fatalf("canceled waiter: %v", e)
	}
	<-p.slot
}

func TestImageHeldDownloadAndReadJoinOnCancel(t *testing.T) {
	for _, stage := range []string{"download", "read"} {
		t.Run(stage, func(t *testing.T) {
			a, _ := completed(t, tinyImage(t, "png"))
			w := &wrappedArtifact{Artifact: a}
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			ctx, cancel := context.WithCancel(context.Background())
			var releaseOnce, readOnce sync.Once
			t.Cleanup(func() {
				cancel()
				releaseOnce.Do(func() { close(release) })
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("actual worker not joined")
				}
			})
			if stage == "read" {
				w.readFn = func(b []byte, o int64) (int, error) {
					readOnce.Do(func() { close(entered); <-release })
					return a.ReadAt(b, o)
				}
			}
			var downloads atomic.Int32
			p := processor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
				downloads.Add(1)
				if stage == "download" {
					close(entered)
					<-release
				}
				return w, nil
			})
			go func() {
				done <- p.WithImage(ctx, descriptor(), func(provider.ContentPart) error { t.Error("canceled consumer"); return nil })
			}()
			await(t, entered)
			cancel()
			wait, waitCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer waitCancel()
			if err := p.WithImage(wait, descriptor(), func(provider.ContentPart) error { t.Error("waiter consumer"); return nil }); !errors.Is(err, context.DeadlineExceeded) || downloads.Load() != 1 {
				t.Fatalf("lost actual lifetime: %v downloads=%d", err, downloads.Load())
			}
			select {
			case e := <-done:
				done <- e
				t.Fatal("returned before actual work")
			default:
			}
			releaseOnce.Do(func() { close(release) })
			select {
			case e := <-done:
				done <- e
				if !errors.Is(e, context.Canceled) || w.closes.Load() != 1 {
					t.Fatalf("result=%v closes=%d", e, w.closes.Load())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("worker not complete")
			}
		})
	}
}

func TestImageKnownErrorsSanitizedAndCleanupFailure(t *testing.T) {
	for _, known := range []error{media.ErrLimit, media.ErrPrivate, media.ErrIO, media.ErrEndpoint, media.ErrTransport, netx.ErrLimit, context.DeadlineExceeded, &netx.HTTPError{Status: 429}} {
		t.Run(known.Error(), func(t *testing.T) {
			a, _ := completed(t, tinyImage(t, "png"))
			w := &wrappedArtifact{Artifact: a}
			p := processor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
				return w, fmt.Errorf("PRIVATE_CANARY: %w", known)
			})
			err := p.WithImage(context.Background(), descriptor(), func(provider.ContentPart) error { t.Error("consumer"); return nil })
			if err == nil || strings.Contains(err.Error(), "PRIVATE_CANARY") || err.Error() != known.Error() || w.closes.Load() != 1 {
				t.Fatalf("classification=%v closes=%d", err, w.closes.Load())
			}
		})
	}
	for _, stage := range []string{"download-error", "consumer-error", "cancel-close"} {
		t.Run(stage, func(t *testing.T) {
			a, _ := completed(t, tinyImage(t, "png"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := &wrappedArtifact{Artifact: a, closeFn: func() error {
				_ = a.Close()
				if stage == "cancel-close" {
					cancel()
				}
				return errors.New("PRIVATE_CANARY")
			}}
			p := processor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
				if stage == "download-error" {
					return w, errors.New("PRIVATE_CANARY")
				}
				return w, nil
			})
			err := p.WithImage(ctx, descriptor(), func(provider.ContentPart) error {
				if stage == "consumer-error" {
					return errors.New("PRIVATE_CANARY")
				}
				return nil
			})
			if !errors.Is(err, ErrCleanup) || strings.Contains(err.Error(), "PRIVATE_CANARY") || w.closes.Load() != 1 {
				t.Fatalf("cleanup=%v closes=%d", err, w.closes.Load())
			}
			if stage == "cancel-close" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost close cancellation: %v", err)
			}
		})
	}
}

func TestImageConstructorAndZeroRefusal(t *testing.T) {
	if _, e := NewImageProcessor("synthetic", nil); e != ErrInvalid {
		t.Fatal(e)
	}
	if _, e := newImageProcessor("", func(context.Context, string, string, string, [16]byte) (Artifact, error) { return nil, nil }); e != ErrInvalid {
		t.Fatal(e)
	}
	if _, e := newImageProcessor("synthetic", nil); e != ErrInvalid {
		t.Fatal(e)
	}
	for _, p := range []*ImageProcessor{nil, {}} {
		if e := p.WithImage(context.Background(), descriptor(), func(provider.ContentPart) error { return nil }); e != ErrInvalid {
			t.Fatal(e)
		}
	}
}
