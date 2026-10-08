package storage

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxFacts = 10000
const maxFactBytes = 4 << 20
const factColumns = "id,content,category,source,confidence,importance,created_at,updated_at,expires_at"

type Fact struct {
	ID                        int64
	Content, Category, Source string
	Confidence                float64
	Importance                int
	CreatedAt, UpdatedAt      time.Time
	ExpiresAt                 *time.Time
}

func (f Fact) validate() error {
	if strings.TrimSpace(f.Content) == "" || len(f.Content) > 16384 || len(f.Category) > 256 || !utf8.ValidString(f.Content) || !utf8.ValidString(f.Category) || strings.ContainsRune(f.Content, 0) || strings.ContainsRune(f.Category, 0) {
		return ErrInvalid
	}
	if f.Source != "manual" && f.Source != "explicit_user" {
		return ErrInvalid
	}
	if math.IsNaN(f.Confidence) || math.IsInf(f.Confidence, 0) || f.Confidence < 0 || f.Confidence > 1 || f.Importance < 0 || f.Importance > 100 {
		return ErrInvalid
	}
	return nil
}

func expiry(f Fact) any {
	if f.ExpiresAt == nil {
		return nil
	}
	return f.ExpiresAt.UTC().UnixMilli()
}

func (s *Store) AddFact(ctx context.Context, scope Scope, f Fact) (int64, error) {
	if err := scope.validate(); err != nil {
		return 0, err
	}
	if err := f.validate(); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, storageError(ctx, err)
	}
	defer tx.Rollback()
	var count, bytes int
	if err = tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(length(CAST(content AS BLOB))),0) FROM facts WHERE account=? AND user=?", scope.Account, scope.User).Scan(&count, &bytes); err != nil {
		return 0, storageError(ctx, err)
	}
	if count >= maxFacts || bytes+len(f.Content) > maxFactBytes {
		return 0, ErrCapacity
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*),coalesce(sum(length(CAST(content AS BLOB))),0) FROM facts").Scan(&count, &bytes); err != nil {
		return 0, storageError(ctx, err)
	}
	if count >= maxFacts || bytes+len(f.Content) > 8<<20 {
		return 0, ErrCapacity
	}
	now := time.Now().UTC().UnixMilli()
	result, err := tx.ExecContext(ctx, `INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, scope.Account, scope.User, f.Content, f.Category, f.Source, f.Confidence, f.Importance, now, now, expiry(f))
	if err != nil {
		return 0, storageError(ctx, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, storageError(ctx, err)
	}
	if err = tx.Commit(); err != nil {
		return 0, storageError(ctx, err)
	}
	return id, nil
}

func (s *Store) UpdateFact(ctx context.Context, scope Scope, f Fact) error {
	if err := scope.validate(); err != nil {
		return err
	}
	if err := f.validate(); err != nil {
		return err
	}
	if f.ID <= 0 {
		return ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storageError(ctx, err)
	}
	defer tx.Rollback()
	var oldBytes, totalBytes int
	err = tx.QueryRowContext(ctx, `SELECT length(CAST(content AS BLOB)),(SELECT coalesce(sum(length(CAST(content AS BLOB))),0) FROM facts WHERE account=? AND user=?) FROM facts WHERE account=? AND user=? AND id=?`, scope.Account, scope.User, scope.Account, scope.User, f.ID).Scan(&oldBytes, &totalBytes)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return storageError(ctx, err)
	}
	if totalBytes-oldBytes+len(f.Content) > maxFactBytes {
		return ErrCapacity
	}
	if err = tx.QueryRowContext(ctx, "SELECT coalesce(sum(length(CAST(content AS BLOB))),0) FROM facts").Scan(&totalBytes); err != nil {
		return storageError(ctx, err)
	}
	if totalBytes-oldBytes+len(f.Content) > 8<<20 {
		return ErrCapacity
	}
	result, err := tx.ExecContext(ctx, `UPDATE facts SET content=?,category=?,source=?,confidence=?,importance=?,updated_at=?,expires_at=? WHERE account=? AND user=? AND id=?`, f.Content, f.Category, f.Source, f.Confidence, f.Importance, time.Now().UTC().UnixMilli(), expiry(f), scope.Account, scope.User, f.ID)
	if err = changed(ctx, result, err); err != nil {
		return err
	}
	return storageError(ctx, tx.Commit())
}

func changed(ctx context.Context, result sql.Result, err error) error {
	if err != nil {
		return storageError(ctx, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return storageError(ctx, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteFact(ctx context.Context, scope Scope, id int64) error {
	if err := scope.validate(); err != nil {
		return err
	}
	if id <= 0 {
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM facts WHERE account=? AND user=? AND id=?", scope.Account, scope.User, id)
	return changed(ctx, result, err)
}

func (s *Store) ClearFacts(ctx context.Context, scope Scope) error {
	if err := scope.validate(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM facts WHERE account=? AND user=?", scope.Account, scope.User)
	return storageError(ctx, err)
}

// SearchFacts searches quoted FTS terms plus literal substrings. The latter is
// necessary for Chinese substrings that unicode61 indexes as one long token.
// The scope, expiry predicate and limit apply to both retrieval paths.
func (s *Store) SearchFacts(ctx context.Context, scope Scope, query string, limit int) ([]Fact, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 1000 || len(query) > 1024 || !utf8.ValidString(query) || strings.ContainsRune(query, 0) {
		return nil, ErrInvalid
	}
	q := `SELECT ` + factColumns + ` FROM facts WHERE account=? AND user=? AND (expires_at IS NULL OR expires_at>?)`
	args := []any{scope.Account, scope.User, time.Now().UTC().UnixMilli()}
	terms := strings.FieldsFunc(query, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(terms) > 32 {
		return nil, ErrInvalid
	}
	if strings.TrimSpace(query) != "" {
		// All MATCH syntax is generated here; quotes in untrusted input cannot become operators.
		var fts []string
		for _, term := range terms {
			fts = append(fts, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
		}
		q += ` AND (`
		if len(fts) > 0 {
			q += `id IN (SELECT rowid FROM facts_fts WHERE facts_fts MATCH ?) OR `
			args = append(args, strings.Join(fts, " OR "))
		}
		q += `content LIKE ? ESCAPE '\'`
		args = append(args, likePattern(strings.TrimSpace(query)))
		// Each term also receives a keyword path; terms themselves cannot introduce wildcards.
		for _, term := range terms {
			q += ` OR content LIKE ? ESCAPE '\'`
			args = append(args, likePattern(term))
		}
		q += `)`
	}
	q += ` ORDER BY importance DESC,updated_at DESC,id DESC LIMIT ?`
	args = append(args, limit)
	return s.readFacts(ctx, q, args...)
}

func likePattern(v string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(v) + "%"
}

// ExportFacts includes expired records so users can inspect and delete retained
// facts. Insertion/update caps bound export to 10,000 records/4MiB of content.
func (s *Store) ExportFacts(ctx context.Context, scope Scope) ([]Fact, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	return s.readFacts(ctx, `SELECT `+factColumns+` FROM facts WHERE account=? AND user=? ORDER BY id LIMIT ?`, scope.Account, scope.User, maxFacts)
}

func (s *Store) readFacts(ctx context.Context, q string, args ...any) ([]Fact, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer rows.Close()
	facts := []Fact{}
	for rows.Next() {
		var f Fact
		var created, updated int64
		var expires sql.NullInt64
		if err = rows.Scan(&f.ID, &f.Content, &f.Category, &f.Source, &f.Confidence, &f.Importance, &created, &updated, &expires); err != nil {
			return nil, storageError(ctx, err)
		}
		f.CreatedAt = time.UnixMilli(created).UTC()
		f.UpdatedAt = time.UnixMilli(updated).UTC()
		if expires.Valid {
			t := time.UnixMilli(expires.Int64).UTC()
			f.ExpiresAt = &t
		}
		facts = append(facts, f)
	}
	return facts, storageError(ctx, rows.Err())
}
