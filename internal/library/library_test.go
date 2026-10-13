package library

import (
	"bytes"
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var scope = storage.Scope{Account: "account", User: "user"}

type fixture struct {
	m   *Manager
	s   *storage.Store
	p   *attachment.ImageProcessor
	dir string
	l   Lease
}

func setup(t *testing.T) fixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	if e := privatefs.EnsureDir(dir); e != nil {
		t.Fatal(e)
	}
	s, e := storage.Open(filepath.Join(dir, "state.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	cdn, e := media.NewCDN()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { cdn.Close() })
	p, e := attachment.NewImageProcessor(dir, cdn)
	if e != nil {
		t.Fatal(e)
	}
	m, e := New(dir, p)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { m.Close() })
	return fixture{m, s, p, dir, func(context.Context) (*storage.Store, func(), error) { return s, func() {}, nil }}
}
func rawImage(t *testing.T, format string, seed uint8) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 2, 3))
	im.Set(0, 0, color.RGBA{seed, 20, 30, 255})
	var b bytes.Buffer
	var e error
	switch format {
	case "png":
		e = png.Encode(&b, im)
	case "jpeg":
		e = jpeg.Encode(&b, im, nil)
	case "gif":
		e = gif.Encode(&b, im, nil)
	}
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func upload(t *testing.T, f fixture, format string) storage.LibraryAsset {
	t.Helper()
	raw := rawImage(t, format, 10)
	a, added, e := f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{Description: "original", Tags: []string{"tag"}})
	if e != nil || !added {
		t.Fatal("upload", e, added)
	}
	return a
}
func TestUploadFormatsDuplicateAndBorrow(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			f := setup(t)
			a := upload(t, f, format)
			raw := rawImage(t, format, 10)
			got, added, e := f.m.Upload(context.Background(), f.l, scope, bytes.NewReader(raw), int64(len(raw)), storage.LibraryMetadata{Description: "changed"})
			if e != nil || added || got.ID != a.ID || got.Description != "original" {
				t.Fatal("duplicate", e, got, added)
			}
			var borrowed attachment.Artifact
			e = f.m.WithAsset(context.Background(), f.l, scope, a.ID, false, func(row storage.LibraryAsset, im attachment.LocalImage, art attachment.Artifact) error {
				borrowed = art
				if im.Info.Format != format || row.ID != a.ID {
					t.Fatal("metadata")
				}
				b := make([]byte, len(raw))
				n, e := art.ReadAt(b, 0)
				if n != len(raw) || e != nil || !bytes.Equal(b, raw) {
					t.Fatal("borrowed bytes", e)
				}
				return nil
			})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = borrowed.ReadAt(make([]byte, 1), 0); e == nil {
				t.Fatal("temporary still readable")
			}
			if e = f.m.WithAsset(context.Background(), f.l, scope, a.ID, true, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error { return nil }); !errors.Is(e, ErrContent) {
				t.Fatal("disabled", e)
			}
		})
	}
}
func TestInvalidUploadBeforeRead(t *testing.T) {
	f := setup(t)
	calls := 0
	r := readerFunc(func([]byte) (int, error) { calls++; return 0, io.EOF })
	for _, v := range []struct {
		sc   storage.Scope
		size int64
		meta storage.LibraryMetadata
	}{{storage.Scope{}, 1, storage.LibraryMetadata{}}, {scope, 0, storage.LibraryMetadata{}}, {scope, 4<<20 + 1, storage.LibraryMetadata{}}, {scope, 1, storage.LibraryMetadata{Enabled: true}}, {scope, 1, storage.LibraryMetadata{Tags: []string{"x", "x"}}}} {
		_, _, e := f.m.Upload(context.Background(), f.l, v.sc, r, v.size, v.meta)
		if !errors.Is(e, ErrInvalid) {
			t.Fatal(e)
		}
	}
	if calls != 0 {
		t.Fatal("invalid read")
	}
}

type readerFunc func([]byte) (int, error)

func (r readerFunc) Read(b []byte) (int, error) { return r(b) }

func TestNewPrivateParentAndNoCatalogScan(t *testing.T) {
	f := setup(t)
	if _, e := New("", f.p); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := New(f.dir, nil); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, e := New(missing, f.p); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := os.Stat(missing); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("parent repaired", e)
	}
	writePrivate(t, filepath.Join(f.m.dir, "unknown"), []byte("x"))
	m, e := New(f.dir, f.p)
	if e != nil {
		t.Fatal("constructor scanned catalog", e)
	}
	m.Close()
}
func TestPublicArgumentBoundaries(t *testing.T) {
	f := setup(t)
	a := upload(t, f, "png")
	calls := 0
	l := func(context.Context) (*storage.Store, func(), error) { calls++; return f.s, func() {}, nil }
	bad := storage.LibraryMetadata{Tags: []string{"\n"}}
	for _, e := range []error{f.m.WithAsset(nil, l, scope, a.ID, false, consumeNothing), f.m.WithAsset(context.Background(), l, scope, a.ID, false, nil), f.m.WithAsset(context.Background(), l, scope, 0, false, consumeNothing), f.m.Update(context.Background(), l, scope, a.ID, bad), f.m.Delete(context.Background(), l, storage.Scope{}, a.ID), f.m.RecordUse(context.Background(), l, scope, a.ID, strings.Repeat("A", 64), 1), f.m.RecordUse(context.Background(), l, scope, a.ID, a.SHA256, 0)} {
		if !errors.Is(e, ErrInvalid) {
			t.Fatal("invalid API", e)
		}
	}
	if calls != 0 {
		t.Fatal("invalid input leased store")
	}
}
