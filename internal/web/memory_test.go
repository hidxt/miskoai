package web

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type offlineModel struct{ calls *atomic.Int64 }

func (m offlineModel) Chat(context.Context, []provider.Message, int) (provider.Reply, error) {
	m.calls.Add(1)
	return provider.Reply{}, errors.New("network forbidden")
}

type offlineChannel struct{ calls *atomic.Int64 }

func (c offlineChannel) RawUpdates(context.Context, string) ([]byte, error) {
	c.calls.Add(1)
	return nil, errors.New("network forbidden")
}
func (c offlineChannel) SendText(context.Context, string, string, string, string) error {
	c.calls.Add(1)
	return errors.New("network forbidden")
}

type offlineSearcher struct{ calls *atomic.Int64 }

func (s offlineSearcher) Search(context.Context, string, int) ([]provider.SearchResult, error) {
	s.calls.Add(1)
	return nil, errors.New("network forbidden")
}
func privateCore(t *testing.T, seed bool) *core.Controller {
	t.Helper()
	for _, k := range []string{"MISKOAI_DATA_DIR", "MISKOAI_LISTEN", "MISKOAI_DEEPSEEK_URL", "MISKOAI_MODEL", "MISKOAI_VISION_MODEL", "MISKOAI_WEIXIN_URL", "MISKOAI_MAX_OUTPUT_TOKENS", "MISKOAI_CONTEXT_BYTES", "MISKOAI_CONTEXT_TOKENS", "DEEPSEEK_API_KEY", "OLLAMA_API_KEY", "MISKOAI_CREDENTIALS_FILE", "MISKOAI_ADMIN_PASSWORD"} {
		old, ok := os.LookupEnv(k)
		if e := os.Unsetenv(k); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			if ok {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
	dir := filepath.Join(t.TempDir(), "private")
	if e := privatefs.EnsureDir(dir); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MISKOAI_DATA_DIR", dir)
	t.Setenv("MISKOAI_ADMIN_PASSWORD", testPassword)
	t.Setenv("DEEPSEEK_API_KEY", "synthetic-management-key-canary")
	t.Setenv("OLLAMA_API_KEY", "synthetic-search-key-canary")
	auth, e := privatefs.Create(filepath.Join(dir, "weixin-auth.json"))
	if e != nil {
		t.Fatal(e)
	}
	_, e = auth.WriteString(`{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	ce := auth.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	cfg, e := config.Load()
	if e != nil {
		t.Fatal(e)
	}
	if seed {
		st, e := storage.Open(filepath.Join(dir, "miskoai.db"))
		if e != nil {
			t.Fatal(e)
		}
		ctx := context.Background()
		sc := storage.Scope{Account: "fixture-account", User: "fixture-user"}
		_, e = st.AddFact(ctx, storage.Scope{Account: "fixture-account", User: "other"}, storage.Fact{Content: "other-scope-canary", Source: "manual"})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = st.AddFact(ctx, storage.Scope{Account: "other-account", User: "fixture-user"}, storage.Fact{Content: "other-account-canary", Source: "manual"}); e != nil {
			t.Fatal(e)
		}
		if ok, e := st.ClaimMessage(ctx, sc, "source", "synthetic genuine quote"); e != nil || !ok {
			t.Fatal(e)
		}
		if e := st.SetMessageState(ctx, sc, "source", "sending"); e != nil {
			t.Fatal(e)
		}
		if e := st.CompleteMessage(ctx, sc, "source", "synthetic reply"); e != nil {
			t.Fatal(e)
		}
		if e := st.SaveDerived(ctx, sc, 0, storage.Summary{Text: "unconfirmed summary", Watermark: 1}, []storage.Candidate{{Content: "unconfirmed fact", MessageID: "source", Quote: "genuine quote"}, {Content: "second candidate", MessageID: "source", Quote: "quote"}}); e != nil {
			t.Fatal(e)
		}
		if e := st.Close(); e != nil {
			t.Fatal(e)
		}
	}
	calls := &atomic.Int64{}
	c, e := core.New(cfg, core.Dependencies{Model: offlineModel{calls}, Search: offlineSearcher{calls}, ChannelFactory: func(config.Authorization) (core.Channel, error) { return offlineChannel{calls}, nil }})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
		if calls.Load() != 0 {
			t.Error("unexpected provider/channel operation", calls.Load())
		}
	})
	return c
}
func TestCRUDAndCandidates(t *testing.T) {
	c := privateCore(t, true)
	s := newTestServer(t, Application(c))
	cookie, csrf := login(t, s)
	w := request(s, "POST", "/api/facts", `{"content":"<script>inert</script>","category":"manual","importance":80,"expires_at":null}`, cookie, testOrigin, csrf)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var added struct {
		ID string `json:"id"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &added); e != nil || added.ID == "" {
		t.Fatal(e, w.Body.String())
	}
	id := added.ID
	w = request(s, "GET", "/api/facts?limit=16", "", cookie, "", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "other-scope-canary") || strings.Contains(w.Body.String(), "other-account-canary") {
		t.Fatal(w.Code, w.Body.String())
	}
	var page struct {
		Items []struct {
			ID, Content, Source string
			Confidence          float64
		}
		NextOffset int  `json:"next_offset"`
		HasMore    bool `json:"has_more"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || len(page.Items) != 1 || page.Items[0].ID != id || page.Items[0].Content != "<script>inert</script>" || page.Items[0].Source != "explicit_user" || page.Items[0].Confidence != 1 {
		t.Fatal(e, w.Body.String())
	}
	w = request(s, "PATCH", "/api/facts", `{"id":"`+id+`","content":"corrected","category":"","importance":0,"expires_at":"2030-01-01T00:00:00Z"}`, cookie, testOrigin, csrf)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = request(s, "GET", "/api/memory", "", cookie, "", "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var raw struct{ Candidates []map[string]json.RawMessage }
	if e := json.Unmarshal(w.Body.Bytes(), &raw); e != nil || len(raw.Candidates) != 2 {
		t.Fatal(e, w.Body.String())
	}
	var candidateID string
	_ = json.Unmarshal(raw.Candidates[0]["id"], &candidateID)
	if string(raw.Candidates[0]["message_id"]) != `"source"` || !strings.Contains(string(raw.Candidates[0]["quote"]), "quote") {
		t.Fatal("lost provenance")
	}
	w = request(s, "POST", "/api/memory/confirm", `{"id":"`+candidateID+`"}`, cookie, testOrigin, csrf)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	candidateID = ""
	_ = json.Unmarshal(raw.Candidates[1]["id"], &candidateID)
	if w = request(s, "POST", "/api/memory/reject", `{"id":"`+candidateID+`"}`, cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request(s, "GET", "/api/history", "", cookie, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "synthetic reply") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request(s, "DELETE", "/api/facts?id="+id, "", cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request(s, "POST", "/api/memory/clear", `{"acknowledge":"clear_memory"}`, cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = request(s, "GET", "/api/facts", "", cookie, "", ""); w.Code != 200 || strings.Contains(w.Body.String(), "unconfirmed fact") {
		t.Fatal(w.Code)
	}
}
func TestBrowserSafeFactIDs(t *testing.T) {
	f := &managementFixture{}
	s := newTestServer(t, newApplication(f))
	cookie, csrf := login(t, s)
	w := request(s, "POST", "/api/facts", `{"content":"x","category":"","importance":1,"expires_at":null}`, cookie, testOrigin, csrf)
	if w.Code != 200 || w.Body.String() != `{"id":"9223372036854775807"}` {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, id := range []string{`9223372036854775807`, `"9223372036854775808"`, `"01"`, `"0"`, `"-1"`, `"+1"`, `"1.0"`, `null`} {
		if w := request(s, "POST", "/api/memory/confirm", `{"id":`+id+`}`, cookie, testOrigin, csrf); w.Code != 400 {
			t.Errorf("ID %s status %d", id, w.Code)
		}
	}
	w = request(s, "POST", "/api/memory/confirm", `{"id":"9223372036854775807"}`, cookie, testOrigin, csrf)
	if w.Code != 200 || f.confirmed != 9223372036854775807 || w.Body.String() != `{"id":"9223372036854775807"}` {
		t.Fatal(w.Code, w.Body.String(), f.confirmed)
	}
	w = request(s, "GET", "/api/facts", "", cookie, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"9223372036854775807"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestManagementProfiles(t *testing.T) {
	c := privateCore(t, false)
	s := newTestServer(t, Application(c))
	cookie, csrf := login(t, s)
	body := `{"id":"custom","name":"<img src=x onerror=alert(1)>","description":"synthetic","style":"warm","address":"friend","length":"normal","sticker":"off","humor":3}`
	if w := request(s, "POST", "/api/profiles", body, cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(s, "POST", "/api/profiles/select", `{"id":"custom"}`, cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w := request(s, "GET", "/api/profiles", "", cookie, "", "")
	var profiles struct {
		Custom     []storage.Profile
		BuiltinIDs []string `json:"builtin_ids"`
		Active     storage.Profile
	}
	if e := json.Unmarshal(w.Body.Bytes(), &profiles); e != nil || w.Code != 200 || len(profiles.Custom) != 1 || profiles.Active.ID != "custom" || profiles.Custom[0].Name != "<img src=x onerror=alert(1)>" || strings.Join(profiles.BuiltinIDs, ",") != "warm,concise,professional" {
		t.Fatal(e, w.Code, w.Body.String())
	}
	for _, bad := range []string{strings.Replace(body, `"humor":3`, `"humor":3.0`, 1), strings.Replace(body, `"humor":3`, `"humor":null`, 1), strings.Replace(body, `"sticker":"off"`, `"sticker":"unsupported"`, 1), strings.Replace(body, `"id":"custom"`, `"id":"warm"`, 1), body[:len(body)-1] + `,"user":"other"}`} {
		if w := request(s, "POST", "/api/profiles", bad, cookie, testOrigin, csrf); w.Code != 400 {
			t.Fatal(w.Code, bad)
		}
	}
	if w := request(s, "DELETE", "/api/profiles?id=custom", "", cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = request(s, "GET", "/api/profiles", "", cookie, "", "")
	if e := json.Unmarshal(w.Body.Bytes(), &profiles); e != nil || profiles.Active.ID != "warm" || len(profiles.Custom) != 0 {
		t.Fatal(e, w.Body.String())
	}
}

func TestFactExpiryRejectsInvalidRFC3339Offsets(t *testing.T) {
	f := &managementFixture{}
	s := newTestServer(t, newApplication(f))
	cookie, csrf := login(t, s)
	for _, stamp := range []string{"2030-01-01T00:00:00+24:00", "2030-01-01T00:00:00+01:60", "0000-01-01T00:00:00Z", "2030-01-01T00:00:00.1234567890Z", "2030-01-01T00:00:00,1Z"} {
		body := `{"content":"synthetic","category":"","importance":1,"expires_at":"` + stamp + `"}`
		if w := request(s, "POST", "/api/facts", body, cookie, testOrigin, csrf); w.Code != 400 {
			t.Errorf("invalid stamp %s accepted %d", stamp, w.Code)
		}
	}
}
func TestFactEscapingBodyBudget(t *testing.T) {
	f := &managementFixture{}
	s := newTestServer(t, newApplication(f))
	cookie, csrf := login(t, s)
	content := strings.Repeat("\x01", 16384)
	category := strings.Repeat("\x02", 256)
	body, _ := json.Marshal(map[string]any{"content": content, "category": category, "importance": 100, "expires_at": nil})
	if len(body) < 16<<10 || len(body) > 128<<10 {
		t.Fatal("fixture not worst-case escaped")
	}
	if w := request(s, "POST", "/api/facts", string(body), cookie, testOrigin, csrf); w.Code != 200 || f.fact.Content != content || f.fact.Category != category {
		t.Fatal(w.Code)
	}
}
func TestManagementOpaqueMemoryExport(t *testing.T) {
	c := privateCore(t, true)
	s := newTestServer(t, Application(c))
	cookie, csrf := login(t, s)
	if w := request(s, "POST", "/api/facts", `{"content":"exported scoped fact","category":"","importance":1,"expires_at":null}`, cookie, testOrigin, csrf); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w := request(s, "POST", "/api/export", `{}`, cookie, testOrigin, csrf)
	if w.Code != 200 || w.Header().Get("Content-Disposition") != `attachment; filename="miskoai-memory.json"` || !json.Valid(w.Body.Bytes()) {
		t.Fatal(w.Code, w.Header())
	}
	for _, excluded := range []string{"other-scope-canary", "other-account-canary", "fixture-account", "fixture-user", "synthetic-token", "synthetic-management-key-canary"} {
		if strings.Contains(w.Body.String(), excluded) {
			t.Fatal("export leaked scope or credentials")
		}
	}
	// The accepted version-1 artifact is opaque to browser code. Decode numbers
	// losslessly only in this Go fixture to verify its compatibility and scope.
	decoder := json.NewDecoder(strings.NewReader(w.Body.String()))
	decoder.UseNumber()
	var exported struct {
		Version int
		Facts   []struct {
			ID      json.Number
			Content string
		}
		Candidates []struct {
			ID        json.Number
			MessageID string `json:"message_id"`
			Quote     string
		}
	}
	if e := decoder.Decode(&exported); e != nil || exported.Version != 1 || len(exported.Facts) != 1 || exported.Facts[0].Content != "exported scoped fact" || len(exported.Candidates) != 2 || exported.Candidates[0].MessageID != "source" {
		t.Fatal(e, w.Body.String())
	}
	// Close must release the one-download allowance after the HTTP transfer.
	next, e := c.PrepareMemoryExport(context.Background())
	if e != nil {
		t.Fatal("download allowance retained", e)
	}
	if e := next.Close(); e != nil {
		t.Fatal(e)
	}
}
