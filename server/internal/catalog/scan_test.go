package catalog_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"magpie/internal/catalog"
	"magpie/internal/catalog/api"
	"magpie/internal/catalog/fakes"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
)

const (
	duneTwoTmdb  domain.TmdbID = 693134
	duneTmdb     domain.TmdbID = 438631
	duneTwoYear                = 2024
	duneYear                   = 2021
	moviesFolder               = "/media/movies/"
	tvFolder                   = "/media/tv/"
)

func duneMovie(id domain.TmdbID, name string, year int) catalog.TmdbMovie {
	return catalog.TmdbMovie{
		ID: id, Title: name, OriginalTitle: name, OriginalLanguage: "en", RuntimeMinutes: 160,
		ReleaseDate: ptr(startOfTest.AddDate(year-startOfTest.Year(), 0, 0)),
	}
}

func folderFound(path, title string, year *int, hint *domain.TmdbID) libraryapi.FolderFound {
	return libraryapi.FolderFound{Path: path, Kind: domain.MediaKindMovie, ParsedTitle: title, Year: year, TmdbHint: hint}
}

func (h *harness) deliverFolder(found libraryapi.FolderFound) {
	h.t.Helper()
	if err := h.handle(catalog.NewFolderFoundHandler(h.tasks), found); err != nil {
		h.t.Fatal(err)
	}
	h.runJobs()
}

func (h *harness) reviewRows() []catalog.ScanReviewResponse {
	h.t.Helper()
	rec := h.request(http.MethodGet, "/api/catalog/scan-review", nil, admin)
	return decodeJSON[[]catalog.ScanReviewResponse](h.t, rec)
}

// Seed catalog #17.
func TestATaggedFolderCreatesTheTitleAndMatchesOnce(t *testing.T) {
	h := newHarness(t)
	h.tmdb.PutMovie(duneMovie(duneTwoTmdb, "Dune: Part Two", duneTwoYear))
	folder := moviesFolder + "Dune Part Two (2024) [tmdbid-693134]"
	found := folderFound(folder, "Dune Part Two", ptr(duneTwoYear), ptr(duneTwoTmdb))

	h.deliverFolder(found)
	h.deliverFolder(found)

	title, err := h.repo.TitleByTmdb(context.Background(), domain.MediaKindMovie, duneTwoTmdb)
	if err != nil {
		t.Fatal(err)
	}
	if title.Origin != domain.OriginLibraryScan || !title.Monitored || title.Profile != "" {
		t.Errorf("title = %+v, want a monitored LIBRARY_SCAN title on the default profile", title)
	}
	added := events[api.TitleAdded](h)
	if len(added) != 1 || added[0].Origin != domain.OriginLibraryScan || added[0].ScanFolder != folder || added[0].Title != title.ID {
		t.Fatalf("TitleAdded = %+v, want one LIBRARY_SCAN with the folder", added)
	}
	matched := events[api.ScanFolderMatched](h)
	if len(matched) != 1 || matched[0] != (api.ScanFolderMatched{Folder: folder, Title: title.ID}) {
		t.Fatalf("ScanFolderMatched = %+v, want one for the folder", matched)
	}
	if order := h.eventTypes(); slices.Index(order, api.TypeTitleAdded) > slices.Index(order, api.TypeScanFolderMatched) {
		t.Errorf("events in order %v, want TitleAdded before ScanFolderMatched", order)
	}
	if rows := h.reviewRows(); len(rows) != 0 {
		t.Errorf("review queue = %+v, want it empty", rows)
	}
}

