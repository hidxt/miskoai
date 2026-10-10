package web

import (
	"context"
	"encoding/json"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/storage"
	"os"
	"strings"
	"testing"
	"time"
)

type managementFixture struct {
	managementBackend
	saved     config.Settings
	fact      storage.Fact
	calls     int
	confirmed int64
	deleted   int64
}

func (f *managementFixture) Status() core.Status { return core.Status{ChannelState: "unconfigured"} }
func (f *managementFixture) Settings() (config.Settings, config.Settings, map[string]bool) {
	d := config.DefaultSettings()
	e := d
	e.Model = "effective-model"
	return d, e, map[string]bool{"model": true}
}
func (f *managementFixture) SaveSettings(_ context.Context, s config.Settings) error {
	f.saved = s
	f.calls++
	return nil
}
func (f *managementFixture) AddFact(_ context.Context, v storage.Fact) (int64, error) {
	f.fact = v
	f.calls++
	return 9223372036854775807, nil
}
func (f *managementFixture) ConfirmCandidate(_ context.Context, id int64) (int64, error) {
	f.confirmed = id
	return id, nil
}
func (f *managementFixture) FactsPage(_ context.Context, q string, offset, limit int) (storage.FactPage, error) {
	return storage.FactPage{Items: []storage.Fact{{ID: 9223372036854775807, Content: "browser-safe", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}}, nil
}
func TestManagementAuthAndScope(t *testing.T) {
	f := &managementFixture{}
	s := newTestServer(t, newApplication(f))
	cookie, csrf := login(t, s)
	if w := request(s, "GET", "/api/status", "", nil, "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, tc := range []struct{ origin, csrf string }{{"", csrf}, {testOrigin, "wrong"}, {"http://hostile.test", csrf}} {
		if w := request(s, "POST", "/api/facts", `{"content":"synthetic","category":"","importance":0,"expires_at":null}`, cookie, tc.origin, tc.csrf); w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	if w := request(s, "POST", "/api/facts", `{"content":"synthetic","category":"","importance":0,"expires_at":null,"account":"hostile"}`, cookie, testOrigin, csrf); w.Code != 400 {
		t.Fatalf("scope body accepted: %d", w.Code)
	}
	if f.calls != 0 {
		t.Fatal("refused request mutated backend")
	}
	if w := request(s, "GET", "/api/status", "", cookie, "", ""); w.Code != 200 {
		t.Fatalf("authorized status %d", w.Code)
	}
}
func TestManagementSettingsNeverSecrets(t *testing.T) {
	f := &managementFixture{}
	s := newTestServer(t, newApplication(f))
	cookie, csrf := login(t, s)
	w := request(s, "GET", "/api/settings", "", cookie, "", "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var v map[string]json.RawMessage
	if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"desired", "effective", "overrides", "restart_required"} {
		if _, ok := v[key]; !ok {
			t.Fatal("missing", key)
		}
	}
	if string(v["restart_required"]) != "true" {
		t.Fatal("reload claim")
	}
	body, _ := json.Marshal(config.DefaultSettings())
	w = request(s, "POST", "/api/settings", string(body), cookie, testOrigin, csrf)
	if w.Code != 200 || f.saved.Model != "deepseek-flash" {
		t.Fatal(w.Code, f.saved)
	}
	if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "token") {
		t.Fatal("secrets exposed")
	}
}
func TestManagementRealSettingsPersistenceAndRedaction(t *testing.T) {
	c := privateCore(t, false)
	s := newTestServer(t, Application(c))
	cookie, csrf := login(t, s)
	settings := config.DefaultSettings()
	settings.Model = "synthetic-new-model"
	body, _ := json.Marshal(settings)
	if w := request(s, "POST", "/api/settings", string(body), cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/settings", "/api/status", "/api/diagnostics"} {
		w := request(s, "GET", path, "", cookie, "", "")
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		for _, canary := range []string{"synthetic-management-key-canary", "synthetic-search-key-canary", "synthetic-token", "fixture-account", "fixture-user", testPassword, "data_dir"} {
			if strings.Contains(w.Body.String(), canary) {
				t.Fatal("private data in", path)
			}
		}
	}
	desired, effective, _ := c.Settings()
	if desired.Model != "synthetic-new-model" || effective.Model != "deepseek-flash" {
		t.Fatal("desired/effective reload false claim")
	}
}

func TestManagementSettingsCanonicalListenRefusesAliases(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:08787", "127.0.0.1:+8787", "[::1]:08787", "[::1]:+8787"} {
		t.Run("backend/"+listen, func(t *testing.T) {
			f := &managementFixture{}
			s := newTestServer(t, newApplication(f))
			cookie, csrf := login(t, s)
			settings := config.DefaultSettings()
			settings.Listen = listen
			body, e := json.Marshal(settings)
			if e != nil {
				t.Fatal(e)
			}
			w := request(s, "POST", "/api/settings", string(body), cookie, testOrigin, csrf)
			if w.Code != 400 {
				t.Errorf("noncanonical listen accepted: status%d", w.Code)
			}
			if f.calls != 0 || f.saved != (config.Settings{}) {
				t.Error("refused listen reached mutation backend")
			}
		})
		t.Run("core/"+listen, func(t *testing.T) {
			c := privateCore(t, false)
			dir := os.Getenv("MISKOAI_DATA_DIR")
			s := newTestServer(t, Application(c))
			cookie, csrf := login(t, s)
			beforeDesired, beforeEffective, _ := c.Settings()
			beforePersisted, e := config.LoadSettings(dir)
			if e != nil {
				t.Fatal(e)
			}
			settings := beforeDesired
			settings.Listen = listen
			body, e := json.Marshal(settings)
			if e != nil {
				t.Fatal(e)
			}
			w := request(s, "POST", "/api/settings", string(body), cookie, testOrigin, csrf)
			if w.Code != 400 {
				t.Errorf("noncanonical listen accepted: status%d", w.Code)
			}
			desired, effective, _ := c.Settings()
			persisted, e := config.LoadSettings(dir)
			if e != nil {
				t.Fatal(e)
			}
			if desired != beforeDesired || persisted != beforePersisted {
				t.Error("refusal changed desired/persisted settings")
			}
			if effective != beforeEffective {
				t.Error("refusal changed effective settings")
			}
		})
	}
}

