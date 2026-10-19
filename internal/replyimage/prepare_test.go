package replyimage

import (
	"bytes"
	"context"
	"crypto/md5" // Official wire checksum only.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/privatefs"
)

type testArtifact struct {
	*bytes.Reader
	size          int64
	reads, closes int
	read          func([]byte, int64) (int, error)
	close         func() error
}

func (a *testArtifact) Size() int64 { return a.size }
func (a *testArtifact) ReadAt(b []byte, o int64) (int, error) {
	a.reads++
	if a.read != nil {
		return a.read(b, o)
	}
	return a.Reader.ReadAt(b, o)
}
func (a *testArtifact) Close() error {
	a.closes++
	if a.close != nil {
		return a.close()
	}
	return nil
}

type testLocator func(context.Context, weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error)

func (f testLocator) GetImageUploadURL(c context.Context, r weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
	return f(c, r)
}
func fixture(t *testing.T, format string) ([]byte, attachment.LocalImage, *testArtifact) {
	t.Helper()
	var b bytes.Buffer
	im := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	im.Set(1, 1, color.NRGBA{R: 123, A: 255})
	var e error
	if format == "jpeg" {
		e = jpeg.Encode(&b, im, nil)
	} else {
		e = png.Encode(&b, im)
	}
	if e != nil {
		t.Fatal(e)
	}
	raw := b.Bytes()
	info, e := media.ValidateImage(context.Background(), bytes.NewReader(raw), int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	return raw, attachment.LocalImage{Info: info, Bytes: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])}, &testArtifact{Reader: bytes.NewReader(raw), size: int64(len(raw))}
}
func testDir(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "private")
	if e := privatefs.EnsureDir(d); e != nil {
		t.Fatal(e)
	}
	return d
}
func defaultDeps() dependencies {
	return dependencies{random: bytes.NewReader(append(bytes.Repeat([]byte{1}, 16), bytes.Repeat([]byte{2}, 16)...)), encrypt: func(c context.Context, d string, r io.Reader, k [16]byte) (attachment.Artifact, error) {
		a, e := media.Encrypt(c, d, r, k)
		if a == nil {
			return nil, e
		}
		return a, e
	}, upload: func(context.Context, attachment.Artifact, string, string, string) (string, error) {
		return "uploaded", nil
	}}
}
func preparer(t *testing.T, d dependencies) *Preparer {
	t.Helper()
	p, e := newPreparer(testDir(t), d)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func zeroFailure(t *testing.T, out weixin.ImageSend, e error) {
	t.Helper()
	if out != (weixin.ImageSend{}) || e == nil {
		t.Fatalf("want zero/error; got descriptor=%v error=%v", out != (weixin.ImageSend{}), e)
	}
}
func location(context.Context, weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
	return weixin.ImageUploadLocation{UploadParam: "location"}, nil
}

func TestPrepareActualEncryption(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			raw, im, plain := fixture(t, format)
			d := defaultDeps()
			var order []string
			var request weixin.ImageUploadRequest
			var ciphertext attachment.Artifact
			encrypt := d.encrypt
			d.encrypt = func(c context.Context, dir string, r io.Reader, k [16]byte) (attachment.Artifact, error) {
				order = append(order, "encrypt")
				a, e := encrypt(c, dir, r, k)
				ciphertext = a
				return a, e
			}
			d.upload = func(c context.Context, a attachment.Artifact, param, url, filekey string) (string, error) {
				order = append(order, "upload")
				if param != "location" || url != "" || filekey != request.FileKey {
					t.Fatal("locator forwarding")
				}
				key, e := media.ParseKey(request.AESKeyHex, "")
				if e != nil {
					t.Fatal(e)
				}
				dec, e := media.Decrypt(c, testDir(t), io.NewSectionReader(a, 0, a.Size()), key)
				if e != nil {
					t.Fatal(e)
				}
				defer dec.Close()
				got := make([]byte, dec.Size())
				if _, e = dec.ReadAt(got, 0); e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(got, raw) {
					t.Fatal("plaintext mismatch")
				}
				return "uploaded", nil
			}
			p := preparer(t, d)
			out, e := p.Prepare(context.Background(), "recipient", im, plain, testLocator(func(c context.Context, r weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
				order = append(order, "locator")
				request = r
				sum := md5.Sum(raw)
				if r.RawMD5 != hex.EncodeToString(sum[:]) || r.RawBytes != int64(len(raw)) || r.ToUserID != "recipient" || r.FileKey == r.AESKeyHex || len(r.FileKey) != 32 || len(r.AESKeyHex) != 32 {
					t.Fatal("actual metadata/random independence")
				}
				return location(c, r)
			}))
			if e != nil {
				t.Fatal(e)
			}
			if out.EncryptQueryParam != "uploaded" || out.AESKeyHex != request.AESKeyHex || out.CiphertextBytes != 16*(im.Bytes/16+1) {
				t.Fatal("descriptor")
			}
			if strings.Join(order, ",") != "encrypt,locator,upload" {
				t.Fatalf("order %v", order)
			}
			if plain.closes != 0 {
				t.Fatal("borrow closed")
			}
			if _, e = ciphertext.ReadAt(make([]byte, 1), 0); e == nil {
				t.Fatal("ciphertext not closed")
			}
		})
	}
}

