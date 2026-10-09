package storage

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

type Turn struct {
	ID, Content, State, Role string
	CreatedAt, UpdatedAt     time.Time
}

func validMessageID(id string) bool {
	return strings.TrimSpace(id) != "" && len(id) <= 512 && utf8.ValidString(id) && !strings.ContainsRune(id, 0)
}

// ClaimMessage persists processing before any remote side effect. All existing
// states remain claimed across restarts, including failed/ambiguous messages.
func (s *Store) ClaimMessage(ctx context.Context, scope Scope, id, text string) (bool, error) {
	if err := scope.validate(); err != nil {
		return false, err
	}
	if !validMessageID(id) || len(text) > 16384 || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return false, ErrInvalid
	}
	now := time.Now().UTC().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, storageError(ctx, err)
	}
	defer tx.Rollback()
	var exists, count int
	var bytes int64
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE account=? AND user=? AND id=?)`, scope.Account, scope.User, id).Scan(&exists); err != nil {
		return false, storageError(ctx, err)
	}
	if exists == 1 {
		return false, nil
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(length(CAST(content AS BLOB))+length(CAST(reply AS BLOB))+CASE WHEN state IN ('processing','sending','ambiguous') THEN 16384 ELSE 0 END),0) FROM messages`).Scan(&count, &bytes); err != nil {
		return false, storageError(ctx, err)
	}
	if count >= 10000 || bytes+int64(len(text))+16384 > 64<<20 {
		return false, ErrCapacity
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES(?,?,?,?,'processing',?,?) ON CONFLICT(account,user,id) DO NOTHING`, scope.Account, scope.User, id, text, now, now)
	if err != nil {
		return false, storageError(ctx, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, storageError(ctx, err)
	}
	if err = tx.Commit(); err != nil {
		return false, storageError(ctx, err)
	}
	return n == 1, nil
}

func (s *Store) SetMessageState(ctx context.Context, scope Scope, id, state string) error {
	if err := scope.validate(); err != nil {
		return err
	}
	if !validMessageID(id) {
		return ErrInvalid
	}
	var prior string
	switch state {
	case "sending", "failed":
		prior = "processing"
	case "sent", "ambiguous":
		prior = "sending"
	default:
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE messages SET state=?,updated_at=? WHERE account=? AND user=? AND id=? AND state=?`, state, time.Now().UTC().UnixMilli(), scope.Account, scope.User, id, prior)
	return changed(ctx, result, err)
}

// CompleteMessage atomically persists the delivered reply and marks its message
// sent. An uncertain send must instead become ambiguous, never call this method.
func (s *Store) CompleteMessage(ctx context.Context, scope Scope, id, reply string) error {
	if err := scope.validate(); err != nil {
		return err
	}
	if !validMessageID(id) || strings.TrimSpace(reply) == "" || len(reply) > 16384 || !utf8.ValidString(reply) || strings.ContainsRune(reply, 0) {
		return ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE messages SET reply=?,state='sent',updated_at=? WHERE account=? AND user=? AND id=? AND state='sending'`, reply, time.Now().UTC().UnixMilli(), scope.Account, scope.User, id)
	return changed(ctx, result, err)
}

// History returns completed user/reply pairs in chronological order. Limit is
// the number of completed messages (each contributes two turns), at most 1000.
func (s *Store) History(ctx context.Context, scope Scope, limit int) ([]Turn, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 1000 {
		return nil, ErrInvalid
	}
	return history(ctx, s.db, scope, limit)
}

func history(ctx context.Context, q rowQuery, scope Scope, limit int) ([]Turn, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,content,reply,state,created_at,updated_at FROM (SELECT id,content,reply,state,created_at,updated_at FROM messages WHERE account=? AND user=? AND state='sent' AND content<>'' AND reply<>'' ORDER BY created_at DESC,id DESC LIMIT ?) ORDER BY created_at,id`, scope.Account, scope.User, limit)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer rows.Close()
	turns := []Turn{}
	for rows.Next() {
		var t Turn
		var reply string
		var created, updated int64
		if err = rows.Scan(&t.ID, &t.Content, &reply, &t.State, &created, &updated); err != nil {
			return nil, storageError(ctx, err)
		}
		if !validMessageID(t.ID) || !validField(t.Content, 16384) || !validField(reply, 16384) || t.State != "sent" {
			return nil, ErrInvalid
		}
		t.CreatedAt = time.UnixMilli(created).UTC()
		t.UpdatedAt = time.UnixMilli(updated).UTC()
		t.Role = "user"
		turns = append(turns, t)
		t.Role = "assistant"
		t.Content = reply
		t.CreatedAt = t.UpdatedAt
		turns = append(turns, t)
	}
	return turns, storageError(ctx, rows.Err())
}
