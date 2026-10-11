// Package attachment owns synchronous processing of durably claimed attachments.
package attachment

import (
	"context"
	"errors"
	"io"

	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/netx"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

var (
	ErrInvalid  = errors.New("attachment invalid")
	ErrDownload = errors.New("attachment download")
	ErrEncode   = errors.New("attachment encode")
	ErrConsume  = errors.New("attachment consumer")
	ErrCleanup  = errors.New("attachment cleanup")
)

// Artifact is a completed immutable private artifact. Ownership transfers even
// when a downloader also returns an error. It exposes no path or key.
type Artifact interface {
	io.ReaderAt
	Size() int64
	Close() error
}

type imageDownloader func(context.Context, string, string, string, [16]byte) (Artifact, error)

// ImageProcessor is reused for a Controller's entire lifetime, including across
// generations. The owner supplies an already admitted private directory and
// existing CDN; this constructor creates no network or model pool.
type ImageProcessor struct {
	dataDir  string
	download imageDownloader
	slot     chan struct{}
}

func NewImageProcessor(dataDir string, cdn *media.CDN) (*ImageProcessor, error) {
	if cdn == nil {
		return nil, ErrInvalid
	}
	return newImageProcessor(dataDir, func(ctx context.Context, dir, query, url string, key [16]byte) (Artifact, error) {
		a, err := cdn.Download(ctx, dir, query, url, key)
		if a == nil {
			return nil, err
		}
		return a, err
	})
}

func newImageProcessor(dataDir string, download imageDownloader) (*ImageProcessor, error) {
	if dataDir == "" || download == nil {
		return nil, ErrInvalid
	}
	return &ImageProcessor{dataDir: dataDir, download: download, slot: make(chan struct{}, 1)}, nil
}

// WithImage requires upstream authorization and a durable message claim. The
// caller supplies its bounded total context (currently 90s). CDN and full image
// validation retain their own existing subdeadlines. No detached worker or
// forced interruption is used: arbitrary reads, consume and Close must cooperate
// and actually return before this call releases admission. consume must finish
// actual model work before returning and must not retain the ContentPart.
func (p *ImageProcessor) WithImage(ctx context.Context, descriptor storage.Attachment, consume func(provider.ContentPart) error) (err error) {
	if ctx == nil || consume == nil || p == nil || p.download == nil || p.slot == nil {
		return ErrInvalid
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
	if descriptor.Validate() != nil {
		return ErrInvalid
	}
	if descriptor.Kind != "image" {
		return media.ErrUnsupported
	}
	key, e := media.ParseKey(descriptor.ImageHex, descriptor.MediaKey)
	if e != nil {
		return ErrInvalid
	}
	artifact, downloadErr := p.download(ctx, p.dataDir, descriptor.Query, descriptor.FullURL, key)
	// Register transferred ownership before inspecting the downloader's error.
	// This defer executes before slot release, even for error-with-artifact.
	if artifact != nil {
		defer func() {
			closeErr := artifact.Close()
			if cancellation := ctx.Err(); cancellation != nil {
				err = cancellation
			}
			if closeErr != nil {
				err = errors.Join(err, ErrCleanup)
			}
		}()
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if downloadErr != nil {
		return safeImageError(downloadErr, ErrDownload)
	}
	if artifact == nil {
		return ErrDownload
	}
	size := artifact.Size()
	if size < 1 {
		return media.ErrInvalid
	}
	if size > 4<<20 {
		return media.ErrLimit
	}
	info, e := media.ValidateImage(ctx, artifact, size)
	if e != nil {
		return safeImageError(e, media.ErrInvalid)
	}
	part, e := provider.InlineImage(ctx, artifact, size, info.Format)
	if e != nil {
		return safeImageError(e, ErrEncode)
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = consume(part); e != nil {
		return safeImageError(e, ErrConsume)
	}
	return ctx.Err()
}

// Return only freshly selected fixed classes, never the arbitrary original
// error or its wrapper text. Copy known HTTP status rather than its wrapper.
func safeImageError(err, fallback error) error {
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, media.ErrInvalid, media.ErrLimit, media.ErrPrivate, media.ErrIO, media.ErrUnsupported, media.ErrEndpoint, media.ErrHTTP, media.ErrTransport, media.ErrClosed, netx.ErrLimit} {
		if errors.Is(err, known) {
			return known
		}
	}
	var remote *netx.HTTPError
	if errors.As(err, &remote) && remote != nil {
		switch remote.Status {
		case 400, 401, 402, 403, 422, 429, 500, 502, 503, 504:
			return &netx.HTTPError{Status: remote.Status}
		}
	}
	return fallback
}
