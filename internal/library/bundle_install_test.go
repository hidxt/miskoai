package library

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/librarybundle"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/storage"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func installBundleFixture(t *testing.T, f fixture, raw []byte) (maintenance.RestoreResult, error) {
	t.Helper()
	lock, e := maintenance.Acquire(f.dir)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	var result maintenance.RestoreResult
	e = f.m.WithMaintenance(context.Background(), func(ctx context.Context, v *Maintenance) error {
		rows, e := f.s.LibrarySnapshot(ctx)
		if e != nil {
			return e
		}
		return v.PrepareBundle(bytes.NewReader(raw), func(p *PreparedBundle) error {
			result, e = v.InstallBundle(p, rows, func(c *maintenance.Candidate) (maintenance.RestoreResult, error) {
				if e := f.s.Close(); e != nil {
					return maintenance.RestoreResult{}, e
				}
				if e := os.Rename(filepath.Join(f.dir, "state.db"), filepath.Join(f.dir, "miskoai.db")); e != nil {
					return maintenance.RestoreResult{}, e
				}
				return maintenance.InstallRestore(ctx, lock, c)
			})
			return e
		})
	})
	return result, e
}
func assertInstalled(t *testing.T, f fixture, want int) {
	t.Helper()
	s, e := storage.Open(filepath.Join(f.dir, "miskoai.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	rows, e := s.LibrarySnapshot(context.Background())
	if e != nil || len(rows) != want {
		t.Fatal("installed rows", len(rows), e)
	}
	lease := func(context.Context) (*storage.Store, func(), error) { return s, func() {}, nil }
	for _, a := range rows {
		if e := f.m.WithAsset(context.Background(), lease, a.Scope, a.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error { return nil }); e != nil {
			t.Fatal("installed bytes", e)
		}
	}
}
func TestBundleInstallRoundTrip(t *testing.T) {
	source := setup(t)
	upload(t, source, "png")
	upload(t, source, "gif")
	raw := bundleBytes(t, source)
	dest := setup(t)
	r, e := installBundleFixture(t, dest, raw)
	if e != nil {
		t.Fatal("install after successful preparation", e)
	}
	if !r.PreviousRetained || !r.ChannelPaused {
		t.Fatal(r)
	}
	assertInstalled(t, dest, 2)
}
func TestBundleInstallRetainsPrior(t *testing.T) {
	source := setup(t)
	upload(t, source, "png")
	raw := bundleBytes(t, source)
	dest := setup(t)
	prior := upload(t, dest, "gif")
	_, e := installBundleFixture(t, dest, raw)
	if e != nil {
		t.Fatal("install after successful preparation", e)
	}
	c, e := dest.m.readRetained()
	if e != nil || len(c.Entries) != 1 || c.Entries[0] != record(prior) {
		t.Fatal("prior not retained", c, e)
	}
	if e := dest.m.validate(context.Background(), prior, noContent); e != nil {
		t.Fatal(e)
	}
	assertInstalled(t, dest, 1)
}
func TestBundleInstallMissingKnownRepair(t *testing.T) {
	source := setup(t)
	upload(t, source, "png")
	raw := bundleBytes(t, source)
	dest := setup(t)
	a := upload(t, dest, "png")
	if e := os.Remove(dest.m.path(a)); e != nil {
		t.Fatal(e)
	}
	_, e := installBundleFixture(t, dest, raw)
	if e != nil {
		t.Fatal("exact repair after successful preparation", e)
	}
	assertInstalled(t, dest, 1)
}

func preparedInstall(t *testing.T, dest fixture, raw []byte, fn func(context.Context, *Maintenance, *PreparedBundle, []storage.LibraryAsset) error) error {
	t.Helper()
	return dest.m.WithMaintenance(context.Background(), func(ctx context.Context, v *Maintenance) error {
		rows, e := dest.s.LibrarySnapshot(ctx)
		if e != nil {
			return e
		}
		return v.PrepareBundle(bytes.NewReader(raw), func(p *PreparedBundle) error { return fn(ctx, v, p, rows) })
	})
}
func TestBundleInstallRefusals(t *testing.T) {
	source := setup(t)
	incoming := upload(t, source, "png")
	raw := bundleBytes(t, source)
	for _, kind := range []string{"unknown", "corrupt", "replaced", "missing-other", "pending", "stage-unknown", "stage-replaced", "overlap"} {
		t.Run(kind, func(t *testing.T) {
			dest := setup(t)
			var a storage.LibraryAsset
			if kind == "unknown" {
				writePrivate(t, dest.m.path(incoming), rawImage(t, "png", 10))
			} else if kind == "corrupt" || kind == "replaced" || kind == "overlap" {
				a = upload(t, dest, "png")
			} else if kind == "missing-other" {
				a = upload(t, dest, "gif")
				if e := os.Remove(dest.m.path(a)); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "corrupt" {
				if e := os.WriteFile(dest.m.path(a), bytes.Repeat([]byte{1}, int(a.Bytes)), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "replaced" {
				if e := os.Rename(dest.m.path(a), dest.m.path(a)+"-old"); e != nil {
					t.Fatal(e)
				}
				writePrivate(t, dest.m.path(a), rawImage(t, "png", 10))
				if e := os.Remove(dest.m.path(a) + "-old"); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "pending" {
				writePrivate(t, filepath.Join(dest.dir, ".library-retained.pending"), []byte("uncertain"))
			}
			calls := 0
			e := preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
				if kind == "stage-unknown" {
					writePrivate(t, p.stage.path("unknown"), []byte("uncertain"))
				}
				if kind == "stage-replaced" {
					k := strings.TrimPrefix(contentName(incoming.Scope, incoming.SHA256), "asset-")
					if e := os.Rename(p.stage.path(k), p.stage.path(k)+"-old"); e != nil {
						t.Fatal(e)
					}
					writePrivate(t, p.stage.path(k), rawImage(t, "png", 10))
				}
				if kind == "overlap" {
					rows[0].Width++
				}
				_, e := v.InstallBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
					calls++
					return maintenance.RestoreResult{}, errors.New("unexpected callback")
				})
				return e
			})
			if e == nil || calls != 0 {
				t.Fatal("effect before refusal", kind, calls, e)
			}
			if kind == "unknown" {
				if _, e := os.Stat(dest.m.path(incoming)); e != nil {
					t.Fatal("unknown swept", e)
				}
			}
		})
	}
}
func TestBundleInstallObjectLifetime(t *testing.T) {
	source := setup(t)
	raw := bundleBytes(t, source)
	dest := setup(t)
	var kept *PreparedBundle
	var expired *Maintenance
	e := preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
		kept = p
		expired = v
		cb := func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
			t.Fatal("unexpected callback")
			return maintenance.RestoreResult{}, nil
		}
		copyP := *p
		copyV := *v
		for _, pair := range []struct {
			v *Maintenance
			p *PreparedBundle
		}{{nil, p}, {&copyV, p}, {v, nil}, {v, &copyP}, {&Maintenance{}, p}} {
			if _, e := pair.v.InstallBundle(pair.p, rows, cb); !errors.Is(e, ErrInvalid) {
				t.Fatal(e)
			}
		}
		if _, e := v.InstallBundle(p, rows, nil); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
		bad := append([]storage.LibraryAsset{}, rows...)
		bad = append(bad, storage.LibraryAsset{})
		if _, e := v.InstallBundle(p, bad, cb); !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
		if _, e := v.InstallBundle(p, rows, cb); !errors.Is(e, ErrInvalid) {
			t.Fatal("twice used", e)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := expired.InstallBundle(kept, nil, func(*maintenance.Candidate) (maintenance.RestoreResult, error) { panic("expired callback") }); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
func TestBundleInstallOperationFailures(t *testing.T) {
	source := setup(t)
	a := upload(t, source, "png")
	raw := bundleBytes(t, source)
	for _, kind := range []string{"read", "write", "sync", "close", "dir", "remove", "replacement"} {
		t.Run(kind, func(t *testing.T) {
			dest := setup(t)
			calls := 0
			e := preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
				ops := defaultBundleInstallIO()
				fault := errors.New("synthetic secret must not escape")
				switch kind {
				case "read":
					ops.read = func(r io.Reader, b []byte) (int, error) { n, _ := r.Read(b); return n, fault }
				case "write", "remove", "replacement":
					ops.write = func(f *os.File, b []byte) (int, error) { n, _ := f.Write(b); return n, fault }
				case "sync":
					ops.sync = func(f *os.File) error {
						if e := f.Sync(); e != nil {
							return e
						}
						return fault
					}
				case "close":
					ops.close = func(f *os.File) error {
						if e := f.Close(); e != nil {
							return e
						}
						return fault
					}
				case "dir":
					ops.syncDir = func(path string) error {
						if e := maintenance.SyncPrivateDirectory(path); e != nil {
							return e
						}
						return fault
					}
				}
				if kind == "remove" {
					ops.remove = func(string) error { return fault }
				}
				if kind == "replacement" {
					ops.close = func(f *os.File) error {
						path := f.Name()
						if e := f.Close(); e != nil {
							return e
						}
						if e := os.Rename(path, path+"-old"); e != nil {
							return e
						}
						writePrivate(t, path, rawImage(t, "gif", 10))
						return nil
					}
				}
				_, e := v.installBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
					calls++
					return maintenance.RestoreResult{}, fault
				}, ops)
				return e
			})
			if e == nil || calls != 0 || strings.Contains(e.Error(), "secret") {
				t.Fatal(calls, e)
			}
			_, se := os.Lstat(dest.m.path(a))
			if kind == "remove" || kind == "replacement" {
				if se != nil {
					t.Fatal("uncertain removed", se)
				}
			} else if !errors.Is(se, os.ErrNotExist) {
				t.Fatal("owned leftover", kind, se)
			}
		})
	}
}
func TestBundleInstallRepairFailurePreserves(t *testing.T) {
	source := setup(t)
	a := upload(t, source, "png")
	raw := bundleBytes(t, source)
	for _, kind := range []string{"copy", "retention"} {
		t.Run(kind, func(t *testing.T) {
			dest := setup(t)
			known := upload(t, dest, "png")
			if e := os.Remove(dest.m.path(known)); e != nil {
				t.Fatal(e)
			}
			calls := 0
			e := preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
				ops := defaultBundleInstallIO()
				if kind == "copy" {
					ops.write = func(f *os.File, b []byte) (int, error) {
						n, e := f.Write(b)
						if e != nil {
							return n, e
						}
						return n, errors.New("copy failure")
					}
				} else {
					ops.syncDir = func(path string) error {
						if e := maintenance.SyncPrivateDirectory(path); e != nil {
							return e
						}
						writePrivate(t, filepath.Join(dest.dir, ".library-retained.pending"), []byte("uncertain"))
						return nil
					}
				}
				_, e := v.installBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
					calls++
					return maintenance.RestoreResult{}, errors.New("unexpected")
				}, ops)
				return e
			})
			if e == nil || calls != 0 {
				t.Fatal(calls, e)
			}
			if _, e := os.Stat(dest.m.path(a)); e != nil {
				t.Fatal("referenced repair removed", e)
			}
		})
	}
}
func TestBundleInstallPostCallbackAmbiguity(t *testing.T) {
	source := setup(t)
	a := upload(t, source, "gif")
	raw := bundleBytes(t, source)
	for _, kind := range []string{"error-before-install", "error-after-install", "cancel-after-install"} {
		t.Run(kind, func(t *testing.T) {
			dest := setup(t)
			lock, e := maintenance.Acquire(dest.dir)
			if e != nil {
				t.Fatal(e)
			}
			defer lock.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			var result maintenance.RestoreResult
			e = dest.m.WithMaintenance(ctx, func(ctx context.Context, v *Maintenance) error {
				rows, e := dest.s.LibrarySnapshot(ctx)
				if e != nil {
					return e
				}
				return v.PrepareBundle(bytes.NewReader(raw), func(p *PreparedBundle) error {
					result, e = v.InstallBundle(p, rows, func(c *maintenance.Candidate) (maintenance.RestoreResult, error) {
						calls++
						if kind == "error-before-install" {
							return maintenance.RestoreResult{}, errors.New("private error")
						}
						if e := dest.s.Close(); e != nil {
							return maintenance.RestoreResult{}, e
						}
						if e := os.Rename(filepath.Join(dest.dir, "state.db"), filepath.Join(dest.dir, "miskoai.db")); e != nil {
							return maintenance.RestoreResult{}, e
						}
						r, e := maintenance.InstallRestore(ctx, lock, c)
						if e != nil {
							return r, e
						}
						if kind == "cancel-after-install" {
							cancel()
							return r, nil
						}
						return r, errors.New("private error")
					})
					return e
				})
			})
			if !errors.Is(e, ErrRecovery) || calls != 1 || strings.Contains(e.Error(), "private error") {
				t.Fatal(calls, e)
			}
			if _, se := os.Stat(dest.m.path(a)); se != nil {
				t.Fatal("ambiguous content removed", se)
			}
			if kind != "error-before-install" {
				if !result.PreviousRetained || !result.ChannelPaused {
					t.Fatal(result)
				}
				assertInstalled(t, dest, 1)
			}
		})
	}
}

