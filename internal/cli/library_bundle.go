package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
)

const libraryCLILimit int64 = 321 << 20

var errLibraryCLI = errors.New("complete library maintenance failed; operator reconciliation may be required")
var errLibraryCleanup = errors.New("complete library cleanup failed; operator reconciliation may be required")

type libraryCLIController interface {
	PrepareLibraryBundle(context.Context) (*core.Download, error)
	RestoreLibraryBundle(context.Context, io.Reader) (maintenance.RestoreResult, error)
	Close() error
}

// All seams belong to one invocation. Defaults construct the actual stopped
// Core and use original handles; no Run, listener, worker or probe is started.
type libraryCLIOperations struct {
	newCore      func(config.Config) (libraryCLIController, error)
	wrapDownload func(*core.Download) io.ReadCloser
	wrapSource   func(*os.File) io.Reader
	write        func(*os.File, []byte) (int, error)
	sync         func(*os.File) error
	close        func(*os.File) error
	syncDir      func(string) error
}

func libraryCLIOptions() libraryCLIOperations {
	return libraryCLIOperations{
		newCore:      func(c config.Config) (libraryCLIController, error) { return core.New(c, core.Dependencies{}) },
		wrapDownload: func(d *core.Download) io.ReadCloser { return d },
		wrapSource:   func(f *os.File) io.Reader { return f },
		write:        func(f *os.File, p []byte) (int, error) { return f.Write(p) },
		sync:         func(f *os.File) error { return f.Sync() }, close: func(f *os.File) error { return f.Close() },
		syncDir: maintenance.SyncPrivateDirectory,
	}
}
func libraryCLIError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	var canceled error
	for _, known := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(e, known) {
			canceled = errors.Join(canceled, known)
		}
	}
	return errors.Join(errLibraryCLI, canceled, ctx.Err())
}
func libraryCLIPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.HasPrefix(path, "--") {
		return "", errLibraryCLI
	}
	p, e := privatefs.Resolve(path)
	if e != nil {
		return "", errLibraryCLI
	}
	return p, nil
}
func librarySame(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
func libraryIdentity(path string, owned os.FileInfo) error {
	f, e := privatefs.OpenRead(path, libraryCLILimit)
	if e != nil {
		return errLibraryCLI
	}
	info, e := f.Stat()
	ce := f.Close()
	named, ne := os.Lstat(path)
	if e != nil || ce != nil || ne != nil || !librarySame(owned, info) || !librarySame(info, named) {
		return errLibraryCLI
	}
	return nil
}
func libraryRemove(path string, owned os.FileInfo, syncDir func(string) error) error {
	named, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil || owned == nil || !os.SameFile(owned, named) || privatefs.CheckFile(path, libraryCLILimit) != nil || os.Remove(path) != nil {
		return errLibraryCleanup
	}
	if syncDir(filepath.Dir(path)) != nil {
		return errLibraryCleanup
	}
	return nil
}

func backupLibrary(parent context.Context, cfg config.Config, dest string, out io.Writer, ops libraryCLIOperations) (err error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	path, e := libraryCLIPath(dest)
	if e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Admit and exclusively create the destination before Core may copy data.
	f, e := privatefs.Create(path)
	if e != nil {
		return errLibraryCLI
	}
	owned, e := f.Stat()
	complete := false
	defer func() {
		if f != nil && ops.close(f) != nil {
			err = errors.Join(err, errLibraryCleanup)
		}
		if !complete {
			err = errors.Join(err, libraryRemove(path, owned, ops.syncDir))
		}
		err = errors.Join(err, ctx.Err())
	}()
	if e != nil {
		return errLibraryCLI
	}
	ctrl, e := ops.newCore(cfg)
	if e != nil {
		return libraryCLIError(ctx, e)
	}
	defer func() {
		if ctrl.Close() != nil {
			err = errors.Join(err, errLibraryCleanup)
		}
	}()
	d, e := ctrl.PrepareLibraryBundle(ctx)
	if e != nil {
		return libraryCLIError(ctx, e)
	}
	input := ops.wrapDownload(d)
	closed := false
	defer func() {
		if !closed && input.Close() != nil {
			err = errors.Join(err, errLibraryCleanup)
		}
	}()
	if d.Filename != "miskoai-library.bundle" || d.Size <= 0 || d.Size > libraryCLILimit {
		return errLibraryCLI
	}
	if e = copyLibrary(ctx, input, f, d.Size, ops.write); e != nil {
		return libraryCLIError(ctx, e)
	}
	e = input.Close()
	closed = true
	if e != nil {
		return errors.Join(errLibraryCLI, errLibraryCleanup)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if ops.sync(f) != nil {
		return errLibraryCLI
	}
	final, e := f.Stat()
	if e != nil || final.Size() != d.Size || !os.SameFile(owned, final) {
		return errLibraryCLI
	}
	e = ops.close(f)
	f = nil
	if e != nil {
		return errors.Join(errLibraryCLI, errLibraryCleanup)
	}
	if ops.syncDir(filepath.Dir(path)) != nil || libraryIdentity(path, final) != nil {
		return errLibraryCLI
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	complete = true
	if _, e = fmt.Fprintln(out, "complete library backup completed; includes all scopes and current image content"); e != nil {
		return libraryCLIError(ctx, e)
	}
	return nil
}

// Fixed scratch and explicit error checks preserve an error accompanying the
// final bytes. io.ReadFull/CopyN could hide that error on exact-length reads.
func copyLibrary(ctx context.Context, input io.Reader, f *os.File, size int64, write func(*os.File, []byte) (int, error)) error {
	scratch := make([]byte, 32<<10)
	remaining := size
	for remaining > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p := scratch
		if remaining < int64(len(p)) {
			p = p[:remaining]
		}
		n, e := input.Read(p)
		if n < 0 || n > len(p) {
			return errLibraryCLI
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if n > 0 {
			written, we := write(f, p[:n])
			if we != nil || written != n {
				return errLibraryCLI
			}
			remaining -= int64(n)
		}
		if e != nil {
			if e == io.EOF && remaining == 0 {
				return ctx.Err()
			}
			return errLibraryCLI
		}
		if n == 0 {
			return errLibraryCLI
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	n, e := input.Read(scratch[:1])
	if n != 0 || e != io.EOF {
		return errLibraryCLI
	}
	return ctx.Err()
}

type librarySourceReader struct {
	ctx       context.Context
	input     io.Reader
	remaining int64
	eof       bool
}

func (r *librarySourceReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var tail [1]byte
		n, e := r.input.Read(tail[:])
		if r.ctx.Err() != nil {
			return 0, r.ctx.Err()
		}
		if n != 0 || e != io.EOF {
			return 0, errLibraryCLI
		}
		r.eof = true
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, e := r.input.Read(p)
	if n < 0 || n > len(p) {
		return 0, errLibraryCLI
	}
	r.remaining -= int64(n)
	if r.ctx.Err() != nil {
		return n, r.ctx.Err()
	}
	if e != nil && e != io.EOF {
		return n, errLibraryCLI
	}
	if e == io.EOF && r.remaining == 0 {
		r.eof = true
	}
	if n == 0 && e == nil {
		return 0, errLibraryCLI
	}
	return n, e
}
func restoreLibrary(parent context.Context, cfg config.Config, source string, out io.Writer, ops libraryCLIOperations) (err error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	path, e := libraryCLIPath(source)
	if e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// The actual owner-private original is admitted before Core construction or
	// its storage migration effects. Keep this same handle through consumption.
	f, e := privatefs.OpenRead(path, libraryCLILimit)
	if e != nil {
		return errLibraryCLI
	}
	defer func() {
		if ops.close(f) != nil {
			err = errors.Join(err, errLibraryCleanup)
		}
		err = errors.Join(err, ctx.Err())
	}()
	owned, e := f.Stat()
	if e != nil || owned.Size() <= 0 || owned.Size() > libraryCLILimit {
		return errLibraryCLI
	}
	ctrl, e := ops.newCore(cfg)
	if e != nil {
		return libraryCLIError(ctx, e)
	}
	defer func() {
		if ctrl.Close() != nil {
			err = errors.Join(err, errLibraryCleanup)
		}
	}()
	reader := &librarySourceReader{ctx: ctx, input: ops.wrapSource(f), remaining: owned.Size()}
	result, e := ctrl.RestoreLibraryBundle(ctx, reader)
	actual, se := f.Stat()
	if se != nil || !librarySame(owned, actual) || libraryIdentity(path, owned) != nil {
		e = errors.Join(e, errLibraryCLI)
	}
	if e != nil {
		return libraryCLIError(ctx, e)
	}
	if reader.remaining != 0 || !reader.eof || ctx.Err() != nil {
		return errors.Join(errLibraryCLI, ctx.Err())
	}
	if _, e = fmt.Fprintf(out, "complete library restore completed; prior_retained=%t; channel_paused=%t; manual operator reconciliation required before resuming channels\n", result.PreviousRetained, result.ChannelPaused); e != nil {
		return libraryCLIError(ctx, e)
	}
	return nil
}
