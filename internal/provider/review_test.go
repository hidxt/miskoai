package provider

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestStreamRequiresTerminalFinish(t *testing.T) {
	d, _ := NewDeepSeek("https://api.deepseek.com", "synthetic", "model")
	d.transport.SetTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		return fixture(200, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: [DONE]\n\n"), nil
	}))
	if _, err := d.Stream(context.Background(), []Message{{Role: "user", Content: "fixture"}}, 64, func(string) error { return nil }); err == nil {
		t.Fatal("unfinished stream accepted")
	}
}

func TestRetryPassesTotalDeadline(t *testing.T) {
	sentinel := errors.New("fixture stop")
	err := retrySafeStatus(context.Background(), func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("callback received no total deadline")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
