package web

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

type managementBackend interface {
	Status() core.Status
	Settings() (config.Settings, config.Settings, map[string]bool)
	SaveSettings(context.Context, config.Settings) error
	Profiles(context.Context) ([]storage.Profile, error)
	ActiveProfile(context.Context) (storage.Profile, error)
	PutProfile(context.Context, storage.Profile) error
	DeleteProfile(context.Context, string) error
	SelectProfile(context.Context, string) error
	FactsPage(context.Context, string, int, int) (storage.FactPage, error)
	HistoryPage(context.Context, string, int, int) (storage.HistoryPage, error)
	AddFact(context.Context, storage.Fact) (int64, error)
	UpdateFact(context.Context, storage.Fact) error
	DeleteFact(context.Context, int64) error
	Derived(context.Context) (storage.Summary, error)
	Candidates(context.Context, int) ([]storage.Candidate, error)
	ConfirmCandidate(context.Context, int64) (int64, error)
	DeleteCandidate(context.Context, int64) error
	ClearMemory(context.Context) error
	Restore(context.Context, io.Reader) (maintenance.RestoreResult, error)
	ResumeAfterReconciliation(context.Context) error
	prepareDownload(context.Context, bool) (managementDownload, error)
}
type managementDownload interface {
	io.ReadCloser
	metadata() (string, int64)
}
type coreBackend struct{ *core.Controller }
type coreDownload struct{ *core.Download }

func (d coreDownload) metadata() (string, int64) { return d.Filename, d.Size }
func (b coreBackend) prepareDownload(ctx context.Context, backup bool) (managementDownload, error) {
	var d *core.Download
	var e error
	if backup {
		d, e = b.PrepareBackup(ctx)
	} else {
		d, e = b.PrepareMemoryExport(ctx)
	}
	if e != nil {
		return nil, e
	}
	return coreDownload{d}, nil
}

// Application supplies management routes. Install it behind Server.Handler.
func Application(c *core.Controller) http.Handler {
	if c == nil {
		return newApplication(nil)
	}
	return newApplication(coreBackend{c})
}
func newApplication(b managementBackend) http.Handler {
	methods := map[string]string{"/api/status": "GET", "/api/settings": "GET POST", "/api/profiles": "GET POST DELETE", "/api/profiles/select": "POST", "/api/facts": "GET POST PATCH DELETE", "/api/history": "GET", "/api/memory": "GET", "/api/memory/confirm": "POST", "/api/memory/reject": "POST", "/api/memory/clear": "POST", "/api/export": "POST", "/api/backup": "POST", "/api/restore": "POST", "/api/channel/resume": "POST", "/api/diagnostics": "GET"}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b == nil {
			safeError(w, 503, "web_unavailable")
			return
		}
		allowed, ok := methods[r.URL.Path]
		if !ok {
			safeError(w, 404, "web_not_found")
			return
		}
		matched := false
		for _, method := range strings.Fields(allowed) {
			if method == r.Method {
				matched = true
			}
		}
		if !matched {
			w.Header().Set("Allow", strings.Join(strings.Fields(allowed), ", "))
			safeError(w, 405, "web_method")
			return
		}
		// DELETE identifies its target only in the query and accepts no body.
		// Read at most one byte for unknown-length bodies before any mutation.
		if r.Method == "DELETE" {
			if r.ContentLength > 0 {
				managementError(w, errManagementInput)
				return
			}
			if r.Body != nil {
				body, e := io.ReadAll(io.LimitReader(r.Body, 1))
				if e != nil || len(body) != 0 {
					managementError(w, errManagementInput)
					return
				}
			}
		}
		queryAllowed := []string{}
		if (r.URL.Path == "/api/facts" || r.URL.Path == "/api/history") && r.Method == "GET" {
			queryAllowed = []string{"q", "offset", "limit"}
		}
		if (r.URL.Path == "/api/facts" || r.URL.Path == "/api/profiles") && r.Method == "DELETE" {
			queryAllowed = []string{"id"}
		}
		query, e := strictQuery(r, queryAllowed...)
		if e != nil {
			managementError(w, e)
			return
		}
		switch r.URL.Path {
		case "/api/settings", "/api/profiles", "/api/profiles/select":
			settingsRoute(w, r, b, query)
		case "/api/facts", "/api/history", "/api/memory", "/api/memory/confirm", "/api/memory/reject", "/api/memory/clear":
			memoryRoute(w, r, b, query)
		default:
			systemRoute(w, r, b)
		}
	})
}

