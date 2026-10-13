package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
)

func writePrivate(t *testing.T, path string, raw []byte) {
	t.Helper()
	f, e := privatefs.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write(raw); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
func consumeNothing(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
	return nil
}

type processorFunc func(context.Context, io.Reader, int64, func(attachment.LocalImage, attachment.Artifact) error) error

func (p processorFunc) WithLocalImage(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
	return p(c, r, n, fn)
}

func TestContentScopedLookupBeforeProcessor(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	calls := 0
	f.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
		calls++
		return f.p.WithLocalImage(c, r, n, fn)
	})
	if e := os.Remove(f.m.path(a)); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		sc storage.Scope
		id int64
	}{{storage.Scope{Account: "foreign", User: "user"}, a.ID}, {scope, math.MaxInt64}} {
		for _, e := range []error{f.m.WithAsset(context.Background(), f.l, tc.sc, tc.id, false, consumeNothing), f.m.Update(context.Background(), f.l, tc.sc, tc.id, storage.LibraryMetadata{Enabled: true}), f.m.Delete(context.Background(), f.l, tc.sc, tc.id), f.m.RecordUse(context.Background(), f.l, tc.sc, tc.id, a.SHA256, 1)} {
			if !errors.Is(e, storage.ErrNotFound) {
				t.Fatal("scope first", e)
			}
		}
	}
	if calls != 0 {
		t.Fatal("processor called before scoped refusal")
	}
}
func TestContentScopedPayloadHasDistinctTargets(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	raw := rawImage(t, "png", 10)
	other := storage.Scope{Account: "account", User: "other"}
	b, added, e := f.m.Upload(context.Background(), f.l, other, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
	if e != nil || !added || a.ID == b.ID || a.SHA256 != b.SHA256 || f.m.path(a) == f.m.path(b) {
		t.Fatal("scoped identity", e)
	}
	if e = f.m.Delete(context.Background(), f.l, scope, a.ID); e != nil {
		t.Fatal(e)
	}
	if e = f.m.WithAsset(context.Background(), f.l, other, b.ID, false, consumeNothing); e != nil {
		t.Fatal("other deleted", e)
	}
}
func TestContentReopenEnableUseDisableDelete(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "gif")
	if e := f.m.Close(); e != nil {
		t.Fatal(e)
	}
	m, e := New(f.dir, f.p)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if e = m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{Enabled: true, Description: "edited", Tags: []string{"ok"}}); e != nil {
		t.Fatal(e)
	}
	if e = m.RecordUse(context.Background(), f.l, scope, a.ID, a.SHA256, 20); e != nil {
		t.Fatal(e)
	}
	row, e := f.s.LibraryAsset(context.Background(), scope, a.ID)
	if e != nil || row.Uses != 1 || row.LastUsedMillis != 20 || row.Description != "edited" {
		t.Fatal("use/edit", row, e)
	}
	if e = m.RecordUse(context.Background(), f.l, scope, a.ID, strings.Repeat("0", 64), 30); !errors.Is(e, ErrRecovery) {
		t.Fatal("digest mismatch", e)
	}
	if e = m.WithAsset(context.Background(), f.l, scope, a.ID, true, consumeNothing); e != nil {
		t.Fatal(e)
	}
	if e = m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{}); e != nil {
		t.Fatal(e)
	}
	if e = m.RecordUse(context.Background(), f.l, scope, a.ID, a.SHA256, 30); !errors.Is(e, storage.ErrInvalid) {
		t.Fatal("disabled use", e)
	}
	if e = m.Delete(context.Background(), f.l, scope, a.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(m.path(a)); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("file retained", e)
	}
	if _, e = f.s.LibraryAsset(context.Background(), scope, a.ID); !errors.Is(e, storage.ErrNotFound) {
		t.Fatal("row retained", e)
	}
}
func TestContentMissingReadyAndMissingTombstone(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	if e := os.Remove(f.m.path(a)); e != nil {
		t.Fatal(e)
	}
	if e := f.m.Delete(context.Background(), f.l, scope, a.ID); !errors.Is(e, ErrRecovery) {
		t.Fatal("missing ready", e)
	}
	row, e := f.s.LibraryAsset(context.Background(), scope, a.ID)
	if e != nil || row.State != "ready" {
		t.Fatal("ready changed", e)
	}
	if e = f.m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{Description: "safe disabled edit"}); e != nil {
		t.Fatal(e)
	}
	if e = f.m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{Enabled: true}); !errors.Is(e, ErrRecovery) {
		t.Fatal("missing enabled", e)
	}
	if e = f.s.MarkLibraryDeleting(context.Background(), scope, a.ID); e != nil {
		t.Fatal(e)
	}
	if e = f.m.WithAsset(context.Background(), f.l, scope, a.ID, false, consumeNothing); !errors.Is(e, ErrContent) {
		t.Fatal("tombstone lent", e)
	}
	if e = f.m.Delete(context.Background(), f.l, scope, a.ID); e != nil {
		t.Fatal("finalize tombstone", e)
	}
}
func TestContentReplacementPreserved(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	path := f.m.path(a)
	saved := path + "-original"
	if e := os.Rename(path, saved); e != nil {
		t.Fatal(e)
	}
	raw := rawImage(t, "png", 10)
	writePrivate(t, path, raw)
	for _, e := range []error{f.m.WithAsset(context.Background(), f.l, scope, a.ID, false, consumeNothing), f.m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{Enabled: true}), f.m.Delete(context.Background(), f.l, scope, a.ID)} {
		if !errors.Is(e, ErrRecovery) {
			t.Fatal("replacement accepted", e)
		}
	}
	got, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(got, raw) {
		t.Fatal("replacement altered", e)
	}
}
func TestContentCorruptReopenPreserved(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	path := f.m.path(a)
	if e := f.m.Close(); e != nil {
		t.Fatal(e)
	}
	raw := rawImage(t, "png", 10)
	raw[len(raw)/2] ^= 1
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	m, e := New(f.dir, f.p)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	for _, e := range []error{m.WithAsset(context.Background(), f.l, scope, a.ID, false, consumeNothing), m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{Enabled: true}), m.Delete(context.Background(), f.l, scope, a.ID)} {
		if !errors.Is(e, ErrContent) {
			t.Fatal("corrupt accepted", e)
		}
	}
	if _, e = os.Stat(path); e != nil {
		t.Fatal("corrupt removed", e)
	}
}
func TestContentRowFullImmutableValidation(t *testing.T) {
	for _, field := range []string{"digest", "format", "bytes", "width", "height", "frames"} {
		t.Run(field, func(t *testing.T) {
			f := setup(t)
			raw := rawImage(t, "gif", 10)
			h := sha256.Sum256(raw)
			a := storage.LibraryAsset{Scope: scope, SHA256: hex.EncodeToString(h[:]), Format: "gif", Bytes: int64(len(raw)), Width: 2, Height: 3, Frames: 1, State: "ready"}
			switch field {
			case "digest":
				a.SHA256 = strings.Repeat("0", 64)
			case "format":
				a.Format = "png"
			case "bytes":
				a.Bytes++
			case "width":
				a.Width++
			case "height":
				a.Height++
			case "frames":
				a.Frames++
			}
			a, _, e := f.s.AddLibraryAsset(context.Background(), scope, a)
			if e != nil {
				t.Fatal(e)
			}
			writePrivate(t, f.m.path(a), raw)
			called := false
			e = f.m.WithAsset(context.Background(), f.l, scope, a.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
				called = true
				return nil
			})
			if e == nil || called {
				t.Fatal("immutable metadata accepted", e)
			}
		})
	}
}
func TestContentUploadValidationAndCanaries(t *testing.T) {
	for _, kind := range []string{"invalid", "trailing", "short", "reader", "consumer", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			raw := rawImage(t, "png", 10)
			size := int64(len(raw))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var r io.Reader = bytes.NewReader(raw)
			switch kind {
			case "invalid":
				raw[0] = 0
				r = bytes.NewReader(raw)
			case "trailing":
				r = bytes.NewReader(append(raw, 1))
			case "short":
				size++
			case "reader":
				r = readerFunc(func([]byte) (int, error) { return 0, fmt.Errorf("SYNTHETIC_RAW_CANARY") })
			case "cancel":
				cancel()
			}
			if kind == "consumer" {
				a := upload(t, f, "png")
				e := f.m.WithAsset(ctx, f.l, scope, a.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
					return fmt.Errorf("SYNTHETIC_RAW_CANARY: %w", storage.ErrCapacity)
				})
				if !errors.Is(e, storage.ErrCapacity) || strings.Contains(e.Error(), "CANARY") {
					t.Fatal("consumer class", e)
				}
				return
			}
			_, _, e := f.m.Upload(ctx, f.l, scope, r, size, storage.LibraryMetadata{})
			if e == nil || strings.Contains(e.Error(), "CANARY") {
				t.Fatal("bad upload", e)
			}
			if kind == "cancel" && !errors.Is(e, context.Canceled) {
				t.Fatal("cancel", e)
			}
			rows, e := f.s.LibrarySnapshot(context.Background())
			if e != nil || len(rows) != 0 {
				t.Fatal("bad input committed", e)
			}
		})
	}
}
func TestContentOrphanBlocksNewAndDuplicateUpload(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	orphan := filepath.Join(f.m.dir, "asset-"+strings.Repeat("0", 64))
	writePrivate(t, orphan, []byte("orphan"))
	for _, seed := range []uint8{10, 11} {
		raw := rawImage(t, "png", seed)
		_, _, e := f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
		if !errors.Is(e, ErrRecovery) {
			t.Fatal("orphan mutation admitted", seed, e)
		}
	}
	if _, e := os.Stat(orphan); e != nil {
		t.Fatal("orphan swept", e)
	}
	if _, e := f.s.LibraryAsset(context.Background(), scope, a.ID); e != nil {
		t.Fatal("row changed", e)
	}
}
func TestContentPhysicalBounds(t *testing.T) {
	for _, kind := range []string{"count128", "count129", "bytes64", "bytes65", "unknownname", "oversize", "directory", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			var rows []storage.LibraryAsset
			count := 1
			size := int64(1)
			switch kind {
			case "count128":
				count = 128
			case "count129":
				count = 129
			case "bytes64":
				count = 16
				size = 4 << 20
			case "bytes65":
				count = 17
				size = 4 << 20
			case "oversize":
				size = 4<<20 + 1
			}
			for i := 0; i < count; i++ {
				a := storage.LibraryAsset{Scope: scope, SHA256: fmt.Sprintf("%064x", i), Bytes: size, State: "ready"}
				path := f.m.path(a)
				h, e := privatefs.Create(path)
				if e != nil {
					t.Fatal(e)
				}
				if e = h.Truncate(size); e != nil {
					t.Fatal(e)
				}
				if e = h.Close(); e != nil {
					t.Fatal(e)
				}
				rows = append(rows, a)
			}
			switch kind {
			case "unknownname":
				writePrivate(t, filepath.Join(f.m.dir, "untrusted-name"), []byte("x"))
			case "directory":
				if e := privatefs.EnsureDir(filepath.Join(f.m.dir, "child")); e != nil {
					t.Fatal(e)
				}
			case "hardlink":
				if e := os.Link(f.m.path(rows[0]), filepath.Join(f.m.dir, "asset-"+strings.Repeat("f", 64))); e != nil {
					t.Fatal("authentic hardlink setup", e)
				}
			}
			e := f.m.physical(rows, 0)
			if kind == "count128" || kind == "bytes64" {
				if e != nil {
					t.Fatal("exact capacity", e)
				}
				if e = f.m.physical(rows, 1); !errors.Is(e, ErrCapacity) {
					t.Fatal("capacity+1", e)
				}
			} else if kind == "count129" || kind == "bytes65" {
				if !errors.Is(e, ErrCapacity) {
					t.Fatal("physical quota", e)
				}
			} else if !errors.Is(e, ErrRecovery) {
				t.Fatal("unsafe physical", e)
			}
		})
	}
}
func TestContentFreshGenerationAndUsageDigest(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	g := setup(t)
	b := upload(t, g, "jpeg")
	if a.ID != b.ID {
		t.Fatal("generation fixture ID")
	}
	if e := g.s.UpdateLibraryAsset(context.Background(), scope, b.ID, storage.LibraryMetadata{Enabled: true}); e != nil {
		t.Fatal(e)
	}
	calls, releases, held := 0, 0, false
	l := func(context.Context) (*storage.Store, func(), error) {
		if held {
			t.Fatal("lease nested across media")
		}
		held = true
		calls++
		s := f.s
		if calls > 1 {
			s = g.s
		}
		return s, func() {
			if !held {
				t.Fatal("double release")
			}
			held = false
			releases++
		}, nil
	}
	e := f.m.WithAsset(context.Background(), l, scope, a.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		t.Fatal("stale object lent")
		return nil
	})
	if !errors.Is(e, ErrRecovery) || calls != 2 || releases != 2 || held {
		t.Fatal("fresh lease", e, calls, releases)
	}
	if e = f.m.RecordUse(context.Background(), g.l, scope, b.ID, a.SHA256, 1); !errors.Is(e, ErrRecovery) {
		t.Fatal("old digest usage", e)
	}
	got, e := g.s.LibraryAsset(context.Background(), scope, b.ID)
	if e != nil || got.Uses != 0 {
		t.Fatal("different generation updated", e)
	}
}
func TestContentLeaseReleaseAndRedaction(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	for _, kind := range []string{"error", "nilstore", "nilrelease", "success"} {
		t.Run(kind, func(t *testing.T) {
			releases := 0
			l := func(context.Context) (*storage.Store, func(), error) {
				release := func() { releases++ }
				switch kind {
				case "error":
					return f.s, release, fmt.Errorf("SYNTHETIC_RAW_CANARY: %w", storage.ErrStorage)
				case "nilstore":
					return nil, release, nil
				case "nilrelease":
					return f.s, nil, nil
				}
				return f.s, release, nil
			}
			e := f.m.WithAsset(context.Background(), l, scope, a.ID, false, consumeNothing)
			want := 1
			if kind == "success" {
				want = 2
				if e != nil {
					t.Fatal(e)
				}
			} else if kind == "nilrelease" {
				want = 0
			}
			if releases != want {
				t.Fatal("release count", releases, want)
			}
			if kind != "success" && (e == nil || strings.Contains(e.Error(), "CANARY")) {
				t.Fatal("lease error unsafe", e)
			}
		})
	}
}

