// Package library owns bounded private immutable image content.
package library

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
)

var (
	ErrInvalid  = errors.New("library invalid")
	ErrCapacity = errors.New("library capacity")
	ErrContent  = errors.New("library content")
	ErrRecovery = errors.New("library recovery required")
	ErrCleanup  = errors.New("library cleanup")
	ErrClosed   = errors.New("library closed")
)

type Processor interface {
	WithLocalImage(context.Context, io.Reader, int64, func(attachment.LocalImage, attachment.Artifact) error) error
}
type Lease func(context.Context) (*storage.Store, func(), error)

// Manager owns one synchronous lifetime, not its borrowed processor or Stores.
// Callers must not enter it while holding a Store lease. Readers and consumers
// must cooperate with cancellation and return before Close can join them.
type Manager struct {
	dir             string
	processor       Processor
	token           chan struct{}
	mu              sync.Mutex
	closed          bool
	stop, closeDone chan struct{}
	active          context.CancelFunc
	identities      map[string]os.FileInfo // admission protects this bounded cache
}

func New(dataDir string, processor Processor) (*Manager, error) {
	if processor == nil {
		return nil, ErrInvalid
	}
	parent, e := privatefs.Resolve(dataDir)
	if e != nil || privatefs.CheckDir(parent) != nil {
		return nil, ErrInvalid
	}
	dir := filepath.Join(parent, "library")
	if privatefs.EnsureDir(dir) != nil {
		return nil, ErrRecovery
	}
	return &Manager{dir: dir, processor: processor, token: make(chan struct{}, 1), stop: make(chan struct{}), closeDone: make(chan struct{}), identities: make(map[string]os.FileInfo)}, nil
}
func validScope(sc storage.Scope) bool {
	return text(sc.Account, 256, false) && strings.TrimSpace(sc.Account) != "" && text(sc.User, 256, false) && strings.TrimSpace(sc.User) != ""
}
func text(s string, max int, controls bool) bool {
	if len(s) > max || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
		return false
	}
	if controls {
		for _, r := range s {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}
func validMeta(meta storage.LibraryMetadata) bool {
	if len(meta.Tags) > 16 || !text(meta.Emotion, 64, true) || !text(meta.Description, 1024, false) {
		return false
	}
	seen := map[string]bool{}
	for _, tag := range meta.Tags {
		if tag == "" || !text(tag, 32, true) || seen[tag] {
			return false
		}
		seen[tag] = true
	}
	b, e := json.Marshal(meta.Tags)
	return e == nil && len(b) <= 2048
}

// safe chooses fresh fixed classes; arbitrary wrapper/callback text is discarded.
func safe(e error) error {
	if e == nil {
		return nil
	}
	var result error
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, ErrInvalid, ErrCapacity, ErrContent, ErrRecovery, ErrCleanup, ErrClosed, storage.ErrInvalid, storage.ErrNotFound, storage.ErrStorage, storage.ErrCapacity, storage.ErrStale} {
		if errors.Is(e, known) {
			result = errors.Join(result, known)
		}
	}
	if errors.Is(e, attachment.ErrCleanup) {
		result = errors.Join(result, ErrCleanup)
	}
	if result == nil {
		return ErrContent
	}
	return result
}
func withStore(ctx context.Context, lease Lease, fn func(*storage.Store) error) (err error) {
	if lease == nil {
		return ErrInvalid
	}
	s, release, e := lease(ctx)
	if release != nil {
		defer func() {
			release()
			if ctx.Err() != nil {
				err = errors.Join(err, ctx.Err())
			}
			err = safe(err)
		}()
	}
	if e != nil {
		return safe(e)
	}
	if s == nil || release == nil {
		return ErrInvalid
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return safe(fn(s))
}
func lookup(ctx context.Context, lease Lease, sc storage.Scope, id int64) (a storage.LibraryAsset, e error) {
	e = withStore(ctx, lease, func(s *storage.Store) error { var err error; a, err = s.LibraryAsset(ctx, sc, id); return err })
	return
}
func sameAsset(a, b storage.LibraryAsset) bool {
	return a.ID == b.ID && a.Scope == b.Scope && a.SHA256 == b.SHA256 && a.Format == b.Format && a.Bytes == b.Bytes && a.Width == b.Width && a.Height == b.Height && a.Frames == b.Frames
}
func (m *Manager) WithAsset(parent context.Context, lease Lease, sc storage.Scope, id int64, requireEnabled bool, consume func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error {
	if !validScope(sc) || id <= 0 || lease == nil || consume == nil {
		return ErrInvalid
	}
	ctx, done, e := m.begin(parent)
	if e != nil {
		return e
	}
	defer done()
	a, e := lookup(ctx, lease, sc, id)
	if e != nil {
		return e
	}
	if a.State != "ready" || requireEnabled && !a.Enabled {
		return ErrContent
	}
	return m.validate(ctx, a, func(im attachment.LocalImage, art attachment.Artifact) error {
		return withStore(ctx, lease, func(s *storage.Store) error {
			fresh, e := s.LibraryAsset(ctx, sc, id)
			if e != nil {
				return e
			}
			if !sameAsset(a, fresh) {
				return ErrRecovery
			}
			if fresh.State != "ready" || requireEnabled && !fresh.Enabled {
				return ErrContent
			}
			if e = ctx.Err(); e != nil {
				return e
			}
			return consume(fresh, im, art)
		})
	})
}
func (m *Manager) Update(parent context.Context, lease Lease, sc storage.Scope, id int64, meta storage.LibraryMetadata) error {
	if !validScope(sc) || id <= 0 || lease == nil || !validMeta(meta) {
		return ErrInvalid
	}
	ctx, done, e := m.begin(parent)
	if e != nil {
		return e
	}
	defer done()
	a, e := lookup(ctx, lease, sc, id)
	if e != nil {
		return e
	}
	if a.State != "ready" {
		return ErrContent
	}
	update := func() error {
		return withStore(ctx, lease, func(s *storage.Store) error {
			fresh, e := s.LibraryAsset(ctx, sc, id)
			if e != nil {
				return e
			}
			if !sameAsset(a, fresh) {
				return ErrRecovery
			}
			return s.UpdateLibraryAsset(ctx, sc, id, meta)
		})
	}
	if meta.Enabled {
		return m.validate(ctx, a, func(attachment.LocalImage, attachment.Artifact) error { return update() })
	}
	return update()
}
func (m *Manager) RecordUse(parent context.Context, lease Lease, sc storage.Scope, id int64, digest string, atMillis int64) error {
	if !validScope(sc) || id <= 0 || lease == nil || !validDigest(digest) || atMillis < 1 || atMillis > 253402300799999 {
		return ErrInvalid
	}
	ctx, done, e := m.begin(parent)
	if e != nil {
		return e
	}
	defer done()
	return withStore(ctx, lease, func(s *storage.Store) error {
		a, e := s.LibraryAsset(ctx, sc, id)
		if e != nil {
			return e
		}
		if a.SHA256 != digest {
			return ErrRecovery
		}
		return s.RecordLibraryUse(ctx, sc, id, atMillis)
	})
}
