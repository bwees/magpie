package catalog_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"magpie/internal/catalog"
	"magpie/internal/catalog/api"
	"magpie/internal/catalog/fakes"
	"magpie/internal/domain"
	"magpie/internal/platform/jobs"
	requestsapi "magpie/internal/requests/api"
)

const (
	severanceTmdb   domain.TmdbID = 95396
	specials                      = 0
	week                          = 7 * day
	episodesPerSeas               = 3
	tmdbIDsPerSeas                = 100
	splitGroupID                  = "split-in-two"
)

// tmdbSeason builds a season whose episodes air weekly from firstAir. Episode n of season s has TMDB ID s*100+n.
func tmdbSeason(number, count int, firstAir time.Time) catalog.TmdbSeason {
	season := catalog.TmdbSeason{Series: severanceTmdb, Number: number}
	for n := 1; n <= count; n++ {
		season.Episodes = append(season.Episodes, catalog.TmdbEpisode{
			ID:      catalog.TmdbEpisodeID(number*tmdbIDsPerSeas + n),
			Season:  number,
			Number:  n,
			Name:    "Episode",
			AirDate: ptr(firstAir.Add(time.Duration(n-1) * week)),
		})
	}
	return season
}

func severanceSeries(seasons ...catalog.TmdbSeason) catalog.TmdbSeries {
	series := catalog.TmdbSeries{
		ID:                    severanceTmdb,
		Name:                  "Severance",
		OriginalName:          "Severance",
		OriginalLanguage:      "en",
		Overview:              "Work and life, severed.",
		FirstAirDate:          ptr(startOfTest.AddDate(-3, 0, 0)),
		EpisodeRuntimeMinutes: 55,
		Images:                catalog.TmdbImages{Poster: posterPath},
	}
	for _, s := range seasons {
		series.Seasons = append(series.Seasons, catalog.TmdbSeasonSummary{Number: s.Number, EpisodeCount: len(s.Episodes)})
	}
	return series
}

// putSeries stores the series and its seasons in the fake TMDB.
func putSeries(h *harness, series catalog.TmdbSeries, seasons ...catalog.TmdbSeason) {
	h.tmdb.PutSeries(series)
	for _, s := range seasons {
		h.tmdb.PutSeason(s)
	}
}

// airedSeasons are specials and two seasons that have aired, plus nothing announced beyond.
func airedSeasons() []catalog.TmdbSeason {
	return []catalog.TmdbSeason{
		tmdbSeason(specials, 1, startOfTest.AddDate(-2, 0, 0)),
		tmdbSeason(1, episodesPerSeas, startOfTest.AddDate(-2, 0, 0)),
		tmdbSeason(2, episodesPerSeas, startOfTest.AddDate(-1, 0, 0)),
	}
}

func seriesRequest(profile domain.ProfileID, scope string, seasons ...int) requestsapi.RequestApproved {
	return requestsapi.RequestApproved{
		Request: domain.NewID[domain.RequestID](),
		User:    domain.NewID[domain.UserID](),
		Kind:    domain.MediaKindSeries,
		Tmdb:    severanceTmdb,
		Scope:   scope,
		Seasons: seasons,
		Profile: profile,
	}
}

func addSeries(t *testing.T, h *harness, req requestsapi.RequestApproved, seasons ...catalog.TmdbSeason) api.Title {
	t.Helper()
	putSeries(h, severanceSeries(seasons...), seasons...)
	h.deliver(req)
	h.runJobs()
	title, err := h.repo.TitleByTmdb(context.Background(), domain.MediaKindSeries, severanceTmdb)
	if err != nil {
		t.Fatal(err)
	}
	return title
}

func (h *harness) episodes(title domain.TitleID) []api.Episode {
	h.t.Helper()
	episodes, err := h.repo.Episodes(context.Background(), title)
	if err != nil {
		h.t.Fatal(err)
	}
	return episodes
}

func (h *harness) view(title domain.TitleID) catalog.TitleView {
	h.t.Helper()
	view, err := h.service.Title(context.Background(), title)
	if err != nil {
		h.t.Fatal(err)
	}
	return view
}

