package agent

import (
	"github.com/hidxt/miskoai/internal/storage"
	"strconv"
)

// Called only after the unchanged selector refused. Observations never grant
// selection authority. Invalid/unmatched inputs remain a generic refusal.
func refusedImageDecision(advice imageAdvice, profile storage.Profile, now int64) string {
	id, err := strconv.ParseInt(advice.id, 10, 64)
	if err != nil {
		return "selection_refused"
	}
	found := false
	for _, row := range advice.candidates {
		if row.ID == id {
			found = true
			break
		}
	}
	if !found {
		return "not_offered"
	}
	cooldown := int64(30000)
	if profile.Sticker == "low" {
		cooldown = 180000
	}
	if advice.last > 0 && now >= advice.last && now-advice.last < cooldown {
		return "cooldown"
	}
	for _, row := range advice.candidates {
		if row.ID == id && row.LastUsedMillis > 0 && now >= row.LastUsedMillis && now-row.LastUsedMillis < 600000 {
			return "repeat_cooldown"
		}
	}
	return "selection_refused"
}
