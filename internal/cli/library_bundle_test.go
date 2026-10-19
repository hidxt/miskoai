package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/librarybundle"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
)

func bundleConfig(t *testing.T) config.Config {
	t.Helper()
	for _, k := range []string{"MISKOAI_LISTEN", "MISKOAI_DEEPSEEK_URL", "MISKOAI_MODEL", "MISKOAI_VISION_MODEL", "MISKOAI_WEIXIN_URL", "MISKOAI_MAX_OUTPUT_TOKENS", "MISKOAI_CONTEXT_BYTES", "MISKOAI_CONTEXT_TOKENS", "DEEPSEEK_API_KEY", "OLLAMA_API_KEY", "MISKOAI_CREDENTIALS_FILE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	t.Setenv("MISKOAI_DATA_DIR", filepath.Join(t.TempDir(), "private"))
	t.Setenv("MISKOAI_ADMIN_PASSWORD", "synthetic-password-123")
	c, e := config.Load()
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func bundlePrivate(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "private")
	if e := privatefs.EnsureDir(p); e != nil {
		t.Fatal(e)
	}
	return p
}
func bundleWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	f, e := privatefs.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	n, e := f.Write(b)
	ce := f.Close()
	if e != nil || ce != nil || n != len(b) {
		t.Fatal(e, ce)
	}
}
func bundleAuth(t *testing.T, cfg config.Config, account string) {
	t.Helper()
	if e := privatefs.EnsureDir(cfg.DataDir); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(cfg.DataDir, "weixin-auth.json")
	if _, e := os.Lstat(p); e == nil {
		if e = os.Remove(p); e != nil {
			t.Fatal(e)
		}
	}
	bundleWrite(t, p, []byte(fmt.Sprintf(`{"bot_token":"synthetic-token","account":%q,"allowed_user":"synthetic-user","base_url":"https://ilinkai.weixin.qq.com"}`, account)))
}
func bundleImage(t *testing.T, animated bool, v byte) []byte {
	t.Helper()
	var b bytes.Buffer
	if animated {
		im := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
		im.SetColorIndex(0, 0, 1)
		if e := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{im, im}, Delay: []int{1, 1}}); e != nil {
			t.Fatal(e)
		}
	} else {
		im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		im.Set(0, 0, color.NRGBA{R: v, A: 255})
		if e := png.Encode(&b, im); e != nil {
			t.Fatal(e)
		}
	}
	return b.Bytes()
}
func bundleSeed(t *testing.T, cfg config.Config, account string, b []byte) storage.LibraryAsset {
	t.Helper()
	bundleAuth(t, cfg, account)
	c, e := core.New(cfg, core.Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	}()
	a, created, e := c.LibraryUpload(context.Background(), bytes.NewReader(b), int64(len(b)), storage.LibraryMetadata{Description: "synthetic fixture", Tags: []string{"synthetic"}})
	if e != nil || !created {
		t.Fatal(e, created)
	}
	return a
}
func bundleBackup(t *testing.T, cfg config.Config) string {
	t.Helper()
	p := filepath.Join(bundlePrivate(t), "fixture.bundle")
	if e := backupLibrary(context.Background(), cfg, p, io.Discard, libraryCLIOptions()); e != nil {
		t.Fatal(e)
	}
	return p
}

