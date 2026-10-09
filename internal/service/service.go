// Package service connects durable receive evidence to one serial account worker.
package service

import (
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/storage"
	"sync"
	"time"
)

type Poller interface {
	RawUpdates(context.Context, string) ([]byte, error)
}
type Handler interface {
	Handle(context.Context, agent.Incoming) (agent.Result, error)
}

// Observer.Wake must return promptly and coalesce any background work.
// The service calls it synchronously after durable successful delivery.
type Observer interface{ Wake() }
type Options struct{ Account, User string }
type Service struct {
	store    *storage.Store
	poller   Poller
	handler  Handler
	scope    storage.Scope
	mu       sync.Mutex
	status   Status
	started  bool
	observer Observer
	wake     chan struct{}
	wait     func(context.Context, time.Duration) bool
}

func (s *Service) SetObserver(o Observer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return errors.New("service already started")
	}
	s.observer = o
	return nil
}

func New(s *storage.Store, p Poller, h Handler, o Options) (*Service, error) {
	if s == nil || p == nil || h == nil || !validField(o.Account, 256) || !validField(o.User, 256) {
		return nil, errors.New("invalid service configuration")
	}
	return &Service{store: s, poller: p, handler: h, scope: storage.Scope{Account: o.Account, User: o.User}, wake: make(chan struct{}, 32), wait: waitContext, status: Status{Channel: "stopped"}}, nil
}
func waitContext(c context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.Done():
		return false
	case <-t.C:
		return true
	}
}

