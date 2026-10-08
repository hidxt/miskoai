package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPollDurabilityBeforeDecodeAndRestart(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	id, err := s.RecordPoll(ctx, scope, "", []byte(`{broken`))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f, err := s.PendingPoll(ctx, scope)
	if err != nil || f == nil || f.ID != id || string(f.Body) != `{broken` {
		t.Fatalf("durable raw frame: %v %v", f, err)
	}
	if _, err = s.RecordPoll(ctx, Scope{"a", "other"}, "", []byte("x")); !errors.Is(err, ErrCapacity) {
		t.Fatalf("account pending exclusivity: %v", err)
	}
}

func TestResolveAtomicCursorAndInbox(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	id, err := s.RecordPoll(ctx, scope, "", []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	entry := InboxEntry{Scope: scope, MessageID: "one", Text: "hello", ContextToken: "token", ReceivedAt: time.Now()}
	if err = s.ResolvePoll(ctx, Scope{"a", "wrong"}, id, "next", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign resolution: %v", err)
	}
	bad := entry
	bad.Scope = Scope{"a", "other"}
	if err = s.ResolvePoll(ctx, scope, id, "next", []InboxEntry{bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign entry: %v", err)
	}
	if cur, err := s.Cursor(ctx, scope); err != nil || cur != "" {
		t.Fatalf("advanced on rejection: %q %v", cur, err)
	}
	if err = s.ResolvePoll(ctx, scope, id, "next", []InboxEntry{entry}); err != nil {
		t.Fatal(err)
	}
	if cur, err := s.Cursor(ctx, scope); err != nil || cur != "next" {
		t.Fatalf("cursor: %q %v", cur, err)
	}
	got, err := s.PendingInbox(ctx, scope, 32)
	if err != nil || len(got) != 1 || got[0].ContextToken != "token" {
		t.Fatalf("inbox: %v %v", got, err)
	}
	if err = s.CompleteInbox(ctx, Scope{"a", "wrong"}, got[0].Sequence); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign complete: %v", err)
	}
	if err = s.CompleteInbox(ctx, scope, got[0].Sequence); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateIDsAndCrossScope(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	for _, scope := range []Scope{{"a", "u"}, {"b", "u"}} {
		id, err := s.RecordPoll(ctx, scope, "", []byte("raw"))
		if err != nil {
			t.Fatal(err)
		}
		entry := InboxEntry{Scope: scope, MessageID: "same", Text: "x", ReceivedAt: time.Now()}
		if err = s.ResolvePoll(ctx, scope, id, "next", []InboxEntry{entry, entry}); err != nil {
			t.Fatal(err)
		}
		got, err := s.PendingInbox(ctx, scope, 32)
		if err != nil || len(got) != 1 {
			t.Fatalf("dedup: %v %v", got, err)
		}
		if _, err = s.ClaimMessage(ctx, scope, "claimed", "x"); err != nil {
			t.Fatal(err)
		}
		id, err = s.RecordPoll(ctx, scope, "next", []byte("raw"))
		if err != nil {
			t.Fatal(err)
		}
		entry.MessageID = "claimed"
		if err = s.ResolvePoll(ctx, scope, id, "last", []InboxEntry{entry}); err != nil {
			t.Fatal(err)
		}
		got, err = s.PendingInbox(ctx, scope, 32)
		if err != nil || len(got) != 1 {
			t.Fatalf("claim dedup: %v %v", got, err)
		}
	}
}

func TestInboxAndFrameCapacityRollBackCursor(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	if _, err := s.db.ExecContext(ctx, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1024) INSERT INTO inbox(account,user,message_id,text,context_token,received_at) SELECT 'b','u',CAST(x AS TEXT),'x','',1 FROM n`); err != nil {
		t.Fatal(err)
	}
	id, err := s.RecordPoll(ctx, scope, "", []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResolvePoll(ctx, scope, id, "next", []InboxEntry{{Scope: scope, MessageID: "x", Text: "x", ReceivedAt: time.Now()}}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity: %v", err)
	}
	if cur, _ := s.Cursor(ctx, scope); cur != "" {
		t.Fatal("capacity advanced cursor")
	}
	if f, _ := s.PendingPoll(ctx, scope); f == nil {
		t.Fatal("capacity removed frame")
	}
	for _, a := range []string{"b", "c", "d"} {
		if _, err = s.RecordPoll(ctx, Scope{a, "u"}, "", []byte("raw")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.RecordPoll(ctx, Scope{"e", "u"}, "", []byte("raw")); !errors.Is(err, ErrCapacity) {
		t.Fatalf("frame capacity: %v", err)
	}
}

func TestQuarantineRetainsEvidenceWithoutCursorAdvance(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	id, err := s.RecordPoll(ctx, scope, "", []byte("bad"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.QuarantinePoll(ctx, scope, id); err != nil {
		t.Fatal(err)
	}
	f, err := s.PendingPoll(ctx, scope)
	if err != nil || f == nil || f.State != "quarantined" || string(f.Body) != "bad" {
		t.Fatalf("quarantine: %v %v", f, err)
	}
	if _, err = s.RecordPoll(ctx, scope, "", []byte("again")); !errors.Is(err, ErrCapacity) {
		t.Fatalf("quarantine did not stop poll: %v", err)
	}
	if err = s.ResolvePoll(ctx, scope, id, "next", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolved quarantine: %v", err)
	}
	if cur, _ := s.Cursor(ctx, scope); cur != "" {
		t.Fatal("quarantine advanced cursor")
	}
}

func TestCompletedReferencesNeverAddressNewWork(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	first, err := s.RecordPoll(ctx, scope, "", []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	e := InboxEntry{Scope: scope, MessageID: "one", Text: "x", ReceivedAt: time.Now()}
	if err = s.ResolvePoll(ctx, scope, first, "1", []InboxEntry{e}); err != nil {
		t.Fatal(err)
	}
	got, err := s.PendingInbox(ctx, scope, 1)
	if err != nil || len(got) != 1 {
		t.Fatal(err)
	}
	old := got[0].Sequence
	if err = s.CompleteInbox(ctx, scope, old); err != nil {
		t.Fatal(err)
	}
	second, err := s.RecordPoll(ctx, scope, "1", []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("frame ID reused; stale resolver can consume new frame")
	}
	e.MessageID = "two"
	if err = s.ResolvePoll(ctx, scope, second, "2", []InboxEntry{e}); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteInbox(ctx, scope, old); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale inbox completion: %v", err)
	}
}

func TestInboxByteBudgetAndInputBounds(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	payload := strings.Repeat("x", 16384)
	if _, err := s.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<256) INSERT INTO inbox(account,user,message_id,text,context_token,received_at) SELECT 'b','u',CAST(x AS TEXT),?,?,1 FROM n`, payload, payload); err != nil {
		t.Fatal(err)
	}
	id, err := s.RecordPoll(ctx, scope, "", []byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	e := InboxEntry{Scope: scope, MessageID: "one", Text: "x", ReceivedAt: time.Now()}
	if err = s.ResolvePoll(ctx, scope, id, "next", []InboxEntry{e}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("inbox byte cap: %v", err)
	}
	if cur, _ := s.Cursor(ctx, scope); cur != "" {
		t.Fatal("byte cap advanced cursor")
	}
	if f, _ := s.PendingPoll(ctx, scope); f == nil {
		t.Fatal("byte cap lost frame")
	}
	for _, bad := range []string{strings.Repeat("x", 16385), "nul\x00", string([]byte{0xff})} {
		if err = s.ResolvePoll(ctx, scope, id, bad, nil); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid cursor accepted: %v", err)
		}
		entry := e
		entry.ContextToken = bad
		if err = s.ResolvePoll(ctx, scope, id, "next", []InboxEntry{entry}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid context accepted: %v", err)
		}
	}
	for _, limit := range []int{0, 33} {
		if _, err = s.PendingInbox(ctx, scope, limit); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid inbox limit: %v", err)
		}
	}
	if _, err = s.RecordPoll(ctx, Scope{"b", "u"}, "", make([]byte, (2<<20)+1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("raw body cap: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.PendingPoll(cancelled, scope); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read: %v", err)
	}
}

func TestPendingReceiveRejectsUnsafeRowsBeforeLoading(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	if _, err := s.db.Exec(`INSERT INTO poll_frames(account,user,cursor,body,state) VALUES('a','u','',?,'pending')`, make([]byte, (2<<20)+1)); err != nil {
		t.Fatal(err)
	}
	if f, err := s.PendingPoll(ctx, scope); !errors.Is(err, ErrInvalid) || f != nil {
		t.Fatalf("unsafe frame loaded: frame present=%v error=%v", f != nil, err)
	}
	if _, err := s.db.Exec(`DELETE FROM poll_frames; INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','x',?,'',1)`, strings.Repeat("x", 16385)); err != nil {
		t.Fatal(err)
	}
	if entries, err := s.PendingInbox(ctx, scope, 1); !errors.Is(err, ErrInvalid) || entries != nil {
		t.Fatalf("unsafe inbox loaded: entries=%d error=%v", len(entries), err)
	}
}
