package core

import (
	"context"
	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/service"
	"github.com/hidxt/miskoai/internal/summary"
	"math"
	"runtime"
	"time"
)

// Attempts are logical delegate operations, not HTTP request or billing counts.
type Metrics struct {
	ChatAttempts, SummaryAttempts, VisionAttempts, SearchAttempts uint64
	SuccessfulUsage                                               provider.Usage
}
type Event struct {
	Kind string
	At   time.Time
}
type Status struct {
	UptimeSeconds                int64
	HeapBytes                    uint64
	Goroutines                   int
	GCCount                      uint32
	RSSAvailable                 bool
	ChannelState                 string
	HasDeepSeekKey, HasOllamaKey bool
	Channel                      service.Status
	Summary                      summary.Status
	Metrics                      Metrics
	Events                       []Event
}

func (c *Controller) eventLocked(kind string) {
	if len(c.events) == 128 {
		copy(c.events, c.events[1:])
		c.events = c.events[:127]
	}
	c.events = append(c.events, Event{Kind: kind, At: time.Now().UTC()})
}
func (c *Controller) Status() Status {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Status{UptimeSeconds: int64(time.Since(c.startedAt) / time.Second), HeapBytes: m.HeapAlloc, Goroutines: runtime.NumGoroutine(), GCCount: m.NumGC, HasDeepSeekKey: c.cfg.DeepSeekKey != "", HasOllamaKey: c.cfg.OllamaKey != "", Channel: c.serviceSnapshot, Summary: c.summarySnapshot, Metrics: c.metrics, Events: append([]Event(nil), c.events...)}
	if g := c.generation; g != nil {
		s.Channel = g.service.Snapshot()
		s.Summary = g.summary.Snapshot()
	}
	switch {
	case c.closed:
		s.ChannelState = "closed"
	case c.closing:
		s.ChannelState = "closing"
	case c.unavailable:
		s.ChannelState = "unavailable"
	case c.expired:
		s.ChannelState = "authorization_expired"
	case c.paused:
		s.ChannelState = "restore_paused"
	case c.scope.Account == "":
		s.ChannelState = "unconfigured"
	case !c.providerReady:
		s.ChannelState = "provider_unconfigured"
	case c.generation != nil:
		s.ChannelState = s.Channel.Channel
	default:
		s.ChannelState = "stopped"
	}
	return s
}
func increment(v *uint64) {
	if *v < math.MaxUint64 {
		*v++
	}
}
func addInt(a, b int) int {
	if b < 0 {
		return a
	}
	if a > math.MaxInt-b {
		return math.MaxInt
	}
	return a + b
}
func addUsage(a *provider.Usage, b provider.Usage) {
	a.PromptTokens = addInt(a.PromptTokens, b.PromptTokens)
	a.CompletionTokens = addInt(a.CompletionTokens, b.CompletionTokens)
	a.TotalTokens = addInt(a.TotalTokens, b.TotalTokens)
	a.CachedTokens = addInt(a.CachedTokens, b.CachedTokens)
	a.CompletionDetails.ReasoningTokens = addInt(a.CompletionDetails.ReasoningTokens, b.CompletionDetails.ReasoningTokens)
}

type countedModel struct {
	c         *Controller
	delegate  agent.Model
	operation string
}

func (m *countedModel) Chat(ctx context.Context, msg []provider.Message, n int) (provider.Reply, error) {
	return m.chat(ctx, msg, n, m.delegate.Chat)
}

// Preserve optional JSON capability through accounting. Legacy delegates use
// one Chat call; errors never cause a second request or a plaintext fallback.
func (m *countedModel) ChatJSON(ctx context.Context, msg []provider.Message, n int) (provider.Reply, error) {
	chat := m.delegate.Chat
	if model, ok := m.delegate.(agent.JSONModel); ok {
		chat = model.ChatJSON
	}
	return m.chat(ctx, msg, n, chat)
}

func (m *countedModel) chat(ctx context.Context, msg []provider.Message, n int, call func(context.Context, []provider.Message, int) (provider.Reply, error)) (provider.Reply, error) {
	m.c.mu.Lock()
	switch m.operation {
	case "summary":
		increment(&m.c.metrics.SummaryAttempts)
	case "vision":
		increment(&m.c.metrics.VisionAttempts)
	default:
		increment(&m.c.metrics.ChatAttempts)
	}
	m.c.mu.Unlock()
	r, e := call(ctx, msg, n)
	if e == nil {
		m.c.mu.Lock()
		addUsage(&m.c.metrics.SuccessfulUsage, r.Usage)
		m.c.mu.Unlock()
	}
	return r, e
}

type countedSearcher struct {
	c        *Controller
	delegate agent.Searcher
}

func (s *countedSearcher) Search(ctx context.Context, q string, n int) ([]provider.SearchResult, error) {
	s.c.mu.Lock()
	increment(&s.c.metrics.SearchAttempts)
	s.c.mu.Unlock()
	return s.delegate.Search(ctx, q, n)
}
func (c *Controller) countedSearch() agent.Searcher {
	if c.search == nil {
		return nil
	}
	return &countedSearcher{c: c, delegate: c.search}
}