type readArtifact struct {
	attachment.Artifact
	read func([]byte, int64) (int, error)
}

func (a readArtifact) ReadAt(b []byte, off int64) (int, error) { return a.read(b, off) }
func TestContentPublicationRollbackAndIdentity(t *testing.T) {
	for _, kind := range []string{"cancel", "readerror", "replacement", "sqlcapacity"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			raw := rawImage(t, "png", 11)
			sum := sha256.Sum256(raw)
			candidate := storage.LibraryAsset{Scope: scope, SHA256: hex.EncodeToString(sum[:])}
			path := f.m.path(candidate)
			if kind == "sqlcapacity" {
				a := upload(t, f, "jpeg")
				db, e := sql.Open("sqlite", filepath.Join(f.dir, "state.db"))
				if e != nil {
					t.Fatal(e)
				}
				if _, e = db.Exec("UPDATE sqlite_sequence SET seq=? WHERE name='library_assets'", int64(math.MaxInt64)); e != nil {
					t.Fatal(e)
				}
				if e = db.Close(); e != nil {
					t.Fatal(e)
				}
				if _, e = os.Stat(f.m.path(a)); e != nil {
					t.Fatal(e)
				}
			} else {
				f.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
					return f.p.WithLocalImage(c, r, n, func(im attachment.LocalImage, art attachment.Artifact) error {
						once := false
						wrapped := readArtifact{Artifact: art, read: func(b []byte, off int64) (int, error) {
							if !once {
								once = true
								switch kind {
								case "cancel":
									cancel()
								case "readerror":
									return 0, fmt.Errorf("SYNTHETIC_RAW_CANARY")
								case "replacement":
									if e := os.Rename(path, path+"-captured"); e != nil {
										t.Fatal(e)
									}
									writePrivate(t, path, []byte("replacement"))
								}
							}
							return art.ReadAt(b, off)
						}}
						return fn(im, wrapped)
					})
				})
			}
			_, added, e := f.m.Upload(ctx, f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
			if e == nil || added || strings.Contains(e.Error(), "CANARY") {
				t.Fatal("failed publication", e, added)
			}
			switch kind {
			case "cancel":
				if !errors.Is(e, context.Canceled) {
					t.Fatal(e)
				}
			case "sqlcapacity":
				if !errors.Is(e, storage.ErrCapacity) {
					t.Fatal(e)
				}
			case "replacement":
				if !errors.Is(e, ErrRecovery) {
					t.Fatal(e)
				}
			}
			if kind == "replacement" {
				got, e := os.ReadFile(path)
				if e != nil || string(got) != "replacement" {
					t.Fatal("replacement removed", e)
				}
				raw2 := rawImage(t, "gif", 11)
				_, _, e = f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw2), int64(len(raw2)), storage.LibraryMetadata{})
				if !errors.Is(e, ErrRecovery) {
					t.Fatal("uncertain orphan not retained", e)
				}
			} else if _, e = os.Stat(path); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("known failed original remains", e)
			}
			rows, e := f.s.LibrarySnapshot(context.Background())
			want := 0
			if kind == "sqlcapacity" {
				want = 1
			}
			if e != nil || len(rows) != want {
				t.Fatal("rollback rows", len(rows), e)
			}
		})
	}
}
func TestContentGlobalRowQuota(t *testing.T) {
	f := setup(t)
	for i := 0; i < 128; i++ {
		a := storage.LibraryAsset{Scope: scope, SHA256: fmt.Sprintf("%064x", i), Format: "png", Bytes: 1, Width: 1, Height: 1, Frames: 1, State: "ready"}
		a, added, e := f.s.AddLibraryAsset(context.Background(), scope, a)
		if e != nil || !added {
			t.Fatal("fill", i, e)
		}
		writePrivate(t, f.m.path(a), []byte("x"))
	}
	raw := rawImage(t, "png", 10)
	_, added, e := f.m.Upload(context.Background(), f.l, storage.Scope{Account: "another", User: "user"}, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
	if !errors.Is(e, ErrCapacity) || added {
		t.Fatal("global quota", e)
	}
	entries, e := os.ReadDir(f.m.dir)
	if e != nil || len(entries) != 128 {
		t.Fatal("quota published", len(entries), e)
	}
}
func TestContentAnimatedGIFAndPixelFrameLimits(t *testing.T) {
	f := setup(t)
	im := image.NewPaletted(image.Rect(0, 0, 2, 3), color.Palette{color.Black, color.White})
	im.SetColorIndex(0, 0, 1)
	var b bytes.Buffer
	if e := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{im, im}, Delay: []int{1, 1}}); e != nil {
		t.Fatal(e)
	}
	a, added, e := f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(b.Bytes()), int64(b.Len()), storage.LibraryMetadata{})
	if e != nil || !added || a.Frames != 2 {
		t.Fatal("animated", a, e)
	}
	if e = f.m.WithAsset(context.Background(), f.l, scope, a.ID, false, consumeNothing); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"pixels", "frames"} {
		t.Run(kind, func(t *testing.T) {
			var raw bytes.Buffer
			g := &gif.GIF{}
			if kind == "frames" {
				for i := 0; i < 33; i++ {
					g.Image = append(g.Image, im)
					g.Delay = append(g.Delay, 1)
				}
			} else {
				g.Image = []*image.Paletted{im}
				g.Delay = []int{1}
				g.Config = image.Config{ColorModel: im.Palette, Width: 8193, Height: 3}
			}
			if e := gif.EncodeAll(&raw, g); e != nil {
				t.Fatal(e)
			}
			_, _, e := f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw.Bytes()), int64(raw.Len()), storage.LibraryMetadata{})
			if !errors.Is(e, ErrContent) {
				t.Fatal("validator cap bypass", e)
			}
		})
	}
}
func TestContentRecoveryAllowsScopedDisableAndDelete(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	if e := f.m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{Enabled: true}); e != nil {
		t.Fatal(e)
	}
	orphan := filepath.Join(f.m.dir, "asset-"+strings.Repeat("0", 64))
	writePrivate(t, orphan, []byte("unknown"))
	if e := f.m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{}); e != nil {
		t.Fatal("safe disable blocked", e)
	}
	if e := f.m.Delete(context.Background(), f.l, scope, a.ID); e != nil {
		t.Fatal("verified deletion blocked", e)
	}
	got, e := os.ReadFile(orphan)
	if e != nil || string(got) != "unknown" {
		t.Fatal("orphan modified", e)
	}
}

