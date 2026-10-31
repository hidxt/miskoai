package web

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFailureDiagnosticAuthenticatedStatus(t *testing.T) {
	for _, tc := range []struct{ code, want string }{{"timeout", "timeout"}, {"balance", "balance"}, {"private-canary", "unknown"}, {"", ""}} {
		f := &trialStatusFixture{}
		json.Unmarshal([]byte(`{"Channel":{"LastModelFailure":"`+tc.code+`"}}`), &f.status)
		s := newTestServer(t, newApplication(f))
		if w := request(s, "GET", "/api/status", "", nil, "", ""); w.Code != 401 {
			t.Fatal("authentication missing")
		}
		cookie, _ := login(t, s)
		for _, path := range []string{"/api/status", "/api/diagnostics"} {
			w := request(s, "GET", path, "", cookie, "", "")
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"model_failure":"`+tc.want+`"`) || strings.Contains(w.Body.String(), "private-canary") {
				t.Fatalf("missing/unsafe field %s", w.Body.String())
			}
		}
	}
}