var errManagementInput = errors.New("web_input")

const maxJSONResponse = 2 << 20
const smallMutationBody = 1024

func managementError(w http.ResponseWriter, e error) {
	status, code := 500, "web_operation"
	var bodyLimit *http.MaxBytesError
	switch {
	case errors.As(e, &bodyLimit):
		status, code = 400, "web_input"
	case errors.Is(e, errManagementInput), errors.Is(e, storage.ErrInvalid), errors.Is(e, config.ErrSettings):
		status, code = 400, "web_input"
	case errors.Is(e, storage.ErrNotFound):
		status, code = 404, "web_not_found"
	case errors.Is(e, storage.ErrCapacity), errors.Is(e, storage.ErrStale), errors.Is(e, core.ErrBusy):
		status, code = 409, "web_busy"
	case errors.Is(e, core.ErrUnconfigured), errors.Is(e, core.ErrUnavailable):
		status, code = 503, "web_unavailable"
	case errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		status, code = 408, "web_timeout"
	case errors.Is(e, core.ErrMaintenance):
		status, code = 400, "web_maintenance"
	}
	safeError(w, status, code)
}
func respondJSON(w http.ResponseWriter, v any) {
	data, e := json.Marshal(v)
	if e != nil || len(data) > maxJSONResponse {
		safeError(w, 500, "web_response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
func mutationResult(w http.ResponseWriter, e error) {
	if e != nil {
		managementError(w, e)
		return
	}
	respondJSON(w, struct {
		OK bool `json:"ok"`
	}{true})
}
func strictQuery(r *http.Request, fields ...string) (url.Values, error) {
	if len(r.URL.RawQuery) > 8192 {
		return nil, errManagementInput
	}
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return nil, errManagementInput
	}
	for key, values := range q {
		allowed := false
		for _, field := range fields {
			if field == key {
				allowed = true
			}
		}
		if !allowed || len(values) != 1 || !utf8.ValidString(values[0]) {
			return nil, errManagementInput
		}
	}
	return q, nil
}
func requestObject(r *http.Request, capBytes int64, fields ...string) (map[string]json.RawMessage, error) {
	values := r.Header.Values("Content-Type")
	if len(values) != 1 {
		return nil, errManagementInput
	}
	kind, params, e := mime.ParseMediaType(values[0])
	if e != nil || kind != "application/json" {
		return nil, errManagementInput
	}
	for key, v := range params {
		if key != "charset" || v != "utf-8" {
			return nil, errManagementInput
		}
	}
	if r.ContentLength > capBytes {
		return nil, errManagementInput
	}
	object, e := strictRawRequestObject(r.Body, capBytes, fields...)
	if e != nil {
		return nil, errManagementInput
	}
	return object, nil
}
func stringValue(raw json.RawMessage) (string, error) {
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return "", errManagementInput
	}
	return s, nil
}
func decimalValue(s string, min, max int64) (int64, error) {
	if s == "" || len(s) > 19 || (len(s) > 1 && s[0] == '0') {
		return 0, errManagementInput
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errManagementInput
		}
	}
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < min || n > max {
		return 0, errManagementInput
	}
	return n, nil
}
func integerValue(raw json.RawMessage, min, max int64) (int, error) {
	n, e := decimalValue(string(raw), min, max)
	return int(n), e
}
func idObject(r *http.Request) (int64, error) {
	o, e := requestObject(r, smallMutationBody, "id")
	if e != nil {
		return 0, e
	}
	s, e := stringValue(o["id"])
	if e != nil {
		return 0, e
	}
	return decimalValue(s, 1, 9223372036854775807)
}
func acknowledge(r *http.Request, want string) error {
	o, e := requestObject(r, smallMutationBody, "acknowledge")
	if e != nil {
		return e
	}
	v, e := stringValue(o["acknowledge"])
	if e != nil || v != want {
		return errManagementInput
	}
	return nil
}
