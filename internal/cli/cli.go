package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/notices"
	"io"
)

const Version = "0.1.0-dev"

func Run(args []string, out io.Writer) error {
	return RunContext(context.Background(), args, out)
}

func RunContext(ctx context.Context, args []string, out io.Writer) error {
	if ctx == nil {
		return errors.New("cli_context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "licenses" {
		_, err := io.WriteString(out, notices.Text)
		return err
	}
	if len(args) == 1 && args[0] == "version" {
		_, err := fmt.Fprintf(out, "MiskoAI %s\n", Version)
		return err
	}
	if len(args) == 0 || len(args) == 1 && args[0] == "help" {
		_, err := fmt.Fprintln(out, "MiskoAI development CLI: init | serve | status | doctor | config validate | backup PATH | restore PATH | poc deepseek|stream|search|vision PATH|weixin | weixin login | version | licenses")
		return err
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "init" {
		return initialize(c, out)
	}
	if len(args) == 1 && args[0] == "doctor" {
		return doctor(c, out)
	}
	if len(args) == 1 && args[0] == "serve" {
		return serve(ctx, c, out)
	}
	if len(args) == 1 && args[0] == "status" {
		return status(ctx, c, out)
	}
	if len(args) == 2 && args[0] == "config" && args[1] == "validate" {
		_, err := fmt.Fprintln(out, "configuration valid; live provider/account readiness is separate")
		return err
	}
	if len(args) == 2 && args[0] == "backup" {
		return backup(ctx, c, args[1], out)
	}
	if len(args) == 2 && args[0] == "restore" {
		return restore(ctx, c, args[1], out)
	}
	if len(args) >= 2 && args[0] == "poc" {
		return probe(c, args[1:], out)
	}
	if len(args) == 2 && args[0] == "weixin" && args[1] == "login" {
		return login(c, out)
	}
	return fmt.Errorf("unknown command; run miskoai help")
}
