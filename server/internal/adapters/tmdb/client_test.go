package tmdb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/domain"
)

const (
	conclave       domain.TmdbID = 974576
	severance      domain.TmdbID = 95396
	productionList               = "64a1f6ae8c0a48013b6c2bd0"
	metadataLang                 = "en-US"
	readAccessJWT                = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ4In0.c2lnbmF0dXJl"
)

func date(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestMovieMapsDetailsReleaseDatesExternalIDsAndImages(t *testing.T) {
	r := newReplay(t, map[string][]response{"/movie/974576": {ok("movie_974576.json")}})
	movie, err := newTestClient(r, testAPIKey, &sleeps{}).Movie(context.Background(), conclave, metadataLang)
	if err != nil {
		t.Fatal(err)
	}

	if movie.ID != conclave || movie.Title != "Conclave" || movie.OriginalLanguage != "en" || movie.RuntimeMinutes != 120 {
		t.Errorf("details = %+v", movie)
	}
	if movie.ReleaseDate == nil || !movie.ReleaseDate.Equal(date(t, "2024-10-25")) {
		t.Errorf("release date = %v", movie.ReleaseDate)
	}
	if want := []string{"Konklave", "Conclave - Il Conclave"}; !slices.Equal(movie.AlternativeTitles, want) {
		t.Errorf("aliases = %v, want %v", movie.AlternativeTitles, want)
	}
	if movie.External.Imdb == nil || *movie.External.Imdb != "tt20215234" || movie.External.Tvdb != nil {
		t.Errorf("external = %+v", movie.External)
	}
	wantImages := catalog.TmdbImages{
		Poster:   "/m5x8D0bZ3eKqIVWZ5y7TnZ2oTVg.jpg",
		Backdrop: "/2n7lYEeIbucsEQCswRcVB6ZYmMP.jpg",
		Logo:     "/conclave-logo-en.png",
	}
	if movie.Images != wantImages {
		t.Errorf("images = %+v, want %+v (SVG logos are skipped)", movie.Images, wantImages)
	}
	digitalUS := catalog.TmdbReleaseDate{Region: "US", Type: catalog.TmdbReleaseDigital, Date: date(t, "2024-11-12")}
	if len(movie.ReleaseDates) != 6 || !slices.Contains(movie.ReleaseDates, digitalUS) {
		t.Errorf("release dates = %+v", movie.ReleaseDates)
	}

	query := r.requests()[0].URL.Query()
	if query.Get("api_key") != testAPIKey || query.Get("language") != metadataLang {
		t.Errorf("query = %v", query)
	}
	if query.Get("append_to_response") != "release_dates,external_ids,images,alternative_titles" {
		t.Errorf("append_to_response = %q", query.Get("append_to_response"))
	}
	if query.Get("include_image_language") != "en,null" {
		t.Errorf("include_image_language = %q", query.Get("include_image_language"))
	}
}

func TestReadAccessTokenIsSentAsBearer(t *testing.T) {
	r := newReplay(t, map[string][]response{"/movie/974576": {ok("movie_974576.json")}})
	if _, err := newTestClient(r, readAccessJWT, &sleeps{}).Movie(context.Background(), conclave, metadataLang); err != nil {
		t.Fatal(err)
	}
	req := r.requests()[0]
	if got := req.Header.Get("Authorization"); got != "Bearer "+readAccessJWT {
		t.Errorf("Authorization = %q", got)
	}
	if req.URL.Query().Has("api_key") {
		t.Error("read access token also sent as api_key")
	}
}

func TestSeriesSeasonAndEpisodeGroups(t *testing.T) {
	r := newReplay(t, map[string][]response{
		"/tv/95396":                           {ok("tv_95396.json")},
		"/tv/95396/season/1":                  {ok("tv_95396_season_1.json")},
		"/tv/95396/episode_groups":            {ok("tv_95396_episode_groups.json")},
		"/tv/episode_group/" + productionList: {ok("episode_group_64a1f6ae8c0a48013b6c2bd0.json")},
	})
	client := newTestClient(r, testAPIKey, &sleeps{})
	ctx := context.Background()

	series, err := client.Series(ctx, severance, metadataLang)
	if err != nil {
		t.Fatal(err)
	}
	if series.Name != "Severance" || series.Ended || series.EpisodeRuntimeMinutes != 55 || len(series.Seasons) != 3 {
		t.Errorf("series = %+v", series)
	}
	if series.External.Tvdb == nil || *series.External.Tvdb != "371980" {
		t.Errorf("tvdb = %v", series.External.Tvdb)
	}
	if series.Seasons[2].AirDate != nil || series.Images.Logo != "/severance-logo.png" {
		t.Errorf("season 3 = %+v, logo = %q", series.Seasons[2], series.Images.Logo)
	}

	season, err := client.Season(ctx, severance, 1, metadataLang)
	if err != nil {
		t.Fatal(err)
	}
	if season.Series != severance || len(season.Episodes) != 2 || season.Episodes[1].ID != 3364813 || season.Episodes[1].RuntimeMinutes != 0 {
		t.Errorf("season = %+v", season)
	}

	groups, err := client.EpisodeGroups(ctx, severance)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Type != catalog.TmdbGroupProduction || groups[0].EpisodeCount != 19 {
		t.Errorf("groups = %+v", groups)
	}

	group, err := client.EpisodeGroup(ctx, productionList)
	if err != nil {
		t.Fatal(err)
	}
	if len(group.Groups) != 1 || group.Groups[0].Episodes[1] != (catalog.TmdbEpisodeGroupEntry{Episode: 3364813, Order: 1}) {
		t.Errorf("group = %+v", group)
	}
}

func TestSearchReturnsMoviesAndSeriesWithoutPeople(t *testing.T) {
	r := newReplay(t, map[string][]response{"/search/multi": {ok("search_multi_dune.json")}})
	page, err := newTestClient(r, testAPIKey, &sleeps{}).Search(context.Background(), "dune", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Page != 1 || page.TotalPages != 3 || page.TotalResults != 52 {
		t.Errorf("page = %+v", page)
	}
	want := []catalog.TmdbSearchItem{
		{ID: 693134, Kind: domain.MediaKindMovie, Name: "Dune: Part Two", OriginalName: "Dune: Part Two", Year: 2024,
			Overview:   "Follow the mythic journey of Paul Atreides as he unites with Chani and the Fremen while on a path of revenge against the conspirators who destroyed his family.",
			PosterPath: "/1pdfLvkbY9ohJlCjQH2CZjjYVvJ.jpg"},
		{ID: 90228, Kind: domain.MediaKindSeries, Name: "Dune: Prophecy", OriginalName: "Dune: Prophecy", Year: 2024,
			Overview:   "Ten thousand years before the ascension of Paul Atreides, two Harkonnen sisters combat forces that threaten the future of humankind.",
			PosterPath: "/7mD7ihV4ILvhyX4wB3o9OHyTvVC.jpg"},
		{ID: 841, Kind: domain.MediaKindMovie, Name: "Dune", OriginalName: "Dune",
			Overview: "In the year 10,191, the most precious substance in the universe is the spice Melange."},
	}
	if !slices.Equal(page.Items, want) {
		t.Errorf("items = %+v\nwant %+v", page.Items, want)
	}
	query := r.requests()[0].URL.Query()
	if query.Get("query") != "dune" || query.Get("page") != "1" || query.Get("include_adult") != "false" {
		t.Errorf("query = %v", query)
	}
}

func TestTrendingReturnsMoviesAndSeries(t *testing.T) {
	r := newReplay(t, map[string][]response{"/trending/all/week": {ok("trending_all_week.json")}})
	items, err := newTestClient(r, testAPIKey, &sleeps{}).Trending(context.Background(), catalog.TmdbTrendingWeek)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "Moana 2" || items[1].Kind != domain.MediaKindSeries || items[1].Year != 2022 {
		t.Errorf("items = %+v", items)
	}
}

func TestChangesFollowsEveryPage(t *testing.T) {
	r := newReplay(t, map[string][]response{"/movie/changes": {ok("movie_changes_page1.json"), ok("movie_changes_page2.json")}})
	since := time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)
	ids, err := newTestClient(r, testAPIKey, &sleeps{}).Changes(context.Background(), domain.MediaKindMovie, since)
	if err != nil {
		t.Fatal(err)
	}
	if want := []domain.TmdbID{974576, 693134, 1110034, 1241982, 329865}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	requests := r.requests()
	if len(requests) != 2 || requests[1].URL.Query().Get("page") != "2" || requests[0].URL.Query().Get("start_date") != "2026-10-04" {
		t.Errorf("requests = %v", requests)
	}
}

func TestFindByTvdbID(t *testing.T) {
	r := newReplay(t, map[string][]response{"/find/371980": {ok("find_tvdb_371980.json")}})
	found, err := newTestClient(r, testAPIKey, &sleeps{}).Find(context.Background(), catalog.TmdbFindTvdb, "371980")
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Movies) != 0 || len(found.Series) != 1 || found.Series[0].ID != severance {
		t.Errorf("found = %+v", found)
	}
	if want := (catalog.TmdbFoundEpisode{ID: 1981452, Series: severance, Season: 1, Number: 1}); len(found.Episodes) != 1 || found.Episodes[0] != want {
		t.Errorf("episodes = %+v", found.Episodes)
	}
	if source := r.requests()[0].URL.Query().Get("external_source"); source != catalog.TmdbFindTvdb {
		t.Errorf("external_source = %q", source)
	}
}

