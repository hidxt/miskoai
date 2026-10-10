package web

import (
	"embed"
	"net/http"
	"strconv"
)

//go:embed assets/index.html assets/app.js assets/style.css
var managementAssets embed.FS

// Assets supplies the fixed embedded management resources.
func Assets() http.Handler {
	type resource struct {
		data []byte
		mime string
	}
	resources := make(map[string]resource, 3)
	for _, entry := range []struct{ route, file, mime string }{
		{"/", "assets/index.html", "text/html; charset=utf-8"},
		{"/app.js", "assets/app.js", "text/javascript; charset=utf-8"},
		{"/style.css", "assets/style.css", "text/css; charset=utf-8"},
	} {
		data, err := managementAssets.ReadFile(entry.file)
		if err != nil {
			panic("web_assets")
		}
		resources[entry.route] = resource{data, entry.mime}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry, ok := resources[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", entry.mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(entry.data)))
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(entry.data)
		}
	})
}
