package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
)

// Removing the wrapper's admission checks must fail these tests even when auth
// would independently refuse the operation. A canceled context also makes the
// valid-auth RED cases safe without a real channel connection.
func TestLiveWeixinProbeAdmissionBeforeAuthorization(t *testing.T) {
	for _, gate := range []string{"owned", "paused", "malformed-pause", "unsafe-pause"} {
		for _, auth := range []string{"missing", "malformed", "valid"} {
			t.Run(gate+"/"+auth, func(t *testing.T) {
				dir := filepath.Join(t.TempDir(), "private")
				if err := privatefs.EnsureDir(dir); err != nil {
					t.Fatal(err)
				}
				write := func(name, body string) {
					t.Helper()
					f, err := privatefs.Create(filepath.Join(dir, name))
					if err != nil {
						t.Fatal(err)
					}
					_, we := f.WriteString(body)
					ce := f.Close()
					if we != nil || ce != nil {
						t.Fatal(we, ce)
					}
				}
				if auth != "missing" {
					body := `{"bot_token":"synthetic-probe-token","account":"fixture","allowed_user":"alice","base_url":"https://ilinkai.weixin.qq.com"}`
					if auth == "malformed" {
						body += " {}"
					}
					write("weixin-auth.json", body)
				}
				var want error
				var marker string
				var before os.FileInfo
				var contents []byte
				if gate == "owned" {
					lock, err := maintenance.Acquire(dir)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if err := lock.Close(); err != nil {
							t.Error(err)
						}
					})
					_, want = maintenance.Acquire(dir)
					if want == nil {
						t.Fatal("fixture failed to retain exclusive owner")
					}
					marker = filepath.Join(dir, ".miskoai.lock")
				} else {
					marker = filepath.Join(dir, ".miskoai-restore-pause.json")
					if gate == "unsafe-pause" {
						if err := os.Mkdir(marker, 0700); err != nil {
							t.Fatal(err)
						}
					} else {
						body := `{"version":1,"reason":"restore_requires_reconciliation"}`
						if gate == "malformed-pause" {
							body += " {}"
						}
						write(filepath.Base(marker), body)
					}
					paused, err := maintenance.RestorePaused(dir)
					if gate == "paused" {
						if err != nil || !paused {
							t.Fatal("fixture did not pause", err)
						}
					} else {
						want = err
						if want == nil {
							t.Fatal("fixture did not refuse unsafe pause")
						}
					}
				}
				var err error
				before, err = os.Lstat(marker)
				if err != nil {
					t.Fatal(err)
				}
				if before.Mode().IsRegular() {
					contents, err = os.ReadFile(marker)
					if err != nil {
						t.Fatal(err)
					}
				}
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				var out bytes.Buffer
				err = liveWeixinProbe(ctx, config.Config{DataDir: dir}, &out)
				if gate == "paused" {
					if err == nil || !strings.Contains(err.Error(), "reconcil") || errors.Is(err, config.ErrAuthorizationMissing) || errors.Is(err, config.ErrAuthorization) {
						t.Errorf("restore pause did not refuse before authorization: %v", err)
					}
				} else if !errors.Is(err, want) {
					t.Errorf("admission did not refuse before authorization: got %v; want %v", err, want)
				}
				if out.Len() != 0 {
					t.Error("refused probe produced public output")
				}
				for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
					if _, err := os.Lstat(filepath.Join(dir, "miskoai.db") + suffix); !os.IsNotExist(err) {
						t.Errorf("refused probe created database%s: %v", suffix, err)
					}
				}
				after, err := os.Lstat(marker)
				if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
					t.Fatal("refused probe changed admission marker", err)
				}
				if before.Mode().IsRegular() {
					got, err := os.ReadFile(marker)
					if err != nil || !bytes.Equal(got, contents) {
						t.Fatal("refused probe changed marker bytes", err)
					}
				}
				if gate != "owned" {
					lock, err := maintenance.Acquire(dir)
					if err != nil {
						t.Fatal("refused probe leaked lifecycle ownership", err)
					}
					if err := lock.Close(); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestProbeStrictPrivateAuthorization(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := privatefs.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	c := config.Config{DataDir: dir}
	var out bytes.Buffer
	if err := liveWeixinProbe(context.Background(), c, &out); !errors.Is(err, config.ErrAuthorizationMissing) {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "weixin-auth.json")
	f, err := privatefs.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(`{"bot_token":"synthetic-secret-canary","account":"fixture","allowed_user":"alice","base_url":"https://ilinkai.weixin.qq.com"} {}`); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = liveWeixinProbe(context.Background(), c, &out); !errors.Is(err, config.ErrAuthorization) {
		t.Fatal("malformed auth not refused before client/database", err)
	}
	if _, err = os.Lstat(filepath.Join(dir, "miskoai.db")); !os.IsNotExist(err) {
		t.Fatal("database opened before auth validation", err)
	}
	if out.Len() != 0 {
		t.Fatal("probe produced public output on refused authorization")
	}
}

type fakeChannel struct {
	updates weixin.Updates
	sends   int
}

func (f *fakeChannel) GetUpdates(context.Context, string) (weixin.Updates, error) {
	return f.updates, nil
}
func (f *fakeChannel) SendText(_ context.Context, to, token, id, text string) error {
	if to != "alice" || token != "synthetic-context" || id == "" || text == "" {
		panic("wrong authorized reply")
	}
	f.sends++
	return nil
}

func TestWeixinProbeAuthorizationAndReplay(t *testing.T) {
	s, err := storage.Open(filepath.Join(t.TempDir(), "private", "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f := &fakeChannel{updates: weixin.Updates{Messages: []weixin.Message{{MessageID: json.Number("42"), FromUserID: "mallory", MessageType: 1, ContextToken: "synthetic-context", Items: []weixin.Item{{Type: 1, Text: &weixin.TextItem{Text: "synthetic probe"}}}}}}}
	a := authorization{Account: "fixture", AllowedUser: "alice"}
	var out bytes.Buffer
	if err = weixinProbe(context.Background(), f, s, a, &out); err == nil || f.sends != 0 {
		t.Fatal("unauthorized send", err)
	}
	f.updates.Messages[0].FromUserID = "alice"
	if err = weixinProbe(context.Background(), f, s, a, &out); err == nil || f.sends != 0 {
		t.Fatal("non-synthetic input replied to", err)
	}
	f.updates.Messages[0].Items[0].Text.Text = "MiskoAI synthetic integration test"
	if err = weixinProbe(context.Background(), f, s, a, &out); err != nil || f.sends != 1 {
		t.Fatal("authorized probe failed", err)
	}
	if err = weixinProbe(context.Background(), f, s, a, &out); err == nil || f.sends != 1 {
		t.Fatal("replayed message sent again", err)
	}
}
