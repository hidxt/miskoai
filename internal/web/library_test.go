package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/config"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/media"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/storage"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func httpLibraryPNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.NRGBA{R: 23, A: 255})
	var b bytes.Buffer
	if e := png.Encode(&b, im); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func httpLibraryUpload(t *testing.T, s *Server, cookie *http.Cookie, csrf string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/library", bytes.NewReader(data))
	r.Host = s.host
	r.Header.Set("Content-Type", "application/octet-stream")
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("X-MiskoAI-CSRF", csrf)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestLibraryHTTPRealLifecycle(t *testing.T) {
	c := httpLibraryCore(t)
	s := newTestServer(t, Application(c))
	cookie, csrf := login(t, s)
	data := httpLibraryPNG(t)
	w := httpLibraryUpload(t, s, cookie, csrf, data)
	if w.Code != 200 {
		t.Fatalf("real raw upload: %d %s", w.Code, w.Body.String())
	}
	var v struct {
		Item struct {
			ID           string `json:"id"`
			Enabled      bool   `json:"enabled"`
			Availability string `json:"availability"`
		}
		Created bool `json:"created"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil || v.Item.ID == "" || !v.Created || v.Item.Enabled || v.Item.Availability != "unchecked" {
		t.Fatal(e, w.Body.String())
	}
	preview := request(s, "GET", "/api/library/preview?id="+v.Item.ID, "", cookie, "", "")
	if preview.Code != 200 || !bytes.Equal(data, preview.Body.Bytes()) {
		t.Fatal("preview", preview.Code, preview.Body.String())
	}
}
func TestLibraryHTTPLegacyUnavailable(t *testing.T) {
	s := newTestServer(t, newApplication(&managementFixture{}))
	cookie, _ := login(t, s)
	w := request(s, "GET", "/api/library", "", cookie, "", "")
	if w.Code != 503 || w.Body.String() != `{"error":"web_unavailable"}` {
		t.Fatal(w.Code, w.Body.String())
	}
}

func httpLibraryCore(t *testing.T) *core.Controller {
	t.Helper()
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(key, "MISKOAI_") || key == "DEEPSEEK_API_KEY" || key == "OLLAMA_API_KEY" {
			old := os.Getenv(key)
			if e := os.Unsetenv(key); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = os.Setenv(key, old) })
		}
	}
	dir := filepath.Join(t.TempDir(), "private")
	if e := privatefs.EnsureDir(dir); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MISKOAI_DATA_DIR", dir)
	t.Setenv("MISKOAI_ADMIN_PASSWORD", testPassword)
	auth, e := privatefs.Create(filepath.Join(dir, "weixin-auth.json"))
	if e != nil {
		t.Fatal(e)
	}
	_, e = auth.WriteString(`{"bot_token":"synthetic-token","account":"fixture-account","allowed_user":"fixture-user","base_url":"https://ilinkai.weixin.qq.com"}`)
	ce := auth.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	cfg, e := config.Load()
	if e != nil {
		t.Fatal(e)
	}
	c, e := core.New(cfg, core.Dependencies{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	})
	return c
}
func httpLibraryGIF(t *testing.T) []byte {
	t.Helper()
	im := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	im.SetColorIndex(0, 0, 1)
	var b bytes.Buffer
	if e := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{im, im}, Delay: []int{1, 1}}); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

type httpLibrarySpy struct {
	managementFixture
	calls    int
	id       int64
	after    int64
	limit    int
	metadata storage.LibraryMetadata
	rows     []storage.LibraryAsset
	err      error
	preview  func(context.Context, int64, bool, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error
}

func (b *httpLibrarySpy) LibraryAssets(_ context.Context, after int64, limit int) ([]storage.LibraryAsset, error) {
	b.calls++
	b.after = after
	b.limit = limit
	return b.rows, b.err
}
func (b *httpLibrarySpy) LibraryUpload(_ context.Context, r io.Reader, size int64, m storage.LibraryMetadata) (storage.LibraryAsset, bool, error) {
	b.calls++
	b.metadata = m
	return storage.LibraryAsset{}, false, b.err
}
func (b *httpLibrarySpy) LibraryUpdate(_ context.Context, id int64, m storage.LibraryMetadata) error {
	b.calls++
	b.id = id
	b.metadata = m
	return b.err
}
func (b *httpLibrarySpy) LibraryDelete(_ context.Context, id int64) error {
	b.calls++
	b.id = id
	return b.err
}
func (b *httpLibrarySpy) WithLibraryAsset(ctx context.Context, id int64, enabled bool, consume func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error {
	b.calls++
	b.id = id
	if b.preview != nil {
		return b.preview(ctx, id, enabled, consume)
	}
	return b.err
}
func httpLibraryValidRow() storage.LibraryAsset {
	return storage.LibraryAsset{ID: 1, SHA256: strings.Repeat("a", 64), Format: "png", Bytes: 68, Width: 2, Height: 2, Frames: 1, State: "ready", Tags: []string{}, Scope: storage.Scope{Account: "private-account-canary", User: "private-user-canary"}}
}
func TestLibraryHTTPBoundaryRefusals(t *testing.T) {
	b := &httpLibrarySpy{}
	s := newTestServer(t, newApplication(b))
	cookie, csrf := login(t, s)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/library?scope=x", ""}, {"GET", "/api/library?after=01", ""}, {"GET", "/api/library?after=-1", ""}, {"GET", "/api/library?after=9223372036854775808", ""}, {"GET", "/api/library?limit=0", ""}, {"GET", "/api/library?limit=33", ""}, {"GET", "/api/library?limit=1&%6cimit=2", ""}, {"GET", "/api/library?after=1;limit=1", ""},
		{"GET", "/api/library/preview", ""}, {"GET", "/api/library/preview?id=0", ""}, {"GET", "/api/library/preview?id=1&id=1", ""}, {"GET", "/api/library/preview?id=1&scope=x", ""},
		{"DELETE", "/api/library?id=01", ""}, {"DELETE", "/api/library?id=1", "x"}, {"DELETE", "/api/library?id=1&%69d=2", ""},
		{"POST", "/api/library?id=1", "x"}, {"PATCH", "/api/library?id=1", `{"id":"1","tags":[],"emotion":"","description":"","enabled":false}`},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := request(s, tc.method, tc.path, tc.body, cookie, testOrigin, csrf)
			want := 400
			if strings.Contains(tc.path, ";") {
				want = 403
			}
			if w.Code != want || b.calls != 0 {
				t.Fatal(w.Code, b.calls, w.Body.String())
			}
		})
	}
	for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
		path := "/api/library"
		if method == "DELETE" {
			path += "?id=1"
		}
		w := request(s, method, path, "", nil, testOrigin, csrf)
		if w.Code != 401 || b.calls != 0 {
			t.Fatal("unauthenticated", method, w.Code, b.calls)
		}
	}
	for _, tc := range []struct{ origin, token string }{{"", csrf}, {testOrigin, ""}, {testOrigin, "bad"}, {"http://hostile.invalid", csrf}} {
		w := request(s, "PATCH", "/api/library", `{}`, cookie, tc.origin, tc.token)
		if w.Code != 403 || b.calls != 0 {
			t.Fatal("CSRF/origin", w.Code, b.calls)
		}
	}
	r := httptest.NewRequest("GET", "/api/library", nil)
	r.Host = "hostile.invalid"
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 || b.calls != 0 {
		t.Fatal("Host", w.Code, b.calls)
	}
}
func TestLibraryHTTPPatchRefusesBeforeEffects(t *testing.T) {
	b := &httpLibrarySpy{}
	s := newTestServer(t, newApplication(b))
	cookie, csrf := login(t, s)
	valid := `{"id":"1","tags":[],"emotion":"","description":"","enabled":false}`
	invalid := []string{`{}`, strings.Replace(valid, `"id":"1"`, `"id":1`, 1), strings.Replace(valid, `"id":"1"`, `"id":"01"`, 1), strings.Replace(valid, `"id":"1"`, `"id":"9223372036854775808"`, 1), strings.Replace(valid, `"tags":[]`, `"tags":null`, 1), strings.Replace(valid, `"tags":[]`, `"tags":[null]`, 1), strings.Replace(valid, `"tags":[]`, `"tags":["x","x"]`, 1), strings.Replace(valid, `"tags":[]`, `"tags":[""]`, 1), strings.Replace(valid, `"tags":[]`, `"tags":["\n"]`, 1), strings.Replace(valid, `"enabled":false`, `"enabled":null`, 1), strings.Replace(valid, `"enabled":false`, `"enabled":"false"`, 1), strings.Replace(valid, `"enabled":false`, `"enabled":0`, 1), strings.Replace(valid, `"emotion":""`, `"emotion":"\u0001"`, 1), strings.Replace(valid, `"description":""`, `"description":"\u0000"`, 1), strings.Replace(valid, `"description":""`, `"description":"\ud800"`, 1), strings.Replace(valid, `"id":"1"`, `"id":"1","\u0069d":"2"`, 1), strings.TrimSuffix(valid, "}") + `,"scope":"hostile"}`, valid + `{}`, strings.Replace(valid, `"description":""`, `"description":"`+strings.Repeat("a", 1025)+`"`, 1), strings.Replace(valid, `"emotion":""`, `"emotion":"`+strings.Repeat("界", 22)+`"`, 1), strings.Replace(valid, `"tags":[]`, `"tags":["`+strings.Repeat("界", 11)+`"]`, 1), strings.Repeat(" ", 16<<10) + valid}
	tags := make([]string, 17)
	for i := range tags {
		tags[i] = strconv.Itoa(i)
	}
	encoded, _ := json.Marshal(tags)
	invalid = append(invalid, strings.Replace(valid, `"tags":[]`, `"tags":`+string(encoded), 1))
	for i, body := range invalid {
		w := request(s, "PATCH", "/api/library", body, cookie, testOrigin, csrf)
		if w.Code != 400 || b.calls != 0 {
			t.Fatalf("case %d: %d calls %d %s", i, w.Code, b.calls, w.Body.String())
		}
	}
	// Fully escaped maximum decoded fields stay under the 16 KiB JSON body cap.
	tags = make([]string, 16)
	for i := range tags {
		tags[i] = strings.Repeat("x", 30) + fmt.Sprintf("%02d", i)
	}
	var body strings.Builder
	body.WriteString(`{"id":"9223372036854775807","tags":[`)
	for i, v := range tags {
		if i > 0 {
			body.WriteByte(',')
		}
		body.WriteByte('"')
		for _, r := range v {
			fmt.Fprintf(&body, `\u%04x`, r)
		}
		body.WriteByte('"')
	}
	body.WriteString(`],"emotion":"` + strings.Repeat(`\u0078`, 64) + `","description":"` + strings.Repeat(`\u0078`, 1024) + `","enabled":true}`)
	w := request(s, "PATCH", "/api/library", body.String(), cookie, testOrigin, csrf)
	if w.Code != 200 || b.calls != 1 || b.id != math.MaxInt64 || !b.metadata.Enabled || len(b.metadata.Description) != 1024 {
		t.Fatal("escaped maximum", w.Code, b.calls, b.id, w.Body.String())
	}
}
func TestLibraryHTTPPagingAndSafeResponse(t *testing.T) {
	row := httpLibraryValidRow()
	row.ID = math.MaxInt64
	row.Uses = math.MaxInt64
	row.LastUsedMillis = 253402300799999
	row.Description = `<script>alert("x")</script>`
	b := &httpLibrarySpy{rows: []storage.LibraryAsset{row}}
	s := newTestServer(t, newApplication(b))
	cookie, _ := login(t, s)
	w := request(s, "GET", "/api/library?after=9223372036854775806&limit=1", "", cookie, "", "")
	if w.Code != 200 || b.after != math.MaxInt64-1 || b.limit != 1 || strings.Contains(w.Body.String(), "private-") || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal(w.Code, b.after, b.limit, w.Body.String())
	}
	var page struct {
		Items     []libraryDTO `json:"items"`
		NextAfter string       `json:"next_after"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || len(page.Items) != 1 || page.Items[0].Uses != "9223372036854775807" || page.Items[0].LastUsedMillis != "253402300799999" || page.NextAfter != "9223372036854775807" || page.Items[0].Description != row.Description || page.Items[0].Availability != "unchecked" {
		t.Fatal(e, w.Body.String())
	}
	b.rows = nil
	w = request(s, "GET", "/api/library?after=4", "", cookie, "", "")
	if w.Code != 200 || w.Body.String() != `{"items":[],"next_after":"4"}` {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, rows := range [][]storage.LibraryAsset{{httpLibraryValidRow(), httpLibraryValidRow()}, {row, row}, {{ID: 1}}, func() []storage.LibraryAsset { r := httpLibraryValidRow(); r.ID = 0; return []storage.LibraryAsset{r} }()} {
		b.rows = rows
		w = request(s, "GET", "/api/library?limit=1", "", cookie, "", "")
		if w.Code != 500 {
			t.Fatal("invalid response", w.Code, w.Body.String())
		}
	}
	b.rows = []storage.LibraryAsset{httpLibraryValidRow()}
	w = request(s, "GET", "/api/library?after=1", "", cookie, "", "")
	if w.Code != 500 {
		t.Fatal("nonincreasing", w.Code)
	}
}

type httpLibraryCountReader struct{ reads int }

func (r *httpLibraryCountReader) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }
func TestLibraryHTTPUploadAdmissionBeforeRead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		length   int64
		types    []string
		encoding []string
	}{{"unknown", -1, []string{"application/octet-stream"}, nil}, {"zero", 0, []string{"application/octet-stream"}, nil}, {"oversized", (4 << 20) + 1, []string{"application/octet-stream"}, nil}, {"missing-type", 1, nil, nil}, {"duplicate-type", 1, []string{"application/octet-stream", "application/octet-stream"}, nil}, {"parameters", 1, []string{"application/octet-stream; charset=utf-8"}, nil}, {"wrong-type", 1, []string{"image/png"}, nil}, {"gzip", 1, []string{"application/octet-stream"}, []string{"gzip"}}, {"empty-encoding", 1, []string{"application/octet-stream"}, []string{""}}} {
		t.Run(tc.name, func(t *testing.T) {
			b := &httpLibrarySpy{}
			s := newTestServer(t, newApplication(b))
			cookie, csrf := login(t, s)
			read := &httpLibraryCountReader{}
			r := httptest.NewRequest("POST", "/api/library", nil)
			r.Host = s.host
			r.ContentLength = tc.length
			r.Body = io.NopCloser(read)
			r.Header["Content-Type"] = tc.types
			if tc.encoding != nil {
				r.Header["Content-Encoding"] = tc.encoding
			}
			r.AddCookie(cookie)
			r.Header.Set("Origin", testOrigin)
			r.Header.Set("X-MiskoAI-CSRF", csrf)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 400 || b.calls != 0 || read.reads != 0 {
				t.Fatal(w.Code, b.calls, read.reads)
			}
		})
	}
}

