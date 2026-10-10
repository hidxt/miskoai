package web

import (
	"context"
	"github.com/hidxt/miskoai/internal/core"
	"io"
	"mime"
	"net/http"
	"runtime"
	"strconv"
)

func systemRoute(w http.ResponseWriter, r *http.Request, b managementBackend) {
	switch r.URL.Path {
	case "/api/status":
		respondJSON(w, statusResponse(b.Status()))
	case "/api/diagnostics":
		respondJSON(w, struct {
			Product       string    `json:"product"`
			GoVersion     string    `json:"go_version"`
			OS            string    `json:"os"`
			Architecture  string    `json:"architecture"`
			ExternalProbe bool      `json:"external_probe"`
			Status        statusDTO `json:"status"`
		}{"MiskoAI", runtime.Version(), runtime.GOOS, runtime.GOARCH, false, statusResponse(b.Status())})
	case "/api/export", "/api/backup":
		if _, e := requestObject(r, 1024); e != nil {
			managementError(w, e)
			return
		}
		backup := r.URL.Path == "/api/backup"
		d, e := b.prepareDownload(r.Context(), backup)
		if e != nil {
			managementError(w, e)
			return
		}
		if d == nil {
			safeError(w, 500, "web_download")
			return
		}
		// A close failure before headers can be reported. After a streaming write it
		// cannot replace bytes already sent; abort the connection instead of success.
		closed := false
		defer func() {
			if !closed {
				_ = d.Close()
			}
		}()
		name, size := d.metadata()
		want, kind, capBytes := "miskoai-memory.json", "application/json", int64(64<<20)
		if backup {
			want, kind, capBytes = "miskoai-backup.db", "application/octet-stream", 256<<20
		}
		if name != want || size < 0 || size > capBytes {
			_ = d.Close()
			safeError(w, 500, "web_download")
			closed = true
			return
		}
		if e = r.Context().Err(); e != nil {
			ce := d.Close()
			closed = true
			if ce != nil {
				safeError(w, 500, "web_download")
			} else {
				managementError(w, e)
			}
			return
		}
		w.Header().Set("Content-Type", kind)
		w.Header().Set("Content-Disposition", `attachment; filename="`+want+`"`)
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusOK)
		n, e := io.CopyN(w, downloadReader{r.Context(), d}, size)
		ce := d.Close()
		closed = true
		if e != nil || n != size || ce != nil || r.Context().Err() != nil {
			panic(http.ErrAbortHandler)
		}
	case "/api/restore":
		headers := r.Header.Values("Content-Type")
		if len(headers) != 1 {
			managementError(w, errManagementInput)
			return
		}
		kind, params, e := mime.ParseMediaType(headers[0])
		if e != nil || kind != "application/octet-stream" || len(params) != 0 || r.ContentLength > 256<<20 {
			managementError(w, errManagementInput)
			return
		}
		reader := http.MaxBytesReader(w, r.Body, 256<<20)
		result, e := b.Restore(r.Context(), downloadReader{r.Context(), reader})
		if e != nil {
			managementError(w, e)
			return
		}
		respondJSON(w, struct {
			PriorRetained bool `json:"prior_retained"`
			ChannelPaused bool `json:"channel_paused"`
		}{result.PreviousRetained, result.ChannelPaused})
	case "/api/channel/resume":
		if e := acknowledge(r, "reconciled_restored_history"); e != nil {
			managementError(w, e)
			return
		}
		mutationResult(w, b.ResumeAfterReconciliation(r.Context()))
	}
}

type downloadReader struct {
	ctx context.Context
	r   io.Reader
}

func (r downloadReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(p)
}

type statusDTO struct {
	UptimeSeconds   int64       `json:"uptime_seconds"`
	HeapBytes       uint64      `json:"heap_bytes"`
	Goroutines      int         `json:"goroutines"`
	GCCount         uint32      `json:"gc_count"`
	RSSAvailable    bool        `json:"rss_available"`
	ChannelState    string      `json:"channel_state"`
	HasDeepSeekKey  bool        `json:"has_deepseek_key"`
	HasOllamaKey    bool        `json:"has_ollama_key"`
	Channel         channelDTO  `json:"channel"`
	Summary         summaryDTO  `json:"summary"`
	LogicalAttempts attemptsDTO `json:"logical_attempts"`
	SuccessfulUsage usageDTO    `json:"successful_usage"`
	Events          []eventDTO  `json:"events"`
}
type channelDTO struct {
	StartedAt string `json:"started_at"`
	State     string `json:"state"`
	Received  int64  `json:"received"`
	Processed int64  `json:"processed"`
	Sent      int64  `json:"sent"`
	Duplicate int64  `json:"duplicate"`
	Failed    int64  `json:"failed"`
	Ambiguous int64  `json:"ambiguous"`
	LastCode  string `json:"last_code"`
}
type summaryDTO struct {
	Completed uint64 `json:"completed"`
	Failed    uint64 `json:"failed"`
	LastCode  string `json:"last_code"`
}
type attemptsDTO struct {
	Chat    uint64 `json:"chat"`
	Summary uint64 `json:"summary"`
	Vision  uint64 `json:"vision"`
	Search  uint64 `json:"search"`
}
type usageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CachedTokens     int `json:"cached_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
}
type eventDTO struct {
	Kind string `json:"kind"`
	At   string `json:"at"`
}

func enum(value, fallback string, values ...string) string {
	for _, v := range values {
		if value == v {
			return v
		}
	}
	return fallback
}
func statusResponse(s core.Status) statusDTO {
	state := enum(s.ChannelState, "unavailable", "closed", "closing", "unavailable", "authorization_expired", "restore_paused", "unconfigured", "provider_unconfigured", "stopped", "receiving", "backoff", "paused", "quarantined")
	channel := channelDTO{webTime(s.Channel.StartedAt), enum(s.Channel.Channel, "stopped", "stopped", "receiving", "backoff", "paused", "quarantined"), s.Channel.Received, s.Channel.Processed, s.Channel.Sent, s.Channel.Duplicate, s.Channel.Failed, s.Channel.Ambiguous, enum(s.Channel.LastCode, "service", "", "authorization", "protocol", "transport", "rate", "service", "storage", "capacity", "handler", "scope", "input", "canceled", "limit", "model", "memory", "search", "output", "send")}
	summary := summaryDTO{s.Summary.Completed, s.Summary.Failed, enum(s.Summary.LastCode, "summary", "", "stale", "capacity", "invalid", "canceled", "provider", "storage", "ok")}
	events := make([]eventDTO, 0, 128)
	for i, event := range s.Events {
		if i == 128 {
			break
		}
		kind := enum(event.Kind, "", "created", "settings_saved", "generation_started", "generation_joined", "restore_paused", "reconciled", "closed", "authorization_expired")
		if kind != "" {
			events = append(events, eventDTO{kind, webTime(event.At)})
		}
	}
	u := s.Metrics.SuccessfulUsage
	return statusDTO{s.UptimeSeconds, s.HeapBytes, s.Goroutines, s.GCCount, false, state, s.HasDeepSeekKey, s.HasOllamaKey, channel, summary, attemptsDTO{s.Metrics.ChatAttempts, s.Metrics.SummaryAttempts, s.Metrics.VisionAttempts, s.Metrics.SearchAttempts}, usageDTO{u.PromptTokens, u.CompletionTokens, u.TotalTokens, u.CachedTokens, u.CompletionDetails.ReasoningTokens}, events}
}
