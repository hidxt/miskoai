package core

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/storage"
)

// libraryAccess captures only Core's configured administrative scope. Offline
// administration does not depend on provider readiness or channel reconciliation.
// No Store lease is held when entering the Manager.
func (c *Controller) libraryAccess(ctx context.Context) (*library.Manager, storage.Scope, error) {
	if ctx == nil {
		return nil, storage.Scope{}, ErrConfiguration
	}
	if e := ctx.Err(); e != nil {
		return nil, storage.Scope{}, e
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.closing || c.unavailable || c.store == nil {
		return nil, storage.Scope{}, ErrUnavailable
	}
	if c.scope.Account == "" || c.scope.User == "" {
		return nil, storage.Scope{}, ErrUnconfigured
	}
	if c.library == nil {
		return nil, storage.Scope{}, ErrUnavailable
	}
	return c.library, c.scope, nil
}

// libraryLease acquires the current Store on every callback and rechecks the
// captured scope while the lease prevents maintenance from replacing its Store.
// Callers must release a returned lease, including an error-with-lease.
func (c *Controller) libraryLease(sc storage.Scope) library.Lease {
	return func(ctx context.Context) (*storage.Store, func(), error) {
		s, release, e := c.lease(ctx, false, true)
		if e != nil {
			return s, release, e
		}
		c.mu.Lock()
		same := c.scope == sc
		c.mu.Unlock()
		if !same {
			return s, release, storage.ErrStale
		}
		return s, release, nil
	}
}

// libraryError preserves only fixed public classes, never callback/OS text.
func libraryError(e error) error {
	if e == nil {
		return nil
	}
	var safe error
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, ErrConfiguration, ErrUnavailable, ErrUnconfigured, library.ErrInvalid, library.ErrCapacity, library.ErrContent, library.ErrRecovery, library.ErrCleanup, library.ErrClosed, storage.ErrInvalid, storage.ErrNotFound, storage.ErrStorage, storage.ErrCapacity, storage.ErrStale} {
		if errors.Is(e, known) {
			safe = errors.Join(safe, known)
		}
	}
	if safe == nil {
		return library.ErrContent
	}
	return safe
}

// LibraryAssets lists bounded metadata only. File availability is established
// by WithLibraryAsset's complete on-use validation, not by a successful listing.
func (c *Controller) LibraryAssets(parent context.Context, afterID int64, limit int) ([]storage.LibraryAsset, error) {
	ctx, cancel, e := bounded(parent, 30*time.Second)
	if e != nil {
		return nil, e
	}
	defer cancel()
	_, sc, e := c.libraryAccess(ctx)
	if e != nil {
		return nil, e
	}
	s, release, e := c.libraryLease(sc)(ctx)
	if release != nil {
		defer release()
	}
	if e != nil {
		return nil, libraryError(e)
	}
	assets, e := s.LibraryAssets(ctx, sc, afterID, limit)
	return assets, libraryError(e)
}

// LibraryUpload publishes a validated private image. Readers must cooperate
// with cancellation; the bounded context includes admission waits and cleanup.
func (c *Controller) LibraryUpload(parent context.Context, input io.Reader, size int64, meta storage.LibraryMetadata) (storage.LibraryAsset, bool, error) {
	ctx, cancel, e := bounded(parent, 30*time.Second)
	if e != nil {
		return storage.LibraryAsset{}, false, e
	}
	defer cancel()
	m, sc, e := c.libraryAccess(ctx)
	if e != nil {
		return storage.LibraryAsset{}, false, e
	}
	a, created, e := m.Upload(ctx, c.libraryLease(sc), sc, input, size, meta)
	return a, created, libraryError(e)
}
func (c *Controller) LibraryUpdate(parent context.Context, id int64, meta storage.LibraryMetadata) error {
	ctx, cancel, e := bounded(parent, 30*time.Second)
	if e != nil {
		return e
	}
	defer cancel()
	m, sc, e := c.libraryAccess(ctx)
	if e != nil {
		return e
	}
	return libraryError(m.Update(ctx, c.libraryLease(sc), sc, id, meta))
}
func (c *Controller) LibraryDelete(parent context.Context, id int64) error {
	ctx, cancel, e := bounded(parent, 30*time.Second)
	if e != nil {
		return e
	}
	defer cancel()
	m, sc, e := c.libraryAccess(ctx)
	if e != nil {
		return e
	}
	return libraryError(m.Delete(ctx, c.libraryLease(sc), sc, id))
}

// WithLibraryAsset lends the actual validated temporary image and current row.
// consume must finish synchronously, must not reenter Core/library, and must not
// retain the Artifact. The current Store lease remains held through its return.
func (c *Controller) WithLibraryAsset(parent context.Context, id int64, requireEnabled bool, consume func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error {
	ctx, cancel, e := bounded(parent, 30*time.Second)
	if e != nil {
		return e
	}
	defer cancel()
	m, sc, e := c.libraryAccess(ctx)
	if e != nil {
		return e
	}
	return libraryError(m.WithAsset(ctx, c.libraryLease(sc), sc, id, requireEnabled, consume))
}

// LibraryRecordUse is only for a trusted caller with confirmed-send evidence.
// Matching the current digest prevents an old restored ID from recording use of
// a different image. No use is inferred from previews or model output.
func (c *Controller) LibraryRecordUse(parent context.Context, id int64, digest string, atMillis int64) error {
	ctx, cancel, e := bounded(parent, 30*time.Second)
	if e != nil {
		return e
	}
	defer cancel()
	m, sc, e := c.libraryAccess(ctx)
	if e != nil {
		return e
	}
	return libraryError(m.RecordUse(ctx, c.libraryLease(sc), sc, id, digest, atMillis))
}