func TestBundleInstallHeldWorkClose(t *testing.T) {
	for _, kind := range []string{"source", "callback"} {
		t.Run(kind, func(t *testing.T) {
			source := setup(t)
			a := upload(t, source, "png")
			raw := bundleBytes(t, source)
			dest := setup(t)
			lock, e := maintenance.Acquire(dest.dir)
			if e != nil {
				t.Fatal(e)
			}
			defer lock.Close()
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			closed := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(func() { unblock(); <-closed })
			go func() {
				done <- preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
					ops := defaultBundleInstallIO()
					if kind == "source" {
						held := false
						ops.read = func(r io.Reader, b []byte) (int, error) {
							n, e := r.Read(b)
							if !held {
								held = true
								close(entered)
								<-release
							}
							return n, e
						}
					}
					_, e := v.installBundle(p, rows, func(c *maintenance.Candidate) (maintenance.RestoreResult, error) {
						close(entered)
						<-release
						return maintenance.InstallRestore(ctx, lock, c)
					}, ops)
					return e
				})
			}()
			<-entered
			go func() { dest.m.Close(); close(closed) }()
			select {
			case <-closed:
				t.Fatal("Close escaped held actual work")
			case <-time.After(40 * time.Millisecond):
			}
			unblock()
			e = <-done
			<-closed
			if !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
			_, se := os.Stat(dest.m.path(a))
			if kind == "source" {
				if !errors.Is(se, os.ErrNotExist) {
					t.Fatal("precallback original not cleaned", se)
				}
			} else {
				if !errors.Is(e, ErrRecovery) || se != nil {
					t.Fatal("callback ambiguity", e, se)
				}
			}
		})
	}
}
func TestBundleInstallScopesAndPriorDeletion(t *testing.T) {
	source := setup(t)
	a := upload(t, source, "png")
	other := storage.Scope{Account: "second-account", User: "user"}
	braw := rawImage(t, "gif", 11)
	b, _, e := source.m.Upload(context.Background(), source.l, other, bytes.NewReader(braw), int64(len(braw)), storage.LibraryMetadata{})
	if e != nil {
		t.Fatal(e)
	}
	raw := bundleBytes(t, source)
	dest := setup(t)
	prior := upload(t, dest, "gif")
	if prior.ID != a.ID {
		t.Fatal("fixture must reuse ID")
	}
	_, e = installBundleFixture(t, dest, raw)
	if e != nil {
		t.Fatal(e)
	}
	assertInstalled(t, dest, 2)
	s, e := storage.Open(filepath.Join(dest.dir, "miskoai.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	lease := func(context.Context) (*storage.Store, func(), error) { return s, func() {}, nil }
	if _, e := s.LibraryAsset(context.Background(), scope, b.ID); !errors.Is(e, storage.ErrNotFound) {
		t.Fatal("scope leak", e)
	}
	// Prior content is retained independently of metadata and survives manager
	// restart. Importing its metadata again then deleting cannot unlink it.
	if e := dest.m.Close(); e != nil {
		t.Fatal(e)
	}
	m, e := New(dest.dir, dest.p)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if e := m.validate(context.Background(), prior, noContent); e != nil {
		t.Fatal(e)
	}
	restored, _, e := m.Upload(context.Background(), lease, scope, bytes.NewReader(rawImage(t, "gif", 10)), prior.Bytes, storage.LibraryMetadata{})
	if e != nil {
		t.Fatal(e)
	}
	if e := m.Delete(context.Background(), lease, scope, restored.ID); e != nil {
		t.Fatal(e)
	}
	if e := m.validate(context.Background(), prior, noContent); e != nil {
		t.Fatal("protected prior removed", e)
	}
}
func TestBundleInstallRetainedRepair(t *testing.T) {
	source := setup(t)
	a := upload(t, source, "png")
	raw := bundleBytes(t, source)
	dest := setup(t)
	known := upload(t, dest, "png")
	rows, e := dest.s.LibrarySnapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	e = dest.m.WithMaintenance(context.Background(), func(_ context.Context, v *Maintenance) error { return v.RetainCurrent(rows) })
	if e != nil {
		t.Fatal(e)
	}
	if e := dest.s.MarkLibraryDeleting(context.Background(), scope, known.ID); e != nil {
		t.Fatal(e)
	}
	if e := dest.s.FinishLibraryDelete(context.Background(), scope, known.ID); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(dest.m.path(a)); e != nil {
		t.Fatal(e)
	}
	_, e = installBundleFixture(t, dest, raw)
	if e != nil {
		t.Fatal(e)
	}
	assertInstalled(t, dest, 1)
}
func TestBundleInstallTargetAppearance(t *testing.T) {
	source := setup(t)
	a := upload(t, source, "png")
	b := upload(t, source, "gif")
	raw := bundleBytes(t, source)
	dest := setup(t)
	assets := []storage.LibraryAsset{a, b}
	sort.Slice(assets, func(i, j int) bool {
		return contentName(assets[i].Scope, assets[i].SHA256) < contentName(assets[j].Scope, assets[j].SHA256)
	})
	calls := 0
	e := preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
		ops := defaultBundleInstallIO()
		appeared := false
		ops.syncDir = func(path string) error {
			if e := maintenance.SyncPrivateDirectory(path); e != nil {
				return e
			}
			if !appeared {
				appeared = true
				writePrivate(t, dest.m.path(assets[1]), rawImage(t, assets[1].Format, 10))
			}
			return nil
		}
		_, e := v.installBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
			calls++
			return maintenance.RestoreResult{}, errors.New("unexpected")
		}, ops)
		return e
	})
	if e == nil || calls != 0 {
		t.Fatal(calls, e)
	}
	if _, e := os.Stat(dest.m.path(assets[1])); e != nil {
		t.Fatal("appeared file removed", e)
	}
	if _, e := os.Stat(dest.m.path(assets[0])); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("owned first file remains", e)
	}
}

