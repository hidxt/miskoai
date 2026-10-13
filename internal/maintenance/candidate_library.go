package maintenance

import (
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/storage"
)

// LibrarySnapshot reads metadata without consuming the owned candidate.
func (c *Candidate) LibrarySnapshot(ctx context.Context) ([]storage.LibraryAsset, error) {
	if c == nil {
		return nil, errOwnership
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.verify(); err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, errRestore
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	assets, err := storage.LibraryBackupSnapshot(ctx, c.path)
	// Even a failed read must not hide an identity change or remove a
	// replacement. The candidate remains owned by the caller throughout.
	if ownershipErr := c.verify(); ownershipErr != nil {
		return nil, ownershipErr
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errRestore
	}
	return assets, nil
}