func seasonMonitoring(view catalog.TitleView) map[int]bool {
	flags := map[int]bool{}
	for _, s := range view.Seasons {
		flags[s.Number] = s.Monitored
	}
	return flags
}

func episodeIDs(episodes []api.Episode, keep func(api.Episode) bool) []domain.EpisodeID {
	ids := []domain.EpisodeID{}
	for _, e := range episodes {
		if keep(e) {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

func inSeason(season int) func(api.Episode) bool {
	return func(e api.Episode) bool { return e.Season == season }
}

func monitored(e api.Episode) bool { return e.Monitored }

func sortedIDs(ids []domain.EpisodeID) []domain.EpisodeID {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	return sorted
}

func TestWholeSeriesRequestMonitorsEverySeasonButSpecials(t *testing.T) {
	h := newHarness(t)
	req := seriesRequest(profileP, requestsapi.ScopeAllSeasons)

	title := addSeries(t, h, req, airedSeasons()...)

	if !title.Monitored || !title.MonitorFutureSeasons || title.Profile != profileP || !title.Released {
		t.Errorf("title = %+v", title)
	}
	if title.SeriesStatus != api.SeriesStatusContinuing || title.RuntimeMinutes != 55 {
		t.Errorf("status = %q, runtime = %d", title.SeriesStatus, title.RuntimeMinutes)
	}
	if got := seasonMonitoring(h.view(title.ID)); !maps.Equal(got, map[int]bool{specials: false, 1: true, 2: true}) {
		t.Errorf("seasons = %v", got)
	}
	episodes := h.episodes(title.ID)
	if len(episodes) != 1+2*episodesPerSeas {
		t.Fatalf("episodes = %d", len(episodes))
	}
	wantMonitored := episodeIDs(episodes, func(e api.Episode) bool { return e.Season != specials })
	added := events[api.TitleAdded](h)
	if len(added) != 1 || added[0].Kind != domain.MediaKindSeries || !added[0].Released || *added[0].RequestID != req.Request {
		t.Fatalf("TitleAdded = %+v", added)
	}
	if !slices.Equal(added[0].Monitored, wantMonitored) {
		t.Errorf("TitleAdded.Monitored = %v, want %v", added[0].Monitored, wantMonitored)
	}
	for _, e := range episodes {
		if !e.Released {
			t.Errorf("aired episode S%dE%d not released", e.Season, e.Number)
		}
	}
}

func TestSeasonRequestMonitorsOnlyTheChosenSeasons(t *testing.T) {
	h := newHarness(t)

	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeSeasons, 2), airedSeasons()...)

	if title.MonitorFutureSeasons {
		t.Error("a season request monitors future seasons")
	}
	if got := seasonMonitoring(h.view(title.ID)); !maps.Equal(got, map[int]bool{specials: false, 1: false, 2: true}) {
		t.Errorf("seasons = %v", got)
	}
	episodes := h.episodes(title.ID)
	if got, want := episodeIDs(episodes, monitored), episodeIDs(episodes, inSeason(2)); !slices.Equal(got, want) {
		t.Errorf("monitored = %v, want season 2 %v", got, want)
	}
}

// Seed catalog #3.
func TestSeasonRequestForATrackedSeriesMonitorsThoseSeasonsAndKeepsTheProfile(t *testing.T) {
	h := newHarness(t)
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeSeasons, 1), airedSeasons()...)

	second := seriesRequest(profileQ, requestsapi.ScopeSeasons, 2)
	h.deliver(second)
	h.runJobs()

	got, _ := h.repo.Title(context.Background(), title.ID)
	if got.Profile != profileP {
		t.Errorf("profile = %q, want P", got.Profile)
	}
	episodes := h.episodes(title.ID)
	for _, e := range episodes {
		if want := e.Season != specials; e.Monitored != want {
			t.Errorf("S%dE%d monitored = %v", e.Season, e.Number, e.Monitored)
		}
	}
	changed := events[api.MonitoringChanged](h)
	if len(changed) != 1 {
		t.Fatalf("MonitoringChanged = %+v", changed)
	}
	if !slices.Equal(changed[0].Added, episodeIDs(episodes, inSeason(2))) || len(changed[0].Removed) != 0 || changed[0].Movie != nil {
		t.Errorf("MonitoringChanged = %+v", changed[0])
	}
	if changed[0].RequestID == nil || *changed[0].RequestID != second.Request {
		t.Errorf("RequestID = %v", changed[0].RequestID)
	}
	if added := events[api.TitleAdded](h); len(added) != 1 {
		t.Errorf("TitleAdded published %d times", len(added))
	}
}

