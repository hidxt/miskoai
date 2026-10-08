package storage

import (
	"context"
	"os"
)

// Backup uses SQLite's consistent snapshot primitive rather than copying the
// live database/WAL files. The target must not already exist.
func (s *Store) Backup(ctx context.Context, dest string) error {
	path, err := privatePath(dest)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ErrInvalid
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return ErrStorage
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err = s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return storageError(ctx, err)
	}
	if err = os.Chmod(path, 0600); err != nil {
		return ErrStorage
	}
	// Ensure the completed backup is flushed before reporting success.
	file, err = os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return ErrStorage
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil || closeErr != nil {
		return ErrStorage
	}
	ok = true
	return nil
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
