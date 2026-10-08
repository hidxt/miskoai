package storage

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExistingParentPermissionsArePreserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits require native Unix execution")
	}
	s, _ := testStore(t)
	for _, mode := range []os.FileMode{0755, 0750, 0700} {
		parent := filepath.Join(t.TempDir(), "existing")
		if err := os.Mkdir(parent, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(parent, mode); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(parent, "backup.db")
		err := s.Backup(context.Background(), path)
		if mode != 0700 && err == nil {
			t.Fatalf("accepted nonprivate directory %o", mode)
		}
		if mode == 0700 && err != nil {
			t.Fatal(err)
		}
		info, statErr := os.Stat(parent)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("changed parent mode from %o to %o", mode, info.Mode().Perm())
		}
	}
}

func TestValidateBackupAcceptsOnlySupportedSnapshotWithoutMutation(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	valid := filepath.Join(t.TempDir(), "snapshot # bound.db")
	if err := s.Backup(ctx, valid); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateBackup(ctx, valid); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("validation modified snapshot")
	}
	for _, version := range []int{0, 1, 2} {
		path := filepath.Join(t.TempDir(), "unrelated.db")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`CREATE TABLE unrelated(secret TEXT); INSERT INTO unrelated VALUES('synthetic')`); err != nil {
			t.Fatal(err)
		}
		if version == 1 {
			_, err = db.Exec("PRAGMA user_version=1")
		}
		if version == 2 {
			_, err = db.Exec("PRAGMA user_version=2")
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
		before, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateBackup(ctx, path); err == nil {
			t.Fatalf("accepted unrelated version%d database", version)
		}
		after, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("validation modified unrelated database")
		}
		if _, err = os.Stat(path + "-wal"); !os.IsNotExist(err) {
			t.Fatalf("created WAL: %v", err)
		}
		if _, err = os.Stat(path + "-shm"); !os.IsNotExist(err) {
			t.Fatalf("created SHM: %v", err)
		}
	}
	for _, statement := range []string{
		"CREATE TABLE unexpected(extra TEXT)",
		"DROP TRIGGER facts_ai",
		"ALTER TABLE facts ADD COLUMN extra TEXT",
		"DROP TRIGGER facts_ai; CREATE TRIGGER facts_ai AFTER INSERT ON facts BEGIN UPDATE facts SET importance=0; END",
		"PRAGMA user_version=0",
	} {
		path := filepath.Join(t.TempDir(), "modified.db")
		if err = s.Backup(ctx, path); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
		before, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateBackup(ctx, path); err == nil {
			t.Fatalf("accepted modified schema: %s", statement)
		}
		after, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("modified rejected snapshot: %s", statement)
		}
	}
}
