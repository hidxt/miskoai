package agent

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/storage"
)

// imageSelectionInput must come from the same trusted answer snapshot. Eligibility
// is an orchestration decision, never inferred from either text or model output.
// Candidates must be one consistent scoped snapshot; the caller also owns clock
// and confirmed-receipt provenance. Selection confers no upload/send authority.
type imageSelectionInput struct {
	Scope                          storage.Scope
	InboundID, UserText, Reply     string
	SuggestedID                    string
	Profile                        storage.Profile
	Candidates                     []storage.LibraryAsset
	NowMillis, LastConfirmedMillis int64
	OrdinarySuccess                bool
}

const imageSelectionMaxMillis int64 = 253402300799999

// selectReplyImage is deliberately inactive: it proposes an immutable metadata
// snapshot only. Availability, full byte validation, generation lifetime and
// durable once-only receipts remain obligations of the eventual orchestrator.
func selectReplyImage(in imageSelectionInput) (storage.LibraryAsset, bool) {
	refuse := storage.LibraryAsset{}
	// Caller-controlled sizes are admitted before parsing, normalization or copies.
	if !in.OrdinarySuccess || len(in.Candidates) == 0 || len(in.Candidates) > 128 ||
		len(in.Scope.Account) > 256 || len(in.Scope.User) > 256 || len(in.InboundID) > 512 ||
		len(in.UserText) > 16384 || len(in.Reply) > 16384 || len(in.SuggestedID) > 19 {
		return refuse, false
	}
	if !imageSelectionScope(in.Scope) || !imageSelectionField(in.InboundID, 512) || strings.TrimSpace(in.InboundID) == "" ||
		!validText(in.UserText) || !validText(in.Reply) || strings.TrimSpace(in.UserText) == "" || strings.TrimSpace(in.Reply) == "" ||
		!imageSelectionProfile(in.Profile) || in.Profile.Sticker == "off" || in.NowMillis <= 0 || in.NowMillis > imageSelectionMaxMillis ||
		!imageSelectionStamp(in.LastConfirmedMillis, in.NowMillis) {
		return refuse, false
	}
	// Validate the entire bounded snapshot, even records that cannot be selected.
	// A malformed row or duplicate identity must never leave a partial choice.
	for n, a := range in.Candidates {
		if a.Scope != in.Scope || !imageSelectionAsset(a) || !imageSelectionStamp(a.LastUsedMillis, in.NowMillis) {
			return refuse, false
		}
		for previous := 0; previous < n; previous++ {
			if in.Candidates[previous].ID == a.ID {
				return refuse, false
			}
		}
	}
	cooldown := int64(30 * 1000)
	if in.Profile.Sticker == "low" {
		cooldown = 180 * 1000
	}
	if in.LastConfirmedMillis != 0 && in.NowMillis-in.LastConfirmedMillis < cooldown {
		return refuse, false
	}
	id, err := strconv.ParseInt(in.SuggestedID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != in.SuggestedID {
		return refuse, false
	}
	var chosen storage.LibraryAsset
	for _, a := range in.Candidates {
		if a.ID != id || !a.Enabled || a.State != "ready" || a.Frames != 1 || (a.Format != "png" && a.Format != "jpeg") ||
			(a.LastUsedMillis != 0 && in.NowMillis-a.LastUsedMillis < 600*1000) {
			continue
		}
		chosen = a
	}
	if chosen.ID == 0 {
		return refuse, false
	}
	chosen.Tags = append([]string(nil), chosen.Tags...)
	return chosen, true
}

func imageSelectionStamp(stamp, now int64) bool {
	return stamp >= 0 && stamp <= imageSelectionMaxMillis && stamp <= now
}
func imageSelectionField(value string, max int) bool {
	return len(value) <= max && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
func imageSelectionScope(sc storage.Scope) bool {
	return imageSelectionField(sc.Account, 256) && imageSelectionField(sc.User, 256) && strings.TrimSpace(sc.Account) != "" && strings.TrimSpace(sc.User) != ""
}
func imageSelectionText(value string, max int, controls bool) bool {
	if !imageSelectionField(value, max) {
		return false
	}
	if controls {
		for _, r := range value {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}

// Mirrors current storage.Profile admission without exporting a storage helper
// or accepting an unresolved ID-only profile. The supplied snapshot is trusted;
// this function cannot prove that a structurally valid custom ID exists in Store.
func imageSelectionProfile(p storage.Profile) bool {
	if len(p.ID) < 1 || len(p.ID) > 64 || p.ID == "warm" || p.ID == "concise" || p.ID == "professional" {
		return false
	}
	for _, r := range p.ID {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	for _, field := range []struct {
		value string
		max   int
	}{{p.Name, 128}, {p.Description, 1024}, {p.Style, 512}, {p.Address, 128}} {
		if !imageSelectionField(field.value, field.max) || !validText(field.value) {
			return false
		}
	}
	return p.Humor >= 0 && p.Humor <= 3 && (p.Length == "short" || p.Length == "normal" || p.Length == "detailed") &&
		(p.Sticker == "off" || p.Sticker == "low" || p.Sticker == "normal")
}

// Matches persisted LibraryAsset admission (positive IDs), including valid rows
// that selection skips. Count canonical JSON bytes without allocating; HTML and
// Unicode separator escaping can exceed storage's 2048-byte tag JSON bound.
func imageSelectionAsset(a storage.LibraryAsset) bool {
	if !imageSelectionScope(a.Scope) || a.ID <= 0 || len(a.SHA256) != 64 || a.Bytes < 1 || a.Bytes > 4<<20 ||
		a.Width < 1 || a.Width > 8192 || a.Height < 1 || a.Height > 8192 || int64(a.Width)*int64(a.Height) > 16000000 ||
		a.Frames < 1 || a.Frames > 32 || a.Uses < 0 || a.LastUsedMillis < 0 || a.LastUsedMillis > imageSelectionMaxMillis || len(a.Tags) > 16 {
		return false
	}
	for _, c := range a.SHA256 {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	if a.Format != "gif" && (a.Format != "png" && a.Format != "jpeg" || a.Frames != 1) {
		return false
	}
	if a.State != "ready" && a.State != "deleting" || a.State == "deleting" && a.Enabled {
		return false
	}
	encodedTags := 2 // Opening and closing array brackets, including empty tags.
	for n, tag := range a.Tags {
		if tag == "" || !imageSelectionText(tag, 32, true) {
			return false
		}
		for previous := 0; previous < n; previous++ {
			if a.Tags[previous] == tag {
				return false
			}
		}
		if n != 0 {
			encodedTags++ // Comma between strings.
		}
		encodedTags += 2 + len(tag) // Quotes and original UTF8 bytes.
		for _, r := range tag {
			switch r {
			case '"', '\\':
				encodedTags++
			case '<', '>', '&':
				encodedTags += 5
			case '\u2028', '\u2029':
				encodedTags += 3
			}
		}
		if encodedTags > 2048 {
			return false
		}
	}
	return imageSelectionText(a.Emotion, 64, true) && imageSelectionText(a.Description, 1024, false)
}
