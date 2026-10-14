package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"io"
	"os"
	"path/filepath"
)

const bundleDBLimit int64 = 256 << 20

// bundleStage reserves one fixed directory. It never adopts named leftovers.
type bundleStage struct {
	dir      string
	identity os.FileInfo
	files    map[string]os.FileInfo
}

func reserveBundle(m *Manager) (*bundleStage, error) {
	if privatefs.CheckDir(m.dataDir) != nil {
		return nil, ErrRecovery
	}
	p := filepath.Join(m.dataDir, ".library-bundle-stage")
	if _, e := os.Lstat(p); !errors.Is(e, os.ErrNotExist) {
		return nil, ErrRecovery
	}
	if privatefs.EnsureDir(p) != nil {
		return nil, ErrRecovery
	}
	i, e := os.Lstat(p)
	if e != nil {
		return nil, ErrRecovery
	}
	s := &bundleStage{p, i, map[string]os.FileInfo{}}
	if maintenance.SyncPrivateDirectory(m.dataDir) != nil {
		return s, ErrCleanup
	}
	return s, nil
}
func (s *bundleStage) check() error {
	n, e := os.Lstat(s.dir)
	if e != nil || !os.SameFile(s.identity, n) || privatefs.CheckDir(s.dir) != nil {
		return ErrRecovery
	}
	return nil
}
func (s *bundleStage) path(name string) string { return filepath.Join(s.dir, name) }
func (s *bundleStage) verify(name string, limit int64) error {
	if s.check() != nil {
		return ErrRecovery
	}
	n, e := os.Lstat(s.path(name))
	if e != nil || !sameFile(s.files[name], n) || privatefs.CheckFile(s.path(name), limit) != nil {
		return ErrRecovery
	}
	return nil
}

type bundleStageIO struct {
	write func(*os.File, []byte) (int, error)
	sync  func(*os.File) error
	close func(*os.File) error
}

func (s *bundleStage) write(ctx context.Context, name string, input io.Reader, size int64) error {
	return s.writeWith(ctx, name, input, size, bundleStageIO{func(f *os.File, b []byte) (int, error) { return f.Write(b) }, func(f *os.File) error { return f.Sync() }, func(f *os.File) error { return f.Close() }})
}
func (s *bundleStage) writeWith(ctx context.Context, name string, input io.Reader, size int64, ops bundleStageIO) (err error) {
	if s.check() != nil || len(s.files) >= 129 || name != "database" && !validDigest(name) {
		return ErrRecovery
	}
	f, e := privatefs.Create(s.path(name))
	if e != nil {
		return ErrRecovery
	}
	original, e := f.Stat()
	if e != nil {
		if ops.close(f) != nil {
			return errors.Join(ErrRecovery, ErrCleanup)
		}
		return ErrRecovery
	}
	s.files[name] = original
	defer func() {
		n, e := f.Stat()
		if e != nil || !os.SameFile(original, n) {
			err = errors.Join(err, ErrRecovery)
		} else {
			s.files[name] = n
		}
		if ops.close(f) != nil {
			err = errors.Join(err, ErrCleanup)
		}
		err = safe(err)
	}()
	var b [4096]byte
	var count int64
	for count < size {
		if e = ctx.Err(); e != nil {
			return e
		}
		want := int64(len(b))
		if size-count < want {
			want = size - count
		}
		n, re := input.Read(b[:int(want)])
		if n < 0 || n > int(want) {
			return ErrContent
		}
		if n > 0 {
			w, we := ops.write(f, b[:n])
			if we != nil || w != n {
				return ErrCleanup
			}
			count += int64(n)
		}
		if re != nil && re != io.EOF {
			return safe(re)
		}
		if re == io.EOF && count < size || n == 0 {
			return ErrContent
		}
	}
	var probe [1]byte
	n, e := input.Read(probe[:])
	if n != 0 || e != io.EOF {
		return ErrContent
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if ops.sync(f) != nil {
		return ErrCleanup
	}
	return nil
}
func (s *bundleStage) open(name string, limit int64) (io.Reader, func() error, error) {
	if e := s.verify(name, limit); e != nil {
		return nil, nil, e
	}
	f, e := privatefs.OpenRead(s.path(name), limit)
	if e != nil {
		return nil, nil, ErrRecovery
	}
	i, e := f.Stat()
	if e != nil || !sameFile(s.files[name], i) {
		ce := f.Close()
		if ce != nil {
			return nil, nil, errors.Join(ErrRecovery, ErrCleanup)
		}
		return nil, nil, ErrRecovery
	}
	return f, func() error {
		var e error
		n, se := f.Stat()
		if se != nil || !sameFile(i, n) {
			e = ErrRecovery
		}
		if f.Close() != nil {
			e = errors.Join(e, ErrCleanup)
		}
		return errors.Join(e, s.verify(name, limit))
	}, nil
}
func (s *bundleStage) digest(ctx context.Context, name string, limit int64) (result string, err error) {
	r, release, e := s.open(name, limit)
	if e != nil {
		return "", e
	}
	defer func() { err = errors.Join(err, release()) }()
	h := sha256.New()
	var b [4096]byte
	for {
		if e = ctx.Err(); e != nil {
			return "", e
		}
		n, re := r.Read(b[:])
		if n > 0 {
			h.Write(b[:n])
		}
		if re == io.EOF {
			break
		}
		if re != nil || n == 0 {
			return "", ErrContent
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (s *bundleStage) cleanup() error {
	if s == nil {
		return nil
	}
	if s.check() != nil {
		return ErrRecovery
	}
	d, e := os.Open(s.dir)
	if e != nil {
		return ErrRecovery
	}
	entries, re := d.ReadDir(130)
	ce := d.Close()
	if re != nil && re != io.EOF || ce != nil || len(entries) > 129 {
		return ErrRecovery
	}
	uncertain := false
	for name := range s.files {
		limit := int64(4 << 20)
		if name == "database" {
			limit = bundleDBLimit
		}
		if s.verify(name, limit) != nil {
			uncertain = true
		}
	}
	var err error
	for _, entry := range entries {
		limit := int64(4 << 20)
		if entry.Name() == "database" {
			limit = bundleDBLimit
		}
		if _, ok := s.files[entry.Name()]; !ok || s.verify(entry.Name(), limit) != nil {
			uncertain = true
			continue
		}
		if os.Remove(s.path(entry.Name())) != nil {
			err = errors.Join(err, ErrCleanup)
		}
	}
	if uncertain {
		return errors.Join(err, ErrRecovery)
	}
	if err != nil {
		return err
	}
	if s.check() != nil {
		return ErrRecovery
	}
	if os.Remove(s.dir) != nil {
		return ErrCleanup
	}
	if maintenance.SyncPrivateDirectory(filepath.Dir(s.dir)) != nil {
		return ErrCleanup
	}
	return nil
}
