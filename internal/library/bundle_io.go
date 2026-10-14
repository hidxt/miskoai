package library

import (
	"errors"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/librarybundle"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
)

// PreparedBundle is lent only during PrepareBundle's synchronous callback.
// It owns an opaque candidate, never a resident Store or exposed path.
type PreparedBundle struct {
	installStarted bool
	self      *PreparedBundle
	view      *Maintenance
	candidate *maintenance.Candidate
	rows      []storage.LibraryAsset
	stage     *bundleStage
}

func (v *Maintenance) bundleValid() error {
	if v == nil || v.self != v || v.manager == nil || v.ctx == nil {
		return ErrInvalid
	}
	return v.ctx.Err()
}
func (p *PreparedBundle) valid(v *Maintenance) error {
	if p == nil || p.self != p || p.view != v || p.candidate == nil {
		return ErrInvalid
	}
	return v.bundleValid()
}
func bundleError(e error) error {
	if e == nil {
		return nil
	}
	var joined error
	if errors.Is(e, librarybundle.ErrCleanup) {
		joined = ErrCleanup
	}
	if errors.Is(e, librarybundle.ErrInvalid) {
		joined = errors.Join(joined, ErrContent)
	}
	return safe(errors.Join(joined, e))
}
func (v *Maintenance) WriteBundle(lease Lease, output io.Writer) (err error) {
	if e := v.bundleValid(); e != nil {
		return e
	}
	if lease == nil || output == nil {
		return ErrInvalid
	}
	m := v.manager
	stage, e := reserveBundle(m)
	if stage != nil {
		defer func() { err = safe(errors.Join(err, stage.cleanup(), v.ctx.Err())) }()
	}
	if e != nil {
		return e
	}
	e = withStore(v.ctx, lease, func(s *storage.Store) error {
		info, e := maintenance.BackupOwned(v.ctx, s, stage.path("database"))
		if info != nil {
			stage.files["database"] = info
		}
		return e
	})
	if e != nil {
		return safe(e)
	}
	if e = stage.verify("database", bundleDBLimit); e != nil {
		return e
	}
	rows, e := storage.LibraryBackupSnapshot(v.ctx, stage.path("database"))
	if e != nil {
		return safe(e)
	}
	if e = stage.verify("database", bundleDBLimit); e != nil {
		return e
	}
	if e = validCurrent(rows); e != nil {
		return e
	}
	if e = m.physicalContext(v.ctx, rows, 0); e != nil {
		return e
	}
	header := librarybundle.Header{Assets: []librarybundle.Asset{}}
	sources := map[string]storage.LibraryAsset{}
	for _, a := range rows {
		if _, e = os.Lstat(m.path(a)); errors.Is(e, os.ErrNotExist) && a.State == "deleting" {
			continue
		} else if e != nil {
			return ErrRecovery
		}
		if e = m.validate(v.ctx, a, noContent); e != nil {
			return e
		}
		k := strings.TrimPrefix(contentName(a.Scope, a.SHA256), "asset-")
		sources[k] = a
		header.Assets = append(header.Assets, librarybundle.Asset{Key: k, Bytes: a.Bytes, SHA256: a.SHA256})
	}
	sort.Slice(header.Assets, func(i, j int) bool { return header.Assets[i].Key < header.Assets[j].Key })
	digest, e := stage.digest(v.ctx, "database", bundleDBLimit)
	if e != nil {
		return e
	}
	header.Database = librarybundle.File{Bytes: stage.files["database"].Size(), SHA256: digest}
	var sourceErr error
	encodedErr := librarybundle.Encode(v.ctx, output, header, func(r librarybundle.Record) (reader io.Reader, release func() error, openErr error) {
		defer func() {
			sourceErr = errors.Join(sourceErr, openErr)
			if release != nil {
				originalRelease := release
				release = func() error { e := originalRelease(); sourceErr = errors.Join(sourceErr, e); return e }
			}
		}()
		if r.Kind == librarybundle.Database {
			return stage.open("database", bundleDBLimit)
		}
		a, ok := sources[r.Key]
		if !ok {
			return nil, nil, ErrInvalid
		}
		path := m.path(a)
		original := m.identities[path]
		if e := m.unchanged(path, original); e != nil {
			return nil, nil, e
		}
		f, e := privatefs.OpenRead(path, 4<<20)
		if e != nil {
			return nil, nil, ErrRecovery
		}
		info, e := f.Stat()
		if e != nil || !sameFile(info, original) {
			ce := f.Close()
			if ce != nil {
				return nil, nil, errors.Join(ErrRecovery, ErrCleanup)
			}
			return nil, nil, ErrRecovery
		}
		return f, func() error {
			var err error
			actual, e := f.Stat()
			if e != nil || !sameFile(info, actual) {
				err = ErrRecovery
			}
			if f.Close() != nil {
				err = errors.Join(err, ErrCleanup)
			}
			return errors.Join(err, m.unchanged(path, original))
		}, nil
	})
	return bundleError(errors.Join(encodedErr, sourceErr))
}
func (v *Maintenance) PrepareBundle(input io.Reader, consume func(*PreparedBundle) error) (err error) {
	if e := v.bundleValid(); e != nil {
		return e
	}
	if input == nil || consume == nil {
		return ErrInvalid
	}
	stage, e := reserveBundle(v.manager)
	var candidate *maintenance.Candidate
	preserve := false
	if stage != nil {
		defer func() {
			if candidate != nil && candidate.Discard() != nil {
				err = errors.Join(err, ErrRecovery, ErrCleanup)
				preserve = true
			}
			if !preserve {
				err = errors.Join(err, stage.cleanup())
			}
			err = safe(errors.Join(err, v.ctx.Err()))
		}()
	}
	if e != nil {
		return e
	}
	var stageErr error
	header, e := librarybundle.Decode(v.ctx, input, func(r librarybundle.Record, in io.Reader) error {
		name := r.Key
		if r.Kind == librarybundle.Database {
			name = "database"
		}
		stageErr = stage.write(v.ctx, name, in, r.Bytes)
		return stageErr
	})
	if e != nil {
		return bundleError(errors.Join(e, stageErr))
	}
	// Decode has completed every wire digest and exact EOF before SQLite reads or migration.
	if e = stage.verify("database", bundleDBLimit); e != nil {
		return e
	}
	rows, e := storage.LibraryBackupSnapshot(v.ctx, stage.path("database"))
	if e != nil {
		return safe(e)
	}
	if e = stage.verify("database", bundleDBLimit); e != nil {
		return e
	}
	if e = validCurrent(rows); e != nil {
		return e
	}
	expected := map[string]storage.LibraryAsset{}
	for _, a := range rows {
		expected[strings.TrimPrefix(contentName(a.Scope, a.SHA256), "asset-")] = a
	}
	for _, r := range header.Assets {
		a, ok := expected[r.Key]
		if !ok || a.Bytes != r.Bytes || a.SHA256 != r.SHA256 {
			return ErrContent
		}
		delete(expected, r.Key)
		if e = v.validateBundleImage(stage, r.Key, a); e != nil {
			return e
		}
	}
	for _, a := range expected {
		if a.State == "ready" {
			return ErrContent
		}
	}
	raw, release, e := stage.open("database", bundleDBLimit)
	if e != nil {
		return e
	}
	candidate, e = maintenance.PrepareRestore(v.ctx, v.manager.dataDir, raw)
	if e != nil {
		preserve = true
	}
	closeErr := release()
	if e != nil || closeErr != nil {
		return safe(errors.Join(e, closeErr, ErrRecovery))
	}
	migrated, e := candidate.LibrarySnapshot(v.ctx)
	if e != nil {
		return safe(e)
	}
	if !reflect.DeepEqual(rows, migrated) {
		return ErrContent
	}
	p := &PreparedBundle{view: v, candidate: candidate, rows: rows, stage: stage}
	p.self = p
	defer func() { p.self = nil; p.view = nil; p.candidate = nil; p.rows = nil; p.stage = nil }()
	if e = v.ctx.Err(); e != nil {
		return e
	}
	return safe(consume(p))
}
func (v *Maintenance) validateBundleImage(s *bundleStage, name string, a storage.LibraryAsset) (err error) {
	r, release, e := s.open(name, 4<<20)
	if e != nil {
		return e
	}
	defer func() { err = safe(errors.Join(err, release(), v.ctx.Err())) }()
	return safe(v.manager.processor.WithLocalImage(v.ctx, r, a.Bytes, func(im attachment.LocalImage, _ attachment.Artifact) error {
		if im.SHA256 != a.SHA256 || im.Bytes != a.Bytes || im.Info.Format != a.Format || im.Info.Width != a.Width || im.Info.Height != a.Height || im.Info.Frames != a.Frames {
			return ErrContent
		}
		return s.verify(name, 4<<20)
	}))
}
