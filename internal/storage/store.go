// Package storage owns private, scoped SQLite data. Callers must authorize a
// scope before using it; scope fields are never inferred from model output.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

var (
	ErrInvalid  = errors.New("invalid storage input")
	ErrNotFound = errors.New("scoped record not found")
	ErrStorage  = errors.New("storage operation failed")
	ErrCapacity = errors.New("storage capacity reached")
)

type Store struct{ db *sql.DB }
type Scope struct{ Account, User string }

func (s Scope) validate() error {
	for _, v := range []string{s.Account, s.User} {
		if strings.TrimSpace(v) == "" || len(v) > 256 || strings.ContainsRune(v, 0) || !utf8.ValidString(v) {
			return ErrInvalid
		}
	}
	return nil
}

// privatePath creates a dedicated private parent directory and refuses an
// existing symlink or non-regular file. Windows deployments additionally need
// owner-only ACLs; Unix permission bits alone do not establish Windows privacy.
func privatePath(path string) (string, error) {
	if path == "" || strings.ContainsRune(path, 0) || path == ":memory:" {
		return "", ErrInvalid
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", ErrInvalid
	}
	dir := filepath.Dir(abs)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", ErrStorage
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", ErrInvalid
	}
	// An existing parent may be an unrelated user directory. Never chmod it.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return "", ErrInvalid
	}
	if info, err = os.Lstat(abs); err == nil {
		if !info.Mode().IsRegular() {
			return "", ErrInvalid
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", ErrStorage
	}
	return abs, nil
}

func Open(path string) (*Store, error) {
	abs, err := privatePath(path)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, ErrStorage
	}
	err = file.Chmod(0600)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return nil, ErrStorage
	}
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		return nil, ErrStorage
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	fail := func(err error) (*Store, error) { _ = db.Close(); return nil, err }
	ctx := context.Background()
	for _, q := range []string{"PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA cache_size=-8192", "PRAGMA wal_autocheckpoint=1000"} {
		if _, err = db.ExecContext(ctx, q); err != nil {
			return fail(storageError(ctx, err))
		}
	}
	if err = s.migrate(ctx); err != nil {
		return fail(err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func storageError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrStorage
}
