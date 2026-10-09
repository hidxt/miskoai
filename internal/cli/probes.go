package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/provider"
)

func imageParts(path string) ([]provider.ContentPart, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return nil, errors.New("image must be a regular file up to 4MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot read image")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, errors.New("image read or size limit failure")
	}
	dimensions, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || dimensions.Width < 1 || dimensions.Height < 1 || dimensions.Width > 8192 || dimensions.Height > 8192 || int64(dimensions.Width)*int64(dimensions.Height) > 16_000_000 {
		return nil, errors.New("invalid image format or dimensions")
	}
	if format != "png" && format != "jpeg" {
		return nil, errors.New("vision PoC currently accepts PNG/JPEG; animated formats need bounded full validation")
	}
	// Decode the bounded image to reject header-valid truncated/corrupt content.
	// Pixel count bounds the decoder allocation; no decoded pixels are retained.
	if _, decodedFormat, decodeErr := image.Decode(bytes.NewReader(data)); decodeErr != nil || decodedFormat != format {
		return nil, errors.New("corrupt image content")
	}
	mime := ""
	switch format {
	case "png":
		mime = "image/png"
	case "jpeg":
		mime = "image/jpeg"
	default:
		return nil, errors.New("unsupported image format")
	}
	return []provider.ContentPart{{Type: "text", Text: "This is an authorized integration test. Briefly describe this image."}, {Type: "image_url", ImageURL: &provider.ImageURL{URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), Detail: "low"}}}, nil
}

// Probes are explicit development commands; no startup or doctor path calls them.
func probe(c config.Config, args []string, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if len(args) == 1 && args[0] == "weixin" {
		return liveWeixinProbe(ctx, c, out)
	}
	if len(args) == 1 && args[0] == "search" {
		s, err := provider.NewSearch(c.OllamaKey)
		if err != nil {
			return err
		}
		results, err := s.Search(ctx, "Ollama official cloud web search API", 3)
		if err != nil {
			return err
		}
		if len(results) == 0 {
			return errors.New("search probe returned no usable sources")
		}
		_, err = fmt.Fprintf(out, "live search returned %d validated source links\n", len(results))
		return err
	}
	model := c.Model
	if len(args) > 0 && args[0] == "vision" {
		model = c.VisionModel
	}
	d, err := provider.NewDeepSeek(c.DeepSeekURL, c.DeepSeekKey, model)
	if err != nil {
		return err
	}
	messages := []provider.Message{{Role: "user", Content: "This is a synthetic integration test. Reply with the word MiskoAI."}}
	if len(args) == 2 && args[0] == "vision" {
		parts, e := imageParts(args[1])
		if e != nil {
			return e
		}
		messages = []provider.Message{{Role: "user", Content: parts}}
	} else if len(args) != 1 || (args[0] != "deepseek" && args[0] != "stream") {
		return errors.New("usage: miskoai poc deepseek|stream|search|vision PATH")
	}
	if args[0] == "stream" {
		count := 0
		usage, err := d.Stream(ctx, messages, c.MaxOutput, func(text string) error { count += len(text); return nil })
		if err != nil {
			return err
		}
		if count == 0 {
			return errors.New("stream probe returned no content")
		}
		_, err = fmt.Fprintf(out, "live stream passed; output_bytes=%d total_tokens=%d\n", count, usage.TotalTokens)
		return err
	}
	reply, err := d.Chat(ctx, messages, c.MaxOutput)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "live %s request passed; output_bytes=%d total_tokens=%d finish=%s\n", args[0], len(reply.Text), reply.Usage.TotalTokens, reply.FinishReason)
	return err
}

type authorization struct {
	Token       string `json:"bot_token"`
	Account     string `json:"account"`
	AllowedUser string `json:"allowed_user"`
	BaseURL     string `json:"base_url"`
}

func saveAuthorization(c config.Config, status weixin.QRStatus) error {
	if status.BotToken == "" || status.BotID == "" || status.UserID == "" || len(status.BotToken) > 16<<10 || len(status.BotID) > 256 || len(status.UserID) > 256 {
		return errors.New("incomplete confirmed authorization")
	}
	if _, err := weixin.New(status.BaseURL, status.BotToken); err != nil {
		return err
	}
	if err := privateDir(c.DataDir); err != nil {
		return err
	}
	file, err := privatefs.Create(filepath.Join(c.DataDir, "weixin-auth.json"))
	if err != nil {
		return errors.New("authorization file exists or cannot be created; inspect existing account before replacing")
	}
	err = json.NewEncoder(file).Encode(authorization{status.BotToken, status.BotID, status.UserID, status.BaseURL})
	syncErr := file.Sync()
	closeErr := file.Close()
	if err != nil || syncErr != nil || closeErr != nil {
		return errors.New("authorization persistence failed")
	}
	return nil
}

func login(c config.Config, out io.Writer) error {
	if err := privateDir(c.DataDir); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(c.DataDir, "weixin-auth.json")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("authorization file already exists or cannot be inspected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, err := weixin.New(c.WeixinURL, "")
	if err != nil {
		return err
	}
	qr, err := client.StartQR(ctx)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(out, "Private QR authorization link (do not log or share):\n%s\n", qr.DisplayContent); err != nil {
		return err
	}
	base := weixin.DefaultBaseURL
	verify := ""
	for {
		status, err := client.QRStatusAt(ctx, base, qr.Code, verify)
		if err != nil {
			return err
		}
		switch status.Status {
		case "confirmed":
			if err = saveAuthorization(c, status); err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, "WeChat authorization stored privately; message/reconnect/media tests still required")
			return err
		case "expired", "verify_code_blocked":
			return errors.New("QR authorization expired or verification blocked; start a new authorized login")
		case "need_verifycode":
			if verify != "" {
				return errors.New("verification not accepted; retry login after checking the phone")
			}
			if _, err = fmt.Fprintln(out, "Enter the one-time code from your phone (hidden terminal input):"); err != nil {
				return err
			}
			verify, err = terminalVerification(ctx)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintln(out); err != nil {
				return err
			}
		case "scaned_but_redirect", "binded_redirect", "scaned":
			base, verify, err = nextQRState(status, base, verify)
			if err != nil {
				return err
			}
		case "wait":
		default:
			return errors.New("unrecognized QR authorization status")
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
