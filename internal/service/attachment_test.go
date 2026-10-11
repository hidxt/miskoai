package service

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/storage"
)

func serviceImageFrame() []byte {
	return []byte(strings.ReplaceAll(strings.ReplaceAll(string(attachmentFrame(attachmentImage())), `"bot"`, `"account"`), `"alice"`, `"user"`))
}
func TestServiceAttachmentPollDurableDelivery(t *testing.T) {
	s, _ := storeFor(t)
	var calls atomic.Int32
	v := mustService(t, s, pollFunc(func(c context.Context, _ string) ([]byte, error) {
		if calls.Add(1) == 1 {
			return serviceImageFrame(), nil
		}
		return idlePoll(c, "")
	}), handleFunc(func(ctx context.Context, in agent.Incoming) (agent.Result, error) {
		if in.Attachment == nil || in.Attachment.Kind != "image" || in.Attachment.ImageHex != attachmentHex || in.Text != "" || in.ID != "18446744073709551615" {
			t.Error("durable attachment/provenance lost")
		}
		cursor, e := s.Cursor(ctx, scope)
		if e != nil || cursor != "cursor" {
			t.Error("handler effect before cursor commit", cursor, e)
		}
		pending, e := s.PendingInbox(ctx, scope, 1)
		if e != nil || len(pending) != 1 || pending[0].Attachment == nil {
			t.Error("descriptor not persisted before handler", e)
		}
		return agent.Result{State: "sent"}, nil
	}))
	cancel, done := start(t, v)
	eventually(t, func() bool { return v.Snapshot().Processed == 1 })
	cancel()
	joinAttachmentService(t, done)
	pending, e := s.PendingInbox(context.Background(), scope, 1)
	if e != nil || len(pending) != 0 {
		t.Fatal("inbox attachment completion failed", pending, e)
	}
}
func TestServiceMalformedAttachmentRetainsCursor(t *testing.T) {
	s, _ := storeFor(t)
	var effects atomic.Int32
	raw := []byte(strings.Replace(string(serviceImageFrame()), `"item_list":[`, `"item_list":[],"item_list":[`, 1))
	v := mustService(t, s, pollFunc(func(context.Context, string) ([]byte, error) { return raw, nil }), handleFunc(func(context.Context, agent.Incoming) (agent.Result, error) {
		effects.Add(1)
		return agent.Result{State: "sent"}, nil
	}))
	cancel, done := start(t, v)
	eventually(t, func() bool { return v.Snapshot().Channel == "quarantined" })
	cancel()
	joinAttachmentService(t, done)
	cursor, e := s.Cursor(context.Background(), scope)
	if e != nil || cursor != "" || effects.Load() != 0 {
		t.Fatal("malformed media advanced cursor/effects", cursor, e, effects.Load())
	}
}
func TestServiceRecoveredAttachmentDescriptor(t *testing.T) {
	s, _ := storeFor(t)
	ctx := context.Background()
	id, e := s.RecordPoll(ctx, scope, "", []byte(`{"synthetic":true}`))
	if e != nil {
		t.Fatal(e)
	}
	a := storage.Attachment{Kind: "unsupported"}
	e = s.ResolvePoll(ctx, scope, id, "saved", []storage.InboxEntry{{Scope: scope, MessageID: "recovered", Text: "caption", ContextToken: "context", ReceivedAt: time.Now().UTC(), Attachment: &a}})
	if e != nil {
		t.Fatal(e)
	}
	v := mustService(t, s, pollFunc(idlePoll), handleFunc(func(_ context.Context, in agent.Incoming) (agent.Result, error) {
		if in.Attachment == nil || *in.Attachment != a || in.Text != "caption" {
			t.Error("recovered descriptor lost")
		}
		return agent.Result{State: "duplicate"}, nil
	}))
	cancel, done := start(t, v)
	eventually(t, func() bool { return v.Snapshot().Processed == 1 })
	cancel()
	joinAttachmentService(t, done)
}

func joinAttachmentService(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("attachment service did not join")
	}
}
