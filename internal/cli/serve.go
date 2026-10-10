package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/web"
	"io"
)

type serveCore interface {
	Run(context.Context) error
	Close() error
}
type serveWeb interface {
	Run(context.Context) error
	Ready() <-chan struct{}
}
type serveConstructor func(config.Config) (serveCore, serveWeb, error)

func serve(ctx context.Context, cfg config.Config, out io.Writer) error {
	return serveWith(ctx, cfg, out, func(cfg config.Config) (serveCore, serveWeb, error) {
		c, e := core.New(cfg, core.Dependencies{})
		if e != nil {
			return nil, nil, e
		}
		w, e := web.New(web.Options{Listen: cfg.Listen, Password: cfg.AdminPassword}, web.Application(c), web.Assets())
		if e != nil {
			_ = c.Close()
			return nil, nil, e
		}
		return c, w, nil
	})
}

// Construction is immutable and local to this invocation. Every started Run is
// joined before releasing Core ownership, including failed output and shutdown.
func serveWith(ctx context.Context, cfg config.Config, out io.Writer, construct serveConstructor) error {
	if ctx == nil {
		return errors.New("cli_context")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	failure := errors.New("cli_serve")
	if len(cfg.AdminPassword) < 16 {
		return failure
	}
	c, w, e := construct(cfg)
	if e != nil {
		if c != nil {
			_ = c.Close()
		}
		return failure
	}
	if c == nil || w == nil {
		if c != nil {
			_ = c.Close()
		}
		return failure
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	wd := make(chan error, 1)
	cd := make(chan error, 1)
	go func() { wd <- w.Run(child) }()
	webJoined, coreStarted, coreJoined := false, false, false
	bad := false
	consume := func(e error) {
		if e != nil && !(child.Err() != nil && (errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded))) {
			bad = true
		}
	}
	select {
	case e = <-wd:
		webJoined = true
		consume(e)
		if ctx.Err() == nil {
			bad = true
		}
	case <-ctx.Done():
	case <-w.Ready():
		// Ready records past bind success. Check both cancellation and a completed
		// server result again before allowing any external workers to start.
		if child.Err() == nil {
			select {
			case e = <-wd:
				webJoined = true
				consume(e)
				if ctx.Err() == nil {
					bad = true
				}
			default:
			}
			if !webJoined && child.Err() == nil {
				if _, e = fmt.Fprintf(out, "MiskoAI %s listening on %s\n", Version, cfg.Listen); e != nil {
					bad = true
				} else if child.Err() == nil {
					// A blocked writer may outlive Web.Run; recheck after output as well.
					select {
					case e = <-wd:
						webJoined = true
						consume(e)
						if ctx.Err() == nil {
							bad = true
						}
					default:
					}
					if !webJoined && child.Err() == nil {
						coreStarted = true
						go func() { cd <- c.Run(child) }()
					}
				}
			}
		}
	}
	if coreStarted {
		select {
		case e = <-wd:
			webJoined = true
			consume(e)
			if ctx.Err() == nil {
				bad = true
			}
		case e = <-cd:
			coreJoined = true
			consume(e)
			if ctx.Err() == nil {
				bad = true
			}
		case <-ctx.Done():
		}
	}
	cancel()
	if !webJoined {
		consume(<-wd)
	}
	if coreStarted && !coreJoined {
		consume(<-cd)
	}
	if c.Close() != nil {
		bad = true
	}
	if bad {
		return failure
	}
	return nil
}