func TestSeriesRequestDeliveredTwiceAddsOneSeries(t *testing.T) {
	h := newHarness(t)
	req := seriesRequest(profileP, requestsapi.ScopeAllSeasons)
	putSeries(h, severanceSeries(airedSeasons()...), airedSeasons()...)

	h.deliver(req)
	h.deliver(req)
	h.runJobs()
	h.deliver(req)

	if added := events[api.TitleAdded](h); len(added) != 1 {
		t.Errorf("TitleAdded published %d times", len(added))
	}
	if changed := events[api.MonitoringChanged](h); len(changed) != 0 {
		t.Errorf("MonitoringChanged = %+v for a redelivered request", changed)
	}
}

func TestSeriesTmdbDoesNotKnowPublishesTitleAddFailed(t *testing.T) {
	h := newHarness(t)

	h.deliver(seriesRequest(profileP, requestsapi.ScopeAllSeasons))
	h.runJobs()

	failed := events[api.TitleAddFailed](h)
	if len(failed) != 1 || failed[0].Reason != api.TitleAddFailedTmdbNotFound || failed[0].Kind != domain.MediaKindSeries {
		t.Errorf("TitleAddFailed = %+v", failed)
	}
}

// Seed catalog #6.
func TestRefreshMonitorsANewSeasonWhenFutureSeasonsAreMonitored(t *testing.T) {
	h := newHarness(t)
	seasons := airedSeasons()
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), seasons...)

	third := tmdbSeason(3, episodesPerSeas, startOfTest.Add(week))
	seasons = append(seasons, third)
	putSeries(h, severanceSeries(seasons...), seasons...)
	h.clock.Advance(day)
	h.tmdb.MarkChanged(domain.MediaKindSeries, severanceTmdb, h.clock.Now())
	h.runTask(catalog.SeriesRefreshTask{Status: api.SeriesStatusContinuing})

	if got := seasonMonitoring(h.view(title.ID)); !got[3] {
		t.Errorf("season 3 not monitored: %v", got)
	}
	episodes := h.episodes(title.ID)
	newOnes := episodeIDs(episodes, inSeason(3))
	if len(newOnes) != episodesPerSeas || !slices.Equal(episodeIDs(episodes, func(e api.Episode) bool { return e.Season == 3 && e.Monitored }), newOnes) {
		t.Errorf("season 3 episodes = %+v", episodes)
	}
	announced := events[api.EpisodesAnnounced](h)
	if len(announced) != 1 || announced[0].Title != title.ID {
		t.Fatalf("EpisodesAnnounced = %+v", announced)
	}
	var refs []domain.EpisodeID
	for _, ref := range announced[0].Episodes {
		if ref.Season != 3 {
			t.Errorf("announced %+v", ref)
		}
		refs = append(refs, ref.Episode)
	}
	if !slices.Equal(refs, newOnes) {
		t.Errorf("announced %v, want %v", refs, newOnes)
	}
}

func TestRefreshLeavesANewSeasonUnmonitoredForASeasonRequest(t *testing.T) {
	h := newHarness(t)
	seasons := airedSeasons()
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeSeasons, 2), seasons...)

	seasons = append(seasons, tmdbSeason(3, episodesPerSeas, startOfTest.Add(week)))
	putSeries(h, severanceSeries(seasons...), seasons...)
	if err := h.service.RefreshTitle(context.Background(), title.ID); err != nil {
		t.Fatal(err)
	}

	if got := seasonMonitoring(h.view(title.ID)); got[3] {
		t.Errorf("season 3 monitored: %v", got)
	}
}

