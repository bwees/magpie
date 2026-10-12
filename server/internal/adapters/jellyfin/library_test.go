package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/library"
	"magpie/internal/platform/settings"

	"github.com/coder/websocket"
)

const (
	libraryFixtureDir     = "testdata/library"
	fixtureItemsByID      = "items_by_id.json"
	fixtureItemsSincePage = "items_since_page_%d.json"
	fixtureForceKeepAlive = "socket_force_keep_alive.json"
	fixtureKeepAlive      = "socket_keep_alive.json"
	fixtureLibraryChanged = "socket_library_changed.json"

	recordedMovieID    = "5d1e3c2b7a9f4e6d8c0b1a2f3e4d5c6b"
	recordedEpisodeID  = "7c2b1a0f9e8d4c7b6a5f4e3d2c1b0a9f"
	recordedMoviePath  = "/data/media/movies/Dune (2021) [tmdbid-438631]/Dune (2021).mkv"
	recordedEpisodeE01 = "/data/media/tv/Severance (2022) [tmdbid-95396]/Season 01/Severance S01E01.mkv"
	recordedEpisodeE02 = "/data/media/tv/Severance (2022) [tmdbid-95396]/Season 01/Severance S01E02.mkv"
	recordedPageSize   = 2
	waitForSocket      = 5 * time.Second
)

// recordedLibraryServer replays Jellyfin 12's answers to the library endpoints and its WebSocket. Like
// Jellyfin 12, it accepts only the MediaBrowser Token header.
type recordedLibraryServer struct {
	t      *testing.T
	server *httptest.Server

	mu        sync.Mutex
	updates   []mediaUpdate
	queries   []string
	keepAlive chan struct{}
	// closeSocket ends the socket after the LibraryChanged message instead of holding it open.
	closeSocket bool
}

func newRecordedLibraryServer(t *testing.T) *recordedLibraryServer {
	t.Helper()
	r := &recordedLibraryServer{t: t, keepAlive: make(chan struct{}, 1)}
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
	query := req.URL.Query()
	switch {
	case req.Method == http.MethodPost && req.URL.Path == pathMediaUpdated:
		var body mediaUpdates
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.mu.Lock()
		r.updates = append(r.updates, body.Updates...)
		r.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case req.Method == http.MethodGet && req.URL.Path == pathItems:
		r.mu.Lock()
		r.queries = append(r.queries, req.URL.RawQuery)
		r.mu.Unlock()
		if query.Has(queryIDs) {
			r.reply(w, fixtureItemsByID)
			return
		}
		start, _ := strconv.Atoi(query.Get(queryStartIndex))
		r.reply(w, fmt.Sprintf(fixtureItemsSincePage, start/recordedPageSize+1))
	case req.URL.Path == pathSocket:
		r.socket(w, req)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (r *recordedLibraryServer) recordedUpdates() []mediaUpdate {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.updates)
}

func (r *recordedLibraryServer) recordedQueries() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.queries)
}

func (r *recordedLibraryServer) reply(w http.ResponseWriter, fixture string) {
	body, err := os.ReadFile(filepath.Join(libraryFixtureDir, fixture))
	if err != nil {
		r.t.Error(err)
		return
	}
	w.Header().Set(contentTypeHeader, jsonContent)
	_, _ = w.Write(body)
}

