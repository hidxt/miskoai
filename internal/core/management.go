package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func (c *Controller) Settings() (config.Settings, config.Settings, map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	o := make(map[string]bool, len(c.overrides))
	for k, v := range c.overrides {
		o[k] = v
	}
	return c.desired, c.effective, o
}
func (c *Controller) SaveSettings(parent context.Context, s config.Settings) error {
	ctx, cancel, e := bounded(parent, 10*time.Second)
	if e != nil {
		return e
	}
	defer cancel()
	if e = acquire(ctx, c.maintenance); e != nil {
		return e
	}
	defer func() { <-c.maintenance }()
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrUnavailable
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = config.SaveSettings(c.cfg.DataDir, s); e != nil {
		return config.ErrSettings
	}
	c.mu.Lock()
	c.desired = s
	c.eventLocked("settings_saved")
	c.mu.Unlock()
	return nil
}
func forward[T any](c *Controller, parent context.Context, f func(context.Context, *storage.Store, storage.Scope) (T, error)) (T, error) {
	var zero T
	ctx, cancel, e := bounded(parent, 10*time.Second)
	if e != nil {
		return zero, e
	}
	defer cancel()
	s, release, e := c.lease(ctx, false, true)
	if e != nil {
		return zero, e
	}
	defer release()
	return f(ctx, s, c.scope)
}
func forwardError(c *Controller, ctx context.Context, f func(context.Context, *storage.Store, storage.Scope) error) error {
	_, e := forward(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) (struct{}, error) {
		return struct{}{}, f(x, s, sc)
	})
	return e
}
func (c *Controller) FactsPage(ctx context.Context, q string, offset, limit int) (storage.FactPage, error) {
	return forward(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) (storage.FactPage, error) {
		return s.FactsPage(x, sc, q, offset, limit)
	})
}
func (c *Controller) HistoryPage(ctx context.Context, q string, offset, limit int) (storage.HistoryPage, error) {
	return forward(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) (storage.HistoryPage, error) {
		return s.HistoryPage(x, sc, q, offset, limit)
	})
}
func (c *Controller) AddFact(ctx context.Context, f storage.Fact) (int64, error) {
	f.Source = "explicit_user"
	f.Confidence = 1
	return forward(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) (int64, error) { return s.AddFact(x, sc, f) })
}
func (c *Controller) UpdateFact(ctx context.Context, f storage.Fact) error {
	f.Source = "explicit_user"
	f.Confidence = 1
	return forwardError(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) error { return s.UpdateFact(x, sc, f) })
}
func (c *Controller) DeleteFact(ctx context.Context, id int64) error {
	return forwardError(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) error { return s.DeleteFact(x, sc, id) })
}
func (c *Controller) Derived(ctx context.Context) (storage.Summary, error) {
	return forward(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) (storage.Summary, error) {
		return s.Derived(x, sc)
	})
}
func (c *Controller) Candidates(ctx context.Context, limit int) ([]storage.Candidate, error) {
	if limit < 1 || limit > 8 {
		return nil, storage.ErrInvalid
	}
	return forward(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) ([]storage.Candidate, error) {
		return s.Candidates(x, sc, limit)
	})
}
func (c *Controller) ConfirmCandidate(ctx context.Context, id int64) (int64, error) {
	return forward(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) (int64, error) {
		return s.ConfirmCandidate(x, sc, id)
	})
}
func (c *Controller) DeleteCandidate(ctx context.Context, id int64) error {
	return forwardError(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) error { return s.DeleteCandidate(x, sc, id) })
}
func (c *Controller) Profiles(ctx context.Context) ([]storage.Profile, error) {
	return forward(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) ([]storage.Profile, error) {
		p, e := s.Profiles(x, sc)
		if len(p) > 16 {
			return nil, storage.ErrInvalid
		}
		return p, e
	})
}
func (c *Controller) ActiveProfile(ctx context.Context) (storage.Profile, error) {
	return forward(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) (storage.Profile, error) {
		return s.ActiveProfile(x, sc)
	})
}
func (c *Controller) PutProfile(ctx context.Context, p storage.Profile) error {
	return forwardError(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) error { return s.PutProfile(x, sc, p) })
}
func (c *Controller) DeleteProfile(ctx context.Context, id string) error {
	return forwardError(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) error { return s.DeleteProfile(x, sc, id) })
}
func (c *Controller) SelectProfile(ctx context.Context, id string) error {
	return forwardError(c, ctx, func(x context.Context, s *storage.Store, sc storage.Scope) error { return s.SelectProfile(x, sc, id) })
}
func (c *Controller) ClearMemory(parent context.Context) error {
	ctx, cancel, e := bounded(parent, 10*time.Second)
	if e != nil {
		return e
	}
	defer cancel()
	if e = acquire(ctx, c.maintenance); e != nil {
		return e
	}
	defer func() { <-c.maintenance }()
	c.mu.Lock()
	eligible := c.eligibleLocked()
	closed := c.closed
	configured := c.scope.Account != ""
	c.mu.Unlock()
	if closed {
		return ErrUnavailable
	}
	if !configured {
		return ErrUnconfigured
	}
	c.stopGeneration()
	s, release, e := c.lease(ctx, true, true)
	if e == nil {
		e = s.ClearMemory(ctx, c.scope)
		release()
	}
	if eligible {
		if restart := c.startGeneration(); e == nil {
			e = restart
		}
	}
	return e
}