func TestRefreshAddsEpisodesThatInheritTheirSeasonsMonitoring(t *testing.T) {
	h := newHarness(t)
	seasons := airedSeasons()
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeSeasons, 2), seasons...)

	seasons[1] = tmdbSeason(1, episodesPerSeas+1, startOfTest.AddDate(-2, 0, 0))
	seasons[2] = tmdbSeason(2, episodesPerSeas+1, startOfTest.AddDate(-1, 0, 0))
	putSeries(h, severanceSeries(seasons...), seasons...)
	if err := h.service.RefreshTitle(context.Background(), title.ID); err != nil {
		t.Fatal(err)
	}

	s1, _ := h.repo.EpisodeByNumber(context.Background(), title.ID, 1, episodesPerSeas+1)
	s2, _ := h.repo.EpisodeByNumber(context.Background(), title.ID, 2, episodesPerSeas+1)
	if s1.Monitored || !s2.Monitored {
		t.Errorf("new S1 episode monitored = %v, new S2 episode monitored = %v", s1.Monitored, s2.Monitored)
	}
}

func TestSeriesRefreshRunsContinuingDailyAndEndedWeekly(t *testing.T) {
	h := newHarness(t)
	addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), airedSeasons()...)
	before := h.tmdb.Called(fakes.TMDBSeries)
	h.clock.Advance(day)
	h.tmdb.MarkChanged(domain.MediaKindSeries, severanceTmdb, h.clock.Now())

	h.runTask(catalog.SeriesRefreshTask{Status: api.SeriesStatusEnded})
	if got := h.tmdb.Called(fakes.TMDBSeries) - before; got != 0 {
		t.Errorf("the ended-series run refreshed a continuing series: %d calls", got)
	}
	h.runTask(catalog.SeriesRefreshTask{Status: api.SeriesStatusContinuing})
	if got := h.tmdb.Called(fakes.TMDBSeries) - before; got != 1 {
		t.Errorf("the continuing-series run made %d Series calls, want 1", got)
	}

	for status, every := range map[string]time.Duration{api.SeriesStatusContinuing: day, api.SeriesStatusEnded: week} {
		next, err := nextRun(catalog.NewSeriesRefreshPeriodic(h.service, status))
		if err != nil {
			t.Fatal(err)
		}
		if want := startOfTest.Add(every); !next.Equal(want) {
			t.Errorf("%s next run = %v, want %v", status, next, want)
		}
	}
}

// Seed catalog #9.
func TestReleaseClockReleasesAnEpisodeOnItsAirDateOnce(t *testing.T) {
	h := newHarness(t)
	seasons := []catalog.TmdbSeason{tmdbSeason(1, 2, startOfTest.Add(-week+day))}
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), seasons...)
	second, _ := h.repo.EpisodeByNumber(context.Background(), title.ID, 1, 2)
	if second.Released {
		t.Fatal("released before its air date")
	}

	h.clock.Advance(day)
	h.runTask(catalog.ReleaseClockTask{})
	h.runTask(catalog.ReleaseClockTask{})

	if got, _ := h.repo.Episode(context.Background(), second.ID); !got.Released {
		t.Error("not released on its air date")
	}
	released := events[api.WantedReleased](h)
	if len(released) != 1 || released[0].Title != title.ID || !slices.Equal(released[0].Episodes, []domain.EpisodeID{second.ID}) {
		t.Errorf("WantedReleased = %+v, want one for %s", released, second.ID)
	}
}

