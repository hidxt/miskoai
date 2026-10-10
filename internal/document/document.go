// Package document extracts bounded text from immutable completed local inputs.
// Input names and URLs are never interpreted as paths. Extracted text is external
// evidence; callers must keep it separate from genuine user instructions.
package document

import (
	"context"
	"errors"
	"io"
	"reflect"
	"time"
)

type Format string

const (
	TXT      Format = "txt"
	Markdown Format = "md"
	DOCX     Format = "docx"
	PDF      Format = "pdf"
)

type Result struct {
	Text   string
	Pages  int
	Format Format
}

var (
	ErrInvalid     = errors.New("document invalid")
	ErrLimit       = errors.New("document limit exceeded")
	ErrUnsupported = errors.New("document unsupported")
	ErrEmpty       = errors.New("document empty")
	ErrCanceled    = errors.New("document canceled")
)

const (
	maxInputBytes  int64 = 4 * 1024 * 1024
	maxOutputBytes       = 128 * 1024
	parseTimeout         = 2 * time.Second
)

// Parser owns one admission slot. A future Core must reuse one Parser rather
// than construct one per request. The zero value is not initialized; use New.
type Parser struct{ admission chan struct{} }

func New() *Parser { return &Parser{admission: make(chan struct{}, 1)} }

// Extract requires an immutable completed private local input. Readers must be
// nonblocking or independently honor caller cancellation. Its cooperative
// deadline includes admission waiting and synchronous ReadAt. An arbitrary
// blocked ReaderAt cannot be forcibly interrupted: admission stays occupied
// until the actual read returns, without detached workers or early release.
func (p *Parser) Extract(ctx context.Context, format Format, input io.ReaderAt, size int64) (Result, error) {
	entered := time.Now()
	if ctx == nil || p == nil || p.admission == nil || nilReader(input) || size < 0 {
		return Result{}, ErrInvalid
	}
	if size > maxInputBytes {
		return Result{}, ErrLimit
	}
	if format != TXT && format != Markdown {
		return Result{}, ErrUnsupported
	}
	ctx, cancel := context.WithDeadline(ctx, entered.Add(parseTimeout))
	defer cancel()
	if err := checkWork(ctx); err != nil {
		return Result{}, err
	}
	select {
	case p.admission <- struct{}{}:
	case <-ctx.Done():
		return Result{}, canceled(ctx.Err())
	}
	defer func() { <-p.admission }()
	if err := checkWork(ctx); err != nil {
		return Result{}, err
	}
	text, err := extractText(ctx, checkedReaderAt{input: input, size: size})
	if err != nil {
		return Result{}, err
	}
	if err := checkWork(ctx); err != nil {
		return Result{}, err
	}
	return Result{Text: text, Pages: 1, Format: format}, nil
}

func nilReader(input io.ReaderAt) bool {
	if input == nil {
		return true
	}
	v := reflect.ValueOf(input)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

type canceledError struct{ cause error }

func (canceledError) Error() string          { return "document canceled" }
func (e canceledError) Is(target error) bool { return target == ErrCanceled || target == e.cause }
func canceled(cause error) error             { return canceledError{cause: cause} }
func checkWork(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return canceled(err)
	}
	return nil
}

// checkedReaderAt refuses out-of-range work and discards all reader error text.
// A full final ReadAt accompanied by EOF is valid; any other error or short/
// dishonest count fails instead of exposing a successful prefix.
type checkedReaderAt struct {
	input io.ReaderAt
	size  int64
}

func (r checkedReaderAt) read(ctx context.Context, dst []byte, offset int64) error {
	if err := checkWork(ctx); err != nil {
		return err
	}
	if offset < 0 || offset > r.size || int64(len(dst)) > r.size-offset {
		return ErrInvalid
	}
	n, err := r.input.ReadAt(dst, offset)
	if workErr := checkWork(ctx); workErr != nil {
		return workErr
	}
	if n != len(dst) || (err != nil && err != io.EOF) {
		return ErrInvalid
	}
	return nil
}

func appendOutput(ctx context.Context, output []byte, text []byte) ([]byte, error) {
	if err := checkWork(ctx); err != nil {
		return nil, err
	}
	if len(text) > maxOutputBytes-len(output) {
		return nil, ErrLimit
	}
	return append(output, text...), nil
}
