package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// InstallBundle consumes a prepared callback object once. The trusted caller
// captures current under this maintenance admission, releases its Store lease,
// and stops/joins generation work before invoking actual InstallRestore. It must
// not reenter Manager, discard the candidate, or detach callback work. After
// callback entry any failure preserves content for explicit recovery because
// the database may have changed. No filesystem/SQLite atomicity is promised.
func (v *Maintenance) InstallBundle(p *PreparedBundle, current []storage.LibraryAsset, install func(*maintenance.Candidate) (maintenance.RestoreResult, error)) (maintenance.RestoreResult, error) {
	return v.installBundle(p, current, install, defaultBundleInstallIO())
}

// Narrow per-call operations wrap actual owned I/O; no process-wide hooks.
type bundleInstallIO struct {
	read    func(io.Reader, []byte) (int, error)
	write   func(*os.File, []byte) (int, error)
	sync    func(*os.File) error
	close   func(*os.File) error
	syncDir func(string) error
	remove  func(string) error
}

func defaultBundleInstallIO() bundleInstallIO {
	return bundleInstallIO{func(r io.Reader, b []byte) (int, error) { return r.Read(b) }, func(f *os.File, b []byte) (int, error) { return f.Write(b) }, func(f *os.File) error { return f.Sync() }, func(f *os.File) error { return f.Close() }, maintenance.SyncPrivateDirectory, os.Remove}
}

type bundleInstallPlan struct {
	known    map[string]storage.LibraryAsset
	incoming map[string]storage.LibraryAsset
	missing  map[string]bool
}

