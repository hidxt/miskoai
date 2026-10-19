package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibrarySchemaFreshReopenBackup(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	var version int
	if e := s.db.QueryRow("PRAGMA user_version").Scan(&version); e != nil || version != schemaVersion {
		t.Fatalf("schema5: %d %v", version, e)
	}
	sc := Scope{"a", "u"}
	a := libraryAdd(t, s, sc, 1)
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	if e := ValidateBackup(ctx, path); e != nil {
		t.Fatal(e)
	}
	r, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	got, e := r.LibraryAsset(ctx, sc, a.ID)
	if e != nil || got.ID != a.ID {
		t.Fatal("reopen", e)
	}
	if e = validateManifest(ctx, r.db); e != nil {
		t.Fatal(e)
	}
}
func TestLibrarySchemaLegacyExactAdmission(t *testing.T) {
	for version := 1; version <= 4; version++ {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private", "fixture.db")
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
			before, _ := os.ReadFile(path)
			if e = ValidateBackup(context.Background(), path); e != nil {
				t.Fatal(e)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("legacy validation mutation")
			}
			s, e := Open(path)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			var got int
			if e = s.db.QueryRow("PRAGMA user_version").Scan(&got); e != nil || got != schemaVersion {
				t.Fatalf("migration %d %v", got, e)
			}
			if e = validateManifest(context.Background(), s.db); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestLibrarySchemaMalformedImmutable(t *testing.T) {
	cases := []string{`description=CAST(X'FF' AS TEXT)`, `description=CAST(zeroblob(1048576) AS TEXT)`, `tags='null'`, `tags='["x","x"]'`, `tags='["\u0000"]'`, `tags='["x"] trailing'`, `tags='[  ]'`, `tags=X'5B5D'`, `sha256='bad'`, `sha256=upper(sha256)`, `emotion=char(127)`, `account=''`, `width=0`, `width=8193`, `height=4000,width=4001`, `frames=2`, `format='webp'`, `bytes=0`, `bytes=4194305`, `bytes='wrong'`, `state='deleting',enabled=1`, `state='foreign'`, `enabled=2`, `uses=-1`, `uses=1.5`, `last_used=-1`, `last_used=253402300800000`}
	for i, set := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			s, path := testStore(t)
			sc := Scope{"a", "u"}
			a := libraryFixture(sc, 26)
			_, _, e := s.AddLibraryAsset(context.Background(), sc, a)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.db.Exec("PRAGMA ignore_check_constraints=ON; UPDATE library_assets SET " + set); e != nil {
				t.Fatal(e)
			}
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			before, _ := os.ReadFile(path)
			if e = ValidateBackup(context.Background(), path); !errors.Is(e, ErrInvalid) {
				t.Fatalf("backup %v", e)
			}
			opened, e := Open(path)
			if opened != nil {
				opened.Close()
			}
			if !errors.Is(e, ErrInvalid) {
				t.Fatalf("open %v", e)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("refusal changed DB")
			}
			for _, suffix := range []string{"-wal", "-shm"} {
				if _, e = os.Stat(path + suffix); !os.IsNotExist(e) {
					t.Fatal("refusal sidecar", e)
				}
			}
		})
	}
}

type libraryNoMaterialize struct {
	db    *sql.DB
	calls int
}