func TestImageDownloadsFromTheImageHost(t *testing.T) {
	r := newReplay(t, map[string][]response{imagePathPrefix + "/w780/poster.jpg": {ok("poster.jpg")}})
	body, err := newTestClient(r, testAPIKey, &sleeps{}).Image(context.Background(), "/poster.jpg", "w780")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil || len(data) == 0 {
		t.Errorf("image body = %d bytes, %v", len(data), err)
	}
	if r.requests()[0].URL.Query().Has("api_key") {
		t.Error("image request carries the api key")
	}
}

func TestImageURLPointsAtTheImageHost(t *testing.T) {
	client := NewClient(Config{})
	if got := client.ImageURL("/poster.jpg", "w342"); got != "https://image.tmdb.org/t/p/w342/poster.jpg" {
		t.Errorf("ImageURL = %q", got)
	}
}

// Seed catalog #5: TMDB returns 429 twice then 200; the adapter retries and the call succeeds.
func TestTooManyRequestsRetriesHonouringRetryAfter(t *testing.T) {
	limited := response{status: http.StatusTooManyRequests, fixture: "error_429.json", header: map[string]string{headerRetry: "3"}}
	limitedNoHeader := response{status: http.StatusTooManyRequests, fixture: "error_429.json"}
	r := newReplay(t, map[string][]response{"/movie/974576": {limited, limitedNoHeader, ok("movie_974576.json")}})
	s := &sleeps{}

	movie, err := newTestClient(r, testAPIKey, s).Movie(context.Background(), conclave, metadataLang)
	if err != nil {
		t.Fatalf("Movie after two 429s: %v", err)
	}
	if movie.ID != conclave {
		t.Errorf("movie = %+v", movie)
	}
	if want := []time.Duration{3 * time.Second, time.Second}; !slices.Equal(s.all(), want) {
		t.Errorf("waits = %v, want %v (Retry-After, then the first backoff)", s.all(), want)
	}
}

