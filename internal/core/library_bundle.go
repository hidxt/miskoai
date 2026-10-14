package core

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
)

const maxLibraryBundleBytes int64 = 321 << 20

// bundleAccess is global administrative access, including stopped/unconfigured
// recovery. It captures no Store and constructs no dependency or generation.
func (c *Controller) bundleAccess(ctx context.Context) (*library.Manager, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.closing || c.unavailable || c.store == nil || c.library == nil {
		return nil, ErrUnavailable
	}
	return c.library, nil
}

type bundleOutputOperations struct {
	write func(*os.File, []byte) (int, error)
	sync  func(*os.File) error
	close func(*os.File) error
	check func(string, int64) error
	open  func(string, int64) (*os.File, error)
}

func defaultBundleOutputOperations() bundleOutputOperations {
	return bundleOutputOperations{func(f *os.File, b []byte) (int, error) { return f.Write(b) }, func(f *os.File) error { return f.Sync() }, func(f *os.File) error { return f.Close() }, privatefs.CheckFile, privatefs.OpenRead}
}

type bundleOutputWriter struct {
	ctx       context.Context
	f         *os.File
	write     func(*os.File, []byte) (int, error)
	remaining int64
}

func (w *bundleOutputWriter) Write(b []byte) (int, error) {
	if e := w.ctx.Err(); e != nil {
		return 0, e
	}
	if int64(len(b)) > w.remaining {
		return 0, library.ErrCapacity
	}
	n, e := w.write(w.f, b)
	if n < 0 || n > len(b) {
		return 0, ErrUnavailable
	}
	w.remaining -= int64(n)
	if ce := w.ctx.Err(); ce != nil {
		return n, ce
	}
	if e != nil || n != len(b) {
		return n, ErrUnavailable
	}
	return n, nil
}