func TestManagementSettingsCanonicalListenPersistsDesired(t *testing.T) {
	c := privateCore(t, false)
	dir := os.Getenv("MISKOAI_DATA_DIR")
	s := newTestServer(t, Application(c))
	cookie, csrf := login(t, s)
	_, beforeEffective, _ := c.Settings()
	for _, listen := range []string{"127.0.0.1:18787", "[::1]:18787"} {
		t.Run(listen, func(t *testing.T) {
			settings := config.DefaultSettings()
			settings.Listen = listen
			body, e := json.Marshal(settings)
			if e != nil {
				t.Fatal(e)
			}
			w := request(s, "POST", "/api/settings", string(body), cookie, testOrigin, csrf)
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			desired, effective, _ := c.Settings()
			persisted, e := config.LoadSettings(dir)
			if e != nil {
				t.Fatal(e)
			}
			if desired != settings || persisted != settings {
				t.Error("canonical listen not persisted as desired")
			}
			if effective != beforeEffective {
				t.Error("settings falsely hot-reloaded")
			}
			w = request(s, "GET", "/api/settings", "", cookie, "", "")
			var got struct {
				Desired         config.Settings `json:"desired"`
				Effective       config.Settings `json:"effective"`
				RestartRequired bool            `json:"restart_required"`
			}
			if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil || w.Code != 200 || got.Desired != settings || got.Effective != beforeEffective || !got.RestartRequired {
				t.Fatal("incorrect restart contract", e, w.Code, w.Body.String())
			}
		})
	}
}

type oversizedPageFixture struct {
	managementBackend
	oversize bool
}

