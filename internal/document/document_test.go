package document

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countReader struct{ calls atomic.Int32 }

func (r *countReader) ReadAt(p []byte, off int64) (int, error) {
	r.calls.Add(1)
	copy(p, "x")
	return len(p), nil
}

// Moving admission/validation after I/O breaks the no-read contract.
func TestParserValidationBeforeRead(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		format Format
		size   int64
		want   error
	}{
		{"nilcontext", nil, TXT, 1, ErrInvalid}, {"negative", context.Background(), TXT, -1, ErrInvalid},
		{"oversize", context.Background(), TXT, 4*1024*1024 + 1, ErrLimit},
		{"unknown", context.Background(), Format("PRIVATE-CANARY/path"), 1, ErrUnsupported},
		{"docx", context.Background(), DOCX, 1, ErrUnsupported}, {"pdf", context.Background(), PDF, 1, ErrUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &countReader{}
			expectError(t, New(), tc.ctx, tc.format, r, tc.size, tc.want)
			if r.calls.Load() != 0 {
				t.Fatal("refusal read payload")
			}
		})
	}
	expectError(t, New(), context.Background(), TXT, nil, 1, ErrInvalid)
	var typedNil *countReader
	expectError(t, New(), context.Background(), TXT, typedNil, 1, ErrInvalid)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &countReader{}
	expectError(t, New(), ctx, TXT, r, 1, ErrCanceled)
	if r.calls.Load() != 0 {
		t.Fatal("canceled input read")
	}
}

type heldReader struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *heldReader) ReadAt(p []byte, off int64) (int, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	copy(p, "x")
	return len(p), nil
}

type extraction struct {
	result Result
	err    error
}

func awaitExtraction(t *testing.T, ch <-chan extraction) extraction {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("parser did not join")
		return extraction{}
	}
}

// Releasing admission on a deadline while actual ReadAt is held breaks serialization.
func TestParserAdmissionAndCancel(t *testing.T) {
	p := New()
	r := &heldReader{entered: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	done := make(chan extraction, 1)
	var releaseOnce sync.Once
	joined := false
	t.Cleanup(func() {
		cancel()
		releaseOnce.Do(func() { close(r.release) })
		if joined {
			return
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("held parser cleanup failed to join")
		}
	})
	go func() { got, err := p.Extract(ctx, TXT, r, 1); done <- extraction{got, err} }()
	select {
	case <-r.entered:
	case <-time.After(time.Second):
		t.Fatal("reader never admitted")
	}
	<-ctx.Done()
	select {
	case <-done:
		t.Fatal("parser returned while read still held")
	default:
	}
	waiter := &countReader{}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer waitCancel()
	expectError(t, p, waitCtx, TXT, waiter, 1, ErrCanceled)
	if waiter.calls.Load() != 0 {
		t.Fatal("deadline released occupied admission")
	}
	// The parser's own two-second budget also includes the wait, with no reader call.
	expectError(t, p, context.Background(), TXT, waiter, 1, ErrCanceled)
	if waiter.calls.Load() != 0 {
		t.Fatal("cooperative deadline waiter entered")
	}
	releaseOnce.Do(func() { close(r.release) })
	got := awaitExtraction(t, done)
	joined = true
	if !errors.Is(got.err, ErrCanceled) || !errors.Is(got.err, context.DeadlineExceeded) || got.result != (Result{}) {
		t.Fatalf("held canceled extraction returned %v", got.err)
	}
	expectTextWithParser(t, p)
}
func expectTextWithParser(t *testing.T, p *Parser) {
	t.Helper()
	got, err := p.Extract(context.Background(), TXT, strings.NewReader("ok"), 2)
	if err != nil || got.Text != "ok" {
		t.Fatalf("slot not reusable: %v", err)
	}
}

type slowReader struct{}

func (slowReader) ReadAt(p []byte, off int64) (int, error) {
	time.Sleep(2100 * time.Millisecond)
	copy(p, "x")
	return len(p), io.EOF
}

type delayedReader struct{ delay time.Duration }

func (r delayedReader) ReadAt(p []byte, off int64) (int, error) {
	time.Sleep(r.delay)
	copy(p, "x")
	return len(p), nil
}

type observedContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *observedContext) Deadline() (time.Time, bool) {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Deadline()
}

// Resetting the two-second budget after admission permits late success here.
func TestParserDeadlineIncludesWaitAndRead(t *testing.T) {
	p := New()
	r := &heldReader{entered: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	active := make(chan extraction, 1)
	waiter := make(chan extraction, 1)
	var releaseOnce sync.Once
	activeJoined, waiterJoined, waiterStarted := false, false, false
	t.Cleanup(func() {
		cancel()
		releaseOnce.Do(func() { close(r.release) })
		if !activeJoined {
			select {
			case <-active:
			case <-time.After(5 * time.Second):
				t.Error("active cleanup did not join")
			}
		}
		if waiterStarted && !waiterJoined {
			select {
			case <-waiter:
			case <-time.After(5 * time.Second):
				t.Error("waiter cleanup did not join")
			}
		}
	})
	go func() { got, err := p.Extract(ctx, TXT, r, 1); active <- extraction{got, err} }()
	select {
	case <-r.entered:
	case <-time.After(time.Second):
		t.Fatal("active reader not admitted")
	}
	waiterStarted = true
	waitContext := &observedContext{Context: ctx, entered: make(chan struct{})}
	go func() {
		got, err := p.Extract(waitContext, TXT, delayedReader{1500 * time.Millisecond}, 1)
		waiter <- extraction{got, err}
	}()
	select {
	case <-waitContext.entered:
	case <-time.After(time.Second):
		t.Fatal("waiter never entered Extract")
	}
	time.Sleep(800 * time.Millisecond)
	releaseOnce.Do(func() { close(r.release) })
	activeResult := awaitExtraction(t, active)
	activeJoined = true
	waiterResult := awaitExtraction(t, waiter)
	waiterJoined = true
	if activeResult.err != nil || activeResult.result.Text != "x" {
		t.Fatalf("active extraction: %v", activeResult.err)
	}
	if !errors.Is(waiterResult.err, ErrCanceled) || !errors.Is(waiterResult.err, context.DeadlineExceeded) || waiterResult.result != (Result{}) {
		t.Fatalf("waiting budget reset: %v", waiterResult.err)
	}
	expectTextWithParser(t, p)
}

// Failing to check the deadline after the synchronous read permits late success.
func TestParserCooperativeDeadline(t *testing.T) {
	p := New()
	got, err := p.Extract(context.Background(), TXT, slowReader{}, 1)
	if !errors.Is(err, ErrCanceled) || !errors.Is(err, context.DeadlineExceeded) || got != (Result{}) {
		t.Fatalf("late read returned %v", err)
	}
	expectTextWithParser(t, p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.Extract(ctx, TXT, strings.NewReader("x"), 1)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrCanceled) {
		t.Fatalf("cancel compatibility: %v", err)
	}
}