func TestBundleInstallUnionCapacity(t *testing.T) {
	source := setup(t)
	incoming := upload(t, source, "gif")
	bundle := bundleBytes(t, source)
	for _, kind := range []string{"count", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			dest := setup(t)
			raw := rawImage(t, "png", 10)
			count := 128
			if kind == "bytes" {
				count = 16
				payload := make([]byte, (4<<20)-len(raw)-12)
				var chunk bytes.Buffer
				binary.Write(&chunk, binary.BigEndian, uint32(len(payload)))
				chunk.WriteString("tEXt")
				chunk.Write(payload)
				binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(chunk.Bytes()[4:]))
				padded := append([]byte{}, raw[:len(raw)-12]...)
				padded = append(padded, chunk.Bytes()...)
				raw = append(padded, raw[len(raw)-12:]...)
			}
			a, added, e := dest.m.Upload(context.Background(), dest.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
			if e != nil || !added {
				t.Fatal("actual PNG prerequisite", e)
			}
			records := map[string]retainedRecord{}
			for i := 0; i < count; i++ {
				b := a
				if i > 0 {
					b.Scope.User = fmt.Sprint("retained-", i)
					writePrivate(t, dest.m.path(b), raw)
				}
				records[record(b).key()] = record(b)
			}
			c, e := orderedCatalog(records)
			if e != nil {
				t.Fatal(e)
			}
			if e := dest.m.writeRetained(context.Background(), c); e != nil {
				t.Fatal(e)
			}
			calls := 0
			e = preparedInstall(t, dest, bundle, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
				_, e := v.InstallBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
					calls++
					return maintenance.RestoreResult{}, errors.New("unexpected")
				})
				return e
			})
			if !errors.Is(e, ErrCapacity) || calls != 0 {
				t.Fatal("union overflow admitted", calls, e)
			}
			if _, e := os.Stat(dest.m.path(incoming)); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("wrote before capacity refusal", e)
			}
		})
	}
}
func TestBundleInstallDeletingAndEmpty(t *testing.T) {
	for _, kind := range []string{"empty", "deleting-absent", "deleting-present"} {
		t.Run(kind, func(t *testing.T) {
			source := setup(t)
			if kind != "empty" {
				a := upload(t, source, "png")
				if e := source.s.MarkLibraryDeleting(context.Background(), scope, a.ID); e != nil {
					t.Fatal(e)
				}
				if kind == "deleting-absent" {
					if e := os.Remove(source.m.path(a)); e != nil {
						t.Fatal(e)
					}
				}
			}
			raw := bundleBytes(t, source)
			dest := setup(t)
			_, e := installBundleFixture(t, dest, raw)
			if e != nil {
				t.Fatal(e)
			}
			s, e := storage.Open(filepath.Join(dest.dir, "miskoai.db"))
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			rows, e := s.LibrarySnapshot(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			if kind == "empty" {
				if len(rows) != 0 {
					t.Fatal(rows)
				}
			} else {
				if len(rows) != 1 || rows[0].State != "deleting" {
					t.Fatal(rows)
				}
				_, e := os.Stat(dest.m.path(rows[0]))
				if kind == "deleting-present" && e != nil || kind == "deleting-absent" && !errors.Is(e, os.ErrNotExist) {
					t.Fatal(e)
				}
			}
		})
	}
}

func TestBundleInstallLateOperationFailures(t *testing.T) {
	source := setup(t)
	a := upload(t, source, "png")
	raw := bundleBytes(t, source)
	for _, kind := range []string{"digest", "extra", "final-dir", "cleanup-dir", "stage-changed"} {
		t.Run(kind, func(t *testing.T) {
			dest := setup(t)
			calls := 0
			e := preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
				ops := defaultBundleInstallIO()
				reads := 0
				syncs := 0
				if kind == "digest" || kind == "extra" || kind == "stage-changed" {
					ops.read = func(r io.Reader, b []byte) (int, error) {
						n, e := r.Read(b)
						reads++
						if kind == "digest" && reads == 1 && n > 0 {
							b[0] ^= 255
						}
						if kind == "extra" && e == io.EOF && n == 0 {
							b[0] = 1
							return 1, nil
						}
						if kind == "stage-changed" && reads == 1 {
							k := strings.TrimPrefix(contentName(a.Scope, a.SHA256), "asset-")
							path := p.stage.path(k)
							info, e := os.Stat(path)
							if e != nil {
								t.Fatal(e)
							}
							if e := os.Chtimes(path, info.ModTime().Add(time.Hour), info.ModTime().Add(time.Hour)); e != nil {
								t.Fatal(e)
							}
						}
						return n, e
					}
				}
				if kind == "cleanup-dir" {
					ops.write = func(f *os.File, b []byte) (int, error) { n, _ := f.Write(b); return n, errors.New("failure") }
				}
				if kind == "final-dir" || kind == "cleanup-dir" {
					ops.syncDir = func(path string) error {
						syncs++
						if e := maintenance.SyncPrivateDirectory(path); e != nil {
							return e
						}
						if kind == "cleanup-dir" || syncs == 2 {
							return errors.New("failure")
						}
						return nil
					}
				}
				_, e := v.installBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
					calls++
					return maintenance.RestoreResult{}, errors.New("unexpected")
				}, ops)
				return e
			})
			if e == nil || calls != 0 {
				t.Fatal(calls, e)
			}
			if _, e := os.Stat(dest.m.path(a)); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("owned content not cleaned", e)
			}
		})
	}
}
func TestBundleInstallLinkedObjects(t *testing.T) {
	source := setup(t)
	a := upload(t, source, "png")
	raw := bundleBytes(t, source)
	for _, kind := range []string{"destination", "stage"} {
		t.Run(kind, func(t *testing.T) {
			dest := setup(t)
			if kind == "destination" {
				a = upload(t, dest, "png")
				if e := os.Link(dest.m.path(a), filepath.Join(dest.dir, "link-proof")); e != nil {
					t.Fatal("actual native Link setup", e)
				}
			}
			calls := 0
			e := preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
				if kind == "stage" {
					k := strings.TrimPrefix(contentName(a.Scope, a.SHA256), "asset-")
					if e := os.Link(p.stage.path(k), filepath.Join(dest.dir, "link-proof")); e != nil {
						t.Fatal("actual native Link setup", e)
					}
				}
				_, e := v.InstallBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
					calls++
					return maintenance.RestoreResult{}, errors.New("unexpected")
				})
				return e
			})
			if e == nil || calls != 0 {
				t.Fatal(calls, e)
			}
		})
	}
}
func TestBundleInstallLegacy(t *testing.T) {
	source := setup(t)
	h, payloads := unpackBundle(t, bundleBytes(t, source))
	path := filepath.Join(source.dir, "legacy-source.db")
	writePrivate(t, path, payloads[""])
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec("DROP TABLE image_replies; DROP TABLE library_assets; PRAGMA user_version=4"); e != nil {
		t.Fatal(e)
	}
	if e := db.Close(); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	payloads[""] = raw
	h.Database = librarybundle.File{Bytes: int64(len(raw)), SHA256: bundleSum(raw)}
	dest := setup(t)
	_, e = installBundleFixture(t, dest, packBundle(t, h, payloads))
	if e != nil {
		t.Fatal(e)
	}
	assertInstalled(t, dest, 0)
}

