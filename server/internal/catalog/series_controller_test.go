package catalog_test

import (
	"net/http"
	"slices"
	"strconv"
	"testing"

	"magpie/internal/catalog"
	"magpie/internal/catalog/api"
	"magpie/internal/domain"
	requestsapi "magpie/internal/requests/api"
)

const languagesPath = "/api/catalog/languages"

func titlePath(id domain.TitleID) string {
	return titlesPath + "/" + string(id)
}

func TestPatchSeasonSetsItsMonitoring(t *testing.T) {
	h := newHarness(t)
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), airedSeasons()...)

	rec := h.request(http.MethodPatch, titlePath(title.ID)+"/seasons/1", catalog.UpdateMonitoredRequest{Monitored: false}, admin)
	got := decodeJSON[catalog.TitleResponse](t, rec)

	for _, season := range got.Seasons {
		if season.Number != 1 {
			continue
		}
		if season.Monitored || slices.ContainsFunc(season.Episodes, func(e catalog.EpisodeResponse) bool { return e.Monitored }) {
			t.Errorf("season 1 = %+v", season)
		}
	}
	if rec := h.request(http.MethodPatch, titlePath(title.ID)+"/seasons/7", catalog.UpdateMonitoredRequest{Monitored: true}, admin); rec.Code != http.StatusNotFound {
		t.Errorf("unknown season status %d", rec.Code)
	}
	if rec := h.request(http.MethodPatch, titlePath(title.ID)+"/seasons/one", catalog.UpdateMonitoredRequest{Monitored: true}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric season status %d", rec.Code)
	}
}

func TestPatchEpisodeSetsItsMonitoring(t *testing.T) {
	h := newHarness(t)
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), airedSeasons()...)
	special, _ := h.repo.EpisodeByNumber(t.Context(), title.ID, specials, 1)

	rec := h.request(http.MethodPatch, titlePath(title.ID)+"/episodes/"+string(special.ID), catalog.UpdateMonitoredRequest{Monitored: true}, admin)
	decodeJSON[catalog.TitleResponse](t, rec)

	if got, _ := h.repo.Episode(t.Context(), special.ID); !got.Monitored {
		t.Error("special not monitored")
	}
	if rec := h.request(http.MethodPatch, titlePath(title.ID)+"/episodes/unknown", catalog.UpdateMonitoredRequest{Monitored: true}, admin); rec.Code != http.StatusNotFound {
		t.Errorf("unknown episode status %d", rec.Code)
	}
}

func TestEpisodeGroupRoutes(t *testing.T) {
	h := newHarness(t)
	const count = 10
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), tmdbSeason(1, count, startOfTest.AddDate(-1, 0, 0)))
	h.tmdb.PutEpisodeGroup(severanceTmdb, splitGroup(count))

	groups := decodeJSON[[]catalog.EpisodeGroupResponse](t, h.request(http.MethodGet, titlePath(title.ID)+"/episode-groups", nil, admin))
	if len(groups) != 1 || groups[0].ID != splitGroupID || groups[0].Type != catalog.EpisodeGroupTypeDVD || groups[0].EpisodeCount != count {
		t.Errorf("groups = %+v", groups)
	}

	rec := h.request(http.MethodPut, titlePath(title.ID)+"/episode-group", catalog.SelectEpisodeGroupRequest{GroupID: splitGroupID}, admin)
	if got := decodeJSON[catalog.TitleResponse](t, rec); got.EpisodeGroupID != splitGroupID || len(got.Seasons) != 2 {
		t.Errorf("title = %+v", got)
	}
	if rec := h.request(http.MethodPut, titlePath(title.ID)+"/episode-group", catalog.SelectEpisodeGroupRequest{GroupID: "nope"}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown group status %d", rec.Code)
	}
	movie := addMovie(t, h, conclave())
	if rec := h.request(http.MethodGet, titlePath(movie.ID)+"/episode-groups", nil, admin); rec.Code != http.StatusConflict {
		t.Errorf("movie groups status %d", rec.Code)
	}
}

func TestLanguagesRoutes(t *testing.T) {
	h := newHarness(t)

	got := decodeJSON[catalog.WantedLanguagesResponse](t, h.request(http.MethodGet, languagesPath, nil, requester))
	if !slices.Equal(got.Languages, []catalog.WantedLanguageEntry{{Language: "en"}}) {
		t.Errorf("languages = %+v", got.Languages)
	}
	if !slices.Contains(got.Available, catalog.LanguageOptionResponse{Code: "ja", Name: "Japanese"}) {
		t.Errorf("available languages lack Japanese: %d entries", len(got.Available))
	}

	want := []catalog.WantedLanguageEntry{{Language: "es", Forced: true}, {Language: "en", HearingImpaired: true}}
	rec := h.request(http.MethodPut, languagesPath, catalog.UpdateWantedLanguagesRequest{Languages: want}, admin)
	if got := decodeJSON[catalog.WantedLanguagesResponse](t, rec); !slices.Equal(got.Languages, want) {
		t.Errorf("saved = %+v", got.Languages)
	}
	if rec := h.request(http.MethodPut, languagesPath, catalog.UpdateWantedLanguagesRequest{Languages: []catalog.WantedLanguageEntry{}}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("empty list status %d", rec.Code)
	}
	if rec := h.request(http.MethodPut, languagesPath, catalog.UpdateWantedLanguagesRequest{Languages: want}, requester); rec.Code != http.StatusForbidden {
		t.Errorf("requester status %d", rec.Code)
	}
}

