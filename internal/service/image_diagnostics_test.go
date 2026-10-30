package service

import (
	"encoding/json"
	"github.com/hidxt/miskoai/internal/agent"
	"testing"
)

func TestImageDecisionDiagnosticStatusRedactsAndClears(t *testing.T) {
	s := &Service{}
	for _, tc := range []struct{ code, want string }{{"plain_text", "plain_text"}, {"private-secret-canary", "unknown"}, {"", ""}} {
		var r agent.Result
		json.Unmarshal([]byte(`{"State":"sent","ImageDecision":"`+tc.code+`"}`), &r)
		s.recordResult(r)
		b, _ := json.Marshal(s.Snapshot())
		var v map[string]any
		json.Unmarshal(b, &v)
		if v["LastImageDecision"] != tc.want {
			t.Fatalf("status=%s want=%s", b, tc.want)
		}
	}
	if got := s.Snapshot(); got.Sent != 3 || got.Failed != 0 || got.LastCode != "" {
		t.Fatal("observation changed counters", got)
	}
}
