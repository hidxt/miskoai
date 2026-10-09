package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
)

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
