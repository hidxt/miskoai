package library

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
)

func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func contentName(sc storage.Scope, digest string) string {
	h := sha256.New()
	var length [4]byte
	for _, v := range []string{sc.Account, sc.User} {
		binary.BigEndian.PutUint32(length[:], uint32(len(v)))
		h.Write(length[:])
		h.Write([]byte(v))
	}
	raw, _ := hex.DecodeString(digest)
	h.Write(raw)
	return "asset-" + hex.EncodeToString(h.Sum(nil))
}
func (m *Manager) path(a storage.LibraryAsset) string {
	return filepath.Join(m.dir, contentName(a.Scope, a.SHA256))
}
func sameFile(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
func (m *Manager) unchanged(path string, original os.FileInfo) error {
	named, e := os.Lstat(path)
	if e != nil || !sameFile(original, named) || privatefs.CheckFile(path, 4<<20) != nil {
		return ErrRecovery
	}
	return nil
}
func (m *Manager) remember(path string, info os.FileInfo) error {
	if original, ok := m.identities[path]; ok {
		if !sameFile(original, info) {
			return ErrRecovery
		}
		return nil
	}
	if len(m.identities) >= 128 {
		return ErrCapacity
	}
	m.identities[path] = info
	return nil
}

// validate stages persistent bytes through the borrowed shared processor. The
// artifact lent to callbacks is temporary; its Close never removes this file.
func (m *Manager) validate(ctx context.Context, a storage.LibraryAsset, consume func(attachment.LocalImage, attachment.Artifact) error) (err error) {
	path := m.path(a)
	f, e := privatefs.OpenRead(path, 4<<20)
	if e != nil {
		return ErrRecovery
	}
	defer func() {
		if f.Close() != nil {
			err = errors.Join(err, ErrCleanup)
		}
		err = safe(err)
	}()
	original, e := f.Stat()
	if e != nil || original.Size() != a.Bytes || !os.SameFile(original, original) {
		return ErrRecovery
	}
	if old, ok := m.identities[path]; ok && !sameFile(old, original) {
		return ErrRecovery
	}
	var callbackErr error
	processorErr := m.processor.WithLocalImage(ctx, io.NewSectionReader(f, 0, a.Bytes), a.Bytes, func(im attachment.LocalImage, art attachment.Artifact) error {
		if im.SHA256 != a.SHA256 || im.Bytes != a.Bytes || im.Info.Format != a.Format || im.Info.Width != a.Width || im.Info.Height != a.Height || im.Info.Frames != a.Frames {
			callbackErr = ErrContent
			return callbackErr
		}
		if callbackErr = m.unchanged(path, original); callbackErr != nil {
			return callbackErr
		}
		actual, e := f.Stat()
		if e != nil || !sameFile(original, actual) {
			callbackErr = ErrRecovery
			return callbackErr
		}
		if callbackErr = m.remember(path, original); callbackErr != nil {
			return callbackErr
		}
		callbackErr = safe(consume(im, art))
		return callbackErr
	})
	if callbackErr != nil {
		err = callbackErr
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		if errors.Is(processorErr, attachment.ErrCleanup) {
			err = errors.Join(err, ErrCleanup)
		}
	} else {
		err = safe(processorErr)
	}
	if ctx.Err() != nil {
		err = errors.Join(err, ctx.Err())
	}
	if e = m.unchanged(path, original); e != nil {
		err = errors.Join(err, e)
	}
	return err
}

// physical reads only bounded metadata. Unknown entries are counted, retained,
// and refuse mutation; no directory scan ever adopts or sweeps an orphan.
func (m *Manager) physical(rows []storage.LibraryAsset, extra int64) error {
	return m.physicalContext(context.Background(), rows, extra)
}
func (m *Manager) physicalContext(ctx context.Context, rows []storage.LibraryAsset, extra int64) error {
	if e := m.checkRetainedStage(); e != nil {
		return e
	}
	c, e := m.readRetained()
	if e != nil {
		return e
	}
	return m.physicalWith(ctx, rows, extra, c)
}
func (m *Manager) physicalWith(ctx context.Context, rows []storage.LibraryAsset, extra int64, c retainedCatalog) error {
	if privatefs.CheckDir(m.dir) != nil {
		return ErrRecovery
	}
	d, e := os.Open(m.dir)
	if e != nil {
		return ErrRecovery
	}
	entries, readErr := d.ReadDir(129)
	closeErr := d.Close()
	if readErr != nil && readErr != io.EOF || closeErr != nil {
		return ErrRecovery
	}
	if len(entries) > 128 {
		return ErrCapacity
	}
	expected := make(map[string]storage.LibraryAsset, len(rows))
	for _, a := range rows {
		expected[contentName(a.Scope, a.SHA256)] = a
	}
	for _, r := range c.Entries {
		name := r.key()
		if a, ok := expected[name]; ok && record(a) != r {
			return ErrRecovery
		}
		expected[name] = r.asset()
		if e := m.compare(ctx, r.asset()); e != nil {
			return safe(errors.Join(e, ErrRecovery))
		}
	}
	seen := make(map[string]bool, len(entries))
	var total int64
	uncertain := false
	for _, entry := range entries {
		path := filepath.Join(m.dir, entry.Name())
		info, e := entry.Info()
		if e != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 4<<20 {
			return ErrRecovery
		}
		total += info.Size()
		if total > 64<<20 {
			return ErrCapacity
		}
		if !strings.HasPrefix(entry.Name(), "asset-") || !validDigest(strings.TrimPrefix(entry.Name(), "asset-")) || privatefs.CheckFile(path, 4<<20) != nil {
			return ErrRecovery
		}
		a, ok := expected[entry.Name()]
		if !ok || a.Bytes != info.Size() {
			uncertain = true
		}
		if old, ok := m.identities[path]; ok && !sameFile(old, info) {
			uncertain = true
		}
		seen[entry.Name()] = true
	}
	if extra > 0 && (len(entries) >= 128 || total > (64<<20)-extra) {
		return ErrCapacity
	}
	for name, a := range expected {
		if !seen[name] && a.State == "ready" {
			uncertain = true
		}
	}
	if uncertain {
		return ErrRecovery
	}
	return nil
}
func (m *Manager) removeOriginal(path string, original os.FileInfo) error {
	if e := m.unchanged(path, original); e != nil {
		return e
	}
	if os.Remove(path) != nil {
		return ErrCleanup
	}
	delete(m.identities, path)
	if maintenance.SyncPrivateDirectory(m.dir) != nil {
		return ErrCleanup
	}
	return nil
}
func (m *Manager) publish(ctx context.Context, a storage.LibraryAsset, art attachment.Artifact) (original os.FileInfo, err error) {
	path := m.path(a)
	f, e := privatefs.Create(path)
	if e != nil {
		return nil, ErrRecovery
	}
	original, e = f.Stat()
	if e != nil || !os.SameFile(original, original) {
		if f.Close() != nil {
			return nil, errors.Join(ErrRecovery, ErrCleanup)
		}
		return nil, ErrRecovery
	}
	// Capture the created original even when copying/sync fails. Refresh only its
	// own final metadata, never identity from an unrelated named replacement.
	defer func() {
		info, e := f.Stat()
		if e == nil && os.SameFile(original, info) {
			original = info
		} else {
			err = errors.Join(err, ErrRecovery)
		}
		if f.Close() != nil {
			err = errors.Join(err, ErrCleanup)
		}
		if err == nil {
			err = m.unchanged(path, original)
		}
		if err == nil && maintenance.SyncPrivateDirectory(m.dir) != nil {
			err = ErrCleanup
		}
	}()
	var scratch [4096]byte
	h := sha256.New()
	for offset := int64(0); offset < a.Bytes; {
		if e = ctx.Err(); e != nil {
			return original, e
		}
		length := int64(len(scratch))
		if a.Bytes-offset < length {
			length = a.Bytes - offset
		}
		block := scratch[:int(length)]
		n, re := art.ReadAt(block, offset)
		if n != len(block) || re != nil && re != io.EOF {
			return original, ErrContent
		}
		h.Write(block)
		n, e = f.Write(block)
		if e != nil || n != len(block) {
			return original, ErrContent
		}
		offset += length
	}
	if hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return original, ErrContent
	}
	if e = ctx.Err(); e != nil {
		return original, e
	}
	if f.Sync() != nil {
		return original, ErrCleanup
	}
	return original, nil
}
func (m *Manager) Upload(parent context.Context, lease Lease, sc storage.Scope, input io.Reader, size int64, meta storage.LibraryMetadata) (result storage.LibraryAsset, added bool, err error) {
	if !validScope(sc) || input == nil || size < 1 || size > 4<<20 || meta.Enabled || !validMeta(meta) || lease == nil {
		return result, false, ErrInvalid
	}
	ctx, done, e := m.begin(parent)
	if e != nil {
		return result, false, e
	}
	defer done()
	var callbackErr error
	processorErr := m.processor.WithLocalImage(ctx, input, size, func(im attachment.LocalImage, art attachment.Artifact) error {
		callbackErr = withStore(ctx, lease, func(s *storage.Store) error {
			rows, e := s.LibrarySnapshot(ctx)
			if e != nil {
				return e
			}
			if e = m.physicalContext(ctx, rows, 0); e != nil {
				return e
			}
			a := storage.LibraryAsset{Scope: sc, SHA256: im.SHA256, Format: im.Info.Format, Bytes: im.Bytes, Width: im.Info.Width, Height: im.Info.Height, Frames: im.Info.Frames, State: "ready", Tags: meta.Tags, Emotion: meta.Emotion, Description: meta.Description}
			for _, old := range rows {
				if old.Scope == sc && old.SHA256 == a.SHA256 {
					if old.State != "ready" {
						return ErrRecovery
					}
					if old.Format != a.Format || old.Bytes != a.Bytes || old.Width != a.Width || old.Height != a.Height || old.Frames != a.Frames {
						return ErrContent
					}
					// Already holding the shared media slot: compare the fully validated input
					// to persistent bytes without recursively entering the processor.
					if e = m.compare(ctx, old); e != nil {
						return e
					}
					result = old
					return nil
				}
			}
			if len(rows) >= 128 {
				return ErrCapacity
			}
			var bytes int64
			for _, row := range rows {
				bytes += row.Bytes
			}
			if bytes > (64<<20)-a.Bytes {
				return ErrCapacity
			}
			catalog, e := m.readRetained()
			if e != nil {
				return e
			}
			for _, r := range catalog.Entries {
				if r.key() == contentName(a.Scope, a.SHA256) {
					if r != record(a) {
						return ErrRecovery
					}
					if e = m.compare(ctx, a); e != nil {
						return e
					}
					result, added, e = s.AddLibraryAsset(ctx, sc, a)
					return e
				}
			}
			if e = m.physicalContext(ctx, rows, a.Bytes); e != nil {
				return e
			}
			original, e := m.publish(ctx, a, art)
			if e != nil {
				if original != nil {
					e = errors.Join(e, m.removeOriginal(m.path(a), original))
				}
				return e
			}
			result, added, e = s.AddLibraryAsset(ctx, sc, a)
			if e != nil || !added {
				// Commit errors may be ambiguous. A fresh authoritative lookup on this
				// same still-leased Store must establish absence before owned removal.
				// Resolving commit ambiguity is synchronous cleanup under the same
				// lease. Cancellation must not abandon the absence check itself.
				snapshot, se := s.LibrarySnapshot(context.WithoutCancel(ctx))
				if se != nil {
					return errors.Join(safe(e), ErrRecovery)
				}
				for _, row := range snapshot {
					if row.Scope == sc && row.SHA256 == a.SHA256 {
						return errors.Join(safe(e), ErrRecovery)
					}
				}
				return errors.Join(safe(e), m.removeOriginal(m.path(a), original))
			}
			if e = m.remember(m.path(a), original); e != nil {
				return e
			}
			return nil
		})
		return callbackErr
	})
	if callbackErr != nil {
		err = callbackErr
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
		}
		if errors.Is(processorErr, attachment.ErrCleanup) {
			err = errors.Join(err, ErrCleanup)
		}
	} else {
		err = safe(processorErr)
	}
	if ctx.Err() != nil {
		err = errors.Join(err, ctx.Err())
	}
	return result, added, err
}
func (m *Manager) compare(ctx context.Context, a storage.LibraryAsset) (err error) {
	path := m.path(a)
	f, e := privatefs.OpenRead(path, 4<<20)
	if e != nil {
		return ErrRecovery
	}
	defer func() {
		if f.Close() != nil {
			err = errors.Join(err, ErrCleanup)
		}
	}()
	original, e := f.Stat()
	if e != nil || original.Size() != a.Bytes || !os.SameFile(original, original) {
		return ErrRecovery
	}
	if old, ok := m.identities[path]; ok && !sameFile(old, original) {
		return ErrRecovery
	}
	var x [4096]byte
	h := sha256.New()
	for offset := int64(0); offset < a.Bytes; {
		if e = ctx.Err(); e != nil {
			return e
		}
		length := int64(len(x))
		if a.Bytes-offset < length {
			length = a.Bytes - offset
		}
		n, e := f.ReadAt(x[:int(length)], offset)
		if n != int(length) || e != nil && e != io.EOF {
			return ErrContent
		}
		h.Write(x[:int(length)])
		offset += length
	}
	if hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return ErrContent
	}
	if e = m.unchanged(path, original); e != nil {
		return e
	}
	return m.remember(path, original)
}
func (m *Manager) Delete(parent context.Context, lease Lease, sc storage.Scope, id int64) error {
	if !validScope(sc) || id <= 0 || lease == nil {
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
	path := m.path(a)
	if e = m.checkRetainedStage(); e != nil {
		return e
	}
	c, e := m.readRetained()
	if e != nil {
		return e
	}
	protected := false
	for _, r := range c.Entries {
		if r.key() == contentName(a.Scope, a.SHA256) {
			if r != record(a) {
				return ErrRecovery
			}
			protected = true
		}
	}
	if _, e = os.Lstat(path); errors.Is(e, os.ErrNotExist) {
		if protected {
			return ErrRecovery
		}
		if a.State != "deleting" {
			return ErrRecovery
		}
		return withStore(ctx, lease, func(s *storage.Store) error {
			fresh, e := s.LibraryAsset(ctx, sc, id)
			if e != nil {
				return e
			}
			if !sameAsset(a, fresh) || fresh.State != "deleting" {
				return ErrRecovery
			}
			if maintenance.SyncPrivateDirectory(m.dir) != nil {
				return ErrCleanup
			}
			if e = s.FinishLibraryDelete(ctx, sc, id); e != nil {
				return e
			}
			delete(m.identities, path)
			return nil
		})
	} else if e != nil {
		return ErrRecovery
	}
	// Persist deletion intent first, but release this short lease before media
	// admission. An uncertain original remains intact behind a disabled tombstone.
	if e = withStore(ctx, lease, func(s *storage.Store) error {
		fresh, e := s.LibraryAsset(ctx, sc, id)
		if e != nil {
			return e
		}
		if !sameAsset(a, fresh) {
			return ErrRecovery
		}
		return s.MarkLibraryDeleting(ctx, sc, id)
	}); e != nil {
		return e
	}
	// Media and persistent handles finish before original-only removal. Corrupt
	// or replaced bytes require recovery rather than deletion or automatic retry.
	if e = m.validate(ctx, a, func(attachment.LocalImage, attachment.Artifact) error { return nil }); e != nil {
		return safe(errors.Join(e, ErrRecovery))
	}
	return withStore(ctx, lease, func(s *storage.Store) error {
		fresh, e := s.LibraryAsset(ctx, sc, id)
		if e != nil {
			return e
		}
		if !sameAsset(a, fresh) || fresh.State != "deleting" || fresh.Enabled {
			return ErrRecovery
		}
		original := m.identities[path]
		if e = m.unchanged(path, original); e != nil {
			return e
		}
		if !protected {
			if e = m.removeOriginal(path, original); e != nil {
				return e
			}
		}
		return s.FinishLibraryDelete(ctx, sc, id)
	})
}
