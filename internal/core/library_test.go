package core

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

func libraryController(t *testing.T, configured bool) *Controller {
	t.Helper()
	cfg := fixtureConfig(t)
	if configured {
		writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	}
	c, e := New(cfg, Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	})
	return c
}
func libraryPNG(t *testing.T, v byte) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.NRGBA{R: v, A: 255})
	var b bytes.Buffer
	if e := png.Encode(&b, im); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func libraryGIF(t *testing.T) []byte {
	t.Helper()
	im := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	im.SetColorIndex(0, 0, 1)
	var b bytes.Buffer
	if e := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{im, im}, Delay: []int{1, 1}}); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func libraryUploaded(t *testing.T, c *Controller, b []byte) storage.LibraryAsset {
	t.Helper()
	a, created, e := c.LibraryUpload(context.Background(), bytes.NewReader(b), int64(len(b)), storage.LibraryMetadata{Tags: []string{"synthetic"}, Description: "fixture"})
	if e != nil || !created {
		t.Fatal("upload", a, created, e)
	}
	return a
}
func TestCoreLibraryOfflineCRUD(t *testing.T) {
	c := libraryController(t, true)
	ctx := context.Background()
	if c.providerReady {
		t.Fatal("fixture unexpectedly provider ready")
	}
	p, ok := c.images.(*attachment.ImageProcessor)
	if !ok || c.documents != p {
		t.Fatal("default media sharing")
	}
	m, sc, e := c.libraryAccess(ctx)
	if e != nil || m == nil || sc != c.scope {
		t.Fatal("manager composition", e)
	}
	if reflect.ValueOf(m).Elem().FieldByName("processor").Elem().Pointer() != reflect.ValueOf(p).Pointer() {
		t.Fatal("second processor")
	}
	for _, b := range [][]byte{libraryPNG(t, 3), libraryGIF(t)} {
		a := libraryUploaded(t, c, b)
		duplicate, created, e := c.LibraryUpload(ctx, bytes.NewReader(b), int64(len(b)), storage.LibraryMetadata{Description: "do not overwrite"})
		if e != nil || created || duplicate.ID != a.ID || duplicate.Description != "fixture" {
			t.Fatal("duplicate", duplicate, created, e)
		}
		if e := c.WithLibraryAsset(ctx, a.ID, true, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
			t.Error("disabled consumer")
			return nil
		}); !errors.Is(e, library.ErrContent) {
			t.Fatal("disabled", e)
		}
		var retained attachment.Artifact
		if e := c.WithLibraryAsset(ctx, a.ID, false, func(row storage.LibraryAsset, im attachment.LocalImage, art attachment.Artifact) error {
			retained = art
			actual := make([]byte, len(b))
			n, e := art.ReadAt(actual, 0)
			if e != nil || n != len(b) || !bytes.Equal(actual, b) || row.SHA256 != im.SHA256 {
				t.Error("preview content", n, e)
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
		if _, e := retained.ReadAt(make([]byte, 1), 0); e == nil {
			t.Fatal("temporary retained after consumer")
		}
		if e := c.LibraryUpdate(ctx, a.ID, storage.LibraryMetadata{Enabled: true, Description: "edited"}); e != nil {
			t.Fatal(e)
		}
		if e := c.LibraryRecordUse(ctx, a.ID, a.SHA256, 1234); e != nil {
			t.Fatal(e)
		}
		rows, e := c.LibraryAssets(ctx, 0, 32)
		if e != nil || len(rows) != 1 || rows[0].Uses != 1 || !rows[0].Enabled || rows[0].Description != "edited" {
			t.Fatal(rows, e)
		}
		if e := c.LibraryDelete(ctx, a.ID); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := c.LibraryAssets(ctx, 0, 32)
	if e != nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	if e := m.WithAsset(ctx, c.libraryLease(sc), sc, 1, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error { return nil }); !errors.Is(e, library.ErrClosed) {
		t.Fatal("owned manager not closed", e)
	}
	b := libraryPNG(t, 7)
	if e := p.WithLocalImage(ctx, bytes.NewReader(b), int64(len(b)), func(attachment.LocalImage, attachment.Artifact) error { return nil }); e != nil {
		t.Fatal("borrowed processor closed", e)
	}
}
func TestCoreLibraryUnavailableAndInputs(t *testing.T) {
	c := libraryController(t, false)
	ctx := context.Background()
	if _, e := c.LibraryAssets(ctx, 0, 1); !errors.Is(e, ErrUnconfigured) {
		t.Fatal(e)
	}
	if _, e := c.LibraryAssets(nil, 0, 1); !errors.Is(e, ErrConfiguration) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := c.LibraryAssets(canceled, 0, 1); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	c2 := libraryController(t, true)
	if _, e := c2.LibraryAssets(ctx, -1, 1); !errors.Is(e, storage.ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := c2.LibraryAssets(ctx, 0, 33); !errors.Is(e, storage.ErrInvalid) {
		t.Fatal(e)
	}
	if _, _, e := c2.LibraryUpload(ctx, nil, 1, storage.LibraryMetadata{}); !errors.Is(e, library.ErrInvalid) {
		t.Fatal(e)
	}
	if e := c2.WithLibraryAsset(ctx, 1, false, nil); !errors.Is(e, library.ErrInvalid) {
		t.Fatal(e)
	}
	if e := c2.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := c2.LibraryAssets(ctx, 0, 1); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	if e := c2.LibraryDelete(ctx, 1); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	cfg := fixtureConfig(t)
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	model := modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		t.Error("model called")
		return provider.Reply{}, nil
	})
	legacy, e := New(cfg, Dependencies{Model: model, Vision: model, Images: imageProcessorFunc(func(context.Context, storage.Attachment, func(provider.ContentPart) error) error {
		t.Error("legacy media called")
		return nil
	}), ChannelFactory: func(config.Authorization) (Channel, error) { return &fakeChannel{poll: idle}, nil }})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = legacy.Close() })
	if _, e := legacy.LibraryAssets(ctx, 0, 1); !errors.Is(e, ErrUnavailable) {
		t.Fatal("legacy invented capability", e)
	}
}
func TestCoreLibraryForeignAndCapturedScope(t *testing.T) {
	c := libraryController(t, true)
	ctx := context.Background()
	a := libraryUploaded(t, c, libraryPNG(t, 4))
	c.mu.Lock()
	old := c.scope
	c.scope = storage.Scope{Account: "other-account", User: "other-user"}
	c.mu.Unlock()
	for _, id := range []int64{a.ID, math.MaxInt64} {
		if e := c.WithLibraryAsset(ctx, id, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
			t.Error("foreign consumer")
			return nil
		}); !errors.Is(e, storage.ErrNotFound) {
			t.Fatal(e)
		}
		if e := c.LibraryDelete(ctx, id); !errors.Is(e, storage.ErrNotFound) {
			t.Fatal(e)
		}
	}
	c.mu.Lock()
	c.scope = old
	c.mu.Unlock()
}
func TestCoreLibraryCloseJoinsConsumer(t *testing.T) {
	c := libraryController(t, true)
	a := libraryUploaded(t, c, libraryPNG(t, 8))
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	done, closed := make(chan error, 1), make(chan error, 1)
	t.Cleanup(func() {
		unblock()
		if e := <-done; e != nil && !errors.Is(e, context.Canceled) {
			t.Error("held operation", e)
		}
		if e := <-closed; e != nil {
			t.Error("joined Close", e)
		}
	})
	store := c.store
	go func() {
		done <- c.WithLibraryAsset(ctx, a.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
			close(entered)
			<-release
			_, e := store.LibraryAsset(ctx, c.scope, a.ID)
			return e
		})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		unblock()
		go func() { closed <- c.Close() }()
		t.Fatal("consumer did not enter")
	}
	go func() { closed <- c.Close() }()
	libraryWaitClosing(t, c)
	select {
	case e := <-closed:
		closed <- e
		t.Fatal("Close returned before actual consumer", e)
	case <-time.After(30 * time.Millisecond):
	}
	if _, e := store.LibraryAsset(ctx, c.scope, a.ID); e != nil {
		t.Fatal("store closed while held", e)
	}
	unblock()
}
func TestCoreLibraryRestoreFreshStore(t *testing.T) {
	c := libraryController(t, true)
	ctx := context.Background()
	old := libraryUploaded(t, c, libraryPNG(t, 10))
	manager := c.library
	source := libraryController(t, true)
	next := libraryUploaded(t, source, libraryPNG(t, 11))
	if next.ID != old.ID || next.SHA256 == old.SHA256 {
		t.Fatal("restore fixture IDs/digests")
	}
	if e := source.LibraryUpdate(ctx, next.ID, storage.LibraryMetadata{Enabled: true}); e != nil {
		t.Fatal(e)
	}
	download, e := source.PrepareBackup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(download)
	ce := download.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	original, e := os.ReadDir(filepath.Join(c.cfg.DataDir, "library"))
	if e != nil || len(original) != 1 {
		t.Fatal(e)
	}
	if _, e := c.Restore(ctx, bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	if c.library != manager || !c.paused {
		t.Fatal("restore lifetime/pause")
	}
	rows, e := c.LibraryAssets(ctx, 0, 32)
	if e != nil || len(rows) != 1 || rows[0].SHA256 != next.SHA256 {
		t.Fatal("stale Store", rows, e)
	}
	if e := c.LibraryRecordUse(ctx, old.ID, old.SHA256, 123); !errors.Is(e, library.ErrRecovery) {
		t.Fatal("old digest updated restore row", e)
	}
	if e := c.WithLibraryAsset(ctx, next.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		t.Error("missing restored content lent")
		return nil
	}); e == nil {
		t.Fatal("missing content accepted")
	}
	if e := c.LibraryRecordUse(ctx, next.ID, next.SHA256, 124); e != nil {
		t.Fatal("fresh lease", e)
	}
	retained, e := os.ReadDir(filepath.Join(c.cfg.DataDir, "library"))
	if e != nil || len(retained) != 1 || retained[0].Name() != original[0].Name() {
		t.Fatal("restore swept original", e)
	}
}

