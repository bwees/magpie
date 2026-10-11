package catalog_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/catalog/api"
	"magpie/internal/catalog/fakes"
	"magpie/internal/domain"
	platformapi "magpie/internal/platform/api"
	requestsapi "magpie/internal/requests/api"
)

const (
	conclaveTmdb domain.TmdbID    = 974576
	day                           = 24 * time.Hour
	profileP     domain.ProfileID = "P"
	profileQ     domain.ProfileID = "Q"
)

func conclave(dates ...catalog.TmdbReleaseDate) catalog.TmdbMovie {
	return catalog.TmdbMovie{
		ID:               conclaveTmdb,
		Title:            "Conclave",
		OriginalTitle:    "Conclave",
		OriginalLanguage: "en",
		Overview:         "Cardinals elect a pope.",
		ReleaseDate:      ptr(startOfTest.AddDate(-1, 0, 0)),
		RuntimeMinutes:   120,
		ReleaseDates:     dates,
		Images:           catalog.TmdbImages{Poster: posterPath, Backdrop: backdropPath},
	}
}

func releaseOn(region string, kind catalog.TmdbReleaseType, date time.Time) catalog.TmdbReleaseDate {
	return catalog.TmdbReleaseDate{Region: region, Type: kind, Date: date}
}

func approved(profile domain.ProfileID) requestsapi.RequestApproved {
	return requestsapi.RequestApproved{
		Request: domain.NewID[domain.RequestID](),
		User:    domain.NewID[domain.UserID](),
		Kind:    domain.MediaKindMovie,
		Tmdb:    conclaveTmdb,
		Scope:   requestsapi.ScopeMovie,
		Profile: profile,
	}
}

func addMovie(t *testing.T, h *harness, movie catalog.TmdbMovie) api.Title {
	t.Helper()
	h.tmdb.PutMovie(movie)
	h.deliver(approved(profileP))
	h.runJobs()
	title, err := h.repo.TitleByTmdb(context.Background(), domain.MediaKindMovie, movie.ID)
	if err != nil {
		t.Fatal(err)
	}
	return title
}

// Seed catalog #1.
func TestRequestApprovedAddsAMonitoredMovie(t *testing.T) {
	h := newHarness(t)
	h.tmdb.PutMovie(conclave(releaseOn(catalog.DefaultReleaseRegion, catalog.TmdbReleaseDigital, startOfTest.Add(-day))))
	req := approved(profileP)

	h.deliver(req)
	h.runJobs()

	title, err := h.repo.TitleByTmdb(context.Background(), domain.MediaKindMovie, conclaveTmdb)
	if err != nil {
		t.Fatal(err)
	}
	if title.Profile != profileP || !title.Monitored || title.Origin != domain.OriginRequest || !title.Released {
		t.Errorf("title = %+v", title)
	}
	if title.RequestedBy == nil || *title.RequestedBy != req.User {
		t.Errorf("requestedBy = %v", title.RequestedBy)
	}
	added := events[api.TitleAdded](h)
	if len(added) != 1 {
		t.Fatalf("TitleAdded published %d times", len(added))
	}
	got := added[0]
	if got.Title != title.ID || got.RequestID == nil || *got.RequestID != req.Request || got.Origin != domain.OriginRequest || !got.Released || got.Profile != profileP {
		t.Errorf("TitleAdded = %+v", got)
	}
	for _, kind := range []string{api.ImageKindPoster, api.ImageKindBackdrop} {
		if _, err := h.repo.ImagePath(context.Background(), title.ID, kind); err != nil {
			t.Errorf("%s not cached: %v", kind, err)
		}
	}
}

