package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// Verification is human input only. Raw terminal mode suppresses echo and is
// restored on every return, including cancellation. No argument or log holds it.
func terminalVerification(ctx context.Context) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("verification requires an interactive terminal")
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", errors.New("cannot hide verification input")
	}
	var once sync.Once
	var restoreErr error
	restore := func() { once.Do(func() { restoreErr = term.Restore(fd, state) }) }
	defer restore()
	// Restore while the descriptor is still open, before cancellation closes it.
	input := verificationReader{Reader: os.Stdin, close: func() error { restore(); return os.Stdin.Close() }}
	code, err := readVerification(ctx, input)
	restore()
	if restoreErr != nil {
		return "", errors.New("cannot restore terminal input mode")
	}
	return code, err
}

type verificationReader struct {
	io.Reader
	close func() error
}

func (r verificationReader) Close() error { return r.close() }

func readVerification(ctx context.Context, input io.ReadCloser) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code := make([]byte, 0, 10)
		one := make([]byte, 1)
		for {
			if _, err := io.ReadFull(input, one); err != nil {
				done <- result{err: errors.New("verification input unavailable")}
				return
			}
			switch one[0] {
			case 3:
				done <- result{err: errors.New("verification canceled")}
				return
			case '\r', '\n':
				if len(code) < 4 {
					done <- result{err: errors.New("invalid verification code length")}
				} else {
					done <- result{code: string(code)}
				}
				return
			case 8, 127:
				if len(code) > 0 {
					code = code[:len(code)-1]
				}
			default:
				if one[0] < '0' || one[0] > '9' || len(code) >= 10 {
					done <- result{err: errors.New("invalid verification code")}
					return
				}
				code = append(code, one[0])
			}
		}
	}()
	select {
	case <-ctx.Done():
		// This explicit CLI exits after a canceled login. Closing stdin also wakes
		// a blocked reader; the terminal wrapper restores the previously saved mode.
		input.Close()
		return "", ctx.Err()
	case value := <-done:
		return value.code, value.err
	}
}
