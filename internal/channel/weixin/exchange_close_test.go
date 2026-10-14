package weixin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type exchangeCloseBody struct {
	io.Reader
	closeErr error
	readErr  error
	cancel   context.CancelFunc
	closed   int
	entered  chan struct{}
	release  chan struct{}
}

func (b *exchangeCloseBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.Reader.Read(p)
}
func (b *exchangeCloseBody) Close() error {
	b.closed++
	if b.entered != nil {
		close(b.entered)
		<-b.release
	}
	if b.cancel != nil {
		b.cancel()
	}
	return b.closeErr
}

func TestExchangeCloseRefusesSuccessfulResponse(t *testing.T) {
	for _, kind := range []string{"upload", "text", "image", "exchange_send", "exchange_nonsend"} {
		for _, mode := range []string{"live", "failure", "cancel", "failure_cancel"} {
			t.Run(kind+"_"+mode, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				b := &exchangeCloseBody{Reader: strings.NewReader(`{"ret":0,"upload_param":"p"}`)}
				if strings.Contains(mode, "failure") {
					b.closeErr = errors.New("synthetic-private-close-error")
				}
				if strings.Contains(mode, "cancel") {
					b.cancel = cancel
				}
				calls := 0
				c := fixture(t, func(*http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: b}, nil
				})
				var err error
				var raw []byte
				var loc ImageUploadLocation
				send := kind == "text" || kind == "image" || kind == "exchange_send"
				switch kind {
				case "upload":
					loc, err = c.GetImageUploadURL(ctx, imageUploadFixture())
				case "text":
					err = c.SendText(ctx, "u", "ctx", "id", "synthetic reply")
				case "image":
					err = c.SendImage(ctx, "u", "ctx", "id", imageSendFixture())
				default:
					raw, err = c.exchange(ctx, c.base, "POST", "/ilink/bot/sendmessage", struct{}{}, true, send, 15*time.Second)
				}
				var want error
				if mode != "live" {
					want = ErrTransport
					if ctx.Err() != nil {
						want = ctx.Err()
					}
					if send {
						want = ErrOutcomeUnknown
					}
				}
				if !errors.Is(err, want) {
					t.Errorf("got %v want %v", err, want)
				}
				if mode != "live" && (len(raw) != 0 || loc != (ImageUploadLocation{})) {
					t.Error("failed cleanup exposed successful output")
				}
				if mode == "live" && kind == "upload" && loc.UploadParam != "p" {
					t.Error("live metadata lost")
				}
				if mode == "live" && strings.HasPrefix(kind, "exchange_") && len(raw) == 0 {
					t.Error("live bytes lost")
				}
				if calls != 1 || b.closed != 1 || len(c.slot) != 0 {
					t.Error("once dispatch/close/release", calls, b.closed, len(c.slot))
				}
				if err != nil && strings.Contains(err.Error(), "synthetic-private") {
					t.Error("raw error leaked")
				}
			})
		}
	}
}

func TestExchangeClosePreservesPriorFailure(t *testing.T) {
	for _, send := range []bool{false, true} {
		for _, tc := range []struct {
			name          string
			status        int
			readErr       error
			payload       string
			nonSend, send error
		}{
			{"unauthorized", 401, nil, "", ErrUnauthorized, ErrUnauthorized},
			{"forbidden", 403, nil, "", ErrUnauthorized, ErrUnauthorized},
			{"rate", 429, nil, "", ErrRateLimited, ErrRateLimited},
			{"server", 503, nil, "", ErrHTTP, ErrOutcomeUnknown},
			{"otherhttp", 400, nil, "", ErrHTTP, ErrHTTP},
			{"read", 200, errors.New("synthetic-private-read-error"), "", ErrTransport, ErrOutcomeUnknown},
			{"limit", 200, nil, strings.Repeat("x", maxResponseBytes+1), ErrResponseLimit, ErrOutcomeUnknown},
		} {
			for _, cancelOnClose := range []bool{false, true} {
				name := tc.name + "_nonsend"
				if send {
					name = tc.name + "_send"
				}
				if cancelOnClose {
					name += "_cancel"
				}
				t.Run(name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					b := &exchangeCloseBody{Reader: strings.NewReader(tc.payload), readErr: tc.readErr, closeErr: errors.New("synthetic-private-close-error")}
					if cancelOnClose {
						b.cancel = cancel
					}
					calls := 0
					c := fixture(t, func(*http.Request) (*http.Response, error) {
						calls++
						return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: b}, nil
					})
					raw, err := c.exchange(ctx, c.base, "POST", "/ilink/bot/sendmessage", struct{}{}, true, send, 15*time.Second)
					want := tc.nonSend
					if send {
						want = tc.send
					}
					if !errors.Is(err, want) || len(raw) != 0 {
						t.Errorf("got %v want %v or output exposed", err, want)
					}
					if calls != 1 || b.closed != 1 || len(c.slot) != 0 {
						t.Error("once close/release")
					}
					if err != nil && strings.Contains(err.Error(), "synthetic-private") {
						t.Error("raw error leaked")
					}
				})
			}
		}
	}
}

func TestExchangeCloseBarrierRetainsAdmissionAndCancellation(t *testing.T) {
	for _, poll := range []bool{false, true} {
		for _, send := range []bool{false, true} {
			name := "outbound_nonsend"
			if poll {
				name = "poll_nonsend"
			}
			if send {
				name = strings.Replace(name, "nonsend", "send", 1)
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				b := &exchangeCloseBody{Reader: strings.NewReader(`{"ret":0}`), entered: make(chan struct{}), release: make(chan struct{})}
				var once sync.Once
				unblock := func() { once.Do(func() { close(b.release) }) }
				var wg sync.WaitGroup
				// Register cleanup before any assertions or goroutine launch. An early
				// failure cancels, unblocks actual Close, then joins only started work.
				t.Cleanup(func() { cancel(); unblock(); wg.Wait() })
				calls := 0
				c := fixture(t, func(*http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: b}, nil
				})
				path := "/ilink/bot/sendmessage"
				slot := c.slot
				if poll {
					path = "/ilink/bot/getupdates"
					slot = c.pollSlot
				}
				type result struct {
					raw []byte
					err error
				}
				done := make(chan result, 1)
				wg.Add(1)
				go func() {
					defer wg.Done()
					raw, err := c.exchange(ctx, c.base, "POST", path, struct{}{}, true, send, 15*time.Second)
					done <- result{raw, err}
				}()
				select {
				case <-b.entered:
				case <-time.After(2 * time.Second):
					t.Fatal("Close not entered")
				}
				cancel()
				if len(slot) != 1 {
					t.Error("slot released before actual Close")
				}
				select {
				case <-done:
					t.Error("returned before actual Close")
				default:
				}
				unblock()
				var got result
				select {
				case got = <-done:
				case <-time.After(2 * time.Second):
					t.Fatal("actual Close did not return")
				}
				want := error(context.Canceled)
				if send {
					want = ErrOutcomeUnknown
				}
				if !errors.Is(got.err, want) || len(got.raw) != 0 {
					t.Errorf("got %v want %v or successful bytes leaked", got.err, want)
				}
				if calls != 1 || b.closed != 1 || len(slot) != 0 {
					t.Error("once close/release", calls, b.closed, len(slot))
				}
			})
		}
	}
}
