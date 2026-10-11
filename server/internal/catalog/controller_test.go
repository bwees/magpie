package catalog_test

import (
	"context"
	"errors"
	"image"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"magpie/internal/catalog"
	"magpie/internal/catalog/api"
	"magpie/internal/catalog/fakes"
	"magpie/internal/domain"
)

const (
	titlesPath   = "/api/catalog/titles"
	discoverPath = "/api/catalog/discover"
	imagesPath   = "/api/catalog/images/"
	settingsPath = "/api/catalog/settings"
)

// Seed catalog #12.
func TestPatchProfilePublishesProfileChanged(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())

	rec := h.request(http.MethodPatch, titlesPath+"/"+string(title.ID), catalog.UpdateTitleRequest{ProfileID: ptr(profileQ)}, admin)
	got := decodeJSON[catalog.TitleResponse](t, rec)

	if got.ProfileID != profileQ {
		t.Errorf("profileId = %q", got.ProfileID)
	}
	changed := events[api.ProfileChanged](h)
	if len(changed) != 1 || changed[0] != (api.ProfileChanged{Title: title.ID, From: profileP, To: profileQ}) {
		t.Errorf("ProfileChanged = %+v", changed)
	}
}

func TestPatchMonitoredPublishesMonitoringChanged(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())

	rec := h.request(http.MethodPatch, titlesPath+"/"+string(title.ID), catalog.UpdateTitleRequest{Monitored: ptr(false)}, admin)
	if got := decodeJSON[catalog.TitleResponse](t, rec); got.Monitored {
		t.Error("still monitored")
	}
	changed := events[api.MonitoringChanged](h)
	if len(changed) != 1 || changed[0].Movie == nil || *changed[0].Movie {
		t.Errorf("MonitoringChanged = %+v", changed)
	}
	if profile := events[api.ProfileChanged](h); len(profile) != 0 {
		t.Errorf("ProfileChanged = %+v without a profile change", profile)
	}
}

func TestAdminRoutesRefuseRequesters(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	path := titlesPath + "/" + string(title.ID)

	for _, rec := range []struct {
		name   string
		status int
	}{
		{"patch", h.request(http.MethodPatch, path, catalog.UpdateTitleRequest{Monitored: ptr(false)}, requester).Code},
		{"delete", h.request(http.MethodDelete, path, nil, requester).Code},
		{"refresh", h.request(http.MethodPost, path+"/refresh", nil, requester).Code},
		{"signed out", h.request(http.MethodPatch, path, catalog.UpdateTitleRequest{Monitored: ptr(false)}, nil).Code},
	} {
		if rec.status != http.StatusForbidden && rec.status != http.StatusUnauthorized {
			t.Errorf("%s: status %d", rec.name, rec.status)
		}
	}
}

// Seed catalog #16.
func TestDeleteRemovesRowsAndImagesAndPublishesTitleRemoved(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	poster := h.image(title.ID, api.ImageKindPoster).LocalPath

	rec := h.request(http.MethodDelete, titlesPath+"/"+string(title.ID)+"?deleteFiles=true", nil, admin)
	decodeJSON[catalog.RemoveTitleResponse](t, rec)

	if _, err := h.repo.Title(context.Background(), title.ID); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("title still readable: %v", err)
	}
	if _, err := h.repo.ImagePath(context.Background(), title.ID, api.ImageKindPoster); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("image row still there: %v", err)
	}
	if _, err := os.Stat(poster); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("image file still there: %v", err)
	}
	removed := events[api.TitleRemoved](h)
	if len(removed) != 1 || removed[0] != (api.TitleRemoved{Title: title.ID, DeleteFiles: true}) {
		t.Errorf("TitleRemoved = %+v", removed)
	}
	if rec := h.request(http.MethodDelete, titlesPath+"/"+string(title.ID), nil, admin); rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
}

