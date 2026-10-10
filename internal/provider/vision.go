package provider

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// InlineImage encodes an immutable completed image already accepted by media
// validation. It does not validate format contents or send a model request.
// Cancellation is cooperative: an arbitrary blocked ReaderAt must return before
// this helper can observe cancellation. No detached reader worker is created.
func InlineImage(ctx context.Context, input io.ReaderAt, size int64, format string) (ContentPart, error) {
	if ctx == nil || input == nil || size < 1 || size > 4<<20 {
		return ContentPart{}, errors.New("invalid inline image input")
	}
	if err := ctx.Err(); err != nil {
		return ContentPart{}, err
	}
	var prefix string
	switch format {
	case "png":
		prefix = "data:image/png;base64,"
	case "jpeg":
		prefix = "data:image/jpeg;base64,"
	case "gif":
		prefix = "data:image/gif;base64,"
	default:
		return ContentPart{}, errors.New("invalid inline image format")
	}
	// The input cap makes conversion and EncodedLen safe even on 32-bit hosts;
	// check the final addition before Grow as well.
	if size > int64(int(^uint(0)>>1))/4*3 {
		return ContentPart{}, errors.New("invalid inline image bounds")
	}
	encoded := base64.StdEncoding.EncodedLen(int(size))
	if encoded < 0 || encoded > (6<<20)-len(prefix) {
		return ContentPart{}, errors.New("invalid inline image bounds")
	}
	var builder strings.Builder
	builder.Grow(len(prefix) + encoded)
	builder.WriteString(prefix)
	encoder := base64.NewEncoder(base64.StdEncoding, &builder)
	reader := io.NewSectionReader(&visionContextReaderAt{ctx: ctx, reader: input}, 0, size)
	copied, err := io.CopyBuffer(encoder, reader, make([]byte, 32<<10))
	closeErr := encoder.Close()
	if ctx.Err() != nil {
		return ContentPart{}, ctx.Err()
	}
	if err != nil || closeErr != nil || copied != size || builder.Len() != len(prefix)+encoded {
		return ContentPart{}, errors.New("inline image read failure")
	}
	return ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: builder.String(), Detail: "low"}}, nil
}

type visionContextReaderAt struct {
	ctx    context.Context
	reader io.ReaderAt
}

func (r *visionContextReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.ReadAt(p, off)
	if cancelErr := r.ctx.Err(); cancelErr != nil {
		return 0, cancelErr
	}
	if n < 0 || n > len(p) || n < len(p) && err == nil {
		return 0, errors.New("inline image read failure")
	}
	if err != nil && err != io.EOF {
		return 0, errors.New("inline image read failure")
	}
	return n, err
}

func validImageDetail(detail string) bool {
	return detail == "" || detail == "low" || detail == "high" || detail == "auto"
}

// canonicalInlineURL checks syntax only; full image validation remains the
// caller's responsibility. It scans bounded input without decoding or allocating
// a plaintext/base64 copy, and excludes bytes that expand during JSON escaping.
func canonicalInlineURL(url string) bool {
	var payload string
	for _, prefix := range []string{"data:image/png;base64,", "data:image/jpeg;base64,", "data:image/gif;base64,"} {
		if strings.HasPrefix(url, prefix) {
			payload = url[len(prefix):]
			break
		}
	}
	if len(payload) == 0 || len(payload)%4 != 0 {
		return false
	}
	end := len(payload)
	padding := 0
	if payload[end-1] == '=' {
		padding++
		end--
		if payload[end-1] == '=' {
			padding++
			end--
		}
	}
	for i := 0; i < end; i++ {
		if base64Value(payload[i]) < 0 {
			return false
		}
	}
	// Canonical padding has zero unused low bits in the last actual sextet.
	last := base64Value(payload[end-1])
	return padding == 0 || padding == 1 && last&3 == 0 || padding == 2 && last&15 == 0
}

func base64Value(b byte) int {
	switch {
	case b >= 'A' && b <= 'Z':
		return int(b - 'A')
	case b >= 'a' && b <= 'z':
		return int(b-'a') + 26
	case b >= '0' && b <= '9':
		return int(b-'0') + 52
	case b == '+':
		return 62
	case b == '/':
		return 63
	default:
		return -1
	}
}
