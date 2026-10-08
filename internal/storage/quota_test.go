package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGlobalFactBytesAndReplyReservations(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	content := strings.Repeat("x", 16384)
	for _, a := range []string{"a", "b"} {
		for i := 0; i < 256; i++ {
			if _, err := s.AddFact(ctx, Scope{a, "u"}, Fact{Content: content, Source: "manual"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := s.AddFact(ctx, Scope{"c", "u"}, Fact{Content: "x", Source: "manual"}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("global facts: %v", err)
	}
	if err := s.ClearFacts(ctx, Scope{"a", "u"}); err != nil {
		t.Fatal(err)
	}
	id, err := s.AddFact(ctx, Scope{"c", "u"}, Fact{Content: "x", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 255; i++ {
		if _, err = s.AddFact(ctx, Scope{"a", "u"}, Fact{Content: content, Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.AddFact(ctx, Scope{"a", "u"}, Fact{Content: content[:16383], Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateFact(ctx, Scope{"c", "u"}, Fact{ID: id, Content: "xx", Source: "manual"}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("global update: %v", err)
	}
	// Fill 64MiB minus one full claim by using already delivered synthetic rows.
	if _, err = s.db.ExecContext(ctx, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2047) INSERT INTO messages(account,user,id,content,reply,state,created_at,updated_at) SELECT 'a','u',CAST(x AS TEXT),?,?,'sent',1,1 FROM n`, content, content); err != nil {
		t.Fatal(err)
	}
	scope := Scope{"a", "u"}
	if ok, err := s.ClaimMessage(ctx, scope, "reserved", content); err != nil || !ok {
		t.Fatalf("reserve: %v %v", ok, err)
	}
	if _, err = s.ClaimMessage(ctx, scope, "overflow", "x"); !errors.Is(err, ErrCapacity) {
		t.Fatalf("reservation missing: %v", err)
	}
	if err = s.SetMessageState(ctx, scope, "reserved", "sending"); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteMessage(ctx, scope, "reserved", content); err != nil {
		t.Fatalf("reserved reply must fit: %v", err)
	}
}

func TestReplyReservationReleasedOnlyAfterTerminalState(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	payload := strings.Repeat("x", 16384)
	if _, err := s.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2047) INSERT INTO messages(account,user,id,content,reply,state,created_at,updated_at) SELECT 'a','u',CAST(x AS TEXT),?,?,'sent',1,1 FROM n`, payload, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimMessage(ctx, scope, "one", payload); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMessageState(ctx, scope, "one", "sending"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMessageState(ctx, scope, "one", "ambiguous"); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimMessage(ctx, scope, "one", "dup"); err != nil || ok {
		t.Fatalf("full ambiguous duplicate: %v %v", ok, err)
	}
	if _, err := s.ClaimMessage(ctx, scope, "two", "x"); !errors.Is(err, ErrCapacity) {
		t.Fatalf("ambiguous reservation released: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM messages WHERE id='1' AND account='a' AND user='u'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimMessage(ctx, scope, "failed", payload); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMessageState(ctx, scope, "failed", "failed"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimMessage(ctx, scope, "released", ""); err != nil {
		t.Fatalf("failed reservation retained: %v", err)
	}
}

func TestGlobalFactCountAcrossScopes(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	if _, err := s.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<10000) INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) SELECT CAST(x AS TEXT),'u','x','','manual',0,0,1,1 FROM n`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFact(ctx, Scope{"new", "u"}, Fact{Content: "x", Source: "manual"}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("global count bypassed across scope: %v", err)
	}
}
