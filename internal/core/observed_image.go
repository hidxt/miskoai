package core

import (
	"context"

	"github.com/hidxt/miskoai/internal/channel/weixin"
)

type imageChannel interface {
	GetImageUploadURL(context.Context, weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error)
	SendImage(context.Context, string, string, string, weixin.ImageSend) error
}

// newObservedChannel preserves image capability only for a complete delegate.
// The plain observer intentionally retains its text-only method set.
func newObservedChannel(c *Controller, g *generation, delegate Channel) Channel {
	base := &observedChannel{c: c, g: g, delegate: delegate}
	if images, ok := delegate.(imageChannel); ok {
		return &observedImageChannel{observedChannel: base, images: images}
	}
	return base
}

type observedImageChannel struct {
	*observedChannel
	images imageChannel
}

func (a *observedImageChannel) GetImageUploadURL(ctx context.Context, request weixin.ImageUploadRequest) (weixin.ImageUploadLocation, error) {
	location, err := a.images.GetImageUploadURL(ctx, request)
	a.observe(err)
	return location, err
}

func (a *observedImageChannel) SendImage(ctx context.Context, to, contextToken, clientID string, image weixin.ImageSend) error {
	err := a.images.SendImage(ctx, to, contextToken, clientID, image)
	a.observe(err)
	return err
}
