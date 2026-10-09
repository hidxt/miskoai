package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
	"math"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type pollFunc func(context.Context, string) ([]byte, error)

func (f pollFunc) RawUpdates(c context.Context, s string) ([]byte, error) { return f(c, s) }

type handleFunc func(context.Context, agent.Incoming) (agent.Result, error)

func (f handleFunc) Handle(c context.Context, i agent.Incoming) (agent.Result, error) { return f(c, i) }

var scope = storage.Scope{Account: "account", User: "user"}

func storeFor(t *testing.T) (*storage.Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "private", "state.db")
	s, e := storage.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}
func mustService(t *testing.T, s *storage.Store, p Poller, h Handler) *Service {
	t.Helper()
	v, e := New(s, p, h, Options{Account: scope.Account, User: scope.User})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func body(id int, cur string) []byte {
	return []byte(fmt.Sprintf(`{"msgs":[{"message_id":%d,"from_user_id":"user","to_user_id":"account","message_type":1,"message_state":2,"context_token":"private-token","item_list":[{"type":1,"text_item":{"text":"synthetic text"}}]}],"get_updates_buf":%q}`, id, cur))
}
func idlePoll(c context.Context, _ string) ([]byte, error) { <-c.Done(); return nil, c.Err() }
func doneHandler(context.Context, agent.Incoming) (agent.Result, error) {
	return agent.Result{State: "sent"}, nil
}
func start(t *testing.T, s *Service) (context.CancelFunc, <-chan error) {
	t.Helper()
	c, x := context.WithCancel(context.Background())
	d := make(chan error, 1)
	go func() { d <- s.Run(c); close(d) }()
	t.Cleanup(func() {
		x()
		select {
		case <-d:
		case <-time.After(3 * time.Second):
			t.Error("Run did not join")
		}
	})
	return x, d
}
func eventually(t *testing.T, f func() bool) {
	t.Helper()
	limit := time.Now().Add(3 * time.Second)
	for time.Now().Before(limit) {
		if f() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

// Removing persistence before decoding would lose these exact private evidence bytes.
func TestMalformedFrameQuarantineKeepsCursor(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("{malformed"), body(-1, "new")} {
		t.Run(fmt.Sprintf("bytes%d", len(raw)), func(t *testing.T) {
			s, _ := storeFor(t)
			var polls atomic.Int32
			v := mustService(t, s, pollFunc(func(context.Context, string) ([]byte, error) { polls.Add(1); return raw, nil }), handleFunc(doneHandler))
			cancel, _ := start(t, v)
			eventually(t, func() bool { return v.Snapshot().Channel == "quarantined" })
			f, e := s.PendingPoll(context.Background(), scope)
			if e != nil || f == nil || f.State != "quarantined" || string(f.Body) != string(raw) {
				t.Fatalf("evidence: %v %v", f, e)
			}
			cur, e := s.Cursor(context.Background(), scope)
			if e != nil || cur != "" || polls.Load() != 1 {
				t.Fatalf("cursor/polls: %q %d %v", cur, polls.Load(), e)
			}
			cancel()
		})
	}
}

// Ignored envelopes may contain unusable IDs/text but cannot reach the handler.
func TestNormalizeAuthorizationAndIDs(t *testing.T) {
	raw := []byte(`{"msgs":[{"from_user_id":"stranger","message_id":{},"item_list":[{"type":1,"text_item":{"text":42}}]},{"from_user_id":"user","group_id":"group","message_id":{}},{"from_user_id":"user","message_type":2,"message_id":{}},{"from_user_id":"user","message_type":1,"message_state":1,"message_id":{}},{"from_user_id":"user","to_user_id":"other","message_type":1,"message_id":{}},{"from_user_id":"user","message_type":1,"message_state":0,"context_token":"token","item_list":[{"type":1,"msg_id":"opaque-id","text_item":{"text":"one"}},{"type":1,"text_item":{"text":"two"}}]}],"get_updates_buf":"next"}`)
	entries, cur, e := normalize(raw, scope, time.Unix(100, 0))
	if e != nil || cur != "next" || len(entries) != 1 {
		t.Fatalf("normalize: %v %q %v", entries, cur, e)
	}
	x := entries[0]
	if x.MessageID != "opaque-id" || x.Text != "one\ntwo" || !x.ReceivedAt.Equal(time.Unix(100, 0)) {
		t.Fatalf("fields: %+v", x)
	}
}
func TestNormalizeBoundsAndLosslessID(t *testing.T) {
	for _, id := range []string{"18446744073709551615", "0"} {
		raw := strings.Replace(string(body(1, "")), `"message_id":1`, `"message_id":`+id, 1)
		e, _, err := normalize([]byte(raw), scope, time.Now())
		if err != nil || len(e) != 1 || e[0].MessageID != id {
			t.Fatalf("lossless %s %v", id, err)
		}
	}
	for _, id := range []string{"-1", "1.5", "18446744073709551616", "1e3"} {
		raw := strings.Replace(string(body(1, "")), `"message_id":1`, `"message_id":`+id, 1)
		if _, _, e := normalize([]byte(raw), scope, time.Now()); e == nil {
			t.Fatalf("accepted %s", id)
		}
	}
	raw := strings.Replace(string(body(1, "")), "synthetic text", strings.Repeat("x", 16385), 1)
	if _, _, e := normalize([]byte(raw), scope, time.Now()); e == nil {
		t.Fatal("accepted overlong text")
	}
}

func TestEmptyPollDoesNotSpin(t *testing.T) {
	s, _ := storeFor(t)
	var polls atomic.Int32
	v := mustService(t, s, pollFunc(func(context.Context, string) ([]byte, error) {
		polls.Add(1)
		return []byte(`{"msgs":[],"get_updates_buf":""}`), nil
	}), handleFunc(doneHandler))
	waiting := make(chan time.Duration, 1)
	v.wait = func(c context.Context, d time.Duration) bool { waiting <- d; <-c.Done(); return false }
	cancel, _ := start(t, v)
	select {
	case d := <-waiting:
		if d < time.Second {
			t.Fatal(d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no wait")
	}
	if polls.Load() != 1 {
		t.Fatal("poll spin")
	}
	cancel()
}
func TestPendingFrameResolvedBeforePollAndEmptyCursorRetained(t *testing.T) {
	s, _ := storeFor(t)
	id, e := s.RecordPoll(context.Background(), scope, "", body(1, "old"))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ResolvePoll(context.Background(), scope, id, "old", nil); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RecordPoll(context.Background(), scope, "old", body(2, "")); e != nil {
		t.Fatal(e)
	}
	called := make(chan string, 1)
	v := mustService(t, s, pollFunc(func(c context.Context, cur string) ([]byte, error) { called <- cur; return idlePoll(c, cur) }), handleFunc(doneHandler))
	cancel, _ := start(t, v)
	select {
	case c := <-called:
		if c != "old" {
			t.Fatalf("cursor=%q", c)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending not resolved")
	}
	eventually(t, func() bool { return v.Snapshot().Sent == 1 })
	cancel()
}

func TestAuthExpiryCancelsHandlerAndOuterLifetime(t *testing.T) {
	for _, err := range []error{weixin.ErrAuthExpired, weixin.ErrUnauthorized} {
		t.Run(err.Error(), func(t *testing.T) {
			s, _ := storeFor(t)
			entered := make(chan struct{})
			ended := make(chan struct{})
			var calls atomic.Int32
			v := mustService(t, s, pollFunc(func(c context.Context, cur string) ([]byte, error) {
				if calls.Add(1) == 1 {
					return body(1, "next"), nil
				}
				<-entered
				return nil, err
			}), handleFunc(func(c context.Context, i agent.Incoming) (agent.Result, error) {
				close(entered)
				<-c.Done()
				close(ended)
				return agent.Result{State: "sent"}, nil
			}))
			cancel, d := start(t, v)
			select {
			case <-ended:
			case <-time.After(3 * time.Second):
				t.Fatal("handler not canceled")
			}
			select {
			case <-d:
				t.Fatal("outer lifetime ended")
			default:
			}
			in, e := s.PendingInbox(context.Background(), scope, 32)
			if e != nil || len(in) != 1 {
				t.Fatalf("canceled inbox lost %v %v", in, e)
			}
			if calls.Load() != 2 {
				t.Fatal("repoll after auth")
			}
			cancel()
		})
	}
}
func TestHandlerStorageAndUnknownStateRetainInbox(t *testing.T) {
	for _, r := range []agent.Result{{State: "failed", Code: "storage"}, {State: "failed", Code: "capacity"}, {State: "private-state", Code: "private-token"}} {
		t.Run(r.State+r.Code, func(t *testing.T) {
			s, _ := storeFor(t)
			id, e := s.RecordPoll(context.Background(), scope, "", body(1, "next"))
			if e != nil {
				t.Fatal(e)
			}
			v := mustService(t, s, pollFunc(idlePoll), handleFunc(func(context.Context, agent.Incoming) (agent.Result, error) { return r, errors.New("private-error") }))
			cancel, _ := start(t, v)
			eventually(t, func() bool { return v.Snapshot().Channel == "paused" })
			in, e := s.PendingInbox(context.Background(), scope, 32)
			if e != nil || len(in) != 1 {
				t.Fatalf("work lost frame%d: %v %v", id, in, e)
			}
			b, _ := json.Marshal(v.Snapshot())
			if strings.Contains(string(b), "private") {
				t.Fatalf("unsafe status %s", b)
			}
			cancel()
		})
	}
}
func TestStatusContainsNoPrivateData(t *testing.T) {
	s, _ := storeFor(t)
	v := mustService(t, s, pollFunc(idlePoll), handleFunc(doneHandler))
	v.recordResult(agent.Result{State: "sent", Code: "private-secret", Usage: provider.Usage{PromptTokens: math.MaxInt, CompletionTokens: -4, TotalTokens: math.MaxInt}})
	v.recordResult(agent.Result{State: "ambiguous", Usage: provider.Usage{PromptTokens: 10, TotalTokens: 10}})
	got := v.Snapshot()
	if got.Usage.PromptTokens != math.MaxInt || got.Usage.CompletionTokens != 0 || got.Usage.TotalTokens != math.MaxInt || got.Sent != 1 || got.Ambiguous != 1 {
		t.Fatalf("status: %+v", got)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), "private") || strings.Contains(string(b), "account") || strings.Contains(string(b), "user") {
		t.Fatalf("leak %s", b)
	}
}

// Original duplicate/Unicode array keys cannot hide an overflowing receive batch.
func TestOriginalEnvelopeBoundsAndAuthStatus(t *testing.T) {
	ignored := `{"from_user_id":"other","message_type":1,"message_id":{}}`
	many := strings.TrimSuffix(strings.Repeat(ignored+",", 256), ",")
	for _, raw := range []string{`{"msgs":[` + many + `],"MSGS":[` + ignored + `]}`, `{"msgs":[` + many + `],"msgſ":[` + ignored + `]}`, `{"msgs":{},"ret":0}`, `{"msgs":[],"get_updates_buf":"` + strings.Repeat("x", 16385) + `"}`} {
		if _, _, e := normalize([]byte(raw), scope, time.Now()); e == nil {
			t.Fatal("accepted original bound/shape violation")
		}
	}
	for _, raw := range []string{`{"ret":-14,"ret":0,"msgs":[]}`, `{"ret":5,"errcode":-14,"msgs":[]}`, `{"msgs":{},"errcode":-14}`} {
		if _, _, e := normalize([]byte(raw), scope, time.Now()); !errors.Is(e, weixin.ErrAuthExpired) {
			t.Fatalf("auth erased: %s %v", raw, e)
		}
	}
}
func TestDuplicateSelectorsUseLastField(t *testing.T) {
	for _, raw := range []string{
		`{"msgs":[{"from_user_id":"user","FROM_USER_ID":"other","message_type":1,"message_id":{}}]}`,
		`{"msgs":[{"from_user_id":"other","from_user_id":"user","message_type":1,"message_state":1,"message_state":2,"context_token":"token","item_list":[{"type":1,"msg_id":"last","text_item":{"text":"allowed"}}]}]}`,
	} {
		e, _, err := normalize([]byte(raw), scope, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, `"last"`) {
			if len(e) != 1 || e[0].MessageID != "last" {
				t.Fatal(e)
			}
		} else if len(e) != 0 {
			t.Fatal(e)
		}
	}
}

// Cursor/work admission must roll back and retry the SAME persisted frame when
// the worker releases capacity; a second remote request would lose ordering.
func TestInboxCapacityRetriesSameFrameLocally(t *testing.T) {
	s, _ := storeFor(t)
	ctx := context.Background()
	for batch := 0; batch < 4; batch++ {
		entries := make([]storage.InboxEntry, 256)
		for j := range entries {
			entries[j] = storage.InboxEntry{Scope: scope, MessageID: fmt.Sprintf("old%d", batch*256+j), Text: "old", ContextToken: "old-token", ReceivedAt: time.Now()}
		}
		cur, e := s.Cursor(ctx, scope)
		if e != nil {
			t.Fatal(e)
		}
		id, e := s.RecordPoll(ctx, scope, cur, []byte(`{"msgs":[]}`))
		if e != nil {
			t.Fatal(e)
		}
		if e = s.ResolvePoll(ctx, scope, id, fmt.Sprintf("c%d", batch), entries); e != nil {
			t.Fatal(e)
		}
	}
	var polls atomic.Int32
	hold := make(chan struct{})
	entered := make(chan struct{}, 1)
	waited := make(chan struct{}, 1)
	release := make(chan struct{})
	v := mustService(t, s, pollFunc(func(c context.Context, cur string) ([]byte, error) {
		if polls.Add(1) == 1 {
			return body(9999, "next"), nil
		}
		return idlePoll(c, cur)
	}), handleFunc(func(c context.Context, i agent.Incoming) (agent.Result, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-hold:
		case <-c.Done():
		}
		return agent.Result{State: "sent"}, nil
	}))
	v.wait = func(c context.Context, d time.Duration) bool {
		select {
		case waited <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return true
		case <-c.Done():
			return false
		}
	}
	cancel, _ := start(t, v)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker absent")
	}
	select {
	case <-waited:
	case <-time.After(3 * time.Second):
		t.Fatal("capacity did not wait")
	}
	f, e := s.PendingPoll(ctx, scope)
	if e != nil || f == nil || f.State != "pending" || string(f.Body) != string(body(9999, "next")) {
		t.Fatalf("frame lost %v %v", f, e)
	}
	cur, e := s.Cursor(ctx, scope)
	if e != nil || cur != "c3" || polls.Load() != 1 {
		t.Fatalf("cursor moved/repolled %q %d %v", cur, polls.Load(), e)
	}
	close(hold)
	eventually(t, func() bool { return v.Snapshot().Sent >= 1 })
	close(release)
	eventually(t, func() bool { cur, e := s.Cursor(ctx, scope); return e == nil && cur == "next" })
	if polls.Load() > 2 {
		t.Fatal("repolled frame")
	}
	cancel()
}

type modelDouble struct{ calls atomic.Int32 }

func (m *modelDouble) Chat(context.Context, []provider.Message, int) (provider.Reply, error) {
	m.calls.Add(1)
	return provider.Reply{Text: "synthetic answer", FinishReason: "stop", Usage: provider.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}}, nil
}

type senderDouble struct {
	calls     atomic.Int32
	ambiguous bool
}

func (s *senderDouble) SendText(_ context.Context, to, token, id, text string) error {
	s.calls.Add(1)
	if to != "user" || token != "private-token" || !strings.HasPrefix(id, "miskoai-") || text != "synthetic answer" {
		return errors.New("incorrect send boundary")
	}
	if s.ambiguous {
		return weixin.ErrOutcomeUnknown
	}
	return nil
}
func TestRealAgentRestartNeverReplaysSentAmbiguousOrPriorClaim(t *testing.T) {
	for _, state := range []string{"sent", "ambiguous", "processing", "sending", "failed"} {
		t.Run(state, func(t *testing.T) {
			s, path := storeFor(t)
			ctx := context.Background()
			m := &modelDouble{}
			send := &senderDouble{ambiguous: state == "ambiguous"}
			h, e := agent.New(s, m, nil, send, agent.Options{Account: "account", User: "user"})
			if e != nil {
				t.Fatal(e)
			}
			// Restart begins with a pending inbox and a durable prior claim. Its claim
			// state is terminal for automatic replay even before a successful send.
			if state == "sent" || state == "ambiguous" {
				r, e := h.Handle(ctx, agent.Incoming{Account: "account", User: "user", ID: "1", Text: "synthetic text", ContextToken: "private-token"})
				if r.State != state {
					t.Fatalf("initial state %v %v", r, e)
				}
			} else {
				if _, e = s.ClaimMessage(ctx, scope, "1", "synthetic text"); e != nil {
					t.Fatal(e)
				}
				if state != "processing" {
					if e = s.SetMessageState(ctx, scope, "1", state); e != nil {
						t.Fatal(e)
					}
				}
			}
			// Inbox reference originally committed before the handler claim is reproduced
			// with ID2, then separately claimed so restart's initial worker hits duplicate.
			id, e := s.RecordPoll(ctx, scope, "", body(2, "saved"))
			if e != nil {
				t.Fatal(e)
			}
			entries, _, e := normalize(body(2, "saved"), scope, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			if e = s.ResolvePoll(ctx, scope, id, "saved", entries); e != nil {
				t.Fatal(e)
			}
			if _, e = s.ClaimMessage(ctx, scope, "2", "synthetic text"); e != nil {
				t.Fatal(e)
			}
			beforeModel, beforeSend := m.calls.Load(), send.calls.Load()
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			s, e = storage.Open(path)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { s.Close() })
			h, e = agent.New(s, m, nil, send, agent.Options{Account: "account", User: "user"})
			if e != nil {
				t.Fatal(e)
			}
			var polls atomic.Int32
			v := mustService(t, s, pollFunc(func(c context.Context, cur string) ([]byte, error) {
				if cur != "saved" {
					t.Errorf("restart cursor %q", cur)
				}
				if polls.Add(1) == 1 {
					return body(1, "saved"), nil
				}
				return idlePoll(c, cur)
			}), h)
			cancel, d := start(t, v)
			eventually(t, func() bool { return v.Snapshot().Duplicate == 1 && v.Snapshot().Received == 1 })
			cancel()
			select {
			case <-d:
			case <-time.After(3 * time.Second):
				t.Fatal("join")
			}
			if m.calls.Load() != beforeModel || send.calls.Load() != beforeSend {
				t.Fatal("replayed side effects")
			}
			in, e := s.PendingInbox(ctx, scope, 32)
			if e != nil || len(in) != 0 {
				t.Fatalf("inbox %v %v", in, e)
			}
		})
	}
}
func TestSingleStartAndCancellationJoinBothWorkers(t *testing.T) {
	s, _ := storeFor(t)
	exited := make(chan struct{})
	entered := make(chan struct{})
	v := mustService(t, s, pollFunc(func(c context.Context, _ string) ([]byte, error) {
		close(entered)
		<-c.Done()
		close(exited)
		return nil, c.Err()
	}), handleFunc(doneHandler))
	cancel, d := start(t, v)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("poller not entered")
	}
	if e := v.Run(context.Background()); e == nil {
		t.Fatal("second start accepted")
	}
	cancel()
	select {
	case <-d:
	case <-time.After(3 * time.Second):
		t.Fatal("did not join")
	}
	select {
	case <-exited:
	default:
		t.Fatal("poller not joined")
	}
}

