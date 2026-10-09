package storage

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// FactPage is an ordinary offset page. Mutations may shift subsequent requests;
// HasMore and NextOffset describe only this request's captured view.
type FactPage struct {
	Items      []Fact
	NextOffset int
	HasMore    bool
}
type HistoryEntry struct {
	ID, Content, Reply string
	CreatedAt          time.Time
}
type HistoryPage struct {
	Items      []HistoryEntry
	NextOffset int
	HasMore    bool
}

func validPage(sc Scope, query string, offset, limit, maxLimit int) bool {
	return sc.validate() == nil && validFactQuery(query) && offset >= 0 && offset <= 10000 && limit >= 1 && limit <= maxLimit
}

// FactsPage applies the conversational search policy to all retained facts,
// including expired records that an administrator may correct or delete.
func (s *Store) FactsPage(ctx context.Context, sc Scope, query string, offset, limit int) (FactPage, error) {
	var p FactPage
	if !validPage(sc, query, offset, limit, 16) {
		return p, ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return p, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateLegacyShape(ctx, tx); e != nil {
		return p, e
	}
	predicate, args := factPredicate(sc, query, true)
	// Select only identifiers while sorting/filtering retained rows. A large
	// tied sort must not retain their payloads before applying the page limit.
	ids := "SELECT id FROM facts" + predicate + factOrder + " LIMIT ? OFFSET ?"
	q := "SELECT " + factColumns + " FROM facts WHERE account=? AND user=? AND id IN (" + ids + ")" + factOrder
	args = append([]any{sc.Account, sc.User}, args...)
	args = append(args, limit+1, offset)
	p.Items, e = readFacts(ctx, tx, q, args...)
	if e != nil {
		return FactPage{}, e
	}
	if len(p.Items) > limit {
		p.Items = p.Items[:limit]
		p.HasMore = true
		p.NextOffset = offset + limit
	}
	if e = tx.Commit(); e != nil {
		return FactPage{}, storageError(ctx, e)
	}
	return p, nil
}

// HistoryPage returns completed, nonempty user/reply pairs in newest-first
// order. Search is a literal trimmed substring of either side of the pair.
func (s *Store) HistoryPage(ctx context.Context, sc Scope, query string, offset, limit int) (HistoryPage, error) {
	p := HistoryPage{Items: []HistoryEntry{}}
	if !validPage(sc, query, offset, limit, 8) {
		return HistoryPage{}, ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return HistoryPage{}, storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateLegacyShape(ctx, tx); e != nil {
		return HistoryPage{}, e
	}
	q := `SELECT id FROM messages WHERE account=? AND user=? AND state='sent' AND content<>'' AND reply<>''`
	args := []any{sc.Account, sc.User}
	if strings.TrimSpace(query) != "" {
		q += ` AND (content LIKE ? ESCAPE '\' OR reply LIKE ? ESCAPE '\')`
		pattern := likePattern(strings.TrimSpace(query))
		args = append(args, pattern, pattern)
	}
	q += ` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`
	args = append(args, limit+1, offset)
	q = `SELECT id,content,reply,created_at FROM messages WHERE account=? AND user=? AND id IN (` + q + `) ORDER BY created_at DESC,id DESC`
	args = append([]any{sc.Account, sc.User}, args...)
	rows, e := tx.QueryContext(ctx, q, args...)
	if e != nil {
		return HistoryPage{}, storageError(ctx, e)
	}
	defer rows.Close()
	for rows.Next() {
		var entry HistoryEntry
		var stamp int64
		if e = rows.Scan(&entry.ID, &entry.Content, &entry.Reply, &stamp); e != nil {
			return HistoryPage{}, storageError(ctx, e)
		}
		if !validMessageID(entry.ID) || !validField(entry.Content, 16384) || !validField(entry.Reply, 16384) {
			return HistoryPage{}, ErrInvalid
		}
		entry.CreatedAt = time.UnixMilli(stamp).UTC()
		p.Items = append(p.Items, entry)
	}
	if e = closeValidatedRows(ctx, rows); e != nil {
		return HistoryPage{}, e
	}
	if len(p.Items) > limit {
		p.Items = p.Items[:limit]
		p.HasMore = true
		p.NextOffset = offset + limit
	}
	if e = tx.Commit(); e != nil {
		return HistoryPage{}, storageError(ctx, e)
	}
	return p, nil
}

const maxMemoryJSONBytes = 64 << 20

// Explicit export DTOs keep public field names stable independently of storage
// structs. Raw messages, channel metadata and receive evidence are excluded.
type memoryJSONFact struct {
	ID         int64      `json:"id"`
	Content    string     `json:"content"`
	Category   string     `json:"category"`
	Source     string     `json:"source"`
	Confidence float64    `json:"confidence"`
	Importance int        `json:"importance"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}
type memoryJSONSummary struct {
	Text      string    `json:"text"`
	Watermark int64     `json:"watermark"`
	Revision  int64     `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}
type memoryJSONCandidate struct {
	ID        int64     `json:"id"`
	Content   string    `json:"content"`
	MessageID string    `json:"message_id"`
	Quote     string    `json:"quote"`
	CreatedAt time.Time `json:"created_at"`
}

// memoryJSONWriter writes at most one encoded row at a time and rejects a
// short write. Writer errors are classified; arbitrary private error text never
// crosses the storage boundary. A writer must itself honor cancellation while
// blocked, since this synchronous API never creates a detached producer.
type memoryJSONWriter struct {
	ctx     context.Context
	w       io.Writer
	written int
}

func (w *memoryJSONWriter) write(p []byte) error {
	if e := w.ctx.Err(); e != nil {
		return e
	}
	if len(p) > maxMemoryJSONBytes-w.written {
		return ErrCapacity
	}
	n, e := w.w.Write(p)
	if e != nil || n != len(p) {
		return storageError(w.ctx, io.ErrShortWrite)
	}
	w.written += n
	return w.ctx.Err()
}
func (w *memoryJSONWriter) value(v any) error {
	if e := w.ctx.Err(); e != nil {
		return e
	}
	p, e := json.Marshal(v)
	if e != nil {
		return ErrInvalid
	}
	return w.write(p)
}

// validateMemoryJSONCapacity checks payload quotas without decoding retained
// messages or materializing a fact list. Shape checks precede row decoding.
func validateMemoryJSONCapacity(ctx context.Context, q rowQuery) error {
	if e := validateLegacyShape(ctx, q); e != nil {
		return e
	}
	if e := capacity(ctx, q, `SELECT count(*),coalesce(sum(length(CAST(content AS BLOB))),0) FROM facts`, 10000, 8<<20); e != nil {
		return e
	}
	if e := scopeQuota(ctx, q, `SELECT EXISTS(SELECT 1 FROM facts GROUP BY account,user HAVING count(*)>10000 OR sum(length(CAST(content AS BLOB)))>4194304)`); e != nil {
		return e
	}
	if e := capacity(ctx, q, `SELECT count(*),coalesce(sum(length(CAST(content AS BLOB))+length(CAST(reply AS BLOB))+CASE WHEN state IN ('processing','sending','ambiguous') THEN 16384 ELSE 0 END),0) FROM messages`, 10000, 64<<20); e != nil {
		return e
	}
	return validateDerivedCapacity(ctx, q)
}

// WriteMemoryJSON streams a complete version-1 object from one captured read
// transaction. Only nil return establishes a completed artifact; callers must
// discard partial output after any error. Historical chats use HistoryPage.
func (s *Store) WriteMemoryJSON(ctx context.Context, sc Scope, writer io.Writer) error {
	if sc.validate() != nil || writer == nil {
		return ErrInvalid
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return storageError(ctx, e)
	}
	defer tx.Rollback()
	if e = validateMemoryJSONCapacity(ctx, tx); e != nil {
		return e
	}
	summary, e := readDerived(ctx, tx, sc)
	if e != nil {
		return e
	}
	w := memoryJSONWriter{ctx: ctx, w: writer}
	if e = w.write([]byte(`{"version":1,"facts":[`)); e != nil {
		return e
	}
	rows, e := tx.QueryContext(ctx, `SELECT `+factColumns+` FROM facts WHERE account=? AND user=? ORDER BY id`, sc.Account, sc.User)
	if e != nil {
		return storageError(ctx, e)
	}
	defer rows.Close()
	first := true
	for rows.Next() {
		if e = ctx.Err(); e != nil {
			return e
		}
		f, e := readFact(ctx, rows)
		if e != nil {
			return e
		}
		if !first {
			if e = w.write([]byte(",")); e != nil {
				return e
			}
		}
		first = false
		if e = w.value(memoryJSONFact{f.ID, f.Content, f.Category, f.Source, f.Confidence, f.Importance, f.CreatedAt, f.UpdatedAt, f.ExpiresAt}); e != nil {
			return e
		}
	}
	if e = closeValidatedRows(ctx, rows); e != nil {
		return e
	}
	if e = w.write([]byte(`],"summary":`)); e != nil {
		return e
	}
	if e = w.value(memoryJSONSummary{summary.Text, summary.Watermark, summary.Revision, summary.UpdatedAt}); e != nil {
		return e
	}
	if e = w.write([]byte(`,"candidates":[`)); e != nil {
		return e
	}
	rows, e = tx.QueryContext(ctx, `SELECT id,content,message_id,quote,created_at FROM candidates WHERE account=? AND user=? ORDER BY id`, sc.Account, sc.User)
	if e != nil {
		return storageError(ctx, e)
	}
	defer rows.Close()
	first = true
	for rows.Next() {
		if e = ctx.Err(); e != nil {
			return e
		}
		var c memoryJSONCandidate
		var stamp int64
		if e = rows.Scan(&c.ID, &c.Content, &c.MessageID, &c.Quote, &stamp); e != nil {
			return storageError(ctx, e)
		}
		c.CreatedAt = time.UnixMilli(stamp).UTC()
		if !first {
			if e = w.write([]byte(",")); e != nil {
				return e
			}
		}
		first = false
		if e = w.value(c); e != nil {
			return e
		}
	}
	if e = closeValidatedRows(ctx, rows); e != nil {
		return e
	}
	if e = w.write([]byte(`]}`)); e != nil {
		return e
	}
	return storageError(ctx, tx.Commit())
}
