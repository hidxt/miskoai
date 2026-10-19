package core

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
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/storage"
)

func generationLibraryFixture(t *testing.T) (*Controller, *generation, context.Context, context.CancelFunc, storage.LibraryAsset) {
	t.Helper()
	c := libraryController(t, true)
	a := libraryUploaded(t, c, libraryPNG(t, 51))
	if e := c.LibraryUpdate(context.Background(), a.ID, storage.LibraryMetadata{Enabled: true}); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	g := &generation{cancel: cancel, done: make(chan struct{})}
	c.generation = g
	c.parent = context.Background()
	// No workers: this marker's done channel is joined after each operation finishes.
	t.Cleanup(func() { cancel(); c.mu.Lock(); c.generation = nil; c.mu.Unlock() })
	return c, g, ctx, cancel, a
}

func TestGenerationLibraryCurrentBorrow(t *testing.T) {
	c, g, ctx, _, a := generationLibraryFixture(t)
	calls := 0
	var retained attachment.Artifact
	e := c.withGenerationLibraryAsset(context.Background(), ctx, g, c.store, c.scope, a.ID, a.SHA256, func(bound context.Context, s *storage.Store, row storage.LibraryAsset, im attachment.LocalImage, art attachment.Artifact) error {
		calls++
		retained = art
		c.mu.Lock()
		held := c.leases
		c.mu.Unlock()
		if s != c.store || held != 1 || row.SHA256 != a.SHA256 || im.SHA256 != a.SHA256 || bound.Err() != nil {
			t.Error("wrong current held borrow")
		}
		return s.RecordLibraryUse(bound, c.scope, a.ID, 1234)
	})
	if e != nil || calls != 1 {
		t.Fatalf("actual borrow: calls=%d err=%v", calls, e)
	}
	if _, e := retained.ReadAt(make([]byte, 1), 0); e == nil {
		t.Fatal("artifact escaped callback")
	}
	row, e := c.store.LibraryAsset(context.Background(), c.scope, a.ID)
	if e != nil || row.Uses != 1 {
		t.Fatal("actual Store use", row, e)
	}
	c.mu.Lock()
	held := c.leases
	c.mu.Unlock()
	if held != 0 {
		t.Fatal("lease leaked")
	}
}

func TestGenerationLibraryDigestAndDisabled(t *testing.T) {
	for _, name := range []string{"digest", "disabled", "unsafe-error"} {
		t.Run(name, func(t *testing.T) {
			c, g, ctx, _, a := generationLibraryFixture(t)
			digest := a.SHA256
			if name == "digest" {
				digest = strings.Repeat("0", 64)
			}
			if name == "disabled" {
				if e := c.LibraryUpdate(context.Background(), a.ID, storage.LibraryMetadata{}); e != nil {
					t.Fatal(e)
				}
			}
			calls := 0
			e := c.withGenerationLibraryAsset(context.Background(), ctx, g, c.store, c.scope, a.ID, digest, func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
				calls++
				return errors.New("private-canary")
			})
			if e == nil || strings.Contains(e.Error(), "private-canary") || (name != "unsafe-error" && calls != 0) || (name == "unsafe-error" && calls != 1) {
				t.Fatal("refusal", calls, e)
			}
		})
	}
}