func TestRefreshUnreleasesAnEpisodeWhoseAirDateMovesIntoTheFuture(t *testing.T) {
	h := newHarness(t)
	seasons := []catalog.TmdbSeason{tmdbSeason(1, 1, startOfTest)}
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), seasons...)
	episode, _ := h.repo.EpisodeByNumber(context.Background(), title.ID, 1, 1)
	if !episode.Released {
		t.Fatal("not released on its air date")
	}

	seasons = []catalog.TmdbSeason{tmdbSeason(1, 1, startOfTest.Add(week))}
	putSeries(h, severanceSeries(seasons...), seasons...)
	h.clock.Advance(day)
	h.tmdb.MarkChanged(domain.MediaKindSeries, severanceTmdb, h.clock.Now())
	h.runTask(catalog.SeriesRefreshTask{Status: api.SeriesStatusContinuing})

	if got, _ := h.repo.Episode(context.Background(), episode.ID); got.Released {
		t.Error("still released after its air date moved a week out")
	}
	unreleased := events[api.EpisodesUnreleased](h)
	if len(unreleased) != 1 || !slices.Equal(unreleased[0].Episodes, []domain.EpisodeID{episode.ID}) {
		t.Errorf("EpisodesUnreleased = %+v, want one for %s", unreleased, episode.ID)
	}
	h.clock.Advance(week - day)
	h.runTask(catalog.ReleaseClockTask{})
	if got, _ := h.repo.Episode(context.Background(), episode.ID); !got.Released {
		t.Error("not released on its new air date")
	}
	released := events[api.WantedReleased](h)
	if len(released) != 1 || !slices.Equal(released[0].Episodes, []domain.EpisodeID{episode.ID}) {
		t.Errorf("WantedReleased = %+v, want one for %s on the new date", released, episode.ID)
	}
}

func TestReleaseClockLeavesUnmonitoredEpisodesOutOfWantedReleased(t *testing.T) {
	h := newHarness(t)
	seasons := []catalog.TmdbSeason{
		tmdbSeason(1, 1, startOfTest.Add(day)),
		tmdbSeason(2, 1, startOfTest.Add(day)),
	}
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeSeasons, 2), seasons...)

	h.clock.Advance(day)
	h.runTask(catalog.ReleaseClockTask{})

	episodes := h.episodes(title.ID)
	released := events[api.WantedReleased](h)
	if len(released) != 1 || !slices.Equal(released[0].Episodes, episodeIDs(episodes, inSeason(2))) {
		t.Errorf("WantedReleased = %+v", released)
	}
	for _, e := range episodes {
		if !e.Released {
			t.Errorf("S%dE%d not marked released", e.Season, e.Number)
		}
	}
	if got, _ := h.repo.Title(context.Background(), title.ID); !got.Released {
		t.Error("series not released once an episode is")
	}
}

func TestAnUnmonitoredSeriesWantsNothing(t *testing.T) {
	h := newHarness(t)
	seasons := []catalog.TmdbSeason{tmdbSeason(1, 2, startOfTest.Add(-week+day))}
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), seasons...)
	episodes := h.episodes(title.ID)

	if _, err := h.service.UpdateTitle(context.Background(), title.ID, catalog.TitleUpdate{Monitored: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(day)
	h.runTask(catalog.ReleaseClockTask{})

	if wanted, ok, _ := h.repo.Wanted(context.Background(), title.ID); ok || len(wanted) != 0 {
		t.Errorf("Wanted = %v, %v for an unmonitored series", wanted, ok)
	}
	if released := events[api.WantedReleased](h); len(released) != 0 {
		t.Errorf("WantedReleased = %+v", released)
	}
	changed := events[api.MonitoringChanged](h)
	if len(changed) != 1 || !slices.Equal(changed[0].Removed, episodeIDs(episodes, monitored)) {
		t.Errorf("MonitoringChanged = %+v, want every monitored episode removed", changed)
	}
}

func TestSeasonMonitoringSetsItsEpisodes(t *testing.T) {
	h := newHarness(t)
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), airedSeasons()...)

	view, err := h.service.SetSeasonMonitored(context.Background(), title.ID, 2, false)
	if err != nil {
		t.Fatal(err)
	}

	if seasonMonitoring(view)[2] {
		t.Error("season 2 still monitored")
	}
	episodes := h.episodes(title.ID)
	changed := events[api.MonitoringChanged](h)
	if len(changed) != 1 || !slices.Equal(changed[0].Removed, episodeIDs(episodes, inSeason(2))) || len(changed[0].Added) != 0 {
		t.Errorf("MonitoringChanged = %+v", changed)
	}
	if _, err := h.service.SetSeasonMonitored(context.Background(), title.ID, 9, true); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("unknown season err = %v", err)
	}
}