func TestContentCancellationAndActualMediaCleanupRefusal(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var replacement string
	var blocker *os.File
	var borrowed attachment.Artifact
	e := f.m.WithAsset(ctx, f.l, scope, a.ID, false, func(_ storage.LibraryAsset, _ attachment.LocalImage, art attachment.Artifact) error {
		borrowed = art
		entries, e := os.ReadDir(f.dir)
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "media-") {
				replacement = filepath.Join(f.dir, entry.Name())
				if runtime.GOOS == "windows" {
					blocker, e = os.Open(replacement)
					if e != nil {
						t.Fatal(e)
					}
				} else {
					if e = os.Rename(replacement, replacement+"-captured"); e != nil {
						t.Fatal(e)
					}
					writePrivate(t, replacement, []byte("replacement"))
				}
				break
			}
		}
		if replacement == "" {
			t.Fatal("actual private stage missing")
		}
		cancel()
		return context.Canceled
	})
	if !errors.Is(e, context.Canceled) || !errors.Is(e, ErrCleanup) {
		t.Fatal("cancel/cleanup lost", e)
	}
	if blocker != nil {
		if e = blocker.Close(); e != nil {
			t.Fatal(e)
		}
	}
	got, e := os.ReadFile(replacement)
	if e != nil || runtime.GOOS != "windows" && string(got) != "replacement" || runtime.GOOS == "windows" && !bytes.Equal(got, rawImage(t, "png", 10)) {
		t.Fatal("cleanup removed replacement", e)
	}
	if _, e = borrowed.ReadAt(make([]byte, 1), 0); e == nil {
		t.Fatal("refused cleanup still open")
	}
	if _, e = os.Stat(f.m.path(a)); e != nil {
		t.Fatal("persistent removed", e)
	}
}
func TestContentLeaseHeldOnlyInsideSharedMediaConsumer(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	held, mediaHeld := false, false
	releases := 0
	f.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
		if held {
			t.Fatal("lookup lease held entering media")
		}
		mediaHeld = true
		defer func() { mediaHeld = false }()
		return f.p.WithLocalImage(c, r, n, fn)
	})
	l := func(context.Context) (*storage.Store, func(), error) {
		if held {
			t.Fatal("nested lease")
		}
		held = true
		return f.s, func() {
			if !held {
				t.Fatal("release twice")
			}
			held = false
			releases++
		}, nil
	}
	if e := f.m.WithAsset(context.Background(), l, scope, a.ID, false, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error {
		if !held || !mediaHeld {
			t.Fatal("consumer lifetime missing")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if held || mediaHeld || releases != 2 {
		t.Fatal("joined lease", releases)
	}
}

func TestContentFailedDeletionPersistsDisabledIntent(t *testing.T) {
	for _, kind := range []string{"corrupt", "replacement"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			a := upload(t, f, "png")
			if e := f.m.Update(context.Background(), f.l, scope, a.ID, storage.LibraryMetadata{Enabled: true}); e != nil {
				t.Fatal(e)
			}
			path := f.m.path(a)
			raw := rawImage(t, "png", 10)
			if kind == "corrupt" {
				raw[len(raw)/2] ^= 1
				if e := os.WriteFile(path, raw, 0600); e != nil {
					t.Fatal(e)
				}
			} else {
				if e := os.Rename(path, path+"-captured"); e != nil {
					t.Fatal(e)
				}
				writePrivate(t, path, raw)
			}
			before, e := os.Stat(path)
			if e != nil {
				t.Fatal(e)
			}
			if !os.SameFile(before, before) {
				t.Fatal("identity capture")
			}
			held := false
			l := func(context.Context) (*storage.Store, func(), error) {
				if held {
					t.Fatal("nested lease")
				}
				held = true
				return f.s, func() { held = false }, nil
			}
			f.m.processor = processorFunc(func(c context.Context, r io.Reader, n int64, fn func(attachment.LocalImage, attachment.Artifact) error) error {
				if held {
					t.Fatal("delete Store lease entering media")
				}
				return f.p.WithLocalImage(c, r, n, fn)
			})
			if e = f.m.Delete(context.Background(), l, scope, a.ID); !errors.Is(e, ErrRecovery) {
				t.Fatal("uncertain deletion", e)
			}
			row, e := f.s.LibraryAsset(context.Background(), scope, a.ID)
			if e != nil || row.State != "deleting" || row.Enabled {
				t.Fatal("intent not durable/disabled", row, e)
			}
			after, e := os.Stat(path)
			if e != nil || !os.SameFile(before, after) {
				t.Fatal("uncertain original changed", e)
			}
			got, e := os.ReadFile(path)
			if e != nil || !bytes.Equal(got, raw) {
				t.Fatal("uncertain bytes changed", e)
			}
		})
	}
}
func TestContentKnownMissingTombstoneReleasesIdentity(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	path := f.m.path(a)
	if e := f.s.MarkLibraryDeleting(context.Background(), scope, a.ID); e != nil {
		t.Fatal(e)
	}
	if e := os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e := f.m.Delete(context.Background(), f.l, scope, a.ID); e != nil {
		t.Fatal(e)
	}
	if len(f.m.identities) != 0 {
		t.Fatal("finalized missing row retains bounded identity")
	}
	upload(t, f, "jpeg")
}

