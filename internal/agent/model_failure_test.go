package agent

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/netx"
	"strings"
	"testing"
)

func assertFailureDiagnostic(t *testing.T, r Result, want string) {
	t.Helper()
	b, _ := json.Marshal(r)
	var fields map[string]any
	json.Unmarshal(b, &fields)
	if fields["ModelFailure"] != want || strings.Contains(string(b), "private-canary") {
		t.Fatalf("missing/unsafe failure category: %s", b)
	}
}
func TestFailureDiagnosticAgentOrdinaryAndJSON(t *testing.T) {
	for _, mode := range []string{"plain", "json"} {
		t.Run(mode, func(t *testing.T) {
			f := newReplyFixture(t)
			a := f.base
			if mode == "json" {
				a = f.a
				a.model = &adviceJSONModel{fakeModel: f.m}
			}
			f.m.err = &netx.HTTPError{Status: 402}
			in := incoming("failure-once", "synthetic")
			r, err := a.Handle(context.Background(), in)
			if err != nil || r.State != "sent" || r.Code != "model" || f.sender.calls != 1 || f.sender.images != 0 {
				t.Fatal("fallback behavior changed")
			}
			assertFailureDiagnostic(t, r, "balance")
			r, _ = a.Handle(context.Background(), in)
			assertFailureDiagnostic(t, r, "")
			if f.sender.calls != 1 {
				t.Fatal("duplicate replay")
			}
			f.m.err = nil
			r, err = a.Handle(context.Background(), incoming("success-new", "synthetic"))
			if err != nil {
				t.Fatal(err)
			}
			assertFailureDiagnostic(t, r, "")
			f.m.err = errors.New("private-canary")
			r, _ = a.Handle(context.Background(), incoming("unknown-new", "synthetic"))
			assertFailureDiagnostic(t, r, "unknown")
		})
	}
}
func TestFailureDiagnosticAgentAttachments(t *testing.T) {
	t.Run("vision", func(t *testing.T) {
		_, _, vision, _, send, image, a := imageFixture(t)
		vision.err = netx.WithFailure("timeout", context.DeadlineExceeded)
		r, err := a.Handle(context.Background(), imageInput("vision-failure", "synthetic"))
		if err != nil || send.calls != 1 || vision.calls != 1 {
			t.Fatal("vision behavior")
		}
		assertFailureDiagnostic(t, r, "timeout")
		vision.err = nil
		image.err = errors.New("private-canary")
		r, _ = a.Handle(context.Background(), imageInput("vision-cleanup", "synthetic"))
		assertFailureDiagnostic(t, r, "")
	})
	t.Run("document", func(t *testing.T) {
		_, m, _, send, d, a := documentFixture(t)
		m.err = &netx.HTTPError{Status: 401}
		r, err := a.Handle(context.Background(), documentInput("document-failure", "synthetic"))
		if err != nil || send.calls != 1 || m.calls != 1 {
			t.Fatal("document behavior")
		}
		assertFailureDiagnostic(t, r, "authentication")
		m.err = nil
		d.err = errors.New("private-canary")
		r, _ = a.Handle(context.Background(), documentInput("document-cleanup", "synthetic"))
		assertFailureDiagnostic(t, r, "")
	})
}