func TestEpisodeMonitoringPublishesMonitoringChanged(t *testing.T) {
	h := newHarness(t)
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), airedSeasons()...)
	special, _ := h.repo.EpisodeByNumber(context.Background(), title.ID, specials, 1)

	if _, err := h.service.SetEpisodeMonitored(context.Background(), title.ID, special.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.SetEpisodeMonitored(context.Background(), title.ID, special.ID, true); err != nil {
		t.Fatal(err)
	}

	changed := events[api.MonitoringChanged](h)
	if len(changed) != 1 || !slices.Equal(changed[0].Added, []domain.EpisodeID{special.ID}) {
		t.Errorf("MonitoringChanged = %+v", changed)
	}
	if _, err := h.service.SetEpisodeMonitored(context.Background(), title.ID, domain.NewID[domain.EpisodeID](), false); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("unknown episode err = %v", err)
	}
	movie := addMovie(t, h, conclave())
	if _, err := h.service.SetEpisodeMonitored(context.Background(), movie.ID, special.ID, false); !errors.Is(err, catalog.ErrNotASeries) {
		t.Errorf("movie err = %v, want ErrNotASeries", err)
	}
}

// splitGroup places season 1's first five episodes in group 1 and the rest in group 2.
func splitGroup(placed int) catalog.TmdbEpisodeGroup {
	group := catalog.TmdbEpisodeGroup{ID: splitGroupID, Name: "Two parts", Type: catalog.TmdbGroupDVD}
	half := placed / 2
	for order := 1; order <= 2; order++ {
		season := catalog.TmdbEpisodeGroupSeason{Name: "Part", Order: order}
		for i := range half {
			id := catalog.TmdbEpisodeID(tmdbIDsPerSeas + (order-1)*half + i + 1)
			season.Episodes = append(season.Episodes, catalog.TmdbEpisodeGroupEntry{Episode: id, Order: i})
		}
		group.Groups = append(group.Groups, season)
	}
	return group
}

// Seed catalog #10.
func TestEpisodeGroupRenumbersEpisodesAndKeepsTheirIDs(t *testing.T) {
	h := newHarness(t)
	const count = 10
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), tmdbSeason(1, count, startOfTest.AddDate(-1, 0, 0)))
	before := h.episodes(title.ID)
	oldSixth := before[5]
	h.tmdb.PutEpisodeGroup(severanceTmdb, splitGroup(count))

	view, err := h.service.SelectEpisodeGroup(context.Background(), title.ID, splitGroupID)
	if err != nil {
		t.Fatal(err)
	}

	if view.EpisodeGroupID != splitGroupID {
		t.Errorf("group = %q", view.EpisodeGroupID)
	}
	after := h.episodes(title.ID)
	if !slices.Equal(sortedIDs(episodeIDs(after, monitored)), sortedIDs(episodeIDs(before, monitored))) {
		t.Error("episode IDs or monitoring changed")
	}
	got, err := h.repo.EpisodeByNumber(context.Background(), title.ID, 2, 1)
	if err != nil || got.ID != oldSixth.ID || got.AiredSeason != 1 || got.AiredNumber != 6 {
		t.Errorf("EpisodeByNumber(2, 1) = %+v, %v; want the old S01E06", got, err)
	}
	if flags := seasonMonitoring(view); !maps.Equal(flags, map[int]bool{1: true, 2: true}) {
		t.Errorf("seasons = %v", flags)
	}
	changed := events[api.EpisodeGroupChanged](h)
	if len(changed) != 1 || changed[0].GroupID != splitGroupID || len(changed[0].Moves) != 5 {
		t.Fatalf("EpisodeGroupChanged = %+v", changed)
	}
	if first := changed[0].Moves[0]; first != (api.EpisodeMove{Episode: oldSixth.ID, FromSeason: 1, FromNo: 6, ToSeason: 2, ToNo: 1}) {
		t.Errorf("first move = %+v", first)
	}
}

