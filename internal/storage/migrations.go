package storage

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
)

const schemaVersion = 4

const schema = `
CREATE TABLE facts (
 id INTEGER PRIMARY KEY, account TEXT NOT NULL, user TEXT NOT NULL,
 content TEXT NOT NULL, category TEXT NOT NULL,
 source TEXT NOT NULL CHECK(source IN ('explicit_user','manual')),
 confidence REAL NOT NULL CHECK(confidence >= 0 AND confidence <= 1),
 importance INTEGER NOT NULL CHECK(importance >= 0 AND importance <= 100),
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, expires_at INTEGER
);
CREATE INDEX facts_scope ON facts(account,user,importance DESC,updated_at DESC);
CREATE VIRTUAL TABLE facts_fts USING fts5(content, content='facts', content_rowid='id');
CREATE TRIGGER facts_ai AFTER INSERT ON facts BEGIN
 INSERT INTO facts_fts(rowid,content) VALUES(new.id,new.content);
END;
CREATE TRIGGER facts_ad AFTER DELETE ON facts BEGIN
 INSERT INTO facts_fts(facts_fts,rowid,content) VALUES('delete',old.id,old.content);
END;
CREATE TRIGGER facts_au AFTER UPDATE ON facts BEGIN
 INSERT INTO facts_fts(facts_fts,rowid,content) VALUES('delete',old.id,old.content);
 INSERT INTO facts_fts(rowid,content) VALUES(new.id,new.content);
END;
CREATE TABLE messages (
 account TEXT NOT NULL, user TEXT NOT NULL, id TEXT NOT NULL, content TEXT NOT NULL,
 reply TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL CHECK(state IN ('processing','sending','sent','failed','ambiguous')),
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 PRIMARY KEY(account,user,id)
);
CREATE INDEX messages_history ON messages(account,user,created_at DESC);
PRAGMA user_version=1;
`

const schema2 = `
CREATE TABLE poll_frames (
 id INTEGER PRIMARY KEY AUTOINCREMENT, account TEXT NOT NULL UNIQUE, user TEXT NOT NULL,
 cursor TEXT NOT NULL, body BLOB NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('pending','quarantined'))
);
CREATE TABLE channel_cursors (
 account TEXT PRIMARY KEY NOT NULL, user TEXT NOT NULL, cursor TEXT NOT NULL
);
CREATE TABLE inbox (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, account TEXT NOT NULL, user TEXT NOT NULL,
 message_id TEXT NOT NULL, text TEXT NOT NULL, context_token TEXT NOT NULL,
 received_at INTEGER NOT NULL, UNIQUE(account,user,message_id)
);
CREATE INDEX inbox_scope ON inbox(account,user,sequence);
PRAGMA user_version=2;
`

const schema3 = `
CREATE TABLE derived (
 account TEXT NOT NULL, user TEXT NOT NULL, text TEXT NOT NULL,
 watermark INTEGER NOT NULL, revision INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 PRIMARY KEY(account,user)
);
CREATE TABLE candidates (
 id INTEGER PRIMARY KEY AUTOINCREMENT, account TEXT NOT NULL, user TEXT NOT NULL,
 content TEXT NOT NULL, message_id TEXT NOT NULL, quote TEXT NOT NULL, created_at INTEGER NOT NULL,
 UNIQUE(account,user,message_id,quote,content)
);
CREATE TABLE profiles (
 account TEXT NOT NULL, user TEXT NOT NULL, id TEXT NOT NULL, body TEXT NOT NULL,
 PRIMARY KEY(account,user,id)
);
CREATE TABLE profile_selections (
 account TEXT NOT NULL, user TEXT NOT NULL, id TEXT NOT NULL,
 PRIMARY KEY(account,user)
);
PRAGMA user_version=3;
`

const schema4 = `
CREATE TABLE inbox_attachments (
 sequence INTEGER PRIMARY KEY REFERENCES inbox(sequence) ON DELETE CASCADE, body BLOB NOT NULL
);
PRAGMA user_version=4;
`

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storageError(ctx, err)
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return storageError(ctx, err)
	}
	if version > schemaVersion {
		return ErrInvalid
	}
	if version == 0 {
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema").Scan(&count); err != nil {
			return storageError(ctx, err)
		}
		if count != 0 {
			return ErrInvalid
		}
		if _, err = tx.ExecContext(ctx, schema); err != nil {
			return storageError(ctx, err)
		}
		version = 1
	}
	if version == 1 {
		// Old data remains valid as a snapshot, but opening it must not promise
		// reply space that does not exist. Reject without changing its schema.
		if err = validateLegacyCapacity(ctx, tx); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, schema2); err != nil {
			return storageError(ctx, err)
		}
		version = 2
	}
	if version == 2 {
		if _, err = tx.ExecContext(ctx, schema3); err != nil {
			return storageError(ctx, err)
		}
		version = 3
	}
	if version == 3 {
		if _, err = tx.ExecContext(ctx, schema4); err != nil {
			return storageError(ctx, err)
		}
	}
	return storageError(ctx, tx.Commit())
}

