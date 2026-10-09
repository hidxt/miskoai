package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func managementFact(t *testing.T, s *Store, sc Scope, text string, importance int, expired bool) int64 {
	t.Helper()
	f := Fact{Content: text, Source: "manual", Importance: importance}
	if expired {
		v := time.Now().Add(-time.Hour)
		f.ExpiresAt = &v
	}
	id, e := s.AddFact(context.Background(), sc, f)
	if e != nil {
		t.Fatal(e)
	}
	return id
}

func TestManagementScopeIsolation(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	id := managementFact(t, s, sc, "mine expired", 100, true)
	managementFact(t, s, sc, "mine current", 50, false)
	for _, foreign := range []Scope{{"a", "v"}, {"b", "u"}} {
		managementFact(t, s, foreign, "foreign secret", 100, false)
		sentFixture(t, s, foreign, "foreign-id", "foreign secret", "foreign reply")
	}
	sentFixture(t, s, sc, "mine-id", "mine chat", "mine reply")
	p, e := s.FactsPage(ctx, sc, "", 0, 1)
	if e != nil || len(p.Items) != 1 || p.Items[0].ID != id || !p.HasMore || p.NextOffset != 1 {
		t.Fatalf("first page %+v %v", p, e)
	}
	p, e = s.FactsPage(ctx, sc, "", 1, 1)
	if e != nil || len(p.Items) != 1 || p.HasMore || p.NextOffset != 0 {
		t.Fatalf("last page %+v %v", p, e)
	}
	h, e := s.HistoryPage(ctx, sc, "", 0, 8)
	if e != nil || len(h.Items) != 1 || h.Items[0].ID != "mine-id" {
		t.Fatalf("history %+v %v", h, e)
	}
	p, e = s.FactsPage(ctx, sc, "foreign", 0, 16)
	if e != nil || len(p.Items) != 0 {
		t.Fatal("foreign fact disclosure", e)
	}
	h, e = s.HistoryPage(ctx, sc, "foreign", 0, 8)
	if e != nil || len(h.Items) != 0 {
		t.Fatal("foreign history disclosure", e)
	}
	if e = s.ClearMemory(ctx, sc); e != nil {
		t.Fatal(e)
	}
	h, e = s.HistoryPage(ctx, sc, "", 0, 8)
	if e != nil || len(h.Items) != 0 {
		t.Fatal("cleared history", e)
	}
	if ok, e := s.ClaimMessage(ctx, sc, "mine-id", "retry"); e != nil || ok {
		t.Fatal("clear lost claim", e)
	}
}

func TestManagementSearchEscapes(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	literal := `100%_\ " OR *`
	managementFact(t, s, sc, literal, 1, false)
	managementFact(t, s, sc, "ordinary", 0, false)
	sentFixture(t, s, sc, "match", literal, "reply literal")
	sentFixture(t, s, sc, "ordinary", "ordinary", "ordinary")
	for _, q := range []string{"%", "_", `\`, `"`, `*`, "  reply literal  "} {
		h, e := s.HistoryPage(ctx, sc, q, 0, 8)
		if e != nil || len(h.Items) != 1 || h.Items[0].ID != "match" {
			t.Fatalf("literal history query %q %+v %v", q, h, e)
		}
	}
	for _, q := range []string{"%", "_", `\`, `"`, `*`} {
		p, e := s.FactsPage(ctx, sc, q, 0, 16)
		if e != nil || len(p.Items) != 1 {
			t.Fatalf("fact literal %q %+v %v", q, p, e)
		}
	}
	for _, q := range []string{`" OR account:foreign NOT *`, "普通"} {
		got, e := s.SearchFacts(ctx, sc, q, 16)
		if e != nil {
			t.Fatal(e)
		}
		p, e := s.FactsPage(ctx, sc, q, 0, 16)
		if e != nil || !reflect.DeepEqual(got, p.Items) {
			t.Fatalf("search compatibility %q %v", q, e)
		}
	}
	for _, q := range []string{strings.Repeat("a", 1025), strings.Repeat("x ", 33), string([]byte{255}), "x\x00"} {
		if _, e := s.FactsPage(ctx, sc, q, 0, 1); !errors.Is(e, ErrInvalid) {
			t.Fatalf("fact query bound %v", e)
		}
		if _, e := s.HistoryPage(ctx, sc, q, 0, 1); !errors.Is(e, ErrInvalid) {
			t.Fatalf("history query bound %v", e)
		}
	}
	for _, v := range [][2]int{{-1, 1}, {10001, 1}, {0, 0}, {0, 17}} {
		if _, e := s.FactsPage(ctx, sc, "", v[0], v[1]); !errors.Is(e, ErrInvalid) {
			t.Fatalf("fact bounds %v", e)
		}
	}
	for _, v := range [][2]int{{-1, 1}, {10001, 1}, {0, 0}, {0, 9}} {
		if _, e := s.HistoryPage(ctx, sc, "", v[0], v[1]); !errors.Is(e, ErrInvalid) {
			t.Fatalf("history bounds %v", e)
		}
	}
}