func TestAiredOrderRestoresTheNumbering(t *testing.T) {
	h := newHarness(t)
	const count = 10
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), tmdbSeason(1, count, startOfTest.AddDate(-1, 0, 0)))
	h.tmdb.PutEpisodeGroup(severanceTmdb, splitGroup(count))
	if _, err := h.service.SelectEpisodeGroup(context.Background(), title.ID, splitGroupID); err != nil {
		t.Fatal(err)
	}

	view, err := h.service.SelectEpisodeGroup(context.Background(), title.ID, "")
	if err != nil {
		t.Fatal(err)
	}

	if view.EpisodeGroupID != "" || len(view.Seasons) != 1 {
		t.Errorf("view = %+v", view.Seasons)
	}
	changed := events[api.EpisodeGroupChanged](h)
	if len(changed) != 2 || changed[1].GroupID != "" || len(changed[1].Moves) != 5 {
		t.Errorf("EpisodeGroupChanged = %+v", changed)
	}
}

func TestEpisodesTheGroupDoesNotPlaceGoToSeasonZero(t *testing.T) {
	h := newHarness(t)
	const count = 10
	seasons := []catalog.TmdbSeason{tmdbSeason(specials, 2, startOfTest.AddDate(-1, 0, 0)), tmdbSeason(1, count, startOfTest.AddDate(-1, 0, 0))}
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), seasons...)
	group := splitGroup(count - 2)
	h.tmdb.PutEpisodeGroup(severanceTmdb, group)

	if _, err := h.service.SelectEpisodeGroup(context.Background(), title.ID, splitGroupID); err != nil {
		t.Fatal(err)
	}

	for number, aired := range map[int][2]int{1: {specials, 1}, 2: {specials, 2}, 3: {1, 9}, 4: {1, 10}} {
		got, err := h.repo.EpisodeByNumber(context.Background(), title.ID, specials, number)
		if err != nil || got.AiredSeason != aired[0] || got.AiredNumber != aired[1] {
			t.Errorf("S00E%02d = %+v, %v; want aired %v", number, got, err, aired)
		}
	}
	if !seasonMonitoring(h.view(title.ID))[specials] {
		t.Error("season 0 holds monitored episodes but is unmonitored")
	}
}

func TestSelectingAGroupOfAnotherSeriesFails(t *testing.T) {
	h := newHarness(t)
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), airedSeasons()...)

	if _, err := h.service.SelectEpisodeGroup(context.Background(), title.ID, "not-listed"); !errors.Is(err, catalog.ErrUnknownEpisodeGroup) {
		t.Errorf("err = %v, want ErrUnknownEpisodeGroup", err)
	}
	movie := addMovie(t, h, conclave())
	if _, err := h.service.SelectEpisodeGroup(context.Background(), movie.ID, ""); !errors.Is(err, catalog.ErrNotASeries) {
		t.Errorf("movie err = %v, want ErrNotASeries", err)
	}
}

func TestRefreshKeepsTheEpisodeGroupForNewEpisodes(t *testing.T) {
	h := newHarness(t)
	const count = 10
	title := addSeries(t, h, seriesRequest(profileP, requestsapi.ScopeAllSeasons), tmdbSeason(1, count, startOfTest.AddDate(-1, 0, 0)))
	h.tmdb.PutEpisodeGroup(severanceTmdb, splitGroup(count))
	if _, err := h.service.SelectEpisodeGroup(context.Background(), title.ID, splitGroupID); err != nil {
		t.Fatal(err)
	}

	longer := tmdbSeason(1, count+2, startOfTest.AddDate(-1, 0, 0))
	putSeries(h, severanceSeries(longer), longer)
	h.tmdb.PutEpisodeGroup(severanceTmdb, splitGroup(count+2))
	if err := h.service.RefreshTitle(context.Background(), title.ID); err != nil {
		t.Fatal(err)
	}

	got, err := h.repo.EpisodeByNumber(context.Background(), title.ID, 2, 6)
	if err != nil || got.AiredNumber != count+2 {
		t.Errorf("S02E06 = %+v, %v; want aired E12", got, err)
	}
	changed := events[api.EpisodeGroupChanged](h)
	if len(changed) != 2 || changed[1].GroupID != splitGroupID {
		t.Fatalf("EpisodeGroupChanged = %+v; the refresh moved episodes", changed)
	}
	if announced := events[api.EpisodesAnnounced](h); len(announced) != 1 || len(announced[0].Episodes) != 2 {
		t.Errorf("EpisodesAnnounced = %+v", announced)
	}
}

