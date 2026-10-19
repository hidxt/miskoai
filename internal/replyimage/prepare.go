// Package replyimage prepares an uploaded image inside a caller-owned media lifetime.
package replyimage

import (
	"context"
	"crypto/md5" // Official wire checksum only, never content authentication.
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/media"
)

// UploadLocator performs only the existing authenticated upload-location request.
// Locations remain untrusted and pass through the borrowed CDN's unchanged policy.
type UploadLocator interface {
	GetImageUploadURL(context.Context, weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error)
}

var (
	ErrPrepare = errors.New("image preparation failed")
	ErrCleanup = errors.New("image preparation cleanup failed")
)

// dependencies is an instance-local deterministic testing seam. New always
// binds real encryption, crypto/rand and the exact borrowed CDN, with no new pool.
type dependencies struct {
	random  io.Reader
	encrypt func(context.Context, string, io.Reader, [16]byte) (attachment.Artifact, error)
	upload  func(context.Context, attachment.Artifact, string, string, string) (string, error)
}

// Preparer borrows its owner's already admitted private directory and CDN.
// The caller must hold the existing Manager/image admission and exact Store
// generation lease through Prepare and the subsequent send. Prepare acquires
// neither of those resources and does not send messages or persist state.
type Preparer struct {
	dir  string
	deps dependencies
}

func New(dataDir string, cdn *media.CDN) (*Preparer, error) {
	if cdn == nil {
		return nil, media.ErrInvalid
	}
	return newPreparer(dataDir, dependencies{
		random: rand.Reader,
		encrypt: func(ctx context.Context, dir string, input io.Reader, key [16]byte) (attachment.Artifact, error) {
			a, err := media.Encrypt(ctx, dir, input, key)
			if a == nil {
				return nil, err
			}
			return a, err
		},
		upload: func(ctx context.Context, artifact attachment.Artifact, param, url, filekey string) (string, error) {
			// Production CDN accepts only its genuine completed private artifact.
			a, ok := artifact.(*media.Artifact)
			if !ok || a == nil {
				return "", media.ErrInvalid
			}
			return cdn.Upload(ctx, a, param, url, filekey)
		},
	})
}

func newPreparer(dataDir string, deps dependencies) (*Preparer, error) {
	if dataDir == "" || nilDependency(deps.random) || deps.encrypt == nil || deps.upload == nil {
		return nil, media.ErrInvalid
	}
	return &Preparer{dir: dataDir, deps: deps}, nil
}

// Prepare borrows completed immutable validated plaintext only from the trusted
// Manager callback. Metadata checks here do not establish validation for arbitrary
// callers. It never closes or retains plaintext. Reads, uploads and ciphertext
// cleanup return synchronously; cancellation cannot abandon a blocked callback.
func (p *Preparer) Prepare(ctx context.Context, recipient string, image attachment.LocalImage, plaintext attachment.Artifact, locator UploadLocator) (out weixin.ImageSend, err error) {
	if ctx == nil {
		return out, media.ErrInvalid
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if p == nil || p.dir == "" || nilDependency(p.deps.random) || p.deps.encrypt == nil || p.deps.upload == nil || nilDependency(plaintext) || nilDependency(locator) || !opaque(recipient, 16<<10) || !validImage(image) {
		return out, media.ErrInvalid
	}
	if plaintext.Size() != image.Bytes {
		return out, media.ErrInvalid
	}
	var key, filekey [16]byte
	if _, e := io.ReadFull(p.deps.random, key[:]); e != nil {
		return out, safe(ctx, e)
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if _, e := io.ReadFull(p.deps.random, filekey[:]); e != nil {
		return out, safe(ctx, e)
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	// One extra byte detects inconsistent actual length before any external call.
	// The existing Encrypt owns its fixed small scratch and enforces the same 4MiB cap.
	input := &digestReader{reader: io.NewSectionReader(plaintext, 0, image.Bytes+1), sha: sha256.New(), md5: md5.New()}
	ciphertext, workErr := p.deps.encrypt(ctx, p.dir, input, key)
	// Register transferred ownership even when encryption returns an error.
	if !nilDependency(ciphertext) {
		defer func() {
			closeErr := ciphertext.Close()
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			if closeErr != nil {
				err = errors.Join(err, ErrCleanup)
			}
			if err != nil {
				out = weixin.ImageSend{}
			}
		}()
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if workErr != nil {
		return out, safe(ctx, workErr)
	}
	if nilDependency(ciphertext) {
		return out, ErrPrepare
	}
	expected := 16 * (image.Bytes/16 + 1)
	if input.bytes != image.Bytes || hex.EncodeToString(input.sha.Sum(nil)) != image.SHA256 || ciphertext.Size() != expected {
		return out, media.ErrInvalid
	}
	keyHex := hex.EncodeToString(key[:])
	fileHex := hex.EncodeToString(filekey[:])
	if e := ctx.Err(); e != nil {
		return out, e
	}
	loc, e := locator.GetImageUploadURL(ctx, weixin.ImageUploadRequest{ToUserID: recipient, FileKey: fileHex, RawMD5: hex.EncodeToString(input.md5.Sum(nil)), AESKeyHex: keyHex, RawBytes: input.bytes})
	if e != nil {
		return out, safe(ctx, e)
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	param, e := p.deps.upload(ctx, ciphertext, loc.UploadParam, loc.UploadFullURL, fileHex)
	if e != nil {
		return out, safe(ctx, e)
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	// The real CDN enforces this bound on its response header before returning.
	if !opaque(param, 4096) {
		return out, media.ErrInvalid
	}
	// The defer checks owned ciphertext Close and cancellation before publication.
	return weixin.ImageSend{EncryptQueryParam: param, AESKeyHex: keyHex, CiphertextBytes: expected}, nil
}

type digestReader struct {
	reader   io.Reader
	sha, md5 hash.Hash
	bytes    int64
}

func (r *digestReader) Read(p []byte) (int, error) {
	n, e := r.reader.Read(p)
	if n < 0 || n > len(p) {
		return 0, media.ErrIO
	}
	_, _ = r.sha.Write(p[:n])
	_, _ = r.md5.Write(p[:n])
	r.bytes += int64(n)
	return n, e
}
func validImage(im attachment.LocalImage) bool {
	i := im.Info
	if im.Bytes < 1 || im.Bytes > 4<<20 || len(im.SHA256) != 64 || i.Format != "png" && i.Format != "jpeg" || i.Animated || i.Frames != 1 || i.Width < 1 || i.Width > 8192 || i.Height < 1 || i.Height > 8192 || int64(i.Width)*int64(i.Height) > 16000000 || i.EstimatedDecodeBytes < 1 || i.EstimatedDecodeBytes > 64<<20 {
		return false
	}
	for _, c := range im.SHA256 {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func opaque(s string, max int) bool {
	if len(s) > max || strings.TrimSpace(s) == "" || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func nilDependency(v any) bool {
	if v == nil {
		return true
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(v).IsNil()
	}
	return false
}
func safe(ctx context.Context, e error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, weixin.ErrAuthExpired} {
		if errors.Is(e, known) {
			return known
		}
	}
	return ErrPrepare
}
