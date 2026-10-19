package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func snapshotUnchanged(t *testing.T, path string) func() {
	t.Helper()
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	entries, e := os.ReadDir(filepath.Dir(path))
	if e != nil {
		t.Fatal(e)
	}
	names := func(es []os.DirEntry) []string {
		var n []string
		for _, x := range es {
			n = append(n, x.Name())
		}
		return n
	}
	return func() {
		t.Helper()
		after, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(before, after) {
			t.Fatal("snapshot bytes changed", e)
		}
		es, e := os.ReadDir(filepath.Dir(path))
		if e != nil || !reflect.DeepEqual(names(entries), names(es)) {
			t.Fatal("directory changed", e)
		}
	}
}

func TestLibraryBackupCurrentComplete(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	for n := 1; n <= 128; n++ {
		sc := Scope{"account", fmt.Sprint(n % 3)}
		a := libraryAdd(t, s, sc, n)
		if n%2 == 0 {
			if e := s.UpdateLibraryAsset(ctx, sc, a.ID, LibraryMetadata{Tags: []string{"one", "二"}, Emotion: "happy", Description: "all fields", Enabled: true}); e != nil {
				t.Fatal(e)
			}
			if e := s.RecordLibraryUse(ctx, sc, a.ID, 12345); e != nil {
				t.Fatal(e)
			}
		}
		if n%5 == 0 {
			if e := s.MarkLibraryDeleting(ctx, sc, a.ID); e != nil {
				t.Fatal(e)
			}
		}
	}
	want, e := s.LibrarySnapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(filepath.Dir(path), "consistent.db")
	if e = s.Backup(ctx, backup); e != nil {
		t.Fatal(e)
	}
	check := snapshotUnchanged(t, backup)
	got, e := LibraryBackupSnapshot(ctx, backup)
	check()
	if e != nil || !reflect.DeepEqual(want, got) {
		t.Fatalf("complete metadata mismatch: count=%d err=%v", len(got), e)
	}
}

func TestLibraryBackupLegacyImmutable(t *testing.T) {
	for version := 1; version <= 4; version++ {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private", "legacy.db")
			if _, e := privatePath(path); e != nil {
				t.Fatal(e)
			}
			db, e := sql.Open("sqlite", path)
			if e != nil {
				t.Fatal(e)
			}
			ddl := schema
			if version >= 2 {
				ddl += schema2
			}
			if version >= 3 {
				ddl += schema3
			}
			if version >= 4 {
				ddl += schema4
			}
			if _, e = db.Exec(ddl); e != nil {
				t.Fatal(e)
			}
			if e = db.Close(); e != nil {
				t.Fatal(e)
			}
			check := snapshotUnchanged(t, path)
			got, e := LibraryBackupSnapshot(context.Background(), path)
			check()
			if e != nil || got == nil || len(got) != 0 {
				t.Fatal("legacy not empty", e)
			}
			db, e = sql.Open("sqlite", path)
			if e != nil {
				t.Fatal(e)
			}
			var actual int
			if e = db.QueryRow("PRAGMA user_version").Scan(&actual); e != nil {
				t.Fatal(e)
			}
			if e = db.Close(); e != nil {
				t.Fatal(e)
			}
			if actual != version {
				t.Fatal("schema changed", actual)
			}
		})
	}
}

func TestLibraryBackupRefusesHostileSnapshots(t *testing.T) {
	cases := []string{
		`UPDATE library_assets SET description=CAST(zeroblob(1048576) AS TEXT)`,
		`UPDATE library_assets SET tags='[  ]'`,
		`UPDATE library_assets SET description=CAST(X'FF' AS TEXT)`,
		`UPDATE library_assets SET uses=1.5`,
		`UPDATE library_assets SET bytes=4194305`,
		`UPDATE library_assets SET state='deleting',enabled=1`,
		`PRAGMA user_version=7`, `CREATE TABLE raw_path_canary(x)`,
		`CREATE TRIGGER raw_sql_canary AFTER INSERT ON library_assets BEGIN SELECT 1; END`,
		`WITH RECURSIVE n(x) AS (VALUES(2) UNION ALL SELECT x+1 FROM n WHERE x<129) INSERT INTO library_assets(account,user,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used) SELECT 'a',cast(x AS TEXT),printf('%064x',x),'png',100,2,3,1,'[]','','','ready',0,0,0 FROM n`,
		`WITH RECURSIVE n(x) AS (VALUES(2) UNION ALL SELECT x+1 FROM n WHERE x<18) INSERT INTO library_assets(account,user,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used) SELECT 'a',cast(x AS TEXT),printf('%064x',x),'png',4194304,2,3,1,'[]','','','ready',0,0,0 FROM n`,
	}
	for i, query := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s, path := testStore(t)
			libraryAdd(t, s, Scope{"a", "u"}, 1)
			if _, e := s.db.Exec("PRAGMA ignore_check_constraints=ON;" + query); e != nil {
				t.Fatal(e)
			}
			if e := s.Close(); e != nil {
				t.Fatal(e)
			}
			check := snapshotUnchanged(t, path)
			got, e := LibraryBackupSnapshot(context.Background(), path)
			check()
			if e == nil || got != nil {
				t.Fatal("unsafe snapshot admitted")
			}
			if strings.Contains(e.Error(), "canary") || strings.Contains(e.Error(), path) {
				t.Fatal("unsafe error", e)
			}
		})
	}
}

