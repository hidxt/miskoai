package privatefs

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenReadBorrowedCheckedHandle(t *testing.T) {
	p := filepath.Join(fixtureDir(t), "read")
	writeFixture(t, p, "data")
	f, e := OpenRead(p, 4)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	b, e := io.ReadAll(f)
	if e != nil || string(b) != "data" {
		t.Fatal("read", e)
	}
	if _, e = f.Write([]byte("x")); e == nil {
		t.Fatal("writable handle")
	}
	if _, e = os.Stat(p); e != nil {
		t.Fatal("borrowed file removed")
	}
	if f, e := OpenRead(p, 3); e == nil {
		f.Close()
		t.Fatal("oversize admitted")
	}
}