func TestTitleLanguagesRoute(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	override := []catalog.WantedLanguageEntry{{Language: "ja"}}

	rec := h.request(http.MethodPut, titlePath(title.ID)+"/languages", catalog.UpdateTitleLanguagesRequest{Languages: override}, admin)
	if got := decodeJSON[catalog.TitleResponse](t, rec); !slices.Equal(got.LanguagesOverride, override) {
		t.Errorf("override = %+v", got.LanguagesOverride)
	}

	rec = h.request(http.MethodPut, titlePath(title.ID)+"/languages", catalog.UpdateTitleLanguagesRequest{}, admin)
	if got := decodeJSON[catalog.TitleResponse](t, rec); got.LanguagesOverride != nil {
		t.Errorf("override after clearing = %+v", got.LanguagesOverride)
	}
	if got, _ := h.repo.WantedLanguages(t.Context(), title.ID); !slices.Equal(got, []api.WantedLanguage{{Language: "en"}}) {
		t.Errorf("WantedLanguages = %+v", got)
	}
}

func TestSeriesRoutesRefuseRequesters(t *testing.T) {
	h := newHarness(t)
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), airedSeasons()...)
	episode, _ := h.repo.EpisodeByNumber(t.Context(), title.ID, 1, 1)
	path := titlePath(title.ID)

	for name, status := range map[string]int{
		"season":    h.request(http.MethodPatch, path+"/seasons/"+strconv.Itoa(1), catalog.UpdateMonitoredRequest{}, requester).Code,
		"episode":   h.request(http.MethodPatch, path+"/episodes/"+string(episode.ID), catalog.UpdateMonitoredRequest{}, requester).Code,
		"group":     h.request(http.MethodPut, path+"/episode-group", catalog.SelectEpisodeGroupRequest{}, requester).Code,
		"languages": h.request(http.MethodPut, path+"/languages", catalog.UpdateTitleLanguagesRequest{}, requester).Code,
	} {
		if status != http.StatusForbidden {
			t.Errorf("%s: status %d", name, status)
		}
	}
}

func TestListedSeriesCarryTheirSeasonsWantedEpisodesAndNextAiring(t *testing.T) {
	h := newHarness(t)
	upcoming := tmdbSeason(3, episodesPerSeas, startOfTest.Add(week))
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), append(airedSeasons(), upcoming)...)
	movie := addMovie(t, h, conclave())

	got := decodeJSON[[]catalog.TitleResponse](t, h.request(http.MethodGet, titlesPath+"?kind=SERIES", nil, requester))
	if len(got) != 1 || got[0].ID != title.ID {
		t.Fatalf("series = %+v, want the one series", got)
	}
	wanted := len(episodeIDs(h.episodes(title.ID), func(e api.Episode) bool { return e.Monitored && e.Released }))
	series := got[0]
	if series.SeasonCount != 3 || series.WantedEpisodeCount != wanted || wanted == 0 {
		t.Errorf("seasons %d, wanted %d; want 3 and %d", series.SeasonCount, series.WantedEpisodeCount, wanted)
	}
	next := series.NextEpisode
	if next == nil || next.Season != 3 || next.Number != 1 || !next.AirDate.Equal(startOfTest.Add(week)) {
		t.Errorf("next episode = %+v, want S03E01 a week out", next)
	}

	byID := decodeJSON[[]catalog.TitleResponse](t, h.request(http.MethodGet, titlesPath+"?ids="+string(movie.ID), nil, requester))
	if len(byID) != 1 || byID[0].ID != movie.ID || byID[0].NextEpisode != nil || byID[0].SeasonCount != 0 {
		t.Errorf("by ID = %+v, want only the movie without a series summary", byID)
	}
	byYear := decodeJSON[[]catalog.TitleResponse](t, h.request(http.MethodGet, titlesPath+"?sort=YEAR", nil, requester))
	if len(byYear) != 2 || byYear[0].Year < byYear[1].Year {
		t.Errorf("by year = %+v, want newest first", byYear)
	}
}

func TestRefreshingASeriesIsQueued(t *testing.T) {
	h := newHarness(t)
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), airedSeasons()...)

	rec := h.request(http.MethodPost, titlePath(title.ID)+"/refresh", nil, admin)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	h.runJobs()
}
