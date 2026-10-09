package maintenance

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func privateDir(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "private")
	if e := privatefs.EnsureDir(d); e != nil {
		t.Fatal(e)
	}
	return d
}

func TestRestorePauseStrictMarker(t *testing.T) {
	for _, payload := range []string{`{"version":"1","reason":"restore_requires_reconciliation"}`, `{"version":null,"reason":"restore_requires_reconciliation"}`, `{"version":1,"version":1,"reason":"restore_requires_reconciliation"}`, `{"Version":1,"reason":"restore_requires_reconciliation"}`, `{"version":1,"reason":"restore_requires_reconciliation","extra":true}`, `{"version":1,"reason":"restore_requires_reconciliation"} null`, `null`, string([]byte{0xff}), strings.Repeat("x", 1025)} {
		t.Run(payload[:min(len(payload), 24)], func(t *testing.T) {
			d := privateDir(t)
			f, e := privatefs.Create(filepath.Join(d, pauseName))
			if e != nil {
				t.Fatal(e)
			}
			f.WriteString(payload)
			f.Close()
			if _, e = RestorePaused(d); e == nil {
				t.Fatal("malformed marker accepted")
			}
		})
	}
}
func TestCandidateChangedRefused(t *testing.T) {
	_, b := snapshot(t)
	d := privateDir(t)
	c, e := PrepareRestore(context.Background(), d, bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Discard()
	if e = os.WriteFile(c.path, []byte("changed candidate"), 0600); e != nil {
		t.Fatal(e)
	}
	l, e := Acquire(d)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	if _, e = InstallRestore(context.Background(), l, c); !errors.Is(e, errOwnership) {
		t.Fatal("changed snapshot admitted past ownership check", e)
	}
	if _, e = os.Lstat(filepath.Join(d, pauseName)); !os.IsNotExist(e) {
		t.Fatal("pause created for unowned candidate")
	}
}
func TestRestoreRollback(t *testing.T) {
	for _, kind := range []string{"preserve", "install", "reopen", "rollback"} {
		t.Run(kind, func(t *testing.T) {
			_, b := snapshot(t)
			d := privateDir(t)
			p := filepath.Join(d, "miskoai.db")
			old := []byte("previous synthetic bytes")
			if e := os.WriteFile(p, old, 0600); e != nil {
				t.Fatal(e)
			}
			c, e := PrepareRestore(context.Background(), d, bytes.NewReader(b))
			if e != nil {
				t.Fatal(e)
			}
			defer c.Discard()
			l, e := Acquire(d)
			if e != nil {
				t.Fatal(e)
			}
			defer l.Close()
			fault := errors.New("injected private operation failure")
			ops := installOperations{rename: func(a, z string) error {
				if kind == "preserve" && a == p {
					return fault
				}
				if (kind == "install" || kind == "rollback") && a == c.path {
					return fault
				}
				if kind == "rollback" && strings.HasPrefix(filepath.Base(a), "pre-restore-") {
					return fault
				}
				return os.Rename(a, z)
			}, open: func(path string) (*storage.Store, error) {
				if kind == "reopen" {
					return nil, fault
				}
				return storage.Open(path)
			}}
			r, e := installRestore(context.Background(), l, c, ops)
			if e == nil {
				t.Fatal("fault ignored")
			}
			paused, pe := RestorePaused(d)
			if pe != nil || !paused || !r.ChannelPaused {
				t.Fatal("conservative pause lost", pe)
			}
			if kind == "rollback" {
				if !errors.Is(e, errRecovery) || !r.PreviousRetained {
					t.Fatal("manual recovery not reported", r, e)
				}
				files, _ := filepath.Glob(filepath.Join(d, "pre-restore-*.db"))
				if len(files) != 1 {
					t.Fatal("recovery snapshot absent")
				}
				got, _ := os.ReadFile(files[0])
				if !bytes.Equal(got, old) {
					t.Fatal("recovery bytes lost")
				}
			} else {
				got, e := os.ReadFile(p)
				if e != nil || !bytes.Equal(got, old) {
					t.Fatal("rollback lost original", e)
				}
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("synthetic private reader error")
}

type fillReader struct{}

func (fillReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
func TestPrepareRestoreFailedReaderAndOversize(t *testing.T) {
	for _, r := range []io.Reader{failingReader{}, io.LimitReader(fillReader{}, maxRestoreBytes+1)} {
		d := privateDir(t)
		if _, e := PrepareRestore(context.Background(), d, r); e == nil {
			t.Fatal("failed/oversize reader accepted")
		}
		files, _ := filepath.Glob(filepath.Join(d, "restore-*"))
		if len(files) != 0 {
			t.Fatal("owned candidate not cleaned")
		}
	}
}
func TestInstallRequiresExactLiveLock(t *testing.T) {
	_, b := snapshot(t)
	d := privateDir(t)
	c, e := PrepareRestore(context.Background(), d, bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Discard()
	other, e := Acquire(privateDir(t))
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if _, e = InstallRestore(context.Background(), nil, c); e == nil {
		t.Fatal("nil lock accepted")
	}
	if _, e = InstallRestore(context.Background(), other, c); e == nil {
		t.Fatal("other directory lock accepted")
	}
	l, e := Acquire(d)
	if e != nil {
		t.Fatal(e)
	}
	l.Close()
	if _, e = InstallRestore(context.Background(), l, c); e == nil {
		t.Fatal("closed lock accepted")
	}
	if e = ClearRestorePause(l); e == nil {
		t.Fatal("closed reconciliation accepted")
	}
}

type failingDurableFile struct {
	*os.File
	kind string
}

func (f failingDurableFile) Write(p []byte) (int, error) {
	if f.kind == "write" {
		return 0, io.ErrShortWrite
	}
	return f.File.Write(p)
}
func (f failingDurableFile) Sync() error {
	if f.kind == "sync" {
		return errors.New("synthetic sync failure")
	}
	return f.File.Sync()
}
func (f failingDurableFile) Close() error {
	e := f.File.Close()
	if f.kind == "close" {
		return errors.New("synthetic close failure")
	}
	return e
}
func TestPauseDurabilityFailureRefusesInstall(t *testing.T) {
	for _, kind := range []string{"create", "write", "sync", "close"} {
		t.Run(kind, func(t *testing.T) {
			_, b := snapshot(t)
			d := privateDir(t)
			p := filepath.Join(d, "miskoai.db")
			old := []byte("previous synthetic unchanged")
			os.WriteFile(p, old, 0600)
			c, e := PrepareRestore(context.Background(), d, bytes.NewReader(b))
			if e != nil {
				t.Fatal(e)
			}
			defer c.Discard()
			l, e := Acquire(d)
			if e != nil {
				t.Fatal(e)
			}
			defer l.Close()
			ops := installOperations{rename: os.Rename, open: storage.Open, pause: func(l *Lock) error {
				return ensurePauseWith(l, func(path string) (durableFile, error) {
					if kind == "create" {
						return nil, errors.New("synthetic create failure")
					}
					f, e := privatefs.Create(path)
					if e != nil {
						return nil, e
					}
					return failingDurableFile{f, kind}, nil
				})
			}}
			if _, e = installRestore(context.Background(), l, c, ops); e == nil {
				t.Fatal("pause durability failure allowed install")
			}
			got, _ := os.ReadFile(p)
			if !bytes.Equal(got, old) {
				t.Fatal("target changed before durable pause")
			}
			files, _ := filepath.Glob(filepath.Join(d, "pre-restore-*.db"))
			if len(files) != 0 {
				t.Fatal("previous moved before durable pause")
			}
		})
	}
}
func TestLifecycleLockOwnership(t *testing.T) {
	d := privateDir(t)
	l, e := Acquire(d)
	if e != nil {
		t.Fatal("lifecycle lock unavailable", e)
	}
	if _, e = Acquire(d); e == nil {
		t.Fatal("second owner admitted")
	}
	if e = l.Close(); e != nil {
		t.Fatal(e)
	}
	if e = l.Close(); e != nil {
		t.Fatal("close not idempotent", e)
	}
	f, e := privatefs.Create(filepath.Join(d, ".miskoai.lock"))
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
	if _, e = Acquire(d); e == nil {
		t.Fatal("stale lock removed")
	}
	os.Remove(filepath.Join(d, ".miskoai.lock"))
	l, e = Acquire(d)
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(d, ".miskoai.lock")
	if e = os.Rename(p, p+".old"); e != nil {
		t.Fatal(e)
	}
	f, e = privatefs.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	f.WriteString("replacement")
	f.Close()
	if e = l.Close(); e == nil {
		t.Fatal("replacement not refused")
	}
	b, e := os.ReadFile(p)
	if e != nil || string(b) != "replacement" {
		t.Fatal("replacement removed", e)
	}
}

func TestLockRejectsMovedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows refuses renaming a directory containing a live lock handle; Linux directory identity check requires native CI")
	}
	d := privateDir(t)
	l, e := Acquire(d)
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(d, ".miskoai.lock")
	if e = os.Rename(d, d+".old"); e != nil {
		t.Fatal(e)
	}
	if e = privatefs.EnsureDir(d); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(filepath.Join(d+".old", ".miskoai.lock"), p); e != nil {
		t.Fatal(e)
	}
	if e = l.Close(); e == nil {
		t.Fatal("moved lock admitted for replacement directory")
	}
	if _, e = os.Stat(p); e != nil {
		t.Fatal("replacement directory lock removed", e)
	}
}
func TestLockRejectsMismatchedDirectory(t *testing.T) {
	d := privateDir(t)
	l, e := Acquire(d)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	l.dir = privateDir(t)
	if e = ClearRestorePause(l); e == nil {
		t.Fatal("mismatched directory admitted for reconciliation")
	}
	l.dir = d
}

func TestRestoreSourceFutureUnsafeOversize(t *testing.T) {
	for _, kind := range []string{"future", "unsafe", "oversize", "candidate-alias"} {
		t.Run(kind, func(t *testing.T) {
			src, _ := snapshot(t)
			d := privateDir(t)
			p := filepath.Join(d, "miskoai.db")
			old := []byte("previous unchanged fixture")
			if e := os.WriteFile(p, old, 0600); e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "future":
				db, e := sql.Open("sqlite", src)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = db.Exec("PRAGMA user_version=999"); e != nil {
					t.Fatal(e)
				}
				if e = db.Close(); e != nil {
					t.Fatal(e)
				}
			case "unsafe":
				src = filepath.Join(t.TempDir(), "broad.db")
				if e := os.WriteFile(src, []byte("unsafe synthetic source"), 0644); e != nil {
					t.Fatal(e)
				}
			case "oversize":
				f, e := os.OpenFile(src, os.O_RDWR, 0600)
				if e != nil {
					t.Fatal(e)
				}
				if e = f.Truncate(maxRestoreBytes + 1); e != nil {
					t.Fatal(e)
				}
				f.Close()
			case "candidate-alias":
				copyPath := filepath.Join(d, "restore-synthetic.tmp")
				b, e := os.ReadFile(src)
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(copyPath, b, 0600); e != nil {
					t.Fatal(e)
				}
				src = copyPath
			}
			before, e := os.Stat(src)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = StoppedRestore(context.Background(), d, src); e == nil {
				t.Fatal("refused source admitted")
			}
			after, e := os.Stat(src)
			if e != nil || !sameMetadata(before, after) {
				t.Fatal("source metadata changed", e)
			}
			got, _ := os.ReadFile(p)
			if !bytes.Equal(got, old) {
				t.Fatal("target changed")
			}
			files, _ := filepath.Glob(filepath.Join(d, "pre-restore-*.db"))
			if len(files) != 0 {
				t.Fatal("target preserved before refusal")
			}
		})
	}
}

func TestCandidateDiscardRefusesReplacementAndJournals(t *testing.T) {
	for _, kind := range []string{"replacement", "-wal", "-shm"} {
		t.Run(kind, func(t *testing.T) {
			_, b := snapshot(t)
			d := privateDir(t)
			c, e := PrepareRestore(context.Background(), d, bytes.NewReader(b))
			if e != nil {
				t.Fatal(e)
			}
			p := c.path
			if kind == "replacement" {
				if e = os.Rename(p, p+".owned"); e != nil {
					t.Fatal(e)
				}
				f, e := privatefs.Create(p)
				if e != nil {
					t.Fatal(e)
				}
				f.WriteString("unowned replacement")
				f.Close()
			} else {
				f, e := privatefs.Create(p + kind)
				if e != nil {
					t.Fatal(e)
				}
				f.WriteString("unexplained synthetic sidecar")
				f.Close()
			}
			before, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			if e = c.Discard(); e == nil {
				t.Fatal("unowned object discarded")
			}
			after, e := os.ReadFile(p)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("candidate/replacement deleted", e)
			}
			if kind != "replacement" {
				got, e := os.ReadFile(p + kind)
				if e != nil || string(got) != "unexplained synthetic sidecar" {
					t.Fatal("sidecar deleted", e)
				}
			}
		})
	}
}
func TestRestoreRefusesSourceJournals(t *testing.T) {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		t.Run(suffix, func(t *testing.T) {
			src, b := snapshot(t)
			f, e := privatefs.Create(src + suffix)
			if e != nil {
				t.Fatal(e)
			}
			f.WriteString("unexplained source journal")
			f.Close()
			d := privateDir(t)
			if _, e = StoppedRestore(context.Background(), d, src); e == nil {
				t.Fatal("source with journals accepted as complete snapshot")
			}
			got, e := os.ReadFile(src)
			if e != nil || !bytes.Equal(got, b) {
				t.Fatal("source main changed", e)
			}
			journal, e := os.ReadFile(src + suffix)
			if e != nil || string(journal) != "unexplained source journal" {
				t.Fatal("source journal changed", e)
			}
			if _, e = os.Stat(filepath.Join(d, "miskoai.db")); !os.IsNotExist(e) {
				t.Fatal("target installed from incomplete snapshot")
			}
		})
	}
}
func TestRestoreDirectorySyncFailure(t *testing.T) {
	for _, failAt := range []int{1, 2, 3, 4, 5} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			_, b := snapshot(t)
			d := privateDir(t)
			p := filepath.Join(d, "miskoai.db")
			old := []byte("previous sync fixture bytes")
			os.WriteFile(p, old, 0600)
			c, e := PrepareRestore(context.Background(), d, bytes.NewReader(b))
			if e != nil {
				t.Fatal(e)
			}
			defer c.Discard()
			l, e := Acquire(d)
			if e != nil {
				t.Fatal(e)
			}
			defer l.Close()
			calls := 0
			ops := installOperations{rename: os.Rename, open: func(path string) (*storage.Store, error) {
				if failAt >= 4 {
					return nil, errors.New("synthetic reopen failure")
				}
				return storage.Open(path)
			}, syncDir: func(dir string) error {
				if dir != d {
					t.Fatal("wrong sync directory")
				}
				calls++
				if calls == failAt {
					return errors.New("synthetic directory sync failure")
				}
				return syncDirectory(dir)
			}}
			r, e := installRestore(context.Background(), l, c, ops)
			if e == nil {
				t.Fatal("directory sync failure ignored")
			}
			if calls < failAt {
				t.Fatal("missing ordered directory sync", calls)
			}
			paused, pe := RestorePaused(d)
			if pe != nil || !paused || !r.ChannelPaused {
				t.Fatal("pause lost on sync failure", pe)
			}
			if failAt <= 3 {
				got, e := os.ReadFile(p)
				if e != nil || !bytes.Equal(got, old) {
					t.Fatal("rollback lost old bytes", e)
				}
			} else {
				files, _ := filepath.Glob(filepath.Join(d, "pre-restore-*.db"))
				if len(files) != 1 || !r.PreviousRetained || !errors.Is(e, errRecovery) {
					t.Fatal("sync recovery snapshot absent", files, r, e)
				}
				got, _ := os.ReadFile(files[0])
				if !bytes.Equal(got, old) {
					t.Fatal("recovery bytes changed")
				}
			}
		})
	}
}
func TestRestoreRetrySyncsExistingPause(t *testing.T) {
	_, b := snapshot(t)
	d := privateDir(t)
	p := filepath.Join(d, "miskoai.db")
	old := []byte("retry original bytes")
	os.WriteFile(p, old, 0600)
	c, e := PrepareRestore(context.Background(), d, bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Discard()
	l, e := Acquire(d)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	if e = ensurePause(l); e != nil {
		t.Fatal(e)
	}
	ops := installOperations{rename: os.Rename, open: storage.Open, syncDir: func(string) error { return errors.New("synthetic retry sync failure") }}
	if _, e = installRestore(context.Background(), l, c, ops); e == nil {
		t.Fatal("existing marker bypassed directory sync")
	}
	got, e := os.ReadFile(p)
	if e != nil || !bytes.Equal(got, old) {
		t.Fatal("retry changed old bytes", e)
	}
}

type cancelReader struct{ cancel context.CancelFunc }

func (r cancelReader) Read(p []byte) (int, error) { p[0] = 0; r.cancel(); return 1, nil }
func TestPrepareRestoreCancellationDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := privateDir(t)
	if _, e := PrepareRestore(ctx, d, cancelReader{cancel}); !errors.Is(e, context.Canceled) {
		t.Fatal("cooperative reader cancellation not classified", e)
	}
	files, _ := filepath.Glob(filepath.Join(d, "restore-*"))
	if len(files) != 0 {
		t.Fatal("canceled candidate retained")
	}
}

