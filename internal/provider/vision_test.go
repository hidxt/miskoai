package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hidxt/miskoai/internal/netx"
)

func visionClient(t *testing.T) *DeepSeek {
	t.Helper()
	d, err := NewDeepSeek("https://api.deepseek.com/v1", "synthetic-key", "text-model")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestVisionCloneSharesTransportAndModelIsolation(t *testing.T) {
	d := visionClient(t)
	clone, err := d.WithModel("vision-model")
	if err != nil {
		t.Fatal(err)
	}
	if clone == d || clone.transport != d.transport || clone.key != d.key || clone.model != "vision-model" || d.model != "text-model" {
		t.Fatal("clone did not preserve isolated wrapper and exact shared transport")
	}
	for _, model := range []string{"", " \t", strings.Repeat("x", 129)} {
		if _, err := d.WithModel(model); err == nil {
			t.Fatal("invalid model accepted")
		}
	}
	for _, source := range []*DeepSeek{nil, {key: "synthetic-key", model: "text-model"}} {
		if _, err := source.WithModel("vision-model"); err == nil {
			t.Fatal("unconfigured clone accepted")
		}
	}
	for _, model := range []string{strings.Repeat("x", 128), " model "} {
		if _, err := d.WithModel(model); err != nil {
			t.Fatal("constructor-compatible model refused")
		}
	}
	withoutKey, err := NewDeepSeek("https://api.deepseek.com", "", "text-model")
	if err != nil {
		t.Fatal(err)
	}
	if clone, err := withoutKey.WithModel("vision-model"); err != nil || clone.key != "" {
		t.Fatal("clone changed constructor credential policy")
	}
	var models []string
	d.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		if r.URL.String() != "https://api.deepseek.com/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-key" {
			return nil, errors.New("wrong endpoint or credential")
		}
		models = append(models, body.Model)
		return fixture(200, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
	}))
	for _, client := range []*DeepSeek{d, clone} {
		if _, err := client.Chat(context.Background(), []Message{{"user", "hi"}}, 1); err != nil {
			t.Fatal(err)
		}
	}
	if len(models) != 2 || models[0] != "text-model" || models[1] != "vision-model" {
		t.Fatal("model selection changed source")
	}
}

func TestVisionSharedAdmissionIncludesOriginalAndClone(t *testing.T) {
	d := visionClient(t)
	clone, err := d.WithModel("vision-model")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	entered := make(chan struct{}, 3)
	results := make(chan error, 3)
	var calls atomic.Int32
	var wg sync.WaitGroup
	var once sync.Once
	t.Cleanup(func() { cancel(); once.Do(func() { close(release) }); wg.Wait() })
	d.transport.SetTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return fixture(200, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
	}))
	run := func(c *DeepSeek, cctx context.Context) {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := c.Chat(cctx, []Message{{"user", "hi"}}, 1); results <- err }()
	}
	run(d, ctx)
	run(clone, ctx)
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("two actual requests did not enter")
		}
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 80*time.Millisecond)
	defer waitCancel()
	run(clone, waitCtx)
	select {
	case err := <-results:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("wait: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waiter did not cancel")
	}
	if calls.Load() != 2 {
		t.Fatal("canceled waiter reached transport")
	}
	once.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("held request not released")
		}
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected attempts")
	}
	if _, err := d.Chat(ctx, []Message{{"user", "after"}}, 1); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("admission not released")
	}
}

type visionReader struct {
	data    []byte
	calls   int
	maxRead int
	failure error
	cancel  context.CancelFunc
}

