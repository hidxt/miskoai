package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

type imageFake struct {
	calls  int
	part   provider.ContentPart
	err    error
	hook   func(context.Context)
	repeat bool
	skip   bool
}

func (f *imageFake) WithImage(ctx context.Context, d storage.Attachment, consume func(provider.ContentPart) error) error {
	f.calls++
	if f.hook != nil {
		f.hook(ctx)
	}
	if f.skip {
		return f.err
	}
	if err := consume(f.part); err != nil {
		return err
	}
	if f.repeat {
		return consume(f.part)
	}
	return f.err
}
func imageInput(id, caption string) Incoming {
	in := incoming(id, caption)
	in.Attachment = &storage.Attachment{Kind: "image", ImageHex: "00000000000000000000000000000000", Query: "synthetic-query"}
	return in
}
func imageFixture(t *testing.T) (*storage.Store, *fakeModel, *fakeModel, *fakeSearch, *fakeSender, *imageFake, *Agent) {
	t.Helper()
	s, _, text, search, send, old := fixture(t)
	vision := &fakeModel{reply: provider.Reply{Text: "synthetic visual interpretation", FinishReason: "stop", Usage: provider.Usage{TotalTokens: 37}}}
	image := &imageFake{part: provider.ContentPart{Type: "image_url", ImageURL: &provider.ImageURL{URL: "data:image/png;base64,AAAA", Detail: "low"}}}
	a, err := NewWithImages(s, text, search, send, old.opts, vision, image)
	if err != nil {
		t.Fatal(err)
	}
	return s, text, vision, search, send, image, a
}

func TestImageCaptionBypassesCommandsAndSearch(t *testing.T) {
	for _, caption := range []string{"/remember synthetic secret", "/memory clear", "/search synthetic", "请记住：synthetic"} {
		t.Run(caption, func(t *testing.T) {
			s, text, vision, search, send, image, a := imageFixture(t)
			if _, err := s.AddFact(context.Background(), a.scope, storage.Fact{Content: "existing", Source: "explicit_user"}); err != nil {
				t.Fatal(err)
			}
			image.hook = func(ctx context.Context) {
				claimed, err := s.ClaimMessage(ctx, a.scope, "caption", caption)
				if err != nil || claimed {
					t.Error("image effect preceded durable claim")
				}
			}
			r, err := a.Handle(context.Background(), imageInput("caption", caption))
			if err != nil || r.State != "sent" || r.Usage.TotalTokens != 37 || vision.calls != 1 || text.calls != 0 || search.calls != 0 || image.calls != 1 || send.calls != 1 {
				t.Fatalf("image routing: %+v %v calls %d/%d/%d/%d/%d", r, err, vision.calls, text.calls, search.calls, image.calls, send.calls)
			}
			parts, ok := vision.messages[len(vision.messages)-1].Content.([]provider.ContentPart)
			if !ok || len(parts) != 2 || parts[0].Text != caption || parts[1].ImageURL == nil {
				t.Fatal("genuine caption/current image absent")
			}
			h, err := s.History(context.Background(), a.scope, 16)
			if err != nil || len(h) != 2 || h[0].Content != caption {
				t.Fatalf("caption provenance: %+v %v", h, err)
			}
			facts, err := s.ExportFacts(context.Background(), a.scope)
			if err != nil || len(facts) != 1 || facts[0].Content != "existing" {
				t.Fatal("caption command/model altered confirmed facts")
			}
			c, err := s.Candidates(context.Background(), a.scope, 8)
			if err != nil || len(c) != 0 {
				t.Fatal("image produced candidate facts")
			}
		})
	}
}