func TestBackupValidationCleanupRetainsReplacement(t *testing.T) {
	for _, kind := range []string{"before-observation", "during-validation"} {
		t.Run(kind, func(t *testing.T) {
			d := privateDir(t)
			s, e := storage.Open(filepath.Join(d, "miskoai.db"))
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			dest := filepath.Join(d, "snapshot.db")
			var replacement os.FileInfo
			replace := func() {
				if e := os.Rename(dest, dest+".owned"); e != nil {
					t.Fatal(e)
				}
				f, e := privatefs.Create(dest)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = f.WriteString("unowned synthetic replacement"); e != nil {
					t.Fatal(e)
				}
				if e = f.Close(); e != nil {
					t.Fatal(e)
				}
				replacement, e = os.Stat(dest)
				if e != nil {
					t.Fatal(e)
				}
			}
			ops := backupOperations{snapshot: func(ctx context.Context, s *storage.Store, p string) (os.FileInfo, error) {
				owned, e := s.BackupOwned(ctx, p)
				if e != nil {
					return nil, e
				}
				if kind == "before-observation" {
					replace()
				}
				return owned, nil
			}, validate: func(context.Context, string) error {
				if kind == "during-validation" {
					replace()
				}
				return errors.New("synthetic validation failure")
			}}
			if e = backupWithOperations(context.Background(), s, dest, ops); e == nil {
				t.Fatal("validation failure ignored")
			}
			got, e := os.ReadFile(dest)
			if e != nil || string(got) != "unowned synthetic replacement" {
				t.Fatal("unowned replacement removed or changed", e)
			}
			after, e := os.Stat(dest)
			if e != nil || !sameMetadata(after, replacement) {
				t.Fatal("replacement metadata changed", e)
			}
		})
	}
}

