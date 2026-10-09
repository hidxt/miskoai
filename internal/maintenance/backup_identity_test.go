package maintenance

import (
	"context"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupOwnedReturnsProducerHandleIdentity(t *testing.T) {
	dir := privateDir(t)
	s, e := storage.Open(filepath.Join(dir, "miskoai.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	path := filepath.Join(dir, "snapshot.db")
	owned, e := BackupOwned(context.Background(), s, path)
	if e != nil {
		t.Fatal(e)
	}
	if owned == nil {
		t.Fatal("verified producer identity absent")
	}
	held := path + ".held"
	if e = os.Rename(path, held); e != nil {
		t.Fatal(e)
	}
	f, e := privatefs.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.WriteString("replacement"); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	original, e := os.Stat(held)
	if e != nil {
		t.Fatal(e)
	}
	replacement, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	if !os.SameFile(owned, original) || os.SameFile(owned, replacement) {
		t.Fatal("returned metadata rebound to replacement pathname")
	}
}
