package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/storage"
)

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
	s, err := storage.Open(filepath.Join(t.TempDir(), "probe.db"))
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
	if err = weixinProbe(context.Background(), f, s, a, &out); err != nil || f.sends != 1 {
		t.Fatal("authorized probe failed", err)
	}
	if err = weixinProbe(context.Background(), f, s, a, &out); err == nil || f.sends != 1 {
		t.Fatal("replayed message sent again", err)
	}
}
