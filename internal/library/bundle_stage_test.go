package library

import (
	"bytes"
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/privatefs"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBundleStageExistingRefusesBeforeRead(t *testing.T) {
	f := setup(t)
	p := filepath.Join(f.dir, ".library-bundle-stage")
	if e := privatefs.EnsureDir(p); e != nil {
		t.Fatal(e)
	}
	e := f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
		return v.PrepareBundle(panicBundleIO{}, func(*PreparedBundle) error { return nil })
	})
	if !errors.Is(e, ErrRecovery) {
		t.Fatal(e)
	}
	if _, e = os.Lstat(p); e != nil {
		t.Fatal("reservation removed", e)
	}
}
func TestBundleStageUnknownAndReplacementPreserved(t *testing.T) {
	for _, kind := range []string{"unknown", "file", "directory"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			s, e := reserveBundle(f.m)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.write(context.Background(), "database", bytes.NewReader([]byte("raw")), 3); e != nil {
				t.Fatal(e)
			}
			path := s.path("database")
			switch kind {
			case "unknown":
				writePrivate(t, s.path("unknown"), []byte("unknown"))
				path = s.path("unknown")
			case "file":
				if e = os.Rename(path, path+"-original"); e != nil {
					t.Fatal(e)
				}
				writePrivate(t, path, []byte("replacement"))
			case "directory":
				old := s.dir + "-original"
				if e = os.Rename(s.dir, old); e != nil {
					t.Fatal(e)
				}
				if e = privatefs.EnsureDir(s.dir); e != nil {
					t.Fatal(e)
				}
				path = s.dir
			}
			if e = s.cleanup(); !errors.Is(e, ErrRecovery) {
				t.Fatal("unsafe cleanup", e)
			}
			if _, e = os.Lstat(path); e != nil {
				t.Fatal("replacement removed", e)
			}
			if _, e = reserveBundle(f.m); !errors.Is(e, ErrRecovery) {
				t.Fatal("second reservation", e)
			}
		})
	}
}
func TestBundleStagePartialReadCleanup(t *testing.T) {
	f := setup(t)
	s, e := reserveBundle(f.m)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.write(context.Background(), "database", bytes.NewReader([]byte("x")), 2); e == nil {
		t.Fatal("short accepted")
	}
	if e = s.cleanup(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Lstat(s.dir); !errors.Is(e, os.ErrNotExist) {
		t.Fatal(e)
	}
}

type heldBundleReader struct {
	input            *bytes.Reader
	entered, release chan struct{}
	held             bool
}

func (r *heldBundleReader) Read(p []byte) (int, error) {
	if !r.held {
		r.held = true
		close(r.entered)
		<-r.release
	}
	return r.input.Read(p)
}
func TestBundleHeldReaderCloseJoins(t *testing.T) {
	f := setup(t)
	raw := bundleBytes(t, f)
	r := &heldBundleReader{input: bytes.NewReader(raw), entered: make(chan struct{}), release: make(chan struct{})}
	finished := make(chan error, 1)
	closed := make(chan error, 1)
	unblocked := false
	t.Cleanup(func() {
		if !unblocked {
			close(r.release)
		}
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
		}
	})
	go func() {
		finished <- f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
			return v.PrepareBundle(r, func(*PreparedBundle) error { t.Error("canceled callback"); return nil })
		})
	}()
	<-r.entered
	go func() { closed <- f.m.Close() }()
	select {
	case e := <-closed:
		t.Fatal("Close returned with reader active", e)
	case <-time.After(25 * time.Millisecond):
	}
	close(r.release)
	unblocked = true
	select {
	case e := <-finished:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reader did not join")
	}
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close did not join")
	}
}
func TestBundleCanceledBeforeEffects(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	e := f.m.WithMaintenance(ctx, func(_ context.Context, v *Maintenance) error {
		cancel()
		if e := v.WriteBundle(f.l, panicBundleIO{}); !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
		return v.PrepareBundle(panicBundleIO{}, func(*PreparedBundle) error { return nil })
	})
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = os.Lstat(filepath.Join(f.dir, ".library-bundle-stage")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("canceled stage", e)
	}
}
func TestBundleCallbackErrorSanitizedAndStageJoined(t *testing.T) {
	f := setup(t)
	raw := bundleBytes(t, f)
	var kept *PreparedBundle
	e := f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
		return v.PrepareBundle(bytes.NewReader(raw), func(p *PreparedBundle) error { kept = p; return errors.New("raw-sensitive-canary") })
	})
	if !errors.Is(e, ErrContent) || e.Error() == "raw-sensitive-canary" {
		t.Fatal(e)
	}
	if kept.self != nil || kept.candidate != nil {
		t.Fatal("candidate escaped")
	}
	entries, e := os.ReadDir(f.dir)
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range entries {
		if x.Name() == ".library-bundle-stage" || len(x.Name()) > 8 && x.Name()[:8] == "restore-" {
			t.Fatal("leftover", x.Name())
		}
	}
}
func TestBundleStageWriteBorrowed(t *testing.T) {
	f := setup(t)
	s, e := reserveBundle(f.m)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.write(context.Background(), "database", io.LimitReader(bytes.NewReader([]byte("abc")), 3), 3); e != nil {
		t.Fatal(e)
	}
	r, release, e := s.open("database", 256<<20)
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(r)
	if e != nil || string(b) != "abc" {
		t.Fatal(e)
	}
	if e = release(); e != nil {
		t.Fatal(e)
	}
	if e = s.cleanup(); e != nil {
		t.Fatal(e)
	}
}
func TestBundleStageActualWriteSyncCloseFailures(t *testing.T) {
	for _, point := range []string{"write", "sync", "close"} {
		t.Run(point, func(t *testing.T) {
			f := setup(t)
			s, e := reserveBundle(f.m)
			if e != nil {
				t.Fatal(e)
			}
			calls := map[string]int{}
			ops := bundleStageIO{write: func(f *os.File, b []byte) (int, error) {
				calls["write"]++
				n, e := f.Write(b)
				if point == "write" {
					return n, errors.New("write-canary")
				}
				return n, e
			}, sync: func(f *os.File) error {
				calls["sync"]++
				e := f.Sync()
				if point == "sync" {
					return errors.New("sync-canary")
				}
				return e
			}, close: func(f *os.File) error {
				calls["close"]++
				e := f.Close()
				if point == "close" {
					return errors.New("close-canary")
				}
				return e
			}}
			e = s.writeWith(context.Background(), "database", bytes.NewReader([]byte("test")), 4, ops)
			if !errors.Is(e, ErrCleanup) || calls["write"] != 1 || calls["close"] != 1 {
				t.Fatal("actual boundary", calls, e)
			}
			if point != "write" && calls["sync"] != 1 {
				t.Fatal("sync not run", calls)
			}
			if e = s.cleanup(); e != nil {
				t.Fatal("original cleanup", e)
			}
		})
	}
}
func TestBundleHeldCallbackCloseJoins(t *testing.T) {
	f := setup(t)
	raw := bundleBytes(t, f)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	closed := make(chan error, 1)
	unblocked := false
	var kept *PreparedBundle
	t.Cleanup(func() {
		if !unblocked {
			close(release)
		}
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
		}
	})
	go func() {
		finished <- f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
			return v.PrepareBundle(bytes.NewReader(raw), func(p *PreparedBundle) error { kept = p; close(entered); <-release; return v.ctx.Err() })
		})
	}()
	<-entered
	go func() { closed <- f.m.Close() }()
	select {
	case e := <-closed:
		t.Fatal("Close returned with callback active", e)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	unblocked = true
	select {
	case e := <-finished:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("callback did not join")
	}
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not join")
	}
	if kept.self != nil || kept.candidate != nil {
		t.Fatal("prepared still active")
	}
}
func TestBundleStageLinkedOriginalPreserved(t *testing.T) {
	f := setup(t)
	s, e := reserveBundle(f.m)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.write(context.Background(), "database", bytes.NewReader([]byte("raw")), 3); e != nil {
		t.Fatal(e)
	}
	path := s.path("database")
	link := filepath.Join(f.dir, "stage-hardlink")
	if e = os.Link(path, link); e != nil {
		t.Skipf("host hardlink unavailable: %v", e)
	}
	if _, release, e := s.open("database", bundleDBLimit); e == nil {
		release()
		t.Fatal("linked source accepted")
	}
	if e = s.cleanup(); !errors.Is(e, ErrRecovery) {
		t.Fatal("linked cleanup", e)
	}
	if _, e = os.Lstat(path); e != nil {
		t.Fatal("linked original removed", e)
	}
}