func validateLegacyCapacity(ctx context.Context, q rowQuery) error {
	var count int
	var bytes int64
	if err := q.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(length(CAST(content AS BLOB))),0) FROM facts`).Scan(&count, &bytes); err != nil {
		return storageError(ctx, err)
	}
	if count > 10000 || bytes > 8<<20 {
		return ErrCapacity
	}
	var overScope int
	if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM facts GROUP BY account,user HAVING count(*)>10000 OR sum(length(CAST(content AS BLOB)))>4194304)`).Scan(&overScope); err != nil {
		return storageError(ctx, err)
	}
	if overScope != 0 {
		return ErrCapacity
	}
	if err := q.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(length(CAST(content AS BLOB))+length(CAST(reply AS BLOB))+CASE WHEN state IN ('processing','sending','ambiguous') THEN 16384 ELSE 0 END),0) FROM messages`).Scan(&count, &bytes); err != nil {
		return storageError(ctx, err)
	}
	if count > 10000 || bytes > 64<<20 {
		return ErrCapacity
	}
	return validateLegacyRows(ctx, q)
}

func validateReceiveCapacity(ctx context.Context, q rowQuery) error {
	var count int
	var bytes int64
	if err := q.QueryRowContext(ctx, `SELECT count(*),coalesce(sum(length(CAST(body AS BLOB))),0) FROM poll_frames`).Scan(&count, &bytes); err != nil {
		return storageError(ctx, err)
	}
	if count > 4 || bytes > 8<<20 {
		return ErrCapacity
	}
	return validateReceiveRows(ctx, q)
}

type boundedColumn struct {
	name string
	max  int
	kind string
}

// SQL checks types and byte lengths before Go loads any row. Descriptors and
// table names are constants below; no untrusted identifier enters SQL.
func validateRowShape(ctx context.Context, q rowQuery, table string, columns []boundedColumn, extra string) error {
	predicates := []string{}
	for _, c := range columns {
		predicates = append(predicates, "typeof("+c.name+") <> '"+c.kind+"' OR length(CAST("+c.name+" AS BLOB)) > "+strconv.Itoa(c.max))
	}
	if extra != "" {
		predicates = append(predicates, extra)
	}
	var bad int
	if err := q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE "+strings.Join(predicates, " OR ")+")").Scan(&bad); err != nil {
		return storageError(ctx, err)
	}
	if bad != 0 {
		return ErrInvalid
	}
	return nil
}

func validateLegacyShape(ctx context.Context, q rowQuery) error {
	scope := []boundedColumn{{"account", 256, "text"}, {"user", 256, "text"}}
	facts := append(append([]boundedColumn{}, scope...), boundedColumn{"content", 16384, "text"}, boundedColumn{"category", 256, "text"}, boundedColumn{"source", 16, "text"})
	if err := validateRowShape(ctx, q, "facts", facts, `typeof(id)<>'integer' OR id<=0 OR typeof(importance)<>'integer' OR typeof(confidence) NOT IN ('real','integer') OR typeof(created_at)<>'integer' OR typeof(updated_at)<>'integer' OR (expires_at IS NOT NULL AND typeof(expires_at)<>'integer')`); err != nil {
		return err
	}
	messages := append(append([]boundedColumn{}, scope...), boundedColumn{"id", 512, "text"}, boundedColumn{"content", 16384, "text"}, boundedColumn{"reply", 16384, "text"}, boundedColumn{"state", 16, "text"})
	if err := validateRowShape(ctx, q, "messages", messages, `typeof(created_at)<>'integer' OR typeof(updated_at)<>'integer'`); err != nil {
		return err
	}
	return nil
}

func validateLegacyRows(ctx context.Context, q rowQuery) error {
	if err := validateLegacyShape(ctx, q); err != nil {
		return err
	}
	rows, err := q.QueryContext(ctx, `SELECT account,user,id,content,category,source,confidence,importance FROM facts`)
	if err != nil {
		return storageError(ctx, err)
	}
	for rows.Next() {
		var sc Scope
		var f Fact
		if err = rows.Scan(&sc.Account, &sc.User, &f.ID, &f.Content, &f.Category, &f.Source, &f.Confidence, &f.Importance); err != nil {
			rows.Close()
			return storageError(ctx, err)
		}
		if sc.validate() != nil || f.validate() != nil {
			rows.Close()
			return ErrInvalid
		}
	}
	if err = closeValidatedRows(ctx, rows); err != nil {
		return err
	}
	rows, err = q.QueryContext(ctx, `SELECT account,user,id,content,reply,state FROM messages`)
	if err != nil {
		return storageError(ctx, err)
	}
	for rows.Next() {
		var sc Scope
		var id, content, reply, state string
		if err = rows.Scan(&sc.Account, &sc.User, &id, &content, &reply, &state); err != nil {
			rows.Close()
			return storageError(ctx, err)
		}
		if sc.validate() != nil || !validMessageID(id) || !validField(content, 16384) || !validField(reply, 16384) || !validState(state) {
			rows.Close()
			return ErrInvalid
		}
	}
	return closeValidatedRows(ctx, rows)
}

func validState(v string) bool {
	switch v {
	case "processing", "sending", "sent", "failed", "ambiguous":
		return true
	}
	return false
}
func closeValidatedRows(ctx context.Context, rows *sql.Rows) error {
	err := rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return storageError(ctx, err)
	}
	return storageError(ctx, closeErr)
}

func validateReceiveRows(ctx context.Context, q rowQuery) error {
	if _, _, err := receiveQueueBudget(ctx, q); err != nil {
		return err
	}
	if err := validateAttachments(ctx, q); err != nil {
		return err
	}
	scope := []boundedColumn{{"account", 256, "text"}, {"user", 256, "text"}}
	frames := append(append([]boundedColumn{}, scope...), boundedColumn{"cursor", 16384, "text"}, boundedColumn{"state", 16, "text"}, boundedColumn{"body", 2 << 20, "blob"})
	if err := validateRowShape(ctx, q, "poll_frames", frames, `typeof(id)<>'integer' OR id<=0`); err != nil {
		return err
	}
	cursors := append(append([]boundedColumn{}, scope...), boundedColumn{"cursor", 16384, "text"})
	if err := validateRowShape(ctx, q, "channel_cursors", cursors, ""); err != nil {
		return err
	}
	inbox := append(append([]boundedColumn{}, scope...), boundedColumn{"message_id", 512, "text"}, boundedColumn{"text", 16384, "text"}, boundedColumn{"context_token", 16384, "text"})
	if err := validateRowShape(ctx, q, "inbox", inbox, `typeof(sequence)<>'integer' OR sequence<=0 OR typeof(received_at)<>'integer' OR received_at<=0`); err != nil {
		return err
	}
	// Raw frame bodies are deliberately excluded: opacity is preserved and no
	// potentially large raw allocation is needed to validate receive admission.
	rows, err := q.QueryContext(ctx, `SELECT account,user,cursor,state FROM poll_frames`)
	if err != nil {
		return storageError(ctx, err)
	}
	for rows.Next() {
		var sc Scope
		var cursor, state string
		if err = rows.Scan(&sc.Account, &sc.User, &cursor, &state); err != nil {
			rows.Close()
			return storageError(ctx, err)
		}
		if sc.validate() != nil || !validField(cursor, 16384) || (state != "pending" && state != "quarantined") {
			rows.Close()
			return ErrInvalid
		}
	}
	if err = closeValidatedRows(ctx, rows); err != nil {
		return err
	}
	rows, err = q.QueryContext(ctx, `SELECT account,user,cursor FROM channel_cursors`)
	if err != nil {
		return storageError(ctx, err)
	}
	for rows.Next() {
		var sc Scope
		var cursor string
		if err = rows.Scan(&sc.Account, &sc.User, &cursor); err != nil {
			rows.Close()
			return storageError(ctx, err)
		}
		if sc.validate() != nil || !validField(cursor, 16384) {
			rows.Close()
			return ErrInvalid
		}
	}
	if err = closeValidatedRows(ctx, rows); err != nil {
		return err
	}
	rows, err = q.QueryContext(ctx, `SELECT account,user,message_id,text,context_token FROM inbox`)
	if err != nil {
		return storageError(ctx, err)
	}
	for rows.Next() {
		var sc Scope
		var id, text, token string
		if err = rows.Scan(&sc.Account, &sc.User, &id, &text, &token); err != nil {
			rows.Close()
			return storageError(ctx, err)
		}
		if sc.validate() != nil || !validMessageID(id) || !validField(text, 16384) || !validField(token, 16384) {
			rows.Close()
			return ErrInvalid
		}
	}
	return closeValidatedRows(ctx, rows)
}
