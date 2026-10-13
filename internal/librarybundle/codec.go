package librarybundle

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
)

type Kind uint8

const (
	Database Kind = 1
	Image    Kind = 2
)

type Record struct {
	Kind   Kind
	Key    string
	Bytes  int64
	SHA256 string
}

var (
	ErrInvalid = errors.New("invalid library bundle")
	ErrIO      = errors.New("library bundle I/O failed")
	ErrCleanup = errors.New("library bundle cleanup failed")
)

// Decode lends unverified payload bytes to a synchronous staging callback.
// A callback must finish its private stage Sync/Close before returning. Complete
// success verifies envelope bytes only; snapshot/image validation and destination
// installation belong to the caller. The input stream remains borrowed.
func Decode(ctx context.Context, input io.Reader, consume func(Record, io.Reader) error) (Header, error) {
	if ctx == nil || input == nil || consume == nil {
		return Header{}, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return Header{}, e
	}
	var prefix [len(magic) + 4]byte
	if e := readExact(ctx, input, prefix[:]); e != nil {
		return Header{}, e
	}
	if string(prefix[:len(magic)]) != magic {
		return Header{}, ErrInvalid
	}
	n := binary.LittleEndian.Uint32(prefix[len(magic):])
	if n < 1 || n > maxHeader {
		return Header{}, ErrInvalid
	}
	b := make([]byte, int(n))
	if e := readExact(ctx, input, b); e != nil {
		return Header{}, e
	}
	h, e := parseHeader(b)
	if e != nil {
		return Header{}, e
	}
	for i := 0; i <= len(h.Assets); i++ {
		if e := ctx.Err(); e != nil {
			return Header{}, e
		}
		r := record(h, i)
		stream := newPayload(ctx, input, r.Bytes)
		callbackErr := consume(r, stream)
		// A callback cannot mask a failed read by returning nil or a different error.
		e = errors.Join(safe(ctx, callbackErr, ErrIO), stream.err)
		if e != nil {
			return Header{}, e
		}
		if stream.remaining != 0 || stream.digest() != r.SHA256 {
			return Header{}, ErrInvalid
		}
	}
	if e := probeEOF(ctx, input); e != nil {
		return Header{}, e
	}
	return h, nil
}

// Encode streams only an admitted, copied header and checks actual source bytes.
// Each source release is joined exactly once, including open failures. Output is
// borrowed; partial output on any failure is an uncommitted artifact.
func Encode(ctx context.Context, output io.Writer, header Header, open func(Record) (io.Reader, func() error, error)) error {
	if ctx == nil || output == nil || open == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	h, e := admit(header)
	if e != nil {
		return e
	}
	b, e := json.Marshal(h)
	if e != nil || len(b) < 1 || len(b) > maxHeader {
		return ErrInvalid
	}
	var prefix [len(magic) + 4]byte
	copy(prefix[:], magic)
	binary.LittleEndian.PutUint32(prefix[len(magic):], uint32(len(b)))
	if e := write(ctx, output, prefix[:]); e != nil {
		return e
	}
	if e := write(ctx, output, b); e != nil {
		return e
	}
	var scratch [4096]byte
	for i := 0; i <= len(h.Assets); i++ {
		if e := ctx.Err(); e != nil {
			return e
		}
		r := record(h, i)
		input, release, openErr := open(r)
		e = safe(ctx, openErr, ErrIO)
		if e == nil {
			if input == nil || release == nil {
				e = ErrInvalid
			} else {
				e = encodeRecord(ctx, output, input, r, scratch[:])
			}
		}
		if release != nil {
			releaseErr := release()
			if releaseErr != nil {
				e = errors.Join(e, ErrCleanup, safe(ctx, releaseErr, ErrCleanup))
			}
		}
		// Cancellation occurring during a successful release is still observable.
		e = errors.Join(e, ctx.Err())
		if e != nil {
			return e
		}
	}
	return nil
}
func record(h Header, i int) Record {
	if i == 0 {
		return Record{Kind: Database, Bytes: h.Database.Bytes, SHA256: h.Database.SHA256}
	}
	a := h.Assets[i-1]
	return Record{Kind: Image, Key: a.Key, Bytes: a.Bytes, SHA256: a.SHA256}
}

