package prowlarr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/domain"
	"magpie/internal/platform/settings"
)

const (
	testAPIKey     = "test-key"
	basePath       = "/prowlarr"
	fixtureDir     = "testdata"
	xmlContentType = "application/xml"
)

// Prowlarr's clock in the fixtures: 1337x is disabled until 13:00.
var fixtureNow = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

type route struct {
	status  int
	fixture string
}

// recorded serves Prowlarr answers captured in testdata under basePath and records every request.
type recorded struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	routes   map[string]route
	requests []*url.URL
}

func newRecorded(t *testing.T) *recorded {
	t.Helper()
	r := &recorded{t: t, routes: map[string]route{
		"/api/v1/indexer":       {http.StatusOK, "indexer.json"},
		"/api/v1/indexerstatus": {http.StatusOK, "indexerstatus.json"},
		"/api/v1/indexerstats":  {http.StatusOK, "indexerstats.json"},
		"/1/api":                {http.StatusTooManyRequests, "caps-1.xml"},
		"/2/api":                {http.StatusOK, "caps-2.xml"},
		"/3/api":                {http.StatusOK, "caps-3.xml"},
	}}
	mux := http.NewServeMux()
	mux.Handle(basePath+"/", http.StripPrefix(basePath, http.HandlerFunc(r.serve)))
	r.server = httptest.NewServer(mux)
	t.Cleanup(r.server.Close)
	return r
}

func (r *recorded) serve(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.requests = append(r.requests, req.URL)
	rt, ok := r.routes[req.URL.Path]
	r.mu.Unlock()

	if req.Header.Get(apiKeyHeader) != testAPIKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !ok {
		http.NotFound(w, req)
		return
	}
	body, err := os.ReadFile(filepath.Join(fixtureDir, rt.fixture))
	if err != nil {
		r.t.Errorf("fixture %s: %v", rt.fixture, err)
		return
	}
	if strings.HasSuffix(rt.fixture, ".xml") {
		w.Header().Set("Content-Type", xmlContentType)
	}
	w.WriteHeader(rt.status)
	_, _ = w.Write(body)
}

func (r *recorded) set(path string, rt route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[path] = rt
}

func (r *recorded) lastRequest() *url.URL {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests[len(r.requests)-1]
}

func (r *recorded) client(apiKey string) *Client {
	c := New(func(context.Context) (settings.ProwlarrConnection, error) {
		return settings.ProwlarrConnection{URL: r.server.URL + basePath, APIKey: apiKey}, nil
	})
	c.now = func() time.Time { return fixtureNow }
	return c
}

