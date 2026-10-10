package storage

import "context"

func validateSchema3IfPresent(ctx context.Context, q rowQuery) error {
	var v int
	if e := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); e != nil {
		return storageError(ctx, e)
	}
	if v == 2 {
		return validateSequenceMetadata(ctx, q, v)
	}
	if v != 3 && v != 4 {
		return nil
	}
	if e := validateDerivedCapacity(ctx, q); e != nil {
		return e
	}
	return validateProfileCapacity(ctx, q)
}
func capacity(ctx context.Context, q rowQuery, query string, maxCount, maxBytes int64) error {
	var count, bytes int64
	if e := q.QueryRowContext(ctx, query).Scan(&count, &bytes); e != nil {
		return storageError(ctx, e)
	}
	if count > maxCount || bytes > maxBytes {
		return ErrCapacity
	}
	return nil
}
func noUnsafe(ctx context.Context, q rowQuery, query string) error {
	var bad int
	if e := q.QueryRowContext(ctx, query).Scan(&bad); e != nil {
		return storageError(ctx, e)
	}
	if bad != 0 {
		return ErrInvalid
	}
	return nil
}
func scopeQuota(ctx context.Context, q rowQuery, query string) error {
	var bad int
	if e := q.QueryRowContext(ctx, query).Scan(&bad); e != nil {
		return storageError(ctx, e)
	}
	if bad != 0 {
		return ErrCapacity
	}
	return nil
}

