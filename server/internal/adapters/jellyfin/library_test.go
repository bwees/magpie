package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"magpie/internal/library"
	"magpie/internal/platform/settings"
)

const recordedMoviePath = "/data/media/movies/Dune (2021) [tmdbid-438631]/Dune (2021).mkv"

// recordedLibraryServer answers /Library/Media/Updated as Jellyfin 12 does. Like Jellyfin 12, it accepts
// only the MediaBrowser Token header.
type recordedLibraryServer struct {
	t      *testing.T
	server *httptest.Server

	mu      sync.Mutex
	updates []mediaUpdate
}

func newRecordedLibraryServer(t *testing.T) *recordedLibraryServer {
	t.Helper()
	r := &recordedLibraryServer{t: t}
	r.server = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.server.Close)
	return r
}

func (r *recordedLibraryServer) serve(w http.ResponseWriter, req *http.Request) {
	legacy := req.Header.Get(embyAuthorization) != "" || req.Header.Get(embyTokenParam) != "" || req.URL.Query().Has(apiKeyParam)
	if legacy || req.Header.Get(authorizationHeader) != fmt.Sprintf(tokenAuthorization, testAPIKey) {
		r.t.Errorf("%s %s sent %v, want only the MediaBrowser Token header", req.Method, req.URL, req.Header)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if req.Method != http.MethodPost || req.URL.Path != pathMediaUpdated {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var body mediaUpdates
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	r.mu.Lock()
	r.updates = append(r.updates, body.Updates...)
	r.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (r *recordedLibraryServer) recordedUpdates() []mediaUpdate {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.updates)
}

func newLibrary(t *testing.T, url string) *Library {
	t.Helper()
	return NewLibrary(newProbe(t, settings.JellyfinConnection{URL: url, APIKey: testAPIKey}).store)
}

func TestRefreshPathReportsTheFolderToJellyfin(t *testing.T) {
	recorded := newRecordedLibraryServer(t)
	folder := filepath.Dir(recordedMoviePath)

	if err := newLibrary(t, recorded.server.URL).RefreshPath(context.Background(), folder); err != nil {
		t.Fatal(err)
	}
	want := []mediaUpdate{{Path: folder, UpdateType: updateModified}}
	if got := recorded.recordedUpdates(); !slices.Equal(got, want) {
		t.Errorf("updates = %+v, want %+v", got, want)
	}
}

func TestLibraryFailsAsUnavailableWithoutJellyfin(t *testing.T) {
	recorded := newRecordedLibraryServer(t)
	closedURL := recorded.server.URL
	recorded.server.Close()

	for name, url := range map[string]string{"unreachable": closedURL, "not configured": ""} {
		err := newLibrary(t, url).RefreshPath(context.Background(), recordedMoviePath)
		if !errors.Is(err, library.ErrJellyfinUnavailable) {
			t.Errorf("%s RefreshPath err = %v", name, err)
		}
	}
}

func TestLibraryReportsARejectedKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	err := newLibrary(t, server.URL).RefreshPath(context.Background(), recordedMoviePath)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v, want the 401", err)
	}
}
