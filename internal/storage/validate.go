package storage

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type schemaObject struct{ Type, Table, DDL string }

// currentManifest pins both application DDL and the FTS5 shadow schema produced
// by the pinned driver. Future migrations must explicitly update this manifest.
func schema1Manifest() map[string]schemaObject {
	statement := func(start, end string) string {
		i := strings.Index(schema, start)
		if i < 0 {
			panic("missing pinned schema statement")
		}
		rest := schema[i:]
		j := strings.Index(rest, end)
		if j < 0 {
			panic("missing pinned schema terminator")
		}
		return strings.TrimSuffix(rest[:j+len(end)], ";")
	}
	return map[string]schemaObject{
		"facts":                       {"table", "facts", statement("CREATE TABLE facts (", ");")},
		"facts_scope":                 {"index", "facts", statement("CREATE INDEX facts_scope", ";")},
		"facts_fts":                   {"table", "facts_fts", statement("CREATE VIRTUAL TABLE facts_fts", ";")},
		"facts_ai":                    {"trigger", "facts", statement("CREATE TRIGGER facts_ai", "END;")},
		"facts_ad":                    {"trigger", "facts", statement("CREATE TRIGGER facts_ad", "END;")},
		"facts_au":                    {"trigger", "facts", statement("CREATE TRIGGER facts_au", "END;")},
		"messages":                    {"table", "messages", statement("CREATE TABLE messages (", ");")},
		"messages_history":            {"index", "messages", statement("CREATE INDEX messages_history", ";")},
		"sqlite_autoindex_messages_1": {"index", "messages", ""},
		"facts_fts_data":              {"table", "facts_fts_data", `CREATE TABLE 'facts_fts_data'(id INTEGER PRIMARY KEY, block BLOB)`},
		"facts_fts_idx":               {"table", "facts_fts_idx", `CREATE TABLE 'facts_fts_idx'(segid, term, pgno, PRIMARY KEY(segid, term)) WITHOUT ROWID`},
		"facts_fts_docsize":           {"table", "facts_fts_docsize", `CREATE TABLE 'facts_fts_docsize'(id INTEGER PRIMARY KEY, sz BLOB)`},
		"facts_fts_config":            {"table", "facts_fts_config", `CREATE TABLE 'facts_fts_config'(k PRIMARY KEY, v) WITHOUT ROWID`},
	}
}

func currentManifest() map[string]schemaObject {
	m := schema1Manifest()
	m["sqlite_sequence"] = schemaObject{"table", "sqlite_sequence", "CREATE TABLE sqlite_sequence(name,seq)"}
	statement := func(start, end string) string {
		i := strings.Index(schema2, start)
		rest := schema2[i:]
		j := strings.Index(rest, end)
		return strings.TrimSuffix(rest[:j+len(end)], ";")
	}
	m["poll_frames"] = schemaObject{"table", "poll_frames", statement("CREATE TABLE poll_frames", ");")}
	m["sqlite_autoindex_poll_frames_1"] = schemaObject{"index", "poll_frames", ""}
	m["channel_cursors"] = schemaObject{"table", "channel_cursors", statement("CREATE TABLE channel_cursors", ");")}
	m["sqlite_autoindex_channel_cursors_1"] = schemaObject{"index", "channel_cursors", ""}
	m["inbox"] = schemaObject{"table", "inbox", statement("CREATE TABLE inbox", ");")}
	m["sqlite_autoindex_inbox_1"] = schemaObject{"index", "inbox", ""}
	m["inbox_scope"] = schemaObject{"index", "inbox", statement("CREATE INDEX inbox_scope", ";")}
	return m
}

func normalizeDDL(v string) string { return strings.Join(strings.Fields(v), " ") }

// ValidateBackup reads a standalone snapshot without creating or migrating any
// schema, chmodding files, or opening a writable connection. WAL/SHM sidecars
// are refused: restore inputs must be completed SQLite snapshots, not live DBs.
// An exact version/DDL manifest prevents arbitrary SQLite databases or altered
// triggers from entering the restore flow. This must run before storage.Open.
func ValidateBackup(ctx context.Context, path string) error {
	if path == "" || strings.ContainsRune(path, 0) {
		return ErrInvalid
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ErrInvalid
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return ErrInvalid
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err = os.Lstat(abs + suffix); err == nil {
			return ErrInvalid
		} else if !errors.Is(err, os.ErrNotExist) {
			return ErrStorage
		}
	}
	uriPath := filepath.ToSlash(abs)
	if filepath.VolumeName(abs) != "" {
		uriPath = "/" + uriPath
	}
	dsn := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro&immutable=1"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return storageError(ctx, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.ExecContext(ctx, "PRAGMA query_only=ON; PRAGMA cache_size=-8192"); err != nil {
		return storageError(ctx, err)
	}
	if err = validateManifest(ctx, db); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return storageError(ctx, err)
	}
	checks := 0
	for rows.Next() {
		var result string
		if err = rows.Scan(&result); err != nil {
			rows.Close()
			return storageError(ctx, err)
		}
		if result != "ok" {
			rows.Close()
			return ErrStorage
		}
		checks++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return storageError(ctx, err)
	}
	if checks != 1 {
		return ErrStorage
	}
	return nil
}

func validateManifest(ctx context.Context, db rowQuery) error {
	var version int
	var err error
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return storageError(ctx, err)
	}
	if version != 1 && version != schemaVersion {
		return ErrInvalid
	}
	expected := currentManifest()
	if version == 1 {
		expected = schema1Manifest()
	}
	rows, err := db.QueryContext(ctx, "SELECT type,name,tbl_name,sql FROM sqlite_schema")
	if err != nil {
		return storageError(ctx, err)
	}
	for rows.Next() {
		var typ, name, table string
		var ddl sql.NullString
		if err = rows.Scan(&typ, &name, &table, &ddl); err != nil {
			rows.Close()
			return storageError(ctx, err)
		}
		want, ok := expected[name]
		if !ok || typ != want.Type || table != want.Table || normalizeDDL(ddl.String) != normalizeDDL(want.DDL) {
			rows.Close()
			return ErrInvalid
		}
		delete(expected, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return storageError(ctx, err)
	}
	if len(expected) != 0 {
		return ErrInvalid
	}
	return nil
}
