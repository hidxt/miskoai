package web

import (
	"github.com/hidxt/miskoai/internal/storage"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"
)

// Match RFC3339's fixed-width fields and offsets, with at most nanosecond
// precision so expiry is never silently truncated by time.Parse.
var expiryTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$`)

type factDTO struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Category   string  `json:"category"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
	Importance int     `json:"importance"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
	ExpiresAt  *string `json:"expires_at"`
}

func factResponse(f storage.Fact) factDTO {
	v := factDTO{ID: strconv.FormatInt(f.ID, 10), Content: f.Content, Category: f.Category, Source: f.Source, Confidence: f.Confidence, Importance: f.Importance, CreatedAt: webTime(f.CreatedAt), UpdatedAt: webTime(f.UpdatedAt)}
	if f.ExpiresAt != nil {
		t := webTime(*f.ExpiresAt)
		v.ExpiresAt = &t
	}
	return v
}
func webTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func readFact(r *http.Request, patch bool) (storage.Fact, error) {
	var f storage.Fact
	fields := []string{"content", "category", "importance", "expires_at"}
	if patch {
		fields = append(fields, "id")
	}
	o, e := requestObject(r, 128<<10, fields...)
	if e != nil {
		return f, e
	}
	if f.Content, e = stringValue(o["content"]); e != nil {
		return f, e
	}
	if f.Category, e = stringValue(o["category"]); e != nil {
		return f, e
	}
	if f.Importance, e = integerValue(o["importance"], 0, 100); e != nil {
		return f, e
	}
	if len(f.Content) > 16384 || len(f.Category) > 256 {
		return f, errManagementInput
	}
	if string(o["expires_at"]) != "null" {
		s, e := stringValue(o["expires_at"])
		if e != nil || len(s) > 64 || !expiryTimestamp.MatchString(s) {
			return f, errManagementInput
		}
		t, e := time.Parse(time.RFC3339Nano, s)
		if e != nil || t.Year() < 1 || t.Year() > 9999 {
			return f, errManagementInput
		}
		f.ExpiresAt = &t
	}
	if patch {
		s, e := stringValue(o["id"])
		if e != nil {
			return f, e
		}
		f.ID, e = decimalValue(s, 1, 9223372036854775807)
		if e != nil {
			return f, e
		}
	}
	return f, nil
}
func pageQuery(q url.Values, maxLimit int) (string, int, int, error) {
	query := q.Get("q")
	if len(query) > 1024 || !utf8.ValidString(query) {
		return "", 0, 0, errManagementInput
	}
	offset, limit := 0, maxLimit
	if q.Has("offset") {
		n, e := decimalValue(q.Get("offset"), 0, 10000)
		if e != nil {
			return "", 0, 0, e
		}
		offset = int(n)
	}
	if q.Has("limit") {
		n, e := decimalValue(q.Get("limit"), 1, int64(maxLimit))
		if e != nil {
			return "", 0, 0, e
		}
		limit = int(n)
	}
	return query, offset, limit, nil
}
func responseID(w http.ResponseWriter, id int64) {
	if id < 1 {
		safeError(w, 500, "web_response")
		return
	}
	respondJSON(w, struct {
		ID string `json:"id"`
	}{strconv.FormatInt(id, 10)})
}
func memoryRoute(w http.ResponseWriter, r *http.Request, b managementBackend, q url.Values) {
	switch r.URL.Path {
	case "/api/facts":
		switch r.Method {
		case "GET":
			query, offset, limit, e := pageQuery(q, 16)
			if e != nil {
				managementError(w, e)
				return
			}
			page, e := b.FactsPage(r.Context(), query, offset, limit)
			if e != nil {
				managementError(w, e)
				return
			}
			if len(page.Items) > limit {
				safeError(w, 500, "web_response")
				return
			}
			items := make([]factDTO, 0, len(page.Items))
			for _, f := range page.Items {
				if f.ID < 1 {
					safeError(w, 500, "web_response")
					return
				}
				items = append(items, factResponse(f))
			}
			respondJSON(w, struct {
				Items      []factDTO `json:"items"`
				NextOffset int       `json:"next_offset"`
				HasMore    bool      `json:"has_more"`
			}{items, page.NextOffset, page.HasMore})
		case "POST", "PATCH":
			f, e := readFact(r, r.Method == "PATCH")
			if e != nil {
				managementError(w, e)
				return
			}
			if r.Method == "PATCH" {
				mutationResult(w, b.UpdateFact(r.Context(), f))
				return
			}
			id, e := b.AddFact(r.Context(), f)
			if e != nil {
				managementError(w, e)
				return
			}
			responseID(w, id)
		case "DELETE":
			id, e := decimalValue(q.Get("id"), 1, 9223372036854775807)
			if e != nil {
				managementError(w, e)
				return
			}
			mutationResult(w, b.DeleteFact(r.Context(), id))
		}
	case "/api/history":
		query, offset, limit, e := pageQuery(q, 8)
		if e != nil {
			managementError(w, e)
			return
		}
		page, e := b.HistoryPage(r.Context(), query, offset, limit)
		if e != nil {
			managementError(w, e)
			return
		}
		if len(page.Items) > limit {
			safeError(w, 500, "web_response")
			return
		}
		type item struct {
			ID        string `json:"id"`
			Content   string `json:"content"`
			Reply     string `json:"reply"`
			CreatedAt string `json:"created_at"`
		}
		items := make([]item, 0, len(page.Items))
		for _, h := range page.Items {
			items = append(items, item{h.ID, h.Content, h.Reply, webTime(h.CreatedAt)})
		}
		respondJSON(w, struct {
			Items      []item `json:"items"`
			NextOffset int    `json:"next_offset"`
			HasMore    bool   `json:"has_more"`
		}{items, page.NextOffset, page.HasMore})
	case "/api/memory":
		derived, e := b.Derived(r.Context())
		if e != nil {
			managementError(w, e)
			return
		}
		candidates, e := b.Candidates(r.Context(), 8)
		if e != nil {
			managementError(w, e)
			return
		}
		if len(candidates) > 8 {
			safeError(w, 500, "web_response")
			return
		}
		type candidate struct {
			ID        string `json:"id"`
			Content   string `json:"content"`
			MessageID string `json:"message_id"`
			Quote     string `json:"quote"`
			CreatedAt string `json:"created_at"`
		}
		items := make([]candidate, 0, len(candidates))
		for _, c := range candidates {
			if c.ID < 1 {
				safeError(w, 500, "web_response")
				return
			}
			items = append(items, candidate{strconv.FormatInt(c.ID, 10), c.Content, c.MessageID, c.Quote, webTime(c.CreatedAt)})
		}
		type summary struct {
			Text      string `json:"text"`
			Watermark string `json:"watermark"`
			Revision  string `json:"revision"`
			UpdatedAt string `json:"updated_at"`
		}
		respondJSON(w, struct {
			Derived    summary     `json:"derived"`
			Candidates []candidate `json:"candidates"`
		}{summary{derived.Text, strconv.FormatInt(derived.Watermark, 10), strconv.FormatInt(derived.Revision, 10), webTime(derived.UpdatedAt)}, items})
	case "/api/memory/confirm", "/api/memory/reject":
		id, e := idObject(r)
		if e != nil {
			managementError(w, e)
			return
		}
		if r.URL.Path == "/api/memory/reject" {
			mutationResult(w, b.DeleteCandidate(r.Context(), id))
			return
		}
		id, e = b.ConfirmCandidate(r.Context(), id)
		if e != nil {
			managementError(w, e)
			return
		}
		responseID(w, id)
	case "/api/memory/clear":
		if e := acknowledge(r, "clear_memory"); e != nil {
			managementError(w, e)
			return
		}
		mutationResult(w, b.ClearMemory(r.Context()))
	}
}