func (r *visionReader) ReadAt(p []byte, off int64) (int, error) {
	r.calls++
	if len(p) > r.maxRead {
		r.maxRead = len(p)
	}
	if r.cancel != nil {
		r.cancel()
	}
	if r.failure != nil {
		return 0, r.failure
	}
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestInlineImageMaximumAndRefusalBeforeRead(t *testing.T) {
	r := &visionReader{data: bytes.Repeat([]byte{0xa5}, 4<<20)}
	p, err := InlineImage(context.Background(), r, int64(len(r.data)), "png")
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != "image_url" || p.ImageURL == nil || p.ImageURL.Detail != "low" || len(p.ImageURL.URL) != len("data:image/png;base64,")+5592408 || r.maxRead > 32<<10 {
		t.Fatal("maximum encoding bounds")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(p.ImageURL.URL, "data:image/png;base64,"))
	if err != nil || !bytes.Equal(decoded, r.data) {
		t.Fatal("maximum encoding differs")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx    context.Context
		size   int64
		format string
		input  io.ReaderAt
	}{{context.Background(), 0, "png", r}, {context.Background(), -1, "png", r}, {context.Background(), 4194305, "png", r}, {context.Background(), 1, "PNG", r}, {context.Background(), 1, "webp", r}, {nil, 1, "png", r}, {canceled, 1, "png", r}, {context.Background(), 1, "png", nil}} {
		before := r.calls
		p, err := InlineImage(tc.ctx, tc.input, tc.size, tc.format)
		if err == nil || p.ImageURL != nil || r.calls != before {
			t.Fatal("preflight read or partial result")
		}
	}
}

func TestInlineImageCanonicalEncodingAndFailures(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		for _, size := range []int{1, 2, 3, 32767, 32768, 32769} {
			data := bytes.Repeat([]byte{0, 0xff, 0x31}, (size+2)/3)[:size]
			p, err := InlineImage(context.Background(), bytes.NewReader(data), int64(size), format)
			if err != nil {
				t.Fatal(err)
			}
			if p.ImageURL.URL != "data:image/"+format+";base64,"+base64.StdEncoding.EncodeToString(data) {
				t.Fatal("noncanonical encoding")
			}
		}
	}
	for _, r := range []*visionReader{{data: []byte{1}}, {failure: errors.New("PRIVATE-READER-CANARY")}} {
		p, err := InlineImage(context.Background(), r, 2, "png")
		if err == nil || p.ImageURL != nil || strings.Contains(err.Error(), "PRIVATE-READER-CANARY") {
			t.Fatal("unsafe reader failure")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &visionReader{data: bytes.Repeat([]byte{1}, 65536), cancel: cancel}
	p, err := InlineImage(ctx, r, 65536, "png")
	if !errors.Is(err, context.Canceled) || p.ImageURL != nil || r.calls != 1 {
		t.Fatal("cooperative cancellation lost")
	}
}

func TestOneImageAcrossMessagesRefusesBeforeMarshal(t *testing.T) {
	d := visionClient(t)
	var calls atomic.Int32
	d.transport.SetTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return fixture(200, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
	}))
	image := ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,AQ==", Detail: "low"}}
	messages := []Message{{"user", []ContentPart{image}}, {"user", []ContentPart{image}}}
	if body, err := d.request(messages, 1, false); err == nil || body != nil {
		t.Error("multiple user arrays passed request preflight")
	}
	if _, err := d.Chat(context.Background(), messages, 1); err == nil {
		t.Error("multiple images accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("refusal reached RoundTripper")
	}
}

func TestMaximumVisionJSONBound(t *testing.T) {
	d := visionClient(t)
	d, err := d.WithModel(strings.Repeat("m", 128))
	if err != nil {
		t.Fatal(err)
	}
	p, err := InlineImage(context.Background(), bytes.NewReader(make([]byte, 4<<20)), 4<<20, "jpeg")
	if err != nil {
		t.Fatal(err)
	}
	messages := []Message{{"user", []ContentPart{{Type: "text", Text: strings.Repeat("\x00", 64<<10)}, p}}}
	for len(messages) < 40 {
		messages = append(messages, Message{"assistant", ""})
	}
	body, err := d.request(messages, 4096, true)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > netx.MaxRequest {
		t.Fatal("maximum escaped JSON exceeds request bound")
	}
	t.Logf("maximum synthetic image JSON bytes=%d cap=%d", len(encoded), netx.MaxRequest)
	messages[0].Content = []ContentPart{{Type: "text", Text: strings.Repeat("x", (64<<10)+1)}, p}
	if _, err := d.request(messages, 4096, false); err == nil {
		t.Fatal("text aggregate accepted")
	}
}

type dishonestVisionReader struct{}

func (dishonestVisionReader) ReadAt(p []byte, off int64) (int, error) { p[0] = 1; return 1, nil }

func TestInlineImageDishonestReaderRefusesPartialSuccess(t *testing.T) {
	p, err := InlineImage(context.Background(), dishonestVisionReader{}, 2, "png")
	if err == nil || p.ImageURL != nil {
		t.Fatal("short ReaderAt without error published content")
	}
}

type heldVisionReader struct {
	entered chan struct{}
	release chan struct{}
}

func (r *heldVisionReader) ReadAt(p []byte, off int64) (int, error) {
	close(r.entered)
	<-r.release
	p[0] = 1
	return 1, nil
}
func TestInlineImageCancellationWaitsForReaderReturn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &heldVisionReader{make(chan struct{}), make(chan struct{})}
	result := make(chan error, 1)
	var once sync.Once
	var wg sync.WaitGroup
	t.Cleanup(func() { cancel(); once.Do(func() { close(r.release) }); wg.Wait() })
	wg.Add(1)
	go func() {
		defer wg.Done()
		p, err := InlineImage(ctx, r, 1, "png")
		if p.ImageURL != nil {
			result <- errors.New("partial result returned")
			return
		}
		result <- err
	}()
	select {
	case <-r.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("reader not entered")
	}
	cancel()
	select {
	case <-result:
		t.Fatal("returned before actual blocked reader completed")
	case <-time.After(30 * time.Millisecond):
	}
	once.Do(func() { close(r.release) })
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation not preserved")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reader did not join")
	}
}