func TestLibraryBackupContextsAndSidecars(t *testing.T) {
	s, path := testStore(t)
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []context.Context{nil, ctx} {
		got, e := LibraryBackupSnapshot(c, path)
		if got != nil || e == nil {
			t.Fatal("context admitted")
		}
		if c != nil && !errors.Is(e, context.Canceled) {
			t.Fatal("lost cancellation", e)
		}
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		t.Run(suffix, func(t *testing.T) {
			if e := os.WriteFile(path+suffix, []byte("synthetic"), 0600); e != nil {
				t.Fatal(e)
			}
			defer os.Remove(path + suffix)
			check := snapshotUnchanged(t, path)
			got, e := LibraryBackupSnapshot(context.Background(), path)
			check()
			if e == nil || got != nil {
				t.Fatal("sidecar admitted")
			}
		})
	}
}

func TestLibraryBackupAdmissionAndConnectionLifetime(t *testing.T) {
	s, path := testStore(t)
	libraryAdd(t, s, Scope{"a", "u"}, 1)
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	var borrowed *sql.DB
	check := snapshotUnchanged(t, path)
	e := validatedBackup(ctx, path, func(db *sql.DB, v int) error {
		borrowed = db
		if v != schemaVersion {
			t.Fatal("version", v)
		}
		if _, e := db.ExecContext(ctx, "DELETE FROM library_assets"); e == nil {
			t.Fatal("writable borrowed connection")
		}
		return errors.New("raw SQL/path canary")
	})
	check()
	if !errors.Is(e, ErrStorage) || strings.Contains(e.Error(), "canary") {
		t.Fatal("unsafe callback error", e)
	}
	if borrowed == nil || borrowed.PingContext(ctx) == nil {
		t.Fatal("connection not closed before return")
	}
	ctx, cancel := context.WithCancel(context.Background())
	e = validatedBackup(ctx, path, func(db *sql.DB, _ int) error { borrowed = db; cancel(); return nil })
	if !errors.Is(e, context.Canceled) || borrowed.PingContext(context.Background()) == nil {
		t.Fatal("canceled result/connection", e)
	}
	if e = validatedBackup(nil, path, func(*sql.DB, int) error { t.Fatal("nil context callback"); return nil }); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	for _, query := range []string{`UPDATE library_assets SET tags=CAST(zeroblob(1048576) AS TEXT)`, `PRAGMA user_version=7`, `CREATE TABLE raw_ddl_canary(x)`, `PRAGMA writable_schema=ON; UPDATE sqlite_schema SET sql=replace(sql,'description TEXT','description BLOB') WHERE name='library_assets'`} {
		t.Run(query[:min(24, len(query))], func(t *testing.T) {
			s, path := testStore(t)
			libraryAdd(t, s, Scope{"a", "u"}, 1)
			if _, e := s.db.Exec(query); e != nil {
				t.Fatal(e)
			}
			if e := s.Close(); e != nil {
				t.Fatal(e)
			}
			check := snapshotUnchanged(t, path)
			e := validatedBackup(context.Background(), path, func(*sql.DB, int) error { t.Fatal("callback preceded admission"); return nil })
			check()
			if e == nil {
				t.Fatal("unsafe admission succeeded")
			}
		})
	}
}

func TestLibraryBackupExactDeclaredByteBudget(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	for n := 1; n <= 16; n++ {
		a := libraryFixture(Scope{"a", "u"}, n)
		a.Bytes = 4 << 20
		if _, _, e := s.AddLibraryAsset(ctx, a.Scope, a); e != nil {
			t.Fatal(e)
		}
	}
	want, e := s.LibrarySnapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	check := snapshotUnchanged(t, path)
	got, e := LibraryBackupSnapshot(ctx, path)
	check()
	if e != nil || !reflect.DeepEqual(want, got) {
		t.Fatal("exact 64MiB refused", e)
	}
}

func TestLibraryBackupJournalPolicyCompatibility(t *testing.T) {
	s, path := testStore(t)
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path+"-journal", []byte("synthetic"), 0600); e != nil {
		t.Fatal(e)
	}
	check := snapshotUnchanged(t, path)
	if e := ValidateBackup(context.Background(), path); e != nil {
		t.Fatal("historical validation journal distinction changed", e)
	}
	got, e := LibraryBackupSnapshot(context.Background(), path)
	if got != nil || !errors.Is(e, ErrInvalid) {
		t.Fatal("new metadata API admitted journal", e)
	}
	check()
}
