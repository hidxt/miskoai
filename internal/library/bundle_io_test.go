package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/librarybundle"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func bundleBytes(t *testing.T, f fixture) []byte {
	t.Helper()
	var b bytes.Buffer
	e := f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error { return v.WriteBundle(f.l, &b) })
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestBundleWriteRoundTrip(t *testing.T) {
	f := setup(t)
	upload(t, f, "png")
	upload(t, f, "gif")
	raw := bundleBytes(t, f)
	count := 0
	_, e := librarybundle.Decode(context.Background(), bytes.NewReader(raw), func(r librarybundle.Record, in io.Reader) error { count++; _, e := io.Copy(io.Discard, in); return e })
	if e != nil || count != 3 {
		t.Fatal(count, e)
	}
}
func TestBundlePrepareBeforeCallback(t *testing.T) {
	f := setup(t)
	upload(t, f, "gif")
	raw := bundleBytes(t, f)
	dest := setup(t)
	called := 0
	e := dest.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
		return v.PrepareBundle(bytes.NewReader(raw), func(p *PreparedBundle) error { called++; return p.valid(v) })
	})
	if e != nil || called != 1 {
		t.Fatal(called, e)
	}
	for _, bad := range [][]byte{raw[:len(raw)-1], append(append([]byte{}, raw...), 1)} {
		called = 0
		e = dest.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
			return v.PrepareBundle(bytes.NewReader(bad), func(*PreparedBundle) error { called++; return nil })
		})
		if e == nil || called != 0 {
			t.Fatal("bad input callback", called, e)
		}
	}
}
func TestBundlePreparedLifetime(t *testing.T) {
	f := setup(t)
	raw := bundleBytes(t, f)
	var kept *PreparedBundle
	var view *Maintenance
	e := f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
		view = v
		return v.PrepareBundle(bytes.NewReader(raw), func(p *PreparedBundle) error {
			kept = p
			copyP := *p
			if !errors.Is(copyP.valid(v), ErrInvalid) {
				t.Fatal("copied accepted")
			}
			return p.valid(v)
		})
	})
	if e != nil {
		t.Fatal(e)
	}
	if !errors.Is(kept.valid(view), ErrInvalid) {
		t.Fatal("expired accepted")
	}
}

