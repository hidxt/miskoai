package storage

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"time"
	"unicode"
)

type Summary struct {
	Text                string
	Watermark, Revision int64
	UpdatedAt           time.Time
}
type Candidate struct {
	ID                        int64
	Content, MessageID, Quote string
	CreatedAt                 time.Time
}
type ContextSnapshot struct {
	History []Turn
	Facts   []Fact
	Summary Summary
	Profile Profile
}

func derivedText(v string, max int, nonempty bool) bool {
	if !validField(v, max) || (nonempty && strings.TrimSpace(v) == "") {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func validCandidate(c Candidate) bool {
	return validMessageID(c.MessageID) && derivedText(c.MessageID, 512, true) && derivedText(c.Content, 1024, true) && derivedText(c.Quote, 1024, true)
}

func readDerived(ctx context.Context, q rowQuery, sc Scope) (Summary, error) {
	var d Summary
	var stamp int64
	e := q.QueryRowContext(ctx, "SELECT text,watermark,revision,updated_at FROM derived WHERE account=? AND user=?", sc.Account, sc.User).Scan(&d.Text, &d.Watermark, &d.Revision, &stamp)
	if e == sql.ErrNoRows {
		return d, nil
	}
	if e != nil {
		return d, storageError(ctx, e)
	}
	d.UpdatedAt = time.UnixMilli(stamp).UTC()
	return d, nil
}
func completedCount(ctx context.Context, q rowQuery, sc Scope) (int64, error) {
	var n int64
	e := q.QueryRowContext(ctx, "SELECT count(*) FROM messages WHERE account=? AND user=? AND state='sent'", sc.Account, sc.User).Scan(&n)
	return n, storageError(ctx, e)
}
func (s *Store) CompletedCount(ctx context.Context, sc Scope) (int64, error) {
	if e := sc.validate(); e != nil {
		return 0, e
	}
	return completedCount(ctx, s.db, sc)
}
func (s *Store) Derived(ctx context.Context, sc Scope) (Summary, error) {
	if e := sc.validate(); e != nil {
		return Summary{}, e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Summary{}, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateDerivedCapacity(ctx, tx); e != nil {
		return Summary{}, e
	}
	d, e := readDerived(ctx, tx, sc)
	if e != nil {
		return Summary{}, e
	}
	return d, storageError(ctx, tx.Commit())
}

func (s *Store) DerivedHistory(ctx context.Context, sc Scope, pairs int) (Summary, int64, []Turn, error) {
	var d Summary
	if e := sc.validate(); e != nil {
		return d, 0, nil, e
	}
	if pairs < 1 || pairs > 16 {
		return d, 0, nil, ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return d, 0, nil, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateDerivedCapacity(ctx, tx); e != nil {
		return d, 0, nil, e
	}
	if e = validateLegacyShape(ctx, tx); e != nil {
		return d, 0, nil, e
	}
	d, e = readDerived(ctx, tx, sc)
	if e != nil {
		return d, 0, nil, e
	}
	n, e := completedCount(ctx, tx, sc)
	if e != nil {
		return d, 0, nil, e
	}
	h, e := history(ctx, tx, sc, pairs)
	if e != nil {
		return d, 0, nil, e
	}
	return d, n, h, storageError(ctx, tx.Commit())
}
func (s *Store) ChatContext(ctx context.Context, sc Scope, query string, pairs, factLimit int) (ContextSnapshot, error) {
	return s.chatContext(ctx, sc, query, pairs, factLimit, "")
}

// ChatContextWithProfile captures an explicit scoped expression profile in the
// same transaction as memory. Empty profileID uses the persistent selection.
// An override never changes that selection.
func (s *Store) ChatContextWithProfile(ctx context.Context, sc Scope, query string, pairs, factLimit int, profileID string) (ContextSnapshot, error) {
	return s.chatContext(ctx, sc, query, pairs, factLimit, profileID)
}
func (s *Store) chatContext(ctx context.Context, sc Scope, query string, pairs, factLimit int, profileID string) (ContextSnapshot, error) {
	var snap ContextSnapshot
	if e := sc.validate(); e != nil {
		return snap, e
	}
	if pairs < 1 || pairs > 16 || factLimit < 1 || factLimit > 8 || !validFactQuery(query) || (profileID != "" && !validProfileID(profileID)) {
		return snap, ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return snap, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateDerivedCapacity(ctx, tx); e != nil {
		return snap, e
	}
	if e = validateProfileCapacity(ctx, tx); e != nil {
		return snap, e
	}
	if e = validateLegacyShape(ctx, tx); e != nil {
		return snap, e
	}
	if snap.Summary, e = readDerived(ctx, tx, sc); e != nil {
		return snap, e
	}
	if profileID == "" {
		snap.Profile, e = activeProfile(ctx, tx, sc)
	} else {
		snap.Profile, e = profileByID(ctx, tx, sc, profileID)
	}
	if e != nil {
		return snap, e
	}
	if snap.History, e = history(ctx, tx, sc, pairs); e != nil {
		return snap, e
	}
	if snap.Facts, e = searchFacts(ctx, tx, sc, query, factLimit); e != nil {
		return snap, e
	}
	return snap, storageError(ctx, tx.Commit())
}

func writeDerived(ctx context.Context, tx *sql.Tx, sc Scope, old Summary, text string, watermark int64) error {
	if old.Revision == math.MaxInt64 {
		return ErrCapacity
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO derived(account,user,text,watermark,revision,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(account,user) DO UPDATE SET text=excluded.text,watermark=excluded.watermark,revision=excluded.revision,updated_at=excluded.updated_at`, sc.Account, sc.User, text, watermark, old.Revision+1, time.Now().UTC().UnixMilli())
	if e != nil {
		return storageError(ctx, e)
	}
	return nil
}
func (s *Store) SaveDerived(ctx context.Context, sc Scope, expectedRevision int64, d Summary, cs []Candidate) error {
	if e := sc.validate(); e != nil {
		return e
	}
	if expectedRevision < 0 || d.Watermark < 0 || !derivedText(d.Text, 8192, false) || len(cs) > 8 {
		return ErrInvalid
	}
	for _, c := range cs {
		if !validCandidate(c) {
			return ErrInvalid
		}
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateDerivedCapacity(ctx, tx); e != nil {
		return e
	}
	old, e := readDerived(ctx, tx, sc)
	if e != nil {
		return e
	}
	if old.Revision != expectedRevision {
		return ErrStale
	}
	n, e := completedCount(ctx, tx, sc)
	if e != nil {
		return e
	}
	if d.Watermark > n || d.Watermark < old.Watermark {
		return ErrInvalid
	}
	for _, c := range cs {
		// SQLite checks the scoped USER column directly; no assistant reply is evidence.
		var proof int
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE account=? AND user=? AND id=? AND state='sent' AND instr(content,?)>0)`, sc.Account, sc.User, c.MessageID, c.Quote).Scan(&proof); e != nil {
			return storageError(ctx, e)
		}
		if proof != 1 {
			return ErrInvalid
		}
	}
	if e = writeDerived(ctx, tx, sc, old, d.Text, d.Watermark); e != nil {
		return e
	}
	for _, c := range cs {
		if _, e = tx.ExecContext(ctx, `INSERT INTO candidates(account,user,content,message_id,quote,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(account,user,message_id,quote,content) DO NOTHING`, sc.Account, sc.User, c.Content, c.MessageID, c.Quote, time.Now().UTC().UnixMilli()); e != nil {
			return storageError(ctx, e)
		}
	}
	if e = validateDerivedCapacity(ctx, tx); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit())
}
func (s *Store) Candidates(ctx context.Context, sc Scope, limit int) ([]Candidate, error) {
	if e := sc.validate(); e != nil {
		return nil, e
	}
	if limit < 1 || limit > 128 {
		return nil, ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateDerivedCapacity(ctx, tx); e != nil {
		return nil, e
	}
	rows, e := tx.QueryContext(ctx, "SELECT id,content,message_id,quote,created_at FROM candidates WHERE account=? AND user=? ORDER BY id LIMIT ?", sc.Account, sc.User, limit)
	if e != nil {
		return nil, storageError(ctx, e)
	}
	out := []Candidate{}
	for rows.Next() {
		var c Candidate
		var stamp int64
		if e = rows.Scan(&c.ID, &c.Content, &c.MessageID, &c.Quote, &stamp); e != nil {
			rows.Close()
			return nil, storageError(ctx, e)
		}
		c.CreatedAt = time.UnixMilli(stamp).UTC()
		out = append(out, c)
	}
	if e = closeValidatedRows(ctx, rows); e != nil {
		return nil, e
	}
	return out, storageError(ctx, tx.Commit())
}
func (s *Store) ConfirmCandidate(ctx context.Context, sc Scope, id int64) (int64, error) {
	if e := sc.validate(); e != nil {
		return 0, e
	}
	if id <= 0 {
		return 0, ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return 0, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateDerivedCapacity(ctx, tx); e != nil {
		return 0, e
	}
	var content string
	e = tx.QueryRowContext(ctx, "SELECT content FROM candidates WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id).Scan(&content)
	if e == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if e != nil {
		return 0, storageError(ctx, e)
	}
	factID, e := insertFact(ctx, tx, sc, Fact{Content: content, Source: "explicit_user", Confidence: 1, Importance: 50})
	if e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM candidates WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id); e != nil {
		return 0, storageError(ctx, e)
	}
	return factID, storageError(ctx, tx.Commit())
}
func (s *Store) DeleteCandidate(ctx context.Context, sc Scope, id int64) error {
	if e := sc.validate(); e != nil {
		return e
	}
	if id <= 0 {
		return ErrInvalid
	}
	r, e := s.db.ExecContext(ctx, "DELETE FROM candidates WHERE account=? AND user=? AND id=?", sc.Account, sc.User, id)
	return changed(ctx, r, e)
}
func (s *Store) ClearDerived(ctx context.Context, sc Scope) error {
	return s.clearMemory(ctx, sc, false)
}

// ClearMemory erases scoped memory and context while keeping all durable claims.
// Administrators must quiesce scoped processing first; otherwise an already
// admitted handler can subsequently persist its reply. Agent commands serialize
// this operation within their normal scoped handler.
func (s *Store) ClearMemory(ctx context.Context, sc Scope) error { return s.clearMemory(ctx, sc, true) }
func (s *Store) clearMemory(ctx context.Context, sc Scope, all bool) error {
	if e := sc.validate(); e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateDerivedCapacity(ctx, tx); e != nil {
		return e
	}
	old, e := readDerived(ctx, tx, sc)
	if e != nil {
		return e
	}
	n, e := completedCount(ctx, tx, sc)
	if e != nil {
		return e
	}
	if e = writeDerived(ctx, tx, sc, old, "", n); e != nil {
		return e
	}
	queries := []string{"DELETE FROM candidates WHERE account=? AND user=?"}
	if all {
		queries = append(queries, "DELETE FROM facts WHERE account=? AND user=?", "UPDATE messages SET content='',reply='' WHERE account=? AND user=?")
	}
	for _, q := range queries {
		if _, e = tx.ExecContext(ctx, q, sc.Account, sc.User); e != nil {
			return storageError(ctx, e)
		}
	}
	if e = validateDerivedCapacity(ctx, tx); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit())
}
