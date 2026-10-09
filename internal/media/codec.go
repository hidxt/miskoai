// Package media implements bounded AES128ECB/PKCS7 wire compatibility.
// ECB provides no authentication. Completed bytes still require separate media
// format validation before downstream use.
package media

import (
	"bufio"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

const maxPlaintext = 4 << 20
const maxCiphertext = maxPlaintext + 16

var (
	ErrInvalid = errors.New("media invalid")
	ErrLimit   = errors.New("media limit")
	ErrPrivate = errors.New("media private")
	ErrIO      = errors.New("media IO")
)

// ParseKey prefers the image hex spelling and never falls back on its failure.
// The fallback accepts canonical standard base64 of raw16 or ASCII hex32.
func ParseKey(imageHex, mediaBase64 string) ([16]byte, error) {
	var key [16]byte
	decodeHex := func(s string) ([16]byte, error) {
		var k [16]byte
		if len(s) != 32 {
			return k, ErrInvalid
		}
		n, e := hex.Decode(k[:], []byte(s))
		if e != nil || n != 16 {
			return [16]byte{}, ErrInvalid
		}
		return k, nil
	}
	if imageHex != "" {
		return decodeHex(imageHex)
	}
	if len(mediaBase64) != 24 && len(mediaBase64) != 44 {
		return key, ErrInvalid
	}
	raw, e := base64.StdEncoding.Strict().DecodeString(mediaBase64)
	if e != nil || base64.StdEncoding.EncodeToString(raw) != mediaBase64 {
		return key, ErrInvalid
	}
	switch len(raw) {
	case 16:
		copy(key[:], raw)
		return key, nil
	case 32:
		return decodeHex(string(raw))
	default:
		return key, ErrInvalid
	}
}

// Encrypt accepts at most 4MiB of actual plaintext, including empty input.
// Cancellation is cooperative between Reader calls; a blocked Reader is not
// interrupted and no goroutine is abandoned.
func Encrypt(ctx context.Context, dataDir string, input io.Reader, key [16]byte) (*Artifact, error) {
	return transform(ctx, dataDir, input, key, false, spoolOps{})
}

// Decrypt holds the final block until strict PKCS7 verification. No partial
// plaintext Artifact is published on failure.
func Decrypt(ctx context.Context, dataDir string, input io.Reader, key [16]byte) (*Artifact, error) {
	return transform(ctx, dataDir, input, key, true, spoolOps{})
}
func transform(ctx context.Context, dir string, input io.Reader, key [16]byte, decrypt bool, ops spoolOps) (artifact *Artifact, err error) {
	if ctx == nil || input == nil {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	s, e := newSpool(dir, ops)
	if e != nil {
		return nil, e
	}
	defer func() {
		if artifact == nil {
			s.cleanup()
		}
	}()
	block, _ := aes.NewCipher(key[:]) // A fixed16-byte key always satisfies aes.
	writer := bufio.NewWriterSize(spoolWriter{s}, 16384)
	var inputBuffer [16384]byte
	var pending, held [16]byte
	pendingN := 0
	haveHeld := false
	var total, out int64
	inputLimit := int64(maxPlaintext)
	outputLimit := int64(maxCiphertext)
	if decrypt {
		inputLimit = maxCiphertext
		outputLimit = maxPlaintext
	}
	emit := func(p []byte) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if int64(len(p)) > outputLimit-out {
			return ErrLimit
		}
		n, e := writer.Write(p)
		if e != nil || n != len(p) {
			return ErrIO
		}
		out += int64(n)
		return nil
	}
	emptyReads := 0
	for {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		// Read only up to the first excess byte, even for a limitless Reader.
		readN := len(inputBuffer)
		if remain := inputLimit - total + 1; int64(readN) > remain {
			readN = int(remain)
		}
		n, readErr := input.Read(inputBuffer[:readN])
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if n < 0 || n > readN {
			return nil, ErrIO
		}
		total += int64(n)
		if total > inputLimit {
			return nil, ErrLimit
		}
		if readErr != nil && readErr != io.EOF {
			return nil, ErrIO
		}
		if n == 0 && readErr == nil {
			emptyReads++
			if emptyReads >= 100 {
				return nil, ErrIO
			}
		} else {
			emptyReads = 0
		}
		for offset := 0; offset < n; {
			take := 16 - pendingN
			if take > n-offset {
				take = n - offset
			}
			copy(pending[pendingN:], inputBuffer[offset:offset+take])
			pendingN += take
			offset += take
			if pendingN != 16 {
				continue
			}
			if decrypt {
				if haveHeld {
					if e = emit(held[:]); e != nil {
						return nil, e
					}
				}
				block.Decrypt(held[:], pending[:])
				haveHeld = true
			} else {
				block.Encrypt(pending[:], pending[:])
				if e = emit(pending[:]); e != nil {
					return nil, e
				}
			}
			pendingN = 0
		}
		if readErr == io.EOF {
			break
		}
	}
	if decrypt {
		if pendingN != 0 || !haveHeld {
			return nil, ErrInvalid
		}
		pad := int(held[15])
		if pad < 1 || pad > 16 {
			return nil, ErrInvalid
		}
		for _, b := range held[16-pad:] {
			if int(b) != pad {
				return nil, ErrInvalid
			}
		}
		if e = emit(held[:16-pad]); e != nil {
			return nil, e
		}
	} else {
		pad := byte(16 - pendingN)
		for i := pendingN; i < 16; i++ {
			pending[i] = pad
		}
		block.Encrypt(pending[:], pending[:])
		if e = emit(pending[:]); e != nil {
			return nil, e
		}
	}
	if e = writer.Flush(); e != nil {
		return nil, ErrIO
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return s.publish(ctx, out)
}
