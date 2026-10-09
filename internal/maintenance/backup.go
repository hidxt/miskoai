package maintenance

import (
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"math"
	"os"
	"path/filepath"
)

// Backup requires the caller to retain lifecycle ownership of the admitted store.
func Backup(ctx context.Context, s *storage.Store, destination string) error {
	return backupWithOperations(ctx, s, destination, backupOperations{})
}

// BackupOwned preserves the verified snapshot producer's opened-handle identity
// across validation and return. The caller must retain lifecycle ownership.
func BackupOwned(ctx context.Context, s *storage.Store, destination string) (os.FileInfo, error) {
	return backupOwnedWithOperations(ctx, s, destination, backupOperations{})
}

type backupOperations struct {
	snapshot func(context.Context, *storage.Store, string) (os.FileInfo, error)
	validate func(context.Context, string) error
}

func backupWithOperations(ctx context.Context, s *storage.Store, destination string, ops backupOperations) error {
	_, err := backupOwnedWithOperations(ctx, s, destination, ops)
	return err
}
func backupOwnedWithOperations(ctx context.Context, s *storage.Store, destination string, ops backupOperations) (os.FileInfo, error) {
	if s == nil || ctx == nil {
		return nil, errMaintenance
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	dest, e := privatefs.Resolve(destination)
	if e != nil {
		return nil, e
	}
	if e = privatefs.EnsureDir(filepath.Dir(dest)); e != nil {
		return nil, e
	}
	if _, e = os.Lstat(dest); !errors.Is(e, os.ErrNotExist) {
		return nil, errMaintenance
	}
	if e = noJournals(dest); e != nil {
		return nil, e
	}
	snapshot := ops.snapshot
	if snapshot == nil {
		snapshot = func(ctx context.Context, s *storage.Store, path string) (os.FileInfo, error) {
			return s.BackupOwned(ctx, path)
		}
	}
	info, e := snapshot(ctx, s, dest)
	if e != nil {
		return nil, errMaintenance
	}
	if !backupIdentityMatches(dest, info) {
		return nil, errMaintenance
	}
	validate := ops.validate
	if validate == nil {
		validate = storage.ValidateBackup
	}
	if privatefs.CheckFile(dest, math.MaxInt64) != nil || validate(ctx, dest) != nil || !backupIdentityMatches(dest, info) {
		if removeOwned(dest, info) != nil {
			return nil, errMaintenance
		}
		return nil, errMaintenance
	}
	return info, nil
}
func backupIdentityMatches(path string, owned os.FileInfo) bool {
	named, e := os.Lstat(path)
	return e == nil && owned != nil && named.Mode().IsRegular() && os.SameFile(named, owned)
}
func StoppedBackup(ctx context.Context, dataDir, destination string) (err error) {
	l, e := Acquire(dataDir)
	if e != nil {
		return e
	}
	defer func() {
		if e := l.Close(); e != nil {
			err = e
		}
	}()
	if ctx == nil || ctx.Err() != nil {
		if ctx != nil {
			return ctx.Err()
		}
		return errMaintenance
	}
	path := filepath.Join(l.dir, "miskoai.db")
	if e = privatefs.CheckFile(path, math.MaxInt64); e != nil {
		return e
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, e = os.Lstat(path + suffix); errors.Is(e, os.ErrNotExist) {
			continue
		} else if e != nil {
			return privatefs.ErrUnsafe
		}
		if e = privatefs.CheckFile(path+suffix, math.MaxInt64); e != nil {
			return e
		}
	}
	s, e := storage.Open(path)
	if e != nil {
		return errMaintenance
	}
	e = Backup(ctx, s, destination)
	ce := s.Close()
	if ce != nil {
		return errMaintenance
	}
	return e
}
