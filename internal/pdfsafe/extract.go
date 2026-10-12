// Package pdfsafe exposes only bounded synchronous extraction of supported text.
package pdfsafe

import (
	"context"
	"io"
	"reflect"
	"unicode/utf8"
)

var (
	ErrInvalid     = errMalformed
	ErrLimit       = errLimit
	ErrUnsupported = errUnsupported
)

// Extract requires immutable, completed input. ReaderAt must return before the
// caller can release admission; cancellation never detaches an actual read.
// Every fault discards all text and the page count.
func Extract(ctx context.Context, input io.ReaderAt, size int64) (text string, pages int, err error) {
	var b *budget
	defer func() {
		if p := recover(); p != nil {
			if f, ok := p.(fault); ok {
				err = f.err
			} else {
				err = errMalformed
			}
		}
		if b != nil {
			if e := b.closeStreams(); e != nil && err == nil {
				err = e
			}
		}
		if err != nil {
			text = ""
			pages = 0
		}
	}()
	if ctx == nil || input == nil || size < 0 {
		return "", 0, errMalformed
	}
	rv := reflect.ValueOf(input)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		if rv.IsNil() {
			return "", 0, errMalformed
		}
	}
	if size > maxInput {
		return "", 0, errLimit
	}
	if size < 10 {
		return "", 0, errMalformed
	}
	r, e := parse(ctx, input, size)
	if e != nil {
		return "", 0, e
	}
	b = r.b
	x := newExtraction(r)
	root := r.trailerValue().key("Root")
	if root.kind() != dictKind || root.key("Type").name() != "Catalog" {
		fail(errMalformed)
	}
	x.walkPages(root.key("Pages"), value{}, 0)
	if x.pages == 0 || len(x.output) == 0 {
		return "", 0, errUnsupported
	}
	b.check()
	b.retain(int64(len(x.output)))
	text = string(x.output)
	if !utf8.ValidString(text) {
		fail(errMalformed)
	}
	b.check()
	return text, x.pages, nil
}

func (x *extraction) append(s string) {
	x.r.b.charge(int64(len(s)) + 1)
	if len(s) > maxOutput-len(x.output) {
		fail(errLimit)
	}
	if len(s) > cap(x.output)-len(x.output) {
		n := cap(x.output) * 2
		if n < 64 {
			n = 64
		}
		if n < len(x.output)+len(s) {
			n = len(x.output) + len(s)
		}
		if n > maxOutput {
			n = maxOutput
		}
		x.r.b.retain(int64(n))
		x.r.b.charge(int64(len(x.output)))
		v := make([]byte, len(x.output), n)
		copy(v, x.output)
		x.output = v
	}
	x.output = append(x.output, s...)
}
