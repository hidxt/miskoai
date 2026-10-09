// Package maintenance owns the shared service lifecycle and stopped database
// maintenance boundary. Checks assume no malicious same-user process.
package maintenance

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/hidxt/miskoai/internal/privatefs"
	"math"
	"os"
	"path/filepath"
	"sync"
)

var (
	errLocked      = errors.New("data directory locked; stop service and inspect stale locks before maintenance")
	errOwnership   = errors.New("maintenance ownership is missing or changed")
	errMaintenance = errors.New("private maintenance operation failed")
	errJournals    = errors.New("SQLite journal files remain; stop and checkpoint or reconcile before maintenance")
)

// Lock retains the exclusive handle and identity for the entire lifecycle.
// It must not be copied. Existing locks are never automatically removed.
type Lock struct {
	mu       sync.Mutex
	self     *Lock
	dir      string
	file     *os.File
	info     os.FileInfo
	dirInfo  os.FileInfo
	closed   bool
	closeErr error
}

func Acquire(dataDir string) (*Lock, error) {
	dir, e := privatefs.Resolve(dataDir)
	if e != nil {
		return nil, e
	}
	if e = privatefs.EnsureDir(dir); e != nil {
		return nil, e
	}
	dirInfo, e := os.Lstat(dir)
	if e != nil {
		return nil, errOwnership
	}
	f, e := privatefs.Create(filepath.Join(dir, ".miskoai.lock"))
	if e != nil {
		return nil, errLocked
	}
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, errOwnership
	}
	l := &Lock{dir: dir, file: f, info: info, dirInfo: dirInfo}
	l.self = l
	return l, nil
}
func (l *Lock) verifyLocked() error {
	if l == nil || l.self != l || l.closed || l.file == nil || l.info == nil || l.dirInfo == nil || l.file.Name() != filepath.Join(l.dir, ".miskoai.lock") {
		return errOwnership
	}
	if privatefs.CheckDir(l.dir) != nil || privatefs.CheckFile(l.file.Name(), math.MaxInt64) != nil {
		return errOwnership
	}
	dirInfo, e := os.Lstat(l.dir)
	if e != nil || !os.SameFile(dirInfo, l.dirInfo) {
		return errOwnership
	}
	current, e := l.file.Stat()
	named, ne := os.Lstat(l.file.Name())
	if e != nil || ne != nil || !os.SameFile(current, l.info) || !os.SameFile(named, l.info) {
		return errOwnership
	}
	return nil
}

// Close is idempotent and refuses to remove a replacement lock identity.
func (l *Lock) Close() error {
	if l == nil {
		return errOwnership
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.self != l {
		return errOwnership
	}
	if l.closed {
		return l.closeErr
	}
	e := l.verifyLocked()
	l.closed = true
	if l.file.Close() != nil {
		l.closeErr = errOwnership
		return l.closeErr
	}
	if e != nil {
		l.closeErr = e
		return e
	}
	named, e := os.Lstat(l.file.Name())
	if e != nil || !os.SameFile(named, l.info) {
		l.closeErr = errOwnership
		return l.closeErr
	}
	if os.Remove(l.file.Name()) != nil {
		l.closeErr = errOwnership
	}
	return l.closeErr
}
func uniquePath(dir, prefix, suffix string) (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", errMaintenance
	}
	return filepath.Join(dir, prefix+hex.EncodeToString(b[:])+suffix), nil
}
func noJournals(path string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, e := os.Lstat(path + suffix); !errors.Is(e, os.ErrNotExist) {
			return errJournals
		}
	}
	return nil
}
func sameMetadata(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
func removeOwned(path string, info os.FileInfo) error {
	named, e := os.Lstat(path)
	if e != nil || info == nil || !os.SameFile(named, info) || privatefs.CheckFile(path, math.MaxInt64) != nil {
		return errOwnership
	}
	if os.Remove(path) != nil {
		return errMaintenance
	}
	return nil
}