func TestGenerationLibraryCheapRefusalAndLeaseIdentity(t *testing.T) {
	c, g, ctx, _, a := generationLibraryFixture(t)
	originalStore, originalScope, originalManager := c.store, c.scope, c.library
	cases := []struct {
		name    string
		mutate  func()
		restore func()
	}{
		{"stale-generation", func() { c.generation = &generation{} }, func() { c.generation = g }},
		{"stale-scope", func() { c.scope.User = "another-user" }, func() { c.scope = originalScope }},
		{"stale-store", func() { c.store = &storage.Store{} }, func() { c.store = originalStore }},
		{"unconfigured", func() { c.scope = storage.Scope{} }, func() { c.scope = originalScope }},
		{"manager-missing", func() { c.library = nil }, func() { c.library = originalManager }},
		{"exclusive", func() { c.exclusive = true }, func() { c.exclusive = false }},
		{"paused", func() { c.paused = true }, func() { c.paused = false }},
		{"expired", func() { c.expired = true }, func() { c.expired = false }},
		{"unavailable", func() { c.unavailable = true }, func() { c.unavailable = false }},
		{"closing", func() { c.closing = true }, func() { c.closing = false }},
		{"closed", func() { c.closed = true }, func() { c.closed = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lease := c.generationLibraryLease(ctx, g, originalStore, originalScope, originalManager)
			c.mu.Lock()
			tc.mutate()
			c.mu.Unlock()
			defer func() { c.mu.Lock(); tc.restore(); c.mu.Unlock() }()
			// No deadline permits a general lease wait to hide an exclusive-join deadlock.
			done := make(chan error, 1)
			go func() {
				done <- c.withGenerationLibraryAsset(context.Background(), ctx, g, originalStore, originalScope, a.ID, a.SHA256, func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
					t.Error("stale callback")
					return nil
				})
			}()
			var e error
			select {
			case e = <-done:
			case <-time.After(time.Second):
				c.mu.Lock()
				tc.restore()
				c.notifyLocked()
				c.mu.Unlock()
				e = <-done
				t.Fatal("refusal waited for maintenance", e)
			}
			if e == nil {
				t.Fatal("cheap state accepted")
			}
			s, release, e := lease(context.Background())
			if release != nil {
				release()
			}
			if e == nil || s != nil || release != nil {
				t.Fatal("lease did not independently recheck", e)
			}
		})
	}
	// Captured Manager identity is checked independently of the cheap capture.
	lease := c.generationLibraryLease(ctx, g, originalStore, originalScope, originalManager)
	c.mu.Lock()
	c.library = &library.Manager{}
	c.mu.Unlock()
	s, release, e := lease(context.Background())
	c.mu.Lock()
	c.library = originalManager
	c.mu.Unlock()
	if release != nil {
		release()
	}
	if !errors.Is(e, storage.ErrStale) || s != nil {
		t.Fatal("Manager identity", e)
	}
	// Release is idempotent and notifies the existing wait channel once.
	before := c.changed
	s, release, e = lease(context.Background())
	if e != nil || s != originalStore {
		t.Fatal(e)
	}
	release()
	release()
	select {
	case <-before:
	default:
		t.Error("release did not notify")
	}
	c.mu.Lock()
	held := c.leases
	c.mu.Unlock()
	if held != 0 {
		t.Fatal("double release", held)
	}
}

func TestGenerationLibraryInvalidAndCanceledInputs(t *testing.T) {
	c, g, ctx, cancel, a := generationLibraryFixture(t)
	fn := func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		t.Error("invalid callback")
		return nil
	}
	for _, sc := range []storage.Scope{{}, {Account: " ", User: "u"}, {Account: strings.Repeat("a", 257), User: "u"}, {Account: "a", User: "\x00"}, {Account: "a", User: string([]byte{255})}} {
		if e := c.withGenerationLibraryAsset(context.Background(), ctx, g, c.store, sc, a.ID, a.SHA256, fn); !errors.Is(e, ErrConfiguration) {
			t.Fatal(e)
		}
	}
	for _, digest := range []string{"", strings.Repeat("a", 63), strings.Repeat("A", 64), strings.Repeat("x", 64)} {
		if e := c.withGenerationLibraryAsset(context.Background(), ctx, g, c.store, c.scope, a.ID, digest, fn); !errors.Is(e, ErrConfiguration) {
			t.Fatal(e)
		}
	}
	for _, e := range []error{
		c.withGenerationLibraryAsset(nil, ctx, g, c.store, c.scope, a.ID, a.SHA256, fn),
		c.withGenerationLibraryAsset(context.Background(), nil, g, c.store, c.scope, a.ID, a.SHA256, fn),
		c.withGenerationLibraryAsset(context.Background(), ctx, nil, c.store, c.scope, a.ID, a.SHA256, fn),
		c.withGenerationLibraryAsset(context.Background(), ctx, g, nil, c.scope, a.ID, a.SHA256, fn),
		c.withGenerationLibraryAsset(context.Background(), ctx, g, c.store, c.scope, 0, a.SHA256, fn),
		c.withGenerationLibraryAsset(context.Background(), ctx, g, c.store, c.scope, a.ID, a.SHA256, nil),
	} {
		if !errors.Is(e, ErrConfiguration) {
			t.Fatal(e)
		}
	}
	parent, pcancel := context.WithCancel(context.Background())
	pcancel()
	if e := c.withGenerationLibraryAsset(parent, ctx, g, c.store, c.scope, a.ID, a.SHA256, fn); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	cancel()
	if e := c.withGenerationLibraryAsset(context.Background(), ctx, g, c.store, c.scope, a.ID, a.SHA256, fn); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

// This hook delegates actual PNG validation and the real shared cap-one media
// processor. It changes Core identity only after initial lookup but before the
// Manager reborrows Store for its consumer.
type generationLibraryProcessor struct {
	delegate library.Processor
	entry    func()
	before   func()
}

