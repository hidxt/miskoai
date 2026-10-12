package media

import (
	"context"
	"errors"
	"io"
)

// StageLocal stages exactly size private bytes and verifies the input's EOF.
// Reads run synchronously and must actually return on cooperative cancellation.
// The caller owns every returned artifact, even alongside an error, and must
// Close it after actual use. No original filename or untrusted path is accepted.
func StageLocal(ctx context.Context, dir string, input io.Reader, size int64) (*Artifact, error) {
	return stageLocal(ctx, dir, input, size, spoolOps{})
}
func stageLocal(ctx context.Context, dir string, input io.Reader, size int64, ops spoolOps) (artifact *Artifact, err error) {
	if ctx == nil || input == nil || size < 1 {
		return nil, ErrInvalid
	}
	if size > maxPlaintext {
		return nil, ErrLimit
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	s, err := newSpool(dir, ops)
	if err != nil {
		return nil, err
	}
	defer func() {
		if artifact != nil {
			return
		}
		if s.ops.beforeCleanup != nil {
			s.ops.beforeCleanup(s.path)
		}
		// Artifact cleanup reports actual close/remove failures and removes only
		// the captured original. spool.cleanup deliberately has no error result.
		partial := &Artifact{path: s.path, file: s.file, original: s.original, size: size}
		if closeErr := partial.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
			// Preserve ownership/cleanup evidence even with a staging error.
			// Reads are already closed; a later Close returns the fixed refusal.
			artifact = partial
		}
		s.file = nil
	}()
	var scratch [4096]byte
	writer := spoolWriter{s}
	for remaining := size; remaining > 0; {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		length := int64(len(scratch))
		if remaining < length {
			length = remaining
		}
		block := scratch[:int(length)]
		n, readErr := input.Read(block)
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if n < 0 || n > len(block) || (readErr != nil && readErr != io.EOF) {
			return nil, ErrIO
		}
		if n == 0 {
			if readErr == io.EOF {
				return nil, ErrInvalid
			}
			return nil, ErrIO
		}
		written, writeErr := writer.Write(block[:n])
		if writeErr != nil || written != n {
			return nil, ErrIO
		}
		remaining -= int64(n)
		if readErr == io.EOF && remaining != 0 {
			return nil, ErrInvalid
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	n, readErr := input.Read(scratch[:1])
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if n < 0 || n > 1 || (readErr != nil && readErr != io.EOF) {
		return nil, ErrIO
	}
	if n != 0 {
		return nil, ErrInvalid
	}
	if readErr != io.EOF {
		return nil, ErrIO
	}
	return s.publish(ctx, size)
}
