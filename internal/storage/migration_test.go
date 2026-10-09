package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchema1SnapshotMigratesAndForeignZeroFails(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, schema); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) VALUES('a','u','old synthetic fact','','manual',0,0,1,1); INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES('a','u','old','question','sending',1,1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateBackup(ctx, path); err != nil {
		t.Fatalf("schema1 snapshot refused: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("schema1 validation mutated snapshot: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 3 {
		t.Fatalf("migration: %d %v", version, err)
	}
	facts, err := s.SearchFacts(ctx, Scope{"a", "u"}, "synthetic", 10)
	if err != nil || len(facts) != 1 {
		t.Fatalf("old fact lost: %v %v", facts, err)
	}
	if ok, err := s.ClaimMessage(ctx, Scope{"a", "u"}, "old", "duplicate"); err != nil || ok {
		t.Fatalf("old sending claim lost: %v %v", ok, err)
	}
	if err = s.CompleteMessage(ctx, Scope{"a", "u"}, "old", "reserved old reply"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "new.db")
	if err = s.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err = ValidateBackup(ctx, dest); err != nil {
		t.Fatalf("schema3 snapshot: %v", err)
	}
	foreign := filepath.Join(dir, "foreign.db")
	db, err = sql.Open("sqlite", foreign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE unrelated(value TEXT); INSERT INTO unrelated VALUES('synthetic')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	before, err = os.ReadFile(foreign)
	if err != nil {
		t.Fatal(err)
	}
	if s, err = Open(foreign); !errors.Is(err, ErrInvalid) {
		if s != nil {
			s.Close()
		}
		t.Fatalf("foreign version0 accepted: %v", err)
	}
	after, err = os.ReadFile(foreign)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("foreign file mutated: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err = os.Stat(foreign + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("foreign sidecar: %v", err)
		}
	}
}

func TestInitializedSchemaIsCheckpointedBeforeReceiving(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		uriPath = "/" + uriPath
	}
	dsn := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro&immutable=1"}
	probe, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	var version int
	if err = probe.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 3 {
		t.Fatalf("fresh main DB not durable before polls: %d %v", version, err)
	}
	if _, err = s.RecordPoll(ctx, scope, "", []byte("durable WAL evidence")); err != nil {
		t.Fatal(err)
	}
	// Reopening while the original connection still retains its WAL exercises
	// the journal recovery path without relying on a graceful Close checkpoint.
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	f, err := reopened.PendingPoll(ctx, scope)
	if err != nil || f == nil || string(f.Body) != "durable WAL evidence" {
		t.Fatalf("WAL frame recovery: %v %v", f, err)
	}
}

func TestLegacyOverCapacityMigrationPreservesSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private", "old.db")
	if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2049) INSERT INTO messages(account,user,id,content,state,created_at,updated_at) SELECT 'a','u',CAST(x AS TEXT),?,'sending',1,1 FROM n`, strings.Repeat("x", 16384)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err = ValidateBackup(ctx, path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if s != nil {
		s.Close()
	}
	if !errors.Is(err, ErrCapacity) {
		t.Fatalf("legacy oversized reservations admitted: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("capacity rejection mutated DB: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err = os.Stat(path + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("capacity created sidecar: %v", err)
		}
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v int
	if err = db.QueryRow("PRAGMA user_version").Scan(&v); err != nil || v != 1 {
		t.Fatalf("failed migration changed schema: %d %v", v, err)
	}
}

func TestSchema2EffectiveWALAdmission(t *testing.T) {
	for _, mode := range []string{"altered", "over-cap"} {
		t.Run(mode, func(t *testing.T) {
			s, path := testSchema2Store(t)
			var err error
			if mode == "altered" {
				_, err = s.db.Exec(`CREATE TABLE foreign_in_wal(value TEXT)`)
			} else {
				_, err = s.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2049) INSERT INTO messages(account,user,id,content,state,created_at,updated_at) SELECT 'a','u',CAST(x AS TEXT),?,'sending',1,1 FROM n`, strings.Repeat("x", 16384))
			}
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(path)
			if reopened != nil {
				reopened.Close()
			}
			want := ErrInvalid
			if mode == "over-cap" {
				want = ErrCapacity
			}
			if !errors.Is(err, want) {
				t.Fatalf("effective WAL %s admitted: %v", mode, err)
			}
		})
	}
}

func TestSchema1WithJournalRequiresCompletedSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "old.db")
	if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES('a','u','x','synthetic','processing',1,1)`); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	walBefore, err := os.ReadFile(path + "-wal")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if s != nil {
		s.Close()
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("live schema1 migration accepted: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("schema1 main changed: %v", err)
	}
	walAfter, err := os.ReadFile(path + "-wal")
	if err != nil || !bytes.Equal(walBefore, walAfter) {
		t.Fatalf("schema1 WAL changed: %v", err)
	}
}

func TestAdmissionRejectsUnsafeRowsBeforeMutation(t *testing.T) {
	cases := []struct {
		name, query string
		args        []any
		want        error
		receive     bool
	}{
		{"frame-blob", `INSERT INTO poll_frames(account,user,cursor,body,state) VALUES('a','u','',?,'pending')`, []any{make([]byte, (2<<20)+1)}, ErrInvalid, true},
		{"frame-unicode-text", `INSERT INTO poll_frames(account,user,cursor,body,state) VALUES('a','u','',?,'pending')`, []any{strings.Repeat("界", 800000)}, ErrInvalid, true},
		{"inbox-text", `INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','x',?,'',1)`, []any{strings.Repeat("x", 16385)}, ErrInvalid, true},
		{"inbox-context", `INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','x','',?,1)`, []any{strings.Repeat("x", 16385)}, ErrInvalid, true},
		{"cursor", `INSERT INTO channel_cursors(account,user,cursor) VALUES('a','u',?)`, []any{strings.Repeat("x", 16385)}, ErrInvalid, true},
		{"frame-scope", `INSERT INTO poll_frames(account,user,cursor,body,state) VALUES(?,'u','',X'00','pending')`, []any{strings.Repeat("x", 257)}, ErrInvalid, true},
		{"inbox-id", `INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u',?,'','',1)`, []any{strings.Repeat("x", 513)}, ErrInvalid, true},
		{"inbox-invalid-utf8", `INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','x',CAST(? AS TEXT),'',1)`, []any{[]byte{0xff}}, ErrInvalid, true},
		{"inbox-nul", `INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','x',?,'',1)`, []any{"x\x00"}, ErrInvalid, true},
		{"inbox-blob-type", `INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','x',?,'',1)`, []any{[]byte("small blob")}, ErrInvalid, true},
		{"frame-cursor-nul", `INSERT INTO poll_frames(account,user,cursor,body,state) VALUES('a','u',?,X'01','pending')`, []any{"x\x00"}, ErrInvalid, true},
		{"cursor-scope-utf8", `INSERT INTO channel_cursors(account,user,cursor) VALUES('a',CAST(? AS TEXT),'')`, []any{[]byte{0xff}}, ErrInvalid, true},
		{"fact-scope-quota", `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<257) INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) SELECT 'a','u',?,'','manual',0,0,1,1 FROM n`, []any{strings.Repeat("x", 16384)}, ErrCapacity, false},
		{"fact-content", `INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) VALUES('a','u',?,'','manual',0,0,1,1)`, []any{strings.Repeat("x", 16385)}, ErrInvalid, false},
		{"fact-category", `INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) VALUES('a','u','x',?,'manual',0,0,1,1)`, []any{strings.Repeat("x", 257)}, ErrInvalid, false},
		{"fact-nul", `INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) VALUES('a','u',?,'','manual',0,0,1,1)`, []any{"x\x00"}, ErrInvalid, false},
		{"fact-blob-type", `INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) VALUES('a','u',?,'','manual',0,0,1,1)`, []any{[]byte("small blob")}, ErrInvalid, false},
		{"fact-utf8", `INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) VALUES('a','u',CAST(? AS TEXT),'','manual',0,0,1,1)`, []any{[]byte{0xff}}, ErrInvalid, false},
		{"message-content", `INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES('a','u','x',?,'failed',1,1)`, []any{strings.Repeat("x", 16385)}, ErrInvalid, false},
		{"message-reply", `INSERT INTO messages(account,user,id,content,reply,state,created_at,updated_at) VALUES('a','u','x','x',?,'sent',1,1)`, []any{strings.Repeat("x", 16385)}, ErrInvalid, false},
		{"message-scope", `INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES(?,'u','x','x','failed',1,1)`, []any{strings.Repeat("x", 257)}, ErrInvalid, false},
		{"message-id", `INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES('a','u',?,'x','failed',1,1)`, []any{strings.Repeat("x", 513)}, ErrInvalid, false},
		{"message-blob-type", `INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES('a','u','x',?,'failed',1,1)`, []any{[]byte("small blob")}, ErrInvalid, false},
		{"message-nul", `INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES('a','u','x',?,'failed',1,1)`, []any{"x\x00"}, ErrInvalid, false},
	}
	for _, tc := range cases {
		versions := []string{"schema2-snapshot", "schema2-wal"}
		if !tc.receive {
			versions = append(versions, "schema1-snapshot")
		}
		for _, mode := range versions {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "private", "fixture.db")
				if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				var db *sql.DB
				if mode == "schema2-wal" {
					var err error
					db, err = sql.Open("sqlite", path)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = db.Exec(schema + schema2 + "PRAGMA journal_mode=WAL; PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { db.Close() })
				} else {
					var err error
					db, err = sql.Open("sqlite", path)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = db.Exec(schema); err != nil {
						t.Fatal(err)
					}
					if mode == "schema2-snapshot" {
						if _, err = db.Exec(schema2); err != nil {
							t.Fatal(err)
						}
					}
				}
				if _, err := db.Exec(tc.query, tc.args...); err != nil {
					t.Fatal(err)
				}
				if mode != "schema2-wal" {
					if err := db.Close(); err != nil {
						t.Fatal(err)
					}
					if err := ValidateBackup(context.Background(), path); err != nil {
						t.Fatalf("exact snapshot validation: %v", err)
					}
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var walBefore []byte
				if mode == "schema2-wal" {
					walBefore, err = os.ReadFile(path + "-wal")
					if err != nil {
						t.Fatal(err)
					}
				}
				reopened, err := Open(path)
				if reopened != nil {
					reopened.Close()
				}
				if !errors.Is(err, tc.want) {
					t.Fatalf("unsafe row admitted, want %v got %v", tc.want, err)
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("main mutated on refusal: %v", err)
				}
				if mode == "schema2-wal" {
					walAfter, err := os.ReadFile(path + "-wal")
					if err != nil || !bytes.Equal(walBefore, walAfter) {
						t.Fatalf("WAL evidence mutated: %v", err)
					}
				} else {
					for _, suffix := range []string{"-wal", "-shm", "-journal"} {
						if _, err = os.Stat(path + suffix); !errors.Is(err, os.ErrNotExist) {
							t.Fatalf("refusal created sidecar %s: %v", suffix, err)
						}
					}
				}
			})
		}
	}
}
