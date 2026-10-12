package attachment

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/hidxt/miskoai/internal/document"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fileDescriptor(name string) storage.Attachment {
	return storage.Attachment{Kind: "file", Query: "synthetic-query", MediaKey: base64.StdEncoding.EncodeToString(make([]byte, 16)), FileName: name}
}
func documentProcessor(t *testing.T, d imageDownloader) *ImageProcessor {
	p := processor(t, d)
	p.parser = document.New()
	return p
}
func generatedDOCX(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, v := range [][2]string{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/document.xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hello 中文</w:t></w:r></w:p></w:body></w:document>`}} {
		w, e := z.Create(v[0])
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(v[1])); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestDocumentSupportedCompleteAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format document.Format
		raw    []byte
		want   string
	}{{"source.TXT", document.TXT, []byte("Hello 中文"), "Hello 中文"}, {"source.Md", document.Markdown, []byte("# Hello 中文"), "# Hello 中文"}, {"source.docx", document.DOCX, generatedDOCX(t), "Hello 中文\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			a, dir := completed(t, tc.raw)
			w := &wrappedArtifact{Artifact: a}
			d := fileDescriptor(tc.name)
			d.Length = strconv.Itoa(len(tc.raw))
			sum := md5.Sum(tc.raw)
			d.MD5 = strings.ToUpper(hex.EncodeToString(sum[:]))
			calls := 0
			p := documentProcessor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) { return w, nil })
			e := p.WithDocument(context.Background(), d, func(r document.Result) error {
				calls++
				if r.Text != tc.want || r.Pages != 1 || r.Format != tc.format {
					t.Errorf("result %#v", r)
				}
				return nil
			})
			if e != nil || calls != 1 || w.closes.Load() != 1 {
				t.Fatalf("err %v calls %d close %d dir-hidden", e, calls, w.closes.Load())
			}
			entries, cleanupErr := os.ReadDir(dir)
			if cleanupErr != nil || len(entries) != 0 {
				t.Fatalf("private artifact remains: %v count %d", cleanupErr, len(entries))
			}
		})
	}
}
func TestDocumentPreflightNoFetch(t *testing.T) {
	for _, name := range []string{"", ".txt", "a/b.txt", `a\b.md`, "a.exe", "a", "bad-key", "bad-length", "bad-md5", "image", "unsupported", "nil-parser", "nil-context", "nil-consumer", "canceled"} {
		t.Run(name, func(t *testing.T) {
			d := fileDescriptor(name)
			ctx := context.Background()
			c := func(document.Result) error { t.Error("consume"); return nil }
			p := documentProcessor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
				t.Error("fetch")
				return nil, nil
			})
			switch name {
			case "bad-key":
				d.FileName = "a.txt"
				d.MediaKey = "bad"
			case "bad-length":
				d.FileName = "a.txt"
				d.Length = "01"
			case "bad-md5":
				d.FileName = "a.txt"
				d.MD5 = "bad"
			case "image":
				d = descriptor()
			case "unsupported":
				d = storage.Attachment{Kind: "unsupported"}
			case "nil-parser":
				d.FileName = "a.txt"
				p.parser = nil
			case "nil-context":
				ctx = nil
			case "nil-consumer":
				c = nil
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if e := p.WithDocument(ctx, d, c); e == nil {
				t.Fatal("missing refusal")
			}
		})
	}
}
func TestDocumentFailureNoPartialAndSafeCleanup(t *testing.T) {
	for _, name := range []string{"length", "md5", "short", "dishonest", "read-error", "full-eof", "download-artifact", "download-nil", "empty", "oversize", "invalid-utf8", "empty-text", "output-limit", "consumer", "close", "cancel-read", "cancel-consumer"} {
		t.Run(name, func(t *testing.T) {
			raw := []byte("Hello 中文")
			switch name {
			case "invalid-utf8":
				raw = []byte{255}
			case "empty-text":
				raw = []byte("  ")
			case "output-limit":
				raw = bytes.Repeat([]byte("a"), 131073)
			}
			a, _ := completed(t, raw)
			w := &wrappedArtifact{Artifact: a}
			d := fileDescriptor("private_canary.txt")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			canary := errors.New("PRIVATE_CANARY_PATH_KEY_URL_BODY")
			reads := 0
			w.readFn = func(b []byte, o int64) (int, error) {
				reads++
				switch name {
				case "short":
					return len(b) - 1, nil
				case "dishonest":
					return len(b) + 1, nil
				case "read-error":
					return 0, canary
				case "cancel-read":
					cancel()
					return 0, canary
				case "full-eof":
					n, _ := a.ReadAt(b, o)
					return n, io.EOF
				}
				return a.ReadAt(b, o)
			}
			switch name {
			case "length":
				d.Length = "1"
			case "md5":
				d.MD5 = strings.Repeat("0", 32)
			case "empty":
				n := int64(0)
				w.sizeOverride = &n
			case "oversize":
				n := int64(4194305)
				w.sizeOverride = &n
			case "close":
				w.closeFn = func() error { _ = a.Close(); return canary }
			}
			p := documentProcessor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
				if name == "download-artifact" {
					return w, canary
				}
				if name == "download-nil" {
					return nil, canary
				}
				return w, nil
			})
			calls := 0
			e := p.WithDocument(ctx, d, func(document.Result) error {
				calls++
				if name == "consumer" {
					return canary
				}
				if name == "cancel-consumer" {
					cancel()
				}
				return nil
			})
			wantCalls := 0
			if name == "full-eof" || name == "consumer" || name == "close" || name == "cancel-consumer" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("calls %d want %d", calls, wantCalls)
			}
			if name == "full-eof" {
				if e != nil {
					t.Fatal(e)
				}
			} else if e == nil || strings.Contains(e.Error(), "PRIVATE_CANARY") || strings.Contains(e.Error(), "private_canary") {
				t.Fatalf("unsafe or missing error %v", e)
			}
			wantClose := int32(1)
			if name == "download-nil" {
				wantClose = 0
			}
			if w.closes.Load() != wantClose {
				t.Fatalf("close %d", w.closes.Load())
			}
			if name == "length" && reads != 0 {
				t.Fatal("length mismatch read")
			}
			if strings.HasPrefix(name, "cancel-") && !errors.Is(e, context.Canceled) {
				t.Fatalf("lost cancel %v", e)
			}
		})
	}
}
func TestDocumentAdmissionThroughActualConsumerClose(t *testing.T) {
	a, _ := completed(t, []byte("Hello"))
	w := &wrappedArtifact{Artifact: a}
	entered := make(chan struct{})
	release := make(chan struct{})
	closeEntered := make(chan struct{})
	closeRelease := make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	var once, closeOnce sync.Once
	t.Cleanup(func() {
		cancel()
		once.Do(func() { close(release) })
		closeOnce.Do(func() { close(closeRelease) })
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("worker not joined")
		}
	})
	w.closeFn = func() error { close(closeEntered); <-closeRelease; return a.Close() }
	var downloads atomic.Int32
	p := documentProcessor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
		downloads.Add(1)
		return w, nil
	})
	go func() {
		done <- p.WithDocument(ctx, fileDescriptor("a.txt"), func(document.Result) error { close(entered); <-release; return nil })
	}()
	await(t, entered)
	cancel()
	check := func() {
		for _, image := range []bool{false, true} {
			wait, c := context.WithTimeout(context.Background(), 20*time.Millisecond)
			var e error
			if image {
				e = p.WithImage(wait, descriptor(), func(provider.ContentPart) error { t.Error("image effect"); return nil })
			} else {
				e = p.WithDocument(wait, fileDescriptor("b.txt"), func(document.Result) error { t.Error("doc effect"); return nil })
			}
			c()
			if !errors.Is(e, context.DeadlineExceeded) || downloads.Load() != 1 {
				t.Fatalf("early slot %v %d", e, downloads.Load())
			}
		}
	}
	check()
	once.Do(func() { close(release) })
	await(t, closeEntered)
	check()
	closeOnce.Do(func() { close(closeRelease) })
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
		done <- e
	case <-time.After(2 * time.Second):
		t.Fatal("join")
	}
	a2, _ := completed(t, []byte("fresh"))
	p.download = func(context.Context, string, string, string, [16]byte) (Artifact, error) { return a2, nil }
	if e := p.WithDocument(context.Background(), fileDescriptor("a.md"), func(document.Result) error { return nil }); e != nil {
		t.Fatal(e)
	}
}

// Releasing on cancellation before a synchronous download/read returns would
// admit a second media operation and violate the joined lifetime.
func TestDocumentHeldDownloadReadJoin(t *testing.T) {
	for _, stage := range []string{"download", "read", "parse"} {
		t.Run(stage, func(t *testing.T) {
			a, _ := completed(t, []byte("held text"))
			w := &wrappedArtifact{Artifact: a}
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			ctx, cancel := context.WithCancel(context.Background())
			var once, readOnce sync.Once
			t.Cleanup(func() {
				cancel()
				once.Do(func() { close(release) })
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("not joined")
				}
			})
			if stage == "read" || stage == "parse" {
				reads := 0
				w.readFn = func(b []byte, o int64) (int, error) {
					reads++
					if stage == "read" || reads > 1 {
						readOnce.Do(func() { close(entered); <-release })
					}
					return a.ReadAt(b, o)
				}
			}
			var downloads atomic.Int32
			p := documentProcessor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
				downloads.Add(1)
				if stage == "download" {
					close(entered)
					<-release
				}
				return w, nil
			})
			go func() {
				done <- p.WithDocument(ctx, fileDescriptor("held.txt"), func(document.Result) error { t.Error("canceled consumed"); return nil })
			}()
			await(t, entered)
			cancel()
			wait, c := context.WithTimeout(context.Background(), 20*time.Millisecond)
			e := p.WithImage(wait, descriptor(), func(provider.ContentPart) error { t.Error("image consumed"); return nil })
			c()
			if !errors.Is(e, context.DeadlineExceeded) || downloads.Load() != 1 {
				t.Fatalf("early slot %v", e)
			}
			select {
			case e := <-done:
				done <- e
				t.Fatal("early return")
			default:
			}
			once.Do(func() { close(release) })
			select {
			case e := <-done:
				done <- e
				if !errors.Is(e, context.Canceled) || w.closes.Load() != 1 {
					t.Fatalf("cancel %v close %d", e, w.closes.Load())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("join")
			}
		})
	}
}

func TestDocumentChecksumScanCompleteBeforeParser(t *testing.T) {
	raw := bytes.Repeat([]byte("x"), 8193)
	a, _ := completed(t, raw)
	w := &wrappedArtifact{Artifact: a}
	reads := 0
	w.readFn = func(b []byte, o int64) (int, error) {
		reads++
		if len(b) > 4096 {
			t.Error("scratch exceeded")
		}
		return a.ReadAt(b, o)
	}
	d := fileDescriptor("a.txt")
	d.MD5 = strings.Repeat("0", 32)
	p := documentProcessor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) { return w, nil })
	e := p.WithDocument(context.Background(), d, func(document.Result) error { t.Error("digest mismatch consumed"); return nil })
	if !errors.Is(e, document.ErrInvalid) || reads != 3 || w.closes.Load() != 1 {
		t.Fatalf("checksum err %v reads %d close %d", e, reads, w.closes.Load())
	}
}

func TestDocumentCompleteResultGuard(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    document.Result
		want error
	}{
		{"format", document.Result{Text: "ok", Pages: 1, Format: document.Markdown}, document.ErrInvalid},
		{"unknown", document.Result{Text: "ok", Pages: 1, Format: "unknown"}, document.ErrInvalid},
		{"utf8", document.Result{Text: string([]byte{255}), Pages: 1, Format: document.TXT}, document.ErrInvalid},
		{"zero-pages", document.Result{Text: "ok", Pages: 0, Format: document.TXT}, document.ErrInvalid},
		{"pages-limit", document.Result{Text: "ok", Pages: 65, Format: document.TXT}, document.ErrLimit},
		{"empty", document.Result{Text: " \n", Pages: 1, Format: document.TXT}, document.ErrEmpty},
		{"output-limit", document.Result{Text: strings.Repeat("a", 131073), Pages: 1, Format: document.TXT}, document.ErrLimit},
		{"boundary", document.Result{Text: strings.Repeat("a", 131072), Pages: 64, Format: document.TXT}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := validateDocumentResult(tc.r, document.TXT); !errors.Is(e, tc.want) {
				t.Fatalf("guard %v want %v", e, tc.want)
			}
		})
	}
}

func TestDocumentCleanupClassesAndConstructorRefusal(t *testing.T) {
	if _, e := NewProcessor("synthetic", nil, document.New()); e != ErrInvalid {
		t.Fatal(e)
	}
	if _, e := NewProcessor("synthetic", nil, nil); e != ErrInvalid {
		t.Fatal(e)
	}
	for _, p := range []*ImageProcessor{nil, {}} {
		if e := p.WithDocument(context.Background(), fileDescriptor("a.txt"), func(document.Result) error { return nil }); e != ErrInvalid {
			t.Fatal(e)
		}
	}
	for _, stage := range []string{"download", "consumer", "cancel-close"} {
		t.Run(stage, func(t *testing.T) {
			a, _ := completed(t, []byte("hello"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := &wrappedArtifact{Artifact: a}
			w.closeFn = func() error {
				_ = a.Close()
				if stage == "cancel-close" {
					cancel()
				}
				return errors.New("PRIVATE_CANARY_CLOSE")
			}
			p := documentProcessor(t, func(context.Context, string, string, string, [16]byte) (Artifact, error) {
				if stage == "download" {
					return w, errors.New("PRIVATE_CANARY_DOWNLOAD")
				}
				return w, nil
			})
			e := p.WithDocument(ctx, fileDescriptor("a.txt"), func(document.Result) error {
				if stage == "consumer" {
					return errors.New("PRIVATE_CANARY_CONSUMER")
				}
				return nil
			})
			want := ErrDownload
			if stage == "consumer" {
				want = ErrConsume
			}
			if stage == "cancel-close" {
				want = context.Canceled
			}
			if !errors.Is(e, want) || !errors.Is(e, ErrCleanup) || strings.Contains(e.Error(), "PRIVATE_CANARY") || w.closes.Load() != 1 {
				t.Fatalf("cleanup %v close %d", e, w.closes.Load())
			}
		})
	}
}

func TestDocumentConstructorSharesBorrowedParserAndSlot(t *testing.T) {
	cdn, e := media.NewCDN()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = cdn.Close() })
	parser := document.New()
	p, e := NewProcessor("synthetic-admitted-dir", cdn, parser)
	if e != nil {
		t.Fatal(e)
	}
	if p.parser != parser {
		t.Fatal("did not borrow parser")
	}
	p.slot <- struct{}{}
	t.Cleanup(func() { <-p.slot })
	for _, image := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		var e error
		if image {
			e = p.WithImage(ctx, descriptor(), func(provider.ContentPart) error { t.Error("image effect"); return nil })
		} else {
			e = p.WithDocument(ctx, fileDescriptor("a.txt"), func(document.Result) error { t.Error("file effect"); return nil })
		}
		cancel()
		if !errors.Is(e, context.DeadlineExceeded) {
			t.Fatalf("shared slot %v", e)
		}
	}
	if _, e := NewProcessor("synthetic", cdn, nil); e != ErrInvalid {
		t.Fatal(e)
	}
}
