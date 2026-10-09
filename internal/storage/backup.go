package storage

import (
	"context"
	"os"
)

// Backup uses SQLite's consistent snapshot primitive rather than copying the
// live database/WAL files. The target must not already exist.
func (s *Store) Backup(ctx context.Context, dest string) error {
	_, err := s.BackupOwned(ctx, dest)
	return err
}

// BackupOwned returns identity captured from an owned open snapshot handle.
// Callers must retain this evidence, rather than infer ownership from a later
// pathname observation, when validating or cleaning up the destination.
func (s *Store) BackupOwned(ctx context.Context, dest string) (os.FileInfo, error) {
	return s.backupOwned(ctx, dest, backupOperations{})
}

type backupOperations struct {
	vacuum       func(context.Context, string) error
	closeCreated func(*os.File) error
	reopen       func(string) (*os.File, error)
}

func (s *Store) backupOwned(ctx context.Context, dest string, ops backupOperations) (owned os.FileInfo, err error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrInvalid
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	path, err := privatePath(dest)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, ErrInvalid
	}
	created, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, ErrStorage
	}
	ok := false
	defer func() {
		if !ok {
			if !backupIdentityMatches(path, created) || os.Remove(path) != nil {
				owned = nil
				err = ErrStorage
			}
		}
	}()
	closeCreated := ops.closeCreated
	if closeCreated == nil {
		closeCreated = func(f *os.File) error { return f.Close() }
	}
	if err = closeCreated(file); err != nil {
		return nil, ErrStorage
	}
	if !backupIdentityMatches(path, created) {
		return nil, ErrStorage
	}
	vacuum := ops.vacuum
	if vacuum == nil {
		vacuum = func(ctx context.Context, path string) error {
			_, e := s.db.ExecContext(ctx, "VACUUM INTO ?", path)
			return e
		}
	}
	if err = vacuum(ctx, path); err != nil {
		return nil, storageError(ctx, err)
	}
	if !backupIdentityMatches(path, created) {
		return nil, ErrStorage
	}
	// Ensure the completed backup is flushed before reporting success.
	reopen := ops.reopen
	if reopen == nil {
		reopen = func(path string) (*os.File, error) { return os.OpenFile(path, os.O_RDWR, 0600) }
	}
	file, err = reopen(path)
	if err != nil {
		return nil, ErrStorage
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(opened, created) || !backupIdentityMatches(path, created) {
		file.Close()
		return nil, ErrStorage
	}
	if err = file.Chmod(0600); err != nil {
		file.Close()
		return nil, ErrStorage
	}
	syncErr := file.Sync()
	owned, statErr := file.Stat()
	closeErr := file.Close()
	if syncErr != nil || statErr != nil || closeErr != nil || !os.SameFile(owned, created) || !backupIdentityMatches(path, created) {
		return nil, ErrStorage
	}
	ok = true
	return owned, nil
}

func backupIdentityMatches(path string, created os.FileInfo) bool {
	named, err := os.Lstat(path)
	return err == nil && created != nil && named.Mode().IsRegular() && os.SameFile(named, created)
}

func (s *Store) Integrity(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return storageError(ctx, err)
	}
	valid := true
	for rows.Next() {
		var result string
		if err = rows.Scan(&result); err != nil {
			rows.Close()
			return storageError(ctx, err)
		}
		if result != "ok" {
			valid = false
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return storageError(ctx, err)
	}
	if !valid {
		return ErrStorage
	}
	rows, err = s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return storageError(ctx, err)
	}
	invalid := rows.Next()
	err = rows.Err()
	rows.Close()
	if err != nil {
		return storageError(ctx, err)
	}
	if invalid {
		return ErrStorage
	}
	// FTS's rank=1 check also compares the external content table with its index.
	_, err = s.db.ExecContext(ctx, "INSERT INTO facts_fts(facts_fts,rank) VALUES('integrity-check',1)")
	return storageError(ctx, err)
}
