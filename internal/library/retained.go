package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/attachment"
	directory "github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"
)

const retainedLimit int64 = 512 << 10

type retainedRecord struct {
	Account string `json:"account"`
	User    string `json:"user"`
	SHA256  string `json:"sha256"`
	Format  string `json:"format"`
	Bytes   int64  `json:"bytes"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Frames  int    `json:"frames"`
}
type retainedCatalog struct {
	Version int              `json:"version"`
	Entries []retainedRecord `json:"entries"`
}

func record(a storage.LibraryAsset) retainedRecord {
	return retainedRecord{a.Scope.Account, a.Scope.User, a.SHA256, a.Format, a.Bytes, a.Width, a.Height, a.Frames}
}
func (r retainedRecord) asset() storage.LibraryAsset {
	return storage.LibraryAsset{Scope: storage.Scope{Account: r.Account, User: r.User}, SHA256: r.SHA256, Format: r.Format, Bytes: r.Bytes, Width: r.Width, Height: r.Height, Frames: r.Frames, State: "ready"}
}
func (r retainedRecord) key() string { return contentName(r.asset().Scope, r.SHA256) }
func (r retainedRecord) valid() bool {
	return validScope(r.asset().Scope) && validDigest(r.SHA256) && r.Bytes > 0 && r.Bytes <= 4<<20 && r.Width > 0 && r.Width <= 8192 && r.Height > 0 && r.Height <= 8192 && int64(r.Width)*int64(r.Height) <= 16000000 && r.Frames > 0 && r.Frames <= 32 && (r.Format == "gif" || (r.Format == "png" || r.Format == "jpeg") && r.Frames == 1)
}
func validCurrent(rows []storage.LibraryAsset) error {
	if len(rows) > 128 {
		return ErrCapacity
	}
	ids := map[int64]bool{}
	keys := map[string]bool{}
	var total int64
	for _, a := range rows {
		r := record(a)
		if a.ID < 1 || ids[a.ID] || !r.valid() || !validMeta(storage.LibraryMetadata{Tags: a.Tags, Emotion: a.Emotion, Description: a.Description}) || a.Uses < 0 || a.LastUsedMillis < 0 || a.LastUsedMillis > 253402300799999 || (a.State != "ready" && a.State != "deleting") || a.State == "deleting" && a.Enabled {
			return ErrInvalid
		}
		key := r.key()
		if keys[key] {
			return ErrInvalid
		}
		ids[a.ID] = true
		keys[key] = true
		total += a.Bytes
	}
	if total > 64<<20 {
		return ErrCapacity
	}
	return nil
}
func orderedCatalog(records map[string]retainedRecord) (retainedCatalog, error) {
	c := retainedCatalog{Version: 1, Entries: make([]retainedRecord, 0, len(records))}
	if len(records) > 128 {
		return c, ErrCapacity
	}
	keys := make([]string, 0, len(records))
	for k := range records {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var total int64
	for _, k := range keys {
		r := records[k]
		if !r.valid() || r.key() != k {
			return c, ErrRecovery
		}
		total += r.Bytes
		if total > 64<<20 {
			return c, ErrCapacity
		}
		c.Entries = append(c.Entries, r)
	}
	return c, nil
}

// Decode tokens before appending each bounded record; canonical comparison
// rejects duplicate/escaped aliases, nulls, whitespace and alternate numbers.
func decodeRetained(raw []byte) (retainedCatalog, error) {
	c := retainedCatalog{Version: 1, Entries: []retainedRecord{}}
	if int64(len(raw)) > retainedLimit || !utf8.Valid(raw) {
		return c, ErrRecovery
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return c, ErrRecovery
	}
	tok, e = d.Token()
	if e != nil || tok != "version" {
		return c, ErrRecovery
	}
	if d.Decode(&c.Version) != nil || c.Version != 1 {
		return c, ErrRecovery
	}
	tok, e = d.Token()
	if e != nil || tok != "entries" {
		return c, ErrRecovery
	}
	tok, e = d.Token()
	if e != nil || tok != json.Delim('[') {
		return c, ErrRecovery
	}
	prev := ""
	var total int64
	for d.More() {
		if len(c.Entries) >= 128 {
			return c, ErrRecovery
		}
		var r retainedRecord
		if d.Decode(&r) != nil || !r.valid() || r.key() <= prev {
			return c, ErrRecovery
		}
		prev = r.key()
		total += r.Bytes
		if total > 64<<20 {
			return c, ErrRecovery
		}
		c.Entries = append(c.Entries, r)
	}
	tok, e = d.Token()
	if e != nil || tok != json.Delim(']') {
		return c, ErrRecovery
	}
	tok, e = d.Token()
	if e != nil || tok != json.Delim('}') {
		return c, ErrRecovery
	}
	canonical, e := json.Marshal(c)
	if e != nil || !bytes.Equal(raw, canonical) {
		return c, ErrRecovery
	}
	return c, nil
}
func (m *Manager) catalogPath() string { return filepath.Join(m.dataDir, "library-retained.json") }
func (m *Manager) checkRetainedStage() error {
	_, e := os.Lstat(filepath.Join(m.dataDir, ".library-retained.pending"))
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	return ErrRecovery
}
func (m *Manager) readRetained() (c retainedCatalog, err error) {
	c = retainedCatalog{Version: 1, Entries: []retainedRecord{}}
	path := m.catalogPath()
	if privatefs.CheckDir(m.dataDir) != nil {
		return c, ErrRecovery
	}
	f, e := privatefs.OpenRead(path, retainedLimit)
	if e != nil {
		if _, se := os.Lstat(path); errors.Is(se, os.ErrNotExist) && m.retainedIdentity == nil {
			return c, nil
		}
		return c, ErrRecovery
	}
	defer func() {
		if f.Close() != nil {
			err = errors.Join(err, ErrCleanup)
		}
	}()
	original, e := f.Stat()
	if e != nil || original.Size() > retainedLimit || m.retainedIdentity != nil && !sameFile(original, m.retainedIdentity) {
		return c, ErrRecovery
	}
	raw, e := io.ReadAll(io.LimitReader(f, retainedLimit+1))
	if e != nil {
		return c, ErrRecovery
	}
	c, e = decodeRetained(raw)
	if e != nil {
		return c, e
	}
	actual, e := f.Stat()
	named, se := os.Lstat(path)
	if e != nil || se != nil || !sameFile(original, actual) || !sameFile(original, named) || privatefs.CheckFile(path, retainedLimit) != nil {
		return c, ErrRecovery
	}
	m.retainedIdentity = original
	return c, nil
}
func noContent(attachment.LocalImage, attachment.Artifact) error { return nil }
func (m *Manager) writeRetained(ctx context.Context, c retainedCatalog) (err error) {
	return m.writeRetainedWith(ctx, c, retainedIO{write: func(f *os.File, b []byte) (int, error) { return f.Write(b) }, sync: func(f *os.File) error { return f.Sync() }, close: func(f *os.File) error { return f.Close() }, rename: os.Rename, syncDir: directory.SyncPrivateDirectory})
}

// Per-call seams wrap actual owned operations in synthetic failure tests.
type retainedIO struct {
	write   func(*os.File, []byte) (int, error)
	sync    func(*os.File) error
	close   func(*os.File) error
	rename  func(string, string) error
	syncDir func(string) error
}

func (m *Manager) writeRetainedWith(ctx context.Context, c retainedCatalog, ops retainedIO) (err error) {
	expected := m.retainedIdentity
	raw, e := json.Marshal(c)
	if e != nil || int64(len(raw)) > retainedLimit {
		return ErrRecovery
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	stage := filepath.Join(m.dataDir, ".library-retained.pending")
	f, e := privatefs.Create(stage)
	if e != nil {
		return ErrRecovery
	}
	original, e := f.Stat()
	if e != nil {
		ce := f.Close()
		return safe(errors.Join(ErrRecovery, ce))
	}
	published := false
	closed := false
	defer func() {
		if !closed {
			if ops.close(f) != nil {
				err = errors.Join(err, ErrCleanup)
			}
		}
		if !published {
			named, e := os.Lstat(stage)
			if e == nil && sameFile(original, named) && privatefs.CheckFile(stage, retainedLimit) == nil {
				if os.Remove(stage) != nil || ops.syncDir(m.dataDir) != nil {
					err = errors.Join(err, ErrCleanup)
				}
			} else {
				err = errors.Join(err, ErrRecovery)
			}
		}
		err = safe(err)
	}()
	n, we := ops.write(f, raw)
	info, se := f.Stat()
	if se != nil || !os.SameFile(original, info) {
		return ErrRecovery
	}
	original = info
	if we != nil || n != len(raw) {
		return ErrCleanup
	}
	if ops.sync(f) != nil {
		return ErrCleanup
	}
	ce := ops.close(f)
	closed = true
	if ce != nil {
		return ErrCleanup
	}
	named, e := os.Lstat(stage)
	if e != nil || !sameFile(original, named) || privatefs.CheckFile(stage, retainedLimit) != nil {
		return ErrRecovery
	}
	// Checked reread establishes that the original catalog has not changed.
	if _, e = m.readRetained(); e != nil {
		return e
	}
	if expected == nil && m.retainedIdentity != nil || expected != nil && !sameFile(expected, m.retainedIdentity) {
		return ErrRecovery
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	target := m.catalogPath()
	named, e = os.Lstat(target)
	if m.retainedIdentity == nil {
		if !errors.Is(e, os.ErrNotExist) {
			return ErrRecovery
		}
	} else if e != nil || !sameFile(m.retainedIdentity, named) {
		return ErrRecovery
	}
	if ops.rename(stage, target) != nil {
		return ErrCleanup
	}
	published = true
	m.retainedIdentity = original
	if ops.syncDir(m.dataDir) != nil {
		return ErrCleanup
	}
	got, e := m.readRetained()
	if e != nil {
		return e
	}
	check, _ := json.Marshal(got)
	if !bytes.Equal(check, raw) {
		return ErrRecovery
	}
	return nil
}