// Download holds no store lease. Callers must Close after transfer or abandonment.
// Only Filename and Size are public; no private path is serializable.
type Download struct {
	Filename string
	Size     int64
	mu       sync.Mutex
	file     *os.File
	path     string
	owned    os.FileInfo
	release  func()
	closed   bool
}

func (d *Download) Read(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return 0, ErrUnavailable
	}
	return d.file.Read(p)
}
func (d *Download) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	var e error
	if d.file.Close() != nil {
		e = ErrUnavailable
	}
	if !removeOwned(d.path, d.owned) {
		e = ErrUnavailable
	}
	d.release()
	return e
}
func removeOwned(path string, info os.FileInfo) bool {
	current, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return true
	}
	return e == nil && os.SameFile(current, info) && os.Remove(path) == nil
}
func randomPath(dir string) (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", ErrUnavailable
	}
	return filepath.Join(dir, ".miskoai-download-"+hex.EncodeToString(b[:])), nil
}
func (c *Controller) PrepareMemoryExport(ctx context.Context) (*Download, error) {
	return c.prepareDownload(ctx, false)
}
func (c *Controller) PrepareBackup(ctx context.Context) (*Download, error) {
	return c.prepareDownload(ctx, true)
}
func (c *Controller) prepareDownload(parent context.Context, backup bool) (d *Download, err error) {
	return c.prepareDownloadWithOperations(parent, backup, downloadOperations{})
}

type downloadOperations struct {
	snapshot       func(context.Context, *storage.Store, string) (os.FileInfo, error)
	check          func(string, int64) error
	open           func(string) (*os.File, error)
	maxBackupBytes int64
}