func TestPrepareAdmissionBeforeWork(t *testing.T) {
	cases := []struct {
		name   string
		change func(*attachment.LocalImage, *testArtifact, *string)
	}{
		{"recipient empty", func(_ *attachment.LocalImage, _ *testArtifact, s *string) { *s = "" }}, {"recipient long", func(_ *attachment.LocalImage, _ *testArtifact, s *string) { *s = strings.Repeat("a", 16385) }}, {"recipient control", func(_ *attachment.LocalImage, _ *testArtifact, s *string) { *s = "a\n" }}, {"recipient utf8", func(_ *attachment.LocalImage, _ *testArtifact, s *string) { *s = string([]byte{255}) }},
		{"size zero", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.Bytes = 0 }}, {"size large", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.Bytes = 4<<20 + 1 }}, {"size inconsistent", func(_ *attachment.LocalImage, a *testArtifact, _ *string) { a.size++ }},
		{"digest malformed", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.SHA256 = "bad" }}, {"digest uppercase", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.SHA256 = strings.Repeat("A", 64) }},
		{"gif", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.Info.Format = "gif" }}, {"animated", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.Info.Animated = true }}, {"frames", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.Info.Frames = 2 }}, {"width", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.Info.Width = 8193 }}, {"height", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.Info.Height = 0 }}, {"pixels", func(i *attachment.LocalImage, _ *testArtifact, _ *string) { i.Info.Width = 5000; i.Info.Height = 5000 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, im, a := fixture(t, "png")
			to := "recipient"
			tc.change(&im, a, &to)
			d := defaultDeps()
			d.encrypt = func(context.Context, string, io.Reader, [16]byte) (attachment.Artifact, error) {
				t.Fatal("encrypt before admission")
				return nil, nil
			}
			p := preparer(t, d)
			o, e := p.Prepare(context.Background(), to, im, a, testLocator(func(context.Context, weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
				t.Fatal("locator before admission")
				return weixin.ImageUploadLocation{}, nil
			}))
			zeroFailure(t, o, e)
			if a.reads != 0 || a.closes != 0 {
				t.Fatal("borrow touched")
			}
		})
	}
}
func TestPrepareNilAndCanceled(t *testing.T) {
	_, im, a := fixture(t, "png")
	p := preparer(t, defaultDeps())
	c, cancel := context.WithCancel(context.Background())
	cancel()
	var nilArtifact *testArtifact
	var nilLocator testLocator
	for _, tc := range []struct {
		name string
		p    *Preparer
		c    context.Context
		a    attachment.Artifact
		l    UploadLocator
	}{{"nil context", p, nil, a, testLocator(location)}, {"cancel", p, c, a, testLocator(location)}, {"nil self", nil, context.Background(), a, testLocator(location)}, {"nil artifact", p, context.Background(), nil, testLocator(location)}, {"typed nil artifact", p, context.Background(), nilArtifact, testLocator(location)}, {"nil locator", p, context.Background(), a, nil}, {"typed nil locator", p, context.Background(), a, nilLocator}, {"zero preparer", &Preparer{}, context.Background(), a, testLocator(location)}} {
		t.Run(tc.name, func(t *testing.T) {
			o, e := tc.p.Prepare(tc.c, "recipient", im, tc.a, tc.l)
			zeroFailure(t, o, e)
			if tc.name == "cancel" && !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
		})
	}
	if a.reads != 0 || a.closes != 0 {
		t.Fatal("touched borrow")
	}
	if p, e := New("x", nil); p != nil || e == nil {
		t.Fatal("nil CDN accepted")
	}
	cdn, e := media.NewCDN()
	if e != nil {
		t.Fatal(e)
	}
	defer cdn.Close()
	if p, e := New("", cdn); p != nil || e == nil {
		t.Fatal("empty directory accepted")
	}
	if p, e := New(testDir(t), cdn); p == nil || e != nil {
		t.Fatal("valid constructor")
	}
}
func TestPrepareActualBytesBeforeNetwork(t *testing.T) {
	for _, kind := range []string{"digest", "short", "extra", "read error"} {
		t.Run(kind, func(t *testing.T) {
			raw, im, a := fixture(t, "png")
			switch kind {
			case "digest":
				im.SHA256 = strings.Repeat("0", 64)
			case "short":
				a.Reader = bytes.NewReader(raw[:len(raw)-1])
			case "extra":
				a.Reader = bytes.NewReader(append(raw, 1))
			case "read error":
				a.read = func([]byte, int64) (int, error) { return 0, errors.New("poison-secret") }
			}
			d := defaultDeps()
			d.upload = func(context.Context, attachment.Artifact, string, string, string) (string, error) {
				t.Fatal("upload before actual validation")
				return "", nil
			}
			p := preparer(t, d)
			o, e := p.Prepare(context.Background(), "recipient", im, a, testLocator(func(context.Context, weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
				t.Fatal("locator before actual validation")
				return weixin.ImageUploadLocation{}, nil
			}))
			zeroFailure(t, o, e)
			if strings.Contains(e.Error(), "poison") {
				t.Fatal("raw error")
			}
			if a.closes != 0 {
				t.Fatal("borrow closed")
			}
		})
	}
}
func TestPrepareFailuresAndClassification(t *testing.T) {
	for _, kind := range []string{"random", "encrypt", "transferred", "cipher size", "locator", "auth", "upload", "empty upload", "cleanup", "cancel encrypt", "cancel locator", "cancel upload", "cancel cleanup", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			_, im, a := fixture(t, "png")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d := defaultDeps()
			var events []string
			cipher := &testArtifact{Reader: bytes.NewReader(make([]byte, 16*(im.Bytes/16+1))), size: 16 * (im.Bytes/16 + 1)}
			poison := errors.New("poison-secret-url-key")
			d.encrypt = func(c context.Context, _ string, r io.Reader, _ [16]byte) (attachment.Artifact, error) {
				events = append(events, "encrypt")
				if _, e := io.Copy(io.Discard, r); e != nil {
					return nil, e
				}
				switch kind {
				case "encrypt":
					return nil, poison
				case "transferred":
					return cipher, poison
				case "cipher size":
					cipher.size++
				case "cancel encrypt":
					cancel()
				}
				return cipher, nil
			}
			d.upload = func(context.Context, attachment.Artifact, string, string, string) (string, error) {
				events = append(events, "upload")
				switch kind {
				case "upload":
					return "secret", poison
				case "empty upload":
					return "", nil
				case "cancel upload":
					cancel()
				}
				return "uploaded", nil
			}
			cipher.close = func() error {
				events = append(events, "close")
				switch kind {
				case "cleanup":
					return poison
				case "cancel cleanup":
					cancel()
				}
				return nil
			}
			if kind == "random" {
				d.random = bytes.NewReader(nil)
			}
			p := preparer(t, d)
			o, e := p.Prepare(ctx, "recipient", im, a, testLocator(func(context.Context, weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
				events = append(events, "locator")
				switch kind {
				case "locator":
					return weixin.ImageUploadLocation{}, poison
				case "auth":
					return weixin.ImageUploadLocation{}, fmt.Errorf("poison: %w", weixin.ErrAuthExpired)
				case "deadline":
					return weixin.ImageUploadLocation{}, fmt.Errorf("poison: %w", context.DeadlineExceeded)
				case "cancel locator":
					cancel()
				}
				return weixin.ImageUploadLocation{UploadParam: "location"}, nil
			}))
			zeroFailure(t, o, e)
			if strings.Contains(e.Error(), "poison") || strings.Contains(e.Error(), "secret") {
				t.Fatal("unsafe error")
			}
			if strings.HasPrefix(kind, "cancel") && !errors.Is(e, context.Canceled) {
				t.Fatal("cancel classification")
			}
			if kind == "auth" && !errors.Is(e, weixin.ErrAuthExpired) {
				t.Fatal("auth classification")
			}
			if kind == "deadline" && !errors.Is(e, context.DeadlineExceeded) {
				t.Fatal("deadline classification")
			}
			wantClose := 1
			if kind == "random" || kind == "encrypt" {
				wantClose = 0
			}
			if cipher.closes != wantClose || a.closes != 0 {
				t.Fatalf("ownership cipher=%d plain=%d events=%v", cipher.closes, a.closes, events)
			}
			for _, name := range []string{"encrypt", "locator", "upload", "close"} {
				count := 0
				for _, event := range events {
					if event == name {
						count++
					}
				}
				if count > 1 {
					t.Fatal("repeated side effect")
				}
			}
			wantOrder := "encrypt,locator,upload,close"
			switch kind {
			case "random":
				wantOrder = ""
			case "encrypt":
				wantOrder = "encrypt"
			case "transferred", "cipher size", "cancel encrypt":
				wantOrder = "encrypt,close"
			case "locator", "auth", "deadline", "cancel locator":
				wantOrder = "encrypt,locator,close"
			}
			if strings.Join(events, ",") != wantOrder {
				t.Fatalf("call order %v; want %s", events, wantOrder)
			}
		})
	}
}
func TestPrepareBorrowAndCleanupJoin(t *testing.T) {
	for _, phase := range []string{"read", "close"} {
		t.Run(phase, func(t *testing.T) {
			raw, im, a := fixture(t, "png")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			d := defaultDeps()
			if phase == "read" {
				a.read = func(b []byte, o int64) (int, error) {
					close(entered)
					<-release
					return bytes.NewReader(raw).ReadAt(b, o)
				}
			} else {
				encrypt := d.encrypt
				d.encrypt = func(c context.Context, dir string, r io.Reader, k [16]byte) (attachment.Artifact, error) {
					art, e := encrypt(c, dir, r, k)
					if e != nil {
						return art, e
					}
					return &closeWrapper{Artifact: art, close: func() error { close(entered); <-release; return art.Close() }}, nil
				}
			}
			p := preparer(t, d)
			var out weixin.ImageSend
			var err error
			go func() { out, err = p.Prepare(ctx, "recipient", im, a, testLocator(location)); close(done) }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("work not entered")
			}
			cancel()
			select {
			case <-done:
				t.Fatal("abandoned actual work")
			default:
			}
			close(release)
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("did not join")
			}
			zeroFailure(t, out, err)
			if !errors.Is(err, context.Canceled) || a.closes != 0 {
				t.Fatal("cancel/ownership")
			}
		})
	}
}