func TestVisionContentFieldsRefuseBeforeMarshal(t *testing.T) {
	url := "data:image/png;base64,AQ=="
	for _, tc := range []struct {
		name string
		part ContentPart
	}{
		{"image_with_large_text", ContentPart{Type: "image_url", Text: strings.Repeat("x", (64<<10)+1), ImageURL: &ImageURL{URL: url}}},
		{"image_with_small_text", ContentPart{Type: "image_url", Text: "unexpected", ImageURL: &ImageURL{URL: url}}},
		{"text_with_large_image", ContentPart{Type: "text", Text: "caption", ImageURL: &ImageURL{URL: strings.Repeat("x", netx.MaxRequest+1)}}},
		{"text_with_small_image", ContentPart{Type: "text", Text: "caption", ImageURL: &ImageURL{URL: url}}},
		{"huge_detail", ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: url, Detail: strings.Repeat("x", netx.MaxRequest+1)}}},
		{"unknown_detail", ContentPart{Type: "image_url", ImageURL: &ImageURL{URL: url, Detail: "unexpected"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := visionClient(t)
			var calls atomic.Int32
			d.transport.SetTransport(transportFunc(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return fixture(200, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
			}))
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			messages := []Message{{"user", []ContentPart{tc.part}}}
			for _, stream := range []bool{false, true} {
				if body, err := d.request(messages, 1, stream); err == nil || body != nil {
					t.Error("mixed or unbounded serialized fields passed preflight")
				}
			}
			if _, err := d.Chat(ctx, messages, 1); err == nil {
				t.Error("malformed Chat accepted")
			}
			if _, err := d.Stream(ctx, messages, 1, func(string) error { return nil }); err == nil {
				t.Error("malformed Stream accepted")
			}
			if calls.Load() != 0 {
				t.Errorf("malformed input reached RoundTripper %d times", calls.Load())
			}
		})
	}
}

func TestVisionContentFieldsPreserveSupportedDetailsAndTextBounds(t *testing.T) {
	d := visionClient(t)
	for _, detail := range []string{"", "low", "high", "auto"} {
		messages := []Message{{"system", strings.Repeat("a", 32<<10)}, {"user", []ContentPart{{Type: "text", Text: strings.Repeat("b", 32<<10)}, {Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,AQ==", Detail: detail}}}}}
		if body, err := d.request(messages, 1, false); err != nil || body == nil {
			t.Fatalf("supported detail/exact64KiB refused: %q %v", detail, err)
		}
		messages = append(messages, Message{"assistant", "x"})
		if body, err := d.request(messages, 1, true); err == nil || body != nil {
			t.Fatal("aggregate64KiB+1 accepted")
		}
	}
}

func TestVisionInlineURLSyntaxRefusesBeforeMarshal(t *testing.T) {
	for _, payload := range []string{"", "A", "AQ=", "AQ===", "AQ==AAAA", "AR==", "AQJ=", "A===", "====", "AA_A", "AA-A", "AA\nA", "AA\rA", "AA A", "AA\x00A", "AA\"A", "AA<A", "AA\\A", strings.Repeat("\x00", (6<<20)-24)} {
		d := visionClient(t)
		var calls atomic.Int32
		d.transport.SetTransport(transportFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return fixture(200, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`), nil
		}))
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		messages := []Message{{"user", []ContentPart{{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64," + payload}}}}}
		for _, stream := range []bool{false, true} {
			if body, err := d.request(messages, 1, stream); err == nil || body != nil {
				t.Errorf("invalid URL payload size%d passed preflight", len(payload))
			}
		}
		if _, err := d.Chat(ctx, messages, 1); err == nil {
			t.Error("invalid URL Chat accepted")
		}
		if _, err := d.Stream(ctx, messages, 1, func(string) error { return nil }); err == nil {
			t.Error("invalid URL Stream accepted")
		}
		if calls.Load() != 0 {
			t.Errorf("invalid URL reached RoundTripper %d times", calls.Load())
		}
	}
}

func TestVisionCanonicalURLBounds(t *testing.T) {
	d := visionClient(t)
	d, err := d.WithModel(strings.Repeat("m", 128))
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"png", "jpeg", "gif"} {
		for _, payload := range []string{"AQ==", "AQI=", "AQID", "////", "++++"} {
			if _, err := d.request([]Message{{"user", []ContentPart{{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/" + format + ";base64," + payload}}}}}, 1, false); err != nil {
				t.Fatal("canonical URL refused", err)
			}
		}
	}
	prefix := "data:image/jpeg;base64,"
	payload := strings.Repeat("A", ((6<<20)-len(prefix))/4*4)
	messages := []Message{{"user", []ContentPart{{Type: "text", Text: strings.Repeat("\x00", 64<<10)}, {Type: "image_url", ImageURL: &ImageURL{URL: prefix + payload, Detail: "auto"}}}}}
	for len(messages) < 40 {
		messages = append(messages, Message{"assistant", ""})
	}
	body, err := d.request(messages, 4096, true)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(body)
	if err != nil || len(encoded) > netx.MaxRequest {
		t.Fatal("maximum accepted canonical URL escaped JSON limit", err)
	}
	t.Logf("maximum accepted canonical URL bytes=%d JSON bytes=%d cap=%d", len(prefix)+len(payload), len(encoded), netx.MaxRequest)
	messages[0].Content = []ContentPart{{Type: "image_url", ImageURL: &ImageURL{URL: prefix + payload + "AAAA"}}}
	if body, err := d.request(messages, 1, false); err == nil || body != nil {
		t.Fatal("canonical URL above6MiB accepted")
	}
}