func (q *libraryNoMaterialize) QueryRowContext(c context.Context, s string, args ...any) *sql.Row {
	return q.db.QueryRowContext(c, s, args...)
}
func (q *libraryNoMaterialize) QueryContext(c context.Context, s string, args ...any) (*sql.Rows, error) {
	q.calls++
	return q.db.QueryContext(c, s, args...)
}
func TestLibrarySchemaPreallocationAdmission(t *testing.T) {
	s, _ := testStore(t)
	sc := Scope{"a", "u"}
	libraryAdd(t, s, sc, 1)
	if _, e := s.db.Exec(`UPDATE library_assets SET description=CAST(zeroblob(1048576) AS TEXT)`); e != nil {
		t.Fatal(e)
	}
	q := &libraryNoMaterialize{db: s.db}
	if e := validateLibraryCapacity(context.Background(), q); !errors.Is(e, ErrInvalid) || q.calls != 0 {
		t.Fatalf("oversized materialized: calls%d err%v", q.calls, e)
	}
}
func TestLibrarySchemaCountBytesSequenceAndHighID(t *testing.T) {
	ctx := context.Background()
	sc := Scope{"a", "u"}
	t.Run("highID", func(t *testing.T) {
		s, _ := testStore(t)
		a := libraryAdd(t, s, sc, 1)
		const high int64 = 9007199254740993
		if _, e := s.db.Exec("UPDATE library_assets SET id=? WHERE id=?; UPDATE sqlite_sequence SET seq=? WHERE name='library_assets'", high, a.ID, high); e != nil {
			t.Fatal(e)
		}
		got, e := s.LibraryAsset(ctx, sc, high)
		if e != nil || got.ID != high {
			t.Fatal("highID", e)
		}
		next := libraryAdd(t, s, sc, 2)
		if next.ID != high+1 {
			t.Fatal("precision lost")
		}
		if _, e = s.db.Exec("UPDATE sqlite_sequence SET seq=? WHERE name='library_assets'", int64(math.MaxInt64)); e != nil {
			t.Fatal(e)
		}
		if _, _, e = s.AddLibraryAsset(ctx, sc, libraryFixture(sc, 3)); !errors.Is(e, ErrCapacity) {
			t.Fatal("exhausted ID", e)
		}
	})
	t.Run("sequence", func(t *testing.T) {
		s, _ := testStore(t)
		libraryAdd(t, s, sc, 1)
		for _, bad := range []string{"UPDATE sqlite_sequence SET seq=0 WHERE name='library_assets'", "DELETE FROM sqlite_sequence WHERE name='library_assets'", "INSERT INTO sqlite_sequence VALUES('foreign',1)", "INSERT INTO sqlite_sequence VALUES('library_assets',1)"} {
			tx, e := s.db.Begin()
			if e != nil {
				t.Fatal(e)
			}
			if _, e = tx.Exec(bad); e != nil {
				t.Fatal(e)
			}
			if e = validateLibraryCapacity(ctx, tx); !errors.Is(e, ErrInvalid) {
				t.Fatalf("sequence %s %v", bad, e)
			}
			tx.Rollback()
		}
	})
	t.Run("count", func(t *testing.T) {
		s, _ := testStore(t)
		libraryAdd(t, s, sc, 1)
		if _, e := s.db.Exec(`WITH RECURSIVE n(i) AS (VALUES(2) UNION ALL SELECT i+1 FROM n WHERE i<129) INSERT INTO library_assets(account,user,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used) SELECT 'a','u',printf('%064x',i),'png',1,1,1,1,'[]','','','ready',0,0,0 FROM n`); e != nil {
			t.Fatal(e)
		}
		q := &libraryNoMaterialize{db: s.db}
		if e := validateLibraryCapacity(ctx, q); !errors.Is(e, ErrCapacity) || q.calls != 0 {
			t.Fatal("count allocated", e, q.calls)
		}
	})
	t.Run("bytes", func(t *testing.T) {
		s, _ := testStore(t)
		libraryAdd(t, s, sc, 1)
		if _, e := s.db.Exec(`WITH RECURSIVE n(i) AS (VALUES(2) UNION ALL SELECT i+1 FROM n WHERE i<17) INSERT INTO library_assets(account,user,sha256,format,bytes,width,height,frames,tags,emotion,description,state,enabled,uses,last_used) SELECT 'a','u',printf('%064x',i),'gif',4194304,1,1,32,'[]','','','ready',0,0,0 FROM n; UPDATE library_assets SET bytes=4194304`); e != nil {
			t.Fatal(e)
		}
		q := &libraryNoMaterialize{db: s.db}
		if e := validateLibraryCapacity(ctx, q); !errors.Is(e, ErrCapacity) || q.calls != 0 {
			t.Fatal("byte budget allocated", e, q.calls)
		}
	})
}
func TestLibrarySchemaEffectiveWALRefusal(t *testing.T) {
	s, path := testStore(t)
	sc := Scope{"a", "u"}
	libraryAdd(t, s, sc, 1)
	if _, e := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE); PRAGMA wal_autocheckpoint=0; UPDATE library_assets SET description=CAST(zeroblob(1048576) AS TEXT)"); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	wal, e := os.ReadFile(path + "-wal")
	if e != nil || len(wal) == 0 {
		t.Fatal("effective WAL fixture", e)
	}
	opened, e := Open(path)
	if opened != nil {
		opened.Close()
	}
	if !errors.Is(e, ErrInvalid) {
		t.Fatal("effective WAL admitted", e)
	}
	after, _ := os.ReadFile(path)
	walAfter, _ := os.ReadFile(path + "-wal")
	if !bytes.Equal(before, after) || !bytes.Equal(wal, walAfter) {
		t.Fatal("WAL refusal mutation")
	}
}
func TestLibrarySchemaUnknownFutureAndRollback(t *testing.T) {
	for _, bad := range []string{"CREATE TABLE foreign_object(x)", "PRAGMA user_version=7", "CREATE TRIGGER library_foreign AFTER INSERT ON library_assets BEGIN SELECT 1; END"} {
		t.Run(strings.Split(bad, " ")[0], func(t *testing.T) {
			s, path := testStore(t)
			if _, e := s.db.Exec(bad); e != nil {
				t.Fatal(e)
			}
			s.Close()
			before, _ := os.ReadFile(path)
			opened, e := Open(path)
			if opened != nil {
				opened.Close()
			}
			if !errors.Is(e, ErrInvalid) {
				t.Fatal("foreign admitted", e)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("foreign mutation")
			}
		})
	}
	t.Run("transactional insert", func(t *testing.T) {
		s, _ := testStore(t)
		if _, e := s.db.Exec("CREATE TRIGGER library_fail AFTER INSERT ON library_assets BEGIN SELECT RAISE(ABORT,'synthetic'); END"); e != nil {
			t.Fatal(e)
		}
		sc := Scope{"a", "u"}
		if _, _, e := s.AddLibraryAsset(context.Background(), sc, libraryFixture(sc, 1)); !errors.Is(e, ErrStorage) {
			t.Fatal(e)
		}
		var count int
		if e := s.db.QueryRow("SELECT count(*) FROM library_assets").Scan(&count); e != nil || count != 0 {
			t.Fatal("partial insert", e)
		}
		if _, e := s.db.Exec("DROP TRIGGER library_fail"); e != nil {
			t.Fatal(e)
		}
		a := libraryAdd(t, s, sc, 1)
		if a.ID != 1 {
			t.Fatal("sequence changed on rollback")
		}
	})
}
