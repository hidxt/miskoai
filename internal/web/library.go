package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hidxt/miskoai/internal/attachment"
	"github.com/hidxt/miskoai/internal/core"
	"github.com/hidxt/miskoai/internal/library"
	"github.com/hidxt/miskoai/internal/storage"
)

// Library capability is optional so existing management backends stay usable.
// Borrowed content must be consumed synchronously, without retain or reentry.
type libraryManagementBackend interface {
	LibraryAssets(context.Context, int64, int) ([]storage.LibraryAsset, error)
	LibraryUpload(context.Context, io.Reader, int64, storage.LibraryMetadata) (storage.LibraryAsset, bool, error)
	LibraryUpdate(context.Context, int64, storage.LibraryMetadata) error
	LibraryDelete(context.Context, int64) error
	WithLibraryAsset(context.Context, int64, bool, func(storage.LibraryAsset, attachment.LocalImage, attachment.Artifact) error) error
}

type libraryDTO struct {
	ID             string   `json:"id"`
	SHA256         string   `json:"sha256"`
	Format         string   `json:"format"`
	Bytes          int64    `json:"bytes"`
	Width          int      `json:"width"`
	Height         int      `json:"height"`
	Frames         int      `json:"frames"`
	Tags           []string `json:"tags"`
	Emotion        string   `json:"emotion"`
	Description    string   `json:"description"`
	State          string   `json:"state"`
	Enabled        bool     `json:"enabled"`
	Uses           string   `json:"uses"`
	LastUsedMillis string   `json:"last_used_millis"`
	Availability   string   `json:"availability"`
}

func libraryText(v string, capBytes int, controls bool) bool {
	if len(v) > capBytes || !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
		return false
	}
	if controls {
		for _, c := range v {
			if unicode.IsControl(c) {
				return false
			}
		}
	}
	return true
}
func libraryMetadataValid(m storage.LibraryMetadata) bool {
	if len(m.Tags) > 16 || !libraryText(m.Emotion, 64, true) || !libraryText(m.Description, 1024, false) {
		return false
	}
	seen := make(map[string]bool, len(m.Tags))
	for _, v := range m.Tags {
		if v == "" || !libraryText(v, 32, true) || seen[v] {
			return false
		}
		seen[v] = true
	}
	encoded, e := json.Marshal(m.Tags)
	return e == nil && len(encoded) <= 2048
}
func libraryRowDTO(a storage.LibraryAsset) (libraryDTO, error) {
	if a.ID <= 0 || len(a.SHA256) != 64 || a.Bytes < 1 || a.Bytes > 4<<20 || a.Width < 1 || a.Width > 8192 || a.Height < 1 || a.Height > 8192 || int64(a.Width)*int64(a.Height) > 16000000 || a.Frames < 1 || a.Frames > 32 || a.Uses < 0 || a.LastUsedMillis < 0 || a.LastUsedMillis > 253402300799999 {
		return libraryDTO{}, errors.New("web_response")
	}
	for _, c := range a.SHA256 {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return libraryDTO{}, errors.New("web_response")
		}
	}
	if a.Format != "gif" && (a.Format != "png" && a.Format != "jpeg" || a.Frames != 1) || a.State != "ready" && a.State != "deleting" || a.State == "deleting" && a.Enabled || !libraryMetadataValid(storage.LibraryMetadata{Tags: a.Tags, Emotion: a.Emotion, Description: a.Description, Enabled: a.Enabled}) {
		return libraryDTO{}, errors.New("web_response")
	}
	tags := a.Tags
	if tags == nil {
		tags = []string{}
	}
	return libraryDTO{ID: strconv.FormatInt(a.ID, 10), SHA256: a.SHA256, Format: a.Format, Bytes: a.Bytes, Width: a.Width, Height: a.Height, Frames: a.Frames, Tags: tags, Emotion: a.Emotion, Description: a.Description, State: a.State, Enabled: a.Enabled, Uses: strconv.FormatInt(a.Uses, 10), LastUsedMillis: strconv.FormatInt(a.LastUsedMillis, 10), Availability: "unchecked"}, nil
}
func libraryManagementError(w http.ResponseWriter, e error) {
	status, code := 500, "web_operation"
	var limit *http.MaxBytesError
	switch {
	case errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		status, code = 408, "web_timeout"
	case errors.Is(e, library.ErrCleanup):
		status, code = 500, "web_operation"
	case errors.Is(e, errManagementInput), errors.Is(e, library.ErrInvalid), errors.Is(e, storage.ErrInvalid), errors.As(e, &limit):
		status, code = 400, "web_input"
	case errors.Is(e, storage.ErrNotFound):
		status, code = 404, "web_not_found"
	case errors.Is(e, library.ErrRecovery):
		status, code = 409, "web_recovery"
	case errors.Is(e, library.ErrContent):
		status, code = 400, "web_content"
	case errors.Is(e, library.ErrCapacity), errors.Is(e, storage.ErrCapacity), errors.Is(e, storage.ErrStale), errors.Is(e, core.ErrBusy):
		status, code = 409, "web_busy"
	case errors.Is(e, library.ErrClosed), errors.Is(e, core.ErrUnconfigured), errors.Is(e, core.ErrUnavailable):
		status, code = 503, "web_unavailable"
	}
	safeError(w, status, code)
}
func libraryMutation(w http.ResponseWriter, e error) {
	if e != nil {
		libraryManagementError(w, e)
		return
	}
	mutationResult(w, nil)
}

