package storage

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"
)

type PollFrame struct {
	ID     int64
	Scope  Scope
	Cursor string
	Body   []byte
	State  string
}
type InboxEntry struct {
	Sequence                      int64
	Scope                         Scope
	MessageID, Text, ContextToken string
	ReceivedAt                    time.Time
	Attachment                    *Attachment
}

func validField(v string, max int) bool {
	return len(v) <= max && utf8.ValidString(v) && !strings.ContainsRune(v, 0)
}

// RecordPoll persists opaque evidence before decoding. A quarantined frame is
// still unresolved and blocks further receiving for its entire account.
func (s *Store) RecordPoll(ctx context.Context, scope Scope, cursor string, body []byte) (int64, error) {
	if err := scope.validate(); err != nil {
		return 0, err
	}
	if !validField(cursor, 16384) || len(body) > 2<<20 {
		return 0, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, storageError(ctx, err)
	}
	defer tx.Rollback()
	var unresolved, count, bytes int
	if err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(length(CAST(body AS BLOB))),0),coalesce(sum(CASE WHEN account=? THEN 1 ELSE 0 END),0) FROM poll_frames`, scope.Account).Scan(&count, &bytes, &unresolved); err != nil {
		return 0, storageError(ctx, err)
	}
	if unresolved != 0 || count >= 4 || bytes+len(body) > 8<<20 {
		return 0, ErrCapacity
	}
	cur, err := cursorFor(ctx, tx, scope)
	if err != nil {
		return 0, err
	}
	if cur != cursor {
		return 0, ErrInvalid
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO poll_frames(account,user,cursor,body,state) VALUES(?,?,?,COALESCE(?,X''),'pending')`, scope.Account, scope.User, cursor, body)
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

func (s *Store) PendingPoll(ctx context.Context, scope Scope) (*PollFrame, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer tx.Rollback()
	if err = validateReceiveRows(ctx, tx); err != nil {
		return nil, err
	}
	f := &PollFrame{Scope: scope}
	err = tx.QueryRowContext(ctx, `SELECT id,cursor,body,state FROM poll_frames WHERE account=? AND user=?`, scope.Account, scope.User).Scan(&f.ID, &f.Cursor, &f.Body, &f.State)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, storageError(ctx, err)
	}
	if err = tx.Commit(); err != nil {
		return nil, storageError(ctx, err)
	}
	return f, nil
}

