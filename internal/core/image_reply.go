package core

import (
	"context"

	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/replyimage"
	"github.com/hidxt/miskoai/internal/storage"
)

// generationReplyAgent is called during generation construction with c.mu held.
// Optional injected text/media dependencies keep their existing text behavior.
func (c *Controller) generationReplyAgent(generationContext context.Context, g *generation, ch Channel, a *agent.Agent) (*agent.Agent, error) {
	if c == nil || generationContext == nil || g == nil || a == nil {
		return nil, ErrConfiguration
	}
	locator, ok := ch.(imageChannel)
	if !ok || c.cdn == nil || c.library == nil {
		return a, nil
	}
	p, err := replyimage.New(c.cfg.DataDir, c.cdn)
	if err != nil {
		return nil, ErrConfiguration
	}
	return c.bindGenerationReplyAgent(generationContext, g, ch, a, func(ctx context.Context, to string, im attachment.LocalImage, art attachment.Artifact) (outImage weixin.ImageSend, err error) {
		return p.Prepare(ctx, to, im, art, locator)
	})
}

// bindGenerationReplyAgent takes only synchronous preparation, not transport or
// lifetime authority. The production caller above always supplies the real
// borrowed-CDN Preparer. Tests can control preparation without network access.
func (c *Controller) bindGenerationReplyAgent(generationContext context.Context, g *generation, ch Channel, a *agent.Agent, prepare agent.ReplyImagePrepare) (*agent.Agent, error) {
	if c == nil || generationContext == nil || g == nil || a == nil || prepare == nil || c.store == nil || c.library == nil {
		return nil, ErrConfiguration
	}
	if _, ok := ch.(imageChannel); !ok {
		return a, nil
	}
	expectedStore, sc, manager := c.store, c.scope, c.library
	borrow := func(ctx context.Context, id int64, digest string, consume func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error {
		if consume == nil {
			return ErrConfiguration
		}
		// This check adds the generation-construction Manager identity; the existing
		// helper independently rechecks Store/scope/generation and fresh lease state.
		c.mu.Lock()
		same := c.library == manager
		c.mu.Unlock()
		if !same {
			return storage.ErrStale
		}
		return c.withGenerationLibraryAsset(ctx, generationContext, g, expectedStore, sc, id, digest, func(bound context.Context, s *storage.Store, row storage.LibraryAsset, im attachment.LocalImage, art attachment.Artifact) error {
			c.mu.Lock()
			same := c.library == manager
			c.mu.Unlock()
			if !same {
				return storage.ErrStale
			}
			return consume(bound, s, row, im, art)
		})
	}
	configured, err := agent.NewWithImageReplies(a, borrow, prepare)
	if err != nil {
		return nil, ErrConfiguration
	}
	return configured, nil
}
