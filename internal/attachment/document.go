package attachment

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/document"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/storage"
)

// NewProcessor borrows one existing parser and CDN. Images and documents share
// the same persistent outer admission through actual consumer and Close return.
func NewProcessor(dataDir string, cdn *media.CDN, parser *document.Parser) (*ImageProcessor, error) {
	if parser == nil {
		return nil, ErrInvalid
	}
	p, err := NewImageProcessor(dataDir, cdn)
	if err != nil {
		return nil, err
	}
	p.parser = parser
	return p, nil
}

// WithDocument requires prior authorization and a durable message claim. Input
// must be completed and immutable; arbitrary ReaderAt, consumer and Close calls
// must cooperate and actually return. consume finishes actual model work and
// must not retain the borrowed result. No filename becomes a path/model field.
func (p *ImageProcessor) WithDocument(ctx context.Context, d storage.Attachment, consume func(document.Result) error) (err error) {
	if ctx == nil || consume == nil || p == nil || p.download == nil || p.slot == nil || p.parser == nil {
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
	if d.Validate() != nil {
		return ErrInvalid
	}
	if d.Kind != "file" {
		return document.ErrUnsupported
	}
	format, e := documentFormat(d.FileName)
	if e != nil {
		return e
	}
	key, e := media.ParseKey("", d.MediaKey)
	if e != nil {
		return ErrInvalid
	}
	a, downloadErr := p.download(ctx, p.dataDir, d.Query, d.FullURL, key)
	// Transferred ownership is registered before inspecting any download error.
	// This runs before admission release even on error-with-artifact.
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
	if e = ctx.Err(); e != nil {
		return e
	}
	if downloadErr != nil {
		return safeDocumentError(downloadErr, ErrDownload)
	}
	if a == nil {
		return ErrDownload
	}
	size := a.Size()
	if size < 1 {
		return document.ErrInvalid
	}
	if size > 4<<20 {
		return document.ErrLimit
	}
	if d.Length != "" && d.Length != strconv.FormatInt(size, 10) {
		return document.ErrInvalid
	}
	if e = checkDocumentArtifact(ctx, a, size, d.MD5); e != nil {
		return e
	}
	result, e := p.parser.Extract(ctx, format, a, size)
	if e != nil {
		return safeDocumentError(e, document.ErrInvalid)
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = validateDocumentResult(result, format); e != nil {
		return e
	}
	if e = consume(result); e != nil {
		return safeDocumentError(e, ErrConsume)
	}
	return ctx.Err()
}

func documentFormat(name string) (document.Format, error) {
	if strings.ContainsAny(name, "/\\") {
		return "", document.ErrUnsupported
	}
	dot := strings.LastIndexByte(name, '.')
	if dot <= 0 || strings.TrimSpace(name[:dot]) == "" {
		return "", document.ErrUnsupported
	}
	switch strings.ToLower(name[dot+1:]) {
	case "txt":
		return document.TXT, nil
	case "md":
		return document.Markdown, nil
	case "docx":
		return document.DOCX, nil
	case "pdf":
		return document.PDF, nil
	}
	return "", document.ErrUnsupported
}

// Scan the complete bounded artifact before parsing, including when no digest
// was supplied. MD5 is protocol compatibility metadata, never authentication.
func checkDocumentArtifact(ctx context.Context, a Artifact, size int64, digest string) error {
	h := md5.New()
	var scratch [4096]byte
	for offset := int64(0); offset < size; {
		if e := ctx.Err(); e != nil {
			return e
		}
		length := int64(len(scratch))
		if size-offset < length {
			length = size - offset
		}
		block := scratch[:int(length)]
		n, e := a.ReadAt(block, offset)
		if cancellation := ctx.Err(); cancellation != nil {
			return cancellation
		}
		if n != len(block) || (e != nil && !(e == io.EOF && offset+length == size)) {
			return document.ErrInvalid
		}
		if digest != "" {
			_, _ = h.Write(block)
		}
		offset += length
	}
	if digest != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), digest) {
		return document.ErrInvalid
	}
	return ctx.Err()
}

func validateDocumentResult(r document.Result, format document.Format) error {
	if r.Format != format || (r.Format != document.TXT && r.Format != document.Markdown && r.Format != document.DOCX && r.Format != document.PDF) || !utf8.ValidString(r.Text) || r.Pages < 1 {
		return document.ErrInvalid
	}
	if len(r.Text) > 128<<10 || r.Pages > 64 {
		return document.ErrLimit
	}
	if strings.TrimSpace(r.Text) == "" {
		return document.ErrEmpty
	}
	return nil
}

func safeDocumentError(err, fallback error) error {
	// Test identity, never return arbitrary wrapper or reader/remote body text.
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, document.ErrCanceled, document.ErrInvalid, document.ErrLimit, document.ErrUnsupported, document.ErrEmpty} {
		if errors.Is(err, known) {
			return known
		}
	}
	return safeImageError(err, fallback)
}