func TestIndexersMixesPublicPrivateAndCloudflareFailing(t *testing.T) {
	r := newRecorded(t)

	indexers, err := r.client(testAPIKey).Indexers(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	ids := make([]int, 0, len(indexers))
	for _, ix := range indexers {
		ids = append(ids, ix.ID)
	}
	if !slices.Equal(ids, []int{1, 2, 3}) {
		t.Fatalf("indexer IDs = %v, want [1 2 3]: the usenet and the disabled indexer are left out", ids)
	}

	cloudflare, private, public := indexers[0], indexers[1], indexers[2]

	if cloudflare.Caps != nil {
		t.Errorf("1337x caps = %+v, want nil after its caps call failed", cloudflare.Caps)
	}
	if !strings.Contains(cloudflare.Health.Error, "CloudFlare") {
		t.Errorf("1337x error = %q, want the Torznab error", cloudflare.Health.Error)
	}
	wantSince := time.Date(2026, 10, 11, 9, 2, 44, 0, time.UTC)
	if cloudflare.Health.FailingSince == nil || !cloudflare.Health.FailingSince.Equal(wantSince) {
		t.Errorf("1337x failing since = %v, want %v", cloudflare.Health.FailingSince, wantSince)
	}
	if cloudflare.Privacy != acquisition.PrivacyPublic {
		t.Errorf("1337x privacy = %s", cloudflare.Privacy)
	}

	if private.Privacy != acquisition.PrivacyPrivate || private.Health.Error != "" {
		t.Errorf("BeyondHD = %+v", private)
	}
	if private.Health.AverageResponse != 420*time.Millisecond {
		t.Errorf("BeyondHD average response = %s", private.Health.AverageResponse)
	}
	wantPrivate := acquisition.TorznabCaps{
		MovieSearch: true,
		MovieParams: []acquisition.TorznabParam{acquisition.ParamQuery, acquisition.ParamIMDb, acquisition.ParamTMDB},
		TVSearch:    true,
		TVParams: []acquisition.TorznabParam{
			acquisition.ParamQuery, acquisition.ParamSeason, acquisition.ParamEpisode,
			acquisition.ParamIMDb, acquisition.ParamTVDB, acquisition.ParamTMDB,
		},
		Categories: []int{2000, 2040, 2045, 5000, 5040, 5045, 100001, 100002},
	}
	assertCaps(t, "BeyondHD", private.Caps, wantPrivate)

	wantPublic := acquisition.TorznabCaps{
		TVSearch: true,
		TVParams: []acquisition.TorznabParam{
			acquisition.ParamQuery, acquisition.ParamSeason, acquisition.ParamEpisode, acquisition.ParamIMDb,
		},
		Categories: []int{5000, 5030, 5040},
	}
	assertCaps(t, "EZTV", public.Caps, wantPublic)
}

func assertCaps(t *testing.T, name string, got *acquisition.TorznabCaps, want acquisition.TorznabCaps) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s caps = nil", name)
	}
	if got.MovieSearch != want.MovieSearch || got.TVSearch != want.TVSearch ||
		!slices.Equal(got.MovieParams, want.MovieParams) || !slices.Equal(got.TVParams, want.TVParams) ||
		!slices.Equal(got.Categories, want.Categories) {
		t.Errorf("%s caps = %+v, want %+v", name, *got, want)
	}
}