// Seed catalog #20.
func TestTitleLanguageOverrideReplacesTheGlobalListForThatTitle(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	title := addMovie(t, h, conclave())
	global := []api.WantedLanguage{{Language: "en"}, {Language: "es"}}
	override := []api.WantedLanguage{{Language: "ja", Forced: true}, {Language: "en", HearingImpaired: true}}
	if _, err := h.service.SetGlobalLanguages(ctx, global); err != nil {
		t.Fatal(err)
	}

	if _, err := h.service.SetTitleLanguages(ctx, title.ID, override); err != nil {
		t.Fatal(err)
	}

	if got, _ := h.repo.WantedLanguages(ctx, title.ID); !slices.Equal(got, override) {
		t.Errorf("WantedLanguages = %+v", got)
	}
	if got, _ := h.repo.GlobalLanguages(ctx); !slices.Equal(got, global) {
		t.Errorf("GlobalLanguages = %+v", got)
	}
	changed := events[api.LanguagesChanged](h)
	if len(changed) != 2 || changed[1].Title == nil || *changed[1].Title != title.ID || !slices.Equal(changed[1].Languages, override) {
		t.Fatalf("LanguagesChanged = %+v", changed)
	}

	if _, err := h.service.SetTitleLanguages(ctx, title.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := h.repo.WantedLanguages(ctx, title.ID); !slices.Equal(got, global) {
		t.Errorf("WantedLanguages after clearing = %+v", got)
	}
	changed = events[api.LanguagesChanged](h)
	if last := changed[len(changed)-1]; *last.Title != title.ID || !slices.Equal(last.Languages, global) {
		t.Errorf("LanguagesChanged after clearing = %+v, want the global list", last)
	}
}

func TestGlobalLanguagesDefaultToEnglishAndPublishChanges(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if got, _ := h.repo.GlobalLanguages(ctx); !slices.Equal(got, []api.WantedLanguage{{Language: "en"}}) {
		t.Errorf("default = %+v", got)
	}

	langs := []api.WantedLanguage{{Language: "fr", Forced: true}, {Language: "en"}}
	if _, err := h.service.SetGlobalLanguages(ctx, langs); err != nil {
		t.Fatal(err)
	}

	changed := events[api.LanguagesChanged](h)
	if len(changed) != 1 || changed[0].Title != nil || !slices.Equal(changed[0].Languages, langs) {
		t.Errorf("LanguagesChanged = %+v", changed)
	}
	if changed[0].AggregateID() != api.GlobalLanguagesAggregate {
		t.Errorf("aggregate = %q", changed[0].AggregateID())
	}
}

func TestWantedLanguagesAreValidated(t *testing.T) {
	h := newHarness(t)
	for name, langs := range map[string][]api.WantedLanguage{
		"empty":     {},
		"unknown":   {{Language: "xx"}},
		"region":    {{Language: "en-US"}},
		"duplicate": {{Language: "en"}, {Language: "en", Forced: true}},
	} {
		if _, err := h.service.SetGlobalLanguages(context.Background(), langs); !errors.Is(err, catalog.ErrInvalidLanguages) {
			t.Errorf("%s: err = %v, want ErrInvalidLanguages", name, err)
		}
	}
	if changed := events[api.LanguagesChanged](h); len(changed) != 0 {
		t.Errorf("LanguagesChanged = %+v for invalid lists", changed)
	}
}

// nextRun is when a periodic job that last ran at the start of the test runs again.
func nextRun(p jobs.Periodic) (time.Time, error) {
	schedule, err := p.Schedule(context.Background())
	if err != nil || schedule == nil {
		return time.Time{}, errors.Join(err, errPaused)
	}
	return schedule.Next(startOfTest), nil
}

var errPaused = errors.New("periodic job paused")
