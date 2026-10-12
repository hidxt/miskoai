package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/hidxt/miskoai/internal/media"
	"io"
)

// LocalImage contains computed metadata for fully validated immutable bytes.
type LocalImage struct {
	Info   media.ImageInfo
	SHA256 string
	Bytes  int64
}

// WithLocalImage lends a completed private image to consume. The consumer must
// finish actual work before returning and must not retain the borrowed artifact.
// Reads, validation, consume and Close cooperate with cancellation and join
// synchronously before the existing image/document slot releases.
func (p *ImageProcessor) WithLocalImage(ctx context.Context, input io.Reader, size int64, consume func(LocalImage, Artifact) error) error {
	return p.withLocalImage(ctx, input, size, consume, func(ctx context.Context, dir string, input io.Reader, size int64) (Artifact, error) {
		a, e := media.StageLocal(ctx, dir, input, size)
		if a == nil {
			return nil, e
		}
		return a, e
	})
}
func (p *ImageProcessor) withLocalImage(ctx context.Context, input io.Reader, size int64, consume func(LocalImage, Artifact) error, stage func(context.Context, string, io.Reader, int64) (Artifact, error)) (err error) {
	if ctx == nil || input == nil || consume == nil || p == nil || p.slot == nil || p.dataDir == "" || stage == nil {
		return ErrInvalid
	}
	if size < 1 {
		return media.ErrInvalid
	}
	if size > 4<<20 {
		return media.ErrLimit
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	select {
	case p.slot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.slot }()
	if err = ctx.Err(); err != nil {
		return err
	}
	a, stageErr := stage(ctx, p.dataDir, input, size)
	// Register transferred ownership before inspecting any staging error. This
	// cleanup executes before slot release even for an error-with-artifact.
	if a != nil {
		defer func() {
			closeErr := a.Close()
			if cancellation := ctx.Err(); cancellation != nil {
				err = cancellation
			}
			if closeErr != nil {
				err = errors.Join(err, ErrCleanup)
			}
		}()
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if stageErr != nil {
		return safeImageError(stageErr, media.ErrIO)
	}
	if a == nil {
		return media.ErrIO
	}
	if a.Size() != size {
		return media.ErrInvalid
	}
	info, e := media.ValidateImage(ctx, a, size)
	if e != nil {
		return safeImageError(e, media.ErrInvalid)
	}
	h := sha256.New()
	var scratch [4096]byte
	for offset := int64(0); offset < size; {
		if e = ctx.Err(); e != nil {
			return e
		}
		length := int64(len(scratch))
		if size-offset < length {
			length = size - offset
		}
		block := scratch[:int(length)]
		n, readErr := a.ReadAt(block, offset)
		if e = ctx.Err(); e != nil {
			return e
		}
		if n != len(block) || (readErr != nil && !(readErr == io.EOF && offset+length == size)) {
			return media.ErrIO
		}
		_, _ = h.Write(block)
		offset += length
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = consume(LocalImage{Info: info, SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: size}, a); e != nil {
		return safeImageError(e, ErrConsume)
	}
	return ctx.Err()
}
