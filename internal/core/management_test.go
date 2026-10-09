package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/maintenance"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagementFixedScopeAndProvenance(t *testing.T) {
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
	ctx := context.Background()
	id, e := c.AddFact(ctx, storage.Fact{Content: "synthetic fact", Source: "model", Confidence: .1})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.FactsPage(ctx, "", 0, 16)
	if e != nil || len(p.Items) != 1 || p.Items[0].Source != "explicit_user" || p.Items[0].Confidence != 1 {
		t.Fatal(p, e)
	}
	if e = c.UpdateFact(ctx, storage.Fact{ID: id, Content: "edited", Source: "model"}); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Candidates(ctx, 9); !errors.Is(e, storage.ErrInvalid) {
		t.Fatal("candidate cap", e)
	}
	if _, e = c.FactsPage(ctx, "", 0, 17); !errors.Is(e, storage.ErrInvalid) {
		t.Fatal("page cap", e)
	}
	if _, e = c.ActiveProfile(ctx); e != nil {
		t.Fatal(e)
	}
	if e = c.DeleteFact(ctx, id); e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = c.AddFact(ctx, storage.Fact{Content: "after close"}); !errors.Is(e, ErrUnavailable) {
		t.Fatal("post-close mutation", e)
	}
}
func TestSettingsSnapshotsAndRestart(t *testing.T) {
	cfg := fixtureConfig(t)
	cfg.Overrides["model"] = true
	cfg.Overrides["secret-canary"] = true
	c, e := New(cfg, Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	cfg.Overrides["listen"] = true
	d, effective, o := c.Settings()
	if o["secret-canary"] || o["listen"] {
		t.Fatal("external overrides retained")
	}
	o["listen"] = true
	d.Model = "next-start"
	if e = c.SaveSettings(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	want, got, o := c.Settings()
	if want.Model != "next-start" || got != effective || o["listen"] {
		t.Fatal("desired/effective not independent")
	}
	d.Model = ""
	if e = c.SaveSettings(context.Background(), d); e == nil {
		t.Fatal("invalid desired settings persisted")
	}
}
func TestClearLockOrderAndStaleSummary(t *testing.T) {
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	c := authorized(t, modelFunc(func(ctx context.Context, _ []provider.Message, n int) (provider.Reply, error) {
		if n != 1024 {
			t.Error("unexpected chat")
			return provider.Reply{}, ErrUnavailable
		}
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return provider.Reply{Text: `{"summary":"stale summary","candidates":[]}`, FinishReason: "stop"}, nil
	}), &fakeChannel{poll: idle})
	seed(t, c, 16)
	_, e := c.AddFact(context.Background(), storage.Fact{Content: "clear me"})
	if e != nil {
		t.Fatal(e)
	}
	cancel, done := runCore(t, c)
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.generation != nil })
	c.mu.Lock()
	g := c.generation
	c.mu.Unlock()
	g.summary.Wake()
	waitSignal(t, entered)
	clearDone := make(chan error, 1)
	go func() { clearDone <- c.ClearMemory(context.Background()) }()
	waitSignal(t, canceled)
	select {
	case e := <-clearDone:
		t.Fatal("clear returned before actual summary join", e)
	default:
	}
	if _, e = c.FactsPage(context.Background(), "", 0, 8); e != nil {
		t.Fatal("clear held exclusive lease while waiting for actual join", e)
	}
	close(release)
	select {
	case e = <-clearDone:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("clear deadlock")
	}
	d, e := c.Derived(context.Background())
	if e != nil || d.Text != "" {
		t.Fatal("stale summary committed", d, e)
	}
	p, e := c.FactsPage(context.Background(), "", 0, 8)
	if e != nil || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
	c.mu.Lock()
	fresh := c.generation
	c.mu.Unlock()
	if fresh == nil || fresh == g {
		t.Fatal("eligible generation did not restart")
	}
	cancel()
	waitSignal(t, done)
}
func TestDownloadReleasesStoreBeforeTransfer(t *testing.T) {
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
	_, e := c.AddFact(context.Background(), storage.Fact{Content: "synthetic download"})
	if e != nil {
		t.Fatal(e)
	}
	for _, backup := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "backup"}[backup], func(t *testing.T) {
			var d *Download
			var e error
			if backup {
				d, e = c.PrepareBackup(context.Background())
			} else {
				d, e = c.PrepareMemoryExport(context.Background())
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = c.PrepareBackup(context.Background()); !errors.Is(e, ErrBusy) {
				t.Fatal("download admission released early", e)
			}
			if e = c.ClearMemory(context.Background()); e != nil {
				t.Fatal("transfer retained store lease", e)
			}
			data, e := io.ReadAll(d)
			if e != nil || int64(len(data)) != d.Size {
				t.Fatal("transfer invalid", len(data), d.Size, e)
			}
			encoded, _ := json.Marshal(d)
			if bytes.Contains(encoded, []byte(c.cfg.DataDir)) {
				t.Fatal("private download path leaked")
			}
			if !backup && !bytes.Contains(data, []byte("synthetic download")) {
				t.Fatal("completed export changed after clear")
			}
			path := d.path
			if e = d.Close(); e != nil {
				t.Fatal(e)
			}
			if _, e = os.Lstat(path); !os.IsNotExist(e) {
				t.Fatal("owned download retained")
			}
			if e = d.Close(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestRestorePausedAcrossRestart(t *testing.T) {
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
	ctx := context.Background()
	_, e := c.AddFact(ctx, storage.Fact{Content: "original"})
	if e != nil {
		t.Fatal(e)
	}
	d, e := c.PrepareBackup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(d)
	if e != nil {
		t.Fatal(e)
	}
	if e = d.Close(); e != nil {
		t.Fatal(e)
	}
	_, e = c.AddFact(ctx, storage.Fact{Content: "after snapshot"})
	if e != nil {
		t.Fatal(e)
	}
	cancel, done := runCore(t, c)
	r, e := c.Restore(ctx, bytes.NewReader(data))
	if e != nil || !r.PreviousRetained || !r.ChannelPaused {
		t.Fatal(r, e)
	}
	if c.Status().ChannelState != "restore_paused" {
		t.Fatal(c.Status().ChannelState)
	}
	cancel()
	waitSignal(t, done)
	cfg := c.cfg
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	var polls atomic.Int32
	next, e := New(cfg, Dependencies{Model: modelFunc(reply), ChannelFactory: func(config.Authorization) (Channel, error) {
		return &fakeChannel{poll: func(ctx context.Context, s string) ([]byte, error) { polls.Add(1); return idle(ctx, s) }}, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	nextCancel, nextDone := runCore(t, next)
	if next.Status().ChannelState != "restore_paused" {
		t.Fatal("restart pause lost")
	}
	p, e := next.FactsPage(ctx, "", 0, 8)
	if e != nil || len(p.Items) != 1 {
		t.Fatal("wrong restored facts", p, e)
	}
	if polls.Load() != 0 {
		t.Fatal("restart remote effects")
	}
	if e = next.ResumeAfterReconciliation(ctx); e != nil {
		t.Fatal(e)
	}
	eventually(t, func() bool { return polls.Load() == 1 })
	nextCancel()
	waitSignal(t, nextDone)
}
func TestCloseWaitsExistingLeaseAndCancellation(t *testing.T) {
	cfg := fixtureConfig(t)
	c, e := New(cfg, Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	_, release, e := c.lease(context.Background(), false, false)
	if e != nil {
		t.Fatal(e)
	}
	closed := make(chan struct{})
	go func() { _ = c.Close(); close(closed) }()
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.closing })
	select {
	case <-closed:
		t.Fatal("close abandoned lease")
	default:
	}
	if c.Status().ChannelState == "closed" {
		t.Error("reported closed while an existing operation still owns store")
	}
	release()
	waitSignal(t, closed)
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
}

type heldReader struct {
	entered, release chan struct{}
	once             atomic.Bool
}

func (r *heldReader) Read(p []byte) (int, error) {
	if !r.once.Swap(true) {
		close(r.entered)
		<-r.release
	}
	return 0, io.EOF
}
func TestRestoreCanceledPreparationJoinsReader(t *testing.T) {
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
	before, e := os.ReadDir(c.cfg.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &heldReader{entered: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	go func() { _, e := c.Restore(ctx, r); done <- e }()
	waitSignal(t, r.entered)
	cancel()
	select {
	case e := <-done:
		t.Fatal("returned before blocked actual input read joined", e)
	default:
	}
	close(r.release)
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal("preparation cancellation classification lost", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restore read failed to join")
	}
	entries, e := os.ReadDir(c.cfg.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	if len(before) != len(entries) {
		t.Fatal("canceled restore changed directory entries")
	}
	for i, entry := range entries {
		if entry.Name() != before[i].Name() {
			t.Fatal("canceled restore retained unexpected artifact")
		}
	}
}

func TestBackupDownloadFailureCleansOriginalAndRecoversAdmission(t *testing.T) {
	for _, failure := range []string{"cap", "privacy", "open"} {
		t.Run(failure, func(t *testing.T) {
			c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
			var path string
			ops := downloadOperations{snapshot: func(ctx context.Context, s *storage.Store, p string) (os.FileInfo, error) {
				path = p
				return maintenance.BackupOwned(ctx, s, p)
			}}
			switch failure {
			case "cap":
				ops.maxBackupBytes = 1
			case "privacy":
				ops.check = func(string, int64) error { return privatefs.ErrUnsafe }
			case "open":
				ops.open = func(string) (*os.File, error) { return nil, os.ErrPermission }
			}
			d, e := c.prepareDownloadWithOperations(context.Background(), true, ops)
			if e == nil {
				if d != nil {
					_ = d.Close()
				}
				t.Error("post-production refusal accepted")
			}
			if _, e = os.Lstat(path); !os.IsNotExist(e) {
				t.Error("original owned snapshot leaked after refusal", e)
			}
			next, e := c.PrepareBackup(context.Background())
			if e != nil {
				t.Fatal("download admission not recovered", e)
			}
			if e = next.Close(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestBackupDownloadRefusesReplacementAndRetainsIt(t *testing.T) {
	for _, boundary := range []string{"after_production", "at_reopen"} {
		t.Run(boundary, func(t *testing.T) {
			c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
			var path string
			replace := func(p string) {
				t.Helper()
				if e := os.Rename(p, p+".held"); e != nil {
					t.Fatal(e)
				}
				f, e := privatefs.Create(p)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = f.WriteString("unowned synthetic replacement"); e != nil {
					t.Fatal(e)
				}
				if e = f.Close(); e != nil {
					t.Fatal(e)
				}
			}
			ops := downloadOperations{snapshot: func(ctx context.Context, s *storage.Store, p string) (os.FileInfo, error) {
				path = p
				owned, e := maintenance.BackupOwned(ctx, s, p)
				if e == nil && boundary == "after_production" {
					replace(p)
				}
				return owned, e
			}}
			if boundary == "at_reopen" {
				ops.open = func(p string) (*os.File, error) { replace(p); return os.Open(p) }
			}
			d, e := c.prepareDownloadWithOperations(context.Background(), true, ops)
			if e == nil {
				t.Error("unowned replacement offered for download")
				if d != nil {
					_ = d.Close()
				}
			}
			b, re := os.ReadFile(path)
			if re != nil || string(b) != "unowned synthetic replacement" {
				t.Error("unowned replacement deleted or changed", re)
			}
			if _, e = os.Stat(path + ".held"); e != nil {
				t.Error("original retained identity lost", e)
			}
			next, e := c.PrepareBackup(context.Background())
			if e != nil {
				t.Fatal("download admission not recovered", e)
			}
			if e = next.Close(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestBackupDownloadCloseRetainsReplacement(t *testing.T) {
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
	d, e := c.PrepareBackup(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	// Close only the test's read handle to permit the Windows rename fixture;
	// retained Download ownership must still refer to the producer's original.
	if e = d.file.Close(); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(d.path, d.path+".held"); e != nil {
		t.Fatal(e)
	}
	f, e := privatefs.Create(d.path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.WriteString("unowned close replacement"); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	if e = d.Close(); !errors.Is(e, ErrUnavailable) {
		t.Fatal("replaced download should refuse cleanup", e)
	}
	b, e := os.ReadFile(d.path)
	if e != nil || string(b) != "unowned close replacement" {
		t.Fatal("Close removed replacement", e)
	}
	next, e := c.PrepareBackup(context.Background())
	if e != nil {
		t.Fatal("Close did not release admission", e)
	}
	if e = next.Close(); e != nil {
		t.Fatal(e)
	}
}

func TestBackupCancellationAfterProductionCleansOriginal(t *testing.T) {
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var path string
	d, e := c.prepareDownloadWithOperations(ctx, true, downloadOperations{snapshot: func(ctx context.Context, s *storage.Store, p string) (os.FileInfo, error) {
		path = p
		info, e := maintenance.BackupOwned(ctx, s, p)
		cancel()
		return info, e
	}})
	if d != nil || !errors.Is(e, context.Canceled) {
		t.Fatal("canceled completed preparation admitted", e)
	}
	if _, e = os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("owned canceled backup leaked", e)
	}
	next, e := c.PrepareBackup(context.Background())
	if e != nil {
		t.Fatal("canceled admission retained", e)
	}
	if e = next.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestLeaseCancellationDoesNotFakeAdmission(t *testing.T) {
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: idle})
	_, release, e := c.lease(context.Background(), true, true)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e = c.lease(ctx, false, true); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	release()
	if _, e = c.FactsPage(context.Background(), "", 0, 8); e != nil {
		t.Fatal(e)
	}
}
func TestMetricsSafeAndSaturating(t *testing.T) {
	cfg := fixtureConfig(t)
	c, e := New(cfg, Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.mu.Lock()
	c.metrics.ChatAttempts = math.MaxUint64
	c.metrics.SuccessfulUsage.TotalTokens = math.MaxInt
	for i := 0; i < 200; i++ {
		c.eventLocked("settings_saved")
	}
	c.mu.Unlock()
	m := &countedModel{c: c, delegate: modelFunc(reply), operation: "chat"}
	if _, e = m.Chat(context.Background(), nil, 1); e != nil {
		t.Fatal(e)
	}
	s := c.Status()
	if s.Metrics.ChatAttempts != math.MaxUint64 || s.Metrics.SuccessfulUsage.TotalTokens != math.MaxInt || len(s.Events) != 128 || s.RSSAvailable {
		t.Fatal("metrics contract", s)
	}
	s.Events[0].Kind = "external"
	if c.Status().Events[0].Kind == "external" {
		t.Fatal("mutable events")
	}
	b, _ := json.Marshal(c.Status())
	for _, private := range []string{cfg.DataDir, cfg.AdminPassword, "fixture-account", "fixture-user"} {
		if strings.Contains(string(b), private) {
			t.Fatal("public status leaked private input")
		}
	}
}

type rejectingTransport struct{ calls atomic.Int32 }

func (r *rejectingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return nil, errors.New("synthetic unplanned network denied secret-canary")
}
func TestSyntheticUnplannedNetworkRejected(t *testing.T) {
	transport := &rejectingTransport{}
	client := &http.Client{Transport: transport}
	attempt := make(chan struct{})
	c := authorized(t, modelFunc(reply), &fakeChannel{poll: func(ctx context.Context, _ string) ([]byte, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://unplanned.invalid/", nil)
		_, e := client.Do(req)
		close(attempt)
		return nil, e
	}})
	cancel, done := runCore(t, c)
	waitSignal(t, attempt)
	cancel()
	waitSignal(t, done)
	if transport.calls.Load() != 1 {
		t.Fatal("synthetic rejected call missing")
	}
	b, _ := json.Marshal(c.Status())
	if bytes.Contains(b, []byte("secret-canary")) || bytes.Contains(b, []byte("unplanned.invalid")) {
		t.Fatal("raw transport error leaked")
	}
}
func TestUnconfiguredRefusesScopedAndMissingKeyNoStart(t *testing.T) {
	cfg := fixtureConfig(t)
	c, e := New(cfg, Dependencies{ChannelFactory: func(config.Authorization) (Channel, error) {
		t.Fatal("unconfigured channel constructed")
		return nil, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if _, e = c.FactsPage(context.Background(), "", 0, 8); !errors.Is(e, ErrUnconfigured) {
		t.Fatal(e)
	}
	if e = c.ClearMemory(context.Background()); !errors.Is(e, ErrUnconfigured) {
		t.Fatal(e)
	}
	cancel, done := runCore(t, c)
	if c.Status().ChannelState != "unconfigured" {
		t.Fatal(c.Status().ChannelState)
	}
	cancel()
	waitSignal(t, done)
	writeFixture(t, cfg.DataDir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c, e = New(cfg, Dependencies{ChannelFactory: func(config.Authorization) (Channel, error) {
		return &fakeChannel{poll: func(context.Context, string) ([]byte, error) {
			t.Error("remote work without key")
			return nil, ErrUnavailable
		}}, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	cancel, done = runCore(t, c)
	if c.Status().ChannelState != "provider_unconfigured" || c.Status().HasDeepSeekKey {
		t.Fatal(c.Status().ChannelState)
	}
	cancel()
	waitSignal(t, done)
}
func TestConfigMutationAndUnsafeSidecars(t *testing.T) {
	cfg := fixtureConfig(t)
	mutated := cfg
	mutated.Model = "externally changed"
	if _, e := New(mutated, Dependencies{}); e == nil {
		t.Fatal("mutated runtime snapshot accepted")
	}
	if _, e := New(config.Config{}, Dependencies{}); e == nil {
		t.Fatal("absent validation snapshot accepted")
	}
	if e := os.Mkdir(filepath.Join(cfg.DataDir, "miskoai.db-wal"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := New(cfg, Dependencies{}); e == nil {
		t.Fatal("unsafe journal accepted")
	}
}
