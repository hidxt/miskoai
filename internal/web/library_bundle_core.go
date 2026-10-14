package web

import (
	"context"
	"github.com/hidxt/miskoai/internal/core"
)

var _ libraryBundleBackend = coreBackend{}

// Controller supplies the actual restore method through the embedded backend.
func (b coreBackend) prepareLibraryBundle(ctx context.Context) (managementDownload, error) {
	if b.Controller == nil {
		return nil, core.ErrUnavailable
	}
	d, e := b.PrepareLibraryBundle(ctx)
	if e != nil {
		return nil, e
	}
	if d == nil {
		return nil, core.ErrUnavailable
	}
	return coreDownload{d}, nil
}
