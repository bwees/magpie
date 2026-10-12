package opensubtitles

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/platform/settings"
	"magpie/internal/subtitles"
)

const (
	clientFixtures     = "testdata/client"
	fixtureSearchShow  = "search_episode.json"
	fixtureSearchMovie = "search_movie.json"
	fixtureDownload    = "download.json"
	fixtureQuota       = "download_quota.json"
	fixtureSubtitle    = "subtitle.srt"
	pathSubtitleFile   = "/download/8916041/Game.of.Thrones.S08E02.srt"
	baseMarker         = "{{BASE}}"

	episodeFileID    = 8916041
	retryAfterSec    = "7"
	retryAfterWait   = 7 * time.Second
	recordedAllowed  = 20
	recordedLeft     = 16
	recordedResetDay = 2
)

var recordedReset = time.Date(2026, time.October, recordedResetDay, 0, 0, 0, 0, time.UTC)

// answer is one recorded OpenSubtitles response.
type answer struct {
	status  int
	fixture string
	header  map[string]string
}

// recordedAPI serves recorded fixtures by path, each path's answers in order, repeating the last.
type recordedAPI struct {
	t      *testing.T
	mu     sync.Mutex
	routes map[string][]answer
	seen   []*http.Request
	bodies []string
	server *httptest.Server
}

func newRecordedAPI(t *testing.T, routes map[string][]answer) *recordedAPI {
	t.Helper()
	api := &recordedAPI{t: t, routes: routes}
	api.server = httptest.NewServer(http.HandlerFunc(api.serve))
	t.Cleanup(api.server.Close)
	return api
}

func (a *recordedAPI) serve(w http.ResponseWriter, r *http.Request) {
	body := make([]byte, r.ContentLength)
	_, _ = r.Body.Read(body)
	a.mu.Lock()
	a.seen = append(a.seen, r)
	a.bodies = append(a.bodies, string(body))
	queue, found := a.routes[r.URL.Path]
	var next answer
	if found {
		next = queue[0]
		if len(queue) > 1 {
			a.routes[r.URL.Path] = queue[1:]
		}
	}
	a.mu.Unlock()
	if !found {
		a.t.Errorf("unexpected request %s", r.URL)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.URL.Path != pathSubtitleFile && (r.Header.Get(apiKeyHeader) != testAppKey || r.Header.Get(userAgentHeader) != userAgent) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	for name, value := range next.header {
		w.Header().Set(name, value)
	}
	w.WriteHeader(next.status)
	if next.fixture == "" {
		return
	}
	content, err := os.ReadFile(filepath.Join(clientFixtures, next.fixture))
	if err != nil {
		content, err = os.ReadFile(filepath.Join(fixtureDir, next.fixture))
	}
	if err != nil {
		a.t.Error(err)
		return
	}
	_, _ = w.Write([]byte(strings.ReplaceAll(string(content), baseMarker, a.server.URL)))
}

func (a *recordedAPI) requests() []*http.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]*http.Request{}, a.seen...)
}

func ok(fixture string) answer { return answer{status: http.StatusOK, fixture: fixture} }

// newTestClient talks to api with a login of user/pass and records the waits its spacer asks for.
func newTestClient(api *recordedAPI, user, pass string) (*Client, *[]time.Duration) {
	var waits []time.Duration
	var mu sync.Mutex
	now := time.Date(2026, time.October, 1, 16, 35, 0, 0, time.UTC)
	c := NewClient(Config{
		Login: func(context.Context) (settings.OpenSubtitlesConnection, error) {
			return settings.OpenSubtitlesConnection{Username: user, Password: pass}, nil
		},
		BaseURL: api.server.URL, AppKey: testAppKey, HTTP: api.server.Client(),
		Now: func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error {
			mu.Lock()
			defer mu.Unlock()
			waits = append(waits, d)
			return nil
		},
	})
	return c, &waits
}

