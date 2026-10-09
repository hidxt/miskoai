package maintenance

import (
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxRestoreBytes int64 = 256 << 20

var errRestore = errors.New("restore candidate is invalid, unsupported, or exceeds the limit")
var errRecovery = errors.New("restore failed; inspect retained private recovery state; manual recovery required")

// Candidate is a private validated snapshot. Its path stays internal.
// It must not be copied; only its original file may be discarded.
type Candidate struct {
	mu        sync.Mutex
	self      *Candidate
	dir, path string
	info      os.FileInfo
	finished  bool
}
type RestoreResult struct {
	PreviousRetained bool
	ChannelPaused    bool
}
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	n, e := r.r.Read(p)
	if ce := r.ctx.Err(); ce != nil {
		return n, ce
	}
	return n, e
}
func PrepareRestore(ctx context.Context, dataDir string, input io.Reader) (candidate *Candidate, err error) {
	if ctx == nil || input == nil {
		return nil, errRestore
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	dir, e := privatefs.Resolve(dataDir)
	if e != nil {
		return nil, e
	}
	if e = privatefs.CheckDir(dir); e != nil {
		return nil, e
	}
	path, e := uniquePath(dir, "restore-", ".tmp")
	if e != nil {
		return nil, e
	}
	f, e := privatefs.Create(path)
	if e != nil {
		return nil, errRestore
	}
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, errRestore
	}
	c := &Candidate{dir: dir, path: path, info: info}
	c.self = c
	defer func() {
		if err != nil {
			if e := c.Discard(); e != nil {
				err = e
			}
		}
	}()
	n, copyErr := io.Copy(f, io.LimitReader(contextReader{ctx, input}, maxRestoreBytes+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil || closeErr != nil || n > maxRestoreBytes {
		return nil, errRestore
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if copyErr != nil {
		return nil, errRestore
	}
	if privatefs.CheckFile(path, maxRestoreBytes) != nil || storage.ValidateBackup(ctx, path) != nil {
		return nil, errRestore
	}
	s, e := storage.Open(path)
	if e != nil {
		return nil, errRestore
	}
	e = s.Integrity(ctx)
	ce := s.Close()
	if e != nil || ce != nil {
		return nil, errRestore
	}
	if e = noJournals(path); e != nil {
		return nil, e
	}
	named, e := os.Lstat(path)
	if e != nil || !os.SameFile(named, info) {
		return nil, errOwnership
	}
	c.info = named
	return c, nil
}
func (c *Candidate) verify() error {
	if c == nil || c.self != c || c.finished || c.info == nil {
		return errOwnership
	}
	if privatefs.CheckFile(c.path, math.MaxInt64) != nil {
		return errOwnership
	}
	named, e := os.Lstat(c.path)
	if e != nil || !sameMetadata(named, c.info) {
		return errOwnership
	}
	return noJournals(c.path)
}
func (c *Candidate) Discard() error {
	if c == nil {
		return errOwnership
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.self != c {
		return errOwnership
	}
	if c.finished {
		return nil
	}
	if e := noJournals(c.path); e != nil {
		return e
	}
	if e := removeOwned(c.path, c.info); e != nil {
		return e
	}
	c.finished = true
	return nil
}

// The per-call seam permits failure testing without global mutable hooks.
type installOperations struct {
	rename  func(string, string) error
	open    func(string) (*storage.Store, error)
	pause   func(*Lock) error
	syncDir func(string) error
}

func InstallRestore(ctx context.Context, l *Lock, c *Candidate) (RestoreResult, error) {
	return installRestore(ctx, l, c, installOperations{rename: os.Rename, open: storage.Open})
}
func installRestore(ctx context.Context, l *Lock, c *Candidate, ops installOperations) (result RestoreResult, err error) {
	if l == nil || c == nil || ctx == nil {
		return result, errOwnership
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := l.verifyLocked(); e != nil {
		return result, e
	}
	if c.dir != l.dir {
		return result, errOwnership
	}
	if e := c.verify(); e != nil {
		return result, e
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	target := filepath.Join(l.dir, "miskoai.db")
	if e := noJournals(target); e != nil {
		return result, e
	}
	old, e := os.Lstat(target)
	if e == nil {
		if privatefs.CheckFile(target, math.MaxInt64) != nil {
			return result, privatefs.ErrUnsafe
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return result, errMaintenance
	}
	// The pause is durable before possibly older dedup state can be installed.
	pause := ops.pause
	if pause == nil {
		pause = ensurePause
	}
	if e = pause(l); e != nil {
		return result, e
	}
	result.ChannelPaused = true
	syncDir := ops.syncDir
	if syncDir == nil {
		syncDir = syncDirectory
	}
	// A file sync does not persist its newly created directory entry on Linux.
	if e = syncDir(l.dir); e != nil {
		return result, errPause
	}
	previous := ""
	if old != nil {
		previous, e = uniquePath(l.dir, "pre-restore-", ".db")
		if e != nil {
			return result, e
		}
		if _, e = os.Lstat(previous); !errors.Is(e, os.ErrNotExist) {
			return result, errMaintenance
		}
		named, e := os.Lstat(target)
		if e != nil || !sameMetadata(old, named) {
			return result, errOwnership
		}
		if e = ops.rename(target, previous); e != nil {
			return result, errMaintenance
		}
		result.PreviousRetained = true
	}
	installed := false
	rollback := func() error {
		if installed {
			if noJournals(target) != nil {
				return errRecovery
			}
			named, e := os.Lstat(target)
			if e != nil || !os.SameFile(named, c.info) || privatefs.CheckFile(target, math.MaxInt64) != nil {
				return errRecovery
			}
			if e = ops.rename(target, c.path); e != nil {
				return errRecovery
			}
			c.info = named
			installed = false
			if syncDir(l.dir) != nil {
				return errRecovery
			}
		}
		if previous != "" {
			previousInfo, e := os.Lstat(previous)
			if e != nil || !os.SameFile(previousInfo, old) || privatefs.CheckFile(previous, math.MaxInt64) != nil || noJournals(previous) != nil {
				return errRecovery
			}
			if _, e = os.Lstat(target); !errors.Is(e, os.ErrNotExist) {
				return errRecovery
			}
			if e := ops.rename(previous, target); e != nil {
				return errRecovery
			}
			result.PreviousRetained = false
			if syncDir(l.dir) != nil {
				// Keep the original private recovery snapshot when rollback's
				// directory durability could not be confirmed.
				if named, ne := os.Lstat(target); ne == nil && os.SameFile(named, old) && noJournals(target) == nil {
					if ops.rename(target, previous) == nil {
						result.PreviousRetained = true
						if syncDir(l.dir) != nil {
							return errRecovery
						}
					}
				}
				return errRecovery
			}
		}
		return errMaintenance
	}
	if previous != "" {
		if e = syncDir(l.dir); e != nil {
			return result, rollback()
		}
	}
	if e = l.verifyLocked(); e != nil {
		return result, rollback()
	}
	if e = ops.rename(c.path, target); e != nil {
		return result, rollback()
	}
	installed = true
	if e = syncDir(l.dir); e != nil {
		return result, rollback()
	}
	s, e := ops.open(target)
	if e != nil {
		return result, rollback()
	}
	e = s.Integrity(ctx)
	ce := s.Close()
	if e != nil || ce != nil || noJournals(target) != nil || privatefs.CheckFile(target, math.MaxInt64) != nil {
		return result, rollback()
	}
	if e = l.verifyLocked(); e != nil {
		return result, rollback()
	}
	if e = syncDir(l.dir); e != nil {
		return result, rollback()
	}
	c.finished = true
	return result, nil
}
func StoppedRestore(ctx context.Context, dataDir, source string) (result RestoreResult, err error) {
	l, e := Acquire(dataDir)
	if e != nil {
		return result, e
	}
	defer func() {
		if e := l.Close(); e != nil {
			err = errors.Join(err, e)
		}
	}()
	if ctx == nil {
		return result, errRestore
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	src, e := privatefs.Resolve(source)
	if e != nil {
		return result, e
	}
	target := filepath.Join(l.dir, "miskoai.db")
	if strings.EqualFold(src, target) || (strings.EqualFold(filepath.Dir(src), l.dir) && strings.HasPrefix(strings.ToLower(filepath.Base(src)), "restore-")) {
		return result, errRestore
	}
	if e = noJournals(target); e != nil {
		return result, e
	}
	if e = privatefs.CheckFile(src, maxRestoreBytes); e != nil {
		return result, e
	}
	if e = noJournals(src); e != nil {
		return result, e
	}
	before, e := os.Lstat(src)
	if e != nil || before.Size() < 16 {
		return result, errRestore
	}
	if targetInfo, e := os.Lstat(target); e == nil && os.SameFile(before, targetInfo) {
		return result, errRestore
	}
	in, e := os.Open(src)
	if e != nil {
		return result, errRestore
	}
	defer func() {
		if in.Close() != nil {
			err = errors.Join(err, errRestore)
		}
	}()
	opened, e := in.Stat()
	if e != nil || !sameMetadata(before, opened) {
		return result, errRestore
	}
	c, e := PrepareRestore(ctx, l.dir, in)
	if e != nil {
		return result, e
	}
	defer func() {
		if e := c.Discard(); e != nil {
			err = errors.Join(err, e)
		}
	}()
	final, e := in.Stat()
	named, ne := os.Lstat(src)
	if e != nil || ne != nil || !sameMetadata(opened, final) || !sameMetadata(opened, named) || privatefs.CheckFile(src, maxRestoreBytes) != nil || noJournals(src) != nil {
		return result, errRestore
	}
	return InstallRestore(ctx, l, c)
}