func TestIndexersErrors(t *testing.T) {
	r := newRecorded(t)
	unconfigured := New(func(context.Context) (settings.ProwlarrConnection, error) {
		return settings.ProwlarrConnection{}, nil
	})
	closed := newRecorded(t)
	closed.server.Close()
	failing := newRecorded(t)
	failing.set("/api/v1/indexer", route{http.StatusServiceUnavailable, "indexer.json"})

	cases := map[string]struct {
		client *Client
		want   error
	}{
		"wrong API key":     {r.client("wrong"), acquisition.ErrUnauthorized},
		"not configured":    {unconfigured, acquisition.ErrUnavailable},
		"connection closed": {closed.client(testAPIKey), acquisition.ErrUnavailable},
		"server error":      {failing.client(testAPIKey), acquisition.ErrUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := tc.client.Indexers(context.Background()); !errors.Is(err, tc.want) {
				t.Errorf("Indexers error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSearchQueriesOneIndexersTorznabEndpoint(t *testing.T) {
	r := newRecorded(t)
	r.set("/2/api", route{http.StatusOK, "search-2.xml"})
	year := 1999

	items, err := r.client(testAPIKey).Search(context.Background(), 2, acquisition.TorznabQuery{
		Kind:       domain.MediaKindMovie,
		Imdb:       "tt0133093",
		Tmdb:       603,
		Year:       &year,
		Categories: []int{2000, 2040},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := r.lastRequest()
	if got.Path != "/2/api" {
		t.Errorf("path = %s, want the indexer's own /2/api", got.Path)
	}
	want := url.Values{
		"t": {"movie"}, "imdbid": {"tt0133093"}, "tmdbid": {"603"}, "year": {"1999"},
		"cat": {"2000,2040"}, "extended": {"1"},
	}
	if got.Query().Encode() != want.Encode() {
		t.Errorf("query = %s, want %s", got.Query().Encode(), want.Encode())
	}

	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	remux, magnet := items[0], items[1]
	if remux.Seeders != 57 || remux.Peers != 61 || remux.SizeBytes != 63350767616 ||
		remux.InfoHash != "0a1b2c3d4e5f60718293a4b5c6d7e8f901234567" || remux.DownloadVolumeFactor != 0 ||
		!strings.Contains(remux.DownloadURL, "/2/download") || remux.MagnetURI != "" {
		t.Errorf("remux item = %+v", remux)
	}
	if !remux.PublishedAt.Equal(time.Date(2026, 10, 11, 8, 30, 0, 0, time.UTC)) {
		t.Errorf("published = %s", remux.PublishedAt)
	}
	if magnet.DownloadURL != "" || !strings.HasPrefix(magnet.MagnetURI, "magnet:?xt=urn:btih:89ABCDEF") || magnet.DownloadVolumeFactor != 1 {
		t.Errorf("magnet item = %+v", magnet)
	}
}

func TestTVSearchAndRSSURLs(t *testing.T) {
	r := newRecorded(t)
	r.set("/3/api", route{http.StatusOK, "search-2.xml"})
	c := r.client(testAPIKey)
	season, episode := 2, 5

	if _, err := c.Search(context.Background(), 3, acquisition.TorznabQuery{
		Kind: domain.MediaKindSeries, Query: "Severance", Season: &season, Episode: &episode,
	}); err != nil {
		t.Fatal(err)
	}
	tv := r.lastRequest()
	if tv.Path != "/3/api" || tv.Query().Encode() != "ep=5&extended=1&q=Severance&season=2&t=tvsearch" {
		t.Errorf("tv search = %s?%s", tv.Path, tv.RawQuery)
	}

	if _, err := c.RSS(context.Background(), 3, []int{5000}); err != nil {
		t.Fatal(err)
	}
	rss := r.lastRequest()
	if rss.Path != "/3/api" || rss.Query().Encode() != "cat=5000&extended=1&t=search" {
		t.Errorf("rss = %s?%s", rss.Path, rss.RawQuery)
	}
}

func TestSearchTorznabErrorIsIndexerFailed(t *testing.T) {
	r := newRecorded(t)

	_, err := r.client(testAPIKey).Search(context.Background(), 1, acquisition.TorznabQuery{Query: "matrix"})

	var failed *acquisition.ErrIndexerFailed
	if !errors.As(err, &failed) || failed.Code != http.StatusTooManyRequests {
		t.Fatalf("error = %v, want ErrIndexerFailed 429", err)
	}
}

func TestDownload(t *testing.T) {
	const (
		magnet  = "magnet:?xt=urn:btih:89ABCDEF0123456789ABCDEF0123456789ABCDEF&dn=The.Matrix"
		torrent = "d8:announce22:http://tracker/announcee"
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/2/download", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, magnet, http.StatusMovedPermanently)
	})
	mux.HandleFunc("/3/download", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/files/release.torrent", http.StatusFound)
	})
	mux.HandleFunc("/files/release.torrent", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(torrent))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := New(func(context.Context) (settings.ProwlarrConnection, error) {
		return settings.ProwlarrConnection{URL: server.URL, APIKey: testAPIKey}, nil
	})

	payload, err := c.Download(context.Background(), server.URL+"/2/download?apikey=test-key&link=abc")
	if err != nil {
		t.Fatal(err)
	}
	if payload.MagnetRedirect != magnet || payload.Torrent != nil {
		t.Errorf("magnet payload = %+v", payload)
	}

	payload, err = c.Download(context.Background(), server.URL+"/3/download?apikey=test-key&link=def")
	if err != nil {
		t.Fatal(err)
	}
	if string(payload.Torrent) != torrent || payload.MagnetRedirect != "" {
		t.Errorf("torrent payload = %+v", payload)
	}
}

func TestWebURLIsTheConfiguredURL(t *testing.T) {
	r := newRecorded(t)
	got, err := r.client(testAPIKey).WebURL(context.Background())
	if err != nil || got != r.server.URL+basePath {
		t.Errorf("WebURL = %q, %v", got, err)
	}
}