// socket sends ForceKeepAlive, waits for the client's KeepAlive, then sends one LibraryChanged.
func (r *recordedLibraryServer) socket(w http.ResponseWriter, req *http.Request) {
	conn, err := websocket.Accept(w, req, nil)
	if err != nil {
		r.t.Error(err)
		return
	}
	defer func() { _ = conn.CloseNow() }()
	ctx := req.Context()
	r.send(ctx, conn, fixtureForceKeepAlive)
	_, data, err := conn.Read(ctx)
	if err != nil {
		return
	}
	var msg socketMessage
	if err := json.Unmarshal(data, &msg); err != nil || msg.MessageType != messageKeepAlive {
		r.t.Errorf("client answered %s, want KeepAlive", data)
		return
	}
	r.keepAlive <- struct{}{}
	r.send(ctx, conn, fixtureKeepAlive)
	r.send(ctx, conn, fixtureLibraryChanged)
	if r.closeSocket {
		_ = conn.Close(websocket.StatusGoingAway, "server restarting")
		return
	}
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func (r *recordedLibraryServer) send(ctx context.Context, conn *websocket.Conn, fixture string) {
	body, err := os.ReadFile(filepath.Join(libraryFixtureDir, fixture))
	if err != nil {
		r.t.Error(err)
		return
	}
	if err := conn.Write(ctx, websocket.MessageText, body); err != nil {
		r.t.Error(err)
	}
}

func newLibrary(t *testing.T, url string) *Library {
	t.Helper()
	l := NewLibrary(newProbe(t, settings.JellyfinConnection{URL: url, APIKey: testAPIKey}).store)
	l.pageSize = recordedPageSize
	return l
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

func TestItemsResolvesIDsToMoviesAndEpisodesWithPaths(t *testing.T) {
	recorded := newRecordedLibraryServer(t)

	items, err := newLibrary(t, recorded.server.URL).Items(context.Background(), []string{recordedMovieID, recordedEpisodeID})
	if err != nil {
		t.Fatal(err)
	}
	want := []library.JellyfinItem{
		{ID: recordedMovieID, Path: recordedMoviePath, Type: itemTypeMovie},
		{ID: recordedEpisodeID, Path: recordedEpisodeE01, Type: itemTypeEpisode},
	}
	if !slices.Equal(items, want) {
		t.Errorf("items = %+v, want %+v", items, want)
	}
	query := recorded.recordedQueries()[0]
	for _, part := range []string{"ids=" + recordedMovieID + "%2C" + recordedEpisodeID, "fields=Path", "includeItemTypes=Movie%2CEpisode"} {
		if !strings.Contains(query, part) {
			t.Errorf("query %q lacks %q", query, part)
		}
	}
}

func TestItemsWithoutIDsAsksJellyfinNothing(t *testing.T) {
	recorded := newRecordedLibraryServer(t)
	items, err := newLibrary(t, recorded.server.URL).Items(context.Background(), nil)
	if queries := recorded.recordedQueries(); err != nil || len(items) != 0 || len(queries) != 0 {
		t.Errorf("Items(nil) = %+v, %v after queries %v", items, err, queries)
	}
}

func TestItemsSincePagesThroughChangedItems(t *testing.T) {
	recorded := newRecordedLibraryServer(t)
	since := time.Date(2026, 10, 11, 3, 0, 0, 0, time.UTC)

	items, err := newLibrary(t, recorded.server.URL).ItemsSince(context.Background(), since)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	if want := []string{recordedEpisodeE01, recordedEpisodeE02, recordedMoviePath}; !slices.Equal(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
	queries := recorded.recordedQueries()
	if len(queries) != 2 {
		t.Fatalf("queries = %v, want two pages", queries)
	}
	for _, part := range []string{"minDateLastSaved=2026-10-11T03%3A00%3A00Z", "recursive=true", "limit=2", "startIndex=0"} {
		if !strings.Contains(queries[0], part) {
			t.Errorf("first query %q lacks %q", queries[0], part)
		}
	}
	if !strings.Contains(queries[1], "startIndex=2") {
		t.Errorf("second query %q doesn't start at 2", queries[1])
	}
}

func TestItemsSinceTheZeroTimeListsEverything(t *testing.T) {
	recorded := newRecordedLibraryServer(t)
	if _, err := newLibrary(t, recorded.server.URL).ItemsSince(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if query := recorded.recordedQueries()[0]; strings.Contains(query, queryMinDateLastSaved) {
		t.Errorf("query %q filters by date", query)
	}
}

func TestChangesKeepsTheSocketAliveAndStreamsLibraryChanged(t *testing.T) {
	recorded := newRecordedLibraryServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changes, err := newLibrary(t, recorded.server.URL).Changes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-recorded.keepAlive:
	case <-time.After(waitForSocket):
		t.Fatal("no KeepAlive after ForceKeepAlive")
	}
	select {
	case change := <-changes:
		want := library.LibraryChange{
			Added:   []string{recordedEpisodeID},
			Updated: []string{"8a7b6c5d4e3f4a2b1c0d9e8f7a6b5c4d", "3f2e1d0c9b8a4f7e6d5c4b3a2f1e0d9c"},
			Removed: []string{"0f1e2d3c4b5a4f6e7d8c9b0a1f2e3d4c"},
		}
		if !slices.Equal(change.Added, want.Added) || !slices.Equal(change.Updated, want.Updated) || !slices.Equal(change.Removed, want.Removed) {
			t.Errorf("change = %+v, want %+v", change, want)
		}
	case <-time.After(waitForSocket):
		t.Fatal("no LibraryChanged")
	}

	cancel()
	drained(t, changes)
}

func TestChangesClosesWhenJellyfinDropsTheSocket(t *testing.T) {
	recorded := newRecordedLibraryServer(t)
	recorded.closeSocket = true

	changes, err := newLibrary(t, recorded.server.URL).Changes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	<-recorded.keepAlive
	drained(t, changes)
}

// drained waits for changes to close, discarding what is left.
func drained(t *testing.T, changes <-chan library.LibraryChange) {
	t.Helper()
	timeout := time.After(waitForSocket)
	for {
		select {
		case _, open := <-changes:
			if !open {
				return
			}
		case <-timeout:
			t.Fatal("Changes stayed open")
		}
	}
}

func TestLibraryFailsAsUnavailableWithoutJellyfin(t *testing.T) {
	recorded := newRecordedLibraryServer(t)
	closedURL := recorded.server.URL
	recorded.server.Close()
	ctx := context.Background()

	for name, url := range map[string]string{"unreachable": closedURL, "not configured": ""} {
		l := newLibrary(t, url)
		if err := l.RefreshPath(ctx, recordedMoviePath); !errors.Is(err, library.ErrJellyfinUnavailable) {
			t.Errorf("%s RefreshPath err = %v", name, err)
		}
		if _, err := l.Items(ctx, []string{recordedMovieID}); !errors.Is(err, library.ErrJellyfinUnavailable) {
			t.Errorf("%s Items err = %v", name, err)
		}
		if _, err := l.ItemsSince(ctx, time.Time{}); !errors.Is(err, library.ErrJellyfinUnavailable) {
			t.Errorf("%s ItemsSince err = %v", name, err)
		}
		if _, err := l.Changes(ctx); !errors.Is(err, library.ErrJellyfinUnavailable) {
			t.Errorf("%s Changes err = %v", name, err)
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
