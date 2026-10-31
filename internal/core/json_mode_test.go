package core

import (
	"context"
	"errors"
	"testing"

	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/provider"
)

type coreJSONModel struct {
	plain, json int
	err         error
	reply       func([]provider.Message) provider.Reply
}

func (m *coreJSONModel) Chat(_ context.Context, msg []provider.Message, _ int) (provider.Reply, error) {
	m.plain++
	return m.reply(msg), m.err
}
func (m *coreJSONModel) ChatJSON(_ context.Context, msg []provider.Message, _ int) (provider.Reply, error) {
	m.json++
	return m.reply(msg), m.err
}

func TestJSONModeCountedForwarding(t *testing.T) {
	for _, mode := range []string{"json", "error", "plain", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			c := &Controller{}
			m := &coreJSONModel{reply: func([]provider.Message) provider.Reply {
				return provider.Reply{Text: "{}", Usage: provider.Usage{TotalTokens: 7}, FinishReason: "stop"}
			}}
			var delegate agent.Model = m
			if mode == "error" {
				m.err = errors.New("synthetic")
			}
			if mode == "legacy" {
				delegate = modelFunc(m.Chat)
			}
			counted := &countedModel{c: c, delegate: delegate, operation: "chat"}
			if mode == "plain" {
				_, _ = counted.Chat(context.Background(), nil, 64)
			} else {
				j, ok := any(counted).(interface {
					ChatJSON(context.Context, []provider.Message, int) (provider.Reply, error)
				})
				if !ok {
					t.Fatal("counted wrapper lost JSON capability")
				}
				_, _ = j.ChatJSON(context.Background(), nil, 64)
			}
			wantJSON := 1
			if mode == "plain" || mode == "legacy" {
				wantJSON = 0
			}
			wantUsage := 7
			if mode == "error" {
				wantUsage = 0
			}
			if m.json != wantJSON || m.plain != 1-wantJSON || c.metrics.ChatAttempts != 1 || c.metrics.SuccessfulUsage.TotalTokens != wantUsage {
				t.Fatalf("JSON=%d plain=%d metrics=%+v", m.json, m.plain, c.metrics)
			}
		})
	}
}

func TestJSONModeCoreActualBorrowTwoACKs(t *testing.T) {
	c, g, gctx, _, asset := generationLibraryFixture(t)
	coreReplyProfile(t, c, asset)
	m := &coreJSONModel{reply: func(msg []provider.Message) provider.Reply {
		r := coreReplyModel(t, msg)
		r.Usage.TotalTokens = 7
		return r
	}}
	texts, images := 0, 0
	d := &coreReplyChannel{}
	d.text = func(context.Context, string, string, string, string) error { texts++; return nil }
	// Reuse the full synthetic channel and actual generation library borrow.
	d.image = func(context.Context, string, string, string, weixin.ImageSend) error { images++; return nil }
	ch := newObservedChannel(c, g, d)
	base, e := agent.New(c.store, &countedModel{c: c, delegate: m, operation: "chat"}, nil, ch, agent.Options{Account: c.scope.Account, User: c.scope.User})
	if e != nil {
		t.Fatal(e)
	}
	configured, e := c.bindGenerationReplyAgent(gctx, g, ch, base, coreReplyPrepared)
	if e != nil {
		t.Fatal(e)
	}
	in := coreReplyIncoming(c)
	r, e := configured.Handle(gctx, in)
	if e != nil || r.State != "sent" {
		t.Fatal(r, e)
	}
	receipt, e := c.store.ImageReplyReceipt(gctx, c.scope, in.ID)
	if e != nil || receipt.TextPhase != "acked" || receipt.ImagePhase != "acked" {
		t.Fatal(receipt, e)
	}
	_, e = configured.Handle(gctx, in)
	if e != nil || texts != 1 || images != 1 || m.json != 1 || m.plain != 0 || c.metrics.ChatAttempts != 1 || c.metrics.SuccessfulUsage.TotalTokens != 7 {
		t.Fatalf("texts=%d images=%d JSON=%d plain=%d metrics=%+v error=%v", texts, images, m.json, m.plain, c.metrics, e)
	}
}
