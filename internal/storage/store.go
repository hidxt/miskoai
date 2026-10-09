// Package storage owns private, scoped SQLite data. Callers must authorize a
// scope before using it; scope fields are never inferred from model output.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

var (
	ErrInvalid  = errors.New("invalid storage input")
	ErrNotFound = errors.New("scoped record not found")
	ErrStorage  = errors.New("storage operation failed")
	ErrCapacity = errors.New("storage capacity reached")
	ErrStale    = errors.New("stale derived revision")
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
	// Inspect existing databases through a read-only connection before chmod,
	// journal configuration or migration. In particular foreign version zero
	// must leave the original file and its journal state untouched.
	needsCheckpoint := true
	if info, e := os.Stat(abs); e == nil && info.Size() > 0 {
		probeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		uriPath := filepath.ToSlash(abs)
		if filepath.VolumeName(abs) != "" {
			uriPath = "/" + uriPath
		}
		// immutable avoids even journal-sidecar creation on foreign inputs.
		// An uncheckpointed live database may have its schema in the WAL; the
		// single-process lifecycle requires closing it before reopening.
		dsn := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro&immutable=1"}
		probe, e := sql.Open("sqlite", dsn.String())
		if e != nil {
			return nil, ErrInvalid
		}
		probe.SetMaxOpenConns(1)
		probe.SetMaxIdleConns(1)
		var version int
		hasSidecar := false
		e = validateManifest(probeCtx, probe)
		if e == nil {
			e = probe.QueryRowContext(probeCtx, "PRAGMA user_version").Scan(&version)
			if e == nil && version >= 2 {
				for _, suffix := range []string{"-wal", "-shm"} {
					if info, sideErr := os.Lstat(abs + suffix); sideErr == nil {
						if !info.Mode().IsRegular() {
							e = ErrInvalid
							break
						}
						hasSidecar = true
					} else if !errors.Is(sideErr, os.ErrNotExist) {
						e = ErrStorage
						break
					}
				}
				// A completed WAL-mode snapshot with no journals must be checked
				// immutably. mode=ro may create empty WAL/SHM even on refusal.
				if e == nil && !hasSidecar {
					e = validateLegacyCapacity(probeCtx, probe)
				}
				if e == nil && !hasSidecar {
					e = validateReceiveCapacity(probeCtx, probe)
				}
				if e == nil && !hasSidecar {
					e = validateSchema3IfPresent(probeCtx, probe)
				}
			}
			if e == nil && version == 1 {
				for _, suffix := range []string{"-wal", "-shm"} {
					if _, sideErr := os.Lstat(abs + suffix); sideErr == nil {
						e = ErrInvalid
						break
					} else if !errors.Is(sideErr, os.ErrNotExist) {
						e = ErrStorage
						break
					}
				}
				if e == nil {
					e = validateLegacyCapacity(probeCtx, probe)
				}
			}
			if e == nil {
				needsCheckpoint = version != schemaVersion
			}
		}
		if closeErr := probe.Close(); e == nil && closeErr != nil {
			e = ErrStorage
		}
		if e != nil {
			return nil, e
		}
		if version >= 2 && hasSidecar {
			// Read the effective schema/data including any retained WAL only
			// after the immutable main manifest established this is MiskoAI.
			dsn.RawQuery = "mode=ro"
			probe, e = sql.Open("sqlite", dsn.String())
			if e != nil {
				return nil, ErrStorage
			}
			probe.SetMaxOpenConns(1)
			probe.SetMaxIdleConns(1)
			_, e = probe.ExecContext(probeCtx, "PRAGMA query_only=ON; PRAGMA busy_timeout=5000; PRAGMA cache_size=-8192")
			var admission *sql.Tx
			if e == nil {
				admission, e = probe.BeginTx(probeCtx, nil)
			}
			if e == nil {
				e = validateManifest(probeCtx, admission)
			}
			if e == nil {
				e = validateLegacyCapacity(probeCtx, admission)
			}
			if e == nil {
				e = validateReceiveCapacity(probeCtx, admission)
			}
			if e == nil {
				e = validateSchema3IfPresent(probeCtx, admission)
			}
			if admission != nil {
				if rollbackErr := admission.Rollback(); e == nil && rollbackErr != nil {
					e = ErrStorage
				}
			}
			if closeErr := probe.Close(); e == nil && closeErr != nil {
				e = ErrStorage
			}
			if e != nil {
				return nil, storageOrPolicyError(probeCtx, e)
			}
		}
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
	// Persist the newly initialized/migrated manifest in the main file before
	// receiving any work. Later immutable admission can inspect it even when
	// an abrupt restart leaves raw messages in the WAL.
	if needsCheckpoint {
		var busy, log, checkpointed int
		if err = db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &checkpointed); err != nil {
			return fail(storageError(ctx, err))
		}
		if busy != 0 {
			return fail(ErrStorage)
		}
	}
	return s, nil
}

func storageOrPolicyError(ctx context.Context, err error) error {
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrCapacity) || errors.Is(err, ErrStorage) {
		return err
	}
	return storageError(ctx, err)
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
