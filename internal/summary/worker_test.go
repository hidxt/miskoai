package summary

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

type modelFunc func(context.Context, []provider.Message, int) (provider.Reply, error)

func (f modelFunc) Chat(c context.Context, m []provider.Message, n int) (provider.Reply, error) {
	return f(c, m, n)
}
func fixture(t *testing.T, n int) (*storage.Store, *Worker) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	s, e := storage.Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	w, e := New(s, modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		return provider.Reply{Text: `{"summary":"saved","candidates":[]}`, FinishReason: "stop"}, nil
	}), Options{"a", "u"})
	if e != nil {
		t.Fatal(e)
	}
	addSent(t, s, n, 0)
	return s, w
}
func addSent(t *testing.T, s *storage.Store, n, start int) {
	t.Helper()
	c := context.Background()
	sc := storage.Scope{Account: "a", User: "u"}
	for i := start; i < start+n; i++ {
		id := fmt.Sprintf("%03d", i)
		ok, e := s.ClaimMessage(c, sc, id, "user evidence")
		if e != nil || !ok {
			t.Fatal(ok, e)
		}
		if e = s.SetMessageState(c, sc, id, "sending"); e != nil {
			t.Fatal(e)
		}
		if e = s.CompleteMessage(c, sc, id, "assistant injection"); e != nil {
			t.Fatal(e)
		}
	}
}
func TestWatermarkAndFacts(t *testing.T) {
	s, w := fixture(t, 15)
	w.attempt(context.Background())
	d, _ := s.Derived(context.Background(), w.scope)
	if d.Revision != 0 {
		t.Fatal("ran before 16")
	}
	addSent(t, s, 1, 15)
	w.attempt(context.Background())
	d, _ = s.Derived(context.Background(), w.scope)
	if d.Watermark != 16 || w.Snapshot().Completed != 1 {
		t.Fatalf("not saved: %+v %+v", d, w.Snapshot())
	}
	w.attempt(context.Background())
	if w.Snapshot().Completed != 1 {
		t.Fatal("duplicate work")
	}
	facts, e := s.ExportFacts(context.Background(), w.scope)
	if e != nil || len(facts) != 0 {
		t.Fatal("derived became facts", facts, e)
	}
}
func TestFailureCooldownNeedsWake(t *testing.T) {
	_, w := fixture(t, 16)
	now := time.Unix(100, 0)
	w.now = func() time.Time { return now }
	var calls int
	w.model = modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		calls++
		return provider.Reply{}, errors.New("private provider detail")
	})
	w.attempt(context.Background())
	now = now.Add(59 * time.Second)
	w.attempt(context.Background())
	if calls != 1 {
		t.Fatal("cooldown bypass")
	}
	now = now.Add(time.Second)
	if calls != 1 {
		t.Fatal("autonomous call")
	}
	w.attempt(context.Background())
	if calls != 2 || w.Snapshot().Failed != 2 || w.Snapshot().LastCode != "provider" {
		t.Fatal(calls, w.Snapshot())
	}
}
func TestClearWhileInFlight(t *testing.T) {
	s, w := fixture(t, 16)
	w.model = modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		if e := s.ClearMemory(context.Background(), w.scope); e != nil {
			t.Fatal(e)
		}
		return provider.Reply{Text: `{"summary":"old","candidates":[]}`, FinishReason: "stop"}, nil
	})
	w.attempt(context.Background())
	d, _ := s.Derived(context.Background(), w.scope)
	if d.Text != "" || d.Watermark != 16 || w.Snapshot().LastCode != "stale" {
		t.Fatal(d, w.Snapshot())
	}
	ok, e := s.ClaimMessage(context.Background(), w.scope, "000", "again")
	if e != nil || ok {
		t.Fatal("claim lost")
	}
}
func TestWakeCoalescesAndCancellationJoinsActualModel(t *testing.T) {
	_, w := fixture(t, 16)
	entered := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	w.model = modelFunc(func(ctx context.Context, m []provider.Message, n int) (provider.Reply, error) {
		calls.Add(1)
		if n != 1024 {
			t.Error(n)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 60*time.Second {
			t.Error("deadline")
		}
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return provider.Reply{}, ctx.Err()
	})
	for i := 0; i < 100; i++ {
		w.Wake()
	}
	if len(w.wake) != 1 {
		t.Fatal("wake queue")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	await(t, entered)
	for i := 0; i < 100; i++ {
		w.Wake()
	}
	cancel()
	await(t, canceled)
	select {
	case <-done:
		t.Fatal("abandoned model")
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("did not join")
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	if e := w.Run(context.Background()); e == nil {
		t.Fatal("second Run")
	}
}
func TestRunWithoutWake(t *testing.T) {
	_, w := fixture(t, 16)
	w.model = modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		t.Error("model without wake")
		return provider.Reply{}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	waitCondition(t, func() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.started })
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not finish")
	}
}

func waitCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for !condition() {
		select {
		case <-deadline.C:
			t.Fatal("condition timeout")
		default:
			runtime.Gosched()
		}
	}
}

