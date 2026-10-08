package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/storage"
)

type probeChannel interface {
	GetUpdates(context.Context, string) (weixin.Updates, error)
	SendText(context.Context, string, string, string, string) error
}

func liveWeixinProbe(ctx context.Context, c config.Config, out io.Writer) error {
	path := filepath.Join(c.DataDir, "weixin-auth.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("private WeChat authorization file unavailable or unsafe")
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("cannot read private authorization")
	}
	defer f.Close()
	var a authorization
	if json.NewDecoder(io.LimitReader(f, (64<<10)+1)).Decode(&a) != nil || a.Account == "" || a.AllowedUser == "" {
		return errors.New("invalid private authorization")
	}
	client, err := weixin.New(a.BaseURL, a.Token)
	if err != nil {
		return err
	}
	s, err := storage.Open(filepath.Join(c.DataDir, "miskoai.db"))
	if err != nil {
		return err
	}
	defer s.Close()
	return weixinProbe(ctx, client, s, a, out)
}

// One controlled synthetic reply at most. This PoC does not claim durable batch
// cursor ingestion or general chat service semantics; those belong to Phase2.
func weixinProbe(ctx context.Context, client probeChannel, s *storage.Store, a authorization, out io.Writer) error {
	updates, err := client.GetUpdates(ctx, "")
	if err != nil {
		return err
	}
	for _, message := range updates.Messages {
		if message.FromUserID != a.AllowedUser || message.GroupID != "" || message.MessageType != 1 || message.ContextToken == "" || message.MessageID.String() == "" {
			continue
		}
		var text strings.Builder
		for _, item := range message.Items {
			if item.Type == 1 && item.Text != nil {
				text.WriteString(item.Text.Text)
				if text.Len() > 16<<10 {
					return errors.New("probe text exceeds limit")
				}
			}
		}
		// The finite live authorization is synthetic-only. Do not claim/store or
		// reply to older ordinary chats delivered by this first cursor-less poll.
		if text.String() != "MiskoAI synthetic integration test" {
			continue
		}
		if text.Len() == 0 {
			continue
		}
		scope := storage.Scope{Account: a.Account, User: a.AllowedUser}
		id := message.MessageID.String()
		claimed, err := s.ClaimMessage(ctx, scope, id, text.String())
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		sum := sha256.Sum256([]byte(a.Account + ":" + id))
		clientID := "miskoai-poc-" + hex.EncodeToString(sum[:])
		if err = s.SetMessageState(ctx, scope, id, "sending"); err != nil {
			return err
		}
		reply := "MiskoAI text integration test passed."
		if err = client.SendText(ctx, a.AllowedUser, message.ContextToken, clientID, reply); err != nil {
			if stateErr := s.SetMessageState(ctx, scope, id, "ambiguous"); stateErr != nil {
				return errors.New("send and persistence failed; reconcile before further testing")
			}
			return err
		}
		if err = s.CompleteMessage(ctx, scope, id, reply); err != nil {
			return errors.New("send acknowledged but persistence failed; reconcile before further testing")
		}
		_, err = fmt.Fprintln(out, "live controlled WeChat text receive/reply acknowledged; payloads and credentials omitted")
		return err
	}
	return errors.New("no new authorized direct text message; send a synthetic test message and retry explicitly")
}

func nextQRState(status weixin.QRStatus, base, code string) (string, string, error) {
	switch status.Status {
	case "binded_redirect":
		return "", "", errors.New("account is already bound; inspect existing authorization in the official client before a new login")
	case "scaned":
		return base, "", nil
	case "scaned_but_redirect":
		host := status.RedirectHost
		next := host
		if !strings.HasPrefix(host, "https://") {
			next = "https://" + host
		}
		if _, err := weixin.New(next, ""); err != nil {
			return "", "", err
		}
		return next, code, nil
	default:
		return base, code, nil
	}
}