func (c *Controller) prepareDownloadWithOperations(parent context.Context, backup bool, ops downloadOperations) (d *Download, err error) {
	duration := 10 * time.Second
	cap := int64(64 << 20)
	name := "miskoai-memory.json"
	if backup {
		duration = 30 * time.Second
		cap = 256 << 20
		name = "miskoai-backup.db"
		if ops.maxBackupBytes > 0 {
			cap = ops.maxBackupBytes
		}
	}
	snapshot := ops.snapshot
	if snapshot == nil {
		snapshot = maintenance.BackupOwned
	}
	check := ops.check
	if check == nil {
		check = privatefs.CheckFile
	}
	open := ops.open
	if open == nil {
		open = os.Open
	}
	ctx, cancel, e := bounded(parent, duration)
	if e != nil {
		return nil, e
	}
	defer cancel()
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
	s, release, e := c.lease(ctx, false, !backup)
	if e != nil {
		return nil, e
	}
	defer release()
	path, e := randomPath(c.cfg.DataDir)
	if e != nil {
		return nil, e
	}
	var owned os.FileInfo
	var f *os.File
	defer func() {
		if err != nil {
			if f != nil {
				_ = f.Close()
			}
			if owned != nil {
				_ = removeOwned(path, owned)
			}
		}
	}()
	if backup {
		owned, e = snapshot(ctx, s, path)
		if e != nil {
			return nil, ErrMaintenance
		}
		if owned == nil {
			return nil, ErrUnavailable
		}
		named, ne := os.Lstat(path)
		if ne != nil || !os.SameFile(named, owned) {
			return nil, ErrUnavailable
		}
		if e = check(path, cap); e != nil {
			return nil, ErrUnavailable
		}
		f, e = open(path)
		if e != nil {
			return nil, ErrUnavailable
		}
	} else {
		f, e = privatefs.Create(path)
		if e != nil {
			return nil, ErrUnavailable
		}
		owned, e = f.Stat()
		if e != nil {
			return nil, ErrUnavailable
		}
		if e = s.WriteMemoryJSON(ctx, c.scope, f); e != nil {
			return nil, e
		}
		if e = f.Sync(); e != nil {
			return nil, ErrUnavailable
		}
		if e = f.Close(); e != nil {
			return nil, ErrUnavailable
		}
		f = nil
		if e = privatefs.CheckFile(path, cap); e != nil {
			return nil, ErrUnavailable
		}
		f, e = os.Open(path)
	}
	if e != nil || f == nil {
		return nil, ErrUnavailable
	}
	info, e := f.Stat()
	named, ne := os.Lstat(path)
	if e != nil || ne != nil || !os.SameFile(info, named) || !os.SameFile(info, owned) || info.Size() > cap {
		return nil, ErrUnavailable
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return &Download{Filename: name, Size: info.Size(), file: f, path: path, owned: info, release: func() { <-c.downloads }}, nil
}
func (c *Controller) Restore(parent context.Context, input io.Reader) (result maintenance.RestoreResult, err error) {
	ctx, cancel, e := bounded(parent, 30*time.Second)
	if e != nil {
		return result, e
	}
	defer cancel()
	if e = acquire(ctx, c.maintenance); e != nil {
		return result, e
	}
	defer func() { <-c.maintenance }()
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return result, ErrUnavailable
	}
	candidate, e := maintenance.PrepareRestore(ctx, c.cfg.DataDir, input)
	if e != nil {
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			return result, e
		}
		return result, ErrMaintenance
	}
	defer func() {
		if candidate.Discard() != nil && err == nil {
			err = ErrMaintenance
		}
	}()
	c.stopGeneration()
	s, release, e := c.lease(ctx, true, false)
	if e != nil {
		return result, e
	}
	defer release()
	if e = s.Close(); e != nil {
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
	path := filepath.Join(c.cfg.DataDir, "miskoai.db")
	var reopened *storage.Store
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
func (c *Controller) ResumeAfterReconciliation(parent context.Context) error {
	ctx, cancel, e := bounded(parent, 10*time.Second)
	if e != nil {
		return e
	}
	defer cancel()
	if e = acquire(ctx, c.maintenance); e != nil {
		return e
	}
	defer func() { <-c.maintenance }()
	c.mu.Lock()
	valid := !c.closed && !c.unavailable && c.scope.Account != "" && c.providerReady && !c.expired
	c.mu.Unlock()
	if !valid {
		return ErrUnavailable
	}
	s, release, e := c.lease(ctx, true, true)
	if e != nil {
		return e
	}
	_ = s
	if e = ctx.Err(); e == nil {
		e = maintenance.ClearRestorePause(c.lock)
	}
	release()
	if e != nil {
		return ErrMaintenance
	}
	c.mu.Lock()
	c.paused = false
	c.eventLocked("reconciled")
	c.mu.Unlock()
	return c.startGeneration()
}
