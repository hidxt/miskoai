package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAttachmentSchemaLegacyCompatibility(t *testing.T) {
	for version := 1; version <= 4; version++ {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.db")
			db, err := sql.Open("sqlite", path)
			if db != nil {
				t.Cleanup(func() { db.Close() })
			}
			if err != nil {
				t.Fatal(err)
			}
			ddl := schema
			if version >= 2 {
				ddl += schema2
			}
			if version >= 3 {
				ddl += schema3
			}
			if version == 4 {
				ddl += schema4
			}
			if _, err = db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = ValidateBackup(context.Background(), path); err != nil {
				t.Fatalf("pinned legacy backup %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatal("validation mutated fixture")
			}
			s, err := Open(path)
			if s != nil {
				t.Cleanup(func() { s.Close() })
			}
			if err != nil {
				t.Fatal(err)
			}
			var v int
			if err = s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != 4 {
				t.Fatalf("compatible migration %d %v", v, err)
			}
			if err = validateManifest(context.Background(), s.db); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestAttachmentSchemaRefusalImmutable(t *testing.T) {
	for _, bad := range []string{`CREATE TABLE foreign_object(x)`, `PRAGMA user_version=5`, `INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','id','','',1); INSERT INTO inbox_attachments VALUES(1,X'7B7D')`} {
		t.Run(bad, func(t *testing.T) {
			s, path := testStore(t)
			if _, err := s.db.Exec(bad); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = ValidateBackup(context.Background(), path); !errors.Is(err, ErrInvalid) {
				t.Fatalf("backup admitted %v", err)
			}
			opened, err := Open(path)
			if opened != nil {
				t.Cleanup(func() { opened.Close() })
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("open admitted %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatal("refusal changed snapshot")
			}
			for _, suffix := range []string{"-wal", "-shm"} {
				if _, err = os.Stat(path + suffix); !os.IsNotExist(err) {
					t.Fatalf("refusal journal %s %v", suffix, err)
				}
			}
		})
	}
}

func TestAttachmentLegacyRefusalPreservesPinnedSchema(t *testing.T) {
	for version := 1; version <= 3; version++ {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bad-legacy.db")
			db, err := sql.Open("sqlite", path)
			if db != nil {
				t.Cleanup(func() { db.Close() })
			}
			if err != nil {
				t.Fatal(err)
			}
			ddl := schema
			if version >= 2 {
				ddl += schema2
			}
			if version == 3 {
				ddl += schema3
			}
			if _, err = db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO messages(account,user,id,content,state,created_at,updated_at) VALUES('a','u','id',?,'failed',1,1)`, string(make([]byte, 16385))); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			opened, err := Open(path)
			if opened != nil {
				t.Cleanup(func() { opened.Close() })
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("legacy unsafe admitted %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatal("legacy refused migration mutated bytes")
			}
			for _, suffix := range []string{"-wal", "-shm"} {
				if _, err = os.Stat(path + suffix); !os.IsNotExist(err) {
					t.Fatalf("legacy sidecar %s %v", suffix, err)
				}
			}
		})
	}
}
