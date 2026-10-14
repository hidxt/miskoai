package weixin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func imageSendFixture() ImageSend {
	return ImageSend{EncryptQueryParam: "synthetic-param", AESKeyHex: strings.Repeat("a", 32), CiphertextBytes: (4 << 20) + 16}
}

func TestImageSendWire(t *testing.T) {
	calls := 0
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/ilink/bot/sendmessage" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Error("route/auth")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var got any
		if json.Unmarshal(raw, &got) != nil {
			t.Fatal("JSON")
		}
		want := `{"msg":{"from_user_id":"","to_user_id":"synthetic-user","context_token":"synthetic-context","client_id":"synthetic-id","message_type":2,"message_state":2,"item_list":[{"type":2,"image_item":{"media":{"encrypt_query_param":"synthetic-param","aes_key":"` + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))) + `","encrypt_type":1},"mid_size":4194320}}]},"base_info":{"channel_version":"2.4.9","bot_agent":"MiskoAI/0.1.0"}}`
		var expected any
		_ = json.Unmarshal([]byte(want), &expected)
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(expected)
		if string(a) != string(b) {
			t.Errorf("wire envelope mismatch: field structure differs")
		}
		return response(`{"ret":0}`), nil
	})
	if err := c.SendImage(context.Background(), "synthetic-user", "synthetic-context", "synthetic-id", imageSendFixture()); err != nil || calls != 1 {
		t.Fatalf("%v calls %d", err, calls)
	}
}

type imageWireBody struct {
	reader  io.Reader
	closed  bool
	readErr error
}

func (b *imageWireBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.reader.Read(p)
}
func (b *imageWireBody) Close() error { b.closed = true; return nil }

func TestImageWireExchangeBoundaries(t *testing.T) {
	for _, send := range []bool{false, true} {
		for _, tc := range []struct {
			name                 string
			status               int
			body                 string
			transport, read      error
			wantUpload, wantSend error
		}{
			{"unauthorized", 401, "", nil, nil, ErrUnauthorized, ErrUnauthorized},
			{"ratelimit", 429, "", nil, nil, ErrRateLimited, ErrRateLimited},
			{"server", 503, "", nil, nil, ErrHTTP, ErrOutcomeUnknown},
			{"http", 400, "", nil, nil, ErrHTTP, ErrHTTP},
			{"transport", 0, "", errors.New("synthetic-secret"), nil, ErrTransport, ErrOutcomeUnknown},
			{"read", 200, "", nil, errors.New("synthetic-secret"), ErrTransport, ErrOutcomeUnknown},
			{"limit", 200, strings.Repeat("x", maxResponseBytes+1), nil, nil, ErrResponseLimit, ErrOutcomeUnknown},
			{"truncated", 200, `{"ret":0`, nil, nil, ErrProtocol, ErrOutcomeUnknown},
		} {
			name := tc.name
			if send {
				name += "_send"
			}
			t.Run(name, func(t *testing.T) {
				calls := 0
				b := &imageWireBody{reader: strings.NewReader(tc.body), readErr: tc.read}
				c := fixture(t, func(*http.Request) (*http.Response, error) {
					calls++
					if tc.transport != nil {
						return nil, tc.transport
					}
					return &http.Response{StatusCode: tc.status, Body: b, Header: make(http.Header)}, nil
				})
				var err error
				want := tc.wantUpload
				if send {
					err = c.SendImage(context.Background(), "u", "c", "id", imageSendFixture())
					want = tc.wantSend
				} else {
					_, err = c.GetImageUploadURL(context.Background(), imageUploadFixture())
				}
				if !errors.Is(err, want) || calls != 1 {
					t.Fatalf("got %v calls %d", err, calls)
				}
				if strings.Contains(err.Error(), "synthetic-secret") {
					t.Error("raw error leaked")
				}
				if tc.transport == nil && !b.closed {
					t.Error("response not closed")
				}
				if len(c.slot) != 0 {
					t.Error("slot retained")
				}
			})
		}
	}
}

