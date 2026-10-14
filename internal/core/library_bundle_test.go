package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/librarybundle"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

type coreBundleAPI interface {
	PrepareLibraryBundle(context.Context) (*Download, error)
	RestoreLibraryBundle(context.Context, io.Reader) (maintenance.RestoreResult, error)
}

func bundleController(t *testing.T, configured bool) *Controller {
	t.Helper()
	c := libraryController(t, configured)
	// Close the production CDN before any operation and refuse model/channel work.
	if c.cdn != nil {
		if e := c.cdn.Close(); e != nil {
			t.Fatal(e)
		}
	}
	refuse := modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		t.Error("unexpected external model")
		return provider.Reply{}, ErrUnavailable
	})
	c.model, c.vision = refuse, refuse
	c.channel = &fakeChannel{poll: idle}
	return c
}

// The RED calls preparation first, so a missing Core API cannot hide failure in
// the accepted envelope/preparation prerequisite.
func preparedCoreFixture(t *testing.T, c *Controller) []byte {
	t.Helper()
	var out bytes.Buffer
	e := c.library.WithMaintenance(context.Background(), func(_ context.Context, v *library.Maintenance) error {
		return v.WriteBundle(func(ctx context.Context) (*storage.Store, func(), error) { return c.lease(ctx, false, false) }, &out)
	})
	if e != nil {
		t.Fatal("accepted preparation prerequisite", e)
	}
	return out.Bytes()
}
func coreBundleMethods(t *testing.T, c *Controller) coreBundleAPI {
	t.Helper()
	api, ok := any(c).(coreBundleAPI)
	if !ok {
		t.Fatal("Core complete-library-bundle methods missing after accepted preparation")
	}
	return api
}
func bundleRead(t *testing.T, d *Download) []byte {
	t.Helper()
	b, e := io.ReadAll(d)
	ce := d.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	return b
}
func bundlePreview(t *testing.T, c *Controller, a storage.LibraryAsset, want []byte) {
	t.Helper()
	e := c.WithLibraryAsset(context.Background(), a.ID, false, func(row storage.LibraryAsset, _ attachment.LocalImage, art attachment.Artifact) error {
		b := make([]byte, len(want))
		n, e := art.ReadAt(b, 0)
		if e != nil || n != len(want) || !bytes.Equal(b, want) || row.SHA256 != a.SHA256 {
			t.Error("restored actual bytes", n, e)
		}
		return nil
	})
	if e != nil {
		t.Fatal("preview", e)
	}
}