type closeWrapper struct {
	attachment.Artifact
	close func() error
}

func (a *closeWrapper) Close() error { return a.close() }

func TestPrepareProductionRandomnessAndBorrowedCDN(t *testing.T) {
	_, im, plain := fixture(t, "png")
	cdn, e := media.NewCDN()
	if e != nil {
		t.Fatal(e)
	}
	defer cdn.Close()
	p, e := New(testDir(t), cdn)
	if e != nil {
		t.Fatal(e)
	}
	// Keep real crypto/rand and encryption. Replace only the local instance's
	// upload function to observe preparation without any external request.
	p.deps.upload = func(context.Context, attachment.Artifact, string, string, string) (string, error) {
		return "uploaded", nil
	}
	seen := map[string]bool{}
	for range 2 {
		out, e := p.Prepare(context.Background(), "recipient", im, plain, testLocator(func(_ context.Context, r weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
			for _, v := range []string{r.AESKeyHex, r.FileKey} {
				if seen[v] || len(v) != 32 || strings.ToLower(v) != v {
					t.Fatal("random key collision/encoding")
				}
				seen[v] = true
			}
			return weixin.ImageUploadLocation{UploadParam: "location"}, nil
		}))
		if e != nil || out.EncryptQueryParam != "uploaded" {
			t.Fatalf("prepare error %v", e)
		}
	}
	if plain.closes != 0 {
		t.Fatal("borrowed plaintext closed")
	}
	// The untouched production adapter reaches the exact borrowed, closed CDN.
	// It refuses without network and still removes its real owned ciphertext.
	cdn.Close()
	q, e := New(testDir(t), cdn)
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	out, e := q.Prepare(context.Background(), "recipient", im, plain, testLocator(func(c context.Context, r weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
		calls++
		return location(c, r)
	}))
	zeroFailure(t, out, e)
	if calls != 1 {
		t.Fatal("locator count")
	}
	entries, e := os.ReadDir(q.dir)
	if e != nil || len(entries) != 0 {
		t.Fatal("owned ciphertext leak")
	}
	if _, e = q.deps.upload(context.Background(), &testArtifact{}, "", "", ""); !errors.Is(e, media.ErrInvalid) {
		t.Fatal("production adapter accepted nongenuine artifact")
	}
}

