package core

import (
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/service"
	"github.com/hidxt/miskoai/internal/storage"
	"github.com/hidxt/miskoai/internal/summary"
	"time"
)

type generation struct {
	cancel  context.CancelFunc
	done    chan struct{}
	service *service.Service
	summary *summary.Worker
}

func acquire(ctx context.Context, ch chan struct{}) error {
	if ctx == nil {
		return ErrConfiguration
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	select {
	case ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *Controller) notifyLocked() { close(c.changed); c.changed = make(chan struct{}) }

// No caller holds a read lease while acquiring maintenance ownership.
func (c *Controller) lease(ctx context.Context, write, scoped bool) (*storage.Store, func(), error) {
	if ctx == nil {
		return nil, nil, ErrConfiguration
	}
	for {
		c.mu.Lock()
		if c.closed || c.closing || c.unavailable || c.store == nil {
			c.mu.Unlock()
			return nil, nil, ErrUnavailable
		}
		if scoped && c.scope.Account == "" {
			c.mu.Unlock()
			return nil, nil, ErrUnconfigured
		}
		if ctx.Err() != nil {
			c.mu.Unlock()
			return nil, nil, ctx.Err()
		}
		if !c.exclusive && (!write || c.leases == 0) {
			if write {
				c.exclusive = true
			} else {
				c.leases++
			}
			s := c.store
			c.mu.Unlock()
			return s, func() {
				c.mu.Lock()
				if write {
					c.exclusive = false
				} else {
					c.leases--
				}
				c.notifyLocked()
				c.mu.Unlock()
			}, nil
		}
		changed := c.changed
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-changed:
		}
	}
}
func (c *Controller) eligibleLocked() bool {
	return !c.closed && !c.closing && !c.unavailable && !c.paused && !c.expired && c.scope.Account != "" && c.providerReady && c.parent != nil && c.parent.Err() == nil
}

// maintenance admission serializes generation replacement. mu is not held while joining.
func (c *Controller) startGeneration() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.eligibleLocked() || c.generation != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(c.parent)
	g := &generation{cancel: cancel, done: make(chan struct{})}
	ch := &observedChannel{c: c, delegate: c.channel, g: g}
	w, e := summary.New(c.store, &countedModel{c: c, delegate: c.model, operation: "summary"}, summary.Options{Account: c.scope.Account, User: c.scope.User})
	if e != nil {
		cancel()
		return ErrUnavailable
	}
	text := &countedModel{c: c, delegate: c.model, operation: "chat"}
	opts := agent.Options{Account: c.scope.Account, User: c.scope.User, MaxOutput: c.effective.MaxOutput, ContextBytes: c.effective.ContextBytes, ContextTokens: c.effective.ContextTokens}
	var a *agent.Agent
	if c.documents != nil {
		var vision agent.Model
		if c.vision != nil {
			vision = &countedModel{c: c, delegate: c.vision, operation: "vision"}
		}
		a, e = agent.NewWithAttachments(c.store, text, c.countedSearch(), ch, opts, vision, c.images, c.documents)
	} else if c.vision != nil && c.images != nil {
		a, e = agent.NewWithImages(c.store, text, c.countedSearch(), ch, opts, &countedModel{c: c, delegate: c.vision, operation: "vision"}, c.images)
	} else {
		a, e = agent.New(c.store, text, c.countedSearch(), ch, opts)
	}
	if e != nil {
		cancel()
		return ErrUnavailable
	}
	s, e := service.New(c.store, ch, a, service.Options{Account: c.scope.Account, User: c.scope.User})
	if e != nil {
		cancel()
		return ErrUnavailable
	}
	if e = s.SetObserver(w); e != nil {
		cancel()
		return ErrUnavailable
	}
	g.service = s
	g.summary = w
	c.generation = g
	c.eventLocked("generation_started")
	go func() {
		summaryDone := make(chan struct{})
		go func() { defer close(summaryDone); _ = w.Run(ctx) }()
		_ = s.Run(ctx)
		cancel()
		<-summaryDone
		c.mu.Lock()
		c.serviceSnapshot = s.Snapshot()
		c.summarySnapshot = w.Snapshot()
		c.mu.Unlock()
		close(g.done)
	}()
	return nil
}
func (c *Controller) stopGeneration() {
	c.mu.Lock()
	g := c.generation
	if g != nil {
		g.cancel()
	}
	c.mu.Unlock()
	if g == nil {
		return
	}
	<-g.done
	c.mu.Lock()
	if c.generation == g {
		c.generation = nil
		c.eventLocked("generation_joined")
	}
	c.mu.Unlock()
}
func (c *Controller) Run(parent context.Context) error {
	if parent == nil {
		return ErrConfiguration
	}
	if e := acquire(parent, c.maintenance); e != nil {
		return e
	}
	c.mu.Lock()
	if c.started || c.closed {
		c.mu.Unlock()
		<-c.maintenance
		return ErrUnavailable
	}
	c.started = true
	c.parent, c.runCancel = context.WithCancel(parent)
	ctx := c.parent
	c.mu.Unlock()
	e := c.startGeneration()
	<-c.maintenance
	if e != nil {
		return e
	}
	// Expired generations join themselves; management Run stays alive.
	for {
		c.mu.Lock()
		g := c.generation
		c.mu.Unlock()
		if g == nil {
			<-ctx.Done()
			break
		}
		select {
		case <-ctx.Done():
		case <-g.done:
			if e = acquire(context.Background(), c.maintenance); e == nil {
				c.mu.Lock()
				if c.generation == g {
					c.generation = nil
					c.eventLocked("generation_joined")
				}
				c.mu.Unlock()
				<-c.maintenance
			}
			continue
		}
		break
	}
	if e = acquire(context.Background(), c.maintenance); e != nil {
		return e
	}
	c.stopGeneration()
	<-c.maintenance
	return nil
}
func (c *Controller) Close() error {
	if e := acquire(context.Background(), c.maintenance); e != nil {
		return e
	}
	defer func() { <-c.maintenance }()
	c.mu.Lock()
	if c.closed {
		e := c.closeErr
		c.mu.Unlock()
		return e
	}
	c.closing = true
	if c.runCancel != nil {
		c.runCancel()
	}
	c.notifyLocked()
	c.mu.Unlock()
	c.stopGeneration()
	for {
		c.mu.Lock()
		if c.leases == 0 && !c.exclusive {
			break
		}
		ch := c.changed
		c.mu.Unlock()
		<-ch
	}
	s, l := c.store, c.lock
	c.store = nil
	c.mu.Unlock()
	var e error
	// Workers have returned through image/document consumers and artifact cleanup.
	// Injected dependencies are borrowed; only the production CDN is owned.
	if c.cdn != nil && c.cdn.Close() != nil {
		e = ErrUnavailable
	}
	if s != nil && s.Close() != nil {
		e = ErrUnavailable
	}
	if l != nil && l.Close() != nil {
		e = ErrUnavailable
	}
	c.mu.Lock()
	c.closeErr = e
	c.closed = true
	c.closing = false
	c.eventLocked("closed")
	close(c.closeDone)
	c.mu.Unlock()
	return e
}

type observedChannel struct {
	c        *Controller
	delegate Channel
	g        *generation
}

func (a *observedChannel) observe(e error) {
	if !errors.Is(e, weixin.ErrAuthExpired) {
		return
	}
	a.c.mu.Lock()
	if a.c.generation == a.g && !a.c.expired {
		a.c.expired = true
		a.c.eventLocked("authorization_expired")
		a.g.cancel()
	}
	a.c.mu.Unlock()
}
func (a *observedChannel) RawUpdates(ctx context.Context, s string) ([]byte, error) {
	v, e := a.delegate.RawUpdates(ctx, s)
	a.observe(e)
	return v, e
}
func (a *observedChannel) SendText(ctx context.Context, a1, a2, a3, a4 string) error {
	e := a.delegate.SendText(ctx, a1, a2, a3, a4)
	a.observe(e)
	return e
}
func bounded(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, ErrConfiguration
	}
	c, cancel := context.WithTimeout(ctx, d)
	return c, cancel, nil
}
