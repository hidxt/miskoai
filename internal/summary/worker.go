// Package summary derives unconfirmed, scoped data from completed conversations.
package summary

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

type Model interface {
	Chat(context.Context, []provider.Message, int) (provider.Reply, error)
}
type Options struct{ Account, User string }

// Status contains only safe codes and saturating counters, never private data.
type Status struct {
	Completed, Failed uint64
	LastCode          string
}
type Worker struct {
	store   *storage.Store
	model   Model
	scope   storage.Scope
	wake    chan struct{}
	mu      sync.Mutex
	started bool
	status  Status
	// now is private to permit deterministic cooldown tests; production uses time.Now.
	now        func() time.Time
	retryAfter time.Time // accessed only by the actual Run loop
}

func New(s *storage.Store, m Model, o Options) (*Worker, error) {
	if s == nil || m == nil || !validText(o.Account, 256, true) || !validText(o.User, 256, true) {
		return nil, errors.New("invalid summary configuration")
	}
	return &Worker{store: s, model: m, scope: storage.Scope{Account: o.Account, User: o.User}, wake: make(chan struct{}, 1), now: time.Now}, nil
}

// Wake is a hint with no payload. At most one pending hint is retained.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run starts once. The model runs inline, so returning joins its actual work.
// A model implementation must honor cancellation for shutdown to finish promptly.
func (w *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("invalid summary context")
	}
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return errors.New("summary already started")
	}
	w.started = true
	w.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.wake:
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		w.attempt(ctx)
	}
}
func (w *Worker) Snapshot() Status { w.mu.Lock(); defer w.mu.Unlock(); return w.status }
func (w *Worker) record(code string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if code == "ok" {
		if w.status.Completed < math.MaxUint64 {
			w.status.Completed++
		}
	} else {
		if w.status.Failed < math.MaxUint64 {
			w.status.Failed++
		}
	}
	w.status.LastCode = code
}
func storageCode(err error) string {
	switch {
	case errors.Is(err, storage.ErrStale):
		return "stale"
	case errors.Is(err, storage.ErrCapacity):
		return "capacity"
	case errors.Is(err, storage.ErrInvalid):
		return "invalid"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	default:
		return "storage"
	}
}
func (w *Worker) fail(code string) { w.retryAfter = w.now().Add(60 * time.Second); w.record(code) }

// attempt is called only after a consumed wake by Run. Tests call it directly
// to exercise the same eligibility path without wall-clock sleeps.
func (w *Worker) attempt(parent context.Context) {
	if parent.Err() != nil || w.now().Before(w.retryAfter) {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	old, count, history, err := w.store.DerivedHistory(ctx, w.scope, 16)
	if err != nil {
		w.fail(storageCode(err))
		return
	}
	if count < old.Watermark || count-old.Watermark < 16 {
		return
	}
	messages, visible := buildPrompt(old, history)
	reply, err := w.model.Chat(ctx, messages, 1024)
	if ctx.Err() != nil {
		w.fail("canceled")
		return
	}
	if err != nil {
		w.fail("provider")
		return
	}
	text, candidates, err := parseReply(reply, visible)
	if err != nil {
		w.fail("invalid")
		return
	}
	err = w.store.SaveDerived(ctx, w.scope, old.Revision, storage.Summary{Text: text, Watermark: count}, candidates)
	if err != nil {
		w.fail(storageCode(err))
		return
	}
	w.retryAfter = time.Time{}
	w.record("ok")
}
