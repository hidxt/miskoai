package core

import (
	"fmt"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureConfig(t *testing.T) config.Config {
	t.Helper()
	for _, k := range []string{"MISKOAI_DATA_DIR", "MISKOAI_LISTEN", "MISKOAI_DEEPSEEK_URL", "MISKOAI_MODEL", "MISKOAI_VISION_MODEL", "MISKOAI_WEIXIN_URL", "MISKOAI_MAX_OUTPUT_TOKENS", "MISKOAI_CONTEXT_BYTES", "MISKOAI_CONTEXT_TOKENS", "DEEPSEEK_API_KEY", "OLLAMA_API_KEY", "MISKOAI_CREDENTIALS_FILE", "MISKOAI_ADMIN_PASSWORD"} {
		old, ok := os.LookupEnv(k)
		os.Unsetenv(k)
		t.Cleanup(func() {
			if ok {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
	dir := filepath.Join(t.TempDir(), "private")
	if err := privatefs.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MISKOAI_DATA_DIR", dir)
	t.Setenv("MISKOAI_ADMIN_PASSWORD", "synthetic-password-123")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
func writeFixture(t *testing.T, dir, name, body string) {
	t.Helper()
	f, err := privatefs.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(body); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestCoreMissingAndInvalidAuthorization(t *testing.T) {
	cfg := fixtureConfig(t)
	c, err := New(cfg, Dependencies{})
	if err != nil {
		t.Fatal("missing authorization must permit management:", err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-secret-canary"}`)
	if c, err = New(cfg, Dependencies{}); err == nil {
		c.Close()
		t.Fatal("invalid authorization accepted")
	} else if strings.Contains(err.Error(), "canary") {
		t.Fatal("secret leaked")
	}
}
func TestConstructorUnwindsLock(t *testing.T) {
	cfg := fixtureConfig(t)
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{}`)
	if _, err := New(cfg, Dependencies{}); err == nil {
		t.Fatal("bad auth accepted")
	}
	l, err := maintenance.Acquire(cfg.DataDir)
	if err != nil {
		t.Fatal("constructor retained lock", err)
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConstructorFactoryFailureClosesStore(t *testing.T) {
	cfg := fixtureConfig(t)
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	if _, err := New(cfg, Dependencies{ChannelFactory: func(config.Authorization) (Channel, error) { return nil, fmt.Errorf("synthetic failure") }}); err == nil {
		t.Fatal("factory failure accepted")
	}
	// Normal store close must remove engine journals before releasing ownership.
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Lstat(filepath.Join(cfg.DataDir, "miskoai.db") + suffix); !os.IsNotExist(err) {
			t.Fatal("constructor leaked engine connection", suffix, err)
		}
	}
}

func TestUnexplainedJournalsRefuseBeforeMainCreation(t *testing.T) {
	for _, empty := range []bool{false, true} {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			for _, unsafe := range []bool{false, true} {
				t.Run(fmt.Sprintf("empty=%v/%s/unsafe=%v", empty, suffix, unsafe), func(t *testing.T) {
					cfg := fixtureConfig(t)
					main := filepath.Join(cfg.DataDir, "miskoai.db")
					if empty {
						writeFixture(t, cfg.DataDir, "miskoai.db", "")
					}
					var originalMain os.FileInfo
					if empty {
						f, e := os.Open(main)
						if e != nil {
							t.Fatal(e)
						}
						originalMain, e = f.Stat()
						ce := f.Close()
						if e != nil || ce != nil {
							t.Fatal(e, ce)
						}
					}
					side := main + suffix
					if unsafe {
						if e := os.Mkdir(side, 0700); e != nil {
							t.Fatal(e)
						}
					} else {
						writeFixture(t, cfg.DataDir, "miskoai.db"+suffix, "unexplained synthetic recovery bytes")
					}
					before, e := os.Lstat(side)
					if e != nil {
						t.Fatal(e)
					}
					if !unsafe {
						f, e := os.Open(side)
						if e != nil {
							t.Fatal(e)
						}
						before, e = f.Stat()
						ce := f.Close()
						if e != nil || ce != nil {
							t.Fatal(e, ce)
						}
					}
					c, e := New(cfg, Dependencies{})
					if e == nil {
						_ = c.Close()
						t.Error("unexplained recovery state admitted")
					}
					info, me := os.Lstat(main)
					if empty {
						if me != nil || info.Size() != 0 || !os.SameFile(info, originalMain) || info.Mode() != originalMain.Mode() || !info.ModTime().Equal(originalMain.ModTime()) {
							t.Error("empty main changed before refusal", me)
						}
					} else if !os.IsNotExist(me) {
						t.Error("missing main created before residual journal refusal", me)
					}
					after, se := os.Lstat(side)
					if se != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
						t.Error("residual journal changed", se)
					}
					if !unsafe {
						b, re := os.ReadFile(side)
						if re != nil || string(b) != "unexplained synthetic recovery bytes" {
							t.Error("residual bytes changed", re)
						}
					}
				})
			}
		}
	}
}