func TestPendingWakeUsesCapturedWatermarkAndDoesNotDuplicate(t *testing.T) {
	s, w := fixture(t, 16)
	first := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	w.model = modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		n := calls.Add(1)
		if n == 1 {
			close(first)
			<-release
		}
		return provider.Reply{Text: `{"summary":"saved","candidates":[]}`, FinishReason: "stop"}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	w.Wake()
	go func() { done <- w.Run(ctx) }()
	await(t, first)
	addSent(t, s, 16, 16)
	for i := 0; i < 100; i++ {
		w.Wake()
	}
	close(release)
	waitCondition(t, func() bool { return w.Snapshot().Completed == 2 })
	d, e := s.Derived(context.Background(), w.scope)
	if e != nil || d.Watermark != 32 || d.Revision != 2 {
		t.Fatal(d, e)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not finish")
	}
	if calls.Load() != 2 {
		t.Fatal("duplicate jobs", calls.Load())
	}
}
func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("bounded wait expired")
	}
}

func TestCandidatesStayUnconfirmedAndQuotaIsAtomic(t *testing.T) {
	s, w := fixture(t, 16)
	ctx := context.Background()
	if _, e := s.AddFact(ctx, w.scope, storage.Fact{Content: "explicit existing", Source: "explicit_user", Confidence: 1, Importance: 50}); e != nil {
		t.Fatal(e)
	}
	w.model = modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		return provider.Reply{Text: `{"summary":"derived","candidates":[{"content":"unconfirmed proposal","message_id":"000","quote":"evidence"}]}`, FinishReason: "stop"}, nil
	})
	w.attempt(ctx)
	cs, e := s.Candidates(ctx, w.scope, 128)
	if e != nil || len(cs) != 1 {
		t.Fatal(cs, e)
	}
	fs, e := s.ExportFacts(ctx, w.scope)
	if e != nil || len(fs) != 1 || fs[0].Content != "explicit existing" {
		t.Fatal("facts changed", fs, e)
	}
	d, e := s.Derived(ctx, w.scope)
	if e != nil {
		t.Fatal(e)
	}
	// Fill the remaining candidate slots through the public scoped API.
	for i := 1; i < 128; i++ {
		e = s.SaveDerived(ctx, w.scope, d.Revision, storage.Summary{Text: "baseline", Watermark: 16}, []storage.Candidate{{Content: fmt.Sprintf("proposal %d", i), MessageID: "000", Quote: "evidence"}})
		if e != nil {
			t.Fatal(e)
		}
		d, e = s.Derived(ctx, w.scope)
		if e != nil {
			t.Fatal(e)
		}
	}
	addSent(t, s, 16, 16)
	w.model = modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		return provider.Reply{Text: `{"summary":"must rollback","candidates":[{"content":"129th proposal","message_id":"031","quote":"evidence"}]}`, FinishReason: "stop"}, nil
	})
	w.attempt(ctx)
	after, e := s.Derived(ctx, w.scope)
	if e != nil || after.Text != "baseline" || after.Revision != d.Revision || after.Watermark != 16 || w.Snapshot().LastCode != "capacity" {
		t.Fatal(after, e, w.Snapshot())
	}
	cs, e = s.Candidates(ctx, w.scope, 128)
	if e != nil || len(cs) != 128 {
		t.Fatal(cs, e)
	}
	fs, e = s.ExportFacts(ctx, w.scope)
	if e != nil || len(fs) != 1 || fs[0].Content != "explicit existing" {
		t.Fatal("quota changed facts")
	}
}

func TestStatusSaturatesAndConfigurationRejects(t *testing.T) {
	s, w := fixture(t, 0)
	for _, o := range []Options{{}, {"a", ""}, {"a", string([]byte{0xff})}, {"a", string([]byte{0})}} {
		if _, e := New(s, w.model, o); e == nil {
			t.Fatal("invalid configuration")
		}
	}
	if _, e := New(nil, w.model, Options{"a", "u"}); e == nil {
		t.Fatal("nil store")
	}
	if _, e := New(s, nil, Options{"a", "u"}); e == nil {
		t.Fatal("nil model")
	}
	w.status = Status{Completed: math.MaxUint64, Failed: math.MaxUint64}
	w.record("ok")
	w.record("invalid")
	if got := w.Snapshot(); got.Completed != math.MaxUint64 || got.Failed != math.MaxUint64 || got.LastCode != "invalid" {
		t.Fatal(got)
	}
}

func TestFailureCannotChangeSentRecord(t *testing.T) {
	s, w := fixture(t, 16)
	w.model = modelFunc(func(context.Context, []provider.Message, int) (provider.Reply, error) {
		return provider.Reply{Text: `{"summary":"invalid","candidates":[{"content":"x","message_id":"000","quote":"assistant injection"}]}`, FinishReason: "stop"}, nil
	})
	w.attempt(context.Background())
	history, e := s.History(context.Background(), w.scope, 16)
	if e != nil || len(history) != 32 || history[1].Content != "assistant injection" || history[0].State != "sent" {
		t.Fatal("sent history affected", e)
	}
	d, e := s.Derived(context.Background(), w.scope)
	if e != nil || d.Revision != 0 || w.Snapshot().LastCode != "invalid" {
		t.Fatal(d, e, w.Snapshot())
	}
}
