package web

import (
	"encoding/json"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/storage"
	"net"
	"net/http"
	"net/url"
	"strconv"
)

var settingsFields = []string{"listen", "deepseek_url", "model", "vision_model", "weixin_url", "max_output_tokens", "context_bytes", "context_tokens"}

func settingsRoute(w http.ResponseWriter, r *http.Request, b managementBackend, q url.Values) {
	switch r.URL.Path {
	case "/api/settings":
		if r.Method == "GET" {
			desired, effective, overrides := b.Settings()
			respondJSON(w, struct {
				Desired         config.Settings `json:"desired"`
				Effective       config.Settings `json:"effective"`
				Overrides       map[string]bool `json:"overrides"`
				RestartRequired bool            `json:"restart_required"`
			}{desired, effective, overrides, desired != effective})
			return
		}
		o, e := requestObject(r, 16<<10, settingsFields...)
		if e != nil {
			managementError(w, e)
			return
		}
		for _, key := range settingsFields[:5] {
			if _, e = stringValue(o[key]); e != nil {
				managementError(w, e)
				return
			}
		}
		for _, field := range []struct {
			key      string
			min, max int64
		}{{"max_output_tokens", 1, 4096}, {"context_bytes", 16384, 49152}, {"context_tokens", 4096, 65536}} {
			if _, e = integerValue(o[field.key], field.min, field.max); e != nil {
				managementError(w, e)
				return
			}
		}
		var s config.Settings
		if e = decodeTyped(o, &s); e != nil {
			managementError(w, e)
			return
		}
		// Persist only a spelling that the fixed loopback server can reopen.
		host, port, listenErr := net.SplitHostPort(s.Listen)
		p, portErr := strconv.Atoi(port)
		if listenErr != nil || portErr != nil || (host != "127.0.0.1" && host != "::1") || p < 1 || p > 65535 || strconv.Itoa(p) != port || net.JoinHostPort(host, port) != s.Listen {
			managementError(w, errManagementInput)
			return
		}
		mutationResult(w, b.SaveSettings(r.Context(), s))
	case "/api/profiles":
		if r.Method == "GET" {
			profiles, e := b.Profiles(r.Context())
			if e != nil {
				managementError(w, e)
				return
			}
			active, e := b.ActiveProfile(r.Context())
			if e != nil {
				managementError(w, e)
				return
			}
			if len(profiles) > 16 {
				safeError(w, 500, "web_response")
				return
			}
			if profiles == nil {
				profiles = []storage.Profile{}
			}
			respondJSON(w, struct {
				Custom     []storage.Profile `json:"custom"`
				BuiltinIDs []string          `json:"builtin_ids"`
				Active     storage.Profile   `json:"active"`
			}{profiles, []string{"warm", "concise", "professional"}, active})
			return
		}
		if r.Method == "DELETE" {
			id := q.Get("id")
			if !profileID(id) {
				managementError(w, errManagementInput)
				return
			}
			mutationResult(w, b.DeleteProfile(r.Context(), id))
			return
		}
		fields := []string{"id", "name", "description", "style", "address", "length", "sticker", "humor"}
		o, e := requestObject(r, 8<<10, fields...)
		if e != nil {
			managementError(w, e)
			return
		}
		for _, key := range fields[:7] {
			if _, e = stringValue(o[key]); e != nil {
				managementError(w, e)
				return
			}
		}
		if _, e = integerValue(o["humor"], 0, 3); e != nil {
			managementError(w, e)
			return
		}
		var p storage.Profile
		if e = decodeTyped(o, &p); e != nil {
			managementError(w, e)
			return
		}
		mutationResult(w, b.PutProfile(r.Context(), p))
	case "/api/profiles/select":
		o, e := requestObject(r, smallMutationBody, "id")
		if e != nil {
			managementError(w, e)
			return
		}
		id, e := stringValue(o["id"])
		if e != nil || !profileID(id) {
			managementError(w, errManagementInput)
			return
		}
		mutationResult(w, b.SelectProfile(r.Context(), id))
	}
}
func decodeTyped(o map[string]json.RawMessage, target any) error {
	data, e := json.Marshal(o)
	if e != nil || json.Unmarshal(data, target) != nil {
		return errManagementInput
	}
	return nil
}
func profileID(id string) bool {
	if len(id) < 1 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
