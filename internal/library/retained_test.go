package library

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	directory "github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/storage"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedProtectedDeleteRestartUpload(t *testing.T) {
	for _, format := range []string{"png", "gif"} {
		t.Run(format, func(t *testing.T) {
			f := setup(t)
			a := upload(t, f, format)
			retain(t, f)
			if e := f.m.Delete(context.Background(), f.l, scope, a.ID); e != nil {
				t.Fatal(e)
			}
			if _, e := os.Stat(f.m.path(a)); e != nil {
				t.Fatal("protected bytes lost", e)
			}
			if e := f.m.Close(); e != nil {
				t.Fatal(e)
			}
			m, e := New(f.dir, f.p)
			if e != nil {
				t.Fatal(e)
			}
			defer m.Close()
			raw := rawImage(t, "png", 99)
			_, added, e := m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
			if e != nil || !added {
				t.Fatal("retained restart refuses safe upload", e, added)
			}
		})
	}
}
func TestRetainedDeleteFailureProtection(t *testing.T) {
	for _, kind := range []string{"row-failure", "missing", "corrupt"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			a := upload(t, f, "gif")
			retain(t, f)
			raw, e := os.ReadFile(f.m.path(a))
			if e != nil {
				t.Fatal(e)
			}
			l := f.l
			switch kind {
			case "row-failure":
				calls := 0
				l = func(ctx context.Context) (*storage.Store, func(), error) {
					calls++
					if calls == 3 {
						return nil, nil, storage.ErrStorage
					}
					return f.l(ctx)
				}
			case "missing":
				if e = os.Remove(f.m.path(a)); e != nil {
					t.Fatal(e)
				}
			case "corrupt":
				raw[len(raw)/2] ^= 1
				if e = os.WriteFile(f.m.path(a), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if e = f.m.Delete(context.Background(), l, scope, a.ID); e == nil {
				t.Fatal("failed deletion accepted")
			}
			row, e := f.s.LibraryAsset(context.Background(), scope, a.ID)
			if e != nil {
				t.Fatal("row silently removed", e)
			}
			if kind != "missing" {
				got, e := os.ReadFile(f.m.path(a))
				if e != nil || !bytes.Equal(got, raw) {
					t.Fatal("protected bytes removed", e)
				}
			}
			if kind == "row-failure" {
				if row.State != "deleting" || row.Enabled {
					t.Fatal("tombstone missing")
				}
				if e = f.m.Delete(context.Background(), f.l, scope, a.ID); e != nil {
					t.Fatal("safe retry", e)
				}
				if _, e = os.Stat(f.m.path(a)); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
func TestRetainedMissingDeletingNoReference(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	if e := f.s.MarkLibraryDeleting(context.Background(), scope, a.ID); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(f.m.path(a)); e != nil {
		t.Fatal(e)
	}
	retain(t, f)
	c, e := f.m.readRetained()
	if e != nil || len(c.Entries) != 0 {
		t.Fatal(e, len(c.Entries))
	}
}
func TestRetainedPhysicalExactCapacity(t *testing.T) {
	for _, kind := range []string{"count128", "bytes64"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			raw := rawImage(t, "png", 10)
			count := 128
			if kind == "bytes64" {
				count = 16
				payload := make([]byte, (4<<20)-len(raw)-12)
				var chunk bytes.Buffer
				binary.Write(&chunk, binary.BigEndian, uint32(len(payload)))
				chunk.WriteString("tEXt")
				chunk.Write(payload)
				sum := crc32.ChecksumIEEE(chunk.Bytes()[4:])
				binary.Write(&chunk, binary.BigEndian, sum)
				padded := append([]byte{}, raw[:len(raw)-12]...)
				padded = append(padded, chunk.Bytes()...)
				raw = append(padded, raw[len(raw)-12:]...)
			}
			a, added, e := f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
			if e != nil || !added {
				t.Fatal("real PNG validation", e, added)
			}
			records := map[string]retainedRecord{}
			rows := []storage.LibraryAsset{a}
			for i := 0; i < count; i++ {
				b := a
				if i > 0 {
					b.Scope.User = fmt.Sprint("retained-", i)
					writePrivate(t, f.m.path(b), raw)
				}
				r := record(b)
				records[r.key()] = r
			}
			c, e := orderedCatalog(records)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.m.physicalWith(context.Background(), rows, 0, c); e != nil {
				t.Fatal("exact physical capacity / overlap", e)
			}
			if e = f.m.physicalWith(context.Background(), rows, 1, c); !errors.Is(e, ErrCapacity) {
				t.Fatal("physical +1 accepted", e)
			}
		})
	}
}
func TestRetainedPublicationFailures(t *testing.T) {
	for _, kind := range []string{"write", "sync", "close", "rename", "directory-sync", "stage-replaced", "destination-appeared"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			a := upload(t, f, "png")
			r := record(a)
			c, e := orderedCatalog(map[string]retainedRecord{r.key(): r})
			if e != nil {
				t.Fatal(e)
			}
			fail := errors.New("synthetic private operation")
			ops := retainedIO{write: func(h *os.File, b []byte) (int, error) { return h.Write(b) }, sync: func(h *os.File) error { return h.Sync() }, close: func(h *os.File) error { return h.Close() }, rename: os.Rename, syncDir: directory.SyncPrivateDirectory}
			switch kind {
			case "write":
				ops.write = func(h *os.File, b []byte) (int, error) {
					n, e := h.Write(b[:len(b)/2])
					if e != nil {
						return n, e
					}
					return n, fail
				}
			case "sync":
				ops.sync = func(h *os.File) error {
					if e := h.Sync(); e != nil {
						return e
					}
					return fail
				}
			case "close":
				ops.close = func(h *os.File) error {
					if e := h.Close(); e != nil {
						return e
					}
					return fail
				}
			case "rename":
				ops.rename = func(_, _ string) error { return fail }
			case "directory-sync":
				ops.syncDir = func(dir string) error {
					if e := directory.SyncPrivateDirectory(dir); e != nil {
						return e
					}
					return fail
				}
			case "stage-replaced":
				ops.close = func(h *os.File) error {
					if e := h.Close(); e != nil {
						return e
					}
					path := filepath.Join(f.dir, ".library-retained.pending")
					if e := os.Remove(path); e != nil {
						return e
					}
					writePrivate(t, path, []byte("replacement"))
					return nil
				}
			case "destination-appeared":
				ops.close = func(h *os.File) error {
					if e := h.Close(); e != nil {
						return e
					}
					writePrivate(t, f.m.catalogPath(), []byte(`{"version":1,"entries":[]}`))
					return nil
				}
			}
			e = f.m.writeRetainedWith(context.Background(), c, ops)
			if e == nil {
				t.Fatal("failure accepted")
			}
			if strings.Contains(e.Error(), "synthetic") || strings.Contains(e.Error(), f.dir) {
				t.Fatal("unsafe error", e)
			}
			stage := filepath.Join(f.dir, ".library-retained.pending")
			if kind == "stage-replaced" {
				b, _ := os.ReadFile(stage)
				if string(b) != "replacement" {
					t.Fatal("replacement cleaned")
				}
			} else if _, e := os.Lstat(stage); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("owned stage remains", e)
			}
			if kind == "directory-sync" {
				got, e := f.m.readRetained()
				if e != nil || len(got.Entries) != 1 {
					t.Fatal("published protections lost", e)
				}
			} else if kind != "destination-appeared" {
				if _, e := os.Stat(f.m.catalogPath()); !errors.Is(e, os.ErrNotExist) {
					t.Fatal("catalog unexpectedly published", e)
				}
			}
		})
	}
}
func TestRetainedRowAdmission(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "gif")
	for _, kind := range []string{"id", "duplicate-id", "duplicate-content", "digest", "format", "dimensions", "frames", "bytes", "scope", "state", "enabled-deleting", "metadata", "usage", "missing-ready", "missing-deleting"} {
		t.Run(kind, func(t *testing.T) {
			rows := []storage.LibraryAsset{a}
			switch kind {
			case "id":
				rows[0].ID = 0
			case "duplicate-id":
				b := a
				b.SHA256 = strings.Repeat("a", 64)
				rows = append(rows, b)
			case "duplicate-content":
				b := a
				b.ID++
				rows = append(rows, b)
			case "digest":
				rows[0].SHA256 = "invalid"
			case "format":
				rows[0].Format = "svg"
			case "dimensions":
				rows[0].Width = 8193
			case "frames":
				rows[0].Frames = 33
			case "bytes":
				rows[0].Bytes = 4<<20 + 1
			case "scope":
				rows[0].Scope.User = " "
			case "state":
				rows[0].State = "other"
			case "enabled-deleting":
				rows[0].State = "deleting"
				rows[0].Enabled = true
			case "metadata":
				rows[0].Description = strings.Repeat("x", 1025)
			case "usage":
				rows[0].Uses = -1
			case "missing-ready", "missing-deleting":
				rows[0].SHA256 = strings.Repeat("a", 64)
				if kind == "missing-deleting" {
					rows[0].State = "deleting"
				}
			}
			e := f.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error { return v.RetainCurrent(rows) })
			if kind == "missing-deleting" { // An unrelated current file cannot be adopted from directory enumeration.
				if !errors.Is(e, ErrRecovery) {
					t.Fatal("unknown content accepted", e)
				}
			} else if e == nil {
				t.Fatal("invalid row accepted")
			}
		})
	}
}
func TestRetainedCatalogGrammar(t *testing.T) {
	r := retainedRecord{Account: "a", User: "u", SHA256: strings.Repeat("a", 64), Format: "png", Bytes: 1, Width: 1, Height: 1, Frames: 1}
	c, e := orderedCatalog(map[string]retainedRecord{r.key(): r})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(c)
	if _, e = decodeRetained(raw); e != nil {
		t.Fatal(e)
	}
	bad := []string{string(raw) + " ", strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1), strings.Replace(string(raw), `"account"`, `"acc\u006funt"`, 1), strings.Replace(string(raw), `"bytes":1`, `"bytes":1.0`, 1), strings.Replace(string(raw), `"account":"a"`, `"account":null`, 1), strings.Replace(string(raw), `"width":1`, `"width":9223372036854775808`, 1), strings.Replace(string(raw), `"format":"png"`, `"format":"png","unknown":1`, 1), `{"version":1,"entries":null}`, string(raw) + "{}", strings.Replace(string(raw), `"a"`, `"\ud800"`, 1)}
	for i, v := range bad {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, e := decodeRetained([]byte(v)); !errors.Is(e, ErrRecovery) {
				t.Fatal("malformed accepted", e)
			}
		})
	}
	if _, e := decodeRetained(bytes.Repeat([]byte("x"), int(retainedLimit+1))); !errors.Is(e, ErrRecovery) {
		t.Fatal(e)
	}
	records := map[string]retainedRecord{}
	for i := 0; i < 128; i++ {
		x := r
		x.SHA256 = fmt.Sprintf("%064x", i)
		records[x.key()] = x
	}
	if _, e := orderedCatalog(records); e != nil {
		t.Fatal(e)
	}
	x := r
	x.SHA256 = strings.Repeat("f", 64)
	records[x.key()] = x
	if _, e := orderedCatalog(records); !errors.Is(e, ErrCapacity) {
		t.Fatal(e)
	}
	records = map[string]retainedRecord{}
	for i := 0; i < 16; i++ {
		x := r
		x.SHA256 = fmt.Sprintf("%064x", i)
		x.Bytes = 4 << 20
		records[x.key()] = x
	}
	if _, e := orderedCatalog(records); e != nil {
		t.Fatal(e)
	}
	x.Bytes = 1
	records[x.key()] = x
	if _, e := orderedCatalog(records); !errors.Is(e, ErrCapacity) {
		t.Fatal(e)
	}
}
func TestRetainedRecoveryRefusals(t *testing.T) {
	for _, kind := range []string{"unknown", "missing", "corrupt", "catalog-invalid", "catalog-replaced", "pending", "overlap", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			a := upload(t, f, "png")
			retain(t, f)
			before, e := os.ReadFile(f.m.path(a))
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "unknown":
				writePrivate(t, filepath.Join(f.m.dir, "unknown"), []byte("untouched"))
			case "missing":
				if e = os.Remove(f.m.path(a)); e != nil {
					t.Fatal(e)
				}
			case "corrupt":
				if e = os.Remove(f.m.path(a)); e != nil {
					t.Fatal(e)
				}
				writePrivate(t, f.m.path(a), bytes.Repeat([]byte("x"), len(before)))
			case "catalog-invalid":
				if e = os.Remove(f.m.catalogPath()); e != nil {
					t.Fatal(e)
				}
				writePrivate(t, f.m.catalogPath(), []byte(`{"version":1,"entries":null}`))
			case "catalog-replaced":
				raw, e := os.ReadFile(f.m.catalogPath())
				if e != nil {
					t.Fatal(e)
				}
				if e = os.Remove(f.m.catalogPath()); e != nil {
					t.Fatal(e)
				}
				writePrivate(t, f.m.catalogPath(), raw)
			case "pending":
				writePrivate(t, filepath.Join(f.dir, ".library-retained.pending"), []byte("uncertain"))
				raw := rawImage(t, "gif", 99)
				if _, _, e = f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{}); !errors.Is(e, ErrRecovery) {
					t.Fatal("pending permits upload", e)
				}
				if e = f.m.Delete(context.Background(), f.l, scope, a.ID); !errors.Is(e, ErrRecovery) {
					t.Fatal("pending permits deletion", e)
				}
				if e = f.m.WithMaintenance(context.Background(), func(ctx context.Context, v *Maintenance) error {
					rows, e := f.s.LibrarySnapshot(ctx)
					if e != nil {
						return e
					}
					return v.RetainCurrent(rows)
				}); !errors.Is(e, ErrRecovery) {
					t.Fatal(e)
				}
				raw, _ = os.ReadFile(filepath.Join(f.dir, ".library-retained.pending"))
				if string(raw) != "uncertain" {
					t.Fatal("pending removed")
				}
				return
			case "overlap":
				rows, _ := f.s.LibrarySnapshot(context.Background())
				rows[0].Width++
				if e = f.m.physical(rows, 0); !errors.Is(e, ErrRecovery) {
					t.Fatal(e)
				}
				return
			case "hardlink":
				if e = os.Link(f.m.catalogPath(), filepath.Join(f.dir, "linked")); e != nil {
					t.Fatal("authentic hardlink", e)
				}
			}
			raw := rawImage(t, "gif", 99)
			_, _, e = f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
			if !errors.Is(e, ErrRecovery) {
				t.Fatal("unsafe upload accepted", e)
			}
			if kind == "unknown" {
				raw, _ := os.ReadFile(filepath.Join(f.m.dir, "unknown"))
				if string(raw) != "untouched" {
					t.Fatal("swept")
				}
			}
		})
	}
}
func TestRetainedReuseAndScope(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	retain(t, f)
	if e := f.m.Delete(context.Background(), f.l, scope, a.ID); e != nil {
		t.Fatal(e)
	}
	raw := rawImage(t, "png", 10)
	b, added, e := f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
	if e != nil || !added {
		t.Fatal(e, added)
	}
	if b.SHA256 != a.SHA256 {
		t.Fatal("identity")
	}
	other := storage.Scope{Account: "other", User: "user"}
	_, added, e = f.m.Upload(context.Background(), f.l, other, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
	if e != nil || !added {
		t.Fatal(e, added)
	}
	retain(t, f)
	c, e := f.m.readRetained()
	if e != nil || len(c.Entries) != 2 {
		t.Fatal(e, len(c.Entries))
	}
}
