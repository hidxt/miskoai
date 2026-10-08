package weixin

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

func objects(n int) string { return strings.TrimSuffix(strings.Repeat("{},", n), ",") }

// Removing admission before typed decoding would accept this oversized batch.
func TestTypedPollRejectsUnboundedArrays(t *testing.T) {
	for _, body := range []string{
		`{"msgs":[` + objects(257) + `]}`,
		`{"msgs":[{"item_list":[` + objects(257) + `]}]}`,
		`{"msgs":[` + objects(257) + `],"msgs":[]}`,
		`{"msgs":[{"item_list":[` + objects(257) + `],"item_list":[]}]}`,
		`{"mſgs":[` + objects(257) + `]}`,
		`{"msgs":[{"item_liſt":[` + objects(257) + `]}]}`,
		`{"msgs":[` + objects(128) + `],"msgs":[` + objects(129) + `]}`,
		`{"msgs":[{"item_list":[` + objects(128) + `],"item_list":[` + objects(129) + `]}]}`,
	} {
		c := fixture(t, func(*http.Request) (*http.Response, error) { return response(body), nil })
		if _, err := c.GetUpdates(context.Background(), ""); !errors.Is(err, ErrProtocol) {
			t.Fatalf("oversized array returned %v", err)
		}
	}
}

func TestRawPollRetainsMalformedBody(t *testing.T) {
	for _, body := range []string{`{broken`, `{"ret":0,"msgs":"private malformed"}`, "\xff", `null`} {
		calls := 0
		c := fixture(t, func(*http.Request) (*http.Response, error) { calls++; return response(body), nil })
		raw, err := c.RawUpdates(context.Background(), "")
		if err != nil || string(raw) != body || calls != 1 {
			t.Fatalf("raw preservation: bytes=%q err=%v calls=%d", raw, err, calls)
		}
		if _, err := DecodeUpdates(raw); !errors.Is(err, ErrProtocol) {
			t.Fatalf("decode malformed: %v", err)
		}
	}
}

func TestRawAndTypedShareWireBounds(t *testing.T) {
	for _, typed := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw", true: "typed"}[typed], func(t *testing.T) {
			calls := 0
			c := fixture(t, func(r *http.Request) (*http.Response, error) {
				calls++
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 40*time.Second || time.Until(deadline) < 39*time.Second {
					t.Error("poll deadline missing or changed")
				}
				if r.Method != "POST" || r.URL.Path != "/ilink/bot/getupdates" || r.Header.Get("Authorization") != "Bearer synthetic-token" || r.Header.Get("AuthorizationType") != "ilink_bot_token" || r.Header.Get("X-WECHAT-UIN") == "" || r.Header.Get("iLink-App-Id") != "bot" {
					t.Error("poll wire changed")
				}
				return response(strings.Repeat("x", 2*1024*1024+1)), nil
			})
			var err error
			if typed {
				_, err = c.GetUpdates(context.Background(), "")
			} else {
				_, err = c.RawUpdates(context.Background(), "")
			}
			if !errors.Is(err, ErrResponseLimit) || calls != 1 {
				t.Fatalf("bound/no retry err=%v calls=%d", err, calls)
			}
			if _, err = c.RawUpdates(context.Background(), strings.Repeat("x", 16385)); !errors.Is(err, ErrInvalidInput) || calls != 1 {
				t.Fatal("oversized input cursor reached transport")
			}
		})
	}
}

func TestDecodeBoundsAndAuthErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       error
	}{
		{"ret auth", `{"ret":-14}`, ErrAuthExpired},
		{"code auth", `{"ret":0,"errcode":-14,"errmsg":"private"}`, ErrAuthExpired},
		{"ret auth mixed", `{"ret":-14,"errcode":1}`, ErrAuthExpired},
		{"code auth mixed", `{"ret":1,"errcode":-14,"errmsg":"private"}`, ErrAuthExpired},
		{"service", `{"ret":1}`, ErrService},
		{"array outer", `[]`, ErrProtocol}, {"number outer", `1`, ErrProtocol},
		{"bad msgs", `{"msgs":{}}`, ErrProtocol}, {"bad message", `{"msgs":[null]}`, ErrProtocol},
		{"bad items", `{"msgs":[{"item_list":1}]}`, ErrProtocol},
		{"bad item", `{"msgs":[{"item_list":[null]}]}`, ErrProtocol},
		{"bad status", `{"ret":"private"}`, ErrProtocol},
		{"trailing", `{"ret":0} {}`, ErrProtocol},
		{"cursor", `{"get_updates_buf":"` + strings.Repeat("x", 16385) + `"}`, ErrProtocol},
		{"sender", `{"msgs":[{"from_user_id":"` + strings.Repeat("x", 16385) + `"}]}`, ErrProtocol},
		{"text", `{"msgs":[{"item_list":[{"type":1,"text_item":{"text":"` + strings.Repeat("x", 16385) + `"}}]}]}`, ErrProtocol},
		{"body cap", strings.Repeat(" ", 2*1024*1024+1), ErrResponseLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeUpdates([]byte(tc.body))
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v got %v", tc.want, err)
			}
		})
	}
	for _, body := range []string{`{}`, `{"msgs":null}`, `{"msgs":[],"unknown":[[{},{}]]}`} {
		u, err := DecodeUpdates([]byte(body))
		if err != nil || len(u.Messages) != 0 {
			t.Fatalf("empty/unknown response: %v", err)
		}
	}
	for _, body := range []string{`{"msgs":[` + objects(256) + `]}`, `{"msgs":[{"item_list":[` + objects(256) + `]}]}`} {
		if _, err := DecodeUpdates([]byte(body)); err != nil {
			t.Fatalf("boundary rejected: %v", err)
		}
	}
}

