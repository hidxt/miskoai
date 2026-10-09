package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

func sentFixture(t *testing.T, s *Store, sc Scope, id, text, reply string) {
	t.Helper()
	ctx := context.Background()
	if ok, e := s.ClaimMessage(ctx, sc, id, text); e != nil || !ok {
		t.Fatalf("claim %v %v", ok, e)
	}
	if e := s.SetMessageState(ctx, sc, id, "sending"); e != nil {
		t.Fatal(e)
	}
	if e := s.CompleteMessage(ctx, sc, id, reply); e != nil {
		t.Fatal(e)
	}
}

func TestCandidateQuotasAndConfirmationRollback(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	sentFixture(t, s, sc, "source", strings.Repeat("q", 1024), "reply")
	batch := make([]Candidate, 9)
	for i := range batch {
		batch[i] = Candidate{Content: fmt.Sprint(i), MessageID: "source", Quote: "q"}
	}
	if e := s.SaveDerived(ctx, sc, 0, Summary{Watermark: 1}, batch); !errors.Is(e, ErrInvalid) {
		t.Fatalf("batch bound %v", e)
	}
	var rev int64
	for i := 0; i < 16; i++ {
		for j := 0; j < 8; j++ {
			batch[j].Content = fmt.Sprintf("item-%d", i*8+j)
		}
		if e := s.SaveDerived(ctx, sc, rev, Summary{Text: "saved", Watermark: 1}, batch[:8]); e != nil {
			t.Fatal(e)
		}
		rev++
	}
	if e := s.SaveDerived(ctx, sc, rev, Summary{Text: "must rollback", Watermark: 1}, []Candidate{{Content: "129th", MessageID: "source", Quote: "q"}}); !errors.Is(e, ErrCapacity) {
		t.Fatalf("scope candidate quota %v", e)
	}
	d, e := s.Derived(ctx, sc)
	if e != nil || d.Text != "saved" || d.Revision != rev {
		t.Fatalf("partial derived %v %v", d, e)
	}
	cs, e := s.Candidates(ctx, sc, 128)
	if e != nil || len(cs) != 128 {
		t.Fatalf("quota %v %v", cs, e)
	}
	if _, e = s.db.Exec(`WITH RECURSIVE n(x) AS(VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<256) INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) SELECT 'a','u',?,'','manual',0,0,1,1 FROM n`, strings.Repeat("x", 16384)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ConfirmCandidate(ctx, sc, cs[0].ID); !errors.Is(e, ErrCapacity) {
		t.Fatalf("fact quota %v", e)
	}
	after, e := s.Candidates(ctx, sc, 128)
	if e != nil || len(after) != 128 || after[0].ID != cs[0].ID {
		t.Fatalf("candidate lost %v %v", after, e)
	}
	var facts int
	if e = s.db.QueryRow("SELECT count(*) FROM facts").Scan(&facts); e != nil || facts != 256 {
		t.Fatalf("partial fact %d %v", facts, e)
	}
}

func TestCandidateGlobalPayloadQuotaRollsBackSummary(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	other := Scope{"a", "other"}
	quote := strings.Repeat("q", 1024)
	sentFixture(t, s, sc, "source", quote, "reply")
	sentFixture(t, s, other, "source", quote, "reply")
	for batch := 0; batch < 16; batch++ {
		cs := []Candidate{}
		for i := 0; i < 8; i++ {
			content := fmt.Sprintf("%03d", batch*8+i) + strings.Repeat("x", 1021)
			cs = append(cs, Candidate{Content: content, MessageID: "source", Quote: quote})
		}
		if e := s.SaveDerived(ctx, sc, int64(batch), Summary{Watermark: 1}, cs); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.SaveDerived(ctx, other, 0, Summary{Text: "rollback", Watermark: 1}, []Candidate{{Content: "x", MessageID: "source", Quote: "q"}}); !errors.Is(e, ErrCapacity) {
		t.Fatalf("global bytes %v", e)
	}
	d, e := s.Derived(ctx, other)
	if e != nil || d.Revision != 0 || d.Text != "" {
		t.Fatalf("partial summary %v %v", d, e)
	}
	cs, e := s.Candidates(ctx, other, 8)
	if e != nil || len(cs) != 0 {
		t.Fatalf("partial candidates %v %v", cs, e)
	}
}

func TestCandidateIDsNeverReuseAfterDeleteAndClear(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	sentFixture(t, s, sc, "source", "user quote", "reply")
	c := Candidate{Content: "statement", MessageID: "source", Quote: "quote"}
	var prior int64
	for revision := int64(0); revision < 3; revision++ {
		if e := s.SaveDerived(ctx, sc, revision*2, Summary{Watermark: 1}, []Candidate{c}); e != nil {
			t.Fatal(e)
		}
		cs, e := s.Candidates(ctx, sc, 8)
		if e != nil || len(cs) != 1 || cs[0].ID <= prior {
			t.Fatalf("reused %v prior%d %v", cs, prior, e)
		}
		if prior > 0 {
			if _, e = s.ConfirmCandidate(ctx, sc, prior); !errors.Is(e, ErrNotFound) {
				t.Fatalf("stale ID targets new guess %v", e)
			}
		}
		prior = cs[0].ID
		if revision == 0 {
			if e = s.DeleteCandidate(ctx, sc, prior); e != nil {
				t.Fatal(e)
			}
			if e = s.ClearDerived(ctx, sc); e != nil {
				t.Fatal(e)
			}
		} else {
			if e = s.ClearMemory(ctx, sc); e != nil {
				t.Fatal(e)
			}
			if revision < 2 {
				sentFixture(t, s, sc, fmt.Sprint(revision), "user quote", "reply")
				c.MessageID = fmt.Sprint(revision)
			}
		}
	}
}

func TestClearMemoryKeepsAllClaimsAndReceiveEvidence(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	other := Scope{"a", "other"}
	for _, state := range []string{"processing", "sending", "sent", "failed", "ambiguous"} {
		if _, e := s.db.Exec(`INSERT INTO messages VALUES('a','u',?,'private question','private reply',?,1,1)`, state, state); e != nil {
			t.Fatal(e)
		}
	}
	sentFixture(t, s, other, "foreign", "other user", "other reply")
	if _, e := s.RecordPoll(ctx, sc, "", []byte("synthetic quarantine")); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec(`INSERT INTO channel_cursors VALUES('separate','u','cursor'); INSERT INTO inbox(account,user,message_id,text,context_token,received_at) VALUES('a','u','inbox','raw inbox','token',1)`); e != nil {
		t.Fatal(e)
	}
	if e := s.ClearMemory(ctx, sc); e != nil {
		t.Fatal(e)
	}
	for _, state := range []string{"processing", "sending", "sent", "failed", "ambiguous"} {
		if ok, e := s.ClaimMessage(ctx, sc, state, "replay"); e != nil || ok {
			t.Fatalf("lost %s %v %v", state, ok, e)
		}
		var content, reply, gotState string
		if e := s.db.QueryRow("SELECT content,reply,state FROM messages WHERE account='a' AND user='u' AND id=?", state).Scan(&content, &reply, &gotState); e != nil || content != "" || reply != "" || gotState != state {
			t.Fatalf("erasure changed state %s %q %q %s %v", state, content, reply, gotState, e)
		}
	}
	for _, table := range []string{"poll_frames", "channel_cursors", "inbox"} {
		var n int
		if e := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil || n != 1 {
			t.Fatalf("receive erased %s %d %v", table, n, e)
		}
	}
	h, e := s.History(ctx, other, 1)
	if e != nil || len(h) != 2 || h[0].Content != "other user" {
		t.Fatalf("foreign clear %v %v", h, e)
	}
}

func TestDerivedCASProvenanceConfirmAndClear(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	other := Scope{"a", "other"}
	sentFixture(t, s, sc, "m1", "I like tea", "assistant says coffee")
	c := Candidate{Content: "likes tea", MessageID: "m1", Quote: "like tea"}
	if e := s.SaveDerived(ctx, sc, 0, Summary{Text: "tea summary", Watermark: 1}, []Candidate{c}); e != nil {
		t.Fatal(e)
	}
	d, n, h, e := s.DerivedHistory(ctx, sc, 16)
	if e != nil || d.Text != "tea summary" || d.Revision != 1 || n != 1 || len(h) != 2 {
		t.Fatalf("snapshot: %v %d %v %v", d, n, h, e)
	}
	if e = s.SaveDerived(ctx, sc, 0, Summary{Text: "stale", Watermark: 1}, nil); !errors.Is(e, ErrStale) {
		t.Fatalf("CAS: %v", e)
	}
	for _, bad := range []Candidate{{Content: "coffee", MessageID: "m1", Quote: "assistant says coffee"}, {Content: "tea", MessageID: "foreign", Quote: "tea"}} {
		if e = s.SaveDerived(ctx, sc, 1, Summary{Text: "bad", Watermark: 1}, []Candidate{bad}); !errors.Is(e, ErrInvalid) {
			t.Fatalf("source: %v", e)
		}
	}
	cs, e := s.Candidates(ctx, sc, 8)
	if e != nil || len(cs) != 1 {
		t.Fatalf("candidates: %v %v", cs, e)
	}
	id := cs[0].ID
	if _, e = s.ConfirmCandidate(ctx, other, id); !errors.Is(e, ErrNotFound) {
		t.Fatalf("isolation: %v", e)
	}
	if _, e = s.ConfirmCandidate(ctx, sc, id); e != nil {
		t.Fatal(e)
	}
	fs, e := s.SearchFacts(ctx, sc, "tea", 8)
	if e != nil || len(fs) != 1 || fs[0].Source != "explicit_user" || fs[0].Confidence != 1 || fs[0].Importance != 50 {
		t.Fatalf("confirmed: %v %v", fs, e)
	}
	if e = s.SaveDerived(ctx, sc, 1, Summary{Text: "again", Watermark: 1}, []Candidate{c, c}); e != nil {
		t.Fatal(e)
	}
	cs, e = s.Candidates(ctx, sc, 8)
	if e != nil || len(cs) != 1 || cs[0].ID <= id {
		t.Fatalf("ID reuse/dedup: %v %v", cs, e)
	}
	if e = s.ClearMemory(ctx, sc); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveDerived(ctx, sc, 2, Summary{Text: "old job", Watermark: 1}, nil); !errors.Is(e, ErrStale) {
		t.Fatalf("clear CAS: %v", e)
	}
	d, n, h, e = s.DerivedHistory(ctx, sc, 16)
	if e != nil || d.Text != "" || d.Revision != 3 || d.Watermark != 1 || n != 1 || len(h) != 0 {
		t.Fatalf("clear: %v %d %v %v", d, n, h, e)
	}
	if ok, e := s.ClaimMessage(ctx, sc, "m1", "replay"); e != nil || ok {
		t.Fatalf("claim lost %v %v", ok, e)
	}
	fs, e = s.ExportFacts(ctx, sc)
	if e != nil || len(fs) != 0 {
		t.Fatalf("fact clear %v %v", fs, e)
	}
	cs, e = s.Candidates(ctx, sc, 8)
	if e != nil || len(cs) != 0 {
		t.Fatalf("candidate clear %v %v", cs, e)
	}
}

func TestDerivedBoundsQuotaRollbackAndOverflow(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	sentFixture(t, s, sc, "m", "user evidence", "reply")
	for _, d := range []Summary{{Text: strings.Repeat("x", 8193), Watermark: 1}, {Text: "x", Watermark: 2}, {Text: "x", Watermark: -1}, {Text: "\x00", Watermark: 1}} {
		if e := s.SaveDerived(ctx, sc, 0, d, nil); !errors.Is(e, ErrInvalid) {
			t.Fatalf("invalid summary %v", e)
		}
	}
	for i := 0; i < 16; i++ {
		q := Scope{"quota", string(rune('a' + i))}
		if e := s.ClearDerived(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.SaveDerived(ctx, sc, 0, Summary{Text: "must rollback", Watermark: 1}, []Candidate{{Content: "evidence", MessageID: "m", Quote: "evidence"}}); !errors.Is(e, ErrCapacity) {
		t.Fatalf("quota %v", e)
	}
	cs, e := s.Candidates(ctx, sc, 8)
	if e != nil || len(cs) != 0 {
		t.Fatalf("partial candidates %v %v", cs, e)
	}
	q := Scope{"quota", "a"}
	if _, e = s.db.Exec("UPDATE derived SET revision=? WHERE account=? AND user=?", int64(math.MaxInt64), q.Account, q.User); e != nil {
		t.Fatal(e)
	}
	if e = s.ClearDerived(ctx, q); !errors.Is(e, ErrCapacity) {
		t.Fatalf("overflow %v", e)
	}
}