func TestGetTitleReturnsTheTitleWithImageURLs(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())

	got := decodeJSON[catalog.TitleResponse](t, h.request(http.MethodGet, titlesPath+"/"+string(title.ID), nil, requester))

	if got.Name != "Conclave" || got.Overview == "" || got.TmdbID != conclaveTmdb || len(got.Seasons) != 0 {
		t.Errorf("title = %+v", got)
	}
	if got.Images.Poster == nil || *got.Images.Poster != imagesPath+string(title.ID)+"/"+api.ImageKindPoster || got.Images.Logo != nil {
		t.Errorf("images = %+v", got.Images)
	}
	if rec := h.request(http.MethodGet, titlesPath+"/"+string(domain.NewID[domain.TitleID]()), nil, requester); rec.Code != http.StatusNotFound {
		t.Errorf("unknown title = %d, want 404", rec.Code)
	}
}

func TestListTitlesSearchesAndFilters(t *testing.T) {
	h := newHarness(t)
	addMovie(t, h, conclave())

	cases := []struct {
		query string
		want  int
	}{
		{"", 1},
		{"?q=concl", 1},
		{"?q=dune", 0},
		{"?kind=SERIES", 0},
		{"?monitored=true&released=false&sort=ADDED", 1},
	}
	for _, tc := range cases {
		got := decodeJSON[[]catalog.TitleResponse](t, h.request(http.MethodGet, titlesPath+tc.query, nil, requester))
		if len(got) != tc.want {
			t.Errorf("%s: %d titles, want %d", tc.query, len(got), tc.want)
		}
	}
	for _, bad := range []string{"?kind=SHOW", "?monitored=maybe", "?sort=RATING"} {
		if rec := h.request(http.MethodGet, titlesPath+bad, nil, requester); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, rec.Code)
		}
	}
}

func TestDiscoverMarksLibraryTitlesAndListsTrendingForAnEmptyQuery(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	other := catalog.TmdbSearchItem{ID: 1, Kind: domain.MediaKindSeries, Name: "Severance", Year: 2022, PosterPath: "/s.jpg"}
	h.tmdb.SetTrending(other)

	trending := decodeJSON[catalog.DiscoverResponse](t, h.request(http.MethodGet, discoverPath, nil, requester))
	if !trending.Trending || len(trending.Items) != 1 || trending.Items[0].InLibrary {
		t.Errorf("trending = %+v", trending)
	}
	if url := trending.Items[0].PosterURL; url == nil || *url != fakes.FakeImageHost+"w342/s.jpg" {
		t.Errorf("posterUrl = %v", url)
	}

	found := decodeJSON[catalog.DiscoverResponse](t, h.request(http.MethodGet, discoverPath+"?q=conclave", nil, requester))
	if found.Trending || len(found.Items) != 1 || !found.Items[0].InLibrary || found.Items[0].TitleID == nil || *found.Items[0].TitleID != title.ID {
		t.Errorf("search = %+v", found)
	}
}

func TestDiscoverDetailReadsTheLibraryOrTmdb(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	putSeries(h, severanceSeries(airedSeasons()...))

	tracked := decodeJSON[catalog.DiscoverDetailResponse](t, h.request(http.MethodGet, discoverPath+"/MOVIE/"+strconv.Itoa(int(conclaveTmdb)), nil, requester))
	wantPoster := imagesPath + string(title.ID) + "/" + api.ImageKindPoster
	if !tracked.InLibrary || tracked.Name != "Conclave" || tracked.PosterURL == nil || *tracked.PosterURL != wantPoster || len(tracked.Seasons) != 0 {
		t.Errorf("tracked movie = %+v", tracked)
	}

	series := decodeJSON[catalog.DiscoverDetailResponse](t, h.request(http.MethodGet, discoverPath+"/SERIES/"+strconv.Itoa(int(severanceTmdb)), nil, requester))
	if series.InLibrary || series.Name != "Severance" || len(series.Seasons) != len(airedSeasons()) || series.Seasons[1].EpisodeCount != episodesPerSeas {
		t.Errorf("series from TMDB = %+v", series)
	}

	if rec := h.request(http.MethodGet, discoverPath+"/SERIES/1", nil, requester); rec.Code != http.StatusNotFound {
		t.Errorf("unknown TMDB ID = %d, want 404", rec.Code)
	}
	if rec := h.request(http.MethodGet, discoverPath+"/PERSON/1", nil, requester); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown kind = %d, want 400", rec.Code)
	}
}