func (v *Maintenance) checkPrepared(p *PreparedBundle) error {
	if e := p.valid(v); e != nil {
		return e
	}
	if p.stage == nil || p.stage.check() != nil {
		return ErrRecovery
	}
	d, e := os.Open(p.stage.dir)
	if e != nil {
		return ErrRecovery
	}
	entries, re := d.ReadDir(130)
	ce := d.Close()
	if re != nil && re != io.EOF || ce != nil || len(entries) != len(p.stage.files) || len(entries) > 129 {
		return ErrRecovery
	}
	for _, entry := range entries {
		if _, ok := p.stage.files[entry.Name()]; !ok {
			return ErrRecovery
		}
		limit := int64(4 << 20)
		if entry.Name() == "database" {
			limit = bundleDBLimit
		}
		if e := p.stage.verify(entry.Name(), limit); e != nil {
			return e
		}
	}
	rows, e := p.candidate.LibrarySnapshot(v.ctx)
	if e != nil {
		return safe(errors.Join(e, ErrRecovery))
	}
	if !reflect.DeepEqual(rows, p.rows) {
		return ErrContent
	}
	if e := p.stage.verify("database", bundleDBLimit); e != nil {
		return e
	}
	for _, a := range p.rows {
		k := strings.TrimPrefix(contentName(a.Scope, a.SHA256), "asset-")
		if _, ok := p.stage.files[k]; ok {
			if e := v.validateBundleImage(p.stage, k, a); e != nil {
				return e
			}
		} else if a.State == "ready" {
			return ErrContent
		}
	}
	return nil
}
func (v *Maintenance) preflightBundle(p *PreparedBundle, current []storage.LibraryAsset) (plan bundleInstallPlan, err error) {
	plan = bundleInstallPlan{map[string]storage.LibraryAsset{}, map[string]storage.LibraryAsset{}, map[string]bool{}}
	if e := validCurrent(current); e != nil {
		return plan, e
	}
	if e := validCurrent(p.rows); e != nil {
		return plan, e
	}
	if e := v.checkPrepared(p); e != nil {
		return plan, e
	}
	m := v.manager
	if e := m.checkRetainedStage(); e != nil {
		return plan, e
	}
	c, e := m.readRetained()
	if e != nil {
		return plan, e
	}
	for _, a := range current {
		plan.known[contentName(a.Scope, a.SHA256)] = a
	}
	for _, r := range c.Entries {
		if a, ok := plan.known[r.key()]; ok && record(a) != r {
			return plan, ErrRecovery
		}
		plan.known[r.key()] = r.asset()
	}
	for _, a := range p.rows {
		k := contentName(a.Scope, a.SHA256)
		if prior, ok := plan.known[k]; ok && record(prior) != record(a) {
			return plan, ErrRecovery
		}
		if _, ok := p.stage.files[strings.TrimPrefix(k, "asset-")]; ok {
			plan.incoming[k] = a
		}
	}
	if privatefs.CheckDir(m.dir) != nil {
		return plan, ErrRecovery
	}
	d, e := os.Open(m.dir)
	if e != nil {
		return plan, ErrRecovery
	}
	entries, re := d.ReadDir(129)
	ce := d.Close()
	if re != nil && re != io.EOF || ce != nil {
		return plan, ErrRecovery
	}
	if len(entries) > 128 {
		return plan, ErrCapacity
	}
	present := map[string]bool{}
	union := map[string]storage.LibraryAsset{}
	for _, entry := range entries {
		a, ok := plan.known[entry.Name()]
		if !ok {
			return plan, ErrRecovery
		}
		present[entry.Name()] = true
		union[entry.Name()] = a
	}
	for k, a := range plan.known {
		if !present[k] {
			if incoming, ok := plan.incoming[k]; ok && record(a) == record(incoming) {
				plan.missing[k] = true
				union[k] = a
			} else if a.State == "ready" {
				return plan, ErrRecovery
			}
		}
	}
	for k, a := range plan.incoming {
		union[k] = a
		if !present[k] {
			plan.missing[k] = true
		}
	}
	if len(union) > 128 {
		return plan, ErrCapacity
	}
	var total int64
	for _, a := range union {
		total += a.Bytes
		if total > 64<<20 {
			return plan, ErrCapacity
		}
	}
	// The cheap immutable union gate precedes potentially expensive decoding.
	// Passing it still requires complete validation of every present original.
	for k := range present {
		if e := m.validate(v.ctx, plan.known[k], noContent); e != nil {
			return plan, safe(errors.Join(e, ErrRecovery))
		}
	}
	return plan, v.ctx.Err()
}
func (v *Maintenance) installBundle(p *PreparedBundle, current []storage.LibraryAsset, install func(*maintenance.Candidate) (maintenance.RestoreResult, error), ops bundleInstallIO) (result maintenance.RestoreResult, err error) {
	if e := v.bundleValid(); e != nil {
		return result, e
	}
	if e := p.valid(v); e != nil {
		return result, e
	}
	if install == nil || p.installStarted {
		return result, ErrInvalid
	}
	p.installStarted = true
	plan, e := v.preflightBundle(p, current)
	if e != nil {
		return result, e
	}
	m := v.manager
	owned := map[string]os.FileInfo{}
	callbackStarted := false
	defer func() {
		err = errors.Join(err, v.ctx.Err())
		if callbackStarted {
			if err != nil {
				err = errors.Join(err, ErrRecovery)
			}
		} else if err != nil {
			err = errors.Join(err, v.cleanupBundleOriginals(current, owned, ops))
		}
		err = safe(err)
	}()
	keys := make([]string, 0, len(plan.missing))
	for k := range plan.missing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Referenced repairs precede retention and survive every later failure.
	for _, k := range keys {
		if _, known := plan.known[k]; !known {
			continue
		}
		a := plan.incoming[k]
		original, e := v.createBundleOriginal(p, a, ops)
		if e != nil {
			return result, safe(errors.Join(e, ErrRecovery))
		}
		// This exact-known restore gate alone replaces a stale identity after
		// absence preflight and completed full-byte original source/output checks.
		if _, remembered := m.identities[m.path(a)]; remembered {
			m.identities[m.path(a)] = original
		} else if e = m.remember(m.path(a), original); e != nil {
			return result, e
		}
		if e = m.validate(v.ctx, a, noContent); e != nil {
			return result, e
		}
	}
	if e = v.RetainCurrent(current); e != nil {
		return result, e
	}
	for _, k := range keys {
		if _, known := plan.known[k]; known {
			continue
		}
		a := plan.incoming[k]
		original, e := v.createBundleOriginal(p, a, ops)
		if original != nil {
			owned[k] = original
		}
		if e != nil {
			return result, e
		}
		if e = m.remember(m.path(a), original); e != nil {
			return result, e
		}
	}
	combined := append(append([]storage.LibraryAsset{}, current...), p.rows...)
	if e = m.physicalContext(v.ctx, combined, 0); e != nil {
		return result, e
	}
	for _, a := range plan.incoming {
		if e = m.validate(v.ctx, a, noContent); e != nil {
			return result, e
		}
	}
	if e = v.checkPrepared(p); e != nil {
		return result, e
	}
	if ops.syncDir(m.dir) != nil {
		return result, ErrCleanup
	}
	if e = v.ctx.Err(); e != nil {
		return result, e
	}
	callbackStarted = true
	result, e = install(p.candidate)
	if e != nil {
		return result, safe(errors.Join(e, ErrRecovery))
	}
	return result, v.ctx.Err()
}
func (v *Maintenance) createBundleOriginal(p *PreparedBundle, a storage.LibraryAsset, ops bundleInstallIO) (original os.FileInfo, err error) {
	name := strings.TrimPrefix(contentName(a.Scope, a.SHA256), "asset-")
	r, release, e := p.stage.open(name, 4<<20)
	if e != nil {
		return nil, e
	}
	defer func() { err = safe(errors.Join(err, release(), v.ctx.Err())) }()
	path := v.manager.path(a)
	f, e := privatefs.Create(path)
	if e != nil {
		return nil, ErrRecovery
	}
	original, e = f.Stat()
	if e != nil {
		ce := ops.close(f)
		return nil, safe(errors.Join(ErrRecovery, ce))
	}
	closed := false
	defer func() {
		if !closed {
			info, e := f.Stat()
			if e != nil || !os.SameFile(original, info) {
				err = errors.Join(err, ErrRecovery)
			} else {
				original = info
			}
			if ops.close(f) != nil {
				err = errors.Join(err, ErrCleanup)
			}
		}
		if original != nil {
			err = errors.Join(err, v.manager.unchanged(path, original))
		}
		err = safe(err)
	}()
	var b [4096]byte
	h := sha256.New()
	var count int64
	for count < a.Bytes {
		if e = v.ctx.Err(); e != nil {
			return original, e
		}
		want := int64(len(b))
		if a.Bytes-count < want {
			want = a.Bytes - count
		}
		n, re := ops.read(r, b[:int(want)])
		if n < 0 || int64(n) > want {
			return original, ErrContent
		}
		if n > 0 {
			h.Write(b[:n])
			w, we := ops.write(f, b[:n])
			if we != nil || w != n {
				return original, ErrCleanup
			}
			count += int64(n)
		}
		if re != nil && re != io.EOF || re == io.EOF && count < a.Bytes || n == 0 {
			return original, ErrContent
		}
	}
	var probe [1]byte
	n, re := ops.read(r, probe[:])
	if n != 0 || re != io.EOF || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return original, ErrContent
	}
	if e = v.ctx.Err(); e != nil {
		return original, e
	}
	if ops.sync(f) != nil {
		return original, ErrCleanup
	}
	info, e := f.Stat()
	if e != nil || !os.SameFile(original, info) {
		return original, ErrRecovery
	}
	original = info
	ce := ops.close(f)
	closed = true
	if ce != nil {
		return original, ErrCleanup
	}
	if e = v.manager.unchanged(path, original); e != nil {
		return original, e
	}
	if ops.syncDir(v.manager.dir) != nil {
		return original, ErrCleanup
	}
	return original, nil
}
func (v *Maintenance) cleanupBundleOriginals(current []storage.LibraryAsset, owned map[string]os.FileInfo, ops bundleInstallIO) error {
	if len(owned) == 0 {
		return nil
	}
	m := v.manager
	if e := m.checkRetainedStage(); e != nil {
		return e
	}
	c, e := m.readRetained()
	if e != nil {
		return e
	}
	referenced := map[string]bool{}
	for _, a := range current {
		referenced[contentName(a.Scope, a.SHA256)] = true
	}
	for _, r := range c.Entries {
		referenced[r.key()] = true
	}
	var err error
	for k, original := range owned {
		if referenced[k] {
			continue
		}
		path := filepath.Join(m.dir, k)
		if e = m.unchanged(path, original); e != nil {
			err = errors.Join(err, e)
			continue
		}
		if ops.remove(path) != nil {
			err = errors.Join(err, ErrCleanup)
			continue
		}
		delete(m.identities, path)
	}
	if ops.syncDir(m.dir) != nil {
		err = errors.Join(err, ErrCleanup)
	}
	return err
}