func TestLibraryHTTPRealCRUDAndMalformedBytes(t *testing.T) {
	c := httpLibraryCore(t)
	s := newTestServer(t, Application(c))
	cookie, csrf := login(t, s)
	for _, data := range [][]byte{httpLibraryPNG(t), httpLibraryGIF(t)} {
		w := httpLibraryUpload(t, s, cookie, csrf, data)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var uploaded struct {
			Item    libraryDTO `json:"item"`
			Created bool       `json:"created"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &uploaded); e != nil || !uploaded.Created {
			t.Fatal(e, w.Body.String())
		}
		id := uploaded.Item.ID
		meta := struct {
			ID          string   `json:"id"`
			Tags        []string `json:"tags"`
			Emotion     string   `json:"emotion"`
			Description string   `json:"description"`
			Enabled     bool     `json:"enabled"`
		}{id, []string{"synthetic", "表情"}, "happy", `<img src=x onerror=alert(1)>`, true}
		body, _ := json.Marshal(meta)
		w = request(s, "PATCH", "/api/library", string(body), cookie, testOrigin, csrf)
		if w.Code != 200 || w.Body.String() != `{"ok":true}` {
			t.Fatal(w.Code, w.Body.String())
		}
		w = httpLibraryUpload(t, s, cookie, csrf, data)
		var duplicate struct {
			Item    libraryDTO `json:"item"`
			Created bool       `json:"created"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &duplicate); e != nil || w.Code != 200 || duplicate.Created || duplicate.Item.ID != id || !duplicate.Item.Enabled || duplicate.Item.Description != meta.Description || !reflect.DeepEqual(duplicate.Item.Tags, meta.Tags) {
			t.Fatal("duplicate changed metadata", e, w.Code, w.Body.String())
		}
		preview := request(s, "GET", "/api/library/preview?id="+id, "", cookie, "", "")
		if preview.Code != 200 || !bytes.Equal(preview.Body.Bytes(), data) || preview.Header().Get("Content-Type") != "image/"+uploaded.Item.Format || preview.Header().Get("Content-Length") != strconv.Itoa(len(data)) || preview.Header().Get("Cache-Control") != "no-store" || preview.Header().Get("X-Content-Type-Options") != "nosniff" || preview.Header().Get("Content-Disposition") != `inline; filename="library-image"` {
			t.Fatal("actual preview", preview.Code, preview.Header())
		}
		meta.Enabled = false
		body, _ = json.Marshal(meta)
		w = request(s, "PATCH", "/api/library", string(body), cookie, testOrigin, csrf)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		preview = request(s, "GET", "/api/library/preview?id="+id, "", cookie, "", "")
		if preview.Code != 200 || !bytes.Equal(preview.Body.Bytes(), data) {
			t.Fatal("disabled preview", preview.Code)
		}
		w = request(s, "DELETE", "/api/library?id="+id, "", cookie, testOrigin, csrf)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		w = request(s, "GET", "/api/library/preview?id="+id, "", cookie, "", "")
		if w.Code != 404 {
			t.Fatal("deleted", w.Code, w.Body.String())
		}
	}
	for _, tc := range []struct {
		name     string
		data     []byte
		declared int64
	}{{"truncated", httpLibraryPNG(t), int64(len(httpLibraryPNG(t))) + 1}, {"trailing", append(httpLibraryPNG(t), 'x'), int64(len(httpLibraryPNG(t)))}, {"unsupported", []byte("synthetic-not-image"), 19}} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/library", bytes.NewReader(tc.data))
			r.ContentLength = tc.declared
			r.Host = s.host
			r.Header.Set("Content-Type", "application/octet-stream")
			r.Header.Set("Origin", testOrigin)
			r.Header.Set("X-MiskoAI-CSRF", csrf)
			r.AddCookie(cookie)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	w := request(s, "GET", "/api/library", "", cookie, "", "")
	if w.Code != 200 || w.Body.String() != `{"items":[],"next_after":"0"}` {
		t.Fatal("refused bytes created rows", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/library/preview?id=9223372036854775807", "/api/library?after=9223372036854775807"} {
		w = request(s, "GET", path, "", cookie, "", "")
		if strings.Contains(path, "preview") && w.Code != 404 || !strings.Contains(path, "preview") && w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
}
func TestLibraryHTTPErrorsAreFixed(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code int
		body string
	}{{library.ErrInvalid, 400, "web_input"}, {library.ErrCapacity, 409, "web_busy"}, {library.ErrRecovery, 409, "web_recovery"}, {library.ErrContent, 400, "web_content"}, {library.ErrCleanup, 500, "web_operation"}, {library.ErrClosed, 503, "web_unavailable"}, {storage.ErrNotFound, 404, "web_not_found"}, {storage.ErrInvalid, 400, "web_input"}, {storage.ErrCapacity, 409, "web_busy"}, {storage.ErrStale, 409, "web_busy"}, {core.ErrUnconfigured, 503, "web_unavailable"}, {core.ErrUnavailable, 503, "web_unavailable"}, {context.Canceled, 408, "web_timeout"}, {context.DeadlineExceeded, 408, "web_timeout"}, {errors.New("private-path-secret-canary"), 500, "web_operation"}} {
		w := httptest.NewRecorder()
		libraryManagementError(w, fmt.Errorf("private-path-secret-canary: %w", tc.err))
		if w.Code != tc.code || w.Body.String() != `{"error":"`+tc.body+`"}` {
			t.Fatal(tc.err, w.Code, w.Body.String())
		}
	}
	b := &httpLibrarySpy{err: library.ErrRecovery}
	s := newTestServer(t, newApplication(b))
	cookie, csrf := login(t, s)
	w := request(s, "DELETE", "/api/library?id=9223372036854775807", "", cookie, testOrigin, csrf)
	if w.Code != 409 || b.calls != 1 || b.id != math.MaxInt64 {
		t.Fatal(w.Code, b.calls, b.id)
	}
	b.err = nil
	w = request(s, "GET", "/api/library/preview?id=1", "", cookie, "", "")
	if w.Code != 500 {
		t.Fatal("success without callback", w.Code, w.Body.String())
	}
}

type httpLibraryArtifact struct {
	*bytes.Reader
	readErr error
	closed  bool
}

func (a *httpLibraryArtifact) Size() int64  { return a.Reader.Size() }
func (a *httpLibraryArtifact) Close() error { a.closed = true; return nil }
func (a *httpLibraryArtifact) ReadAt(p []byte, off int64) (int, error) {
	if a.readErr != nil {
		return 0, a.readErr
	}
	return a.Reader.ReadAt(p, off)
}
func httpLibraryPreviewMetadata(data []byte) (storage.LibraryAsset, attachment.LocalImage, *httpLibraryArtifact) {
	sum := sha256.Sum256(data)
	r := httpLibraryValidRow()
	r.Bytes = int64(len(data))
	r.SHA256 = hex.EncodeToString(sum[:])
	im := attachment.LocalImage{Info: media.ImageInfo{Format: r.Format, Width: r.Width, Height: r.Height, Frames: r.Frames}, SHA256: r.SHA256, Bytes: r.Bytes}
	return r, im, &httpLibraryArtifact{Reader: bytes.NewReader(data)}
}
func httpLibraryAbort(t *testing.T, handler func()) {
	t.Helper()
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Fatalf("abort = %v", v)
		}
	}()
	handler()
}

