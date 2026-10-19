package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/storage"
)

func TestBorrowedStoreActualUse(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	if e := f.s.UpdateLibraryAsset(context.Background(), scope, a.ID, storage.LibraryMetadata{Enabled: true}); e != nil {
		t.Fatal(e)
	}
	leases, held, callbacks := 0, 0, 0
	l := func(context.Context) (*storage.Store, func(), error) {
		leases++
		held++
		return f.s, func() { held-- }, nil
	}
	var borrowed attachment.Artifact
	e := f.m.WithAssetStore(context.Background(), l, scope, a.ID, false, func(s *storage.Store, fresh storage.LibraryAsset, im attachment.LocalImage, art attachment.Artifact) error {
		callbacks++
		borrowed = art
		if s != f.s || leases != 2 || held != 1 || fresh.ID != a.ID || im.SHA256 != a.SHA256 {
			t.Error("callback did not receive current held Store and validated content")
		}
		return s.RecordLibraryUse(context.Background(), scope, fresh.ID, 1234)
	})
	if e != nil || callbacks != 1 || leases != 2 || held != 0 {
		t.Fatalf("borrow callback: err=%v callbacks=%d leases=%d held=%d", e, callbacks, leases, held)
	}
	fresh, e := f.s.LibraryAsset(context.Background(), scope, a.ID)
	if e != nil || fresh.Uses != 1 || fresh.LastUsedMillis != 1234 {
		t.Fatalf("actual scoped Store update: %+v %v", fresh, e)
	}
	if _, e := borrowed.ReadAt(make([]byte, 1), 0); e == nil {
		t.Fatal("artifact survived synchronous callback")
	}
}

type borrowedCloseArtifact struct {
	attachment.Artifact
	closes    int
	closeHook func()
}

func (a *borrowedCloseArtifact) Close() error {
	a.closes++
	if a.closeHook != nil {
		a.closeHook()
	}
	return a.Artifact.Close()
}

func TestBorrowedStoreActualCloseOnceCancelAndFailure(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	held, releases := 0, 0
	l := func(context.Context) (*storage.Store, func(), error) {
		held++
		return f.s, func() { held--; releases++ }, nil
	}
	var original *borrowedCloseArtifact
	f.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) (err error) {
		stage, e := media.StageLocal(c, f.dir, r, n)
		if e != nil {
			return e
		}
		original = &borrowedCloseArtifact{Artifact: stage, closeHook: func() {
			if held != 0 || releases != 2 {
				t.Error("Store not released before actual Close")
			}
			cancel()
		}}
		defer func() {
			ce := original.Close()
			err = errors.Join(err, c.Err(), ce, attachment.ErrCleanup) // injected delegate cleanup failure, actual file still closes.
		}()
		info, e := media.ValidateImage(c, original, n)
		if e != nil {
			return e
		}
		h := sha256.New()
		if _, e := io.Copy(h, io.NewSectionReader(original, 0, n)); e != nil {
			return e
		}
		return fn(attachment.LocalImage{Info: info, SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n}, original)
	})
	e := f.m.WithAssetStore(ctx, l, scope, a.ID, false, func(_ *storage.Store, _ storage.LibraryAsset, _ attachment.LocalImage, art attachment.Artifact) error {
		if art != original {
			t.Error("original artifact substituted")
		}
		return nil
	})
	if !errors.Is(e, context.Canceled) || !errors.Is(e, ErrCleanup) || original == nil || original.closes != 1 {
		t.Fatalf("actual cleanup: %v %+v", e, original)
	}
	if _, re := original.ReadAt(make([]byte, 1), 0); re == nil {
		t.Fatal("actual original remains open")
	}
}

func TestBorrowedStoreRefusalsAndFreshMetadata(t *testing.T) {
	for _, name := range []string{"scope", "disabled", "fresh-disabled", "fresh-metadata", "fresh-store", "stale-digest", "lease-refusal", "callback-refusal", "cancel-release"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			a := upload(t, f, "png")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls, callbacks, releases := 0, 0, 0
			var alternate *storage.Store
			if name == "stale-digest" || name == "fresh-store" {
				other := setup(t)
				seed := uint8(20)
				if name == "fresh-store" {
					seed = 10
				}
				raw := rawImage(t, "png", seed)
				b, _, e := other.m.Upload(ctx, other.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
				if e != nil || b.ID != a.ID || (b.SHA256 == a.SHA256) != (name == "fresh-store") {
					t.Fatal("stale fixture", e)
				}
				alternate = other.s
			}
			sc, enabled := scope, false
			want := error(nil)
			if name == "scope" {
				sc.User = "foreign"
				want = storage.ErrNotFound
			}
			if name == "disabled" {
				enabled = true
				want = ErrContent
			}
			if name == "fresh-disabled" {
				enabled = true
				want = ErrContent
				if e := f.s.UpdateLibraryAsset(ctx, scope, a.ID, storage.LibraryMetadata{Enabled: true}); e != nil {
					t.Fatal(e)
				}
			}
			l := func(context.Context) (*storage.Store, func(), error) {
				calls++
				if calls == 2 {
					switch name {
					case "fresh-disabled", "fresh-metadata":
						if e := f.s.UpdateLibraryAsset(ctx, scope, a.ID, storage.LibraryMetadata{Description: "fresh"}); e != nil {
							return nil, nil, e
						}
					case "stale-digest", "fresh-store":
						// A distinct current Store has the same row ID and scope but other bytes.
						return alternate, func() { releases++ }, nil
					case "lease-refusal":
						return nil, func() { releases++ }, storage.ErrStale
					}
				}
				return f.s, func() {
					releases++
					if calls == 2 && name == "cancel-release" {
						cancel()
					}
				}, nil
			}
			if name == "stale-digest" {
				want = ErrRecovery
			}
			if name == "lease-refusal" {
				want = storage.ErrStale
			}
			if name == "callback-refusal" {
				want = ErrContent
			}
			if name == "cancel-release" {
				want = context.Canceled
			}
			e := f.m.WithAssetStore(ctx, l, sc, a.ID, enabled, func(s *storage.Store, fresh storage.LibraryAsset, _ attachment.LocalImage, _ attachment.Artifact) error {
				callbacks++
				if name == "fresh-store" && s != alternate {
					t.Error("lookup Store lent instead of fresh held Store")
				}
				if name == "fresh-metadata" && fresh.Description != "fresh" {
					t.Error("stale metadata lent")
				}
				if name == "callback-refusal" {
					return errors.New("synthetic private callback detail")
				}
				return nil
			})
			if want == nil && e != nil || want != nil && !errors.Is(e, want) {
				t.Fatalf("err=%v want=%v", e, want)
			}
			if e != nil && strings.Contains(e.Error(), "synthetic private") {
				t.Fatal("callback detail leaked")
			}
			wantCallbacks := 0
			if name == "fresh-metadata" || name == "fresh-store" || name == "callback-refusal" || name == "cancel-release" {
				wantCallbacks = 1
			}
			if callbacks != wantCallbacks || releases != calls {
				t.Fatalf("callbacks=%d/%d release=%d calls=%d", callbacks, wantCallbacks, releases, calls)
			}
		})
	}
}

