package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/librarybundle"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/provider"
	"github.com/hidxt/miskoai/internal/storage"
)

// Actual local Core/Store/Processor/Lock; every external dependency refuses.
type bundleCoreRefusal struct{ calls atomic.Int64 }

func (r *bundleCoreRefusal) Chat(context.Context, []provider.Message, int) (provider.Reply, error) {
	r.calls.Add(1)
	return provider.Reply{}, errors.New("synthetic delegate refused")
}
func (r *bundleCoreRefusal) Search(context.Context, string, int) ([]provider.SearchResult, error) {
	r.calls.Add(1)
	return nil, errors.New("synthetic delegate refused")
}
func (r *bundleCoreRefusal) RawUpdates(context.Context, string) ([]byte, error) {
	r.calls.Add(1)
	return nil, errors.New("synthetic delegate refused")
}
func (r *bundleCoreRefusal) SendText(context.Context, string, string, string, string) error {
	r.calls.Add(1)
	return errors.New("synthetic delegate refused")
}

func bundleCoreController(t *testing.T, configured bool) *core.Controller {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private")
	if e := privatefs.EnsureDir(dir); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MISKOAI_DATA_DIR", dir)
	t.Setenv("MISKOAI_ADMIN_PASSWORD", testPassword)
	if configured {
		f, e := privatefs.Create(filepath.Join(dir, "weixin-auth.json"))
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.WriteString(`{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
		ce := f.Close()
		if e != nil || ce != nil {
			t.Fatal(e, ce)
		}
	}
	cfg, e := config.Load()
	if e != nil {
		t.Fatal(e)
	}
	cdn, e := media.NewCDN()
	if e != nil {
		t.Fatal(e)
	}
	if e = cdn.Close(); e != nil {
		t.Fatal(e)
	}
	p, e := attachment.NewImageProcessor(dir, cdn)
	if e != nil {
		t.Fatal(e)
	}
	refuse := &bundleCoreRefusal{}
	c, e := core.New(cfg, core.Dependencies{Model: refuse, Vision: refuse, Images: p, Search: refuse, ChannelFactory: func(config.Authorization) (core.Channel, error) { return refuse, nil }})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
		if refuse.calls.Load() != 0 {
			t.Error("external delegate invoked", refuse.calls.Load())
		}
	})
	return c
}

func bundleCoreBackupHTTP(t *testing.T, c *core.Controller, path string) []byte {
	t.Helper()
	s, cookie, csrf := bundleHTTPServer(t, coreBackend{c})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, bundleHTTPRequest(path, "{}", "application/json", cookie, csrf))
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/octet-stream" || int64(w.Body.Len()) != parseBundleSize(t, w.Header().Get("Content-Length")) {
		t.Fatal("actual prepared download", w.Code, w.Body.String())
	}
	return append([]byte{}, w.Body.Bytes()...)
}
func parseBundleSize(t *testing.T, s string) int64 {
	t.Helper()
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n <= 0 {
		t.Fatal("invalid actual size", e)
	}
	return n
}

func TestLibraryBundleCoreHTTPRoundTrip(t *testing.T) {
	source := bundleCoreController(t, true)
	input := httpLibraryPNG(t)
	next, _, e := source.LibraryUpload(context.Background(), bytes.NewReader(input), int64(len(input)), storage.LibraryMetadata{Tags: []string{"<img synthetic>"}, Description: "new PNG"})
	if e != nil {
		t.Fatal(e)
	}
	raw := bundleCoreBackupHTTP(t, source, "/api/library/backup")
	h, e := librarybundle.Decode(context.Background(), bytes.NewReader(raw), func(_ librarybundle.Record, in io.Reader) error { _, e := io.Copy(io.Discard, in); return e })
	if e != nil || len(h.Assets) != 1 {
		t.Fatal("actual complete envelope", e)
	}
	dest := bundleCoreController(t, true)
	prior := httpLibraryGIF(t)
	old, _, e := dest.LibraryUpload(context.Background(), bytes.NewReader(prior), int64(len(prior)), storage.LibraryMetadata{})
	if e != nil || old.ID != next.ID || old.SHA256 == next.SHA256 {
		t.Fatal("reused ID fixture", e)
	}
	s, cookie, csrf := bundleHTTPServer(t, coreBackend{dest})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, bundleHTTPRequest("/api/library/restore", string(raw), "application/octet-stream", cookie, csrf))
	if w.Code != 200 || w.Body.String() != `{"prior_retained":true,"channel_paused":true}` {
		t.Fatal("actual restore/pause result", w.Code, w.Body.String())
	}
	rows, e := dest.LibraryAssets(context.Background(), 0, 32)
	if e != nil || len(rows) != 1 || rows[0].SHA256 != next.SHA256 || rows[0].Uses != 0 {
		t.Fatal("fresh actual Store rows", e, rows)
	}
	w = request(s, "GET", "/api/library/preview?id="+strconv.FormatInt(next.ID, 10), "", cookie, "", "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), input) {
		t.Fatal("actual reopened image preview", w.Code)
	}
	if e = dest.LibraryUpdate(context.Background(), next.ID, storage.LibraryMetadata{Enabled: true, Description: "fresh reopened Store"}); e != nil {
		t.Fatal(e)
	}
	if e = dest.LibraryDelete(context.Background(), next.ID); e != nil {
		t.Fatal(e)
	}
}

func TestLibraryBundleCoreHTTPFormatRefusal(t *testing.T) {
	c := bundleCoreController(t, true)
	image := httpLibraryPNG(t)
	a, _, e := c.LibraryUpload(context.Background(), bytes.NewReader(image), int64(len(image)), storage.LibraryMetadata{})
	if e != nil {
		t.Fatal(e)
	}
	bundle := bundleCoreBackupHTTP(t, c, "/api/library/backup")
	database := bundleCoreBackupHTTP(t, c, "/api/backup")
	if !bytes.HasPrefix(database, []byte("SQLite format 3\x00")) {
		t.Fatal("legacy snapshot bytes changed")
	}
	s, cookie, csrf := bundleHTTPServer(t, coreBackend{c})
	for _, tc := range []struct {
		path string
		raw  []byte
	}{{"/api/library/restore", database}, {"/api/restore", bundle}, {"/api/library/restore", append(append([]byte{}, bundle...), 1)}} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, bundleHTTPRequest(tc.path, string(tc.raw), "application/octet-stream", cookie, csrf))
		if w.Code != 400 {
			t.Fatal("format confusion accepted", tc.path, w.Code, w.Body.String())
		}
		rows, e := c.LibraryAssets(context.Background(), 0, 32)
		if e != nil || len(rows) != 1 || rows[0].SHA256 != a.SHA256 {
			t.Fatal("refusal changed current rows", e)
		}
	}
}

func TestLibraryBundleCoreHTTPUnconfiguredAdmin(t *testing.T) {
	c := bundleCoreController(t, false)
	raw := bundleCoreBackupHTTP(t, c, "/api/library/backup")
	s, cookie, csrf := bundleHTTPServer(t, coreBackend{c})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, bundleHTTPRequest("/api/library/restore", string(raw), "application/octet-stream", cookie, csrf))
	var result struct {
		PriorRetained bool `json:"prior_retained"`
		ChannelPaused bool `json:"channel_paused"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.PriorRetained || !result.ChannelPaused {
		t.Fatal("unconfigured global administration", w.Code, w.Body.String())
	}
	if _, e := c.LibraryAssets(context.Background(), 0, 32); !errors.Is(e, core.ErrUnconfigured) {
		t.Fatal("scope requirement weakened", e)
	}
}
