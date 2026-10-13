package maintenance

import (
	"bytes"
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/storage"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func libraryCandidate(t *testing.T) (*Candidate, []storage.LibraryAsset) {
	t.Helper()
	d := privateDir(t)
	s, e := storage.Open(filepath.Join(d, "source.db"))
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	sc := storage.Scope{Account: "a", User: "u"}
	a := storage.LibraryAsset{Scope: sc, SHA256: strings.Repeat("a", 64), Format: "png", Bytes: 123, Width: 2, Height: 3, Frames: 1, Tags: []string{"tag"}, Emotion: "emotion", Description: "synthetic", State: "ready"}
	if _, _, e = s.AddLibraryAsset(ctx, sc, a); e != nil {
		t.Fatal(e)
	}
	want, e := s.LibrarySnapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(d, "snapshot.db")
	if e = s.Backup(ctx, p); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	c, e := PrepareRestore(ctx, d, bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Discard() })
	return c, want
}

func TestCandidateLibraryOwnershipAndEquality(t *testing.T) {
	c, want := libraryCandidate(t)
	before, e := os.ReadFile(c.path)
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.LibrarySnapshot(context.Background())
	if e != nil || !reflect.DeepEqual(want, got) {
		t.Fatal("metadata mismatch", e)
	}
	after, e := os.ReadFile(c.path)
	if e != nil || !bytes.Equal(before, after) || c.finished {
		t.Fatal("candidate changed", e)
	}
	// Construct a copied value without copying the mutex (go vet copylocks).
	copy := &Candidate{self: c.self, dir: c.dir, path: c.path, info: c.info}
	if got, e = copy.LibrarySnapshot(context.Background()); got != nil || !errors.Is(e, errOwnership) {
		t.Fatal("copy accepted", e)
	}
	if e = c.Discard(); e != nil {
		t.Fatal(e)
	}
	if got, e = c.LibrarySnapshot(context.Background()); got != nil || !errors.Is(e, errOwnership) {
		t.Fatal("discarded accepted", e)
	}
}

func TestCandidateLibraryContextsChangedAndFinished(t *testing.T) {
	for _, mode := range []string{"nil", "canceled", "changed", "replacement", "finished", "journal"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := libraryCandidate(t)
			ctx := context.Background()
			expected := errOwnership
			switch mode {
			case "nil":
				ctx = nil
				expected = errRestore
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				expected = context.Canceled
			case "changed":
				f, e := os.OpenFile(c.path, os.O_APPEND|os.O_WRONLY, 0600)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = f.Write([]byte("change")); e != nil {
					t.Fatal(e)
				}
				if e = f.Close(); e != nil {
					t.Fatal(e)
				}
			case "replacement":
				if e := os.Rename(c.path, c.path+"-original"); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(c.path, []byte("replacement canary"), 0600); e != nil {
					t.Fatal(e)
				}
			case "finished":
				c.finished = true
			case "journal":
				if e := os.WriteFile(c.path+"-wal", []byte("journal"), 0600); e != nil {
					t.Fatal(e)
				}
				expected = errJournals
			}
			before, e := os.ReadFile(c.path)
			if e != nil {
				t.Fatal(e)
			}
			got, e := c.LibrarySnapshot(ctx)
			if got != nil || !errors.Is(e, expected) {
				t.Fatal("unsafe/context candidate accepted", mode, e)
			}
			after, e := os.ReadFile(c.path)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("replacement changed", e)
			}
		})
	}
	var c *Candidate
	if got, e := c.LibrarySnapshot(context.Background()); got != nil || !errors.Is(e, errOwnership) {
		t.Fatal("nil candidate accepted", e)
	}
}

func TestCandidateLibraryInstalledRefused(t *testing.T) {
	c, _ := libraryCandidate(t)
	l, e := Acquire(c.dir)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	if _, e = InstallRestore(context.Background(), l, c); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(c.dir, "miskoai.db")
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.LibrarySnapshot(context.Background())
	if got != nil || !errors.Is(e, errOwnership) {
		t.Fatal("installed candidate admitted", e)
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("installed database changed", e)
	}
}