// PrepareLibraryBundle returns only a fully completed private artifact. It
// holds no Store/Manager/maintenance lease after return; Close releases the
// existing download slot. Actual borrowed I/O must cooperate with cancellation.
func (c *Controller) PrepareLibraryBundle(parent context.Context) (*Download, error) {
	return c.prepareLibraryBundle(parent, defaultBundleOutputOperations())
}
func (c *Controller) prepareLibraryBundle(parent context.Context, ops bundleOutputOperations) (d *Download, err error) {
	ctx, cancel, e := bounded(parent, 30*time.Second)
	if e != nil {
		return nil, e
	}
	defer cancel()
	defer func() {
		if err != nil {
			err = errors.Join(err, ctx.Err())
		}
	}()
	if e = acquire(ctx, c.maintenance); e != nil {
		return nil, e
	}
	defer func() { <-c.maintenance }()
	m, e := c.bundleAccess(ctx)
	if e != nil {
		return nil, e
	}
	select {
	case c.downloads <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	defer func() {
		if err != nil {
			<-c.downloads
		}
	}()
	path, e := randomPath(c.cfg.DataDir)
	if e != nil {
		return nil, e
	}
	f, e := privatefs.Create(path)
	if e != nil {
		return nil, ErrUnavailable
	}
	var owned os.FileInfo
	defer func() {
		if err == nil {
			return
		}
		if f != nil && ops.close(f) != nil {
			err = errors.Join(err, library.ErrCleanup)
		}
		if owned != nil && !removeOwned(path, owned) {
			err = errors.Join(err, library.ErrRecovery, library.ErrCleanup)
		}
	}()
	owned, e = f.Stat()
	if e != nil {
		return nil, ErrUnavailable
	}
	output := &bundleOutputWriter{ctx: ctx, f: f, write: ops.write, remaining: maxLibraryBundleBytes}
	e = m.WithMaintenance(ctx, func(_ context.Context, v *library.Maintenance) error {
		return v.WriteBundle(func(x context.Context) (*storage.Store, func(), error) { return c.lease(x, false, false) }, output)
	})
	if e != nil {
		return nil, libraryError(e)
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if ops.sync(f) != nil {
		return nil, ErrUnavailable
	}
	// A close failure is uncertainty even if the actual handle has closed.
	e = ops.close(f)
	f = nil
	if e != nil {
		return nil, errors.Join(ErrUnavailable, library.ErrCleanup)
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	named, e := os.Lstat(path)
	if e != nil || !os.SameFile(named, owned) || ops.check(path, maxLibraryBundleBytes) != nil {
		return nil, ErrUnavailable
	}
	f, e = ops.open(path, maxLibraryBundleBytes)
	if e != nil || f == nil {
		return nil, ErrUnavailable
	}
	info, e := f.Stat()
	named, ne := os.Lstat(path)
	if e != nil || ne != nil || !os.SameFile(info, owned) || !os.SameFile(info, named) || info.Size() <= 0 || info.Size() > maxLibraryBundleBytes || info.Size() != maxLibraryBundleBytes-output.remaining {
		return nil, ErrUnavailable
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return &Download{Filename: "miskoai-library.bundle", Size: info.Size(), file: f, path: path, owned: info, release: func() { <-c.downloads }}, nil
}

// RestoreLibraryBundle stops and joins generation before Manager admission.
// Invalid input may therefore leave generation stopped with the old DB intact.
// It never resumes channels; reconciliation remains explicit. Close waits for
// this maintenance operation, including actual reader and cleanup completion.
func (c *Controller) RestoreLibraryBundle(parent context.Context, input io.Reader) (result maintenance.RestoreResult, err error) {
	ctx, cancel, e := bounded(parent, 30*time.Second)
	if e != nil {
		return result, e
	}
	defer cancel()
	if e = acquire(ctx, c.maintenance); e != nil {
		return result, e
	}
	defer func() { <-c.maintenance }()
	m, e := c.bundleAccess(ctx)
	if e != nil {
		return result, e
	}
	if input == nil {
		return result, library.ErrInvalid
	}
	c.stopGeneration()
	var installErr error
	e = m.WithMaintenance(ctx, func(x context.Context, v *library.Maintenance) error {
		return v.PrepareBundle(input, func(p *library.PreparedBundle) error {
			s, release, e := c.lease(x, false, false)
			if e != nil {
				return e
			}
			current, e := s.LibrarySnapshot(x)
			release()
			if e != nil {
				return e
			}
			result, e = v.InstallBundle(p, current, func(candidate *maintenance.Candidate) (maintenance.RestoreResult, error) {
				r, e := c.installLibraryBundleDatabase(x, candidate)
				installErr = e
				return r, e
			})
			return e
		})
	})
	// Manager's safe boundary deliberately strips Core classes; preserve the
	// actual trusted callback's fixed maintenance class without exposing OS text.
	err = libraryError(e)
	if installErr != nil {
		err = errors.Join(err, installErr)
	}
	return result, err
}

// Only the trusted installer callback enters this exclusive Core section, after
// all media/preflight/retention/copy work. No Manager or Processor reentry here.
func (c *Controller) installLibraryBundleDatabase(ctx context.Context, candidate *maintenance.Candidate) (result maintenance.RestoreResult, err error) {
	s, release, e := c.lease(ctx, true, false)
	if e != nil {
		return result, e
	}
	defer release()
	if s.Close() != nil {
		c.mu.Lock()
		c.unavailable = true
		c.mu.Unlock()
		return result, ErrMaintenance
	}
	c.mu.Lock()
	c.store = nil
	c.mu.Unlock()
	result, e = maintenance.InstallRestore(ctx, c.lock, candidate)
	paused, pe := maintenance.RestorePaused(c.cfg.DataDir)
	var reopened *storage.Store
	path := filepath.Join(c.cfg.DataDir, "miskoai.db")
	if admitDB(path, false) == nil {
		reopened, err = storage.Open(path)
	} else {
		err = ErrUnavailable
	}
	c.mu.Lock()
	c.store = reopened
	c.unavailable = reopened == nil
	c.paused = paused || pe != nil || e == nil
	c.eventLocked("restore_paused")
	c.mu.Unlock()
	if e != nil || err != nil || pe != nil {
		return result, ErrMaintenance
	}
	return result, nil
}
