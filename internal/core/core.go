package core

import (
	"context"
	"errors"
	"github.com/hidxt/miskoai/internal/agent"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/document"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/service"
	"github.com/hidxt/miskoai/internal/storage"
	"github.com/hidxt/miskoai/internal/summary"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

type Channel interface {
	RawUpdates(context.Context, string) ([]byte, error)
	SendText(context.Context, string, string, string, string) error
}
type Dependencies struct {
	Model agent.Model
	// Vision and Images are borrowed synchronous dependencies. Supply both or neither.
	Vision         agent.Model
	Images         agent.ImageProcessor
	Search         agent.Searcher
	ChannelFactory func(config.Authorization) (Channel, error)
	// Documents is borrowed and optional; nil preserves legacy injected agents.
	Documents agent.DocumentProcessor
}

var (
	ErrUnavailable   = errors.New("controller unavailable")
	ErrUnconfigured  = errors.New("authorization not configured")
	ErrBusy          = errors.New("operation already active")
	ErrConfiguration = errors.New("invalid controller configuration")
	ErrMaintenance   = errors.New("maintenance failed; operator reconciliation may be required")
)

// Controller owns one store and its actual worker lifetimes. Do not copy it.
type Controller struct {
	mu                                                     sync.Mutex
	cfg                                                    config.Config
	desired, effective                                     config.Settings
	overrides                                              map[string]bool
	auth                                                   config.Authorization
	scope                                                  storage.Scope
	model                                                  agent.Model
	vision                                                 agent.Model
	images                                                 agent.ImageProcessor
	documents                                              agent.DocumentProcessor
	cdn                                                    *media.CDN
	search                                                 agent.Searcher
	channel                                                Channel
	providerReady                                          bool
	store                                                  *storage.Store
	lock                                                   *maintenance.Lock
	maintenance                                            chan struct{}
	downloads                                              chan struct{}
	leases                                                 int
	exclusive                                              bool
	changed                                                chan struct{}
	closed, closing, started, paused, expired, unavailable bool
	parent                                                 context.Context
	runCancel                                              context.CancelFunc
	generation                                             *generation
	serviceSnapshot                                        service.Status
	summarySnapshot                                        summary.Status
	startedAt                                              time.Time
	closeDone                                              chan struct{}
	closeErr                                               error
	metrics                                                Metrics
	events                                                 []Event
}

func New(cfg config.Config, d Dependencies) (c *Controller, err error) {
	// Reject partial borrowed media dependencies before filesystem effects.
	if (d.Vision == nil) != (d.Images == nil) {
		return nil, ErrConfiguration
	}
	s := cfg.EffectiveSettings()
	if s == (config.Settings{}) || s != (config.Settings{Listen: cfg.Listen, DeepSeekURL: cfg.DeepSeekURL, Model: cfg.Model, VisionModel: cfg.VisionModel, WeixinURL: cfg.WeixinURL, MaxOutput: cfg.MaxOutput, ContextBytes: cfg.ContextBytes, ContextTokens: cfg.ContextTokens}) || len(cfg.AdminPassword) < 16 || len(cfg.AdminPassword) > 256 || !utf8.ValidString(cfg.AdminPassword) {
		return nil, ErrConfiguration
	}
	for _, r := range cfg.AdminPassword {
		if r < 32 || r == 127 {
			return nil, ErrConfiguration
		}
	}
	dir, e := privatefs.Resolve(cfg.DataDir)
	if e != nil || privatefs.EnsureDir(dir) != nil {
		return nil, ErrConfiguration
	}
	cfg.DataDir = dir
	l, e := maintenance.Acquire(dir)
	if e != nil {
		return nil, ErrUnavailable
	}
	var ownedStore *storage.Store
	var ownedCDN *media.CDN
	defer func() {
		if err != nil {
			if ownedCDN != nil && ownedCDN.Close() != nil {
				err = ErrUnavailable
			}
			if ownedStore != nil && ownedStore.Close() != nil {
				err = ErrUnavailable
			}
			if l.Close() != nil {
				err = ErrUnavailable
			}
		}
	}()
	c = &Controller{cfg: cfg, desired: cfg.DesiredSettings(), effective: s, lock: l, maintenance: make(chan struct{}, 1), downloads: make(chan struct{}, 1), changed: make(chan struct{}), closeDone: make(chan struct{}), startedAt: time.Now(), overrides: make(map[string]bool)}
	for _, k := range []string{"listen", "deepseek_url", "model", "vision_model", "weixin_url", "max_output_tokens", "context_bytes", "context_tokens"} {
		if cfg.Overrides[k] {
			c.overrides[k] = true
		}
	}
	c.cfg.Overrides = nil
	c.auth, e = config.LoadAuthorization(dir)
	if e != nil && !errors.Is(e, config.ErrAuthorizationMissing) {
		return nil, ErrConfiguration
	}
	if e == nil {
		c.scope = storage.Scope{Account: c.auth.Account, User: c.auth.AllowedUser}
	}
	c.paused, e = maintenance.RestorePaused(dir)
	if e != nil {
		return nil, ErrMaintenance
	}
	path := filepath.Join(dir, "miskoai.db")
	if e = admitDB(path, true); e != nil {
		return nil, e
	}
	c.store, e = storage.Open(path)
	if e != nil {
		return nil, ErrUnavailable
	}
	ownedStore = c.store
	c.providerReady = d.Model != nil || cfg.DeepSeekKey != ""
	c.model = d.Model
	c.vision, c.images = d.Vision, d.Images
	c.documents = d.Documents
	if c.model == nil {
		var text *provider.DeepSeek
		text, e = provider.NewDeepSeek(s.DeepSeekURL, cfg.DeepSeekKey, s.Model)
		if e != nil {
			return nil, ErrConfiguration
		}
		c.model = text
		if c.vision == nil {
			// Clone the original before counted wrappers; both share admission.
			c.vision, e = text.WithModel(s.VisionModel)
			if e != nil {
				return nil, ErrConfiguration
			}
			c.cdn, e = media.NewCDN()
			if e != nil {
				return nil, ErrUnavailable
			}
			ownedCDN = c.cdn
			// One persistent parser and outer media slot survive generation and
			// database replacement. Explicit document injection stays borrowed.
			processor, processorErr := attachment.NewProcessor(dir, c.cdn, document.New())
			e = processorErr
			if e != nil {
				return nil, ErrConfiguration
			}
			c.images = processor
			if c.documents == nil {
				c.documents = processor
			}
		}
	}
	c.search = d.Search
	if c.search == nil && cfg.OllamaKey != "" {
		c.search, e = provider.NewSearch(cfg.OllamaKey)
		if e != nil {
			return nil, ErrConfiguration
		}
	}
	if c.scope.Account != "" {
		if d.ChannelFactory != nil {
			c.channel, e = d.ChannelFactory(c.auth)
		} else {
			c.channel, e = weixin.New(c.auth.BaseURL, c.auth.Token)
		}
		if e != nil || c.channel == nil {
			return nil, ErrConfiguration
		}
	}
	c.eventLocked("created")
	return c, nil
}
func admitDB(path string, create bool) error {
	var main os.FileInfo
	missingMain, journal := false, false
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		p := path + suffix
		info, e := os.Lstat(p)
		if errors.Is(e, os.ErrNotExist) {
			if suffix == "" {
				missingMain = true
			}
			continue
		}
		if e != nil || privatefs.CheckFile(p, math.MaxInt64) != nil {
			return ErrUnavailable
		}
		if suffix == "" {
			main = info
		} else {
			journal = true
		}
	}
	// A missing/empty main has no admitted SQLite history. Residual recovery
	// state must be preserved for the operator before creation or engine access.
	if journal && (missingMain || main.Size() == 0) {
		return ErrUnavailable
	}
	if missingMain {
		if !create {
			return ErrUnavailable
		}
		f, e := privatefs.Create(path)
		if e != nil {
			return ErrUnavailable
		}
		if f.Close() != nil {
			return ErrUnavailable
		}
	}
	return nil
}
