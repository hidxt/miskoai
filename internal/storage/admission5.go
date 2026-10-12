package storage

import "context"

func validateSchema5IfPresent(ctx context.Context, q rowQuery) error {
	var v int
	if e := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); e != nil {
		return storageError(ctx, e)
	}
	if v != 5 {
		return nil
	}
	return validateLibraryCapacity(ctx, q)
}

// SQL admits scalar types/ranges and text byte lengths before any library text
// or JSON is transferred into Go. Per-row bytes and count precede sum(bytes).
func libraryPreflight(ctx context.Context, q rowQuery) error {
	cols := []boundedColumn{{"account", 256, "text"}, {"user", 256, "text"}, {"sha256", 64, "text"}, {"format", 4, "text"}, {"tags", 2048, "text"}, {"emotion", 64, "text"}, {"description", 1024, "text"}, {"state", 8, "text"}}
	if e := validateRowShape(ctx, q, "library_assets", cols, `typeof(id)<>'integer' OR id<=0 OR typeof(bytes)<>'integer' OR bytes<1 OR bytes>4194304 OR typeof(width)<>'integer' OR width<1 OR width>8192 OR typeof(height)<>'integer' OR height<1 OR height>8192 OR width*height>16000000 OR typeof(frames)<>'integer' OR frames<1 OR frames>32 OR format NOT IN ('png','jpeg','gif') OR (format<>'gif' AND frames<>1) OR typeof(enabled)<>'integer' OR enabled NOT IN (0,1) OR state NOT IN ('ready','deleting') OR (state='deleting' AND enabled<>0) OR typeof(uses)<>'integer' OR uses<0 OR typeof(last_used)<>'integer' OR last_used<0 OR last_used>253402300799999`); e != nil {
		return e
	}
	var count int64
	if e := q.QueryRowContext(ctx, "SELECT count(*) FROM library_assets").Scan(&count); e != nil {
		return storageError(ctx, e)
	}
	if count > 128 {
		return ErrCapacity
	}
	if e := capacity(ctx, q, "SELECT count(*),coalesce(sum(bytes),0) FROM library_assets", 128, 64<<20); e != nil {
		return e
	}
	return capacity(ctx, q, `SELECT count(*),coalesce(sum(length(CAST(account AS BLOB))+length(CAST(user AS BLOB))+length(CAST(sha256 AS BLOB))+length(CAST(format AS BLOB))+length(CAST(tags AS BLOB))+length(CAST(emotion AS BLOB))+length(CAST(description AS BLOB))+length(CAST(state AS BLOB))),0) FROM library_assets`, 128, 128*(512+64+4+2048+64+1024+8))
}
func validateLibraryCapacity(ctx context.Context, q rowQuery) error {
	if e := libraryPreflight(ctx, q); e != nil {
		return e
	}
	if e := validateSequenceMetadata(ctx, q, 5); e != nil {
		return e
	}
	rows, e := q.QueryContext(ctx, "SELECT "+libraryColumns+" FROM library_assets")
	if e != nil {
		return storageError(ctx, e)
	}
	for rows.Next() {
		if _, e = scanLibrary(ctx, rows); e != nil {
			rows.Close()
			return e
		}
	}
	return closeValidatedRows(ctx, rows)
}