func TestBackupRejectsSupportedReplacementAfterValidation(t *testing.T) {
	d := privateDir(t)
	s, e := storage.Open(filepath.Join(d, "miskoai.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	dest := filepath.Join(d, "snapshot.db")
	var before []byte
	ops := backupOperations{validate: func(ctx context.Context, path string) error {
		var e error
		before, e = os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.Rename(path, path+".owned"); e != nil {
			t.Fatal(e)
		}
		f, e := privatefs.Create(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(before); e != nil {
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
		return storage.ValidateBackup(ctx, path)
	}}
	if e = backupWithOperations(context.Background(), s, dest, ops); e == nil {
		t.Fatal("supported replacement reported as owned backup")
	}
	got, e := os.ReadFile(dest)
	if e != nil || !bytes.Equal(got, before) {
		t.Fatal("supported replacement removed or modified", e)
	}
}

func TestBackupValidationFailureCleansOwnedSnapshot(t *testing.T) {
	d := privateDir(t)
	s, e := storage.Open(filepath.Join(d, "miskoai.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	dest := filepath.Join(d, "snapshot.db")
	ops := backupOperations{validate: func(context.Context, string) error { return errors.New("synthetic invalid owned snapshot") }}
	if e = backupWithOperations(context.Background(), s, dest, ops); e == nil {
		t.Fatal("validation error ignored")
	}
	if _, e = os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("owned invalid backup not removed", e)
	}
}

func snapshot(t *testing.T) (string, []byte) {
	t.Helper()
	d := privateDir(t)
	s, e := storage.Open(filepath.Join(d, "miskoai.db"))
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(d, "snapshot.db")
	if e = s.Backup(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return p, b
}
func TestBackupRefusesRunningOwner(t *testing.T) {
	d := privateDir(t)
	s, e := storage.Open(filepath.Join(d, "miskoai.db"))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	l, e := Acquire(d)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	p := filepath.Join(d, "miskoai.db")
	before, _ := os.ReadFile(p)
	dest := filepath.Join(d, "out.db")
	if e = StoppedBackup(context.Background(), d, dest); e == nil {
		t.Fatal("running owner ignored")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("source mutated")
	}
	if _, e = os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("destination created")
	}
}
func TestRestorePauseSurvivesRestart(t *testing.T) {
	src, b := snapshot(t)
	d := privateDir(t)
	p := filepath.Join(d, "miskoai.db")
	if e := os.WriteFile(p, []byte("old synthetic database"), 0600); e != nil {
		t.Fatal(e)
	}
	r, e := StoppedRestore(context.Background(), d, src)
	if e != nil {
		t.Fatal("restore unavailable", e)
	}
	if !r.PreviousRetained || !r.ChannelPaused {
		t.Fatal("restore omitted retained/paused state", r)
	}
	got, _ := os.ReadFile(src)
	if !bytes.Equal(got, b) {
		t.Fatal("source changed")
	}
	paused, e := RestorePaused(d)
	if e != nil || !paused {
		t.Fatal("pause not durable", e)
	}
	l, e := Acquire(d)
	if e != nil {
		t.Fatal(e)
	}
	if e = ClearRestorePause(l); e != nil {
		t.Fatal(e)
	}
	if e = l.Close(); e != nil {
		t.Fatal(e)
	}
	paused, e = RestorePaused(d)
	if e != nil || paused {
		t.Fatal("explicit clear failed", e)
	}
	matches, _ := filepath.Glob(filepath.Join(d, "pre-restore-*.db"))
	if len(matches) != 1 {
		t.Fatal("previous snapshot absent")
	}
	old, _ := os.ReadFile(matches[0])
	if string(old) != "old synthetic database" {
		t.Fatal("previous bytes lost")
	}
}
func TestPrepareRestoreSnapshotAndCancellation(t *testing.T) {
	_, b := snapshot(t)
	d := privateDir(t)
	c, e := PrepareRestore(context.Background(), d, bytes.NewReader(b))
	if e != nil {
		t.Fatal("candidate unavailable", e)
	}
	if e = c.Discard(); e != nil {
		t.Fatal(e)
	}
	if e = c.Discard(); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = PrepareRestore(ctx, d, bytes.NewReader(b)); e == nil {
		t.Fatal("cancel ignored")
	}
}
func TestRestoreRefusalPreservesBytes(t *testing.T) {
	for _, kind := range []string{"corrupt", "alias", "wal", "shm"} {
		t.Run(kind, func(t *testing.T) {
			src, _ := snapshot(t)
			d := privateDir(t)
			p := filepath.Join(d, "miskoai.db")
			old := []byte("unchanged synthetic old")
			if e := os.WriteFile(p, old, 0600); e != nil {
				t.Fatal(e)
			}
			if kind == "corrupt" {
				src = filepath.Join(d, "bad.db")
				os.WriteFile(src, []byte("corrupt snapshot bytes"), 0600)
			}
			if kind == "alias" {
				src = p
			}
			if kind == "wal" || kind == "shm" {
				f, e := privatefs.Create(p + "-" + kind)
				if e != nil {
					t.Fatal(e)
				}
				f.WriteString("unexplained journal")
				f.Close()
			}
			if _, e := StoppedRestore(context.Background(), d, src); e == nil {
				t.Fatal("invalid restore accepted")
			}
			got, _ := os.ReadFile(p)
			if !bytes.Equal(got, old) {
				t.Fatal("target bytes changed")
			}
		})
	}
}