func TestBorrowedStoreInvalidBeforeAdmission(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	f.m.token <- struct{}{}
	defer func() { <-f.m.token }()
	calls := 0
	l := func(context.Context) (*storage.Store, func(), error) { calls++; return f.s, func() {}, nil }
	fn := func(*storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		return nil
	}
	for _, e := range []error{
		f.m.WithAssetStore(context.Background(), l, scope, a.ID, false, nil),
		f.m.WithAssetStore(context.Background(), nil, scope, a.ID, false, fn),
		f.m.WithAssetStore(context.Background(), l, storage.Scope{}, a.ID, false, fn),
		f.m.WithAssetStore(context.Background(), l, scope, 0, false, fn),
		f.m.WithAssetStore(nil, l, scope, a.ID, false, fn),
	} {
		if !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
	if calls != 0 {
		t.Fatal("invalid arguments acquired lease")
	}
}

func borrowedSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("signal timeout")
	}
}

func TestBorrowedStoreHeldCallbackCloseAndMedia(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	entered, allow := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(allow) }) }
	op, closed, mediaDone := make(chan error, 1), make(chan error, 1), make(chan error, 1)
	finished := make(chan struct{})
	t.Cleanup(func() { unblock(); borrowedSignal(t, finished) })
	go func() {
		op <- f.m.WithAssetStore(context.Background(), f.l, scope, a.ID, false, func(*storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
			close(entered)
			<-allow
			return nil
		})
		close(finished)
	}()
	borrowedSignal(t, entered)
	waitCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	maintenanceCalls := 0
	e := f.m.WithMaintenance(waitCtx, func(context.Context, *Maintenance) error { maintenanceCalls++; return nil })
	if !errors.Is(e, context.DeadlineExceeded) || maintenanceCalls != 0 {
		t.Fatalf("maintenance escaped: %v", e)
	}
	raw := rawImage(t, "png", 11)
	mediaFinished, closeFinished := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { unblock(); borrowedSignal(t, mediaFinished); borrowedSignal(t, closeFinished) })
	go func() {
		mediaDone <- f.p.WithLocalImage(context.Background(), bytes.NewReader(raw), int64(len(raw)), func(attachment.LocalImage, attachment.Artifact) error { return nil })
		close(mediaFinished)
	}()
	assertHeld(t, mediaDone)
	go func() { closed <- f.m.Close(); close(closeFinished) }()
	borrowedSignal(t, f.m.stop)
	assertHeld(t, closed)
	assertHeld(t, op)
	unblock()
	if e := await(t, op); !errors.Is(e, context.Canceled) {
		t.Fatal("Close cancellation", e)
	}
	if e := await(t, closed); e != nil {
		t.Fatal(e)
	}
	if e := await(t, mediaDone); e != nil {
		t.Fatal(e)
	}
	if e := f.m.WithAssetStore(context.Background(), f.l, scope, a.ID, false, func(*storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		return nil
	}); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
}

func TestBorrowedStoreLeaseReleasedBeforeActualArtifactCleanup(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	held, releases, callbacks := 0, 0, 0
	l := func(context.Context) (*storage.Store, func(), error) {
		held++
		return f.s, func() { held--; releases++ }, nil
	}
	var original attachment.Artifact
	f.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
		e := f.p.WithLocalImage(c, r, n, func(im attachment.LocalImage, art attachment.Artifact) error {
			original = art
			result := fn(im, art)
			if held != 0 || releases != 2 {
				t.Error("Store release did not precede actual Close")
			}
			if _, re := art.ReadAt(make([]byte, 1), 0); re != nil {
				t.Error("artifact closed before Store release", re)
			}
			return result
		})
		if _, re := original.ReadAt(make([]byte, 1), 0); re == nil {
			t.Error("actual artifact cleanup absent")
		}
		cancel() // cancellation after actual cleanup must still reach the caller.
		return e
	})
	e := f.m.WithAssetStore(ctx, l, scope, a.ID, false, func(*storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		callbacks++
		return nil
	})
	if !errors.Is(e, context.Canceled) || callbacks != 1 || held != 0 || releases != 2 {
		t.Fatalf("cleanup cancellation: %v %d %d %d", e, callbacks, held, releases)
	}
}
