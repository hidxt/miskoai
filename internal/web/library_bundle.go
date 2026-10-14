package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/maintenance"
)

const maxLibraryBundleBytes int64 = 321 << 20

// Optional until the reviewed Core adapter is installed. Restore must consume
// exact EOF before installation and join every borrowed operation before return.
type libraryBundleBackend interface {
	prepareLibraryBundle(context.Context) (managementDownload, error)
	RestoreLibraryBundle(context.Context, io.Reader) (maintenance.RestoreResult, error)
}

func libraryBundleError(w http.ResponseWriter, e error) {
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) || errors.Is(e, library.ErrCleanup) {
		libraryManagementError(w, e)
		return
	}
	if errors.Is(e, core.ErrMaintenance) {
		managementError(w, e)
		return
	}
	libraryManagementError(w, e)
}

func libraryBundleRoute(w http.ResponseWriter, r *http.Request, b managementBackend) {
	lb, ok := b.(libraryBundleBackend)
	if !ok {
		safeError(w, 503, "web_unavailable")
		return
	}
	if r.URL.Path == "/api/library/backup" {
		libraryBundleBackup(w, r, lb)
		return
	}
	values := r.Header.Values("Content-Type")
	if len(values) != 1 || values[0] != "application/octet-stream" || len(r.Header.Values("Content-Encoding")) != 0 || r.Body == nil || r.ContentLength < 1 || r.ContentLength > maxLibraryBundleBytes {
		libraryBundleError(w, errManagementInput)
		return
	}
	if e := r.Context().Err(); e != nil {
		libraryBundleError(w, e)
		return
	}
	input := &libraryBundleBody{input: libraryContextReader{r.Context(), http.MaxBytesReader(w, r.Body, maxLibraryBundleBytes)}, left: r.ContentLength}
	result, e := lb.RestoreLibraryBundle(r.Context(), input)
	if e == nil {
		e = r.Context().Err()
	}
	if e == nil {
		e = input.err
	}
	// Never drain after a potentially mutating return. The trusted backend must
	// request EOF as part of validation before its installation callback.
	if e == nil && !input.complete {
		e = errManagementInput
	}
	if e != nil {
		libraryBundleError(w, e)
		return
	}
	respondJSON(w, struct {
		PriorRetained bool `json:"prior_retained"`
		ChannelPaused bool `json:"channel_paused"`
	}{result.PreviousRetained, result.ChannelPaused})
}

func libraryBundleBackup(w http.ResponseWriter, r *http.Request, b libraryBundleBackend) {
	if len(r.Header.Values("Content-Encoding")) != 0 || r.Body == nil {
		libraryBundleError(w, errManagementInput)
		return
	}
	if _, e := requestObject(r, smallMutationBody); e != nil {
		libraryBundleError(w, e)
		return
	}
	if e := r.Context().Err(); e != nil {
		libraryBundleError(w, e)
		return
	}
	d, e := b.prepareLibraryBundle(r.Context())
	if e != nil {
		if d != nil {
			if ce := d.Close(); ce != nil && !errors.Is(e, context.Canceled) && !errors.Is(e, context.DeadlineExceeded) {
				safeError(w, 500, "web_download")
				return
			}
		}
		libraryBundleError(w, e)
		return
	}
	if d == nil {
		safeError(w, 500, "web_download")
		return
	}
	closed := false
	defer func() {
		if !closed {
			_ = d.Close()
		}
	}()
	name, size := d.metadata()
	if name != "miskoai-library.bundle" || size < 1 || size > maxLibraryBundleBytes {
		closed = true
		_ = d.Close()
		safeError(w, 500, "web_download")
		return
	}
	if e = r.Context().Err(); e != nil {
		closed = true
		ce := d.Close()
		if ce != nil {
			safeError(w, 500, "web_download")
		} else {
			libraryBundleError(w, e)
		}
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="miskoai-library.bundle"`)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	e = copyLibraryBundle(r.Context(), w, d, size)
	closed = true
	ce := d.Close()
	if e != nil || ce != nil || r.Context().Err() != nil {
		panic(http.ErrAbortHandler)
	}
}

// Inspect every actual Read error before exposing its bytes. io.ReadFull drops
// errors when the requested buffer fills, including a final non-EOF failure.
func copyLibraryBundle(ctx context.Context, output io.Writer, input io.Reader, size int64) error {
	var scratch [4096]byte
	for left := size; left > 0; {
		if e := ctx.Err(); e != nil {
			return e
		}
		count := int64(len(scratch))
		if left < count {
			count = left
		}
		n, readErr := input.Read(scratch[:int(count)])
		if e := ctx.Err(); e != nil {
			return e
		}
		if n < 0 || n > int(count) {
			return io.ErrUnexpectedEOF
		}
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if readErr == io.EOF && int64(n) != left {
			return io.ErrUnexpectedEOF
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		written, e := output.Write(scratch[:n])
		if e != nil {
			return e
		}
		if written != n {
			return io.ErrShortWrite
		}
		left -= int64(n)
	}
	return ctx.Err()
}

// Length enforcement is independent of the envelope's own strict EOF checks.
// Only a fixed one-byte probe is read beyond the declared size.
type libraryBundleBody struct {
	input    io.Reader
	left     int64
	complete bool
	err      error
}

func (r *libraryBundleBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	if r.complete {
		return 0, io.EOF
	}
	if r.left == 0 {
		var probe [1]byte
		n, e := r.input.Read(probe[:])
		if n != 0 {
			r.err = errManagementInput
			return 0, r.err
		}
		if e == io.EOF {
			r.complete = true
		} else if e != nil {
			r.err = e
		}
		return 0, e
	}
	if int64(len(p)) > r.left {
		p = p[:int(r.left)]
	}
	n, e := r.input.Read(p)
	r.left -= int64(n)
	if e == io.EOF {
		if r.left != 0 {
			r.err = errManagementInput
			return n, r.err
		}
		r.complete = true
	} else if e != nil {
		r.err = e
	}
	return n, e
}
