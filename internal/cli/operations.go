package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
)

func privateDir(dir string) error { return privatefs.EnsureDir(dir) }

func initialize(c config.Config, out io.Writer) error {
	if err := privateDir(c.DataDir); err != nil {
		return err
	}
	if err := config.InitSettings(c.DataDir, c.EffectiveSettings()); err != nil {
		return errors.New("configuration already exists or cannot be created")
	}
	_, err := fmt.Fprintln(out, "initialized private settings; environment overrides saved values")
	return err
}

func doctor(c config.Config, out io.Writer) error {
	dir, err := os.MkdirTemp("", "miskoai-doctor-")
	if err != nil {
		return errors.New("cannot create diagnostic directory")
	}
	defer os.RemoveAll(dir)
	private := filepath.Join(dir, "private")
	if err := privatefs.EnsureDir(private); err != nil {
		return err
	}
	db, err := storage.Open(filepath.Join(private, "probe.db"))
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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := maintenance.StoppedBackup(ctx, c.DataDir, dest); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "consistent backup completed")
	return err
}

func restore(ctx context.Context, c config.Config, source string, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := maintenance.StoppedRestore(ctx, c.DataDir, source)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "restore completed; prior database retained: %t; external channels paused pending explicit operator reconciliation\n", result.PreviousRetained)
	return err
}
