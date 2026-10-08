package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private", "memory.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestScopedFactsChineseExpiryAndFTSSync(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	a := Scope{"account-a", "alice"}
	b := Scope{"account-b", "alice"}
	c := Scope{"account-a", "bob"}
	f := Fact{Content: "我喜欢茉莉花茶", Category: "preference", Source: "explicit_user", Confidence: 1, Importance: 80}
	id, err := s.AddFact(ctx, a, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []Scope{b, c} {
		if _, err = s.AddFact(ctx, other, Fact{Content: "private jasmine", Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	expired := time.Now().Add(-time.Hour)
	f.Content = "茉莉 expired"
	f.ExpiresAt = &expired
	if _, err = s.AddFact(ctx, a, f); err != nil {
		t.Fatal(err)
	}
	got, err := s.SearchFacts(ctx, a, "茉莉", 10)
	if err != nil || len(got) != 1 || got[0].ID != id {
		t.Fatalf("Chinese scoped active search: %v %v", got, err)
	}
	got, err = s.SearchFacts(ctx, b, "茉莉", 10)
	if err != nil || len(got) != 0 {
		t.Fatalf("cross-account leak: %v %v", got, err)
	}
	f.ID = id
	f.Content = "jasmine tea preference"
	f.ExpiresAt = nil
	if err = s.UpdateFact(ctx, b, f); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-scope update: %v", err)
	}
	if err = s.UpdateFact(ctx, a, f); err != nil {
		t.Fatal(err)
	}
	// Execute MATCH directly: keyword fallback must not conceal absent/broken FTS5.
	var count int
	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM facts_fts JOIN facts f ON f.id=facts_fts.rowid WHERE facts_fts MATCH 'jasmine' AND f.account=? AND f.user=?`, a.Account, a.User).Scan(&count); err != nil || count != 1 {
		t.Fatalf("FTS5 actual match: %d %v", count, err)
	}
	got, err = s.SearchFacts(ctx, a, `jasmine " OR *`, 10)
	if err != nil {
		t.Fatalf("literal query must be safe: %v", err)
	}
	if err = s.DeleteFact(ctx, b, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-scope delete: %v", err)
	}
	if err = s.DeleteFact(ctx, a, id); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM facts_fts JOIN facts f ON f.id=facts_fts.rowid WHERE facts_fts MATCH 'jasmine' AND f.account=? AND f.user=?`, a.Account, a.User).Scan(&count); err != nil || count != 0 {
		t.Fatalf("FTS delete sync: %d %v", count, err)
	}
	if err = s.ClearFacts(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err = s.ExportFacts(ctx, b)
	if err != nil || len(got) != 1 || got[0].Content != "private jasmine" {
		t.Fatalf("clear crossed scope: %v %v", got, err)
	}
}

func TestValidationAndCancellation(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"account", "user"}
	for _, source := range []string{"", "model", "external"} {
		if _, err := s.AddFact(ctx, scope, Fact{Content: "untrusted", Source: source}); err == nil {
			t.Fatalf("accepted source %q", source)
		}
	}
	for _, bad := range []Scope{{}, {Account: "a"}, {User: "u"}} {
		if _, err := s.AddFact(ctx, bad, Fact{Content: "x", Source: "manual"}); err == nil {
			t.Fatal("accepted invalid scope")
		}
		if _, err := s.SearchFacts(ctx, bad, "x", 10); err == nil {
			t.Fatal("searched invalid scope")
		}
		if _, err := s.ExportFacts(ctx, bad); err == nil {
			t.Fatal("exported invalid scope")
		}
		if err := s.ClearFacts(ctx, bad); err == nil {
			t.Fatal("cleared invalid scope")
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.SearchFacts(cancelled, scope, "x", 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := s.SearchFacts(ctx, scope, "x", 1001); err == nil {
		t.Fatal("unbounded search accepted")
	}
}

func TestWALBackupReopenIntegrityAndMigration(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	var version string
	var fts5 int
	if err := s.db.QueryRowContext(ctx, "SELECT sqlite_version(),sqlite_compileoption_used('ENABLE_FTS5')").Scan(&version, &fts5); err != nil || fts5 != 1 {
		t.Fatalf("installed driver FTS5 compile option: %s %d %v", version, fts5, err)
	}
	t.Logf("SQLite %s; ENABLE_FTS5=%d", version, fts5)
	if _, err := s.AddFact(ctx, scope, Fact{Content: "committed WAL fact", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("WAL: %s %v", mode, err)
	}
	dest := filepath.Join(t.TempDir(), "private", "backup ' bound.db")
	if err := s.Backup(ctx, dest); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(ctx, dest); err == nil {
		t.Fatal("backup overwrote existing file")
	}
	if err := s.Backup(ctx, path); err == nil {
		t.Fatal("backup accepted the live database as destination")
	}
	backup, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	if err = backup.Integrity(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := backup.ExportFacts(ctx, scope)
	if err != nil || len(got) != 1 || got[0].Content != "committed WAL fact" {
		t.Fatalf("backup omitted WAL: %v %v", got, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.SearchFacts(ctx, scope, "committed", 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("migration/reopen: %v %v", got, err)
	}
	if _, err = os.Stat(dest); err != nil {
		t.Fatal(err)
	}
}

func TestClaimsRemainDurableAndScoped(t *testing.T) {
	s, path := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	const n = 8
	var wg sync.WaitGroup
	results := make(chan bool, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.ClaimMessage(ctx, scope, "message-1", "hello")
			results <- ok
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	winners := 0
	for ok := range results {
		if ok {
			winners++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("claim winners=%d", winners)
	}
	if err := s.SetMessageState(ctx, scope, "message-1", "sent"); err == nil {
		t.Fatal("illegal processing-to-sent transition")
	}
	if err := s.SetMessageState(ctx, scope, "message-1", "sending"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMessageState(ctx, scope, "message-1", "ambiguous"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"processing-crash", "sending-crash"} {
		if _, err := s.ClaimMessage(ctx, scope, id, "synthetic"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetMessageState(ctx, scope, "sending-crash", "sending"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if ok, err := reopened.ClaimMessage(ctx, scope, "message-1", "hello"); err != nil || ok {
		t.Fatalf("reclaimed ambiguous: %v %v", ok, err)
	}
	for _, id := range []string{"processing-crash", "sending-crash"} {
		if ok, err := reopened.ClaimMessage(ctx, scope, id, "synthetic"); err != nil || ok {
			t.Fatalf("reclaimed crash state %s: %v %v", id, ok, err)
		}
	}
	if ok, err := reopened.ClaimMessage(ctx, Scope{"b", "u"}, "message-1", "hello"); err != nil || !ok {
		t.Fatalf("account scoped IDs: %v %v", ok, err)
	}
	history, err := reopened.History(ctx, scope, 10)
	if err != nil || len(history) != 0 {
		t.Fatalf("history: %v %v", history, err)
	}
}

func TestCompletedHistoryHasUserAndAssistantOnly(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	if _, err := s.ClaimMessage(ctx, scope, "done", "question"); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteMessage(ctx, scope, "done", "answer"); err == nil {
		t.Fatal("completed before send intent")
	}
	if err := s.SetMessageState(ctx, scope, "done", "sending"); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteMessage(ctx, Scope{"b", "u"}, "done", "answer"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-scope completion: %v", err)
	}
	if err := s.CompleteMessage(ctx, scope, "done", "answer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimMessage(ctx, scope, "pending", "unfinished"); err != nil {
		t.Fatal(err)
	}
	got, err := s.History(ctx, scope, 10)
	if err != nil || len(got) != 2 || got[0].Role != "user" || got[0].Content != "question" || got[1].Role != "assistant" || got[1].Content != "answer" {
		t.Fatalf("completed context: %v %v", got, err)
	}
}

func TestFactContentBudget(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	content := strings.Repeat("x", 16384)
	var first int64
	for i := 0; i < 256; i++ {
		if id, err := s.AddFact(ctx, scope, Fact{Content: content, Source: "manual"}); err != nil {
			t.Fatal(err)
		} else if i == 0 {
			first = id
		}
	}
	if _, err := s.AddFact(ctx, scope, Fact{Content: "exceeds budget", Source: "manual"}); err == nil {
		t.Fatal("accepted more than 4MiB fact content")
	}
	if err := s.UpdateFact(ctx, scope, Fact{ID: first, Content: content[:16383], Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddFact(ctx, scope, Fact{Content: "z", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateFact(ctx, scope, Fact{ID: first, Content: content, Source: "manual"}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("update bypassed 4MiB cap: %v", err)
	}
	// A different scope retains its independent budget.
	if _, err := s.AddFact(ctx, Scope{"b", "u"}, Fact{Content: "private", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
}

func TestMessageCapacityPreservesExistingClaims(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	scope := Scope{"a", "u"}
	if _, err := s.db.ExecContext(ctx, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<10000) INSERT INTO messages(account,user,id,content,state,created_at,updated_at) SELECT 'a','u',CAST(x AS TEXT),'synthetic','processing',1,1 FROM n`); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimMessage(ctx, scope, "1", "duplicate"); err != nil || ok {
		t.Fatalf("full store duplicate: %v %v", ok, err)
	}
	if _, err := s.ClaimMessage(ctx, scope, "new", "new message"); err == nil {
		t.Fatal("unbounded message records")
	}
}

func BenchmarkChineseRecall(b *testing.B) {
	path := filepath.Join(b.TempDir(), "memory.db")
	s, err := Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	scope := Scope{"a", "u"}
	for i := 0; i < 100; i++ {
		if _, err = s.AddFact(ctx, scope, Fact{Content: "我喜欢茉莉花茶", Source: "manual"}); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = s.SearchFacts(ctx, scope, "茉莉", 10); err != nil {
			b.Fatal(err)
		}
	}
}