func TestImageWireCancelAndHeldSlot(t *testing.T) {
	for _, send := range []bool{false, true} {
		t.Run(map[bool]string{false: "upload", true: "send"}[send], func(t *testing.T) {
			c := fixture(t, func(*http.Request) (*http.Response, error) {
				t.Error("unexpected dispatch")
				return response(`{}`), nil
			})
			c.slot <- struct{}{}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			var err error
			if send {
				err = c.SendImage(ctx, "u", "c", "id", imageSendFixture())
			} else {
				_, err = c.GetImageUploadURL(ctx, imageUploadFixture())
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
			<-c.slot
			ctx2, cancel2 := context.WithCancel(context.Background())
			cancel2()
			if send {
				err = c.SendImage(ctx2, "u", "c", "id", imageSendFixture())
			} else {
				_, err = c.GetImageUploadURL(ctx2, imageUploadFixture())
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestImageWireCancellationAfterDispatch(t *testing.T) {
	for _, send := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		c := fixture(t, func(r *http.Request) (*http.Response, error) { calls++; cancel(); return nil, r.Context().Err() })
		var err error
		want := error(context.Canceled)
		if send {
			err = c.SendImage(ctx, "u", "c", "id", imageSendFixture())
			want = ErrOutcomeUnknown
		} else {
			_, err = c.GetImageUploadURL(ctx, imageUploadFixture())
		}
		if !errors.Is(err, want) || calls != 1 {
			t.Fatalf("%v %d", err, calls)
		}
	}
}

func TestImageWireMissingAuthentication(t *testing.T) {
	c := fixture(t, func(*http.Request) (*http.Response, error) {
		t.Error("unauthenticated dispatch")
		return response(`{}`), nil
	})
	c.token = ""
	if _, err := c.GetImageUploadURL(context.Background(), imageUploadFixture()); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if err := c.SendImage(context.Background(), "u", "c", "id", imageSendFixture()); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
}

type imageWireCloseBarrier struct{ entered, release chan struct{} }

func (b *imageWireCloseBarrier) Read(p []byte) (int, error) {
	return strings.NewReader(`{"ret":0,"upload_param":"p"}`).Read(p)
}
func (b *imageWireCloseBarrier) Close() error { close(b.entered); <-b.release; return nil }

func TestImageWireSlotHeldUntilResponseClose(t *testing.T) {
	for _, send := range []bool{false, true} {
		b := &imageWireCloseBarrier{make(chan struct{}), make(chan struct{})}
		reader := strings.NewReader(`{"ret":0,"upload_param":"p"}`)
		c := fixture(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: &imageWireCloseReader{reader, b}, Header: make(http.Header)}, nil
		})
		done := make(chan error, 1)
		go func() {
			if send {
				done <- c.SendImage(context.Background(), "u", "c", "id", imageSendFixture())
			} else {
				_, err := c.GetImageUploadURL(context.Background(), imageUploadFixture())
				done <- err
			}
		}()
		select {
		case <-b.entered:
		case <-time.After(time.Second):
			t.Fatal("close not entered")
		}
		if len(c.slot) != 1 {
			t.Error("slot released before actual Close")
		}
		select {
		case <-done:
			t.Error("returned before actual Close")
		default:
		}
		close(b.release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if len(c.slot) != 0 {
			t.Error("slot not released")
		}
	}
}

type imageWireCloseReader struct {
	io.Reader
	b *imageWireCloseBarrier
}

func (b *imageWireCloseReader) Close() error { return b.b.Close() }

func TestImageSendAcknowledgement(t *testing.T) {
	for _, body := range []string{`{"ret":0}`, `{"message_id":"18446744073709551615"}`, `{}`, `{"ret":null}`, `{"ret":0,"RET":1}`, `{"errcode":-14,"ret":9}`, `{"ret":9}`, `{"ret":0} trailing`, "{\"ret\":0,\"extra\":\"\xff\"}"} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			c := fixture(t, func(*http.Request) (*http.Response, error) { calls++; return response(body), nil })
			err := c.SendImage(context.Background(), "u", "c", "id", imageSendFixture())
			if !errors.Is(err, sendAcknowledgement([]byte(body))) || calls != 1 {
				t.Fatalf("%v calls %d", err, calls)
			}
		})
	}
}

func TestImageSendAdmission(t *testing.T) {
	for _, mutate := range []func(*ImageSend){func(q *ImageSend) { q.CiphertextBytes = 0 }, func(q *ImageSend) { q.CiphertextBytes = 17 }, func(q *ImageSend) { q.CiphertextBytes = (4 << 20) + 32 }, func(q *ImageSend) { q.AESKeyHex = strings.Repeat("A", 32) }, func(q *ImageSend) { q.EncryptQueryParam = "" }, func(q *ImageSend) { q.EncryptQueryParam = "p\n" }, func(q *ImageSend) { q.EncryptQueryParam = "\xff" }, func(q *ImageSend) { q.EncryptQueryParam = strings.Repeat("p", maxOpaqueBytes+1) }} {
		q := imageSendFixture()
		mutate(&q)
		c := fixture(t, func(*http.Request) (*http.Response, error) { t.Error("dispatch"); return response(`{"ret":0}`), nil })
		if err := c.SendImage(context.Background(), "u", "c", "id", q); !errors.Is(err, ErrInvalidInput) {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"", "\xff", "bad\n", strings.Repeat("u", maxOpaqueBytes+1)} {
		for i := 0; i < 3; i++ {
			args := []string{"u", "c", "id"}
			args[i] = value
			c := fixture(t, func(*http.Request) (*http.Response, error) {
				t.Error("identity dispatch")
				return response(`{"ret":0}`), nil
			})
			if err := c.SendImage(context.Background(), args[0], args[1], args[2], imageSendFixture()); !errors.Is(err, ErrInvalidInput) {
				t.Fatal(err)
			}
		}
	}
}