func TestLibraryBundleCLIAllScopesCurrentImagesReusedID(t *testing.T) {
	cfg := bundleConfig(t)
	pngBytes, gifBytes := bundleImage(t, false, 31), bundleImage(t, true, 0)
	a := bundleSeed(t, cfg, "synthetic-first", pngBytes)
	b := bundleSeed(t, cfg, "synthetic-other", gifBytes)
	p := bundleBackup(t, cfg)
	dest := cfg
	dest.DataDir = bundlePrivate(t)
	old := bundleSeed(t, dest, "synthetic-first", bundleImage(t, false, 32))
	if a.ID != old.ID || a.SHA256 == old.SHA256 {
		t.Fatal("reused ID fixture")
	}
	var out bytes.Buffer
	if e := restoreLibrary(context.Background(), dest, p, &out, libraryCLIOptions()); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "prior_retained=true; channel_paused=true") {
		t.Fatal(out.String())
	}
	s, e := storage.Open(filepath.Join(dest.DataDir, "miskoai.db"))
	if e != nil {
		t.Fatal(e)
	}
	rows, e := s.LibrarySnapshot(context.Background())
	ce := s.Close()
	if e != nil || ce != nil || len(rows) != 2 {
		t.Fatal(rows, e, ce)
	}
	for _, fixture := range []struct {
		asset storage.LibraryAsset
		data  []byte
	}{{a, pngBytes}, {b, gifBytes}} {
		bundleAuth(t, dest, fixture.asset.Scope.Account)
		c, e := core.New(dest, core.Dependencies{})
		if e != nil {
			t.Fatal(e)
		}
		e = c.WithLibraryAsset(context.Background(), fixture.asset.ID, false, func(row storage.LibraryAsset, _ attachment.LocalImage, art attachment.Artifact) error {
			actual := make([]byte, len(fixture.data))
			n, e := art.ReadAt(actual, 0)
			if e != nil || n != len(actual) || !bytes.Equal(actual, fixture.data) || row.SHA256 != fixture.asset.SHA256 {
				t.Error("reopened actual media bytes", e)
			}
			return nil
		})
		ce = c.Close()
		if e != nil || ce != nil {
			t.Fatal(e, ce)
		}
	}
	paused, e := maintenance.RestorePaused(dest.DataDir)
	if e != nil || !paused {
		t.Fatal("persistent paused marker", e)
	}
	entries, e := os.ReadDir(filepath.Join(dest.DataDir, "library"))
	if e != nil || len(entries) != 3 {
		t.Fatal("current two plus rollback-only old image retained", len(entries), e)
	}
	// A new complete backup enumerates only the two CURRENT images, not old ID1.
	again := bundleBackup(t, dest)
	fresh := cfg
	fresh.DataDir = bundlePrivate(t)
	if e := restoreLibrary(context.Background(), fresh, again, io.Discard, libraryCLIOptions()); e != nil {
		t.Fatal(e)
	}
	entries, e = os.ReadDir(filepath.Join(fresh.DataDir, "library"))
	if e != nil || len(entries) != 2 {
		t.Fatal("rollback-only content leaked into bundle", len(entries), e)
	}
}

func TestLibraryBundleCLISourceAdmissionBeforeCore(t *testing.T) {
	for _, kind := range []string{"empty-path", "missing", "empty-file", "directory", "oversize", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			cfg := bundleConfig(t)
			dir := bundlePrivate(t)
			p := filepath.Join(dir, "input.bundle")
			switch kind {
			case "empty-path":
				p = ""
			case "empty-file":
				bundleWrite(t, p, nil)
			case "directory":
				p = dir
			case "oversize":
				f, e := privatefs.Create(p)
				if e != nil {
					t.Fatal(e)
				}
				e = f.Truncate(libraryCLILimit + 1)
				ce := f.Close()
				if e != nil || ce != nil {
					t.Fatal(e, ce)
				}
			case "hardlink":
				bundleWrite(t, p, []byte("synthetic"))
				if e := os.Link(p, filepath.Join(dir, "link")); e != nil {
					t.Fatal(e)
				}
			}
			calls := 0
			ops := libraryCLIOptions()
			ops.newCore = func(config.Config) (libraryCLIController, error) { calls++; return nil, errors.New("CANARY") }
			e := restoreLibrary(context.Background(), cfg, p, io.Discard, ops)
			if e == nil || calls != 0 || strings.Contains(e.Error(), p) && p != "" || strings.Contains(e.Error(), "CANARY") {
				t.Fatal("unsafe source reached effects or leaked", calls, e)
			}
			if _, e := os.Lstat(cfg.DataDir); !os.IsNotExist(e) {
				t.Fatal("source refusal constructed data directory", e)
			}
		})
	}
}