// Seed catalog #18.
func TestAnUncertainFolderWaitsForTheAdmin(t *testing.T) {
	h := newHarness(t)
	h.tmdb.PutMovie(duneMovie(duneTwoTmdb, "Dune: Part Two", duneTwoYear))
	h.tmdb.PutMovie(duneMovie(duneTmdb+1, "Dune Drifter", duneYear))
	folder := moviesFolder + "Dune"

	h.deliverFolder(folderFound(folder, "Dune", nil, nil))
	h.deliverFolder(folderFound(folder, "Dune", nil, nil))

	rows := h.reviewRows()
	if len(rows) != 1 || rows[0].FolderPath != folder || rows[0].State != catalog.ScanReviewPending || len(rows[0].Candidates) != 2 {
		t.Fatalf("review queue = %+v, want one pending row with two candidates", rows)
	}
	for _, c := range rows[0].Candidates {
		if c.Score >= catalog.MatchThreshold {
			t.Errorf("candidate %+v scores at least the threshold", c)
		}
	}
	if n := len(events[api.TitleAdded](h)) + len(events[api.ScanFolderMatched](h)); n != 0 {
		t.Fatalf("%d events published before the admin chose", n)
	}

	rec := h.request(http.MethodPost, "/api/catalog/scan-review/"+rows[0].ID+"/match", catalog.MatchScanReviewRequest{TmdbID: duneTwoTmdb}, admin)
	got := decodeJSON[catalog.ScanReviewResponse](t, rec)

	if got.State != catalog.ScanReviewMatched {
		t.Errorf("row state = %s, want MATCHED", got.State)
	}
	matched := events[api.ScanFolderMatched](h)
	if len(matched) != 1 || matched[0].Folder != folder {
		t.Fatalf("ScanFolderMatched = %+v, want one for the folder", matched)
	}
	if rows := h.reviewRows(); len(rows) != 0 {
		t.Errorf("review queue = %+v after the match, want it empty", rows)
	}
	again := h.request(http.MethodPost, "/api/catalog/scan-review/"+rows[0].ID+"/match", catalog.MatchScanReviewRequest{TmdbID: duneTwoTmdb}, admin)
	if again.Code != http.StatusConflict {
		t.Errorf("matching a matched row = %d, want 409", again.Code)
	}
}

// Seed catalog #19.
func TestAFolderOfATrackedTitleOnlyMatches(t *testing.T) {
	h := newHarness(t)
	tracked := addMovie(t, h, conclave())
	folder := moviesFolder + "Conclave {tmdb-974576}"

	h.deliverFolder(folderFound(folder, "Conclave", nil, ptr(conclaveTmdb)))

	if added := events[api.TitleAdded](h); len(added) != 1 || added[0].Origin != domain.OriginRequest {
		t.Errorf("TitleAdded = %+v, want only the request's", added)
	}
	matched := events[api.ScanFolderMatched](h)
	if len(matched) != 1 || matched[0] != (api.ScanFolderMatched{Folder: folder, Title: tracked.ID}) {
		t.Errorf("ScanFolderMatched = %+v, want one for the tracked title", matched)
	}
}

func TestAFolderWhoseTitleAndYearMatchOneCandidateMatches(t *testing.T) {
	h := newHarness(t)
	h.tmdb.PutMovie(duneMovie(duneTmdb, "Dune", duneYear))
	h.tmdb.PutMovie(duneMovie(duneTmdb+1, "Dune", 1984))

	h.deliverFolder(folderFound(moviesFolder+"Dune (2021)", "Dune", ptr(duneYear), nil))

	matched := events[api.ScanFolderMatched](h)
	if len(matched) != 1 {
		t.Fatalf("ScanFolderMatched = %+v, want one", matched)
	}
	title, err := h.repo.Title(context.Background(), matched[0].Title)
	if err != nil || title.External.Tmdb != duneTmdb {
		t.Errorf("matched %+v, %v; want the 2021 film", title, err)
	}
}

func TestTwoCertainCandidatesStillGoToReview(t *testing.T) {
	h := newHarness(t)
	h.tmdb.PutMovie(duneMovie(duneTmdb, "Dune", duneYear))
	h.tmdb.PutMovie(duneMovie(duneTmdb+1, "Dune", 1984))

	h.deliverFolder(folderFound(moviesFolder+"Dune", "Dune", nil, nil))

	if rows := h.reviewRows(); len(rows) != 1 || len(rows[0].Candidates) != 2 {
		t.Errorf("review queue = %+v, want the folder with both films", rows)
	}
	if n := len(events[api.ScanFolderMatched](h)); n != 0 {
		t.Errorf("%d ScanFolderMatched, want none", n)
	}
}