func TestContentExactUploadSizeAndUntrackedKey(t *testing.T) {
	f := setup(t)
	raw := rawImage(t, "png", 10)
	chunk := make([]byte, (4<<20)-len(raw))
	binary.BigEndian.PutUint32(chunk[:4], uint32(len(chunk)-12))
	copy(chunk[4:8], "tEXt")
	copy(chunk[8:], "synthetic\x00")
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	raw = append(append(append([]byte{}, raw[:len(raw)-12]...), chunk...), raw[len(raw)-12:]...)
	a, added, e := f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{})
	if e != nil || !added || a.Bytes != 4<<20 {
		t.Fatal("exact4MiB", e, a.Bytes)
	}
	if e = f.m.WithAsset(context.Background(), f.l, scope, a.ID, false, consumeNothing); e != nil {
		t.Fatal("exact size reopen", e)
	}
	raw2 := rawImage(t, "jpeg", 11)
	sum := sha256.Sum256(raw2)
	candidate := storage.LibraryAsset{Scope: scope, SHA256: hex.EncodeToString(sum[:])}
	path := f.m.path(candidate)
	writePrivate(t, path, raw2)
	_, added, e = f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw2), int64(len(raw2)), storage.LibraryMetadata{})
	if !errors.Is(e, ErrRecovery) || added {
		t.Fatal("untracked same key adopted", e)
	}
	got, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(got, raw2) {
		t.Fatal("untracked key overwritten", e)
	}
}
