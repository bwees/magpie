package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"magpie/internal/platform/config"
)

const (
	testChunkPath   = "/_app/immutable/nodes/0.current.js"
	staleChunkPath  = "/_app/immutable/nodes/0.stale.js"
	testVersionPath = "/_app/version.json"
)

func newWebServer(t *testing.T) http.Handler {
	t.Helper()
	root := t.TempDir()
	writeWebFile(t, root, spaIndex, "<!doctype html>")
	writeWebFile(t, root, testChunkPath, "export {}")
	writeWebFile(t, root, testVersionPath, `{"version":"1"}`)
	return Build(config.Env{WebDir: root}, ModeServe, []Controller{NewHealthController()}).Mux
}

func writeWebFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClientRouteServesIndexThatBrowsersRevalidate(t *testing.T) {
	for _, path := range []string{"/", "/movies", "/titles/42"} {
		rec := get(t, newWebServer(t), path)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != cacheRevalidate {
			t.Errorf("%s: cache-control = %q, want %q", path, got, cacheRevalidate)
		}
	}
}

func TestImmutableAssetIsCachedForever(t *testing.T) {
	rec := get(t, newWebServer(t), testChunkPath)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheImmutable {
		t.Errorf("cache-control = %q, want %q", got, cacheImmutable)
	}
}

func TestVersionFileIsRevalidated(t *testing.T) {
	rec := get(t, newWebServer(t), testVersionPath)

	if got := rec.Header().Get("Cache-Control"); got != cacheRevalidate {
		t.Errorf("cache-control = %q, want %q", got, cacheRevalidate)
	}
}

// A page from an older build asks for chunks that no longer exist; answering with index.html makes it reload forever.
func TestMissingBuildAssetIsNotFound(t *testing.T) {
	rec := get(t, newWebServer(t), staleChunkPath)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