type managementExport struct {
	Version int `json:"version"`
	Facts   []struct {
		ID         int64      `json:"id"`
		Content    string     `json:"content"`
		Category   string     `json:"category"`
		Source     string     `json:"source"`
		Confidence float64    `json:"confidence"`
		Importance int        `json:"importance"`
		CreatedAt  time.Time  `json:"created_at"`
		UpdatedAt  time.Time  `json:"updated_at"`
		ExpiresAt  *time.Time `json:"expires_at"`
	} `json:"facts"`
	Summary struct {
		Text      string    `json:"text"`
		Revision  int64     `json:"revision"`
		Watermark int64     `json:"watermark"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"summary"`
	Candidates []struct {
		ID        int64     `json:"id"`
		Content   string    `json:"content"`
		MessageID string    `json:"message_id"`
		Quote     string    `json:"quote"`
		CreatedAt time.Time `json:"created_at"`
	} `json:"candidates"`
}

type managementHookWriter struct {
	bytes.Buffer
	hook   func()
	called bool
}

func (w *managementHookWriter) Write(p []byte) (int, error) {
	if !w.called {
		w.called = true
		w.hook()
	}
	return w.Buffer.Write(p)
}

func TestMemoryExportCompleteAndConsistent(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	text := strings.Repeat("quoted\"\x01", 1800)
	// More than the chat response bound overall; each fact stays within its own cap.
	id := managementFact(t, s, sc, text, 100, true)
	managementFact(t, s, sc, text, 0, false)
	managementFact(t, s, Scope{"b", "u"}, "foreign secret", 0, false)
	sentFixture(t, s, sc, "proof", "a visible quote", "reply")
	if e := s.SaveDerived(ctx, sc, 0, Summary{Text: "captured summary", Watermark: 1}, []Candidate{{Content: "candidate", MessageID: "proof", Quote: "visible quote"}}); e != nil {
		t.Fatal(e)
	}
	expectedFacts, e := s.ExportFacts(ctx, sc)
	if e != nil {
		t.Fatal(e)
	}
	expectedSummary, e := s.Derived(ctx, sc)
	if e != nil {
		t.Fatal(e)
	}
	expectedCandidates, e := s.Candidates(ctx, sc, 128)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	started := make(chan struct{})
	w := &managementHookWriter{hook: func() { go func() { close(started); done <- s.ClearMemory(ctx, sc) }(); <-started }}
	if e := s.WriteMemoryJSON(ctx, sc, w); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	var got managementExport
	if e := json.Unmarshal(w.Bytes(), &got); e != nil {
		t.Fatal(e)
	}
	if got.Version != 1 || len(got.Facts) != 2 || got.Facts[0].ID != id || got.Facts[0].Content != text || got.Facts[0].ExpiresAt == nil || got.Summary.Text != "captured summary" || got.Summary.Revision != 1 || len(got.Candidates) != 1 || got.Candidates[0].Content != "candidate" {
		t.Fatalf("incomplete captured export %+v", got)
	}
	decodedFacts := make([]Fact, 0, len(got.Facts))
	for _, f := range got.Facts {
		decodedFacts = append(decodedFacts, Fact{ID: f.ID, Content: f.Content, Category: f.Category, Source: f.Source, Confidence: f.Confidence, Importance: f.Importance, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt, ExpiresAt: f.ExpiresAt})
	}
	decodedSummary := Summary{Text: got.Summary.Text, Watermark: got.Summary.Watermark, Revision: got.Summary.Revision, UpdatedAt: got.Summary.UpdatedAt}
	decodedCandidates := make([]Candidate, 0, len(got.Candidates))
	for _, c := range got.Candidates {
		decodedCandidates = append(decodedCandidates, Candidate{ID: c.ID, Content: c.Content, MessageID: c.MessageID, Quote: c.Quote, CreatedAt: c.CreatedAt})
	}
	if !reflect.DeepEqual(expectedFacts, decodedFacts) || !reflect.DeepEqual(expectedSummary, decodedSummary) || !reflect.DeepEqual(expectedCandidates, decodedCandidates) {
		t.Fatal("export omitted or changed stored fields")
	}
	if bytes.Contains(w.Bytes(), []byte("foreign secret")) || bytes.Contains(w.Bytes(), []byte(`"history"`)) {
		t.Fatal("export disclosed excluded data")
	}
	var empty bytes.Buffer
	if e := s.WriteMemoryJSON(ctx, sc, &empty); e != nil {
		t.Fatal(e)
	}
	if e := json.Unmarshal(empty.Bytes(), &got); e != nil || len(got.Facts) != 0 || len(got.Candidates) != 0 || got.Summary.Text != "" {
		t.Fatal("post clear export", e)
	}
}

type managementFaultWriter struct {
	cancel context.CancelFunc
	calls  int
	mode   string
}

func (w *managementFaultWriter) Write(p []byte) (int, error) {
	w.calls++
	switch w.mode {
	case "error":
		return 0, errors.New("synthetic private writer secret")
	case "short":
		return len(p) - 1, nil
	case "cancel":
		w.cancel()
	}
	return len(p), nil
}
func TestMemoryExportWriterFailureAndCancel(t *testing.T) {
	s, _ := testStore(t)
	sc := Scope{"a", "u"}
	managementFact(t, s, sc, "fact", 1, false)
	for _, mode := range []string{"error", "short", "cancel"} {
		ctx, cancel := context.WithCancel(context.Background())
		w := &managementFaultWriter{mode: mode, cancel: cancel}
		e := s.WriteMemoryJSON(ctx, sc, w)
		cancel()
		expected := ErrStorage
		if mode == "cancel" {
			expected = context.Canceled
		}
		if !errors.Is(e, expected) || strings.Contains(e.Error(), "secret") || w.calls != 1 {
			t.Fatalf("writer %s %v calls %d", mode, e, w.calls)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &managementFaultWriter{}
	if e := s.WriteMemoryJSON(ctx, sc, w); !errors.Is(e, context.Canceled) || w.calls != 0 {
		t.Fatalf("pre cancel %v calls %d", e, w.calls)
	}
	if e := s.WriteMemoryJSON(context.Background(), sc, nil); !errors.Is(e, ErrInvalid) {
		t.Fatalf("nil writer %v", e)
	}
}

func managementBytes(t *testing.T, f func()) uint64 {
	t.Helper()
	f()
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for i := 0; i < 4; i++ {
		f()
	}
	runtime.ReadMemStats(&b)
	return (b.TotalAlloc - a.TotalAlloc) / 4
}
func managementBulk(t *testing.T, s *Store, sc Scope, n int, content, category string, messages bool) {
	t.Helper()
	tx, e := s.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	for i := 0; i < n; i++ {
		if messages {
			_, e = tx.Exec(`INSERT INTO messages(account,user,id,content,reply,state,created_at,updated_at) VALUES(?,?,?,?,?,'sent',1,1)`, sc.Account, sc.User, fmt.Sprint(i), content, content)
		} else {
			_, e = tx.Exec(`INSERT INTO facts(account,user,content,category,source,confidence,importance,created_at,updated_at) VALUES(?,?,?,?,'manual',1,1,1,1)`, sc.Account, sc.User, content, category)
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
}
func TestManagementPageMaterializationBound(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	managementBulk(t, s, sc, 1, strings.Repeat("x", 16000), "", false)
	managementBulk(t, s, sc, 1, strings.Repeat("x", 16000), "", true)
	f := func() {
		p, e := s.FactsPage(ctx, sc, "", 0, 1)
		if e != nil || len(p.Items) != 1 {
			t.Fatal(e)
		}
	}
	h := func() {
		p, e := s.HistoryPage(ctx, sc, "", 0, 1)
		if e != nil || len(p.Items) != 1 {
			t.Fatal(e)
		}
	}
	smallF, smallH := managementBytes(t, f), managementBytes(t, h)
	// Large retention is inserted directly into the synthetic fixture in one transaction.
	if _, e := s.db.Exec("DELETE FROM facts; DELETE FROM messages"); e != nil {
		t.Fatal(e)
	}
	managementBulk(t, s, sc, 250, "x", "", false)
	managementBulk(t, s, sc, 1000, "x", "", true)
	compactF, compactH := managementBytes(t, f), managementBytes(t, h)
	if _, e := s.db.Exec("UPDATE facts SET content=?", strings.Repeat("x", 16000)); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec("UPDATE messages SET content=?,reply=?", strings.Repeat("x", 16000), strings.Repeat("x", 16000)); e != nil {
		t.Fatal(e)
	}
	largeF, largeH := managementBytes(t, f), managementBytes(t, h)
	t.Logf("bytes/op facts small=%d compact=%d large=%d history small=%d compact=%d large=%d", smallF, compactF, largeF, smallH, compactH, largeH)
	// Mandatory SQL admission visits retained pages; driver allocations grow
	// with this scan. Total allocation is not constant. These measured ceilings
	// are well below the 4MB facts and 32MB messages that loading all returned
	// records would necessarily allocate, and permit limit+1 payload decoding.
	if largeF > 1<<20 || largeH > 3<<20 {
		t.Fatal("page allocated retained payloads")
	}
	p, e := s.HistoryPage(ctx, sc, "", 0, 8)
	if e != nil || len(p.Items) != 8 || !p.HasMore || p.NextOffset != 8 {
		t.Fatalf("history lookahead %+v %v", p, e)
	}
}

type managementCountingWriter struct {
	bytes, max int
	tail       []byte
	rows       int
	invalid    bool
}

func (w *managementCountingWriter) Write(p []byte) (int, error) {
	w.bytes += len(p)
	if len(p) > w.max {
		w.max = len(p)
	}
	if len(p) <= 32 {
		w.tail = append(w.tail[:0], p...)
	}
	if bytes.HasPrefix(p, []byte(`{"id":`)) {
		// Validate each full synthetic fact without retaining the artifact.
		var row struct {
			ID       int64  `json:"id"`
			Content  string `json:"content"`
			Category string `json:"category"`
		}
		if json.Unmarshal(p, &row) != nil || row.ID != int64(w.rows+1) || row.Content != "x"+strings.Repeat("\x01", 418) || row.Category != strings.Repeat("\x02", 256) {
			w.invalid = true
		}
		w.rows++
	} else if !bytes.Equal(p, []byte(`{"version":1,"facts":[`)) && !bytes.Equal(p, []byte(",")) && !bytes.Equal(p, []byte(`],"summary":`)) && !bytes.Equal(p, []byte(`,"candidates":[`)) && !bytes.Equal(p, []byte(`]}`)) && !json.Valid(p) {
		w.invalid = true
	}
	return len(p), nil
}
func TestMemoryExportControlExpansion(t *testing.T) {
	s, _ := testStore(t)
	sc := Scope{"a", "u"}
	managementBulk(t, s, sc, 10000, "x"+strings.Repeat("\x01", 418), strings.Repeat("\x02", 256), false)
	w := &managementCountingWriter{}
	if e := s.WriteMemoryJSON(context.Background(), sc, w); e != nil {
		t.Fatal(e)
	}
	t.Logf("complete streamed bytes=%d maximum write=%d", w.bytes, w.max)
	if w.bytes <= 32<<20 || w.bytes > 64<<20 || w.max > 128<<10 || !bytes.HasSuffix(w.tail, []byte("}")) || w.rows != 10000 || w.invalid {
		t.Fatal("export expansion/bounded writes violated")
	}
}

var _ io.Writer = (*managementCountingWriter)(nil)

func TestManagementHistoryStatesAndOrdering(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	for i := 0; i < 10; i++ {
		sentFixture(t, s, sc, fmt.Sprintf("id-%02d", i), "body", "reply")
	}
	if _, e := s.db.Exec("UPDATE messages SET created_at=1"); e != nil {
		t.Fatal(e)
	}
	if ok, e := s.ClaimMessage(ctx, sc, "processing", "uncompleted"); e != nil || !ok {
		t.Fatal(e)
	}
	if ok, e := s.ClaimMessage(ctx, sc, "failed", "uncompleted"); e != nil || !ok {
		t.Fatal(e)
	}
	if e := s.SetMessageState(ctx, sc, "failed", "failed"); e != nil {
		t.Fatal(e)
	}
	if ok, e := s.ClaimMessage(ctx, sc, "ambiguous", "uncompleted"); e != nil || !ok {
		t.Fatal(e)
	}
	if e := s.SetMessageState(ctx, sc, "ambiguous", "sending"); e != nil {
		t.Fatal(e)
	}
	if e := s.SetMessageState(ctx, sc, "ambiguous", "ambiguous"); e != nil {
		t.Fatal(e)
	}
	p, e := s.HistoryPage(ctx, sc, "", 0, 8)
	if e != nil || len(p.Items) != 8 || !p.HasMore || p.NextOffset != 8 {
		t.Fatalf("first history page %+v %v", p, e)
	}
	for i, entry := range p.Items {
		if entry.ID != fmt.Sprintf("id-%02d", 9-i) {
			t.Fatal("history tie order")
		}
	}
	p, e = s.HistoryPage(ctx, sc, "", 8, 8)
	if e != nil || len(p.Items) != 2 || p.HasMore || p.NextOffset != 0 || p.Items[0].ID != "id-01" || p.Items[1].ID != "id-00" {
		t.Fatalf("final history page %+v %v", p, e)
	}
	for _, offset := range []int{10, 10000} {
		p, e = s.HistoryPage(ctx, sc, "", offset, 1)
		if e != nil || len(p.Items) != 0 || p.HasMore {
			t.Fatal("empty terminal page", e)
		}
	}
}

func TestMemoryExportAdmissionAndCeiling(t *testing.T) {
	t.Run("oversized legacy before writing", func(t *testing.T) {
		s, _ := testStore(t)
		sc := Scope{"a", "u"}
		managementFact(t, s, sc, "good", 0, false)
		if _, e := s.db.Exec("UPDATE facts SET content=?", strings.Repeat("x", 16385)); e != nil {
			t.Fatal(e)
		}
		w := &managementCountingWriter{}
		if e := s.WriteMemoryJSON(context.Background(), sc, w); !errors.Is(e, ErrInvalid) || w.bytes != 0 {
			t.Fatalf("legacy admission %v bytes=%d", e, w.bytes)
		}
		if _, e := s.FactsPage(context.Background(), sc, "", 0, 1); !errors.Is(e, ErrInvalid) {
			t.Fatalf("fact admission %v", e)
		}
	})
	t.Run("scoped quota before writing", func(t *testing.T) {
		s, _ := testStore(t)
		sc := Scope{"a", "u"}
		managementBulk(t, s, sc, 257, strings.Repeat("x", 16384), "", false)
		w := &managementCountingWriter{}
		if e := s.WriteMemoryJSON(context.Background(), sc, w); !errors.Is(e, ErrCapacity) || w.bytes != 0 {
			t.Fatalf("quota admission %v bytes=%d", e, w.bytes)
		}
	})
	t.Run("stream ceiling before writer", func(t *testing.T) {
		sink := &managementFaultWriter{}
		w := memoryJSONWriter{ctx: context.Background(), w: sink, written: maxMemoryJSONBytes - 1}
		if e := w.write([]byte("x")); e != nil || sink.calls != 1 {
			t.Fatal("exact ceiling", e)
		}
		if e := w.write([]byte("x")); !errors.Is(e, ErrCapacity) || sink.calls != 1 || w.written != maxMemoryJSONBytes {
			t.Fatalf("above ceiling %v calls=%d", e, sink.calls)
		}
	})
}

func TestMemoryExportConcurrentConfirmation(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	sc := Scope{"a", "u"}
	sentFixture(t, s, sc, "proof", "visible quote", "reply")
	if e := s.SaveDerived(ctx, sc, 0, Summary{Text: "summary", Watermark: 1}, []Candidate{{Content: "reviewable", MessageID: "proof", Quote: "visible quote"}}); e != nil {
		t.Fatal(e)
	}
	cs, e := s.Candidates(ctx, sc, 128)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	started := make(chan struct{})
	w := &managementHookWriter{hook: func() {
		go func() { close(started); _, e := s.ConfirmCandidate(ctx, sc, cs[0].ID); done <- e }()
		<-started
	}}
	if e := s.WriteMemoryJSON(ctx, sc, w); e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	var before managementExport
	if e := json.Unmarshal(w.Bytes(), &before); e != nil || len(before.Facts) != 0 || len(before.Candidates) != 1 {
		t.Fatal("inconsistent confirmation snapshot", e)
	}
	var afterBuffer bytes.Buffer
	if e := s.WriteMemoryJSON(ctx, sc, &afterBuffer); e != nil {
		t.Fatal(e)
	}
	var after managementExport
	if e := json.Unmarshal(afterBuffer.Bytes(), &after); e != nil || len(after.Facts) != 1 || len(after.Candidates) != 0 {
		t.Fatal("post confirmation snapshot", e)
	}
}
