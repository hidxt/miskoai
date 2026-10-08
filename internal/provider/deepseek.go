package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/hidxt/miskoai/internal/netx"
)

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}
type Usage struct {
	PromptTokens      int `json:"prompt_tokens"`
	CompletionTokens  int `json:"completion_tokens"`
	TotalTokens       int `json:"total_tokens"`
	CachedTokens      int `json:"prompt_cache_hit_tokens"`
	CompletionDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}
type Reply struct {
	Text         string
	Usage        Usage
	FinishReason string
}
type DeepSeek struct {
	transport  *netx.Client
	key, model string
}

func NewDeepSeek(base, key, model string) (*DeepSeek, error) {
	if strings.TrimSpace(model) == "" || len(model) > 128 || strings.ContainsAny(key, "\r\n") || len(key) > 1024 {
		return nil, errors.New("invalid model configuration")
	}
	c, err := netx.New(base, []string{"api.deepseek.com"}, 60*time.Second, 2<<20)
	if err != nil {
		return nil, err
	}
	return &DeepSeek{c, key, model}, nil
}

func (d *DeepSeek) request(messages []Message, maxTokens int, stream bool) (map[string]any, error) {
	if d.key == "" {
		return nil, errors.New("DeepSeek key is not configured")
	}
	if len(messages) == 0 || len(messages) > 40 || maxTokens < 1 || maxTokens > 4096 {
		return nil, errors.New("model request bounds exceeded")
	}
	textBytes := 0
	for _, m := range messages {
		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" {
			return nil, errors.New("invalid message role")
		}
		switch content := m.Content.(type) {
		case string:
			textBytes += len(content)
		case []ContentPart:
			if m.Role != "user" || len(content) > 4 {
				return nil, errors.New("invalid image message")
			}
			images := 0
			for _, p := range content {
				switch p.Type {
				case "text":
					textBytes += len(p.Text)
				case "image_url":
					images++
					if p.ImageURL == nil || (!strings.HasPrefix(p.ImageURL.URL, "data:image/png;base64,") && !strings.HasPrefix(p.ImageURL.URL, "data:image/jpeg;base64,") && !strings.HasPrefix(p.ImageURL.URL, "data:image/gif;base64,")) {
						return nil, errors.New("only validated inline images are accepted")
					}
					if len(p.ImageURL.URL) > 6<<20 {
						return nil, netx.ErrLimit
					}
				default:
					return nil, errors.New("invalid content type")
				}
			}
			if images > 1 {
				return nil, errors.New("one image per request is supported")
			}
		default:
			return nil, errors.New("invalid model content")
		}
	}
	if textBytes > 64<<10 {
		return nil, errors.New("context byte budget exceeded")
	}
	body := map[string]any{"model": d.model, "messages": messages, "max_tokens": maxTokens, "stream": stream, "reasoning_effort": "none"}
	if stream {
		body["stream_options"] = map[string]bool{"include_usage": true}
	}
	return body, nil
}

func (d *DeepSeek) Chat(ctx context.Context, messages []Message, maxTokens int) (Reply, error) {
	body, err := d.request(messages, maxTokens, false)
	if err != nil {
		return Reply{}, err
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	err = retrySafeStatus(ctx, func(attemptCtx context.Context) error {
		return d.transport.JSON(attemptCtx, "POST", "/chat/completions", d.key, body, &result)
	})
	if err != nil {
		return Reply{}, err
	}
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return Reply{}, errors.New("empty model response")
	}
	c := result.Choices[0]
	if c.FinishReason != "stop" && c.FinishReason != "length" {
		return Reply{}, errors.New("model response did not finish successfully")
	}
	if len(c.Message.Content) > 32<<10 {
		return Reply{}, netx.ErrLimit
	}
	return Reply{c.Message.Content, result.Usage, c.FinishReason}, nil
}

func (d *DeepSeek) Stream(ctx context.Context, messages []Message, maxTokens int, emit func(string) error) (Usage, error) {
	body, err := d.request(messages, maxTokens, true)
	if err != nil {
		return Usage{}, err
	}
	if emit == nil {
		return Usage{}, errors.New("stream consumer required")
	}
	var usage Usage
	total := 0
	finished := false
	err = d.transport.SSE(ctx, "/chat/completions", d.key, body, func(data []byte) error {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *Usage          `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &chunk) != nil || len(chunk.Error) > 0 {
			return errors.New("invalid model stream event")
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil && *choice.FinishReason != "stop" && *choice.FinishReason != "length" {
				return errors.New("model stream failed")
			}
			text := choice.Delta.Content
			if finished && text != "" {
				return errors.New("model content after terminal finish")
			}
			total += len(text)
			if total > 32<<10 {
				return netx.ErrLimit
			}
			if text != "" {
				if err := emit(text); err != nil {
					return err
				}
			}
			if choice.FinishReason != nil {
				finished = true
			}
		}
		return nil
	})
	if err == nil && (!finished || total == 0) {
		return usage, errors.New("model stream lacked completed content")
	}
	return usage, err
}

// Only a received explicit 429/503 is retried. Transport timeouts and partial streams
// may have incurred tokens, so they are returned without automatic resubmission.
func retrySafeStatus(ctx context.Context, call func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := call(ctx)
		var status *netx.HTTPError
		if !errors.As(err, &status) || (status.Status != 429 && status.Status != 503) || attempt == 2 {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return errors.New("retry budget exceeded")
}
