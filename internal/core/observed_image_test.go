package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/hidxt/miskoai/internal/channel/weixin"
)

// Deliberately independent of the production interface: signature drift must fail.
type observedImageCapability interface {
	GetImageUploadURL(context.Context, weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error)
	SendImage(context.Context, string, string, string, weixin.ImageSend) error
}

type observedImageTextDelegate struct {
	raw                 []byte
	err                 error
	rawCalls, textCalls int
	ctx                 context.Context
	cursor              string
	textArgs            [4]string
}

func (d *observedImageTextDelegate) RawUpdates(ctx context.Context, cursor string) ([]byte, error) {
	d.rawCalls++
	d.ctx, d.cursor = ctx, cursor
	return d.raw, d.err
}
func (d *observedImageTextDelegate) SendText(ctx context.Context, a, b, c, text string) error {
	d.textCalls++
	d.ctx, d.textArgs = ctx, [4]string{a, b, c, text}
	return d.err
}

type observedImageUploadDelegate struct {
	*observedImageTextDelegate
	uploadCalls int
	request     weixin.ImageUploadRequest
	location    weixin.ImageUploadLocation
}

func (d *observedImageUploadDelegate) GetImageUploadURL(ctx context.Context, request weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
	d.uploadCalls++
	d.ctx, d.request = ctx, request
	return d.location, d.err
}

type observedImageSendDelegate struct{ *observedImageTextDelegate }

func (d *observedImageSendDelegate) SendImage(context.Context, string, string, string, weixin.ImageSend) error {
	return d.err
}

type observedImageFullDelegate struct {
	*observedImageUploadDelegate
	sendCalls int
	sendArgs  [3]string
	image     weixin.ImageSend
}

func (d *observedImageFullDelegate) SendImage(ctx context.Context, to, token, id string, image weixin.ImageSend) error {
	d.sendCalls++
	d.ctx, d.sendArgs, d.image = ctx, [3]string{to, token, id}, image
	return d.err
}
func observedImageFixture() (*Controller, *generation, context.Context, *int, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := new(int)
	g := &generation{cancel: func() { *calls++; cancel() }}
	c := &Controller{generation: g}
	return c, g, ctx, calls, func() { cancel(); c.generation = nil }
}
func observedImageFull(err error) *observedImageFullDelegate {
	return &observedImageFullDelegate{observedImageUploadDelegate: &observedImageUploadDelegate{observedImageTextDelegate: &observedImageTextDelegate{err: err}, location: weixin.ImageUploadLocation{UploadParam: "synthetic-param", UploadFullURL: "https://invalid.example/synthetic"}}}
}

func TestObservedImageExposure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		delegate Channel
		want     bool
	}{
		{"text", &observedImageTextDelegate{}, false},
		{"upload-only", &observedImageUploadDelegate{observedImageTextDelegate: &observedImageTextDelegate{}}, false},
		{"send-only", &observedImageSendDelegate{&observedImageTextDelegate{}}, false},
		{"full", observedImageFull(nil), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, g, _, _, cleanup := observedImageFixture()
			wrapped := newObservedChannel(c, g, tc.delegate)
			_, got := wrapped.(observedImageCapability)
			_, base := wrapped.(*observedChannel)
			cleanup()
			if got != tc.want || base == tc.want {
				t.Fatalf("capability=%v base=%v want capability=%v", got, base, tc.want)
			}
		})
	}
}

func TestObservedImageDelegationAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"success", nil}, {"unknown", weixin.ErrOutcomeUnknown}, {"unclassified", errors.New("synthetic unknown outcome")},
		{"read", io.ErrUnexpectedEOF}, {"cleanup", weixin.ErrTransport},
		{"cancel", context.Canceled}, {"deadline", context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, g, ctx, cancels, cleanup := observedImageFixture()
			d := observedImageFull(tc.err)
			wrapped, ok := newObservedChannel(c, g, d).(observedImageCapability)
			if !ok {
				cleanup()
				t.Fatal("full image capability lost")
			}
			req := weixin.ImageUploadRequest{ToUserID: "synthetic-peer", FileKey: "synthetic-file", RawMD5: "synthetic-md5", AESKeyHex: "synthetic-key", RawBytes: 32}
			image := weixin.ImageSend{EncryptQueryParam: "synthetic-query", AESKeyHex: "synthetic-key", CiphertextBytes: 48}
			location, uploadErr := wrapped.GetImageUploadURL(ctx, req)
			sendErr := wrapped.SendImage(ctx, "synthetic-peer", "synthetic-context", "synthetic-id", image)
			expired, events, ncancel := c.expired, len(c.events), *cancels
			cleanup()
			if location != d.location || uploadErr != tc.err || sendErr != tc.err || d.uploadCalls != 1 || d.sendCalls != 1 || d.ctx != ctx || d.request != req || d.image != image || d.sendArgs != [3]string{"synthetic-peer", "synthetic-context", "synthetic-id"} {
				t.Fatal("image input/output/error identity or exactly-once delegation changed")
			}
			if expired || events != 0 || ncancel != 0 {
				t.Fatal("non-auth error expired generation")
			}
		})
	}
}

