package httpserver

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/go-fuego/fuego"
)

//go:embed all:webdist
var embeddedWebAssets embed.FS

func resolveWebFS(configured fs.FS) (fs.FS, error) {
	if configured != nil {
		return configured, nil
	}
	webFS, err := fs.Sub(embeddedWebAssets, "webdist")
	if err != nil {
		return nil, fmt.Errorf("open embedded web assets: %w", err)
	}
	return webFS, nil
}

func registerWebRoutes(s *fuego.Server, webFS fs.FS) {
	s.Mux.Handle("/", webUIHandler(webFS))
}

func webUIHandler(webFS fs.FS) http.Handler {
	files := http.FileServer(http.FS(webFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if isReservedHTTPPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel != "" {
			info, err := fs.Stat(webFS, rel)
			if err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				http.Error(w, "read web asset", http.StatusInternalServerError)
				return
			}
		}

		http.ServeFileFS(w, r, webFS, "index.html")
	})
}

func isReservedHTTPPath(requestPath string) bool {
	return requestPath == "/api" ||
		strings.HasPrefix(requestPath, "/api/") ||
		requestPath == "/swagger" ||
		strings.HasPrefix(requestPath, "/swagger/")
}