type libraryHeldReader struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	reader  *bytes.Reader
}

func (r *libraryHeldReader) Read(b []byte) (int, error) {
	r.once.Do(func() { close(r.entered); <-r.release })
	return r.reader.Read(b)
}
func TestCoreLibraryCloseJoinsUploadReader(t *testing.T) {
	c := libraryController(t, true)
	b := libraryPNG(t, 20)
	r := &libraryHeldReader{entered: make(chan struct{}), release: make(chan struct{}), reader: bytes.NewReader(b)}
	done, closed := make(chan error, 1), make(chan error, 1)
	store := c.store
	var once sync.Once
	unblock := func() { once.Do(func() { close(r.release) }) }
	t.Cleanup(func() {
		unblock()
		e := <-done
		if e == nil {
			t.Error("closed upload succeeded")
		}
		if e := <-closed; e != nil {
			t.Error(e)
		}
	})
	go func() {
		_, _, e := c.LibraryUpload(context.Background(), r, int64(len(b)), storage.LibraryMetadata{})
		done <- e
	}()
	select {
	case <-r.entered:
	case <-time.After(5 * time.Second):
		go func() { closed <- c.Close() }()
		t.Fatal("reader not entered")
	}
	go func() { closed <- c.Close() }()
	libraryWaitClosing(t, c)
	select {
	case e := <-closed:
		closed <- e
		t.Fatal("Close returned before actual upload reader", e)
	case <-time.After(30 * time.Millisecond):
	}
	if _, e := store.LibraryAssets(context.Background(), c.scope, 0, 1); e != nil {
		t.Fatal("store closed before actual reader joined", e)
	}
	unblock()
}