func TestLoginReadsTheConnectionAndKeepsTheToken(t *testing.T) {
	api := newRecordedAPI(t, map[string][]answer{
		pathLogin:        {ok(fixtureLogin)},
		pathDownload:     {ok(fixtureDownload)},
		pathSubtitleFile: {ok(fixtureSubtitle)},
	})
	c, _ := newTestClient(api, testUsername, testPassword)

	token, err := c.Login(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != recordedToken || token.VIP || token.AllowedDownloads != recordedAllowed {
		t.Errorf("token = %+v", token)
	}
	var sent loginRequest
	if err := json.Unmarshal([]byte(api.bodies[0]), &sent); err != nil || sent.Username != testUsername || sent.Password != testPassword {
		t.Errorf("login body = %q", api.bodies[0])
	}

	content, remaining, resetAt, err := c.Download(context.Background(), episodeFileID)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(filepath.Join(clientFixtures, fixtureSubtitle))
	if string(content) != string(want) || remaining != recordedLeft || !resetAt.Equal(recordedReset) {
		t.Errorf("Download = %q, %d, %v", content, remaining, resetAt)
	}
	reqs := api.requests()
	if got := reqs[1].Header.Get(authorizationHeader); got != bearerPrefix+recordedToken {
		t.Errorf("download Authorization = %q", got)
	}
	if !strings.Contains(api.bodies[1], `"file_id":8916041`) {
		t.Errorf("download body = %q", api.bodies[1])
	}
}

func TestLoginErrors(t *testing.T) {
	api := newRecordedAPI(t, map[string][]answer{pathLogin: {{status: http.StatusUnauthorized, fixture: fixtureLoginDeny}}})
	c, _ := newTestClient(api, testUsername, "wrong")
	if _, err := c.Login(context.Background()); !errors.Is(err, subtitles.ErrAuth) {
		t.Errorf("rejected login = %v, want ErrAuth", err)
	}

	unset, _ := newTestClient(api, "", "")
	if _, err := unset.Login(context.Background()); !errors.Is(err, subtitles.ErrAuth) {
		t.Errorf("login without credentials = %v, want ErrAuth", err)
	}
	if n := len(api.requests()); n != 1 {
		t.Errorf("%d requests, want none for the missing login", n-1)
	}
}

func TestABuildWithoutTheAppKeyNeverCallsOut(t *testing.T) {
	api := newRecordedAPI(t, map[string][]answer{})
	c := NewClient(Config{BaseURL: api.server.URL, HTTP: api.server.Client()})
	c.appKey = ""
	if _, err := c.Search(context.Background(), subtitles.SearchQuery{Tmdb: 438631, Language: "en"}); !errors.Is(err, subtitles.ErrAuth) {
		t.Errorf("Search = %v, want ErrAuth", err)
	}
	if n := len(api.requests()); n != 0 {
		t.Errorf("%d requests, want none", n)
	}
}

func TestSearchAnEpisode(t *testing.T) {
	api := newRecordedAPI(t, map[string][]answer{pathSubtitles: {ok(fixtureSearchShow)}})
	c, _ := newTestClient(api, testUsername, testPassword)

	cands, err := c.Search(context.Background(), subtitles.SearchQuery{
		MovieHash: "8e245d9679d31e12", Tmdb: 1399, Imdb: "tt0944947", Season: 8, Episode: 2, Language: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	const wantQuery = "episode_number=2&languages=en&moviehash=8e245d9679d31e12&parent_tmdb_id=1399&season_number=8"
	if got := api.requests()[0].URL.RawQuery; got != wantQuery {
		t.Errorf("query = %s, want %s", got, wantQuery)
	}
	want := []subtitles.Candidate{
		{FileID: 8916041, Language: "en", ReleaseName: "Game.of.Thrones.S08E02.1080p.WEB.H264-MEMENTO", HashMatch: true,
			Tmdb: 1399, Imdb: "tt944947", Year: 2019, Season: 8, Episode: 2, FPS: 23.976},
		{FileID: 8916130, Language: "en", ReleaseName: "Game.of.Thrones.S08E02.720p.WEB.H264-MEMENTO", HearingImpaired: true,
			Tmdb: 1399, Imdb: "tt944947", Year: 2019, Season: 8, Episode: 2, FPS: 23.976},
		{FileID: 9805233, Language: "pt", ReleaseName: "Game.of.Thrones.S08E02.1080p.WEB.H264-MEMENTO", Forced: true,
			MachineTranslated: true, Tmdb: 1399, Imdb: "tt944947", Year: 2019, Season: 8, Episode: 2},
	}
	if len(cands) != len(want) {
		t.Fatalf("candidates = %+v", cands)
	}
	for i := range want {
		if cands[i] != want[i] {
			t.Errorf("candidate %d = %+v, want %+v", i, cands[i], want[i])
		}
	}
}

func TestSearchAMovieAndRegionalLanguages(t *testing.T) {
	api := newRecordedAPI(t, map[string][]answer{pathSubtitles: {ok(fixtureSearchMovie)}})
	c, _ := newTestClient(api, testUsername, testPassword)

	cands, err := c.Search(context.Background(), subtitles.SearchQuery{Imdb: "tt1160419", Language: "pt"})
	if err != nil || len(cands) != 1 || cands[0].Tmdb != 438631 || cands[0].Imdb != "tt1160419" || cands[0].Year != 2021 {
		t.Fatalf("Search = %+v, %v", cands, err)
	}
	const wantQuery = "imdb_id=1160419&languages=pt-pt%2Cpt-br"
	if got := api.requests()[0].URL.RawQuery; got != wantQuery {
		t.Errorf("query = %s, want %s", got, wantQuery)
	}
}

func TestDownloadErrors(t *testing.T) {
	api := newRecordedAPI(t, map[string][]answer{
		pathLogin: {ok(fixtureLogin)},
		pathDownload: {
			{status: http.StatusNotAcceptable, fixture: fixtureQuota},
			{status: http.StatusTooManyRequests, header: map[string]string{retryAfterHeader: retryAfterSec}},
			{status: http.StatusServiceUnavailable},
			{status: http.StatusUnauthorized},
		},
	})
	c, _ := newTestClient(api, testUsername, testPassword)
	ctx := context.Background()

	if _, _, _, err := c.Download(ctx, episodeFileID); !errors.Is(err, subtitles.ErrAuth) {
		t.Fatalf("download before login = %v, want ErrAuth", err)
	}
	if len(api.requests()) != 0 {
		t.Fatal("download before login reached OpenSubtitles")
	}
	if _, err := c.Login(ctx); err != nil {
		t.Fatal(err)
	}

	_, _, resetAt, err := c.Download(ctx, episodeFileID)
	if !errors.Is(err, subtitles.ErrQuotaExceeded) || !resetAt.Equal(recordedReset) {
		t.Errorf("over quota = %v, %v; want ErrQuotaExceeded with the reset time", resetAt, err)
	}
	_, _, _, err = c.Download(ctx, episodeFileID)
	var retry *subtitles.RetryAfterError
	if !errors.As(err, &retry) || retry.Wait != retryAfterWait || !errors.Is(err, subtitles.ErrRateLimited) {
		t.Errorf("rate limited = %v, want a 7 s Retry-After", err)
	}
	if _, _, _, err = c.Download(ctx, episodeFileID); !errors.Is(err, subtitles.ErrUnavailable) {
		t.Errorf("503 = %v, want ErrUnavailable", err)
	}
	if _, _, _, err = c.Download(ctx, episodeFileID); !errors.Is(err, subtitles.ErrAuth) {
		t.Errorf("401 = %v, want ErrAuth", err)
	}
	requests := len(api.requests())
	if _, _, _, err = c.Download(ctx, episodeFileID); !errors.Is(err, subtitles.ErrAuth) || len(api.requests()) != requests {
		t.Errorf("after a 401 the token should be dropped without another request: %v", err)
	}
}

func TestRequestsAreSpacedToFivePerSecond(t *testing.T) {
	api := newRecordedAPI(t, map[string][]answer{pathSubtitles: {ok(fixtureSearchMovie)}})
	c, waits := newTestClient(api, testUsername, testPassword)
	const calls = 6
	for range calls {
		if _, err := c.Search(context.Background(), subtitles.SearchQuery{Tmdb: 438631, Language: "en"}); err != nil {
			t.Fatal(err)
		}
	}
	if len(*waits) != calls-1 || (*waits)[calls-2] != (calls-1)*requestGap {
		t.Errorf("waits = %v, want each call %s after the one before", *waits, requestGap)
	}
}