func validateDerivedCapacity(ctx context.Context, q rowQuery) error {
	if e := validateSequenceMetadata(ctx, q, 3); e != nil {
		return e
	}
	if e := capacity(ctx, q, "SELECT count(*),coalesce(sum(length(CAST(text AS BLOB))),0) FROM derived", 16, 128<<10); e != nil {
		return e
	}
	if e := capacity(ctx, q, "SELECT count(*),coalesce(sum(length(CAST(content AS BLOB))+length(CAST(quote AS BLOB))),0) FROM candidates", 256, 256<<10); e != nil {
		return e
	}
	if e := scopeQuota(ctx, q, "SELECT EXISTS(SELECT 1 FROM candidates GROUP BY account,user HAVING count(*)>128)"); e != nil {
		return e
	}
	sc := []boundedColumn{{"account", 256, "text"}, {"user", 256, "text"}}
	cols := append(append([]boundedColumn{}, sc...), boundedColumn{"text", 8192, "text"})
	// Unix milliseconds stay within time.Time's ordinary year1..9999 range.
	if e := validateRowShape(ctx, q, "derived", cols, `typeof(watermark)<>'integer' OR watermark<0 OR typeof(revision)<>'integer' OR revision<1 OR typeof(updated_at)<>'integer' OR updated_at<1 OR updated_at>253402300799999 OR watermark>(SELECT count(*) FROM messages m WHERE m.account=derived.account AND m.user=derived.user AND m.state='sent')`); e != nil {
		return e
	}
	cols = append(append([]boundedColumn{}, sc...), boundedColumn{"content", 1024, "text"}, boundedColumn{"message_id", 512, "text"}, boundedColumn{"quote", 1024, "text"})
	if e := validateRowShape(ctx, q, "candidates", cols, `typeof(id)<>'integer' OR id<=0 OR typeof(created_at)<>'integer' OR created_at<1 OR created_at>253402300799999`); e != nil {
		return e
	}
	if e := noUnsafe(ctx, q, `SELECT EXISTS(SELECT 1 FROM candidates c WHERE NOT EXISTS(SELECT 1 FROM messages m WHERE m.account=c.account AND m.user=c.user AND m.id=c.message_id AND m.state='sent' AND instr(m.content,c.quote)>0))`); e != nil {
		return e
	}
	rows, e := q.QueryContext(ctx, "SELECT account,user,text FROM derived")
	if e != nil {
		return storageError(ctx, e)
	}
	for rows.Next() {
		var scope Scope
		var text string
		if e = rows.Scan(&scope.Account, &scope.User, &text); e != nil {
			rows.Close()
			return storageError(ctx, e)
		}
		if scope.validate() != nil || !derivedText(text, 8192, false) {
			rows.Close()
			return ErrInvalid
		}
	}
	if e = closeValidatedRows(ctx, rows); e != nil {
		return e
	}
	rows, e = q.QueryContext(ctx, "SELECT account,user,content,message_id,quote FROM candidates")
	if e != nil {
		return storageError(ctx, e)
	}
	for rows.Next() {
		var scope Scope
		var c Candidate
		if e = rows.Scan(&scope.Account, &scope.User, &c.Content, &c.MessageID, &c.Quote); e != nil {
			rows.Close()
			return storageError(ctx, e)
		}
		if scope.validate() != nil || !validCandidate(c) {
			rows.Close()
			return ErrInvalid
		}
	}
	return closeValidatedRows(ctx, rows)
}
func validateSequenceMetadata(ctx context.Context, q rowQuery, version int) error {
	countQuery := "SELECT count(*)>2 FROM sqlite_sequence"
	allowed := `name NOT IN ('poll_frames','inbox') OR typeof(seq)<>'integer' OR seq<0`
	if version == 3 || version == 4 {
		countQuery = "SELECT count(*)>3 FROM sqlite_sequence"
		allowed = `name NOT IN ('poll_frames','inbox','candidates') OR typeof(seq)<>'integer' OR seq<0`
	} else if version != 2 {
		return ErrInvalid
	}
	if e := noUnsafe(ctx, q, countQuery); e != nil {
		return e
	}
	if e := validateRowShape(ctx, q, "sqlite_sequence", []boundedColumn{{"name", 64, "text"}}, allowed); e != nil {
		return e
	}
	if e := noUnsafe(ctx, q, "SELECT EXISTS(SELECT 1 FROM sqlite_sequence GROUP BY name HAVING count(*)>1)"); e != nil {
		return e
	}
	if e := validateSequence(ctx, q, "poll_frames", "id"); e != nil {
		return e
	}
	if e := validateSequence(ctx, q, "inbox", "sequence"); e != nil {
		return e
	}
	if version == 3 || version == 4 {
		return validateSequence(ctx, q, "candidates", "id")
	}
	return nil
}
func validateSequence(ctx context.Context, q rowQuery, table, column string) error {
	// Table/column arguments are fixed implementation identifiers, never input.
	return noUnsafe(ctx, q, `SELECT EXISTS(SELECT 1 FROM sqlite_sequence WHERE name='`+table+`' AND (typeof(seq)<>'integer' OR seq<0 OR seq<(SELECT coalesce(max(`+column+`),0) FROM `+table+`))) OR (SELECT count(*) FROM sqlite_sequence WHERE name='`+table+`')>1 OR ((SELECT count(*) FROM `+table+`)>0 AND NOT EXISTS(SELECT 1 FROM sqlite_sequence WHERE name='`+table+`'))`)
}
func validateProfileCapacity(ctx context.Context, q rowQuery) error {
	if e := capacity(ctx, q, "SELECT count(*),coalesce(sum(length(CAST(body AS BLOB))),0) FROM profiles", 64, 128<<10); e != nil {
		return e
	}
	if e := capacity(ctx, q, "SELECT count(*),0 FROM profile_selections", 16, 0); e != nil {
		return e
	}
	if e := scopeQuota(ctx, q, "SELECT EXISTS(SELECT 1 FROM profiles GROUP BY account,user HAVING count(*)>16)"); e != nil {
		return e
	}
	sc := []boundedColumn{{"account", 256, "text"}, {"user", 256, "text"}, {"id", 64, "text"}}
	if e := validateRowShape(ctx, q, "profiles", append(append([]boundedColumn{}, sc...), boundedColumn{"body", 8192, "text"}), ""); e != nil {
		return e
	}
	if e := validateRowShape(ctx, q, "profile_selections", sc, ""); e != nil {
		return e
	}
	rows, e := q.QueryContext(ctx, "SELECT account,user,id,body FROM profiles")
	if e != nil {
		return storageError(ctx, e)
	}
	for rows.Next() {
		var scope Scope
		var id, body string
		if e = rows.Scan(&scope.Account, &scope.User, &id, &body); e != nil {
			rows.Close()
			return storageError(ctx, e)
		}
		p, err := decodeProfile(body)
		if scope.validate() != nil || err != nil || p.ID != id {
			rows.Close()
			return ErrInvalid
		}
	}
	if e = closeValidatedRows(ctx, rows); e != nil {
		return e
	}
	rows, e = q.QueryContext(ctx, "SELECT account,user,id FROM profile_selections")
	if e != nil {
		return storageError(ctx, e)
	}
	for rows.Next() {
		var scope Scope
		var id string
		if e = rows.Scan(&scope.Account, &scope.User, &id); e != nil {
			rows.Close()
			return storageError(ctx, e)
		}
		if scope.validate() != nil || !validProfileID(id) {
			rows.Close()
			return ErrInvalid
		}
	}
	if e = closeValidatedRows(ctx, rows); e != nil {
		return e
	}
	return noUnsafe(ctx, q, `SELECT EXISTS(SELECT 1 FROM profile_selections s WHERE id NOT IN ('warm','concise','professional') AND NOT EXISTS(SELECT 1 FROM profiles p WHERE p.account=s.account AND p.user=s.user AND p.id=s.id))`)
}
