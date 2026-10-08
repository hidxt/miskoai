package weixin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(t *testing.T, f roundTripFunc) *Client {
	t.Helper()
	c, e := New("https://ilinkai.weixin.qq.com", "synthetic-token")
	if e != nil {
		t.Fatal(e)
	}
	c.httpClient.Transport = f
	return c
}
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestEndpointPolicy(t *testing.T) {
	for _, u := range []string{"http://ilinkai.weixin.qq.com", "https://127.0.0.1", "https://weixin.qq.com.evil.example", "https://evilweixin.qq.com", "https://u:p@ilinkai.weixin.qq.com", "https://ilinkai.weixin.qq.com:444", "https://ilinkai.weixin.qq.com/path", "https://ilinkai.weixin.qq.com?token=x"} {
		if _, e := New(u, ""); e == nil {
			t.Errorf("accepted %s", u)
		}
	}
	if _, e := New("https://ilinkai.weixin.qq.com", ""); e != nil {
		t.Fatal(e)
	}
}

func TestPublicIPPolicy(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "::1", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "64:ff9b::a00:1"} {
		if publicIP(netip.MustParseAddr(address)) {
			t.Errorf("accepted restricted address %s", address)
		}
	}
	for _, address := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicIP(netip.MustParseAddr(address)) {
			t.Errorf("rejected public address %s", address)
		}
	}
}

func TestRejectIPv6TransitionDestinations(t *testing.T) {
	for _, address := range []string{"2002:0a00:0001::1", "2001:0000:4136:e378:8000:63bf:3fff:fdd2"} {
		if publicIP(netip.MustParseAddr(address)) {
			t.Errorf("accepted transition address %s", address)
		}
	}
}
func TestGetUpdatesWireAndLosslessID(t *testing.T) {
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/ilink/bot/getupdates" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-token" || r.Header.Get("AuthorizationType") != "ilink_bot_token" || r.Header.Get("X-WECHAT-UIN") == "" {
			t.Error("missing official authentication headers")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Error("missing deadline")
		}
		var b map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&b)
		if string(b["get_updates_buf"]) != "\"previous\"" || len(b["base_info"]) == 0 {
			t.Error("wrong cursor/metadata")
		}
		return response(`{"ret":0,"msgs":[{"message_id":18446744073709551615,"from_user_id":"peer","context_token":"opaque","item_list":[{"type":1,"text_item":{"text":"你好"}}]}],"get_updates_buf":"next","longpolling_timeout_ms":35000}`), nil
	})
	u, e := c.GetUpdates(context.Background(), "previous")
	if e != nil || u.Cursor != "next" || u.Messages[0].MessageID.String() != "18446744073709551615" {
		t.Fatalf("updates %+v error %v", u, e)
	}
}
func TestSendTextContextAndNoRetry(t *testing.T) {
	calls := 0
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		var b struct {
			Msg Message `json:"msg"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		if r.URL.Path != "/ilink/bot/sendmessage" || b.Msg.ToUserID != "peer" || b.Msg.ContextToken != "ctx" || b.Msg.ClientID != "stable" || b.Msg.MessageType != 2 || b.Msg.MessageState != 2 || b.Msg.Items[0].Text.Text != "hello" {
			t.Errorf("wrong payload %+v", b)
		}
		return nil, errors.New("synthetic transport failure")
	})
	e := c.SendText(context.Background(), "peer", "ctx", "stable", "hello")
	if !errors.Is(e, ErrOutcomeUnknown) || calls != 1 {
		t.Fatalf("error %v calls %d", e, calls)
	}
	if e = c.SendText(context.Background(), "peer", "", "id", "hello"); !errors.Is(e, ErrInvalidInput) {
		t.Fatal(e)
	}
}
func TestAuthExpirySafeError(t *testing.T) {
	c := fixture(t, func(*http.Request) (*http.Response, error) {
		return response(`{"ret":0,"errcode":-14,"errmsg":"secret synthetic-token"}`), nil
	})
	_, e := c.GetUpdates(context.Background(), "")
	if !errors.Is(e, ErrAuthExpired) || strings.Contains(e.Error(), "secret") {
		t.Fatal(e)
	}
}
func TestQRWireAndStatus(t *testing.T) {
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "get_bot_qrcode") {
			if r.Method != "POST" || r.URL.Query().Get("bot_type") != "3" || r.Header.Get("Authorization") != "" {
				t.Error("wrong QR bootstrap")
			}
			return response(`{"qrcode":"synthetic-qr","qrcode_img_content":"synthetic-display"}`), nil
		}
		if r.Method != "GET" || r.Header.Get("AuthorizationType") != "" || r.URL.Query().Get("verify_code") != "123" {
			t.Error("wrong QR status")
		}
		return response(`{"status":"scaned_but_redirect","redirect_host":"private.example","bot_token":"synthetic-issued","ilink_bot_id":"bot@im.bot","ilink_user_id":"peer","baseurl":"https://ilinkai.weixin.qq.com"}`), nil
	})
	q, e := c.StartQR(context.Background())
	if e != nil || q.Code != "synthetic-qr" {
		t.Fatalf("%+v %v", q, e)
	}
	s, e := c.QRStatus(context.Background(), q.Code, "123")
	if e != nil || s.Status != "scaned_but_redirect" || s.RedirectHost != "private.example" || s.BotID != "bot@im.bot" {
		t.Fatalf("%+v %v", s, e)
	}
}
func TestCapsCancellationAndRedirect(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", maxResponseBytes+1), `{"ret":0} trailing`} {
		c := fixture(t, func(*http.Request) (*http.Response, error) { return response(body), nil })
		if _, e := c.GetUpdates(context.Background(), ""); e == nil {
			t.Error("accepted oversized/invalid payload")
		}
	}
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://evil.example"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if _, e := c.GetUpdates(context.Background(), ""); !errors.Is(e, ErrHTTP) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.GetUpdates(ctx, ""); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestConcurrencyBound(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		entered <- struct{}{}
		select {
		case <-release:
			return response(`{"ret":0}`), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})
	done := make(chan error, 2)
	go func() { _, e := c.GetUpdates(context.Background(), ""); done <- e }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, e := c.GetUpdates(ctx, "")
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	select {
	case <-entered:
		t.Error("concurrency cap exceeded")
	default:
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}

func TestPollDoesNotBlockSend(t *testing.T) {
	entered := make(chan struct{})
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/ilink/bot/getupdates" {
			close(entered)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return response(`{"ret":0}`), nil
	})
	pollCtx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan struct{})
	go func() { defer close(done); c.GetUpdates(pollCtx, "") }()
	<-entered
	sendCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if e := c.SendText(sendCtx, "peer", "ctx", "stable", "hi"); e != nil {
		t.Errorf("active poll blocked send: %v", e)
	}
	stop()
	<-done
}
