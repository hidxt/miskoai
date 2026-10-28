package web

import (
	"encoding/json"
	"fmt"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTrialLoginRequiresOwnerUsername(t *testing.T) {
	for _, name := range []string{"", "wrong", "admin"} {
		t.Run(fmt.Sprintf("name_%q", name), func(t *testing.T) {
			s := newTestServer(t, nil)
			nonce := bootstrap(t, s)
			body, _ := json.Marshal(map[string]string{"username": name, "password": testPassword, "nonce": nonce})
			w := request(s, "POST", "/api/login", string(body), nil, testOrigin, "")
			want := 401
			if name == "admin" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("login=%d want %d", w.Code, want)
			}
			if name != "admin" {
				if w := request(s, "POST", "/api/login", string(body), nil, testOrigin, ""); w.Code != 401 {
					t.Fatal("nonce reuse", w.Code)
				}
			}
		})
	}
	s := newTestServer(t, nil)
	nonce := bootstrap(t, s)
	body, _ := json.Marshal(map[string]string{"password": testPassword, "nonce": nonce})
	if w := request(s, "POST", "/api/login", string(body), nil, testOrigin, ""); w.Code != 400 {
		t.Fatal("missing username accepted", w.Code)
	}
}

func TestTrialCustomUsername(t *testing.T) {
	o := Options{Listen: testHost, Username: "owner", Password: testPassword}
	s, e := New(o, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"admin", "owner"} {
		body, _ := json.Marshal(map[string]string{"username": name, "password": testPassword, "nonce": bootstrap(t, s)})
		w := request(s, "POST", "/api/login", string(body), nil, testOrigin, "")
		want := 401
		if name == "owner" {
			want = 200
		}
		if w.Code != want {
			t.Fatal(name, w.Code)
		}
	}
}

type trialStatusFixture struct {
	managementFixture
	status core.Status
}

func (f *trialStatusFixture) Status() core.Status { return f.status }
func TestTrialEventsPaging(t *testing.T) {
	f := &trialStatusFixture{}
	for i := 0; i < 140; i++ {
		f.status.Events = append(f.status.Events, core.Event{Kind: "created", At: time.Unix(int64(i+1), 0)})
	}
	s := newTestServer(t, newApplication(f))
	cookie, _ := login(t, s)
	for _, path := range []string{"/api/events", "/api/settings/defaults"} {
		if w := request(s, "GET", path, "", nil, "", ""); w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	for _, q := range []string{"?limit=33", "?limit=0", "?offset=-1", "?offset=01", "?offset=129", "?limit=1&limit=2", "?q=x", "?offset=", "?limit=+1"} {
		if w := request(s, "GET", "/api/events"+q, "", cookie, "", ""); w.Code != 400 {
			t.Fatal("bad query", q, w.Code)
		}
	}
	var page struct {
		Items []eventDTO `json:"items"`
		Next  int        `json:"next_offset"`
		More  bool       `json:"has_more"`
	}
	w := request(s, "GET", "/api/events?offset=0&limit=32", "", cookie, "", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 32 || page.Items[0].At != "1970-01-01T00:02:20Z" || page.Next != 32 || !page.More {
		t.Fatal("first page", w.Code, w.Body.String())
	}
	w = request(s, "GET", "/api/events?offset=96&limit=32", "", cookie, "", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 32 || page.Items[31].At != "1970-01-01T00:00:13Z" || page.Next != 128 || page.More {
		t.Fatal("last retained page", w.Code, w.Body.String())
	}
	w = request(s, "GET", "/api/events", "", cookie, "", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 20 || page.Next != 20 || !page.More {
		t.Fatal("default page", w.Code, w.Body.String())
	}
	w = request(s, "GET", "/api/events?offset=128&limit=32", "", cookie, "", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 0 || page.Next != 128 || page.More {
		t.Fatal("end page", w.Code, w.Body.String())
	}
}
func TestTrialDiagnosticCodes(t *testing.T) {
	for _, tc := range []struct {
		state, code, want string
		ambiguous         int64
	}{
		{"receiving", "", "receiving", 0}, {"backoff", "rate", "rate", 0}, {"backoff", "transport", "transport", 0}, {"receiving", "model", "model", 0}, {"receiving", "storage", "storage", 0}, {"authorization_expired", "transport", "auth-expired", 0}, {"restore_paused", "", "restore-paused", 0}, {"unconfigured", "", "unconfigured", 0}, {"receiving", "send", "send-ambiguous", 1}, {"private-canary", "private-canary", "unavailable", 0},
	} {
		s := core.Status{ChannelState: tc.state}
		s.Channel.LastCode = tc.code
		s.Channel.Ambiguous = tc.ambiguous
		b, _ := json.Marshal(statusResponse(s))
		var v map[string]any
		json.Unmarshal(b, &v)
		if v["diagnostic_code"] != tc.want {
			t.Fatal(tc, v["diagnostic_code"])
		}
	}
}
func TestTrialEventsEntry(t *testing.T) {
	s, e := New(Options{Listen: testHost, Password: testPassword}, nil, Assets())
	if e != nil {
		t.Fatal(e)
	}
	a := request(s, http.MethodGet, "/", "", nil, "", "")
	b := request(s, http.MethodGet, "/events", "", nil, "", "")
	if b.Code != 200 || a.Body.String() != b.Body.String() {
		t.Fatal("events entry", b.Code)
	}
}

func TestTrialDefaultsReadOnly(t *testing.T) {
	f := &trialStatusFixture{}
	s := newTestServer(t, newApplication(f))
	cookie, _ := login(t, s)
	w := request(s, "GET", "/api/settings/defaults", "", cookie, "", "")
	var got config.Settings
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got != config.DefaultSettings() {
		t.Fatal("defaults contract", w.Code, w.Body.String())
	}
	if f.calls != 0 {
		t.Fatal("read-only defaults mutated")
	}
}

func TestTrialOwnerUsernameBounds(t *testing.T) {
	for _, name := range []string{" owner", "owner ", " ", "owner\n", "owner\u0085", strings.Repeat("x", 65), string([]byte{255})} {
		if _, e := New(Options{Listen: testHost, Username: name, Password: testPassword}, nil, nil); e == nil {
			t.Fatal("unsafe username accepted")
		}
	}
	for _, name := range []string{"", "admin", strings.Repeat("x", 64), strings.Repeat("\u754c", 21)} {
		if _, e := New(Options{Listen: testHost, Username: name, Password: testPassword}, nil, nil); e != nil {
			t.Fatal("valid username rejected", e)
		}
	}
}
func TestTrialReadOnlyRoutesAndSafeEvents(t *testing.T) {
	f := &trialStatusFixture{}
	f.status.Events = []core.Event{{Kind: "private-kind-canary"}, {Kind: "created", At: time.Unix(1, 0)}}
	s := newTestServer(t, newApplication(f))
	cookie, csrf := login(t, s)
	for _, path := range []string{"/api/events", "/api/settings/defaults"} {
		if w := request(s, "POST", path, "{}", cookie, testOrigin, csrf); w.Code != 405 {
			t.Fatal("read-only route", path, w.Code)
		}
		if w := request(s, "GET", path+"?extra=x", "", cookie, "", ""); w.Code != 400 {
			t.Fatal("unexpected query", path, w.Code)
		}
	}
	w := request(s, "GET", "/api/events", "", cookie, "", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-kind-canary") {
		t.Fatal("unsafe event response", w.Code)
	}
	if f.calls != 0 {
		t.Fatal("read-only route mutated")
	}
}
