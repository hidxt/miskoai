package cli

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestVerificationInputBoundedAndCancelable(t *testing.T) {
	for _, input := range []string{"123456\r", "123456\n", "12345\b6\n"} {
		code, err := readVerification(context.Background(), io.NopCloser(strings.NewReader(input)))
		if err != nil || code != "123456" && input != "12345\b6\n" || input == "12345\b6\n" && code != "12346" {
			t.Fatalf("input failed: %v", err)
		}
	}
	for _, input := range []string{"123\n", "12345678901\n", "12x456\n", "\x03"} {
		if _, err := readVerification(context.Background(), io.NopCloser(strings.NewReader(input))); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	r, w := io.Pipe()
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := readVerification(ctx, r); err == nil || time.Since(started) > time.Second {
		t.Fatal("blocked verification did not cancel")
	}
}