// Seed catalog #2.
func TestRequestApprovedDeliveredTwiceAddsOneTitle(t *testing.T) {
	h := newHarness(t)
	h.tmdb.PutMovie(conclave())
	req := approved(profileP)

	h.deliver(req)
	h.deliver(req)
	h.runJobs()
	h.deliver(req)
	h.runJobs()

	titles, err := h.repo.Titles(context.Background(), api.TitleFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 {
		t.Errorf("titles = %d, want 1", len(titles))
	}
	if added := events[api.TitleAdded](h); len(added) != 1 {
		t.Errorf("TitleAdded published %d times", len(added))
	}
	if changed := events[api.MonitoringChanged](h); len(changed) != 0 {
		t.Errorf("MonitoringChanged = %+v for a redelivered request", changed)
	}
}

func TestRequestApprovedForATrackedMovieMonitorsItAndKeepsItsProfile(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	if _, err := h.service.UpdateTitle(context.Background(), title.ID, catalog.TitleUpdate{Monitored: ptr(false)}); err != nil {
		t.Fatal(err)
	}

	second := approved(profileQ)
	h.deliver(second)
	h.runJobs()

	got, _ := h.repo.Title(context.Background(), title.ID)
	if !got.Monitored || got.Profile != profileP {
		t.Errorf("title = %+v, want monitored with profile P", got)
	}
	changed := events[api.MonitoringChanged](h)
	last := changed[len(changed)-1]
	if last.RequestID == nil || *last.RequestID != second.Request || last.Movie == nil || !*last.Movie {
		t.Errorf("MonitoringChanged = %+v", last)
	}
	if added := events[api.TitleAdded](h); len(added) != 1 {
		t.Errorf("TitleAdded published %d times", len(added))
	}
}

// Seed catalog #4.
func TestRequestApprovedForAMovieTmdbDoesNotKnowPublishesTitleAddFailed(t *testing.T) {
	h := newHarness(t)
	req := approved(profileP)

	h.deliver(req)
	h.runJobs()

	if titles, _ := h.repo.Titles(context.Background(), api.TitleFilter{}); len(titles) != 0 {
		t.Errorf("titles = %+v", titles)
	}
	failed := events[api.TitleAddFailed](h)
	if len(failed) != 1 || failed[0].Reason != api.TitleAddFailedTmdbNotFound || failed[0].RequestID == nil || *failed[0].RequestID != req.Request {
		t.Errorf("TitleAddFailed = %+v", failed)
	}
	if calls := h.tmdb.Called(fakes.TMDBMovie); calls != 1 {
		t.Errorf("a 404 was retried: %d calls", calls)
	}
}

func TestTmdbOutageRetriesThreeTimesThenPublishesTitleAddFailed(t *testing.T) {
	h := newHarness(t)
	h.tmdb.FailAlways(fakes.TMDBMovie, catalog.ErrTmdbUnavailable)

	h.deliver(approved(profileP))
	h.runJobs()

	if calls := h.tmdb.Called(fakes.TMDBMovie); calls != 4 {
		t.Errorf("TMDB calls = %d, want 1 + 3 retries", calls)
	}
	if !slices.Equal(h.waits, catalog.AddTitleRetries) {
		t.Errorf("waits = %v, want %v", h.waits, catalog.AddTitleRetries)
	}
	failed := events[api.TitleAddFailed](h)
	if len(failed) != 1 || failed[0].Reason != api.TitleAddFailedTmdbUnavailable {
		t.Errorf("TitleAddFailed = %+v", failed)
	}
}

func TestTmdbOutageThatRecoversAddsTheTitle(t *testing.T) {
	h := newHarness(t)
	h.tmdb.PutMovie(conclave())
	h.tmdb.FailNext(fakes.TMDBMovie, catalog.ErrTmdbUnavailable, catalog.ErrTmdbUnavailable)

	h.deliver(approved(profileP))
	h.runJobs()

	if added := events[api.TitleAdded](h); len(added) != 1 {
		t.Errorf("TitleAdded published %d times", len(added))
	}
	if failed := events[api.TitleAddFailed](h); len(failed) != 0 {
		t.Errorf("TitleAddFailed = %+v", failed)
	}
}

// Seed catalog #7.
func TestReleaseClockReleasesAMovieWhoseOnlyDigitalDateIsInAnotherRegion(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave(releaseOn("GB", catalog.TmdbReleaseDigital, startOfTest.Add(5*day))))
	if title.Released {
		t.Fatal("released before its digital date")
	}

	h.clock.Advance(6 * day)
	h.runTask(catalog.ReleaseClockTask{})
	h.runTask(catalog.ReleaseClockTask{})

	got, _ := h.repo.Title(context.Background(), title.ID)
	if !got.Released || got.ReleasedAt == nil || !got.ReleasedAt.Equal(h.clock.Now()) {
		t.Errorf("title = %+v", got)
	}
	released := events[api.WantedReleased](h)
	if len(released) != 1 || released[0].Title != title.ID {
		t.Errorf("WantedReleased = %+v, want one for the title", released)
	}
}

// Seed catalog #8.
func TestReleaseClockUsesTheatricalPlusNinetyDays(t *testing.T) {
	h := newHarness(t)
	theatrical := startOfTest.Add(-day)
	title := addMovie(t, h, conclave(releaseOn(catalog.DefaultReleaseRegion, catalog.TmdbReleaseTheatrical, theatrical)))

	h.clock.Set(theatrical.Add(89 * day))
	h.runTask(catalog.ReleaseClockTask{})
	if got, _ := h.repo.Title(context.Background(), title.ID); got.Released {
		t.Fatal("released 89 days after the theatrical date")
	}

	h.clock.Set(theatrical.Add(90 * day))
	h.runTask(catalog.ReleaseClockTask{})
	h.runTask(catalog.ReleaseClockTask{})
	if got, _ := h.repo.Title(context.Background(), title.ID); !got.Released {
		t.Error("not released 90 days after the theatrical date")
	}
	if released := events[api.WantedReleased](h); len(released) != 1 {
		t.Errorf("WantedReleased published %d times", len(released))
	}
}

func TestReleaseClockNeverReleasesAMovieWithoutDates(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())

	h.clock.Advance(10 * 365 * day)
	h.runTask(catalog.ReleaseClockTask{})

	if got, _ := h.repo.Title(context.Background(), title.ID); got.Released {
		t.Error("a movie with no dates was released")
	}
	if released := events[api.WantedReleased](h); len(released) != 0 {
		t.Errorf("WantedReleased = %+v", released)
	}
}

