package httpserver

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"magpie/internal/platform/problem"

	"github.com/go-fuego/fuego"
)

const (
	spaIndex       = "index.html"
	unknownAPIPath = "/api/"
	buildPrefix    = "/_app/"
	immutablePath  = "/_app/immutable/"

	cacheRevalidate = "no-cache"
	cacheImmutable  = "public, max-age=31536000, immutable"
)

// Unknown API paths answer with problem JSON instead of falling through to the SPA.
func registerAPINotFound(server *fuego.Server) {
	fuego.Handle(server, unknownAPIPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		problem.Write(w, r, problem.New(problem.TypeNotFound, http.StatusNotFound, "no API route for "+r.URL.Path))
	}), fuego.OptionHide())
}

// Serves the built SPA, falling back to index.html so client-side routes load.
// Build files are never answered with index.html: a page left over from an older build would reload forever.
func registerStaticWeb(server *fuego.Server, root string) {
	if root == "" {
		return
	}

	index := filepath.Join(root, spaIndex)
	files := http.FileServer(http.Dir(root))

	handler := func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(root, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			w.Header().Set("Cache-Control", cacheControlFor(r.URL.Path))
			files.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, buildPrefix) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", cacheRevalidate)
		http.ServeFile(w, r, index)
	}

	fuego.Handle(server, "/", http.HandlerFunc(handler), fuego.OptionHide())
	slog.Info("serving web frontend", "path", root)
}

func cacheControlFor(urlPath string) string {
	if strings.HasPrefix(urlPath, immutablePath) {
		return cacheImmutable
	}
	return cacheRevalidate
}