func TestCoreLibraryBundleDownloadRoundTrip(t *testing.T) {
	c := bundleController(t, true)
	a := libraryUploaded(t, c, libraryPNG(t, 31))
	second := storage.Scope{Account: "synthetic-other", User: "synthetic-user"}
	b := libraryGIF(t)
	_, created, e := c.library.Upload(context.Background(), func(ctx context.Context) (*storage.Store, func(), error) { return c.lease(ctx, false, false) }, second, bytes.NewReader(b), int64(len(b)), storage.LibraryMetadata{Description: "other scope"})
	if e != nil || !created {
		t.Fatal(e)
	}
	preparedCoreFixture(t, c)
	api := coreBundleMethods(t, c)
	d, e := api.PrepareLibraryBundle(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if d.Filename != "miskoai-library.bundle" || d.Size <= 0 {
		t.Fatal("download metadata", d.Filename, d.Size)
	}
	serialized, e := json.Marshal(d)
	if e != nil || bytes.Contains(serialized, []byte(c.cfg.DataDir)) || bytes.Contains(serialized, []byte(".miskoai-download")) {
		t.Fatal("private path serialized", e)
	}
	c.mu.Lock()
	held := c.leases
	exclusive := c.exclusive
	c.mu.Unlock()
	if held != 0 || exclusive {
		t.Fatal("download retains Store lease")
	}
	data := bundleRead(t, d)
	assets := map[string][]byte{}
	var db []byte
	h, e := librarybundle.Decode(context.Background(), bytes.NewReader(data), func(r librarybundle.Record, in io.Reader) error {
		v, e := io.ReadAll(in)
		if r.Kind == librarybundle.Database {
			db = v
		} else {
			assets[r.Key] = v
		}
		return e
	})
	if e != nil || len(h.Assets) != 2 || len(assets) != 2 || len(db) == 0 {
		t.Fatal("all-scope bundle", e, len(h.Assets))
	}
	dest := bundleController(t, false)
	result, e := coreBundleMethods(t, dest).RestoreLibraryBundle(context.Background(), bytes.NewReader(data))
	if e != nil || !result.ChannelPaused || !result.PreviousRetained {
		t.Fatal("unconfigured global restore", result, e)
	}
	rows, e := dest.store.LibrarySnapshot(context.Background())
	if e != nil || len(rows) != 2 {
		t.Fatal(rows, e)
	}
	if rows[0].SHA256 != a.SHA256 {
		t.Fatal("scope ordering/digest")
	}
}

func TestCoreLibraryBundleRestoreAndPause(t *testing.T) {
	source := bundleController(t, true)
	nextBytes := libraryGIF(t)
	next := libraryUploaded(t, source, nextBytes)
	data := preparedCoreFixture(t, source)
	dest := bundleController(t, true)
	old := libraryUploaded(t, dest, libraryPNG(t, 32))
	oldData := preparedCoreFixture(t, dest)
	oldStore := dest.store
	manager := dest.library
	processor := dest.images
	if old.ID != next.ID || old.SHA256 == next.SHA256 {
		t.Fatal("same-ID different-digest fixture")
	}
	result, e := coreBundleMethods(t, dest).RestoreLibraryBundle(context.Background(), bytes.NewReader(data))
	if e != nil || !result.PreviousRetained || !result.ChannelPaused || !dest.paused || dest.store == oldStore || dest.library != manager || dest.images != processor {
		t.Fatal("actual install/lifetime/pause", result, e)
	}
	if _, e := oldStore.LibrarySnapshot(context.Background()); e == nil {
		t.Fatal("old Store still open")
	}
	bundlePreview(t, dest, next, nextBytes)
	if e := dest.LibraryRecordUse(context.Background(), old.ID, old.SHA256, 1); !errors.Is(e, library.ErrRecovery) {
		t.Fatal("old digest mutated fresh row", e)
	}
	if e := dest.LibraryUpdate(context.Background(), next.ID, storage.LibraryMetadata{Enabled: true, Description: "new Store"}); e != nil {
		t.Fatal(e)
	}
	if e := dest.LibraryDelete(context.Background(), next.ID); e != nil {
		t.Fatal(e)
	}
	entries, e := os.ReadDir(filepath.Join(dest.cfg.DataDir, "library"))
	if e != nil || len(entries) != 1 {
		t.Fatal("prior retained content", len(entries), e)
	}
	result, e = dest.RestoreLibraryBundle(context.Background(), bytes.NewReader(oldData))
	if e != nil || !result.ChannelPaused || !result.PreviousRetained {
		t.Fatal("rollback bundle", result, e)
	}
	bundlePreview(t, dest, old, libraryPNG(t, 32))
}

func TestCoreLibraryBundleEmptyAndSlots(t *testing.T) {
	c := bundleController(t, false)
	ctx := context.Background()
	api := coreBundleMethods(t, c)
	d, e := api.PrepareLibraryBundle(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := api.PrepareLibraryBundle(ctx); !errors.Is(e, ErrBusy) {
		t.Fatal("bundle slot", e)
	}
	if _, e := c.PrepareBackup(ctx); !errors.Is(e, ErrBusy) {
		t.Fatal("shared legacy slot", e)
	}
	data := bundleRead(t, d)
	db, e := c.PrepareBackup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := api.PrepareLibraryBundle(ctx); !errors.Is(e, ErrBusy) {
		t.Fatal("legacy occupies bundle slot", e)
	}
	raw := bundleRead(t, db)
	if !bytes.HasPrefix(raw, []byte("SQLite format 3\x00")) {
		t.Fatal("legacy backup envelope changed")
	}
	if _, e := api.RestoreLibraryBundle(ctx, bytes.NewReader(raw)); e == nil {
		t.Fatal("SQLite accepted as full bundle")
	}
	if _, e := c.Restore(ctx, bytes.NewReader(data)); !errors.Is(e, ErrMaintenance) {
		t.Fatal("envelope accepted as SQLite", e)
	}
	result, e := api.RestoreLibraryBundle(ctx, bytes.NewReader(data))
	if e != nil || !result.ChannelPaused {
		t.Fatal(result, e)
	}
	rows, e := c.store.LibrarySnapshot(ctx)
	if e != nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
	d, e = api.PrepareLibraryBundle(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	if len(bundleRead(t, d)) == 0 || len(c.downloads) != 0 {
		t.Fatal("caller download did not survive Core.Close/release")
	}
}

func TestCoreLibraryBundleInputRefusals(t *testing.T) {
	c := bundleController(t, true)
	a := libraryUploaded(t, c, libraryPNG(t, 40))
	oldStore := c.store
	api := coreBundleMethods(t, c)
	ctx := context.Background()
	if _, e := api.PrepareLibraryBundle(nil); !errors.Is(e, ErrConfiguration) {
		t.Fatal(e)
	}
	if _, e := api.RestoreLibraryBundle(nil, bytes.NewReader(nil)); !errors.Is(e, ErrConfiguration) {
		t.Fatal(e)
	}
	if _, e := api.RestoreLibraryBundle(ctx, nil); !errors.Is(e, library.ErrInvalid) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := api.PrepareLibraryBundle(canceled); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := api.RestoreLibraryBundle(canceled, bytes.NewReader(nil)); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	data := preparedCoreFixture(t, c)
	for _, bad := range [][]byte{[]byte("bad private canary"), data[:len(data)-1], append(append([]byte{}, data...), 1)} {
		result, e := api.RestoreLibraryBundle(ctx, bytes.NewReader(bad))
		if e == nil || result != (maintenance.RestoreResult{}) || c.store != oldStore || c.paused {
			t.Fatal("preparation changed DB/pause", result, e)
		}
		bundlePreview(t, c, a, libraryPNG(t, 40))
	}
	if len(c.maintenance) != 0 {
		t.Fatal("refusal leaked maintenance")
	}
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := api.PrepareLibraryBundle(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	if _, e := api.RestoreLibraryBundle(ctx, bytes.NewReader(data)); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}

func TestCoreLibraryBundleOutputFailures(t *testing.T) {
	for _, stage := range []string{"write", "short", "sync", "close", "check", "open"} {
		t.Run(stage, func(t *testing.T) {
			c := bundleController(t, false)
			ops := defaultBundleOutputOperations()
			raw := errors.New("private-path-canary")
			switch stage {
			case "write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, raw }
			case "short":
				ops.write = func(f *os.File, b []byte) (int, error) { return f.Write(b[:len(b)-1]) }
			case "sync":
				ops.sync = func(f *os.File) error {
					if e := f.Sync(); e != nil {
						return e
					}
					return raw
				}
			case "close":
				ops.close = func(f *os.File) error {
					if e := f.Close(); e != nil {
						return e
					}
					return raw
				}
			case "check":
				ops.check = func(string, int64) error { return raw }
			case "open":
				ops.open = func(string, int64) (*os.File, error) { return nil, raw }
			}
			d, e := c.prepareLibraryBundle(context.Background(), ops)
			if e == nil || d != nil || bytes.Contains([]byte(e.Error()), []byte("canary")) {
				t.Fatal("partial/raw output success", e)
			}
			paths, e := filepath.Glob(filepath.Join(c.cfg.DataDir, ".miskoai-download-*"))
			if e != nil || len(paths) != 0 || len(c.downloads) != 0 {
				t.Fatal("failure cleanup/admission", paths, e)
			}
			d, e = c.PrepareLibraryBundle(context.Background())
			if e != nil {
				t.Fatal("slot not reusable", e)
			}
			bundleRead(t, d)
		})
	}
}

func TestCoreLibraryBundleOutputReplacement(t *testing.T) {
	c := bundleController(t, false)
	ops := defaultBundleOutputOperations()
	var replaced string
	ops.check = func(path string, cap int64) error {
		if e := os.Rename(path, path+".original"); e != nil {
			return e
		}
		replaced = path
		writeFixture(t, filepath.Dir(path), filepath.Base(path), "synthetic replacement")
		return nil
	}
	d, e := c.prepareLibraryBundle(context.Background(), ops)
	if e == nil || d != nil || !errors.Is(e, library.ErrRecovery) || !errors.Is(e, library.ErrCleanup) {
		t.Fatal("replaced output accepted/uncertainty lost", e)
	}
	b, e := os.ReadFile(replaced)
	if e != nil || string(b) != "synthetic replacement" {
		t.Fatal("replacement removed", e)
	}
	if len(c.downloads) != 0 {
		t.Fatal("failed replacement slot leaked")
	}
}

func TestCoreLibraryBundleDownloadCloseReplacement(t *testing.T) {
	c := bundleController(t, false)
	d, e := c.PrepareLibraryBundle(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(d.path, d.path+".original"); e != nil {
		t.Fatal(e)
	}
	writeFixture(t, c.cfg.DataDir, filepath.Base(d.path), "synthetic replacement")
	if e := d.Close(); !errors.Is(e, ErrUnavailable) {
		t.Fatal("replacement close accepted", e)
	}
	b, e := os.ReadFile(d.path)
	if e != nil || string(b) != "synthetic replacement" || len(c.downloads) != 0 {
		t.Fatal("replacement removed/slot leaked", e)
	}
	if e := d.Close(); e != nil {
		t.Fatal("idempotent close", e)
	}
}

func TestCoreLibraryBundleOutputCap(t *testing.T) {
	c := bundleController(t, false)
	path := filepath.Join(c.cfg.DataDir, "synthetic-limit")
	f, e := privatefs.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	w := bundleOutputWriter{ctx: context.Background(), f: f, write: defaultBundleOutputOperations().write, remaining: 2}
	if n, e := w.Write([]byte("123")); n != 0 || !errors.Is(e, library.ErrCapacity) {
		t.Fatal("cap before write", n, e)
	}
	if n, e := w.Write([]byte("12")); n != 2 || e != nil {
		t.Fatal(n, e)
	}
	if n, e := w.Write([]byte("3")); n != 0 || !errors.Is(e, library.ErrCapacity) {
		t.Fatal(n, e)
	}
	info, e := f.Stat()
	if e != nil || info.Size() != 2 {
		t.Fatal("cap bytes", e)
	}
}

// A refused operation may finish before reaching its held boundary. Preserve
// that completion for the registered cleanup, and report it without waiting for
// an entry signal that can no longer arrive.
func bundleWaitEntered(t *testing.T, entered <-chan struct{}, done chan error) {
	t.Helper()
	select {
	case <-entered:
	case e := <-done:
		done <- e
		t.Fatal("fixture operation ended before held boundary", e)
	case <-time.After(5 * time.Second):
		t.Fatal("actual job boundary timed out")
	}
}

func TestCoreLibraryBundleHeldOutputClose(t *testing.T) {
	c := bundleController(t, false)
	ops := defaultBundleOutputOperations()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	ops.write = func(f *os.File, b []byte) (int, error) {
		once.Do(func() { close(entered); <-release })
		return f.Write(b)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done, closed := make(chan error, 1), make(chan error, 1)
	outputStarted, closeStarted := false, false
	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(release) }) }
	t.Cleanup(func() {
		cancel()
		unblock()
		if outputStarted {
			if e := <-done; !errors.Is(e, context.Canceled) {
				t.Error(e)
			}
		}
		if closeStarted {
			if e := <-closed; e != nil {
				t.Error(e)
			}
		}
	})
	outputStarted = true
	go func() {
		d, e := c.prepareLibraryBundle(ctx, ops)
		if d != nil {
			_ = d.Close()
			e = errors.New("unexpected successful held download")
		}
		done <- e
	}()
	bundleWaitEntered(t, entered, done)
	closeStarted = true
	go func() { closed <- c.Close() }()
	cancel()
	select {
	case e := <-done:
		done <- e
		t.Fatal("returned before actual output", e)
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case e := <-closed:
		closed <- e
		t.Fatal("Core.Close returned before actual output", e)
	case <-time.After(30 * time.Millisecond):
	}
	if _, e := c.store.LibrarySnapshot(context.Background()); e != nil {
		t.Fatal("Store closed before output join", e)
	}
	unblock()
}

func TestCoreLibraryBundleHeldInputClose(t *testing.T) {
	source := bundleController(t, false)
	data := preparedCoreFixture(t, source)
	c := bundleController(t, true)
	a := libraryUploaded(t, c, libraryPNG(t, 41))
	oldStore := c.store
	reader := &libraryHeldReader{entered: make(chan struct{}), release: make(chan struct{}), reader: bytes.NewReader(data)}
	ctx, cancel := context.WithCancel(context.Background())
	done, closed := make(chan error, 1), make(chan error, 1)
	restoreStarted, closeStarted := false, false
	var once sync.Once
	unblock := func() { once.Do(func() { close(reader.release) }) }
	t.Cleanup(func() {
		cancel()
		unblock()
		if restoreStarted {
			if e := <-done; !errors.Is(e, context.Canceled) {
				t.Error(e)
			}
		}
		if closeStarted {
			if e := <-closed; e != nil {
				t.Error(e)
			}
		}
	})
	restoreStarted = true
	go func() { _, e := c.RestoreLibraryBundle(ctx, reader); done <- e }()
	bundleWaitEntered(t, reader.entered, done)
	closeStarted = true
	go func() { closed <- c.Close() }()
	select {
	case e := <-closed:
		closed <- e
		t.Fatal("Close canceled or returned before caller/reader", e)
	case <-time.After(30 * time.Millisecond):
	}
	if _, e := oldStore.LibraryAsset(context.Background(), c.scope, a.ID); e != nil {
		t.Fatal("Store closed before input returned", e)
	}
	cancel()
	select {
	case e := <-done:
		done <- e
		t.Fatal("cancellation detached reader", e)
	case <-time.After(30 * time.Millisecond):
	}
	unblock()
}

type bundleCountReader struct{ reads atomic.Int64 }

func (r *bundleCountReader) Read([]byte) (int, error) { r.reads.Add(1); return 0, io.EOF }

func TestCoreLibraryBundleActualGenerationInvalidInput(t *testing.T) {
	c := bundleController(t, true)
	a := libraryUploaded(t, c, libraryPNG(t, 42))
	oldStore := c.store
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	c.channel = &fakeChannel{poll: func(ctx context.Context, _ string) ([]byte, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return nil, ctx.Err()
	}, send: func(context.Context, string, string, string, string) error {
		t.Error("unexpected synthetic send")
		return ErrUnavailable
	}}
	c.mu.Lock()
	c.providerReady = true
	c.parent = context.Background()
	c.mu.Unlock()
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); c.stopGeneration() })
	if e := c.startGeneration(); e != nil {
		t.Fatal(e)
	}
	waitSignal(t, entered)
	reader := &bundleCountReader{}
	done := make(chan error, 1)
	t.Cleanup(func() {
		unblock()
		if e := <-done; e == nil {
			t.Error("invalid restore succeeded")
		}
	})
	go func() { _, e := c.RestoreLibraryBundle(context.Background(), reader); done <- e }()
	waitSignal(t, canceled)
	if reader.reads.Load() != 0 {
		t.Fatal("input read before actual generation joined")
	}
	select {
	case e := <-done:
		done <- e
		t.Fatal("restore returned before generation join", e)
	case <-time.After(30 * time.Millisecond):
	}
	unblock()
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.generation == nil })
	if c.store != oldStore || c.paused {
		t.Fatal("preparation refusal changed original DB/pause")
	}
	bundlePreview(t, c, a, libraryPNG(t, 42))
}

