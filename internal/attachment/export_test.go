package attachment

import (
	"context"
	"testing"

	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/storage"
)

// These test-only adapters let external integration tests reuse the completed
// private artifact fixtures and real parser configuration used by package tests.
// They are excluded from production builds.
func GeneratedDOCXForTest(t *testing.T) []byte {
	t.Helper()
	return generatedDOCX(t)
}

func CompletedForTest(t *testing.T, raw []byte) (*media.Artifact, string) {
	t.Helper()
	return completed(t, raw)
}

func FileDescriptorForTest(name string) storage.Attachment {
	return fileDescriptor(name)
}

func DocumentProcessorForTest(t *testing.T, download func(context.Context, string, string, string, [16]byte) (Artifact, error)) *ImageProcessor {
	t.Helper()
	return documentProcessor(t, download)
}