// Run starts exactly two account workers and joins them before returning. Account
// pause leaves the caller's management lifetime intact until parent cancellation.
func (s *Service) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("invalid service context")
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("service already started")
	}
	s.started = true
	s.status.StartedAt = time.Now().UTC()
	s.status.Channel = "receiving"
	s.mu.Unlock()
	account, cancel := context.WithCancel(ctx)
	defer cancel()
	var group sync.WaitGroup
	group.Add(2)
	go func() { defer group.Done(); s.poll(account, cancel) }()
	go func() { defer group.Done(); s.work(account, cancel) }()
	<-ctx.Done()
	cancel()
	group.Wait()
	s.setChannel("stopped", "")
	return nil
}
func (s *Service) setChannel(state, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Channel == "paused" && state != "stopped" {
		return
	}
	s.status.Channel = state
	if code != "" {
		s.status.LastCode = safeCode(code)
	}
}
func (s *Service) pause(cancel context.CancelFunc, code string) {
	s.mu.Lock()
	if s.status.Channel != "paused" {
		s.status.Channel = "paused"
		s.status.LastCode = safeCode(code)
	}
	s.mu.Unlock()
	cancel()
}
func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Service) storagePause(cancel context.CancelFunc, err error) {
	code := "storage"
	if errors.Is(err, storage.ErrCapacity) {
		code = "capacity"
	}
	s.pause(cancel, code)
}
func (s *Service) poll(ctx context.Context, cancel context.CancelFunc) {
	delay := time.Second
	for ctx.Err() == nil {
		frame, err := s.store.PendingPoll(ctx, s.scope)
		if err != nil {
			s.storagePause(cancel, err)
			return
		}
		if frame != nil && frame.State == "quarantined" {
			s.setChannel("quarantined", "protocol")
			return
		}
		if frame == nil {
			cursor, err := s.store.Cursor(ctx, s.scope)
			if err != nil {
				s.storagePause(cancel, err)
				return
			}
			raw, err := s.poller.RawUpdates(ctx, cursor)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				if errors.Is(err, weixin.ErrAuthExpired) || errors.Is(err, weixin.ErrUnauthorized) {
					s.pause(cancel, "authorization")
					return
				}
				if errors.Is(err, weixin.ErrProtocol) || errors.Is(err, weixin.ErrResponseLimit) || errors.Is(err, weixin.ErrInvalidInput) || errors.Is(err, weixin.ErrEndpoint) {
					s.pause(cancel, "protocol")
					return
				}
				code := "transport"
				if errors.Is(err, weixin.ErrRateLimited) {
					code = "rate"
				}
				if errors.Is(err, weixin.ErrService) {
					code = "service"
				}
				s.setChannel("backoff", code)
				if !s.wait(ctx, delay) {
					return
				}
				delay *= 2
				if delay > 30*time.Second {
					delay = 30 * time.Second
				}
				continue
			}
			// Retain opaque HTTP evidence, including zero bytes, before parsing it.
			id, err := s.store.RecordPoll(ctx, s.scope, cursor, raw)
			// Global frame pressure retains these bounded bytes in this fixed poller;
			// no further remote call is made until the same response is persisted.
			for errors.Is(err, storage.ErrCapacity) && ctx.Err() == nil {
				s.setChannel("backoff", "capacity")
				if !s.wait(ctx, time.Second) {
					return
				}
				id, err = s.store.RecordPoll(ctx, s.scope, cursor, raw)
			}
			if err != nil {
				s.storagePause(cancel, err)
				return
			}
			frame = &storage.PollFrame{ID: id, Scope: s.scope, Cursor: cursor, Body: raw, State: "pending"}
		}
		entries, next, err := normalize(frame.Body, s.scope, time.Now().UTC())
		if err != nil {
			if errors.Is(err, weixin.ErrAuthExpired) || errors.Is(err, weixin.ErrUnauthorized) {
				s.pause(cancel, "authorization")
				return
			}
			if q := s.store.QuarantinePoll(ctx, s.scope, frame.ID); q != nil {
				s.storagePause(cancel, q)
				return
			}
			s.setChannel("quarantined", "protocol")
			return
		}
		if next == "" {
			next = frame.Cursor
		}
		err = s.store.ResolvePoll(ctx, s.scope, frame.ID, next, entries)
		for errors.Is(err, storage.ErrCapacity) && ctx.Err() == nil {
			s.setChannel("backoff", "capacity")
			s.notify()
			if !s.wait(ctx, time.Second) {
				return
			}
			err = s.store.ResolvePoll(ctx, s.scope, frame.ID, next, entries)
		}
		if err != nil {
			s.storagePause(cancel, err)
			return
		}
		s.mu.Lock()
		s.status.Received = saturate64(s.status.Received, int64(len(entries)))
		if s.status.Channel == "receiving" || s.status.Channel == "backoff" {
			s.status.Channel = "receiving"
		}
		s.mu.Unlock()
		s.notify()
		delay = time.Second
		if len(entries) == 0 && !s.wait(ctx, time.Second) {
			return
		}
	}
}
func (s *Service) work(ctx context.Context, cancel context.CancelFunc) {
	for ctx.Err() == nil {
		entries, err := s.store.PendingInbox(ctx, s.scope, 32)
		if err != nil {
			s.storagePause(cancel, err)
			return
		}
		if len(entries) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			}
			continue
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return
			}
			r, handleErr := s.handler.Handle(ctx, agent.Incoming{Account: e.Scope.Account, User: e.Scope.User, ID: e.MessageID, Text: e.Text, ContextToken: e.ContextToken})
			if ctx.Err() != nil {
				return
			}
			if r.Code == "storage" || r.Code == "capacity" {
				s.pause(cancel, r.Code)
				return
			}
			switch r.State {
			case "sent", "duplicate", "rejected", "failed", "ambiguous":
			default:
				s.pause(cancel, "handler")
				return
			}
			// Never detach inbox completion: canceled work must remain durable.
			if err = s.store.CompleteInbox(ctx, s.scope, e.Sequence); err != nil {
				s.storagePause(cancel, err)
				return
			}
			s.recordResult(r)
			if r.State == "sent" && handleErr == nil && s.observer != nil {
				s.observer.Wake()
			}
		}
	}
}