func TestLibraryBundleCLIActiveLockAndExistingDestination(t *testing.T) {
	cfg := bundleConfig(t)
	p := bundleBackup(t, cfg)
	before, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = backupLibrary(context.Background(), cfg, p, io.Discard, libraryCLIOptions()); e == nil {
		t.Fatal("existing destination overwritten")
	}
	after, e := os.ReadFile(p)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("existing changed", e)
	}
	owner, e := core.New(cfg, core.Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	dest := filepath.Join(bundlePrivate(t), "new.bundle")
	if e = backupLibrary(context.Background(), cfg, dest, io.Discard, libraryCLIOptions()); e == nil {
		t.Fatal("active backup lock")
	}
	if _, e = os.Lstat(dest); !os.IsNotExist(e) {
		t.Fatal("lock refusal leaked destination", e)
	}
	if e = restoreLibrary(context.Background(), cfg, p, io.Discard, libraryCLIOptions()); e == nil {
		t.Fatal("active restore lock")
	}
}

func TestLibraryBundleCLIConfigRequirementAndSyntax(t *testing.T) {
	cfg := bundleConfig(t)
	cfg.AdminPassword = ""
	p := filepath.Join(bundlePrivate(t), "new.bundle")
	if e := backupLibrary(context.Background(), cfg, p, io.Discard, libraryCLIOptions()); e == nil {
		t.Fatal("admin password bypass")
	}
	if _, e := os.Lstat(p); !os.IsNotExist(e) {
		t.Fatal("incomplete output retained", e)
	}
	for _, args := range [][]string{{"backup", "--library", ""}, {"restore", "--library", ""}, {"backup", "--unknown", "x"}, {"restore", "--library", "x", "extra"}, {"backup", "--library", "--unknown"}} {
		if e := Run(args, io.Discard); e == nil {
			t.Fatal("malformed accepted", args)
		}
	}
	var out bytes.Buffer
	if e := Run([]string{"help"}, &out); e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"backup --library PATH", "restore --library PATH", "321MiB", "stops generation before validation", "manual reconciliation"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal("help omitted", want)
		}
	}
}

type bundleCloseController struct {
	*core.Controller
	closeCalls *int
}

func (c *bundleCloseController) Close() error {
	*c.closeCalls++
	return errors.Join(c.Controller.Close(), errors.New("CANARY close"))
}

type bundleReaderClose struct {
	io.ReadCloser
	close func() error
	read  func([]byte) (int, error)
}

func (r *bundleReaderClose) Read(p []byte) (int, error) {
	if r.read != nil {
		return r.read(p)
	}
	return r.ReadCloser.Read(p)
}
func (r *bundleReaderClose) Close() error {
	if r.close != nil {
		return r.close()
	}
	return r.ReadCloser.Close()
}

type bundleFailOutput struct{}

func (bundleFailOutput) Write([]byte) (int, error) { return 0, errors.New("CANARY output") }