func TestObservedImageAuthExpiry(t *testing.T) {
	for _, operation := range []string{"upload", "send"} {
		t.Run(operation, func(t *testing.T) {
			c, g, ctx, cancels, cleanup := observedImageFixture()
			err := fmt.Errorf("synthetic classified -14: %w", weixin.ErrAuthExpired)
			d := observedImageFull(err)
			wrapped, ok := newObservedChannel(c, g, d).(observedImageCapability)
			if !ok {
				cleanup()
				t.Fatal("full image capability lost")
			}
			var got error
			if operation == "upload" {
				_, got = wrapped.GetImageUploadURL(ctx, weixin.ImageUploadRequest{})
			} else {
				got = wrapped.SendImage(ctx, "", "", "", weixin.ImageSend{})
			}
			// Repeated auth failures from either method must observe just once.
			_, again := wrapped.GetImageUploadURL(ctx, weixin.ImageUploadRequest{})
			last := wrapped.SendImage(ctx, "", "", "", weixin.ImageSend{})
			expired, ncancel, ctxErr := c.expired, *cancels, ctx.Err()
			events := append([]Event(nil), c.events...)
			cleanup()
			if got != err || again != err || last != err || !expired || ncancel != 1 || ctxErr != context.Canceled || len(events) != 1 || events[0].Kind != "authorization_expired" {
				t.Fatal("current image auth expiry did not cancel and mark exactly once")
			}
		})
	}
}

func TestObservedImageStaleGeneration(t *testing.T) {
	c, old, oldctx, oldCancels, cleanupOld := observedImageFixture()
	_, replacement, newctx, newCancels, cleanupNew := observedImageFixture()
	d := observedImageFull(weixin.ErrAuthExpired)
	wrapped, ok := newObservedChannel(c, old, d).(observedImageCapability)
	if !ok {
		cleanupOld()
		cleanupNew()
		t.Fatal("full image capability lost")
	}
	c.generation = replacement
	_, uploadErr := wrapped.GetImageUploadURL(oldctx, weixin.ImageUploadRequest{})
	sendErr := wrapped.SendImage(oldctx, "", "", "", weixin.ImageSend{})
	expired, events, oldN, newN, oldErr, newErr := c.expired, len(c.events), *oldCancels, *newCancels, oldctx.Err(), newctx.Err()
	cleanupOld()
	cleanupNew()
	if uploadErr != weixin.ErrAuthExpired || sendErr != weixin.ErrAuthExpired || expired || events != 0 || oldN != 0 || newN != 0 || oldErr != nil || newErr != nil || d.uploadCalls != 1 || d.sendCalls != 1 {
		t.Fatal("stale observer changed generation state or error/delegation")
	}
}

func TestObservedImageTextRegression(t *testing.T) {
	for _, full := range []bool{false, true} {
		for _, auth := range []bool{false, true} {
			t.Run(fmt.Sprintf("full=%v/auth=%v", full, auth), func(t *testing.T) {
				c, g, ctx, cancels, cleanup := observedImageFixture()
				err := io.ErrUnexpectedEOF
				if auth {
					err = weixin.ErrAuthExpired
				}
				d := observedImageFull(err)
				d.raw = []byte("synthetic-raw")
				var delegate Channel = d.observedImageTextDelegate
				if full {
					delegate = d
				}
				wrapped := newObservedChannel(c, g, delegate)
				raw, rawErr := wrapped.RawUpdates(ctx, "synthetic-cursor")
				sendErr := wrapped.SendText(ctx, "a", "b", "c", "synthetic-text")
				expired, ncancel, events := c.expired, *cancels, len(c.events)
				cleanup()
				if len(raw) == 0 || &raw[0] != &d.raw[0] || rawErr != err || sendErr != err || d.rawCalls != 1 || d.textCalls != 1 || d.ctx != ctx || d.cursor != "synthetic-cursor" || d.textArgs != [4]string{"a", "b", "c", "synthetic-text"} {
					t.Fatal("raw/text delegation changed")
				}
				want := 0
				if auth {
					want = 1
				}
				if expired != auth || ncancel != want || events != want || d.uploadCalls != 0 || d.sendCalls != 0 {
					t.Fatal("raw/text observation or image isolation changed")
				}
			})
		}
	}
}
