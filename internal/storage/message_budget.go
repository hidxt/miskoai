package storage

import "context"

const imageReplyReservation int64 = 1664

func messageBudget(ctx context.Context, q rowQuery) (int, int64, error) {
	if e := validateLegacyShape(ctx, q); e != nil {
		return 0, 0, e
	}
	var count int
	var bytes int64
	var version int
	if e := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); e != nil {
		return 0, 0, storageError(ctx, e)
	}
	if e := q.QueryRowContext(ctx, "SELECT count(*) FROM messages").Scan(&count); e != nil {
		return 0, 0, storageError(ctx, e)
	}
	if count > 10000 {
		return 0, 0, ErrCapacity
	}
	var receipts int64
	if version >= 6 {
		if e := imageReplyPreflight(ctx, q); e != nil {
			return 0, 0, e
		}
		if e := q.QueryRowContext(ctx, "SELECT count(*) FROM image_replies").Scan(&receipts); e != nil {
			return 0, 0, storageError(ctx, e)
		}
	}
	if e := q.QueryRowContext(ctx, `SELECT coalesce(sum(length(CAST(content AS BLOB))+length(CAST(reply AS BLOB))+CASE WHEN state IN ('processing','sending','ambiguous') THEN 16384 ELSE 0 END),0) FROM messages`).Scan(&bytes); e != nil {
		return 0, 0, storageError(ctx, e)
	}
	bytes += receipts * imageReplyReservation
	if bytes > 64<<20 {
		return 0, 0, ErrCapacity
	}
	return count, bytes, nil
}
