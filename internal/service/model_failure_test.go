package service

import (
	"encoding/json"
	"github.com/hidxt/miskoai/internal/agent"
	"strings"
	"testing"
)

func TestFailureDiagnosticStatusResetAndCounters(t *testing.T) {
	s := &Service{}
	for _, tc := range []struct{ state, code, want string }{{"sent", "balance", "balance"}, {"sent", "private-canary", "unknown"}, {"duplicate", "", ""}, {"sent", "timeout", "timeout"}, {"sent", "", ""}} {
		var r agent.Result
		json.Unmarshal([]byte(`{"State":"`+tc.state+`","ModelFailure":"`+tc.code+`"}`), &r)
		s.recordResult(r)
		b, _ := json.Marshal(s.Snapshot())
		var fields map[string]any
		json.Unmarshal(b, &fields)
		if fields["LastModelFailure"] != tc.want || strings.Contains(string(b), "private-canary") {
			t.Fatalf("unsafe/stale diagnostic %s", b)
		}
	}
	if got := s.Snapshot(); got.Sent != 4 || got.Duplicate != 1 || got.Failed != 0 || got.Processed != 5 {
		t.Fatal("counter behavior changed")
	}
}
