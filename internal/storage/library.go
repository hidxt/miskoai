package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"unicode"
)

type LibraryAsset struct {
	ID                          int64
	Scope                       Scope
	SHA256, Format              string
	Bytes                       int64
	Width, Height, Frames       int
	Tags                        []string
	Emotion, Description, State string
	Enabled                     bool
	Uses, LastUsedMillis        int64
}
type LibraryMetadata struct {
	Tags                 []string
	Emotion, Description string
	Enabled              bool
}

const libraryColumns = `id,account,user,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used`

func libraryText(v string, max int, controls bool) bool {
	if !validField(v, max) {
		return false
	}
	if controls {
		for _, r := range v {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}
func libraryTags(tags []string) (string, error) {
	if len(tags) > 16 {
		return "", ErrInvalid
	}
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if tag == "" || !libraryText(tag, 32, true) || seen[tag] {
			return "", ErrInvalid
		}
		seen[tag] = true
	}
	if tags == nil {
		tags = []string{}
	}
	b, e := json.Marshal(tags)
	if e != nil || len(b) > 2048 {
		return "", ErrInvalid
	}
	return string(b), nil
}
func validLibraryAsset(a LibraryAsset) bool {
	if a.Scope.validate() != nil || a.ID < 0 || len(a.SHA256) != 64 || a.Bytes < 1 || a.Bytes > 4<<20 || a.Width < 1 || a.Width > 8192 || a.Height < 1 || a.Height > 8192 || int64(a.Width)*int64(a.Height) > 16000000 || a.Frames < 1 || a.Frames > 32 || a.Uses < 0 || a.LastUsedMillis < 0 || a.LastUsedMillis > 253402300799999 {
		return false
	}
	for _, c := range a.SHA256 {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	if a.Format != "gif" && (a.Format != "png" && a.Format != "jpeg" || a.Frames != 1) {
		return false
	}
	if a.State != "ready" && a.State != "deleting" || a.State == "deleting" && a.Enabled {
		return false
	}
	_, e := libraryTags(a.Tags)
	return e == nil && libraryText(a.Emotion, 64, true) && libraryText(a.Description, 1024, false)
}

type libraryScanner interface{ Scan(...any) error }

func scanLibrary(ctx context.Context, row libraryScanner) (LibraryAsset, error) {
	var a LibraryAsset
	var tags string
	var enabled int
	e := row.Scan(&a.ID, &a.Scope.Account, &a.Scope.User, &a.SHA256, &a.Format, &a.Bytes, &a.Width, &a.Height, &a.Frames, &tags, &a.Emotion, &a.Description, &a.State, &enabled, &a.Uses, &a.LastUsedMillis)
	if e == sql.ErrNoRows {
		return a, ErrNotFound
	}
	if e != nil {
		return a, storageError(ctx, e)
	}
	if len(tags) > 2048 || !libraryText(tags, 2048, false) || json.Unmarshal([]byte(tags), &a.Tags) != nil || a.Tags == nil {
		return LibraryAsset{}, ErrInvalid
	}
	canonical, e := libraryTags(a.Tags)
	if e != nil || canonical != tags || enabled < 0 || enabled > 1 || a.ID <= 0 {
		return LibraryAsset{}, ErrInvalid
	}
	a.Enabled = enabled == 1
	if !validLibraryAsset(a) {
		return LibraryAsset{}, ErrInvalid
	}
	return a, nil
}
func libraryGet(ctx context.Context, q rowQuery, sc Scope, id int64) (LibraryAsset, error) {
	return scanLibrary(ctx, q.QueryRowContext(ctx, "SELECT "+libraryColumns+" FROM library_assets WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id))
}

// AddLibraryAsset admits metadata only; actual bytes still require the full
// image validator. Scoped duplicates, including tombstones, remain unchanged.
func (s *Store) AddLibraryAsset(ctx context.Context, sc Scope, a LibraryAsset) (LibraryAsset, bool, error) {
	if sc.validate() != nil || a.Scope != sc || a.ID != 0 || a.Uses != 0 || a.LastUsedMillis != 0 || a.State != "ready" || a.Enabled || !validLibraryAsset(a) {
		return LibraryAsset{}, false, ErrInvalid
	}
	tags, _ := libraryTags(a.Tags)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return LibraryAsset{}, false, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateLibraryCapacity(ctx, tx); e != nil {
		return LibraryAsset{}, false, e
	}
	old, e := scanLibrary(ctx, tx.QueryRowContext(ctx, "SELECT "+libraryColumns+" FROM library_assets WHERE account=? AND user=? AND sha256=?", sc.Account, sc.User, a.SHA256))
	if e == nil {
		return old, false, nil
	}
	if e != ErrNotFound {
		return LibraryAsset{}, false, e
	}
	var count, bytes int64
	if e = tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(bytes),0) FROM library_assets").Scan(&count, &bytes); e != nil {
		return LibraryAsset{}, false, storageError(ctx, e)
	}
	if count >= 128 || bytes > (64<<20)-a.Bytes {
		return LibraryAsset{}, false, ErrCapacity
	}
	var seq int64
	if e = tx.QueryRowContext(ctx, "SELECT coalesce(max(seq),0) FROM sqlite_sequence WHERE name='library_assets'").Scan(&seq); e != nil {
		return LibraryAsset{}, false, storageError(ctx, e)
	}
	if seq == math.MaxInt64 {
		return LibraryAsset{}, false, ErrCapacity
	}
	result, e := tx.ExecContext(ctx, `INSERT INTO library_assets(account,user,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used) VALUES(?,?,?,?,?,?,?,?,?,?,?,'ready',0,0,0)`, sc.Account, sc.User, a.SHA256, a.Format, a.Bytes, a.Width, a.Height, a.Frames, tags, a.Emotion, a.Description)
	if e != nil {
		return LibraryAsset{}, false, storageError(ctx, e)
	}
	a.ID, e = result.LastInsertId()
	if e != nil {
		return LibraryAsset{}, false, storageError(ctx, e)
	}
	if e = tx.Commit(); e != nil {
		return LibraryAsset{}, false, storageError(ctx, e)
	}
	a.Tags = append([]string{}, a.Tags...)
	return a, true, nil
}
func (s *Store) LibraryAsset(ctx context.Context, sc Scope, id int64) (LibraryAsset, error) {
	if sc.validate() != nil || id <= 0 {
		return LibraryAsset{}, ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return LibraryAsset{}, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = libraryPreflight(ctx, tx); e != nil {
		return LibraryAsset{}, e
	}
	return libraryGet(ctx, tx, sc, id)
}
func libraryList(ctx context.Context, q rowQuery, query string, args ...any) ([]LibraryAsset, error) {
	rows, e := q.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, storageError(ctx, e)
	}
	assets := []LibraryAsset{}
	for rows.Next() {
		a, e := scanLibrary(ctx, rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		assets = append(assets, a)
	}
	if e = closeValidatedRows(ctx, rows); e != nil {
		return nil, e
	}
	return assets, nil
}
func (s *Store) LibraryAssets(ctx context.Context, sc Scope, after int64, limit int) ([]LibraryAsset, error) {
	if sc.validate() != nil || after < 0 || limit < 1 || limit > 32 {
		return nil, ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = libraryPreflight(ctx, tx); e != nil {
		return nil, e
	}
	return libraryList(ctx, tx, "SELECT "+libraryColumns+" FROM library_assets WHERE account=? AND user=? AND id>? ORDER BY id LIMIT ?", sc.Account, sc.User, after, limit)
}
func (s *Store) LibrarySnapshot(ctx context.Context) ([]LibraryAsset, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateLibraryCapacity(ctx, tx); e != nil {
		return nil, e
	}
	return libraryList(ctx, tx, "SELECT "+libraryColumns+" FROM library_assets ORDER BY id LIMIT 128")
}
func (s *Store) libraryMutation(ctx context.Context, sc Scope, id int64, mutate func(*sql.Tx, LibraryAsset) error) error {
	if sc.validate() != nil || id <= 0 {
		return ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = libraryPreflight(ctx, tx); e != nil {
		return e
	}
	a, e := libraryGet(ctx, tx, sc, id)
	if e != nil {
		return e
	}
	if e = mutate(tx, a); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit())
}
func (s *Store) UpdateLibraryAsset(ctx context.Context, sc Scope, id int64, meta LibraryMetadata) error {
	tags, e := libraryTags(meta.Tags)
	if e != nil || !libraryText(meta.Emotion, 64, true) || !libraryText(meta.Description, 1024, false) {
		return ErrInvalid
	}
	return s.libraryMutation(ctx, sc, id, func(tx *sql.Tx, a LibraryAsset) error {
		if a.State != "ready" {
			return ErrInvalid
		}
		_, e := tx.ExecContext(ctx, "UPDATE library_assets SET tags=?,emotion=?,description=?,enabled=? WHERE account=? AND user=? AND id=?", tags, meta.Emotion, meta.Description, meta.Enabled, sc.Account, sc.User, id)
		return storageError(ctx, e)
	})
}
func (s *Store) MarkLibraryDeleting(ctx context.Context, sc Scope, id int64) error {
	return s.libraryMutation(ctx, sc, id, func(tx *sql.Tx, a LibraryAsset) error {
		_, e := tx.ExecContext(ctx, "UPDATE library_assets SET state='deleting',enabled=0 WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id)
		return storageError(ctx, e)
	})
}
func (s *Store) FinishLibraryDelete(ctx context.Context, sc Scope, id int64) error {
	return s.libraryMutation(ctx, sc, id, func(tx *sql.Tx, a LibraryAsset) error {
		if a.State != "deleting" {
			return ErrInvalid
		}
		_, e := tx.ExecContext(ctx, "DELETE FROM library_assets WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id)
		return storageError(ctx, e)
	})
}
func (s *Store) RecordLibraryUse(ctx context.Context, sc Scope, id int64, atMillis int64) error {
	if atMillis < 1 || atMillis > 253402300799999 {
		return ErrInvalid
	}
	return s.libraryMutation(ctx, sc, id, func(tx *sql.Tx, a LibraryAsset) error {
		if a.State != "ready" || !a.Enabled {
			return ErrInvalid
		}
		if a.Uses == math.MaxInt64 {
			return ErrCapacity
		}
		_, e := tx.ExecContext(ctx, "UPDATE library_assets SET uses=uses+1,last_used=? WHERE account=? AND user=? AND id=?", atMillis, sc.Account, sc.User, id)
		return storageError(ctx, e)
	})
}
