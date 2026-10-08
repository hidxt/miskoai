package storage

import "context"

const schemaVersion = 1

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
		if _, err = tx.ExecContext(ctx, schema); err != nil {
			return storageError(ctx, err)
		}
	}
	return storageError(ctx, tx.Commit())
}