func TestBundleInstallRepairCacheBound(t *testing.T) {
	source := setup(t)
	a := upload(t, source, "png")
	raw := bundleBytes(t, source)
	dest := setup(t)
	// Simulate bounded stale identities from former originals that disappeared.
	// Each remembered identity belongs to an actual newly created private file.
	for i := 0; i < 128; i++ {
		old := a
		old.Scope.User = fmt.Sprint("former-", i)
		path := dest.m.path(old)
		writePrivate(t, path, rawImage(t, "png", 10))
		info, e := os.Stat(path)
		if e != nil {
			t.Fatal(e)
		}
		if e := dest.m.remember(path, info); e != nil {
			t.Fatal(e)
		}
		if e := os.Remove(path); e != nil {
			t.Fatal(e)
		}
	}
	c, e := orderedCatalog(map[string]retainedRecord{record(a).key(): record(a)})
	if e != nil {
		t.Fatal(e)
	}
	if e := dest.m.writeRetained(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	calls := 0
	e = preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
		_, e := v.InstallBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
			calls++
			return maintenance.RestoreResult{}, errors.New("unexpected")
		})
		return e
	})
	if !errors.Is(e, ErrCapacity) || calls != 0 || len(dest.m.identities) > 128 {
		t.Fatal("repair escaped bounded cache", calls, len(dest.m.identities), e)
	}
	if _, e := os.Stat(dest.m.path(a)); e != nil {
		t.Fatal("referenced repair removed on cache failure", e)
	}
}

