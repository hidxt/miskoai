package web

import (
	"github.com/hidxt/miskoai/internal/core"
	"net/http"
	"net/url"
)

// Event history is a bounded current-process snapshot, not a durable audit log.
func eventsRoute(w http.ResponseWriter, r *http.Request, b managementBackend, q url.Values) {
	offset, limit := int64(0), int64(20)
	var err error
	if values, ok := q["offset"]; ok {
		offset, err = decimalValue(values[0], 0, 128)
		if err != nil {
			managementError(w, err)
			return
		}
	}
	if values, ok := q["limit"]; ok {
		limit, err = decimalValue(values[0], 1, 32)
		if err != nil {
			managementError(w, err)
			return
		}
	}
	snapshot := b.Status()
	// Core records oldest to newest. Bound before projecting safe event fields.
	if len(snapshot.Events) > 128 {
		snapshot.Events = snapshot.Events[len(snapshot.Events)-128:]
	}
	events := statusResponse(snapshot).Events
	items := make([]eventDTO, 0, int(limit))
	start := int(offset)
	if start > len(events) {
		start = len(events)
	}
	end := start + int(limit)
	if end > len(events) {
		end = len(events)
	}
	for i := start; i < end; i++ {
		items = append(items, events[len(events)-1-i])
	}
	respondJSON(w, struct {
		Items      []eventDTO `json:"items"`
		NextOffset int        `json:"next_offset"`
		HasMore    bool       `json:"has_more"`
	}{items, end, end < len(events)})
}

func diagnosticCode(s core.Status, state string) string {
	switch state {
	case "authorization_expired":
		return "auth-expired"
	case "restore_paused":
		return "restore-paused"
	case "unconfigured", "provider_unconfigured", "closed", "closing", "unavailable":
		return state
	}
	switch s.Channel.LastCode {
	case "rate", "transport", "model", "storage":
		return s.Channel.LastCode
	case "send":
		if s.Channel.Ambiguous > 0 {
			return "send-ambiguous"
		}
		return "send"
	}
	return state
}