func TestLibraryBundleCLIBackupFailuresAndCompletedRetention(t *testing.T) {
	for _, kind := range []string{"write", "short-write", "sync", "close", "dirsync", "download-close", "last-read-error", "replacement", "controller-close", "output", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			cfg := bundleConfig(t)
			p := filepath.Join(bundlePrivate(t), "new.bundle")
			ops := libraryCLIOptions()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			closeCalls := 0
			out := io.Writer(io.Discard)
			switch kind {
			case "write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, errors.New("CANARY") }
			case "short-write":
				ops.write = func(_ *os.File, p []byte) (int, error) { return len(p) - 1, nil }
			case "sync":
				ops.sync = func(f *os.File) error { return errors.Join(f.Sync(), errors.New("CANARY")) }
			case "close":
				ops.close = func(f *os.File) error { return errors.Join(f.Close(), errors.New("CANARY")) }
			case "dirsync":
				ops.syncDir = func(string) error { return errors.New("CANARY") }
			case "download-close":
				ops.wrapDownload = func(d *core.Download) io.ReadCloser {
					return &bundleReaderClose{ReadCloser: d, close: func() error { return errors.Join(d.Close(), errors.New("CANARY")) }}
				}
			case "last-read-error":
				ops.wrapDownload = func(d *core.Download) io.ReadCloser {
					remaining := d.Size
					return &bundleReaderClose{ReadCloser: d, read: func(p []byte) (int, error) {
						n, e := d.Read(p)
						remaining -= int64(n)
						if remaining == 0 && n > 0 {
							return n, errors.New("CANARY")
						}
						return n, e
					}}
				}
			case "replacement":
				ops.write = func(f *os.File, b []byte) (int, error) {
					n, e := f.Write(b)
					if e == nil {
						if e = os.Rename(p, p+".original"); e == nil {
							bundleWrite(t, p, []byte("replacement"))
						}
					}
					return n, e
				}
			case "controller-close":
				ops.newCore = func(cfg config.Config) (libraryCLIController, error) {
					c, e := core.New(cfg, core.Dependencies{})
					if e != nil {
						return nil, e
					}
					return &bundleCloseController{c, &closeCalls}, nil
				}
			case "output":
				out = bundleFailOutput{}
			case "cancel":
				ops.write = func(f *os.File, b []byte) (int, error) { n, e := f.Write(b); cancel(); return n, e }
			}
			e := backupLibrary(ctx, cfg, p, out, ops)
			if e == nil || strings.Contains(e.Error(), "CANARY") || strings.Contains(e.Error(), p) {
				t.Fatal("failure hidden or leaked", e)
			}
			if kind == "cancel" && !errors.Is(e, context.Canceled) {
				t.Fatal("cancel lost", e)
			}
			retained := kind == "controller-close" || kind == "output" || kind == "replacement"
			if retained {
				if e := privatefs.CheckFile(p, libraryCLILimit); e != nil {
					t.Fatal("completed/replacement removed", e)
				}
			} else {
				if _, e := os.Lstat(p); !os.IsNotExist(e) {
					t.Fatal("incomplete original not removed", e)
				}
			}
			if kind == "replacement" {
				b, e := os.ReadFile(p)
				if e != nil || string(b) != "replacement" {
					t.Fatal("replacement changed", e)
				}
			}
			if kind == "controller-close" && closeCalls != 1 {
				t.Fatal("close replay", closeCalls)
			}
			// Actual Core lock/download teardown must permit a later invocation.
			next := filepath.Join(bundlePrivate(t), "next.bundle")
			if e := backupLibrary(context.Background(), cfg, next, io.Discard, libraryCLIOptions()); e != nil {
				t.Fatal("lifecycle leaked", e)
			}
		})
	}
}

func TestLibraryBundleCLIRestoreFailuresAndEffectRetention(t *testing.T) {
	for _, kind := range []string{"source-close", "controller-close", "output", "last-read-error", "replacement", "metadata", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			cfg := bundleConfig(t)
			bundleSeed(t, cfg, "synthetic-first", bundleImage(t, false, 47))
			p := bundleBackup(t, cfg)
			dest := cfg
			dest.DataDir = bundlePrivate(t)
			ops := libraryCLIOptions()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := io.Writer(io.Discard)
			calls := 0
			switch kind {
			case "source-close":
				ops.close = func(f *os.File) error { return errors.Join(f.Close(), errors.New("CANARY")) }
			case "controller-close":
				ops.newCore = func(cfg config.Config) (libraryCLIController, error) {
					c, e := core.New(cfg, core.Dependencies{})
					if e != nil {
						return nil, e
					}
					return &bundleCloseController{c, &calls}, nil
				}
			case "output":
				out = bundleFailOutput{}
			case "last-read-error":
				ops.wrapSource = func(f *os.File) io.Reader {
					info, e := f.Stat()
					if e != nil {
						t.Fatal(e)
					}
					remaining := info.Size()
					return readerFunction(func(b []byte) (int, error) {
						n, e := f.Read(b)
						remaining -= int64(n)
						if remaining == 0 && n > 0 {
							return n, errors.New("CANARY")
						}
						return n, e
					})
				}
			case "replacement":
				ops.wrapSource = func(f *os.File) io.Reader {
					once := false
					return readerFunction(func(b []byte) (int, error) {
						if !once {
							once = true
							if e := os.Rename(p, p+".original"); e != nil {
								t.Fatal(e)
							}
							bundleWrite(t, p, []byte("replacement"))
						}
						return f.Read(b)
					})
				}
			case "cancel":
				ops.wrapSource = func(f *os.File) io.Reader {
					return readerFunction(func(b []byte) (int, error) { n, e := f.Read(b); cancel(); return n, e })
				}
			case "metadata":
				ops.wrapSource = func(f *os.File) io.Reader {
					once := false
					return readerFunction(func(b []byte) (int, error) {
						if !once {
							once = true
							stamp := time.Unix(1000000000, 0)
							if e := os.Chtimes(p, stamp, stamp); e != nil {
								t.Fatal(e)
							}
						}
						return f.Read(b)
					})
				}
			}
			e := restoreLibrary(ctx, dest, p, out, ops)
			if e == nil || strings.Contains(e.Error(), "CANARY") || strings.Contains(e.Error(), p) {
				t.Fatal("restore failure hidden/leaked", e)
			}
			if kind == "cancel" && !errors.Is(e, context.Canceled) {
				t.Fatal("cancel lost", e)
			}
			s, e := storage.Open(filepath.Join(dest.DataDir, "miskoai.db"))
			if e != nil {
				t.Fatal(e)
			}
			rows, e := s.LibrarySnapshot(context.Background())
			ce := s.Close()
			if e != nil || ce != nil {
				t.Fatal(e, ce)
			}
			effected := kind == "source-close" || kind == "controller-close" || kind == "output" || kind == "replacement" || kind == "metadata"
			if effected && len(rows) != 1 || !effected && len(rows) != 0 {
				t.Fatal("actual effect disagrees", kind, len(rows))
			}
			if kind == "controller-close" && calls != 1 {
				t.Fatal("close retried", calls)
			}
		})
	}
}