func TestCoreLibraryBundleInstallLockFailure(t *testing.T) {
	source := bundleController(t, true)
	libraryUploaded(t, source, libraryPNG(t, 43))
	data := preparedCoreFixture(t, source)
	c := bundleController(t, true)
	old := libraryUploaded(t, c, libraryPNG(t, 44))
	oldStore := c.store
	if e := c.lock.Close(); e != nil {
		t.Fatal(e)
	}
	result, e := c.RestoreLibraryBundle(context.Background(), bytes.NewReader(data))
	if !errors.Is(e, ErrMaintenance) || !errors.Is(e, library.ErrRecovery) || result != (maintenance.RestoreResult{}) || c.paused || c.unavailable || c.store == nil || c.store == oldStore {
		t.Fatal("actual failed install state/result", result, e)
	}
	bundlePreview(t, c, old, libraryPNG(t, 44))
	b := libraryPNG(t, 45)
	if _, _, e := c.LibraryUpload(context.Background(), bytes.NewReader(b), int64(len(b)), storage.LibraryMetadata{}); !errors.Is(e, library.ErrRecovery) {
		t.Fatal("ambiguous incoming orphan silently adopted", e)
	}
}

func TestCoreLibraryBundleMediaBeforeExclusiveLease(t *testing.T) {
	source := bundleController(t, true)
	libraryUploaded(t, source, libraryPNG(t, 46))
	data := preparedCoreFixture(t, source)
	c := bundleController(t, true)
	p, ok := c.images.(*attachment.ImageProcessor)
	if !ok {
		t.Fatal("actual shared processor missing")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	heldDone, restoreDone := make(chan error, 1), make(chan error, 1)
	heldStarted, restoreStarted := false, false
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() {
		unblock()
		if heldStarted {
			if e := <-heldDone; e != nil {
				t.Error(e)
			}
		}
		if restoreStarted {
			if e := <-restoreDone; e != nil {
				t.Error(e)
			}
		}
	})
	b := libraryGIF(t)
	heldStarted = true
	go func() {
		heldDone <- p.WithLocalImage(context.Background(), bytes.NewReader(b), int64(len(b)), func(attachment.LocalImage, attachment.Artifact) error { close(entered); <-release; return nil })
	}()
	bundleWaitEntered(t, entered, heldDone)
	reader := &libraryHeldReader{entered: make(chan struct{}), release: make(chan struct{}), reader: bytes.NewReader(data)}
	close(reader.release)
	restoreStarted = true
	go func() { _, e := c.RestoreLibraryBundle(context.Background(), reader); restoreDone <- e }()
	bundleWaitEntered(t, reader.entered, restoreDone)
	// While incoming full-media validation waits on the persistent cap1 processor,
	// a Core write lease must remain usable; no exclusive lease surrounds it.
	select {
	case e := <-restoreDone:
		restoreDone <- e
		t.Fatal("restore bypassed shared media admission", e)
	case <-time.After(30 * time.Millisecond):
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, releaseLease, e := c.lease(ctx, true, false)
	if e != nil {
		t.Fatal("exclusive lease held before media completed", e)
	}
	releaseLease()
	unblock()
}

func TestCoreLibraryBundleCanceledFinalization(t *testing.T) {
	for _, stage := range []string{"sync", "close", "check", "open"} {
		t.Run(stage, func(t *testing.T) {
			c := bundleController(t, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ops := defaultBundleOutputOperations()
			switch stage {
			case "sync":
				original := ops.sync
				ops.sync = func(f *os.File) error { e := original(f); cancel(); return e }
			case "close":
				original := ops.close
				ops.close = func(f *os.File) error { e := original(f); cancel(); return e }
			case "check":
				original := ops.check
				ops.check = func(path string, n int64) error { e := original(path, n); cancel(); return e }
			case "open":
				original := ops.open
				ops.open = func(path string, n int64) (*os.File, error) { f, e := original(path, n); cancel(); return f, e }
			}
			d, e := c.prepareLibraryBundle(ctx, ops)
			if d != nil || !errors.Is(e, context.Canceled) {
				t.Fatal("late cancellation lost", e)
			}
			paths, e := filepath.Glob(filepath.Join(c.cfg.DataDir, ".miskoai-download-*"))
			if e != nil || len(paths) != 0 || len(c.downloads) != 0 {
				t.Fatal("late cancel cleanup/slot", paths, e)
			}
		})
	}
}

func TestCoreLibraryBundleCanceledFinalizationFailure(t *testing.T) {
	for _, stage := range []string{"sync", "close", "check", "open"} {
		t.Run(stage, func(t *testing.T) {
			c := bundleController(t, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ops := defaultBundleOutputOperations()
			raw := errors.New("private-error-canary")
			switch stage {
			case "sync":
				original := ops.sync
				ops.sync = func(f *os.File) error {
					if e := original(f); e != nil {
						return e
					}
					cancel()
					return raw
				}
			case "close":
				original := ops.close
				ops.close = func(f *os.File) error {
					if e := original(f); e != nil {
						return e
					}
					cancel()
					return raw
				}
			case "check":
				ops.check = func(string, int64) error { cancel(); return raw }
			case "open":
				ops.open = func(string, int64) (*os.File, error) { cancel(); return nil, raw }
			}
			d, e := c.prepareLibraryBundle(ctx, ops)
			if d != nil || !errors.Is(e, context.Canceled) || bytes.Contains([]byte(e.Error()), []byte("canary")) {
				t.Fatal("cancellation lost when finalization also failed", e)
			}
		})
	}
}

type bundleInspectProcessor struct {
	p     *attachment.ImageProcessor
	c     *Controller
	calls int
}

func (p *bundleInspectProcessor) WithImage(context.Context, storage.Attachment, func(provider.ContentPart) error) error {
	return attachment.ErrDownload
}
func (p *bundleInspectProcessor) WithLocalImage(ctx context.Context, r io.Reader, n int64, consume func(attachment.LocalImage, attachment.Artifact) error) error {
	p.calls++
	p.c.mu.Lock()
	held, exclusive := p.c.leases, p.c.exclusive
	p.c.mu.Unlock()
	if held != 0 || exclusive {
		return errors.New("Core Store lease surrounds media validation")
	}
	return p.p.WithLocalImage(ctx, r, n, consume)
}
func TestCoreLibraryBundleEveryMediaGateReleasesLease(t *testing.T) {
	source := bundleController(t, true)
	libraryUploaded(t, source, libraryPNG(t, 47))
	data := preparedCoreFixture(t, source)
	cfg := fixtureConfig(t)
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	cdn, e := media.NewCDN()
	if e != nil {
		t.Fatal(e)
	}
	if e := cdn.Close(); e != nil {
		t.Fatal(e)
	}
	actual, e := attachment.NewImageProcessor(cfg.DataDir, cdn)
	if e != nil {
		t.Fatal(e)
	}
	observing := &bundleInspectProcessor{p: actual}
	refuse := modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		t.Error("external model called")
		return provider.Reply{}, ErrUnavailable
	})
	c, e := New(cfg, Dependencies{Model: refuse, Vision: refuse, Images: observing, ChannelFactory: func(config.Authorization) (Channel, error) {
		return &fakeChannel{poll: idle, send: func(context.Context, string, string, string, string) error {
			t.Error("unexpected send")
			return ErrUnavailable
		}}, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	observing.c = c
	t.Cleanup(func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	})
	libraryUploaded(t, c, libraryPNG(t, 48))
	observing.calls = 0
	if _, e := c.RestoreLibraryBundle(context.Background(), bytes.NewReader(data)); e != nil {
		t.Fatal("restore media lease order", e)
	}
	if observing.calls < 5 {
		t.Fatal("actual preparation/install gates were not reached", observing.calls)
	}
	d, e := c.PrepareLibraryBundle(context.Background())
	if e != nil {
		t.Fatal("export media lease order", e)
	}
	bundleRead(t, d)
}

func TestCoreLibraryBundleLegacyEmptyImport(t *testing.T) {
	source := bundleController(t, false)
	data := preparedCoreFixture(t, source)
	var raw []byte
	h, e := librarybundle.Decode(context.Background(), bytes.NewReader(data), func(r librarybundle.Record, in io.Reader) error { var e error; raw, e = io.ReadAll(in); return e })
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(source.cfg.DataDir, "synthetic-legacy.db")
	f, e := privatefs.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := f.Write(raw); e != nil {
		t.Fatal(e)
	}
	if e := f.Close(); e != nil {
		t.Fatal(e)
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec("DROP TABLE library_assets; PRAGMA user_version=4"); e != nil {
		_ = db.Close()
		t.Fatal(e)
	}
	if e := db.Close(); e != nil {
		t.Fatal(e)
	}
	raw, e = os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	h.Database = librarybundle.File{Bytes: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])}
	var wire bytes.Buffer
	e = librarybundle.Encode(context.Background(), &wire, h, func(librarybundle.Record) (io.Reader, func() error, error) {
		return bytes.NewReader(raw), func() error { return nil }, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	c := bundleController(t, false)
	result, e := c.RestoreLibraryBundle(context.Background(), bytes.NewReader(wire.Bytes()))
	if e != nil || !result.PreviousRetained || !result.ChannelPaused {
		t.Fatal("legacy import", result, e)
	}
	rows, e := c.store.LibrarySnapshot(context.Background())
	if e != nil || len(rows) != 0 {
		t.Fatal("migrated legacy rows", rows, e)
	}
}

func TestCoreLibraryBundleLeaseOrdering(t *testing.T) {
	source := bundleController(t, true)
	libraryUploaded(t, source, libraryPNG(t, 33))
	data := preparedCoreFixture(t, source)
	dest := bundleController(t, true)
	api := coreBundleMethods(t, dest)
	// A real Manager admission holds no Core lease. The generation callback tries
	// the same Manager after cancellation; restore must cancel/join before waiting.
	entered, release, heldDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() {
		unblock()
		if e := <-heldDone; e != nil {
			t.Error(e)
		}
	})
	go func() {
		heldDone <- dest.library.WithMaintenance(context.Background(), func(context.Context, *library.Maintenance) error { close(entered); <-release; return nil })
	}()
	<-entered
	genCtx, cancel := context.WithCancel(context.Background())
	g := &generation{cancel: cancel, done: make(chan struct{})}
	dest.mu.Lock()
	dest.generation = g
	dest.mu.Unlock()
	go func() {
		defer close(g.done)
		<-genCtx.Done()
		_ = dest.library.WithMaintenance(genCtx, func(context.Context, *library.Maintenance) error { return nil })
	}()
	done := make(chan error, 1)
	t.Cleanup(func() {
		unblock()
		if e := <-done; e != nil {
			t.Error(e)
		}
	})
	go func() { _, e := api.RestoreLibraryBundle(context.Background(), bytes.NewReader(data)); done <- e }()
	select {
	case <-g.done:
	case <-time.After(2 * time.Second):
		t.Fatal("generation was not joined before Manager admission")
	}
	dest.mu.Lock()
	held, exclusive := dest.leases, dest.exclusive
	dest.mu.Unlock()
	if held != 0 || exclusive {
		t.Fatal("Store lease held waiting Manager")
	}
	unblock()
}