func TestCoreLibraryCapturedScopeRecheck(t *testing.T) {
	c := libraryController(t, true)
	ctx := context.Background()
	original := c.scope
	lease := c.libraryLease(original)
	c.mu.Lock()
	c.scope = storage.Scope{Account: "other-account", User: "other-user"}
	c.mu.Unlock()
	s, release, e := lease(ctx)
	if release != nil {
		release()
	}
	if s == nil || release == nil || !errors.Is(e, storage.ErrStale) {
		t.Fatal("captured scope reused", e)
	}
	c.mu.Lock()
	held := c.leases
	c.scope = original
	c.mu.Unlock()
	if held != 0 {
		t.Fatal("scope mismatch leaked lease")
	}
	a := libraryUploaded(t, c, libraryPNG(t, 21))
	if e := c.WithLibraryAsset(ctx, a.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		return errors.New("raw-private-canary")
	}); e == nil || e.Error() == "raw-private-canary" {
		t.Fatal("consumer error unsafe", e)
	}
}

func TestCoreLibraryConstructorFailureUnwinds(t *testing.T) {
	cfg := fixtureConfig(t)
	writeFixture(t, cfg.DataDir, "library", "synthetic wrong directory type")
	if c, e := New(cfg, Dependencies{}); c != nil || !errors.Is(e, ErrUnavailable) {
		if c != nil {
			_ = c.Close()
		}
		t.Fatal("manager constructor failure", e)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, e := os.Lstat(filepath.Join(cfg.DataDir, "miskoai.db") + suffix); !os.IsNotExist(e) {
			t.Fatal("constructor retained store", suffix, e)
		}
	}
	// A failed Manager construction must still release Core's exclusive process lock.
	if _, e := os.Stat(filepath.Join(cfg.DataDir, "library")); e != nil {
		t.Fatal("existing content changed", e)
	}
	lock, e := maintenance.Acquire(cfg.DataDir)
	if e != nil {
		t.Fatal("constructor retained lock", e)
	}
	if e := lock.Close(); e != nil {
		t.Fatal(e)
	}
}