type readerFunction func([]byte) (int, error)

func (f readerFunction) Read(p []byte) (int, error) { return f(p) }

func TestLibraryBundleCLIFormatConfusionAndLegacy(t *testing.T) {
	cfg := bundleConfig(t)
	p := bundleBackup(t, cfg)
	sqlite := filepath.Join(bundlePrivate(t), "sqlite.db")
	if e := backup(context.Background(), cfg, sqlite, io.Discard); e != nil {
		t.Fatal(e)
	}
	if e := restoreLibrary(context.Background(), cfg, sqlite, io.Discard, libraryCLIOptions()); e == nil {
		t.Fatal("SQLite accepted as bundle")
	}
	if e := restore(context.Background(), cfg, p, io.Discard); e == nil {
		t.Fatal("bundle accepted as SQLite")
	}
	db, e := sql.Open("sqlite", sqlite)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("DROP TABLE image_replies; DROP TABLE library_assets; PRAGMA user_version=4"); e != nil {
		db.Close()
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(sqlite)
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	header := librarybundle.Header{Database: librarybundle.File{Bytes: int64(len(raw)), SHA256: hex.EncodeToString(digest[:])}, Assets: []librarybundle.Asset{}}
	var encoded bytes.Buffer
	if e = librarybundle.Encode(context.Background(), &encoded, header, func(librarybundle.Record) (io.Reader, func() error, error) {
		return bytes.NewReader(raw), func() error { return nil }, nil
	}); e != nil {
		t.Fatal(e)
	}
	legacy := filepath.Join(bundlePrivate(t), "legacy.bundle")
	bundleWrite(t, legacy, encoded.Bytes())
	dest := cfg
	dest.DataDir = bundlePrivate(t)
	if e = restoreLibrary(context.Background(), dest, legacy, io.Discard, libraryCLIOptions()); e != nil {
		t.Fatal("legacy schema4 bundle", e)
	}
	reopened, e := storage.Open(filepath.Join(dest.DataDir, "miskoai.db"))
	if e != nil {
		t.Fatal(e)
	}
	rows, e := reopened.LibrarySnapshot(context.Background())
	ce := reopened.Close()
	if e != nil || ce != nil || len(rows) != 0 {
		t.Fatal("legacy migration", e, ce)
	}
	// Repeated old mode remains accepted after bundle refusal and keeps its text.
	var out bytes.Buffer
	if e = restore(context.Background(), cfg, sqlite, &out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "prior database retained: true") {
		t.Fatal("old behavior changed", out.String())
	}
}

func TestLibraryBundleCLIHeldDownloadJoinsBeforeReturn(t *testing.T) {
	cfg := bundleConfig(t)
	p := filepath.Join(bundlePrivate(t), "held.bundle")
	ops := libraryCLIOptions()
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	ops.wrapDownload = func(d *core.Download) io.ReadCloser {
		once := false
		return &bundleReaderClose{ReadCloser: d, read: func(p []byte) (int, error) {
			if !once {
				once = true
				close(entered)
				<-release
			}
			return d.Read(p)
		}}
	}
	go func() { done <- backupLibrary(ctx, cfg, p, io.Discard, ops) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-release:
		default:
			close(release)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("operation did not join")
		}
	})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual download not entered")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned before held I/O joined")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	select {
	case e := <-done:
		done <- e
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel not joined")
	}
	if _, e := os.Lstat(p); !os.IsNotExist(e) {
		t.Fatal("held canceled output retained", e)
	}
}