func (f oversizedPageFixture) FactsPage(context.Context, string, int, int) (storage.FactPage, error) {
	if f.oversize {
		return storage.FactPage{Items: []storage.Fact{{ID: 1, Content: strings.Repeat("x", (2<<20)+1)}}}, nil
	}
	rows := make([]storage.Fact, 16)
	for i := range rows {
		rows[i] = storage.Fact{ID: int64(i + 1), Content: strings.Repeat("\x01", 16384), Category: strings.Repeat("\x02", 256)}
	}
	return storage.FactPage{Items: rows}, nil
}
func TestManagementJSONResponseBudget(t *testing.T) {
	for _, oversize := range []bool{false, true} {
		s := newTestServer(t, newApplication(oversizedPageFixture{oversize: oversize}))
		cookie, _ := login(t, s)
		w := request(s, "GET", "/api/facts", "", cookie, "", "")
		if oversize {
			if w.Code != 500 || w.Body.String() != `{"error":"web_response"}` {
				t.Fatal(w.Code, w.Body.String())
			}
		} else {
			if w.Code != 200 || w.Body.Len() > 2<<20 || w.Body.Len() < 1<<20 {
				t.Fatal(w.Code, w.Body.Len())
			}
		}
	}
}
func TestManagementStrictInputs(t *testing.T) {
	f := &managementFixture{}
	s := newTestServer(t, newApplication(f))
	cookie, csrf := login(t, s)
	for _, body := range []string{`{}`, `{"content":"x","category":"","importance":1.0,"expires_at":null}`, `{"content":"x","category":"","importance":-1,"expires_at":null}`, `{"content":"x","category":"","importance":101,"expires_at":null}`, `{"content":"x","content":"y","category":"","importance":1,"expires_at":null}`, `{"content":"x","category":"","importance":1,"expires_at":0}`, `{"content":"x","category":"","importance":1,"expires_at":null} {}`, `{"content":"\ud800","category":"","importance":1,"expires_at":null}`, strings.Repeat(" ", 128<<10) + `{}`} {
		if w := request(s, "POST", "/api/facts", body, cookie, testOrigin, csrf); w.Code != 400 {
			t.Errorf("body refusal %d", w.Code)
		}
	}
	for _, q := range []string{"?limit=0", "?limit=17", "?offset=-1", "?offset=10001", "?limit=1&limit=2", "?account=other", "?limit=1.0", "?offset=01"} {
		if w := request(s, "GET", "/api/facts"+q, "", cookie, "", ""); w.Code != 400 {
			t.Errorf("%s %d", q, w.Code)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid input mutation")
	}
}
func (f *managementFixture) DeleteFact(_ context.Context, id int64) error { f.deleted = id; return nil }
func TestManagementDeleteRejectsUnexpectedBodies(t *testing.T) {
	f := &managementFixture{}
	s := newTestServer(t, newApplication(f))
	cookie, csrf := login(t, s)
	if w := request(s, "DELETE", "/api/facts?id=1", `{"account":"hostile"}`, cookie, testOrigin, csrf); w.Code != 400 || f.deleted != 0 {
		t.Fatal("unexpected body accepted", w.Code, f.deleted)
	}
	if w := request(s, "DELETE", "/api/facts?id=9223372036854775807", "", cookie, testOrigin, csrf); w.Code != 200 || f.deleted != 9223372036854775807 {
		t.Fatal(w.Code, f.deleted)
	}
	for _, q := range []string{"?id=9223372036854775808", "?id=0", "?id=01", "?id=1&id=2", "?id=1&user=hostile"} {
		if w := request(s, "DELETE", "/api/facts"+q, "", cookie, testOrigin, csrf); w.Code != 400 {
			t.Fatal(w.Code, q)
		}
	}
}
func TestManagementMutationObjectCaps(t *testing.T) {
	f := &managementFixture{}
	s := newTestServer(t, newApplication(f))
	cookie, csrf := login(t, s)
	for _, tc := range []struct {
		path string
		cap  int
	}{{"/api/settings", 16 << 10}, {"/api/profiles", 8 << 10}, {"/api/profiles/select", 1024}, {"/api/memory/confirm", 1024}, {"/api/memory/reject", 1024}, {"/api/memory/clear", 1024}, {"/api/channel/resume", 1024}, {"/api/export", 1024}, {"/api/backup", 1024}} {
		t.Run(tc.path, func(t *testing.T) {
			if w := request(s, "POST", tc.path, strings.Repeat(" ", tc.cap)+`{}`, cookie, testOrigin, csrf); w.Code != 400 {
				t.Fatal(w.Code)
			}
			if w := request(s, "POST", tc.path, `{"account":"hostile"}`, cookie, testOrigin, csrf); w.Code != 400 {
				t.Fatal(w.Code)
			}
		})
	}
}
