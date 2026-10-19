package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/storage"
)

// withGenerationLibraryAsset is inactive composition groundwork. All identities
// come from trusted generation composition; consume is synchronous and must not
// reenter Core/Manager or retain its Store or artifact. No Store lease precedes
// Manager admission, and no transaction spans consume.
func (c *Controller) withGenerationLibraryAsset(parent, generationContext context.Context, g *generation, expectedStore *storage.Store, sc storage.Scope, id int64, digest string, consume func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) (err error) {
	if c == nil || parent == nil || generationContext == nil || g == nil || expectedStore == nil || consume == nil || id <= 0 {
		return ErrConfiguration
	}
	for _, v := range []string{sc.Account, sc.User} {
		if strings.TrimSpace(v) == "" || len(v) > 256 || !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
			return ErrConfiguration
		}
	}
	if len(digest) != 64 {
		return ErrConfiguration
	}
	for _, v := range digest {
		if !(v >= '0' && v <= '9' || v >= 'a' && v <= 'f') {
			return ErrConfiguration
		}
	}
	if e := parent.Err(); e != nil {
		return e
	}
	if e := generationContext.Err(); e != nil {
		return e
	}
	c.mu.Lock()
	m := c.library
	err = c.generationLibraryCheckLocked(parent, generationContext, g, expectedStore, sc, m)
	c.mu.Unlock()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	joined := make(chan struct{})
	stop := context.AfterFunc(generationContext, func() { defer close(joined); cancel() })
	defer func() {
		if !stop() {
			<-joined
		}
		// Inspect both captured contexts after real Manager/artifact cleanup.
		err = libraryError(errors.Join(err, parent.Err(), generationContext.Err()))
		cancel()
	}()
	if generationContext.Err() != nil {
		cancel()
	}
	return m.WithAssetStore(ctx, c.generationLibraryLease(generationContext, g, expectedStore, sc, m), sc, id, true, func(s *storage.Store, a storage.LibraryAsset, im attachment.LocalImage, art attachment.Artifact) error {
		if a.SHA256 != digest {
			return library.ErrRecovery
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		return consume(ctx, s, a, im, art)
	})
}

// Must hold c.mu. Refusal is immediate: waiting on c.changed here can deadlock
// with exclusive maintenance that is joining this same generation.
func (c *Controller) generationLibraryCheckLocked(ctx, generationContext context.Context, g *generation, expectedStore *storage.Store, sc storage.Scope, m *library.Manager) error {
	if c.closed || c.closing || c.unavailable || c.paused || c.expired || c.store == nil || c.library == nil || m == nil {
		return ErrUnavailable
	}
	if c.scope.Account == "" || c.scope.User == "" {
		return ErrUnconfigured
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := generationContext.Err(); e != nil {
		return e
	}
	if c.parent == nil {
		return ErrUnavailable
	}
	if e := c.parent.Err(); e != nil {
		return e
	}
	if c.exclusive {
		return storage.ErrStale
	}
	if c.generation != g || c.store != expectedStore || c.scope != sc || c.library != m {
		return storage.ErrStale
	}
	return nil
}

func (c *Controller) generationLibraryLease(generationContext context.Context, g *generation, expectedStore *storage.Store, sc storage.Scope, m *library.Manager) library.Lease {
	return func(ctx context.Context) (*storage.Store, func(), error) {
		if ctx == nil || generationContext == nil || g == nil || expectedStore == nil || m == nil {
			return nil, nil, ErrConfiguration
		}
		c.mu.Lock()
		if e := c.generationLibraryCheckLocked(ctx, generationContext, g, expectedStore, sc, m); e != nil {
			c.mu.Unlock()
			return nil, nil, e
		}
		c.leases++
		c.mu.Unlock()
		var once sync.Once
		return expectedStore, func() { once.Do(func() { c.mu.Lock(); c.leases--; c.notifyLocked(); c.mu.Unlock() }) }, nil
	}
}
