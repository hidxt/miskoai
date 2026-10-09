package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func replaceBackup(t *testing.T, path string) os.FileInfo {
	t.Helper()
	if e := os.Rename(path, path+".owned"); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte("unowned synthetic replacement"), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	return info
}
func TestBackupFailureCleanupRetainsReplacement(t *testing.T) {
	for _, kind := range []string{"vacuum-failure", "create-close-failure"} {
		t.Run(kind, func(t *testing.T) {
			s, path := testStore(t)
			dest := filepath.Join(filepath.Dir(path), "snapshot.db")
			var replacement os.FileInfo
			fault := errors.New("synthetic backup failure")
			ops := backupOperations{}
			if kind == "vacuum-failure" {
				ops.vacuum = func(_ context.Context, path string) error { replacement = replaceBackup(t, path); return fault }
			} else {
				ops.closeCreated = func(f *os.File) error {
					if e := f.Close(); e != nil {
						t.Fatal(e)
					}
					replacement = replaceBackup(t, f.Name())
					return fault
				}
			}
			if _, e := s.backupOwned(context.Background(), dest, ops); e == nil {
				t.Fatal("injected backup failure ignored")
			}
			got, e := os.ReadFile(dest)
			if e != nil || string(got) != "unowned synthetic replacement" {
				t.Fatal("unowned replacement removed or modified", e)
			}
			after, e := os.Stat(dest)
			if e != nil || !os.SameFile(after, replacement) || after.Mode() != replacement.Mode() || after.Size() != replacement.Size() || !after.ModTime().Equal(replacement.ModTime()) {
				t.Fatal("replacement metadata changed", e)
			}
		})
	}
}
func TestBackupRefusesReplacementBeforeReopen(t *testing.T) {
	s, path := testStore(t)
	dest := filepath.Join(filepath.Dir(path), "snapshot.db")
	var replacement os.FileInfo
	ops := backupOperations{vacuum: func(ctx context.Context, p string) error {
		if _, e := s.db.ExecContext(ctx, "VACUUM INTO ?", p); e != nil {
			return e
		}
		replacement = replaceBackup(t, p)
		return nil
	}}
	if _, e := s.backupOwned(context.Background(), dest, ops); e == nil {
		t.Fatal("replaced snapshot admitted for reopen")
	}
	got, e := os.ReadFile(dest)
	if e != nil || string(got) != "unowned synthetic replacement" {
		t.Fatal("replacement changed", e)
	}
	after, e := os.Stat(dest)
	if e != nil || !os.SameFile(after, replacement) {
		t.Fatal("replacement removed", e)
	}
}

func TestBackupFailureCleansOnlyOwnedDestination(t *testing.T) {
	s, path := testStore(t)
	dest := filepath.Join(filepath.Dir(path), "snapshot.db")
	ops := backupOperations{vacuum: func(context.Context, string) error { return errors.New("synthetic owned backup failure") }}
	if _, e := s.backupOwned(context.Background(), dest, ops); e == nil {
		t.Fatal("backup failure ignored")
	}
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("owned failed destination retained", e)
	}
	before := []byte("existing unowned snapshot")
	if e := os.WriteFile(dest, before, 0600); e != nil {
		t.Fatal(e)
	}
	if e := s.Backup(context.Background(), dest); e == nil {
		t.Fatal("existing snapshot overwritten")
	}
	got, e := os.ReadFile(dest)
	if e != nil || string(got) != string(before) {
		t.Fatal("existing snapshot changed", e)
	}
}

func TestBackupOwnedReturnsOriginalHandleIdentity(t *testing.T) {
	s, path := testStore(t)
	dest := filepath.Join(filepath.Dir(path), "snapshot.db")
	owned, e := s.BackupOwned(context.Background(), dest)
	if e != nil {
		t.Fatal(e)
	}
	replacement := replaceBackup(t, dest)
	if os.SameFile(owned, replacement) {
		t.Fatal("returned identity rebound to replacement")
	}
	original, e := os.Stat(dest + ".owned")
	if e != nil || !os.SameFile(owned, original) {
		t.Fatal("original created identity lost", e)
	}
}

func TestBackupRefusesReplacementDuringReopen(t *testing.T) {
	s, path := testStore(t)
	dest := filepath.Join(filepath.Dir(path), "snapshot.db")
	var replacement os.FileInfo
	ops := backupOperations{reopen: func(path string) (*os.File, error) {
		replacement = replaceBackup(t, path)
		return os.OpenFile(path, os.O_RDWR, 0600)
	}}
	if _, e := s.backupOwned(context.Background(), dest, ops); e == nil {
		t.Fatal("replacement during reopen accepted")
	}
	got, e := os.ReadFile(dest)
	if e != nil || string(got) != "unowned synthetic replacement" {
		t.Fatal("replacement changed", e)
	}
	after, e := os.Stat(dest)
	if e != nil || !os.SameFile(replacement, after) {
		t.Fatal("replacement identity changed", e)
	}
}