// ResolvePoll inserts only new work and advances the account cursor in the
// same transaction. Any invalid entry or capacity error retains raw evidence.
func (s *Store) ResolvePoll(ctx context.Context, scope Scope, frameID int64, nextCursor string, entries []InboxEntry) error {
	if err := scope.validate(); err != nil {
		return err
	}
	if frameID <= 0 || !validField(nextCursor, 16384) || len(entries) > 256 {
		return ErrInvalid
	}
	attachments := make([][]byte, len(entries))
	for i, e := range entries {
		if e.Scope != scope || !validMessageID(e.MessageID) || !validField(e.Text, 16384) || !validField(e.ContextToken, 16384) || e.ReceivedAt.IsZero() || e.ReceivedAt.UnixMilli() <= 0 {
			return ErrInvalid
		}
		var err error
		attachments[i], err = encodeAttachment(e.Attachment)
		if err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storageError(ctx, err)
	}
	defer tx.Rollback()
	var previous string
	err = tx.QueryRowContext(ctx, `SELECT cursor FROM poll_frames WHERE id=? AND account=? AND user=? AND state='pending'`, frameID, scope.Account, scope.User).Scan(&previous)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return storageError(ctx, err)
	}
	cur, err := cursorFor(ctx, tx, scope)
	if err != nil {
		return err
	}
	if cur != previous {
		return ErrInvalid
	}
	if err = validateReceiveRows(ctx, tx); err != nil {
		return err
	}
	count, bytes, err := receiveQueueBudget(ctx, tx)
	if err != nil {
		return err
	}
	for i, e := range entries {
		var exists int
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inbox WHERE account=? AND user=? AND message_id=? UNION ALL SELECT 1 FROM messages WHERE account=? AND user=? AND id=?)`, scope.Account, scope.User, e.MessageID, scope.Account, scope.User, e.MessageID).Scan(&exists); err != nil {
			return storageError(ctx, err)
		}
		if exists != 0 {
			continue
		}
		if count >= 1024 || bytes+len(e.Text)+len(e.ContextToken)+len(attachments[i]) > 8<<20 {
			return ErrCapacity
		}
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES(?,?,?,?,?,?)`, scope.Account, scope.User, e.MessageID, e.Text, e.ContextToken, e.ReceivedAt.UTC().UnixMilli())
		if insertErr != nil {
			err = insertErr
			return storageError(ctx, err)
		}
		if attachments[i] != nil {
			sequence, e := result.LastInsertId()
			if e != nil {
				return storageError(ctx, e)
			}
			if _, e = tx.ExecContext(ctx, `INSERT INTO inbox_attachments(sequence,body) VALUES(?,?)`, sequence, attachments[i]); e != nil {
				return storageError(ctx, e)
			}
		}
		count++
		bytes += len(e.Text) + len(e.ContextToken) + len(attachments[i])
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO channel_cursors(account,user,cursor) VALUES(?,?,?) ON CONFLICT(account) DO UPDATE SET cursor=excluded.cursor WHERE channel_cursors.user=excluded.user`, scope.Account, scope.User, nextCursor); err != nil {
		return storageError(ctx, err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM poll_frames WHERE id=? AND account=? AND user=?`, frameID, scope.Account, scope.User); err != nil {
		return storageError(ctx, err)
	}
	return storageError(ctx, tx.Commit())
}
func (s *Store) QuarantinePoll(ctx context.Context, scope Scope, frameID int64) error {
	if err := scope.validate(); err != nil {
		return err
	}
	if frameID <= 0 {
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE poll_frames SET state='quarantined' WHERE id=? AND account=? AND user=? AND state='pending'`, frameID, scope.Account, scope.User)
	return changed(ctx, result, err)
}

type rowQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func cursorFor(ctx context.Context, q rowQuery, scope Scope) (string, error) {
	var user, cur string
	err := q.QueryRowContext(ctx, `SELECT user,cursor FROM channel_cursors WHERE account=?`, scope.Account).Scan(&user, &cur)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", storageError(ctx, err)
	}
	if user != scope.User {
		return "", ErrNotFound
	}
	return cur, nil
}

// Cursor is account-wide; the stored authorized user must match the caller.
func (s *Store) Cursor(ctx context.Context, scope Scope) (string, error) {
	if err := scope.validate(); err != nil {
		return "", err
	}
	return cursorFor(ctx, s.db, scope)
}
func (s *Store) PendingInbox(ctx context.Context, scope Scope, limit int) ([]InboxEntry, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 32 {
		return nil, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer tx.Rollback()
	if err = validateReceiveRows(ctx, tx); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT i.sequence,i.message_id,i.text,i.context_token,i.received_at,a.body FROM inbox i LEFT JOIN inbox_attachments a ON a.sequence=i.sequence WHERE i.account=? AND i.user=? ORDER BY i.sequence LIMIT ?`, scope.Account, scope.User, limit)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer rows.Close()
	entries := []InboxEntry{}
	for rows.Next() {
		e := InboxEntry{Scope: scope}
		var ms int64
		var body []byte
		if err = rows.Scan(&e.Sequence, &e.MessageID, &e.Text, &e.ContextToken, &ms, &body); err != nil {
			return nil, storageError(ctx, err)
		}
		if body != nil {
			e.Attachment, err = decodeAttachment(body)
			if err != nil {
				return nil, err
			}
		}
		e.ReceivedAt = time.UnixMilli(ms).UTC()
		entries = append(entries, e)
	}
	if err = closeValidatedRows(ctx, rows); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, storageError(ctx, err)
	}
	return entries, nil
}
func (s *Store) CompleteInbox(ctx context.Context, scope Scope, sequence int64) error {
	if err := scope.validate(); err != nil {
		return err
	}
	if sequence <= 0 {
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM inbox WHERE sequence=? AND account=? AND user=?`, sequence, scope.Account, scope.User)
	return changed(ctx, result, err)
}