func TestDiscoverReportsTmdbUnavailable(t *testing.T) {
	h := newHarness(t)
	h.tmdb.FailAlways(fakes.TMDBSearch, catalog.ErrTmdbUnavailable)

	rec := h.request(http.MethodGet, discoverPath+"?q=dune", nil, requester)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), string(catalog.TypeTmdbUnavailable)) {
		t.Errorf("status %d: %s", rec.Code, rec.Body)
	}
}

func TestImagesAreServedResized(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	path := imagesPath + string(title.ID) + "/" + api.ImageKindPoster

	cases := []struct {
		query string
		width int
	}{
		{"", posterWidth},
		{"?w=100", 154},
		{"?w=342", 342},
		{"?w=2000", posterWidth},
	}
	for _, tc := range cases {
		rec := h.request(http.MethodGet, path+tc.query, nil, requester)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", tc.query, rec.Code, rec.Body)
		}
		cfg, _, err := image.DecodeConfig(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width != tc.width || cfg.Height != tc.width*posterHeight/posterWidth {
			t.Errorf("%s: %dx%d, want width %d", tc.query, cfg.Width, cfg.Height, tc.width)
		}
	}
	if rec := h.request(http.MethodGet, imagesPath+string(title.ID)+"/"+api.ImageKindLogo, nil, requester); rec.Code != http.StatusNotFound {
		t.Errorf("uncached logo = %d, want 404", rec.Code)
	}
	if rec := h.request(http.MethodGet, path+"?w=-4", nil, requester); rec.Code != http.StatusBadRequest {
		t.Errorf("negative width = %d, want 400", rec.Code)
	}
}

func TestSettingsRoundTripAndValidate(t *testing.T) {
	h := newHarness(t)

	got := decodeJSON[catalog.CatalogSettingsResponse](t, h.request(http.MethodGet, settingsPath, nil, requester))
	if got.Language != catalog.DefaultMetadataLanguage || got.ReleaseRegion != catalog.DefaultReleaseRegion {
		t.Errorf("defaults = %+v", got)
	}

	update := catalog.UpdateCatalogSettingsRequest{Language: "pt-BR", ReleaseRegion: "BR"}
	decodeJSON[catalog.CatalogSettingsResponse](t, h.request(http.MethodPut, settingsPath, update, admin))
	got = decodeJSON[catalog.CatalogSettingsResponse](t, h.request(http.MethodGet, settingsPath, nil, requester))
	if got.Language != update.Language || got.ReleaseRegion != update.ReleaseRegion {
		t.Errorf("after PUT = %+v", got)
	}

	bad := catalog.UpdateCatalogSettingsRequest{Language: "english", ReleaseRegion: "BR"}
	if rec := h.request(http.MethodPut, settingsPath, bad, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid language = %d, want 400", rec.Code)
	}
}

func TestAttributionCarriesTheTmdbNoticeAndLogo(t *testing.T) {
	h := newHarness(t)
	got := decodeJSON[catalog.AttributionResponse](t, h.request(http.MethodGet, "/api/catalog/attribution", nil, nil))
	if got.Notice != "This product uses the TMDB API but is not endorsed or certified by TMDB." {
		t.Errorf("notice = %q", got.Notice)
	}
	if !strings.HasPrefix(got.LogoURL, "data:image/svg+xml;base64,") {
		t.Errorf("logo = %.40q", got.LogoURL)
	}
}

func TestRefreshQueuesAJob(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())

	rec := h.request(http.MethodPost, titlesPath+"/"+string(title.ID)+"/refresh", nil, admin)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	before := h.tmdb.Called(fakes.TMDBMovie)
	h.runJobs()
	if h.tmdb.Called(fakes.TMDBMovie) != before+1 {
		t.Error("refresh job did not read TMDB")
	}
}