func TestATaggedSeriesFolderIsMonitoredWithFutureSeasons(t *testing.T) {
	h := newHarness(t)
	putSeries(h, severanceSeries(airedSeasons()...), airedSeasons()...)
	found := libraryapi.FolderFound{Path: tvFolder + "Severance [tmdbid-95396]", Kind: domain.MediaKindSeries, ParsedTitle: "Severance", TmdbHint: ptr(severanceTmdb)}

	h.deliverFolder(found)

	added := events[api.TitleAdded](h)
	if len(added) != 1 || len(added[0].Monitored) != 2*episodesPerSeas || added[0].Origin != domain.OriginLibraryScan {
		t.Fatalf("TitleAdded = %+v, want the regular seasons monitored", added)
	}
	title, err := h.repo.Title(context.Background(), added[0].Title)
	if err != nil || !title.MonitorFutureSeasons {
		t.Errorf("title = %+v, %v; want future seasons monitored", title, err)
	}
}

func TestTmdbTroubleRetriesTheMatchWithBackoff(t *testing.T) {
	h := newHarness(t)
	h.tmdb.PutMovie(duneMovie(duneTmdb, "Dune", duneYear))
	h.tmdb.FailNext(fakes.TMDBSearch, catalog.ErrTmdbUnavailable, catalog.ErrTmdbUnavailable)

	h.deliverFolder(folderFound(moviesFolder+"Dune (2021)", "Dune", ptr(duneYear), nil))

	if len(h.waits) != 2 || h.waits[1] <= h.waits[0] {
		t.Errorf("retry waits = %v, want two growing waits", h.waits)
	}
	if n := len(events[api.ScanFolderMatched](h)); n != 1 {
		t.Errorf("%d ScanFolderMatched once TMDB answered, want 1", n)
	}
}

func TestIgnoringAFolderTakesItOffTheQueue(t *testing.T) {
	h := newHarness(t)
	h.deliverFolder(folderFound(moviesFolder+"Home Videos", "Home Videos", nil, nil))
	rows := h.reviewRows()
	if len(rows) != 1 || len(rows[0].Candidates) != 0 {
		t.Fatalf("review queue = %+v, want the folder with no candidates", rows)
	}

	rec := h.request(http.MethodPost, "/api/catalog/scan-review/"+rows[0].ID+"/ignore", nil, admin)
	if got := decodeJSON[catalog.ScanReviewResponse](t, rec); got.State != catalog.ScanReviewIgnored {
		t.Errorf("row = %+v, want IGNORED", got)
	}
	if rows := h.reviewRows(); len(rows) != 0 {
		t.Errorf("review queue = %+v, want it empty", rows)
	}
	if rec := h.request(http.MethodGet, "/api/catalog/scan-review", nil, requester); rec.Code != http.StatusForbidden {
		t.Errorf("requester listing the queue = %d, want 403", rec.Code)
	}
}

func TestMatchingAnUnknownTmdbIDKeepsTheRow(t *testing.T) {
	h := newHarness(t)
	h.deliverFolder(folderFound(moviesFolder+"Home Videos", "Home Videos", nil, nil))
	rows := h.reviewRows()

	rec := h.request(http.MethodPost, "/api/catalog/scan-review/"+rows[0].ID+"/match", catalog.MatchScanReviewRequest{TmdbID: duneTmdb}, admin)

	if rec.Code != http.StatusNotFound {
		t.Errorf("match = %d, want 404 for an ID TMDB doesn't know", rec.Code)
	}
	if rows := h.reviewRows(); len(rows) != 1 {
		t.Errorf("review queue = %+v, want the row still pending", rows)
	}
}

func (h *harness) eventTypes() []string {
	h.t.Helper()
	var types []string
	if err := h.db.Read(context.Background()).Table("platform_outbox").Order("seq").Pluck("type", &types).Error; err != nil {
		h.t.Fatal(err)
	}
	return types
}