func TestBundleCorrespondence(t *testing.T) {
	f := setup(t)
	upload(t, f, "png")
	upload(t, f, "gif")
	raw := bundleBytes(t, f)
	h, payloads := unpackBundle(t, raw)
	for _, tc := range []string{"missing", "extra", "foreign", "bytes", "digest", "corrupt-image", "raw-hash", "trailing", "truncated"} {
		t.Run(tc, func(t *testing.T) {
			ch := h
			ch.Assets = append([]librarybundle.Asset{}, h.Assets...)
			cp := make(map[string][]byte)
			for k, b := range payloads {
				cp[k] = append([]byte{}, b...)
			}
			switch tc {
			case "missing":
				ch.Assets = ch.Assets[:1]
			case "extra":
				ch.Assets = append(ch.Assets, librarybundle.Asset{Key: strings.Repeat("f", 64), Bytes: ch.Assets[0].Bytes, SHA256: ch.Assets[0].SHA256})
				cp[strings.Repeat("f", 64)] = cp[ch.Assets[0].Key]
			case "foreign":
				old := ch.Assets[0].Key
				ch.Assets[0].Key = strings.Repeat("0", 64)
				cp[ch.Assets[0].Key] = cp[old]
			case "bytes":
				r := &ch.Assets[0]
				cp[r.Key] = append(cp[r.Key], 0)
				r.Bytes++
				r.SHA256 = bundleSum(cp[r.Key])
			case "digest", "corrupt-image":
				r := &ch.Assets[0]
				cp[r.Key][0] ^= 0xff
				r.SHA256 = bundleSum(cp[r.Key])
			}
			sort.Slice(ch.Assets, func(i, j int) bool { return ch.Assets[i].Key < ch.Assets[j].Key })
			bad := packBundle(t, ch, cp)
			if tc == "raw-hash" {
				bad[len(bad)-1] ^= 1
			}
			if tc == "trailing" {
				bad = append(bad, 0)
			}
			if tc == "truncated" {
				bad = bad[:len(bad)-1]
			}
			dest := setup(t)
			calls := 0
			e := dest.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
				return v.PrepareBundle(bytes.NewReader(bad), func(*PreparedBundle) error { calls++; return nil })
			})
			if e == nil || calls != 0 {
				t.Fatal("unadmitted callback", calls, e)
			}
			rows, e := dest.s.LibrarySnapshot(context.Background())
			if e != nil || len(rows) != 0 {
				t.Fatal("destination mutation", e)
			}
		})
	}
}
func bundleSum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func unpackBundle(t *testing.T, raw []byte) (librarybundle.Header, map[string][]byte) {
	t.Helper()
	p := map[string][]byte{}
	h, e := librarybundle.Decode(context.Background(), bytes.NewReader(raw), func(r librarybundle.Record, in io.Reader) error { b, e := io.ReadAll(in); p[r.Key] = b; return e })
	if e != nil {
		t.Fatal(e)
	}
	return h, p
}
func packBundle(t *testing.T, h librarybundle.Header, p map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	e := librarybundle.Encode(context.Background(), &b, h, func(r librarybundle.Record) (io.Reader, func() error, error) {
		return bytes.NewReader(p[r.Key]), func() error { return nil }, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestBundleScopedMetadataAndDeleting(t *testing.T) {
	for _, state := range []string{"ready", "deleting-present", "deleting-absent", "deleting-corrupt"} {
		t.Run(state, func(t *testing.T) {
			f := setup(t)
			a := upload(t, f, "png")
			raw := rawImage(t, "png", 10)
			b, _, e := f.m.Upload(context.Background(), f.l, storage.Scope{Account: "account", User: "other"}, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{Tags: []string{" "}, Description: "disabled"})
			if e != nil {
				t.Fatal(e)
			}
			if state != "ready" {
				if e = f.s.MarkLibraryDeleting(context.Background(), scope, a.ID); e != nil {
					t.Fatal(e)
				}
			}
			if state == "deleting-absent" {
				if e = os.Remove(f.m.path(a)); e != nil {
					t.Fatal(e)
				}
			}
			if state == "deleting-corrupt" {
				if e = os.WriteFile(f.m.path(a), bytes.Repeat([]byte{1}, int(a.Bytes)), 0600); e != nil {
					t.Fatal(e)
				}
			}
			var out bytes.Buffer
			e = f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error { return v.WriteBundle(f.l, &out) })
			if state == "deleting-corrupt" {
				if e == nil {
					t.Fatal("corrupt export")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			dest := setup(t)
			e = dest.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
				return v.PrepareBundle(bytes.NewReader(out.Bytes()), func(p *PreparedBundle) error {
					if len(p.rows) != 2 || p.rows[1].ID != b.ID || p.rows[1].Enabled || p.rows[1].Tags[0] != " " {
						t.Fatal("metadata changed")
					}
					return nil
				})
			})
			if e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestBundleViewRefusesBeforeIO(t *testing.T) {
	f := setup(t)
	var expired *Maintenance
	e := f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
		expired = v
		copyV := *v
		bad := panicBundleIO{}
		for _, view := range []*Maintenance{nil, &copyV} {
			if !errors.Is(view.WriteBundle(f.l, bad), ErrInvalid) || !errors.Is(view.PrepareBundle(bad, func(*PreparedBundle) error { return nil }), ErrInvalid) {
				t.Fatal("invalid view")
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if !errors.Is(expired.WriteBundle(f.l, panicBundleIO{}), ErrInvalid) {
		t.Fatal("expired")
	}
}

type panicBundleIO struct{}

func (panicBundleIO) Read([]byte) (int, error)  { panic("unexpected read") }
func (panicBundleIO) Write([]byte) (int, error) { panic("unexpected write") }
func TestBundleLegacyRawHashBeforeMigration(t *testing.T) {
	f := setup(t)
	h, p := unpackBundle(t, bundleBytes(t, f))
	path := filepath.Join(f.dir, "legacy-source.db")
	writePrivate(t, path, p[""])
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("DROP TABLE image_replies; DROP TABLE library_assets; PRAGMA user_version=4"); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	p[""] = original
	h.Database = librarybundle.File{Bytes: int64(len(original)), SHA256: bundleSum(original)}
	wire := packBundle(t, h, p)
	for _, badHash := range []bool{true, false} {
		dest := setup(t)
		input := append([]byte{}, wire...)
		if badHash {
			input[len(input)-1] ^= 1
		}
		calls := 0
		processed := 0
		dest.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
			processed++
			return dest.p.WithLocalImage(c, r, n, fn)
		})
		e = dest.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
			return v.PrepareBundle(bytes.NewReader(input), func(b *PreparedBundle) error {
				calls++
				if len(b.rows) != 0 {
					t.Fatal("legacy not empty")
				}
				actual, e := os.ReadFile(b.stage.path("database"))
				if e != nil || !bytes.Equal(actual, original) {
					t.Fatal("raw bytes migrated", e)
				}
				digest, e := b.stage.digest(v.ctx, "database", bundleDBLimit)
				if e != nil || digest != h.Database.SHA256 {
					t.Fatal("wire identity lost", e)
				}
				return nil
			})
		})
		if badHash {
			if e == nil || calls != 0 || processed != 0 {
				t.Fatal("raw hash gate", calls, processed, e)
			}
		} else if e != nil || calls != 1 {
			t.Fatal("legacy prepare", calls, e)
		}
	}
}
func TestBundleCandidateDiscardFailureBlocksReservation(t *testing.T) {
	f := setup(t)
	wire := bundleBytes(t, f)
	var replacement string
	e := f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
		return v.PrepareBundle(bytes.NewReader(wire), func(*PreparedBundle) error {
			entries, e := os.ReadDir(f.dir)
			if e != nil {
				return e
			}
			for _, x := range entries {
				if strings.HasPrefix(x.Name(), "restore-") {
					replacement = filepath.Join(f.dir, x.Name())
					if e = os.Rename(replacement, replacement+"-original"); e != nil {
						return e
					}
					writePrivate(t, replacement, []byte("replacement"))
					return nil
				}
			}
			t.Fatal("candidate missing")
			return nil
		})
	})
	if !errors.Is(e, ErrRecovery) || replacement == "" {
		t.Fatal(e)
	}
	actual, e := os.ReadFile(replacement)
	if e != nil || string(actual) != "replacement" {
		t.Fatal("replacement removed", e)
	}
	e = f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
		return v.PrepareBundle(panicBundleIO{}, func(*PreparedBundle) error { return nil })
	})
	if !errors.Is(e, ErrRecovery) {
		t.Fatal("reservation not blocking", e)
	}
}
func TestBundleMissingReadyRefuses(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	if e := os.Remove(f.m.path(a)); e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	e := f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error { return v.WriteBundle(f.l, &b) })
	if !errors.Is(e, ErrRecovery) || b.Len() != 0 {
		t.Fatal("unchecked output", e, b.Len())
	}
}
func TestBundleFullImageGateAndHighID(t *testing.T) {
	f := setup(t)
	upload(t, f, "png")
	h, p := unpackBundle(t, bundleBytes(t, f))
	for _, corrupt := range []bool{false, true} {
		dest := setup(t)
		cp := map[string][]byte{}
		for k, b := range p {
			cp[k] = append([]byte{}, b...)
		}
		ch := h
		ch.Assets = append([]librarybundle.Asset{}, h.Assets...)
		path := filepath.Join(dest.dir, "source.db")
		writePrivate(t, path, cp[""])
		db, e := sql.Open("sqlite", path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec("UPDATE library_assets SET id=9223372036854775807; UPDATE sqlite_sequence SET seq=9223372036854775807 WHERE name='library_assets'"); e != nil {
			t.Fatal(e)
		}
		if corrupt {
			old := ch.Assets[0].Key
			cp[old][0] ^= 255
			digest := bundleSum(cp[old])
			if _, e = db.Exec("UPDATE library_assets SET sha256=?", digest); e != nil {
				t.Fatal(e)
			}
			key := strings.TrimPrefix(contentName(scope, digest), "asset-")
			cp[key] = cp[old]
			ch.Assets[0].SHA256 = digest
			ch.Assets[0].Key = key
		}
		if e = db.Close(); e != nil {
			t.Fatal(e)
		}
		cp[""], e = os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		ch.Database = librarybundle.File{Bytes: int64(len(cp[""])), SHA256: bundleSum(cp[""])}
		calls := 0
		actualProcessor := 0
		dest.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
			actualProcessor++
			return dest.p.WithLocalImage(c, r, n, fn)
		})
		wire := packBundle(t, ch, cp)
		e = dest.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error {
			return v.PrepareBundle(bytes.NewReader(wire), func(b *PreparedBundle) error {
				calls++
				if b.rows[0].ID != 9223372036854775807 {
					t.Fatal("high ID changed")
				}
				return nil
			})
		})
		if actualProcessor != 1 {
			t.Fatal("real processor not reached", actualProcessor, e)
		}
		if corrupt {
			if e == nil || calls != 0 {
				t.Fatal("corrupt image reached callback", calls, e)
			}
		} else if e != nil || calls != 1 {
			t.Fatal("valid high ID", calls, e)
		}
	}
}
func TestBundleLeaseReleaseBeforeImageValidation(t *testing.T) {
	f := setup(t)
	upload(t, f, "png")
	held := false
	lease := func(context.Context) (*storage.Store, func(), error) {
		if held {
			t.Fatal("recursive lease")
		}
		held = true
		return f.s, func() { held = false }, nil
	}
	calls := 0
	f.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
		if held {
			t.Fatal("image validation holds Store")
		}
		calls++
		return f.p.WithLocalImage(c, r, n, fn)
	})
	var b bytes.Buffer
	e := f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error { return v.WriteBundle(lease, &b) })
	if e != nil || held || calls != 1 {
		t.Fatal("lease order", calls, held, e)
	}
}
func TestBundleRetainedContentExcluded(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	rows, e := f.s.LibrarySnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if e = f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error { return v.RetainCurrent(rows) }); e != nil {
		t.Fatal(e)
	}
	if e = f.s.MarkLibraryDeleting(context.Background(), scope, a.ID); e != nil {
		t.Fatal(e)
	}
	if e = f.s.FinishLibraryDelete(context.Background(), scope, a.ID); e != nil {
		t.Fatal(e)
	}
	h, _ := unpackBundle(t, bundleBytes(t, f))
	if len(h.Assets) != 0 {
		t.Fatal("retained records exported as current")
	}
	if _, e = os.Lstat(f.m.path(a)); e != nil {
		t.Fatal("retained original removed", e)
	}
}