func TestPollBackoffIsBoundedAndCancellationStopsRetry(t *testing.T) {
	s, _ := storeFor(t)
	var calls atomic.Int32
	v := mustService(t, s, pollFunc(func(context.Context, string) ([]byte, error) { calls.Add(1); return nil, weixin.ErrTransport }), handleFunc(doneHandler))
	ctx, cancel := context.WithCancel(context.Background())
	delays := []time.Duration{}
	v.wait = func(c context.Context, d time.Duration) bool {
		delays = append(delays, d)
		if len(delays) == 8 {
			cancel()
			return false
		}
		return true
	}
	if e := v.Run(ctx); e != nil {
		t.Fatal(e)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	if len(delays) != len(want) || calls.Load() != 8 {
		t.Fatalf("retries %v %d", delays, calls.Load())
	}
	for i, d := range want {
		if delays[i] != d {
			t.Fatal(delays)
		}
	}
}
func TestNormalizeAllowedTextMissingContextAndInvalidTimestamp(t *testing.T) {
	for _, raw := range []string{strings.Replace(string(body(1, "")), `"context_token":"private-token"`, `"context_token":""`, 1), strings.Replace(string(body(1, "")), "synthetic text", "", 1)} {
		if _, _, e := normalize([]byte(raw), scope, time.Unix(100, 0)); e == nil {
			t.Fatal("accepted invalid authorized text")
		}
	}
	for _, timestamp := range []string{`-1`, `0`, `"bad"`, `9223372036854775808`, `253402300800000`} {
		raw := strings.Replace(string(body(1, "")), `"message_id":1`, `"message_id":1,"create_time_ms":`+timestamp, 1)
		entries, _, e := normalize([]byte(raw), scope, time.Unix(100, 0))
		if e != nil || len(entries) != 1 || !entries[0].ReceivedAt.Equal(time.Unix(100, 0)) {
			t.Fatalf("timestamp %s %v %v", timestamp, entries, e)
		}
	}
}
func TestUnsupportedMediaDoesNotReachHandler(t *testing.T) {
	s, _ := storeFor(t)
	var calls atomic.Int32
	raw := []byte(`{"msgs":[{"from_user_id":"user","message_type":1,"message_state":2,"context_token":"token","item_list":[{"type":2,"image_item":{}}]}],"get_updates_buf":"media"}`)
	var polls atomic.Int32
	v := mustService(t, s, pollFunc(func(c context.Context, cur string) ([]byte, error) {
		if polls.Add(1) == 1 {
			return raw, nil
		}
		return idlePoll(c, cur)
	}), handleFunc(func(context.Context, agent.Incoming) (agent.Result, error) {
		calls.Add(1)
		return agent.Result{State: "sent"}, nil
	}))
	cancel, _ := start(t, v)
	eventually(t, func() bool { cur, e := s.Cursor(context.Background(), scope); return e == nil && cur == "media" })
	if calls.Load() != 0 || v.Snapshot().Received != 0 {
		t.Fatal("unsupported media processed")
	}
	cancel()
}
func TestClosedStoragePausesWithoutPolling(t *testing.T) {
	s, _ := storeFor(t)
	s.Close()
	var calls atomic.Int32
	v := mustService(t, s, pollFunc(func(c context.Context, _ string) ([]byte, error) { calls.Add(1); return nil, weixin.ErrTransport }), handleFunc(doneHandler))
	cancel, _ := start(t, v)
	eventually(t, func() bool { return v.Snapshot().Channel == "paused" })
	if calls.Load() != 0 || v.Snapshot().LastCode != "storage" {
		t.Fatal("unsafe closed store")
	}
	cancel()
}

func TestRealAgentFirstDeliveryAndRestartedPollDuplicate(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(fmt.Sprintf("ambiguous%t", ambiguous), func(t *testing.T) {
			s, path := storeFor(t)
			m := &modelDouble{}
			send := &senderDouble{ambiguous: ambiguous}
			h, e := agent.New(s, m, nil, send, agent.Options{Account: "account", User: "user"})
			if e != nil {
				t.Fatal(e)
			}
			poll := func() Poller {
				var calls atomic.Int32
				return pollFunc(func(c context.Context, cur string) ([]byte, error) {
					if calls.Add(1) == 1 {
						return body(1, "saved"), nil
					}
					return idlePoll(c, cur)
				})
			}
			v := mustService(t, s, poll(), h)
			cancel, d := start(t, v)
			eventually(t, func() bool { return v.Snapshot().Processed == 1 })
			cancel()
			<-d
			if v.Snapshot().Usage.TotalTokens != 5 || m.calls.Load() != 1 || send.calls.Load() != 1 {
				t.Fatal("first delivery did not complete expected boundary")
			}
			if ambiguous {
				if v.Snapshot().Ambiguous != 1 {
					t.Fatal("ambiguity lost")
				}
			} else if v.Snapshot().Sent != 1 {
				t.Fatal("send lost")
			}
			s.Close()
			s, e = storage.Open(path)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { s.Close() })
			h, e = agent.New(s, m, nil, send, agent.Options{Account: "account", User: "user"})
			if e != nil {
				t.Fatal(e)
			}
			v = mustService(t, s, poll(), h)
			cancel, d = start(t, v)
			eventually(t, func() bool { return v.Snapshot().Received == 1 })
			cancel()
			<-d
			if m.calls.Load() != 1 || send.calls.Load() != 1 || v.Snapshot().Processed != 0 || v.Snapshot().Duplicate != 0 {
				t.Fatal("replayed a receive-side suppressed claim or invented handler duplicate")
			}
		})
	}
}
func TestRawAuthExpiryPreservesEvidenceAndStopsPolls(t *testing.T) {
	s, _ := storeFor(t)
	var calls atomic.Int32
	raw := []byte(`{"ret":-14,"errmsg":"private-error","msgs":[]}`)
	v := mustService(t, s, pollFunc(func(context.Context, string) ([]byte, error) { calls.Add(1); return raw, nil }), handleFunc(doneHandler))
	cancel, d := start(t, v)
	eventually(t, func() bool { return v.Snapshot().Channel == "paused" })
	if v.Snapshot().LastCode != "authorization" || calls.Load() != 1 {
		t.Fatal("auth status or polling")
	}
	f, e := s.PendingPoll(context.Background(), scope)
	if e != nil || f == nil || string(f.Body) != string(raw) {
		t.Fatalf("raw auth evidence %v %v", f, e)
	}
	select {
	case <-d:
		t.Fatal("outer lifetime ended")
	default:
	}
	cancel()
}
func TestQuarantineAllowsAlreadyAcceptedInboxToFinish(t *testing.T) {
	s, _ := storeFor(t)
	ctx := context.Background()
	id, e := s.RecordPoll(ctx, scope, "", body(1, "before"))
	if e != nil {
		t.Fatal(e)
	}
	entries, _, e := normalize(body(1, "before"), scope, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ResolvePoll(ctx, scope, id, "before", entries); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RecordPoll(ctx, scope, "before", []byte("bad")); e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	v := mustService(t, s, pollFunc(func(context.Context, string) ([]byte, error) { calls.Add(1); return nil, nil }), handleFunc(doneHandler))
	cancel, _ := start(t, v)
	eventually(t, func() bool { return v.Snapshot().Channel == "quarantined" && v.Snapshot().Sent == 1 })
	if calls.Load() != 0 {
		t.Fatal("quarantined account repolled")
	}
	cancel()
}

// Concurrent workers may finish a database call after another worker pauses the
// account. Their late backoff/storage/result observations must preserve the pause.
func TestAccountPauseCannotBeOverwrittenByLateWorker(t *testing.T) {
	s, _ := storeFor(t)
	v := mustService(t, s, pollFunc(idlePoll), handleFunc(doneHandler))
	_, cancel := context.WithCancel(context.Background())
	v.pause(cancel, "authorization")
	v.setChannel("backoff", "capacity")
	v.pause(cancel, "storage")
	v.recordResult(agent.Result{State: "sent"})
	got := v.Snapshot()
	if got.Channel != "paused" || got.LastCode != "authorization" {
		t.Fatalf("pause overwritten %+v", got)
	}
}

// Each text item satisfies the transport limit, but separator bytes also count
// toward the envelope limit. Omitting separators from admission breaks this.
func TestNormalizeAggregateTextIncludesSeparators(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second int
		accepted      bool
	}{{"exact", 8192, 8191, true}, {"separator-overflow", 8192, 8192, false}, {"items-fit-aggregate-overflow", 16384, 1, false}} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"msgs":[{"message_id":1,"from_user_id":"user","message_type":1,"context_token":"token","item_list":[{"type":1,"text_item":{"text":"` + strings.Repeat("a", tc.first) + `"}},{"type":1,"text_item":{"text":"` + strings.Repeat("b", tc.second) + `"}}]}]}`)
			entries, _, err := normalize(raw, scope, time.Unix(100, 0))
			if !tc.accepted {
				if !errors.Is(err, weixin.ErrProtocol) || len(entries) != 0 {
					t.Fatalf("accepted aggregate %d+1+%d", tc.first, tc.second)
				}
				return
			}
			if err != nil || len(entries) != 1 || len(entries[0].Text) != 16384 || entries[0].Text[8192] != '\n' {
				t.Fatalf("exact boundary rejected: entries=%d error=%v", len(entries), err)
			}
		})
	}
}

// Late joining would allocate a one-MiB string before rejecting it. The helper
// isolates that admission boundary from JSON decoder allocations and uses six
// total invocations (warmup plus five measurements), not a heavy benchmark loop.
func TestRejectedAggregateTextDoesNotAllocateJoinedString(t *testing.T) {
	item := strings.Repeat("x", 16384)
	parts := make([]string, 64)
	for i := range parts {
		parts[i] = item
	}
	var joined string
	var fits bool
	allocations := testing.AllocsPerRun(5, func() { joined, fits = joinText(parts) })
	if fits || joined != "" {
		t.Fatal("oversized aggregate accepted")
	}
	if allocations != 0 {
		t.Fatalf("rejected text allocated joined output: %.1f allocations/run", allocations)
	}
}