type libraryContextReader struct {
	ctx   context.Context
	input io.Reader
}

func (r libraryContextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	n, e := r.input.Read(p)
	if ce := r.ctx.Err(); ce != nil {
		return n, ce
	}
	return n, e
}

func libraryRoute(w http.ResponseWriter, r *http.Request, b managementBackend, q url.Values) {
	lb, ok := b.(libraryManagementBackend)
	if !ok {
		safeError(w, 503, "web_unavailable")
		return
	}
	if r.URL.Path == "/api/library/preview" {
		id, e := decimalValue(q.Get("id"), 1, math.MaxInt64)
		if e != nil {
			libraryManagementError(w, e)
			return
		}
		libraryPreview(w, r, lb, id)
		return
	}
	switch r.Method {
	case "GET":
		after := int64(0)
		limit := int64(32)
		var e error
		if v, ok := q["after"]; ok {
			after, e = decimalValue(v[0], 0, math.MaxInt64)
			if e != nil {
				libraryManagementError(w, e)
				return
			}
		}
		if v, ok := q["limit"]; ok {
			limit, e = decimalValue(v[0], 1, 32)
			if e != nil {
				libraryManagementError(w, e)
				return
			}
		}
		rows, e := lb.LibraryAssets(r.Context(), after, int(limit))
		if e != nil {
			libraryManagementError(w, e)
			return
		}
		if len(rows) > int(limit) {
			safeError(w, 500, "web_response")
			return
		}
		items := make([]libraryDTO, 0, len(rows))
		next := after
		for _, a := range rows {
			item, e := libraryRowDTO(a)
			if e != nil || a.ID <= next {
				safeError(w, 500, "web_response")
				return
			}
			items = append(items, item)
			next = a.ID
		}
		respondJSON(w, struct {
			Items     []libraryDTO `json:"items"`
			NextAfter string       `json:"next_after"`
		}{items, strconv.FormatInt(next, 10)})
	case "POST":
		values := r.Header.Values("Content-Type")
		if len(values) != 1 || values[0] != "application/octet-stream" || len(r.Header.Values("Content-Encoding")) != 0 || r.ContentLength < 1 || r.ContentLength > 4<<20 || r.Body == nil {
			libraryManagementError(w, errManagementInput)
			return
		}
		input := http.MaxBytesReader(w, r.Body, 4<<20)
		a, created, e := lb.LibraryUpload(r.Context(), libraryContextReader{r.Context(), input}, r.ContentLength, storage.LibraryMetadata{})
		if e != nil {
			libraryManagementError(w, e)
			return
		}
		item, e := libraryRowDTO(a)
		if e != nil {
			safeError(w, 500, "web_response")
			return
		}
		respondJSON(w, struct {
			Item    libraryDTO `json:"item"`
			Created bool       `json:"created"`
		}{item, created})
	case "PATCH":
		id, m, e := libraryPatch(r)
		if e != nil {
			libraryManagementError(w, e)
			return
		}
		libraryMutation(w, lb.LibraryUpdate(r.Context(), id, m))
	case "DELETE":
		id, e := decimalValue(q.Get("id"), 1, math.MaxInt64)
		if e != nil {
			libraryManagementError(w, e)
			return
		}
		libraryMutation(w, lb.LibraryDelete(r.Context(), id))
	}
}
func libraryPatch(r *http.Request) (int64, storage.LibraryMetadata, error) {
	m := storage.LibraryMetadata{}
	o, e := requestObject(r, 16<<10, "id", "tags", "emotion", "description", "enabled")
	if e != nil {
		return 0, m, e
	}
	idText, e := stringValue(o["id"])
	if e != nil {
		return 0, m, e
	}
	id, e := decimalValue(idText, 1, math.MaxInt64)
	if e != nil {
		return 0, m, e
	}
	if len(o["tags"]) == 0 || o["tags"][0] != '[' || json.Unmarshal(o["tags"], &m.Tags) != nil || m.Tags == nil {
		return 0, m, errManagementInput
	}
	m.Emotion, e = stringValue(o["emotion"])
	if e != nil {
		return 0, m, e
	}
	m.Description, e = stringValue(o["description"])
	if e != nil {
		return 0, m, e
	}
	if string(o["enabled"]) != "true" && string(o["enabled"]) != "false" {
		return 0, m, errManagementInput
	}
	m.Enabled = string(o["enabled"]) == "true"
	if !libraryMetadataValid(m) {
		return 0, m, errManagementInput
	}
	return id, m, nil
}
func libraryPreview(w http.ResponseWriter, r *http.Request, b libraryManagementBackend, id int64) {
	started, called := false, false
	e := b.WithLibraryAsset(r.Context(), id, false, func(a storage.LibraryAsset, image attachment.LocalImage, artifact attachment.Artifact) error {
		if called {
			return library.ErrContent
		}
		called = true
		_, e := libraryRowDTO(a)
		if e != nil || a.ID != id || a.State != "ready" || artifact == nil || image.Bytes != a.Bytes || artifact.Size() != a.Bytes || image.SHA256 != a.SHA256 || image.Info.Format != a.Format || image.Info.Width != a.Width || image.Info.Height != a.Height || image.Info.Frames != a.Frames {
			return library.ErrContent
		}
		if e = r.Context().Err(); e != nil {
			return e
		}
		contentType := "image/" + a.Format
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", `inline; filename="library-image"`)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.FormatInt(a.Bytes, 10))
		started = true
		w.WriteHeader(http.StatusOK)
		input := io.NewSectionReader(artifact, 0, a.Bytes)
		var scratch [4096]byte
		for left := a.Bytes; left > 0; {
			if e = r.Context().Err(); e != nil {
				return e
			}
			count := int64(len(scratch))
			if left < count {
				count = left
			}
			n, readErr := io.ReadFull(input, scratch[:int(count)])
			if readErr != nil {
				return readErr
			}
			if e = r.Context().Err(); e != nil {
				return e
			}
			written, writeErr := w.Write(scratch[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
			left -= int64(n)
		}
		return r.Context().Err()
	})
	if e == nil && !called {
		e = errors.New("web_operation")
	}
	if e == nil {
		e = r.Context().Err()
	}
	if e != nil {
		if started {
			panic(http.ErrAbortHandler)
		}
		libraryManagementError(w, e)
	}
}
