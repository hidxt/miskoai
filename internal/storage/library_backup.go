package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
)

// LibraryBackupSnapshot reads metadata from a completed immutable snapshot.
func LibraryBackupSnapshot(ctx context.Context, path string) ([]LibraryAsset, error) {
	if ctx == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Preserve ValidateBackup's historical WAL/SHM-only policy. The new
	// metadata API additionally requires no retained rollback journal.
	if _, err := os.Lstat(path + "-journal"); err == nil {
		return nil, ErrInvalid
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, ErrStorage
	}
	assets := []LibraryAsset{}
	err := validatedBackup(ctx, path, func(db *sql.DB, version int) error {
		if version < 5 {
			return nil
		}
		// The wrapper borrows the existing query-only connection. No writable
		// Open, migration or second connection participates in this read.
		var err error
		assets, err = (&Store{db: db}).LibrarySnapshot(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return assets, nil
}