func TestImageEmptyCaptionHistoryExclusionAndRestartDuplicate(t *testing.T) {
	s, text, vision, search, send, image, a := imageFixture(t)
	in := imageInput("empty", "")
	r, err := a.Handle(context.Background(), in)
	if err != nil || r.State != "sent" || vision.calls != 1 || send.calls != 1 {
		t.Fatalf("empty image: %+v %v", r, err)
	}
	h, err := s.History(context.Background(), a.scope, 16)
	if err != nil || len(h) != 0 {
		t.Fatalf("fabricated USER/history: %+v %v", h, err)
	}
	parts, ok := vision.messages[len(vision.messages)-1].Content.([]provider.ContentPart)
	if !ok || len(parts) != 2 || strings.TrimSpace(parts[0].Text) == "" {
		t.Fatal("model-only helper absent")
	}
	restarted, err := NewWithImages(s, text, search, send, a.opts, vision, image)
	if err != nil {
		t.Fatal(err)
	}
	r, err = restarted.Handle(context.Background(), in)
	if err != nil || r.State != "duplicate" || vision.calls != 1 || image.calls != 1 || send.calls != 1 {
		t.Fatal("restart repeated image effects")
	}
}

func TestImageAuthorizationAndInvalidDescriptorBeforeClaim(t *testing.T) {
	s, _, vision, _, send, image, a := imageFixture(t)
	in := imageInput("invalid", "caption")
	in.Attachment.ImageHex = "invalid"
	in.User = "foreign"
	r, err := a.Handle(context.Background(), in)
	if err == nil || r.Code != "scope" {
		t.Fatal("authorization must precede descriptor")
	}
	in.User = "alice"
	r, err = a.Handle(context.Background(), in)
	if err == nil || r.State != "rejected" || image.calls != 0 || vision.calls != 0 || send.calls != 0 {
		t.Fatalf("invalid descriptor effects: %+v %v", r, err)
	}
	claimed, err := s.ClaimMessage(context.Background(), a.scope, in.ID, in.Text)
	if err != nil || !claimed {
		t.Fatal("invalid descriptor durably claimed")
	}
}

func TestImageUnsupportedAndGenerationBoundNoEffects(t *testing.T) {
	for _, kind := range []string{"unsupported", "file", "long"} {
		t.Run(kind, func(t *testing.T) {
			_, text, vision, search, send, image, a := imageFixture(t)
			in := imageInput("unsupported", "/search synthetic")
			if kind == "unsupported" {
				in.Attachment = &storage.Attachment{Kind: "unsupported"}
			}
			if kind == "file" {
				in.Attachment.Kind = "file"
				in.Attachment.MediaKey = "AAAAAAAAAAAAAAAAAAAAAA=="
				in.Attachment.ImageHex = ""
				in.Attachment.FileName = "private.docx"
			}
			if kind == "long" {
				in.Text = strings.Repeat("x", 8193)
			}
			r, err := a.Handle(context.Background(), in)
			if err != nil || r.State != "sent" || send.calls != 1 || image.calls != 0 || vision.calls != 0 || text.calls != 0 || search.calls != 0 {
				t.Fatalf("unsupported effects %+v %v", r, err)
			}
			if strings.Contains(send.text, "private.docx") || strings.Contains(send.text, "synthetic-query") {
				t.Fatal("private descriptor leaked")
			}
			if kind == "long" && r.Code != "limit" {
				t.Fatal("generation bound ignored")
			}
		})
	}
}

func TestImageUsageSurvivesCleanupAndOutputFailures(t *testing.T) {
	for _, badOutput := range []bool{false, true} {
		t.Run(fmt.Sprint(badOutput), func(t *testing.T) {
			_, _, vision, _, send, image, a := imageFixture(t)
			image.err = errors.New("private cleanup path")
			if badOutput {
				vision.reply.FinishReason = "unknown"
				vision.reply.Text = "\x00private"
			}
			r, err := a.Handle(context.Background(), imageInput("cleanup", "caption"))
			if err != nil || r.Usage.TotalTokens != 37 || send.calls != 1 || strings.Contains(send.text, "private") {
				t.Fatalf("cleanup usage/output %+v %v", r, err)
			}
		})
	}
}

