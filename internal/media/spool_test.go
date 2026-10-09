package media

import (
	"bytes"
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/privatefs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testingDeadlinePast() time.Time { return time.Unix(1, 0) }
func replaceSpool(t *testing.T, path string) {
	t.Helper()
	if e := os.Rename(path, path+".original"); e != nil {
		t.Fatal(e)
	}
	f, e := privatefs.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write([]byte("replacement remains")); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
func replacementRemains(t *testing.T, path string) {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil || string(b) != "replacement remains" {
		t.Fatalf("replacement changed: %v", e)
	}
}
func TestArtifactReplacementOwnership(t *testing.T) {
	t.Run("before-reopen", func(t *testing.T) {
		dir := mediaDir(t)
		var path string
		ops := spoolOps{beforePublish: func(p string) { path = p; replaceSpool(t, p) }}
		a, e := transform(context.Background(), dir, bytes.NewReader([]byte("data")), [16]byte{}, false, ops)
		if a != nil || !errors.Is(e, ErrPrivate) {
			t.Fatalf("replacement published: %v", e)
		}
		replacementRemains(t, path)
	})
	t.Run("before-failure-cleanup", func(t *testing.T) {
		dir := mediaDir(t)
		var path string
		ops := spoolOps{write: func(*os.File, []byte) (int, error) { return 0, errors.New("fail") }, beforeCleanup: func(p string) { path = p; replaceSpool(t, p) }}
		a, e := transform(context.Background(), dir, bytes.NewReader([]byte("data")), [16]byte{}, false, ops)
		if a != nil || !errors.Is(e, ErrIO) {
			t.Fatalf("bad cleanup error: %v", e)
		}
		replacementRemains(t, path)
	})
	t.Run("artifact-close", func(t *testing.T) {
		dir := mediaDir(t)
		a, e := Encrypt(context.Background(), dir, bytes.NewReader([]byte("data")), [16]byte{})
		if e != nil {
			t.Fatal(e)
		}
		// Windows os.Open denies deletion while this read handle is open.
		// Close only this synthetic internal handle to exercise identity refusal
		// on Close after the pathname can be replaced. Production is unchanged.
		if e = a.file.Close(); e != nil {
			t.Fatal(e)
		}
		replaceSpool(t, a.path)
		if !errors.Is(a.Close(), ErrPrivate) {
			t.Fatal("close must refuse replacement")
		}
		replacementRemains(t, a.path)
		if !errors.Is(a.Close(), ErrPrivate) {
			t.Fatal("idempotent refusal")
		}
	})
}
func TestArtifactReadAtAndConcurrentClose(t *testing.T) {
	dir := mediaDir(t)
	a, e := Encrypt(context.Background(), dir, bytes.NewReader([]byte("data")), [16]byte{})
	if e != nil {
		t.Fatal(e)
	}
	if e = privatefs.CheckFile(a.path, a.Size()); e != nil {
		t.Fatal(e)
	}
	b := make([]byte, 16)
	if n, e := a.ReadAt(b, 0); n != 16 || e != nil {
		t.Fatalf("ReadAt %d %v", n, e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := a.Close(); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	noSpools(t, dir)
	if n, e := a.Read(b); n != 0 || !errors.Is(e, ErrIO) {
		t.Fatal("closed artifact usable")
	}
	if filepath.Base(a.path) == "data" {
		t.Fatal("personal filename")
	}
}
