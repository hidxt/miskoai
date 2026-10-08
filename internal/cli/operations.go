package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/storage"
)

func privateDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return errors.New("invalid private directory")
	}
	cwd, _ := os.Getwd()
	if abs == filepath.VolumeName(abs)+string(os.PathSeparator) || abs == cwd {
		return errors.New("use a dedicated data directory")
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return errors.New("cannot create private directory")
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("private directory is not a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("data directory requires owner-only permissions")
	}
	return nil
}

func initialize(c config.Config, out io.Writer) error {
	if err := privateDir(c.DataDir); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(c.DataDir, "settings.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("configuration already exists or cannot be created")
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(c)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.New("configuration write failed")
	}
	_, err = fmt.Fprintln(out, "initialized private settings template; environment is authoritative for this development version")
	return err
}

func doctor(c config.Config, out io.Writer) error {
	dir, err := os.MkdirTemp("", "miskoai-doctor-")
	if err != nil {
		return errors.New("cannot create diagnostic directory")
	}
	defer os.RemoveAll(dir)
	db, err := storage.Open(filepath.Join(dir, "probe.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.Integrity(ctx); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "MiskoAI %s; %s %s/%s\nSQLite migrations/FTS5/integrity: passed (synthetic local database)\nDeepSeek configured: %t; Ollama configured: %t\nLive API, WeChat and Linux 512MB validation: not performed by doctor\n", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH, c.DeepSeekKey != "", c.OllamaKey != "")
	return err
}

func backup(ctx context.Context, c config.Config, dest string, out io.Writer) error {
	path := filepath.Join(c.DataDir, "miskoai.db")
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return errors.New("source database does not exist as a regular file")
	}
	s, err := storage.Open(path)
	if err != nil {
		return err
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err = s.Backup(ctx, dest); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "consistent backup completed")
	return err
}

// Restore requires a stopped service. All future serve operations must retain
// this exclusive lock for their entire lifetime; stale locks require inspection.
func restore(ctx context.Context, c config.Config, source string, out io.Writer) error {
	if err := privateDir(c.DataDir); err != nil {
		return err
	}
	lockPath := filepath.Join(c.DataDir, ".miskoai.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("data directory locked; stop service and inspect stale locks before restore")
	}
	defer func() { lock.Close(); os.Remove(lockPath) }()
	path := filepath.Join(c.DataDir, "miskoai.db")
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err = os.Lstat(path + suffix); !errors.Is(err, os.ErrNotExist) {
			return errors.New("SQLite journal files exist; stop and checkpoint database before restore")
		}
	}
	sourceAbs, err := filepath.Abs(source)
	if err != nil || sourceAbs == path {
		return errors.New("invalid restore source")
	}
	info, err := os.Lstat(sourceAbs)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 16 || info.Size() > 256<<20 {
		return errors.New("invalid restore backup file")
	}
	in, err := os.Open(sourceAbs)
	if err != nil {
		return errors.New("cannot read restore source")
	}
	defer in.Close()
	temp, err := os.CreateTemp(c.DataDir, "restore-*.tmp")
	if err != nil {
		return errors.New("cannot create restore candidate")
	}
	candidate := temp.Name()
	defer func() { os.Remove(candidate); os.Remove(candidate + "-wal"); os.Remove(candidate + "-shm") }()
	n, copyErr := io.Copy(temp, io.LimitReader(in, 256<<20+1))
	syncErr := temp.Sync()
	closeErr := temp.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || n > 256<<20 {
		return errors.New("restore copy failed or exceeded limit")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err = storage.ValidateBackup(ctx, candidate); err != nil {
		return errors.New("restore candidate is not a supported MiskoAI snapshot")
	}
	check, err := storage.Open(candidate)
	if err != nil {
		return errors.New("restore candidate invalid")
	}
	err = check.Integrity(ctx)
	closeCheckErr := check.Close()
	if err != nil || closeCheckErr != nil {
		return errors.New("restore integrity check failed")
	}
	previous := ""
	if info, e := os.Lstat(path); e == nil {
		if !info.Mode().IsRegular() {
			return errors.New("restore target is not regular")
		}
		previous = filepath.Join(c.DataDir, fmt.Sprintf("pre-restore-%d.db", time.Now().UnixNano()))
		if err = os.Rename(path, previous); err != nil {
			var pathErr *os.PathError
			if errors.As(err, &pathErr) {
				return fmt.Errorf("cannot preserve existing database: %w", pathErr.Err)
			}
			var linkErr *os.LinkError
			if errors.As(err, &linkErr) {
				return fmt.Errorf("cannot preserve existing database: %w", linkErr.Err)
			}
			return errors.New("cannot preserve existing database")
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return errors.New("cannot inspect restore target")
	}
	if err = os.Rename(candidate, path); err != nil {
		if previous != "" {
			if os.Rename(previous, path) != nil {
				return errors.New("restore failed; prior database retained under pre-restore name; manual recovery required")
			}
		}
		return errors.New("restore installation failed")
	}
	_, err = fmt.Fprintln(out, "restore completed; prior database preserved in private data directory when present")
	return err
}