func (p generationLibraryProcessor) WithLocalImage(ctx context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
	if p.entry != nil {
		p.entry()
	}
	return p.delegate.WithLocalImage(ctx, r, n, func(im attachment.LocalImage, a attachment.Artifact) error {
		if p.before != nil {
			p.before()
		}
		return fn(im, a)
	})
}
func TestGenerationLibraryFreshLeaseAfterValidation(t *testing.T) {
	for _, name := range []string{"generation", "scope", "store", "manager", "exclusive"} {
		t.Run(name, func(t *testing.T) {
			c, g, ctx, _, a := generationLibraryFixture(t)
			original := c.library
			replacement, e := library.New(c.cfg.DataDir, generationLibraryProcessor{delegate: c.images.(library.Processor), before: func() {
				c.mu.Lock()
				defer c.mu.Unlock()
				switch name {
				case "generation":
					c.generation = &generation{}
				case "scope":
					c.scope.User = "replacement"
				case "store":
					c.store = &storage.Store{}
				case "manager":
					c.library = original
				case "exclusive":
					c.exclusive = true
				}
			}})
			if e != nil {
				t.Fatal(e)
			}
			store, sc := c.store, c.scope
			c.library = replacement
			defer func() {
				c.mu.Lock()
				c.generation = g
				c.scope = sc
				c.store = store
				c.library = original
				c.exclusive = false
				c.mu.Unlock()
				if e := replacement.Close(); e != nil {
					t.Error(e)
				}
			}()
			calls := 0
			e = c.withGenerationLibraryAsset(context.Background(), ctx, g, store, sc, a.ID, a.SHA256, func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
				calls++
				return nil
			})
			if !errors.Is(e, storage.ErrStale) || calls != 0 {
				t.Fatal("fresh exact lease", calls, e)
			}
			c.mu.Lock()
			held := c.leases
			c.mu.Unlock()
			if held != 0 {
				t.Fatal("failed reborrow leaked lease")
			}
		})
	}
}

func TestGenerationLibraryCancelMediaWait(t *testing.T) {
	for _, name := range []string{"parent", "generation"} {
		t.Run(name, func(t *testing.T) {
			c, g, ctx, gcancel, a := generationLibraryFixture(t)
			parent, pcancel := context.WithCancel(context.Background())
			defer pcancel()
			hold, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			mediaDone, borrowDone := make(chan error, 1), make(chan error, 1)
			mediaEntry := make(chan struct{})
			oldManager := c.library
			m, err := library.New(c.cfg.DataDir, generationLibraryProcessor{delegate: c.images.(library.Processor), entry: func() { close(mediaEntry) }})
			if err != nil {
				t.Fatal(err)
			}
			c.library = m
			defer func() {
				c.library = oldManager
				if e := m.Close(); e != nil {
					t.Error(e)
				}
			}()
			started := false
			t.Cleanup(func() {
				unblock()
				<-mediaDone
				if started {
					<-borrowDone
				}
			})
			raw := libraryPNG(t, 80)
			go func() {
				mediaDone <- c.images.(library.Processor).WithLocalImage(context.Background(), bytes.NewReader(raw), int64(len(raw)), func(attachment.LocalImage, attachment.Artifact) error { close(hold); <-release; return nil })
			}()
			select {
			case <-hold:
			case <-time.After(5 * time.Second):
				t.Fatal("media holder not entered")
			}
			started = true
			go func() {
				borrowDone <- c.withGenerationLibraryAsset(parent, ctx, g, c.store, c.scope, a.ID, a.SHA256, func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
					t.Error("canceled media callback")
					return nil
				})
			}()
			select {
			case <-mediaEntry:
				c.mu.Lock()
				held := c.leases
				c.mu.Unlock()
				if held != 0 {
					t.Error("outer Store lease held while waiting for actual media")
				}
			case <-time.After(5 * time.Second):
				pcancel()
				t.Fatal("borrow did not reach occupied actual processor")
			}
			if name == "parent" {
				pcancel()
			} else {
				gcancel()
			}
			select {
			case e := <-borrowDone:
				borrowDone <- e
				if !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancel did not leave held media wait")
			}
		})
	}
}