func TestImageAmbiguousSendNeverReplays(t *testing.T) {
	s, text, vision, search, send, image, a := imageFixture(t)
	send.err = errors.New("unknown ack")
	in := imageInput("ambiguous", "caption")
	r, err := a.Handle(context.Background(), in)
	if err == nil || r.State != "ambiguous" || r.Usage.TotalTokens != 37 {
		t.Fatalf("ambiguous %+v %v", r, err)
	}
	b, err := NewWithImages(s, text, search, send, a.opts, vision, image)
	if err != nil {
		t.Fatal(err)
	}
	r, err = b.Handle(context.Background(), in)
	if err != nil || r.State != "duplicate" || send.calls != 1 || vision.calls != 1 || image.calls != 1 {
		t.Fatal("ambiguous repeated")
	}
}

func TestImageModelReturnIsJoinedAfterCancellation(t *testing.T) {
	_, _, vision, _, send, image, a := imageFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan Result, 1)
	var once sync.Once
	releaseModel := func() { once.Do(func() { close(release) }) }
	// Register release/cancel/join before launching the held model.
	t.Cleanup(func() {
		cancel()
		releaseModel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("held Handle failed to join")
		}
	})
	vision.hook = func(context.Context) { close(entered); <-release }
	go func() { r, _ := a.Handle(ctx, imageInput("held", "caption")); done <- r }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("vision model never entered")
	}
	cancel()
	select {
	case r := <-done:
		done <- r
		t.Fatal("Handle returned before actual model")
	case <-time.After(30 * time.Millisecond):
	}
	if send.calls != 0 || image.calls != 1 {
		t.Fatal("send preceded actual model return")
	}
	releaseModel()
	select {
	case r := <-done:
		done <- r
		if r.State != "failed" || r.Code != "canceled" || r.Usage.TotalTokens != 37 || send.calls != 0 {
			t.Fatalf("actual canceled return %+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("actual model return failed to join")
	}
}

func TestImageContextArraysCountTextAndRejectUnboundedForms(t *testing.T) {
	o := Options{ContextBytes: 16384, ContextTokens: 20000}
	part := provider.ContentPart{Type: "image_url", ImageURL: &provider.ImageURL{URL: "data:image/png;base64,AAAA", Detail: "low"}}
	for _, test := range []struct {
		name    string
		content any
		want    bool
	}{
		{"bounded", []provider.ContentPart{{Type: "text", Text: strings.Repeat("x", 16300)}, part}, true},
		{"text overflow", []provider.ContentPart{{Type: "text", Text: strings.Repeat("x", 16385)}, part}, false},
		{"misplaced text", []provider.ContentPart{{Type: "image_url", Text: strings.Repeat("x", 16385), ImageURL: part.ImageURL}}, false},
		{"unknown", map[string]string{"text": "unbounded"}, false},
		{"two images", []provider.ContentPart{part, part}, false},
		{"image cap", []provider.ContentPart{{Type: "image_url", ImageURL: &provider.ImageURL{URL: strings.Repeat("x", (6<<20)+1)}}}, false},
		{"external image", []provider.ContentPart{{Type: "image_url", ImageURL: &provider.ImageURL{URL: "https://example.invalid/private"}}}, false},
		{"newline payload", []provider.ContentPart{{Type: "image_url", ImageURL: &provider.ImageURL{URL: "data:image/png;base64,\r\n\r\n"}}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := withinContext([]provider.Message{{Role: "user", Content: test.content}}, o); got != test.want {
				t.Fatalf("budget got %v want %v", got, test.want)
			}
		})
	}
	b, tokens := contextCost([]provider.Message{{Role: "user", Content: []provider.ContentPart{{Type: "text", Text: "hello"}, part}}})
	if b < 5 || b > 100 || tokens <= b {
		t.Fatalf("binary counted as text or overhead missing: %d/%d", b, tokens)
	}
}

func TestImageCurrentAndProfileMandatoryWhileOptionalContextDrops(t *testing.T) {
	part := provider.ContentPart{Type: "image_url", ImageURL: &provider.ImageURL{URL: "data:image/png;base64,AAAA", Detail: "low"}}
	last := provider.Message{Role: "user", Content: []provider.ContentPart{{Type: "text", Text: strings.Repeat("c", 6000)}, part}}
	snap := storage.ContextSnapshot{Profile: storage.Profile{ID: "custom", Name: "Synthetic", Style: strings.Repeat("p", 1800)}}
	for i := 0; i < 16; i++ {
		snap.History = append(snap.History, storage.Turn{Role: "user", Content: strings.Repeat("h", 2000)}, storage.Turn{Role: "assistant", Content: strings.Repeat("a", 2000)})
	}
	for i := 0; i < 8; i++ {
		snap.Facts = append(snap.Facts, storage.Fact{Content: strings.Repeat("f", 1600)})
	}
	messages, ok := buildSnapshotWithCurrent(last, snap, nil, Options{ContextBytes: 16384, ContextTokens: 10000})
	if !ok || len(messages) > 40 || !withinContext(messages, Options{ContextBytes: 16384, ContextTokens: 10000}) {
		t.Fatal("bounded context unavailable")
	}
	parts, ok := messages[len(messages)-1].Content.([]provider.ContentPart)
	if !ok || len(parts) != 2 || parts[0].Text != strings.Repeat("c", 6000) || parts[1].ImageURL.URL != "data:image/png;base64,AAAA" {
		t.Fatal("current image/caption dropped")
	}
	profileFound := false
	for _, m := range messages {
		if text, ok := m.Content.(string); ok && strings.Contains(text, strings.Repeat("p", 1800)) {
			profileFound = true
			if m.Role != "user" {
				t.Fatal("custom profile promoted to policy")
			}
		}
	}
	if !profileFound {
		t.Fatal("mandatory profile dropped as optional data")
	}
	if _, ok := buildSnapshotWithCurrent(last, snap, nil, Options{ContextBytes: 16384, ContextTokens: 7000}); ok {
		t.Fatal("over-budget mandatory profile silently removed")
	}
}

func TestImageProcessorConsumerIsOnceAndActuallyRequired(t *testing.T) {
	for _, skip := range []bool{false, true} {
		t.Run(fmt.Sprint(skip), func(t *testing.T) {
			_, _, vision, _, send, image, a := imageFixture(t)
			image.repeat = !skip
			image.skip = skip
			r, err := a.Handle(context.Background(), imageInput("consumer", "caption"))
			want := 1
			if skip {
				want = 0
			}
			if err != nil || r.State != "sent" || vision.calls != want || send.calls != 1 || strings.Contains(send.text, "synthetic visual") {
				t.Fatalf("consumer contract %+v %v model=%d", r, err, vision.calls)
			}
		})
	}
}

func TestImageDuplicateAfterActualDatabaseReopen(t *testing.T) {
	s, path, text, search, send, old := fixture(t)
	vision := &fakeModel{reply: provider.Reply{Text: "synthetic", FinishReason: "stop"}}
	image := &imageFake{part: provider.ContentPart{Type: "image_url", ImageURL: &provider.ImageURL{URL: "data:image/png;base64,AAAA"}}}
	a, err := NewWithImages(s, text, search, send, old.opts, vision, image)
	if err != nil {
		t.Fatal(err)
	}
	in := imageInput("reopen", "caption")
	if _, err = a.Handle(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	b, err := NewWithImages(reopened, text, search, send, old.opts, vision, image)
	if err != nil {
		t.Fatal(err)
	}
	r, err := b.Handle(context.Background(), in)
	if err != nil || r.State != "duplicate" || vision.calls != 1 || image.calls != 1 || send.calls != 1 {
		t.Fatalf("database restart replay %+v %v", r, err)
	}
}
