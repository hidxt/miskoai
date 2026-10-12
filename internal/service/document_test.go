package service

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/storage"
)

func serviceDocumentFrame() []byte {
	items := `{"type":4,"file_item":{"media":{"aes_key":"AAAAAAAAAAAAAAAAAAAAAA==","encrypt_query_param":"synthetic-query"},"file_name":"fixture.txt","len":"27","md5":"00112233445566778899aabbccddeeff"}}`
	return []byte(strings.ReplaceAll(strings.ReplaceAll(string(attachmentFrame(items)), `"bot"`, `"account"`), `"alice"`, `"user"`))
}

func TestServiceDocumentEmptyCaptionDurableDelivery(t *testing.T) {
	s, _ := storeFor(t)
	var polls, handlers atomic.Int32
	v := mustService(t, s, pollFunc(func(ctx context.Context, _ string) ([]byte, error) {
		if polls.Add(1) == 1 {
			return serviceDocumentFrame(), nil
		}
		return idlePoll(ctx, "")
	}), handleFunc(func(ctx context.Context, in agent.Incoming) (agent.Result, error) {
		handlers.Add(1)
		want := storage.Attachment{Kind: "file", MediaKey: "AAAAAAAAAAAAAAAAAAAAAA==", Query: "synthetic-query", FileName: "fixture.txt", Length: "27", MD5: "00112233445566778899aabbccddeeff"}
		if in.Attachment == nil || *in.Attachment != want || in.Text != "" || in.ID != "18446744073709551615" || in.Account != "account" || in.User != "user" {
			t.Error("file descriptor or genuine empty caption/identity lost")
		}
		cursor, e := s.Cursor(ctx, scope)
		if e != nil || cursor != "cursor" {
			t.Error("handler entered before durable cursor", cursor, e)
		}
		pending, e := s.PendingInbox(ctx, scope, 1)
		if e != nil || len(pending) != 1 || pending[0].Attachment == nil || *pending[0].Attachment != want {
			t.Error("file metadata not durable before handler", pending, e)
		}
		return agent.Result{State: "sent"}, nil
	}))
	cancel, done := start(t, v)
	eventually(t, func() bool { return v.Snapshot().Processed == 1 })
	cancel()
	joinAttachmentService(t, done)
	pending, e := s.PendingInbox(context.Background(), scope, 1)
	if e != nil || len(pending) != 0 || handlers.Load() != 1 {
		t.Fatal("file delivered more than once or not completed", pending, e, handlers.Load())
	}
}

func TestServiceDocumentAuthorizationBeforeKeyAndInvalidKeyQuarantine(t *testing.T) {
	for _, kind := range []string{"account", "user", "invalid-key"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := storeFor(t)
			var handlers, polls atomic.Int32
			raw := string(serviceDocumentFrame())
			if kind == "invalid-key" {
				raw = strings.Replace(raw, "AAAAAAAAAAAAAAAAAAAAAA==", "invalid", 1)
			} else {
				raw = strings.Replace(raw, `"`+kind+`"`, `"foreign"`, 1)
				raw = strings.Replace(raw, "AAAAAAAAAAAAAAAAAAAAAA==", "invalid", 1)
			}
			v := mustService(t, s, pollFunc(func(ctx context.Context, _ string) ([]byte, error) {
				if polls.Add(1) == 1 {
					return []byte(raw), nil
				}
				return idlePoll(ctx, "")
			}), handleFunc(func(context.Context, agent.Incoming) (agent.Result, error) {
				handlers.Add(1)
				return agent.Result{State: "sent"}, nil
			}))
			cancel, done := start(t, v)
			if kind == "invalid-key" {
				eventually(t, func() bool { return v.Snapshot().Channel == "quarantined" })
			} else {
				eventually(t, func() bool { cursor, e := s.Cursor(context.Background(), scope); return e == nil && cursor == "cursor" })
			}
			cancel()
			joinAttachmentService(t, done)
			cursor, e := s.Cursor(context.Background(), scope)
			if e != nil || handlers.Load() != 0 {
				t.Fatal("unauthorized/invalid document reached handler", e, handlers.Load())
			}
			if kind == "invalid-key" && cursor != "" {
				t.Fatal("invalid file advanced cursor")
			}
		})
	}
}