func TestReleaseClockMarksUnmonitoredMoviesWithoutAnEvent(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave(releaseOn(catalog.DefaultReleaseRegion, catalog.TmdbReleasePhysical, startOfTest.Add(day))))
	if _, err := h.service.UpdateTitle(context.Background(), title.ID, catalog.TitleUpdate{Monitored: ptr(false)}); err != nil {
		t.Fatal(err)
	}

	h.clock.Advance(2 * day)
	h.runTask(catalog.ReleaseClockTask{})

	if got, _ := h.repo.Title(context.Background(), title.ID); !got.Released {
		t.Error("not released")
	}
	if released := events[api.WantedReleased](h); len(released) != 0 {
		t.Errorf("WantedReleased = %+v for an unmonitored movie", released)
	}
}

// Seed catalog #14.
func TestRefreshRefetchesImagesOlderThanFiveMonths(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	h.ageImage(title.ID, api.ImageKindPoster, h.clock.Now().AddDate(0, -5, -1))
	h.ageImage(title.ID, api.ImageKindBackdrop, h.clock.Now().AddDate(0, -4, 0))
	downloads := h.tmdb.Called(fakes.TMDBImage)

	h.runTask(catalog.MovieRefreshTask{})

	if got := h.tmdb.Called(fakes.TMDBImage) - downloads; got != 1 {
		t.Errorf("image downloads = %d, want only the stale poster", got)
	}
	if img := h.image(title.ID, api.ImageKindPoster); !img.FetchedAt.Equal(h.clock.Now()) {
		t.Errorf("poster fetchedAt = %v, want %v", img.FetchedAt, h.clock.Now())
	}
	if calls := h.tmdb.Called(fakes.TMDBMovie); calls != 1 {
		t.Errorf("an unchanged movie refreshed in full before 30 days: %d Movie calls", calls)
	}
}

func TestRefreshUpdatesMoviesTmdbChangedAndThoseNotRefreshedFor30Days(t *testing.T) {
	h := newHarness(t)
	movie := conclave()
	title := addMovie(t, h, movie)

	movie.Overview = "A new overview."
	h.tmdb.PutMovie(movie)
	h.clock.Advance(31 * day)
	h.runTask(catalog.MovieRefreshTask{})
	if view, _ := h.service.Title(context.Background(), title.ID); view.Overview != movie.Overview {
		t.Errorf("overview = %q after a 30-day refresh", view.Overview)
	}

	movie.Overview = "Changed on TMDB."
	h.tmdb.PutMovie(movie)
	h.tmdb.MarkChanged(domain.MediaKindMovie, conclaveTmdb, h.clock.Now())
	h.clock.Advance(day)
	h.runTask(catalog.MovieRefreshTask{})
	if view, _ := h.service.Title(context.Background(), title.ID); view.Overview != movie.Overview {
		t.Errorf("overview = %q after a TMDB change", view.Overview)
	}
}

// Seed catalog #15. The connection status itself is the platform health probe's job.
func TestUnauthorizedTmdbPausesRefreshAndReadersKeepServing(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	h.tmdb.FailAlways(fakes.TMDBChanges, catalog.ErrTmdbUnauthorized)

	h.runTask(catalog.MovieRefreshTask{})

	schedule, err := catalog.NewMovieRefreshPeriodic(h.service).Schedule(context.Background())
	if err != nil || schedule != nil {
		t.Errorf("refresh schedule = %v, %v; want paused", schedule, err)
	}
	if _, err := h.repo.Title(context.Background(), title.ID); err != nil {
		t.Errorf("Reader.Title while TMDB is unauthorized: %v", err)
	}

	h.deliverHealth(platformapi.ServiceHealthChanged{Service: platformapi.ServiceTMDB, Healthy: true})
	if schedule, _ := catalog.NewMovieRefreshPeriodic(h.service).Schedule(context.Background()); schedule == nil {
		t.Error("refresh still paused after TMDB is healthy again")
	}
}

func TestOtherServicesBecomingHealthyLeaveRefreshPaused(t *testing.T) {
	h := newHarness(t)
	if err := catalog.RefreshPausedSetting.Set(context.Background(), h.store, true); err != nil {
		t.Fatal(err)
	}
	h.deliverHealth(platformapi.ServiceHealthChanged{Service: platformapi.ServiceJellyfin, Healthy: true})
	if paused, _ := h.service.RefreshPaused(context.Background()); !paused {
		t.Error("a Jellyfin health event resumed TMDB refresh")
	}
}

func TestRemovingATitleDeletesItsImageFiles(t *testing.T) {
	h := newHarness(t)
	title := addMovie(t, h, conclave())
	poster := h.image(title.ID, api.ImageKindPoster).LocalPath

	if err := h.service.RemoveTitle(context.Background(), title.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(poster); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("poster file still there: %v", err)
	}
}