func TestTooManyRequestsGivesUpAfterFiveRetries(t *testing.T) {
	r := newReplay(t, map[string][]response{"/movie/974576": {{status: http.StatusTooManyRequests, fixture: "error_429.json"}}})
	s := &sleeps{}

	_, err := newTestClient(r, testAPIKey, s).Movie(context.Background(), conclave, metadataLang)
	if !errors.Is(err, catalog.ErrTmdbUnavailable) {
		t.Fatalf("err = %v, want ErrTmdbUnavailable", err)
	}
	if got := len(r.requests()); got != 6 {
		t.Errorf("requests = %d, want 1 + 5 retries", got)
	}
	if want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}; !slices.Equal(s.all(), want) {
		t.Errorf("waits = %v, want %v", s.all(), want)
	}
}

func TestBackoffDoublesUpToThirtySeconds(t *testing.T) {
	var got []time.Duration
	for delay := initialBackoff; len(got) < 7; delay = nextBackoff(delay) {
		got = append(got, delay)
	}
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30}
	for i := range want {
		want[i] *= time.Second
	}
	if !slices.Equal(got, want) {
		t.Errorf("backoff = %v, want %v", got, want)
	}
}

func TestServerErrorsRetryLikeRateLimits(t *testing.T) {
	r := newReplay(t, map[string][]response{"/movie/974576": {{status: http.StatusBadGateway}, ok("movie_974576.json")}})
	if _, err := newTestClient(r, testAPIKey, &sleeps{}).Movie(context.Background(), conclave, metadataLang); err != nil {
		t.Fatal(err)
	}
}

func TestStatusErrorsMapToPortSentinels(t *testing.T) {
	cases := []struct {
		name   string
		status int
		file   string
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, "error_401.json", catalog.ErrTmdbUnauthorized},
		{"not found", http.StatusNotFound, "error_404.json", catalog.ErrTmdbNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newReplay(t, map[string][]response{"/movie/974576": {{status: tc.status, fixture: tc.file}}})
			_, err := newTestClient(r, testAPIKey, &sleeps{}).Movie(context.Background(), conclave, metadataLang)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if len(r.requests()) != 1 {
				t.Errorf("retried a %d", tc.status)
			}
		})
	}
}

func TestMissingKeyIsUnauthorizedWithoutCallingTMDB(t *testing.T) {
	r := newReplay(t, map[string][]response{})
	_, err := newTestClient(r, "", &sleeps{}).Trending(context.Background(), catalog.TmdbTrendingWeek)
	if !errors.Is(err, catalog.ErrTmdbUnauthorized) {
		t.Errorf("err = %v, want ErrTmdbUnauthorized", err)
	}
	if len(r.requests()) != 0 {
		t.Error("called TMDB without a key")
	}
}
