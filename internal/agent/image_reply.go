package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

// ReplyImageBorrow synchronously lends a fully validated immutable image while
// retaining the original generation, Store, scope and Manager lifetime. The
// callback must finish all work before returning and must not retain artifacts.
type ReplyImageBorrow func(context.Context, int64, string, func(context.Context, *storage.Store, storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error

// ReplyImagePrepare returns uploaded metadata only after encryption, upload and
// ciphertext Close have actually completed. It must not retain the artifact.
type ReplyImagePrepare func(context.Context, string, attachment.LocalImage, attachment.Artifact) (weixin.ImageSend, error)

type replyImageSender interface {
	SendImage(context.Context, string, string, string, weixin.ImageSend) error
}

// NewWithImageReplies configures an immutable copy; admission and existing
// dependencies remain shared. Construction performs no I/O.
func NewWithImageReplies(base *Agent, borrow ReplyImageBorrow, prepare ReplyImagePrepare) (*Agent, error) {
	if base == nil || base.store == nil || base.model == nil || base.sender == nil || base.admission == nil || borrow == nil || prepare == nil {
		return nil, errors.New("invalid image reply configuration")
	}
	if _, ok := base.sender.(replyImageSender); !ok {
		return nil, errors.New("invalid image reply configuration")
	}
	copy := *base
	copy.replyImageBorrow = borrow
	copy.replyImagePrepare = prepare
	return &copy, nil
}

func imageReplyClientID(in Incoming) string {
	encoded, _ := json.Marshal([4]string{in.Account, in.User, in.ID, "image-v1"})
	digest := sha256.Sum256(encoded)
	return "miskoai-" + hex.EncodeToString(digest[:])
}

// sendImageReply retains only local durable progress. Receipts are descriptive:
// neither failures nor a reconstructed Agent authorize retries or continuation.
func (a *Agent) sendImageReply(ctx context.Context, in Incoming, text string, usage provider.Usage, selected storage.LibraryAsset) (Result, error) {
	textID, imageID := stableClientID(in), imageReplyClientID(in)
	if err := a.store.PrepareImageReply(ctx, a.scope, in.ID, selected.ID, selected.SHA256, textID, imageID); err != nil {
		// Reservation failure may leave a receiptless processing claim. This
		// bounded state-only cleanup is safe even for an uncertain commit:
		// storage refuses the generic update whenever a sidecar exists.
		detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		_ = a.store.SetMessageState(detached, a.scope, in.ID, "failed")
		cancel()
		return Result{State: "failed", ImageState: "stopped", Usage: usage, Code: storageCode(err)}, errors.New(storageCode(err))
	}
	phase, stopCode, code := storage.ImageStopPreparation, storage.ImageStopRefused, "image"
	textStarted, textACK, imageStarted, imageACK, called := false, false, false, false, false
	err := a.replyImageBorrow(ctx, selected.ID, selected.SHA256, func(work context.Context, held *storage.Store, row storage.LibraryAsset, local attachment.LocalImage, artifact attachment.Artifact) (consumeErr error) {
		// Core may supply a generation-bound child context. Preserve its
		// cancellation even when the borrower's outer context remains live.
		defer func() {
			if ctx.Err() != nil || work == nil || work.Err() != nil {
				code, stopCode = "canceled", storage.ImageStopCanceled
				consumeErr = errors.New(code)
			}
		}()
		if called {
			return errors.New("image")
		}
		called = true
		canceled := func() bool { return ctx.Err() != nil || work == nil || work.Err() != nil }
		if canceled() {
			code, stopCode = "canceled", storage.ImageStopCanceled
			return errors.New(code)
		}
		// The borrower supplies trusted byte validation and lifetime, but cannot
		// substitute a Store, identity or stale/disabled/deleting metadata.
		if held != a.store || artifact == nil || !replyImageMatches(selected, row, local, artifact) {
			return errors.New(code)
		}
		fresh, e := held.LibraryAsset(work, a.scope, selected.ID)
		if e != nil {
			code, stopCode = "storage", storage.ImageStopUnavailable
			return errors.New(code)
		}
		if !replyImageMatches(selected, fresh, local, artifact) {
			return errors.New(code)
		}
		code, stopCode = "storage", storage.ImageStopUnavailable
		if e = held.EnterImagePreparation(work, a.scope, in.ID); e != nil {
			return errors.New(code)
		}
		if canceled() {
			code, stopCode = "canceled", storage.ImageStopCanceled
			return errors.New(code)
		}
		code = "prepare"
		uploaded, e := a.replyImagePrepare(work, in.User, local, artifact)
		if e != nil {
			return errors.New(code)
		}
		if canceled() {
			code, stopCode = "canceled", storage.ImageStopCanceled
			return errors.New(code)
		}
		code = "storage"
		if e = held.FinishImagePreparation(work, a.scope, in.ID); e != nil {
			return errors.New(code)
		}
		phase = storage.ImageStopText
		if e = held.BeginImageReplyText(work, a.scope, in.ID); e != nil {
			return errors.New(code)
		}
		textStarted = true
		if canceled() {
			code, stopCode = "canceled", storage.ImageStopCanceled
			return errors.New(code)
		}
		code, stopCode = "send", storage.ImageStopUncertain
		if e = a.sender.SendText(work, in.User, in.ContextToken, textID, text); e != nil {
			return errors.New(code)
		}
		if canceled() {
			code, stopCode = "canceled", storage.ImageStopCanceled
			return errors.New(code)
		}
		code = "storage"
		if e = held.ConfirmImageReplyText(work, a.scope, in.ID, text); e != nil {
			return errors.New(code)
		}
		textACK = true
		phase, stopCode = storage.ImageStopImage, storage.ImageStopUnavailable
		if e = held.BeginImageReplyImage(work, a.scope, in.ID); e != nil {
			return errors.New(code)
		}
		imageStarted = true
		if canceled() {
			code, stopCode = "canceled", storage.ImageStopCanceled
			return errors.New(code)
		}
		code, stopCode = "send", storage.ImageStopUncertain
		if e = a.sender.(replyImageSender).SendImage(work, in.User, in.ContextToken, imageID, uploaded); e != nil {
			return errors.New(code)
		}
		if canceled() {
			code, stopCode = "canceled", storage.ImageStopCanceled
			return errors.New(code)
		}
		code = "storage"
		if e = held.ConfirmImageReplyImage(work, a.scope, in.ID, time.Now().UTC().UnixMilli()); e != nil {
			return errors.New(code)
		}
		imageACK = true
		return nil
	})
	result := Result{State: "failed", ImageState: "stopped", Usage: usage}
	if textStarted {
		result.State = "ambiguous"
	}
	if textACK {
		result.State = "sent"
	}
	if imageStarted {
		result.ImageState = "unknown"
	}
	if imageACK {
		result.ImageState = "acked"
	}
	if err == nil && ctx.Err() == nil && imageACK {
		return result, nil
	}
	if code == "canceled" || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code, stopCode = "canceled", storage.ImageStopCanceled
	} else if errors.Is(err, attachment.ErrCleanup) {
		code, stopCode = "cleanup", storage.ImageStopCleanup
	} else if imageACK {
		code = "cleanup"
	}
	// Acknowledged phases are immutable, including plaintext cleanup failure
	// returned by the outer borrower after both callback ACK writes succeeded.
	if !imageACK {
		detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		_ = a.store.StopImageReply(detached, a.scope, in.ID, phase, stopCode)
		cancel()
	}
	result.Code = code
	return result, errors.New(code)
}

func replyImageMatches(selected, row storage.LibraryAsset, local attachment.LocalImage, artifact attachment.Artifact) bool {
	return imageSelectionAsset(row) && row.ID == selected.ID && row.Scope == selected.Scope && row.SHA256 == selected.SHA256 &&
		row.Enabled && row.State == "ready" && row.Frames == 1 && (row.Format == "png" || row.Format == "jpeg") &&
		row.Bytes == selected.Bytes && row.Width == selected.Width && row.Height == selected.Height && row.Format == selected.Format &&
		local.SHA256 == row.SHA256 && local.Bytes == row.Bytes && artifact.Size() == row.Bytes &&
		local.Info.Format == row.Format && local.Info.Width == row.Width && local.Info.Height == row.Height && local.Info.Frames == 1 && !local.Info.Animated
}
