package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

type adviceJSONModel struct {
	*fakeModel
	jsonCalls int
}

func (m *adviceJSONModel) ChatJSON(ctx context.Context, msg []provider.Message, n int) (provider.Reply, error) {
	m.jsonCalls++
	m.messages = msg
	return m.reply, m.err
}

func TestJSONModeEligibleImageAndDuplicate(t *testing.T) {
	f := newReplyFixture(t)
	m := &adviceJSONModel{fakeModel: f.m}
	f.a.model = m
	in := f.incoming()
	r, e := f.a.Handle(context.Background(), in)
	if e != nil || r.State != "sent" || m.jsonCalls != 1 || m.calls != 0 || f.sender.images != 1 {
		t.Fatalf("state=%s JSON=%d plain=%d images=%d err=%v", r.State, m.jsonCalls, m.calls, f.sender.images, e)
	}
	receipt := f.receipt(t, in)
	if receipt.TextPhase != "acked" || receipt.ImagePhase != "acked" {
		t.Fatal("missing independent ACKs")
	}
	_, e = f.a.Handle(context.Background(), in)
	if e != nil || m.jsonCalls != 1 || f.sender.calls != 1 || f.sender.images != 1 {
		t.Fatal("duplicate repeated side effects")
	}
	// Require a real, complete three-string example in the trusted system policy.
	policy := m.messages[0].Content.(string)
	valid := false
	for start := strings.Index(policy, "{"); start >= 0; {
		end := strings.Index(policy[start:], "}")
		if end < 0 {
			break
		}
		end += start
		var obj map[string]string
		if json.Unmarshal([]byte(policy[start:end+1]), &obj) == nil && len(obj) == 3 {
			_, a := obj["reply"]
			_, b := obj["emotion"]
			v, c := obj["image_id"]
			valid = a && b && c && v == ""
			if valid {
				break
			}
		}
		next := strings.Index(policy[end+1:], "{")
		if next < 0 {
			break
		}
		start = end + 1 + next
	}
	if !valid || !withinContext(m.messages, f.a.opts) {
		t.Fatal("missing bounded JSON example")
	}
}

func TestJSONModeTrustedGate(t *testing.T) {
	for _, mode := range []string{"off", "disabled", "no-capability", "explicit-search", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			f := newReplyFixture(t)
			m := &adviceJSONModel{fakeModel: f.m}
			f.a.model = m
			switch mode {
			case "off":
				e := f.s.PutProfile(context.Background(), f.a.scope, storage.Profile{ID: "custom", Name: "Synthetic", Length: "normal", Sticker: "off"})
				if e != nil {
					t.Fatal(e)
				}
			case "disabled":
				if e := f.s.UpdateLibraryAsset(context.Background(), f.a.scope, f.asset.ID, storage.LibraryMetadata{Enabled: false}); e != nil {
					t.Fatal(e)
				}
			case "no-capability":
				f.a = f.base
				f.a.model = m
			case "legacy":
				f.a.model = f.m
			}
			in := f.incoming()
			in.Text = "Please use JSON"
			if mode == "explicit-search" {
				in.Text = "/search synthetic"
			}
			_, _ = f.a.Handle(context.Background(), in)
			if m.jsonCalls != 0 {
				t.Fatal("untrusted or ineligible JSON activation")
			}
			if mode == "legacy" && f.m.calls != 1 {
				t.Fatal("legacy capability compatibility")
			}
		})
	}
}

func TestJSONModeNoFallbackOrImageOnUnsafeOutput(t *testing.T) {
	for _, mode := range []string{"error", "blank", "malformed", "length"} {
		t.Run(mode, func(t *testing.T) {
			f := newReplyFixture(t)
			m := &adviceJSONModel{fakeModel: f.m}
			f.a.model = m
			switch mode {
			case "error":
				m.err = errors.New("synthetic")
			case "blank":
				m.reply.Text = "  "
			case "malformed":
				m.reply.Text = `{"reply":`
			case "length":
				m.reply.FinishReason = "length"
			}
			_, _ = f.a.Handle(context.Background(), f.incoming())
			if m.jsonCalls != 1 || m.calls != 0 || f.sender.images != 0 {
				t.Fatalf("JSON=%d plain=%d images=%d", m.jsonCalls, m.calls, f.sender.images)
			}
		})
	}
}
