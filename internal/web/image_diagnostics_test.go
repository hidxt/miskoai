package web

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestImageDecisionDiagnosticAuthenticatedStatus(t *testing.T) {
	for _, tc := range []struct{ code, want string }{{"plain_text", "plain_text"}, {"image_acked", "image_acked"}, {"private-token-canary", "unknown"}, {"", ""}} {
		f := &trialStatusFixture{}
		json.Unmarshal([]byte(`{"Channel":{"LastImageDecision":"`+tc.code+`"}}`), &f.status)
		s := newTestServer(t, newApplication(f))
		if w := request(s, "GET", "/api/status", "", nil, "", ""); w.Code != 401 {
			t.Fatal("status authentication missing", w.Code)
		}
		cookie, _ := login(t, s)
		w := request(s, "GET", "/api/status", "", cookie, "", "")
		var dto struct {
			Channel struct {
				Decision *string `json:"image_decision"`
			} `json:"channel"`
		}
		json.Unmarshal(w.Body.Bytes(), &dto)
		if w.Code != 200 || dto.Channel.Decision == nil || *dto.Channel.Decision != tc.want || strings.Contains(w.Body.String(), "private-token-canary") {
			t.Fatalf("status=%d %s", w.Code, w.Body.String())
		}
	}
}