func libraryWaitClosing(t *testing.T, c *Controller) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		c.mu.Lock()
		closing := c.closing || c.closed
		c.mu.Unlock()
		if closing {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("Close did not publish its closing state")
		default:
			runtime.Gosched()
		}
	}
}

func TestCoreLibraryRestoreMissingThenVerifiedContent(t *testing.T) {
	c := libraryController(t, true)
	source := libraryController(t, true)
	ctx := context.Background()
	payload := libraryPNG(t, 30)
	a := libraryUploaded(t, source, payload)
	d, e := source.PrepareBackup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(d)
	ce := d.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	if _, e := c.Restore(ctx, bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	if e := c.WithLibraryAsset(ctx, a.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		t.Error("missing consumer")
		return nil
	}); e == nil {
		t.Fatal("missing content accepted")
	}
	files, e := os.ReadDir(filepath.Join(source.cfg.DataDir, "library"))
	if e != nil || len(files) != 1 {
		t.Fatal(e)
	}
	// This is explicit synthetic fixture placement, not an implementation of bundle recovery.
	f, e := privatefs.Create(filepath.Join(c.cfg.DataDir, "library", files[0].Name()))
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.Write(payload)
	se := f.Sync()
	ce = f.Close()
	if e != nil || se != nil || ce != nil {
		t.Fatal(e, se, ce)
	}
	called := false
	if e := c.WithLibraryAsset(ctx, a.ID, false, func(row storage.LibraryAsset, im attachment.LocalImage, art attachment.Artifact) error {
		called = true
		if row.SHA256 != a.SHA256 || im.SHA256 != a.SHA256 {
			t.Error("restored content identity")
		}
		return nil
	}); e != nil || !called {
		t.Fatal("fresh restored consumer", e)
	}
}

type libraryProbeReader struct{ reads int }

func (r *libraryProbeReader) Read([]byte) (int, error) {
	r.reads++
	return 0, errors.New("private-reader-canary")
}
func TestCoreLibrarySharedAdmissionAndCanceledInputs(t *testing.T) {
	c := libraryController(t, true)
	p := c.images.(*attachment.ImageProcessor)
	b := libraryPNG(t, 40)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() {
		unblock()
		if e := <-done; e != nil {
			t.Error(e)
		}
	})
	go func() {
		done <- p.WithLocalImage(context.Background(), bytes.NewReader(b), int64(len(b)), func(attachment.LocalImage, attachment.Artifact) error { close(entered); <-release; return nil })
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("shared local processor not entered")
	}
	// The actual existing media slot is held, so Manager upload must wait before
	// touching its caller's reader and respect the caller's shorter deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	probe := &libraryProbeReader{}
	if _, _, e := c.LibraryUpload(ctx, probe, int64(len(b)), storage.LibraryMetadata{}); !errors.Is(e, context.DeadlineExceeded) || probe.reads != 0 {
		t.Fatal("shared admission bypass", probe.reads, e)
	}
	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	for _, ctx := range []context.Context{nil, canceled} {
		if e := c.WithLibraryAsset(ctx, 1, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
			t.Error("invalid-context consumer")
			return nil
		}); e == nil {
			t.Fatal("invalid context accepted")
		}
		if _, _, e := c.LibraryUpload(ctx, probe, 1, storage.LibraryMetadata{}); e == nil || probe.reads != 0 {
			t.Fatal("invalid-context input read", e)
		}
	}
	unblock()
}
func TestCoreLibraryUnavailableBeforeContent(t *testing.T) {
	c := libraryController(t, true)
	probe := &libraryProbeReader{}
	ctx := context.Background()
	c.mu.Lock()
	c.unavailable = true
	c.mu.Unlock()
	if _, _, e := c.LibraryUpload(ctx, probe, 1, storage.LibraryMetadata{}); !errors.Is(e, ErrUnavailable) || probe.reads != 0 {
		t.Fatal(e)
	}
	if e := c.WithLibraryAsset(ctx, 1, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		t.Error("unavailable consumer")
		return nil
	}); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	c.mu.Lock()
	c.unavailable = false
	c.mu.Unlock()
	if e := c.Close(); e != nil {
		t.Fatal(e)
	}
	if _, _, e := c.LibraryUpload(ctx, probe, 1, storage.LibraryMetadata{}); !errors.Is(e, ErrUnavailable) || probe.reads != 0 {
		t.Fatal("closed input read", e)
	}
	if e := c.WithLibraryAsset(ctx, 1, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		t.Error("closed consumer")
		return nil
	}); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}
