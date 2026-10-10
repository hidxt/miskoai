package media

import (
	"bufio"
	"context"
	"errors"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"time"
)

// ImageInfo describes validated contents, without retaining decoded pixels.
type ImageInfo struct {
	Format                string
	Width, Height, Frames int
	Animated              bool
	EstimatedDecodeBytes  int64
}

// ValidateImage validates completed immutable bytes without transferring caller
// ownership. Its synchronous decoder must return before the call returns;
// cancellation cannot forcibly interrupt a blocked ReaderAt or computation.
// EstimatedDecodeBytes is an admission estimate, not a heap/RSS/time guarantee.
func ValidateImage(ctx context.Context, input io.ReaderAt, size int64) (ImageInfo, error) {
	return validateImage(ctx, input, size, nil)
}

var ErrUnsupported = errors.New("media unsupported")

type imageDecoder func(string, io.Reader) (ImageInfo, error)

const imageDecodeLimit int64 = 64 << 20
const imageFixedBytes int64 = 1 << 20

// ValidateImage consumes completed immutable bytes synchronously. The caller
// retains ownership. Cancellation is cooperative; a blocking ReaderAt or pure
// decoder computation must return before this call returns. The estimate bounds
// admitted decoder buffers, not total heap, RSS or a hard two-second lifetime.
func validateImage(ctx context.Context, input io.ReaderAt, size int64, decode imageDecoder) (ImageInfo, error) {
	if ctx == nil || input == nil || size <= 0 {
		return ImageInfo{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return ImageInfo{}, err
	}
	if size > maxPlaintext {
		return ImageInfo{}, ErrLimit
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	reader := &imageReaderAt{ctx: ctx, input: input}
	s := &imageScanner{ctx: ctx, r: bufio.NewReaderSize(io.NewSectionReader(reader, 0, size), 4096), remaining: size}
	sig, err := s.r.Peek(8)
	if err != nil && (!errors.Is(err, io.EOF) || len(sig) < 2) {
		return ImageInfo{}, imageReadError(err)
	}
	var info ImageInfo
	switch {
	case len(sig) >= 8 && string(sig[:8]) == "\x89PNG\r\n\x1a\n":
		info, err = scanPNG(s)
	case len(sig) >= 6 && (string(sig[:6]) == "GIF87a" || string(sig[:6]) == "GIF89a"):
		info, err = scanGIF(s)
	case len(sig) >= 2 && sig[0] == 255 && sig[1] == 216:
		info, err = scanJPEG(s)
	default:
		err = ErrUnsupported
	}
	if ctx.Err() != nil {
		return ImageInfo{}, ctx.Err()
	}
	if reader.failed {
		return ImageInfo{}, ErrIO
	}
	if err != nil {
		return ImageInfo{}, err
	}
	if decode == nil {
		decode = decodeImage
	}
	actual, err := decode(info.Format, io.NewSectionReader(reader, 0, size))
	if ctx.Err() != nil {
		return ImageInfo{}, ctx.Err()
	}
	// Some standard decoders wrap Reader errors as formatted text. Remember
	// the fixed IO classification at the reader boundary instead of exposing
	// or parsing that decoder text, including an ignored terminal read error.
	if reader.failed {
		return ImageInfo{}, ErrIO
	}
	if err != nil {
		if errors.Is(err, ErrIO) {
			return ImageInfo{}, ErrIO
		}
		var pu png.UnsupportedError
		var ju jpeg.UnsupportedError
		if errors.As(err, &pu) || errors.As(err, &ju) {
			return ImageInfo{}, ErrUnsupported
		}
		return ImageInfo{}, ErrInvalid
	}
	if actual.Width != info.Width || actual.Height != info.Height || actual.Frames != info.Frames || actual.Format != info.Format {
		return ImageInfo{}, ErrInvalid
	}
	return info, nil
}

func decodeImage(format string, r io.Reader) (ImageInfo, error) {
	if format == "gif" {
		g, err := gif.DecodeAll(r)
		if err != nil {
			return ImageInfo{}, err
		}
		return ImageInfo{Format: format, Width: g.Config.Width, Height: g.Config.Height, Frames: len(g.Image)}, nil
	}
	if format == "png" {
		m, err := png.Decode(r)
		if err != nil {
			return ImageInfo{}, err
		}
		return ImageInfo{Format: format, Width: m.Bounds().Dx(), Height: m.Bounds().Dy(), Frames: 1}, nil
	}
	m, err := jpeg.Decode(r)
	if err != nil {
		return ImageInfo{}, err
	}
	return ImageInfo{Format: format, Width: m.Bounds().Dx(), Height: m.Bounds().Dy(), Frames: 1}, nil
}

type imageReaderAt struct {
	ctx    context.Context
	input  io.ReaderAt
	failed bool
}

func (r *imageReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.input.ReadAt(p, off)
	if e := r.ctx.Err(); e != nil {
		return n, e
	}
	if err != nil && !errors.Is(err, io.EOF) {
		r.failed = true
		return n, ErrIO
	}
	return n, err
}
func imageReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, ErrIO) {
		return ErrIO
	}
	return ErrInvalid
}

type imageScanner struct {
	ctx       context.Context
	r         *bufio.Reader
	remaining int64
}

func (s *imageScanner) read(p []byte) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if int64(len(p)) > s.remaining {
		return ErrInvalid
	}
	n, err := io.ReadFull(s.r, p)
	s.remaining -= int64(n)
	if err != nil {
		return imageReadError(err)
	}
	return nil
}
func (s *imageScanner) byte() (byte, error) { var p [1]byte; err := s.read(p[:]); return p[0], err }
func (s *imageScanner) skip(n int64) error {
	if n < 0 || n > s.remaining {
		return ErrInvalid
	}
	var buf [4096]byte
	for n > 0 {
		k := min(n, int64(len(buf)))
		if err := s.read(buf[:k]); err != nil {
			return err
		}
		n -= k
	}
	return nil
}
func imageDimensions(w, h int64) error {
	if w <= 0 || h <= 0 {
		return ErrInvalid
	}
	if w > 8192 || h > 8192 || w*h > 16000000 {
		return ErrLimit
	}
	return nil
}
func imageMul(a, b int64) (int64, error) {
	if a < 0 || b < 0 || a != 0 && b > math.MaxInt64/a {
		return 0, ErrLimit
	}
	return a * b, nil
}
func imageAdd(a, b int64) (int64, error) {
	if a < 0 || b < 0 || b > math.MaxInt64-a {
		return 0, ErrLimit
	}
	return a + b, nil
}
func imageEstimate(parts ...int64) (int64, error) {
	var n int64
	for _, p := range parts {
		var err error
		n, err = imageAdd(n, p)
		if err != nil {
			return 0, err
		}
	}
	if n > imageDecodeLimit {
		return 0, ErrLimit
	}
	return n, nil
}

func imageProduct(factors ...int64) (int64, error) {
	n := int64(1)
	for _, factor := range factors {
		var err error
		n, err = imageMul(n, factor)
		if err != nil {
			return 0, err
		}
	}
	return n, nil
}