// safe preserves only documented classes and cancellation, never arbitrary text.
func safe(ctx context.Context, e, errorClass error) error {
	if e == nil {
		return ctx.Err()
	}
	var parts []error
	if errors.Is(e, context.Canceled) {
		parts = append(parts, context.Canceled)
	}
	if errors.Is(e, context.DeadlineExceeded) {
		parts = append(parts, context.DeadlineExceeded)
	}
	for _, class := range []error{ErrInvalid, ErrIO, ErrCleanup} {
		if errors.Is(e, class) {
			parts = append(parts, class)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, errorClass)
	}
	parts = append(parts, ctx.Err())
	return errors.Join(parts...)
}
func write(ctx context.Context, w io.Writer, p []byte) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	n, e := w.Write(p)
	if e != nil {
		return safe(ctx, e, ErrIO)
	}
	if n != len(p) {
		return errors.Join(ErrIO, ctx.Err())
	}
	return ctx.Err()
}
func readExact(ctx context.Context, r io.Reader, p []byte) error {
	for len(p) > 0 {
		if e := ctx.Err(); e != nil {
			return e
		}
		n, e := r.Read(p)
		if n < 0 || n > len(p) {
			return ErrIO
		}
		p = p[n:]
		if e != nil {
			if e == io.EOF {
				if len(p) == 0 {
					return ctx.Err()
				}
				return errors.Join(ErrInvalid, ctx.Err())
			}
			return safe(ctx, e, ErrIO)
		}
		if ce := ctx.Err(); ce != nil {
			return ce
		}
		if n == 0 {
			return ErrIO
		}
	}
	return nil
}
func probeEOF(ctx context.Context, r io.Reader) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	var b [1]byte
	n, e := r.Read(b[:])
	if n < 0 || n > 1 {
		return errors.Join(ErrIO, ctx.Err())
	}
	if e != nil && e != io.EOF {
		return safe(ctx, e, ErrIO)
	}
	if n != 0 {
		return errors.Join(ErrInvalid, ctx.Err())
	}
	if e != io.EOF {
		return errors.Join(ErrIO, ctx.Err())
	}
	return ctx.Err()
}

type payloadReader struct {
	ctx       context.Context
	input     io.Reader
	remaining int64
	sum       hash.Hash
	err       error
}

func newPayload(ctx context.Context, input io.Reader, n int64) *payloadReader {
	return &payloadReader{ctx: ctx, input: input, remaining: n, sum: sha256.New()}
}
func (r *payloadReader) Read(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	if e := r.ctx.Err(); e != nil {
		r.err = e
		return 0, e
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:int(r.remaining)]
	}
	n, e := r.input.Read(p)
	if n < 0 || n > len(p) {
		r.err = ErrIO
		return 0, r.err
	}
	if n > 0 {
		_, _ = r.sum.Write(p[:n])
		r.remaining -= int64(n)
	}
	if e == io.EOF {
		if r.remaining != 0 {
			r.err = ErrInvalid
		}
	} else if e != nil {
		r.err = safe(r.ctx, e, ErrIO)
	} else if n == 0 {
		r.err = ErrIO
	}
	r.err = errors.Join(r.err, r.ctx.Err())
	if r.err != nil {
		return n, r.err
	}
	return n, e
}
func (r *payloadReader) digest() string { return hex.EncodeToString(r.sum.Sum(nil)) }
func encodeRecord(ctx context.Context, w io.Writer, input io.Reader, record Record, scratch []byte) error {
	r := newPayload(ctx, input, record.Bytes)
	for r.remaining > 0 {
		n, e := r.Read(scratch)
		if e != nil && e != io.EOF {
			return e
		}
		if n > 0 {
			if e := write(ctx, w, scratch[:n]); e != nil {
				return e
			}
		}
	}
	if r.digest() != record.SHA256 {
		return ErrInvalid
	}
	return probeEOF(ctx, input)
}