func TestLibraryBundleCLIHeldSourceJoinsBeforeReturn(t *testing.T) {
	cfg := bundleConfig(t)
	p := bundleBackup(t, cfg)
	dest := cfg
	dest.DataDir = bundlePrivate(t)
	ops := libraryCLIOptions()
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	ops.wrapSource = func(f *os.File) io.Reader {
		once := false
		return readerFunction(func(p []byte) (int, error) {
			if !once {
				once = true
				close(entered)
				<-release
			}
			return f.Read(p)
		})
	}
	go func() { done <- restoreLibrary(ctx, dest, p, io.Discard, ops) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-release:
		default:
			close(release)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("restore did not join")
		}
	})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual source not entered")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned before held source joined")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	select {
	case e := <-done:
		done <- e
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restore not joined")
	}
	c, e := core.New(dest, core.Dependencies{})
	if e != nil {
		t.Fatal("restore lock leaked", e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
}

func TestLibraryBundleCLIPrecanceledAndCloseCancellation(t *testing.T) {
	cfg := bundleConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := filepath.Join(bundlePrivate(t), "never.bundle")
	if e := backupLibrary(ctx, cfg, p, io.Discard, libraryCLIOptions()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := os.Lstat(p); !os.IsNotExist(e) {
		t.Fatal("precanceled output created", e)
	}
	if e := restoreLibrary(ctx, cfg, p, io.Discard, libraryCLIOptions()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := os.Lstat(cfg.DataDir); !os.IsNotExist(e) {
		t.Fatal("precanceled Core created", e)
	}
	p = bundleBackup(t, cfg)
	dest := cfg
	dest.DataDir = bundlePrivate(t)
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	ops := libraryCLIOptions()
	ops.close = func(f *os.File) error { e := f.Close(); cancel(); return e }
	if e := restoreLibrary(ctx, dest, p, io.Discard, ops); !errors.Is(e, context.Canceled) {
		t.Fatal("source cleanup cancellation hidden", e)
	}
	paused, e := maintenance.RestorePaused(dest.DataDir)
	if e != nil || !paused {
		t.Fatal("actual effect not retained", e)
	}
}

func TestLibraryBundleCLISemanticRoundTrip(t *testing.T) {
	cfg := bundleConfig(t)
	// Prove the real prerequisite works before exercising the missing CLI mode.
	c, e := core.New(cfg, core.Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	d, e := c.PrepareLibraryBundle(context.Background())
	if e != nil {
		c.Close()
		t.Fatal(e)
	}
	if _, e = io.Copy(io.Discard, d); e != nil {
		t.Fatal(e)
	}
	if e = d.Close(); e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(t.TempDir(), "private")
	if e = privatefs.EnsureDir(dir); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(dir, "snapshot.bundle")
	var out bytes.Buffer
	if e = Run([]string{"backup", "--library", p}, &out); e != nil {
		t.Fatal("complete-library CLI backup missing after real Core prerequisite:", e)
	}
	if e = privatefs.CheckFile(p, 321<<20); e != nil {
		t.Fatal(e)
	}
	out.Reset()
	if e = Run([]string{"restore", "--library", p}, &out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "prior_retained=true") || !strings.Contains(out.String(), "channel_paused=true") {
		t.Fatal("actual restore flags missing", out.String())
	}
}