func TestBundleInstallForeignView(t *testing.T) {
	source := setup(t)
	raw := bundleBytes(t, source)
	dest := setup(t)
	e := preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
		return source.m.WithMaintenance(ctx, func(_ context.Context, foreign *Maintenance) error {
			_, e := foreign.InstallBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
				t.Fatal("foreign prepared callback")
				return maintenance.RestoreResult{}, nil
			})
			if !errors.Is(e, ErrInvalid) {
				t.Fatal(e)
			}
			if p.installStarted {
				t.Fatal("foreign consumed original")
			}
			return nil
		})
	})
	if e != nil {
		t.Fatal(e)
	}
}
func TestBundleInstallAmbiguityBlocksMutation(t *testing.T) {
	source := setup(t)
	upload(t, source, "png")
	raw := bundleBytes(t, source)
	dest := setup(t)
	e := preparedInstall(t, dest, raw, func(ctx context.Context, v *Maintenance, p *PreparedBundle, rows []storage.LibraryAsset) error {
		_, e := v.InstallBundle(p, rows, func(*maintenance.Candidate) (maintenance.RestoreResult, error) {
			return maintenance.RestoreResult{}, errors.New("ambiguous failure")
		})
		return e
	})
	if !errors.Is(e, ErrRecovery) {
		t.Fatal(e)
	}
	gif := rawImage(t, "gif", 10)
	_, _, e = dest.m.Upload(context.Background(), dest.l, scope, bytes.NewReader(gif), int64(len(gif)), storage.LibraryMetadata{})
	if !errors.Is(e, ErrRecovery) {
		t.Fatal("ambiguous leftover silently adopted", e)
	}
}
