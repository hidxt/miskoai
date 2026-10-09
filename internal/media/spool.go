package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/hidxt/miskoai/internal/privatefs"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type spoolOps struct {
	write         func(*os.File, []byte) (int, error)
	sync          func(*os.File) error
	close         func(*os.File) error
	beforePublish func(string)
	beforeCleanup func(string)
}
type spool struct {
	path     string
	file     *os.File
	original os.FileInfo
	ops      spoolOps
}
type spoolWriter struct{ spool *spool }

func (w spoolWriter) Write(p []byte) (int, error) {
	if w.spool.ops.write != nil {
		return w.spool.ops.write(w.spool.file, p)
	}
	return w.spool.file.Write(p)
}
func newSpool(dir string, ops spoolOps) (*spool, error) {
	abs, e := privatefs.Resolve(dir)
	if e != nil || privatefs.EnsureDir(abs) != nil {
		return nil, ErrPrivate
	}
	var random [16]byte
	if _, e = rand.Read(random[:]); e != nil {
		return nil, ErrIO
	}
	path := filepath.Join(abs, "media-"+hex.EncodeToString(random[:]))
	f, e := privatefs.Create(path)
	if e != nil {
		return nil, ErrPrivate
	}
	original, e := f.Stat()
	// Windows FileInfo can lazily consult its pathname. Resolve its identity
	// now, while the original handle/path are present, before any payload.
	if e != nil || !os.SameFile(original, original) {
		f.Close()
		return nil, ErrPrivate
	}
	s := &spool{path: path, file: f, original: original, ops: ops}
	if !s.owned() || privatefs.CheckFile(path, 0) != nil {
		s.cleanup()
		return nil, ErrPrivate
	}
	return s, nil
}
func (s *spool) owned() bool { return owns(s.path, s.original) }
func owns(path string, original os.FileInfo) bool {
	named, e := os.Lstat(path)
	return e == nil && named.Mode().IsRegular() && os.SameFile(original, named)
}
func (s *spool) cleanup() {
	if s.ops.beforeCleanup != nil {
		s.ops.beforeCleanup(s.path)
	}
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	// Refuse same-path replacements; never follow their payload or remove them.
	if s.owned() {
		os.Remove(s.path)
	}
}
func (s *spool) publish(ctx context.Context, size int64) (*Artifact, error) {
	syncFile := s.file.Sync
	if s.ops.sync != nil {
		syncFile = func() error { return s.ops.sync(s.file) }
	}
	if syncFile() != nil {
		return nil, ErrIO
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if !s.owned() || privatefs.CheckFile(s.path, size) != nil {
		return nil, ErrPrivate
	}
	actual, e := s.file.Stat()
	if e != nil || actual.Size() != size || !os.SameFile(s.original, actual) {
		return nil, ErrPrivate
	}
	closeFile := s.file.Close
	if s.ops.close != nil {
		closeFile = func() error { return s.ops.close(s.file) }
	}
	if closeFile() != nil {
		return nil, ErrIO
	}
	s.file = nil
	if s.ops.beforePublish != nil {
		s.ops.beforePublish(s.path)
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if !s.owned() || privatefs.CheckFile(s.path, size) != nil {
		return nil, ErrPrivate
	}
	f, e := os.Open(s.path)
	if e != nil {
		return nil, ErrPrivate
	}
	reopened, e := f.Stat()
	if e != nil || !os.SameFile(s.original, reopened) || reopened.Size() != size || !reopened.Mode().IsRegular() || !s.owned() || privatefs.CheckFile(s.path, size) != nil {
		f.Close()
		return nil, ErrPrivate
	}
	if e = ctx.Err(); e != nil {
		f.Close()
		return nil, e
	}
	return &Artifact{path: s.path, file: f, original: s.original, size: size}, nil
}

// Artifact exposes only completed bounded bytes. Its size is immutable. Reads
// and Close serialize; Close is idempotent and removes only its original file.
// Callers must preserve immutability against other same-user filesystem access.
type Artifact struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	original os.FileInfo
	size     int64
	closed   bool
	closeErr error
}

func (a *Artifact) Read(p []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.file == nil {
		return 0, ErrIO
	}
	n, e := a.file.Read(p)
	return safeRead(n, e)
}
func (a *Artifact) ReadAt(p []byte, off int64) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.file == nil {
		return 0, ErrIO
	}
	n, e := a.file.ReadAt(p, off)
	return safeRead(n, e)
}
func safeRead(n int, e error) (int, error) {
	if e != nil && e != io.EOF {
		return n, ErrIO
	}
	return n, e
}
func (a *Artifact) Size() int64 { return a.size }
func (a *Artifact) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return a.closeErr
	}
	a.closed = true
	owned := owns(a.path, a.original)
	if a.file != nil && a.file.Close() != nil {
		a.closeErr = ErrIO
	}
	a.file = nil
	if !owned || !owns(a.path, a.original) {
		a.closeErr = ErrPrivate
		return a.closeErr
	}
	if os.Remove(a.path) != nil {
		a.closeErr = ErrIO
	}
	return a.closeErr
}