type httpLibraryFailureWriter struct {
	*httptest.ResponseRecorder
	fail   error
	short  bool
	cancel context.CancelFunc
}

func (w *httpLibraryFailureWriter) Write(p []byte) (int, error) {
	if w.cancel != nil {
		w.cancel()
	}
	if w.fail != nil {
		return 0, w.fail
	}
	if w.short {
		return len(p) - 1, nil
	}
	return w.ResponseRecorder.Write(p)
}
func TestLibraryHTTPPreviewStreamingFailures(t *testing.T) {
	for _, name := range []string{"read", "write", "short", "cleanup", "cancel"} {
		t.Run(name, func(t *testing.T) {
			data := httpLibraryPNG(t)
			row, im, a := httpLibraryPreviewMetadata(data)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := &httpLibrarySpy{preview: func(_ context.Context, id int64, enabled bool, consume func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error {
				if id != 1 || enabled {
					t.Fatal("preview contract")
				}
				e := consume(row, im, a)
				if name == "cleanup" {
					return library.ErrCleanup
				}
				return e
			}}
			s := newTestServer(t, newApplication(b))
			cookie, _ := login(t, s)
			r := httptest.NewRequest("GET", "/api/library/preview?id=1", nil).WithContext(ctx)
			r.Host = s.host
			r.AddCookie(cookie)
			w := &httpLibraryFailureWriter{ResponseRecorder: httptest.NewRecorder()}
			switch name {
			case "read":
				a.readErr = errors.New("private-read-secret")
			case "write":
				w.fail = errors.New("private-write-secret")
			case "short":
				w.short = true
			case "cancel":
				w.cancel = cancel
			}
			httpLibraryAbort(t, func() { s.Handler().ServeHTTP(w, r) })
			if w.Header().Get("Content-Type") != "image/png" || strings.Contains(w.Body.String(), "error") || strings.Contains(w.Body.String(), "secret") || a.closed {
				t.Fatal("unsafe append or borrowed Close", w.Body.String(), a.closed)
			}
			if name == "cleanup" && !bytes.Equal(w.Body.Bytes(), data) {
				t.Fatal("cleanup fixture incomplete")
			}
			if w := request(s, "GET", "/api/status", "", cookie, "", ""); w.Code != 200 {
				t.Fatal("admission not released", w.Code)
			}
		})
	}
}
func TestLibraryHTTPPreviewRefusesMetadataBeforeHeaders(t *testing.T) {
	for _, name := range []string{"id", "digest", "size", "format", "width", "frames", "deleting", "nil-artifact", "canceled"} {
		t.Run(name, func(t *testing.T) {
			row, im, a := httpLibraryPreviewMetadata(httpLibraryPNG(t))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var artifact attachment.Artifact = a
			switch name {
			case "id":
				row.ID = 2
			case "digest":
				im.SHA256 = strings.Repeat("b", 64)
			case "size":
				im.Bytes++
			case "format":
				im.Info.Format = "svg"
			case "width":
				im.Info.Width++
			case "frames":
				im.Info.Frames++
			case "deleting":
				row.State = "deleting"
			case "nil-artifact":
				artifact = nil
			case "canceled":
				cancel()
			}
			b := &httpLibrarySpy{preview: func(_ context.Context, _ int64, _ bool, consume func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error {
				return consume(row, im, artifact)
			}}
			r := httptest.NewRequest("GET", "/api/library/preview?id=1", nil).WithContext(ctx)
			w := httptest.NewRecorder()
			libraryPreview(w, r, b, 1)
			want := 400
			if name == "canceled" {
				want = 408
			}
			if w.Code != want || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Content-Disposition") != "" || a.closed {
				t.Fatal(w.Code, w.Header(), a.closed)
			}
		})
	}
}

type httpLibraryHeldWriter struct {
	*httptest.ResponseRecorder
	entered, release chan struct{}
	once             sync.Once
}

func (w *httpLibraryHeldWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered); <-w.release })
	return w.ResponseRecorder.Write(p)
}
func TestLibraryHTTPRealPreviewCloseAndCancelJoin(t *testing.T) {
	for _, mode := range []string{"close", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			c := httpLibraryCore(t)
			data := httpLibraryPNG(t)
			a, created, e := c.LibraryUpload(context.Background(), bytes.NewReader(data), int64(len(data)), storage.LibraryMetadata{})
			if e != nil || !created {
				t.Fatal(e)
			}
			s := newTestServer(t, Application(c))
			cookie, _ := login(t, s)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest("GET", "/api/library/preview?id="+strconv.FormatInt(a.ID, 10), nil).WithContext(ctx)
			r.Host = s.host
			r.AddCookie(cookie)
			w := &httpLibraryHeldWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{})}
			done := make(chan any, 1)
			closed := make(chan error, 1)
			var releaseOnce sync.Once
			var closeOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(w.release) }) }
			startClose := func() { closeOnce.Do(func() { go func() { closed <- c.Close() }() }) }
			// Register actual unblock and both joins before any held-lifetime assertion.
			t.Cleanup(func() {
				unblock()
				v := <-done
				if v != http.ErrAbortHandler {
					t.Error("committed canceled preview did not abort", v)
				}
				startClose()
				if e := <-closed; e != nil {
					t.Error(e)
				}
			})
			go func() {
				var result any
				defer func() {
					if v := recover(); v != nil {
						result = v
					}
					done <- result
				}()
				s.Handler().ServeHTTP(w, r)
			}()
			select {
			case <-w.entered:
			case <-time.After(5 * time.Second):
				startClose()
				t.Fatal("real response writer not entered")
			}
			if mode == "close" {
				startClose()
				select {
				case e := <-closed:
					closed <- e
					t.Fatal("Core.Close returned before actual writer joined", e)
				case <-time.After(50 * time.Millisecond):
				}
			} else {
				cancel()
				blockedCtx, blockedCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				_, _, blockedErr := c.LibraryUpload(blockedCtx, bytes.NewReader(data), int64(len(data)), storage.LibraryMetadata{})
				blockedCancel()
				if !errors.Is(blockedErr, context.DeadlineExceeded) {
					t.Fatal("canceled preview released admission before writer joined", blockedErr)
				}
				select {
				case v := <-done:
					done <- v
					t.Fatal("preview returned before actual writer joined", v)
				case <-time.After(50 * time.Millisecond):
				}
			}
			unblock()
		})
	}
}
func TestLibraryHTTPRealRestoreMissingAndForeignMetadata(t *testing.T) {
	c := httpLibraryCore(t)
	data := httpLibraryPNG(t)
	original, _, e := c.LibraryUpload(context.Background(), bytes.NewReader(data), int64(len(data)), storage.LibraryMetadata{})
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(t.TempDir(), "private-source")
	if e := privatefs.EnsureDir(dir); e != nil {
		t.Fatal(e)
	}
	source, e := storage.Open(filepath.Join(dir, "source.db"))
	if e != nil {
		t.Fatal(e)
	}
	sourceClosed := false
	t.Cleanup(func() {
		if !sourceClosed {
			_ = source.Close()
		}
	})
	missing := httpLibraryValidRow()
	missing.ID = 0
	missing.Scope = storage.Scope{Account: "fixture-account", User: "fixture-user"}
	missing.SHA256 = strings.Repeat("b", 64)
	missing.Description = "missing-supported-image"
	row, _, e := source.AddLibraryAsset(context.Background(), missing.Scope, missing)
	if e != nil {
		t.Fatal(e)
	}
	foreign := missing
	foreign.Scope = storage.Scope{Account: "foreign-account-canary", User: "foreign-user-canary"}
	foreign.Description = "foreign-description-canary"
	foreign.SHA256 = strings.Repeat("c", 64)
	foreignRow, _, e := source.AddLibraryAsset(context.Background(), foreign.Scope, foreign)
	if e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(dir, "backup.db")
	if e := source.Backup(context.Background(), backup); e != nil {
		t.Fatal(e)
	}
	if e := source.Close(); e != nil {
		t.Fatal(e)
	}
	sourceClosed = true
	input, e := os.Open(backup)
	if e != nil {
		t.Fatal(e)
	}
	result, restoreErr := c.Restore(context.Background(), input)
	closeErr := input.Close()
	if restoreErr != nil || closeErr != nil || !result.ChannelPaused {
		t.Fatal(restoreErr, closeErr, result)
	}
	s := newTestServer(t, Application(c))
	cookie, csrf := login(t, s)
	w := request(s, "GET", "/api/library", "", cookie, "", "")
	var page struct {
		Items []libraryDTO `json:"items"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || w.Code != 200 || len(page.Items) != 1 || page.Items[0].SHA256 != missing.SHA256 || page.Items[0].Availability != "unchecked" || strings.Contains(w.Body.String(), "foreign-") {
		t.Fatal("restored metadata", e, w.Code, w.Body.String())
	}
	w = request(s, "GET", "/api/library/preview?id="+strconv.FormatInt(row.ID, 10), "", cookie, "", "")
	if w.Code != 409 || w.Body.String() != `{"error":"web_recovery"}` {
		t.Fatal("missing content", w.Code, w.Body.String())
	}
	for _, method := range []string{"GET", "DELETE"} {
		path := "/api/library?id=" + strconv.FormatInt(foreignRow.ID, 10)
		if method == "GET" {
			path = "/api/library/preview?id=" + strconv.FormatInt(foreignRow.ID, 10)
		}
		w = request(s, method, path, "", cookie, testOrigin, csrf)
		if w.Code != 404 {
			t.Fatal("foreign ID", method, w.Code, w.Body.String())
		}
	}
	// Database-only restore keeps the previous owned content, without reusing it
	// for an equal ID carrying a different digest.
	entries, e := os.ReadDir(filepath.Join(os.Getenv("MISKOAI_DATA_DIR"), "library"))
	if e != nil || len(entries) != 1 || original.SHA256 == row.SHA256 {
		t.Fatal("retained content", e, len(entries))
	}
}
func TestLibraryHTTPCleanupClassDominatesInvalid(t *testing.T) {
	w := httptest.NewRecorder()
	libraryManagementError(w, errors.Join(library.ErrInvalid, library.ErrCleanup))
	if w.Code != 500 || w.Body.String() != `{"error":"web_operation"}` {
		t.Fatal(w.Code, w.Body.String())
	}
}
