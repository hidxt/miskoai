package service

import (
	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/netx"
	"github.com/hidxt/miskoai/internal/provider"
	"math"
	"time"
)

// Status contains only fixed states/codes and process-local, saturated totals.
// Received counts normalized authorized entries committed from a frame, including
// repeat deliveries. Duplicate counts Handler duplicate results, not receive-side
// suppression by storage. These observations are not durable delivery totals.
type Status struct {
	StartedAt                                               time.Time
	Channel                                                 string
	Received, Processed, Sent, Duplicate, Failed, Ambiguous int64
	Usage                                                   provider.Usage
	LastCode                                                string
	LastImageDecision                                       string
	LastModelFailure                                        string
}

func (s *Service) Snapshot() Status { s.mu.Lock(); defer s.mu.Unlock(); return s.status }

// ImageDecisionCode also protects status consumers using an injected backend.
// No untrusted model/provider text is allowed in these diagnostic observations.
func ImageDecisionCode(code string) string {
	switch code {
	case "", "not_applicable", "unavailable", "profile_invalid", "profile_off", "empty_catalogue", "context_limit", "snapshot_error", "canceled", "model_error", "invalid_output", "plain_text", "no_suggestion", "invalid_advice", "output_limited", "not_offered", "cooldown", "repeat_cooldown", "selection_refused", "image_pipeline", "image_acked":
		return code
	default:
		return "unknown"
	}
}
func safeCode(c string) string {
	switch c {
	case "", "authorization", "protocol", "transport", "rate", "service", "storage", "capacity", "handler", "scope", "input", "canceled", "limit", "model", "memory", "search", "output", "send":
		return c
	default:
		return "handler"
	}
}
func saturate64(a, b int64) int64 {
	if b < 0 {
		return a
	}
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}
func saturateInt(a, b int) int {
	if b < 0 {
		return a
	}
	if a > math.MaxInt-b {
		return math.MaxInt
	}
	return a + b
}
func (s *Service) recordResult(r agent.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := &s.status
	x.LastImageDecision = ImageDecisionCode(r.ImageDecision)
	x.LastModelFailure = netx.NormalizeFailureCode(r.ModelFailure)
	x.Processed = saturate64(x.Processed, 1)
	switch r.State {
	case "sent":
		x.Sent = saturate64(x.Sent, 1)
	case "duplicate":
		x.Duplicate = saturate64(x.Duplicate, 1)
	case "ambiguous":
		x.Ambiguous = saturate64(x.Ambiguous, 1)
	case "rejected", "failed":
		x.Failed = saturate64(x.Failed, 1)
	}
	if x.Channel != "paused" && x.Channel != "quarantined" {
		x.LastCode = safeCode(r.Code)
	}
	x.Usage.PromptTokens = saturateInt(x.Usage.PromptTokens, r.Usage.PromptTokens)
	x.Usage.CompletionTokens = saturateInt(x.Usage.CompletionTokens, r.Usage.CompletionTokens)
	x.Usage.TotalTokens = saturateInt(x.Usage.TotalTokens, r.Usage.TotalTokens)
	x.Usage.CachedTokens = saturateInt(x.Usage.CachedTokens, r.Usage.CachedTokens)
	x.Usage.CompletionDetails.ReasoningTokens = saturateInt(x.Usage.CompletionDetails.ReasoningTokens, r.Usage.CompletionDetails.ReasoningTokens)
}