func TestGenerationLibraryHeldCallbackCancellationAndCloseJoin(t *testing.T) {
	for _, name := range []string{"parent", "generation", "maintenance", "close"} {
		t.Run(name, func(t *testing.T) {
			c, g, ctx, gcancel, a := generationLibraryFixture(t)
			parent, pcancel := context.WithCancel(context.Background())
			defer pcancel()
			entered, release := make(chan struct{}), make(chan struct{})
			done, joinDone := make(chan error, 1), make(chan error, 1)
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			joinStarted := false
			t.Cleanup(func() {
				unblock()
				<-done
				if joinStarted {
					<-joinDone
				}
			})
			store, sc := c.store, c.scope
			go func() {
				e := c.withGenerationLibraryAsset(parent, ctx, g, store, sc, a.ID, a.SHA256, func(bound context.Context, s *storage.Store, _ storage.LibraryAsset, _ attachment.LocalImage, _ attachment.Artifact) error {
					close(entered)
					<-bound.Done()
					<-release
					c.mu.Lock()
					held := c.leases
					c.mu.Unlock()
					if held != 1 {
						t.Error("lease released before callback returned")
					}
					_, e := s.LibraryAsset(context.Background(), sc, a.ID)
					return e
				})
				close(g.done)
				done <- e
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				pcancel()
				t.Fatal("held callback not entered")
			}
			switch name {
			case "parent":
				pcancel()
			case "generation":
				gcancel()
			case "maintenance":
				joinStarted = true
				go func() {
					c.mu.Lock()
					c.exclusive = true
					c.mu.Unlock()
					c.stopGeneration()
					c.mu.Lock()
					c.exclusive = false
					c.notifyLocked()
					c.mu.Unlock()
					joinDone <- nil
				}()
			case "close":
				joinStarted = true
				go func() { joinDone <- c.Close() }()
			}
			if joinStarted {
				select {
				case e := <-joinDone:
					joinDone <- e
					t.Error("join returned before actual callback")
				case <-time.After(20 * time.Millisecond):
				}
			}
			if _, e := store.LibraryAsset(context.Background(), sc, a.ID); e != nil {
				t.Error("Store closed before callback", e)
			}
			unblock()
			select {
			case e := <-done:
				done <- e
				if !errors.Is(e, context.Canceled) {
					t.Error("cancellation lost", e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("callback did not join")
			}
		})
	}
}

// This processor stages and validates real local PNG bytes, and exposes actual
// artifact Close for the release-before-cleanup assertion. No successful image
// metadata is invented.
type generationLibraryCloseArtifact struct {
	attachment.Artifact
	hook   func()
	closes int
}

func (a *generationLibraryCloseArtifact) Close() error {
	a.closes++
	a.hook()
	return a.Artifact.Close()
}

type generationLibraryCloseProcessor struct {
	dir       string
	closeHook func()
	original  *generationLibraryCloseArtifact
}

func (p *generationLibraryCloseProcessor) WithLocalImage(ctx context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) (err error) {
	stage, e := media.StageLocal(ctx, p.dir, r, n)
	if e != nil {
		return e
	}
	art := &generationLibraryCloseArtifact{Artifact: stage, hook: p.closeHook}
	p.original = art
	defer func() { err = errors.Join(err, art.Close(), ctx.Err(), attachment.ErrCleanup) }()
	info, e := media.ValidateImage(ctx, art, n)
	if e != nil {
		return e
	}
	h := sha256.New()
	if _, e = io.Copy(h, io.NewSectionReader(art, 0, n)); e != nil {
		return e
	}
	return fn(attachment.LocalImage{Info: info, SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n}, art)
}
func TestGenerationLibraryActualCleanupReleaseAndCancel(t *testing.T) {
	c, g, ctx, cancel, a := generationLibraryFixture(t)
	original := c.library
	p := &generationLibraryCloseProcessor{dir: c.cfg.DataDir, closeHook: func() {
		c.mu.Lock()
		held := c.leases
		c.mu.Unlock()
		if held != 0 {
			t.Error("Store still held during actual artifact Close")
		}
		cancel()
	}}
	m, e := library.New(c.cfg.DataDir, p)
	if e != nil {
		t.Fatal(e)
	}
	c.library = m
	defer func() {
		c.library = original
		if e := m.Close(); e != nil {
			t.Error(e)
		}
	}()
	calls := 0
	e = c.withGenerationLibraryAsset(context.Background(), ctx, g, c.store, c.scope, a.ID, a.SHA256, func(_ context.Context, s *storage.Store, _ storage.LibraryAsset, _ attachment.LocalImage, art attachment.Artifact) error {
		calls++
		if art != p.original || s != c.store {
			t.Error("substituted Store/artifact")
		}
		return nil
	})
	if calls != 1 || !errors.Is(e, context.Canceled) || !errors.Is(e, library.ErrCleanup) || p.original == nil || p.original.closes != 1 {
		t.Fatal("cleanup result", calls, e)
	}
	if _, e := p.original.ReadAt(make([]byte, 1), 0); e == nil {
		t.Fatal("actual artifact not closed")
	}
	// Cancellation on the generation path does not poison ordinary library leases.
	c.library = original
	if e := c.WithLibraryAsset(context.Background(), a.ID, true, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error { return nil }); e != nil {
		t.Fatal("ordinary library regression", e)
	}
}
