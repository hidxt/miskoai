package storage

import "context"

// ImageSelectionSnapshot returns bounded descriptive library metadata and the
// latest durable image ACK for one scope. It grants no pending-work authority.
// The caller supplies its original same-answer ContextSnapshot.Profile to the
// selector separately; this read does not load or replace that profile.
func (s *Store) ImageSelectionSnapshot(ctx context.Context, sc Scope) ([]LibraryAsset, int64, error) {
	if ctx == nil || sc.validate() != nil {
		return nil, 0, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, storageError(ctx, err)
	}
	defer tx.Rollback()
	// Admission precedes materialization, including the global 128-asset cap,
	// scalar/text byte bounds and receipt parent/phase/timestamp semantics.
	if err = validateLibraryCapacity(ctx, tx); err != nil {
		return nil, 0, err
	}
	if err = validateSchema6IfPresent(ctx, tx); err != nil {
		return nil, 0, err
	}
	assets, err := libraryList(ctx, tx, "SELECT "+libraryColumns+" FROM library_assets WHERE account=? AND user=? ORDER BY id LIMIT 128", sc.Account, sc.User)
	if err != nil {
		return nil, 0, err
	}
	var acknowledgedAt int64
	if err = tx.QueryRowContext(ctx, `SELECT coalesce(max(image_acked_at),0) FROM image_replies WHERE account=? AND user=? AND image_phase='acked'`, sc.Account, sc.User).Scan(&acknowledgedAt); err != nil {
		return nil, 0, storageError(ctx, err)
	}
	// libraryList has checked row completion/Close. Commit releases the short
	// read transaction and must succeed before any materialized data escapes.
	if err = tx.Commit(); err != nil {
		return nil, 0, storageError(ctx, err)
	}
	return assets, acknowledgedAt, nil
}