func TestPrepareAdditionalDependencyAndEntropyRefusals(t *testing.T) {
	for _, name := range []string{"nil entropy", "typed nil entropy", "nil encrypt", "nil upload", "empty dir"} {
		t.Run(name, func(t *testing.T) {
			d := defaultDeps()
			dir := "borrowed"
			switch name {
			case "nil entropy":
				d.random = nil
			case "typed nil entropy":
				var r *bytes.Reader
				d.random = r
			case "nil encrypt":
				d.encrypt = nil
			case "nil upload":
				d.upload = nil
			case "empty dir":
				dir = ""
			}
			if p, e := newPreparer(dir, d); p != nil || e == nil {
				t.Fatal("invalid dependencies accepted")
			}
		})
	}
	for _, name := range []string{"second key failure", "entropy cancellation", "nil ciphertext", "typed nil ciphertext", "cleanup plus cancel", "unsafe uploaded param", "oversized uploaded param"} {
		t.Run(name, func(t *testing.T) {
			_, im, a := fixture(t, "png")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d := defaultDeps()
			calls := 0
			cipher := &testArtifact{Reader: bytes.NewReader(nil), size: 16 * (im.Bytes/16 + 1)}
			switch name {
			case "second key failure":
				d.random = bytes.NewReader(make([]byte, 16))
			case "entropy cancellation":
				d.random = readerFunc(func(b []byte) (int, error) { cancel(); return len(b), nil })
			case "nil ciphertext":
				d.encrypt = func(context.Context, string, io.Reader, [16]byte) (attachment.Artifact, error) { return nil, nil }
			case "typed nil ciphertext":
				d.encrypt = func(context.Context, string, io.Reader, [16]byte) (attachment.Artifact, error) {
					var a *testArtifact
					return a, nil
				}
			case "cleanup plus cancel":
				d.encrypt = func(_ context.Context, _ string, r io.Reader, _ [16]byte) (attachment.Artifact, error) {
					_, e := io.Copy(io.Discard, r)
					return cipher, e
				}
				cipher.close = func() error { cancel(); return errors.New("poison-cleanup") }
			case "unsafe uploaded param":
				d.upload = func(context.Context, attachment.Artifact, string, string, string) (string, error) {
					return "query\n", nil
				}
			case "oversized uploaded param":
				d.upload = func(context.Context, attachment.Artifact, string, string, string) (string, error) {
					return strings.Repeat("a", 4097), nil
				}
			}
			p := preparer(t, d)
			o, e := p.Prepare(ctx, "recipient", im, a, testLocator(func(c context.Context, r weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
				calls++
				return location(c, r)
			}))
			zeroFailure(t, o, e)
			if strings.Contains(e.Error(), "poison") {
				t.Fatal("raw cleanup error")
			}
			if name == "cleanup plus cancel" && (!errors.Is(e, context.Canceled) || !errors.Is(e, ErrCleanup) || cipher.closes != 1) {
				t.Fatal("joined cleanup/cancel classification")
			}
			if name == "entropy cancellation" && !errors.Is(e, context.Canceled) {
				t.Fatal("entropy cancel classification")
			}
			if name != "cleanup plus cancel" && name != "unsafe uploaded param" && name != "oversized uploaded param" && calls != 0 {
				t.Fatal("locator before actual readiness")
			}
			if a.closes != 0 {
				t.Fatal("closed borrow")
			}
			entries, e := os.ReadDir(p.dir)
			if e != nil || len(entries) != 0 {
				t.Fatal("ciphertext leak")
			}
		})
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(b []byte) (int, error) { return f(b) }