// A post-unmarshal count check would allocate hundreds of MiB for these arrays.
// The generous ceiling catches whole typed-array allocation, not minor changes.
func TestDecodeRejectsTinyObjectFloodBeforeTypedAllocation(t *testing.T) {
	for _, body := range []string{
		`{"msgs":[` + objects(500000) + `]}`,
		`{"msgs":[{"item_list":[` + objects(500000) + `]}]}`,
		`{"msgs":[` + objects(500000) + `],"msgs":[]}`,
		`{"msgs":[{"item_list":[` + objects(500000) + `],"item_list":[]}]}`,
	} {
		raw := []byte(body)
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := DecodeUpdates(raw)
		runtime.ReadMemStats(&after)
		if !errors.Is(err, ErrProtocol) {
			t.Fatalf("flood accepted: %v", err)
		}
		if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32*1024*1024 {
			t.Fatalf("typed flood allocated %d bytes", allocated)
		}
	}
	// A large unknown array remains allowed and cannot grow a typed slice.
	raw := []byte(`{"msgs":[],"future":[` + objects(500000) + `]}`)
	if _, err := DecodeUpdates(raw); err != nil {
		t.Fatalf("unknown field rejected: %v", err)
	}
}

func TestRawPollSharesSlotAndAllowsSend(t *testing.T) {
	entered := make(chan struct{}, 2)
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/ilink/bot/getupdates" {
			entered <- struct{}{}
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return response(`{"ret":0}`), nil
	})
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan error, 1)
	go func() { _, err := c.RawUpdates(ctx, ""); done <- err }()
	<-entered
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.GetUpdates(short, ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("typed poll did not share raw slot: %v", err)
	}
	select {
	case <-entered:
		t.Fatal("overlapping raw/typed polls")
	default:
	}
	if err := c.SendText(context.Background(), "peer", "ctx", "stable", "hi"); err != nil {
		t.Fatalf("raw poll blocked send: %v", err)
	}
	stop()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("raw cancellation: %v", err)
	}
}

func TestRawPollSafeBusinessAndHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{
		{200, `{"ret":-14,"errmsg":"private"}`, ErrAuthExpired},
		{200, `{"errcode":-14}`, ErrAuthExpired},
		{200, `{"ret":1,"errcode":-14,"errmsg":"private"}`, ErrAuthExpired},
		{200, `{"ret":-14,"errcode":1}`, ErrAuthExpired},
		{200, `{"ret":1}`, ErrService},
		{401, "private", ErrUnauthorized}, {403, "private", ErrUnauthorized},
		{429, "private", ErrRateLimited}, {503, "private", ErrHTTP},
	} {
		calls := 0
		c := fixture(t, func(*http.Request) (*http.Response, error) {
			calls++
			r := response(tc.body)
			r.StatusCode = tc.status
			return r, nil
		})
		raw, err := c.RawUpdates(context.Background(), "")
		if !errors.Is(err, tc.want) || raw != nil || calls != 1 || strings.Contains(err.Error(), "private") {
			t.Fatalf("unsafe failure: bytes=%q err=%v calls=%d", raw, err, calls)
		}
	}
}

func TestDecodeAllOpaqueFieldBounds(t *testing.T) {
	for _, key := range []string{"message_id", "from_user_id", "to_user_id", "client_id", "context_token", "session_id", "group_id"} {
		_, err := DecodeUpdates([]byte(`{"msgs":[{"` + key + `":"` + strings.Repeat("x", 16385) + `"}]}`))
		if !errors.Is(err, ErrProtocol) {
			t.Fatalf("%s bound: %v", key, err)
		}
	}
	for _, key := range []string{"image_item", "voice_item", "file_item", "video_item", "ref_msg", "msg_id"} {
		_, err := DecodeUpdates([]byte(`{"msgs":[{"item_list":[{"` + key + `":"` + strings.Repeat("x", 16385) + `"}]}]}`))
		if !errors.Is(err, ErrProtocol) {
			t.Fatalf("%s bound: %v", key, err)
		}
	}
	if _, err := DecodeUpdates([]byte(`{"msgs":[{"from_user_id":"` + strings.Repeat("x", 16384) + `","item_list":[{"text_item":{"text":"` + strings.Repeat("x", 16384) + `"}}]}],"get_updates_buf":"` + strings.Repeat("x", 16384) + `"}`)); err != nil {
		t.Fatalf("16KiB boundary rejected: %v", err)
	}
}
