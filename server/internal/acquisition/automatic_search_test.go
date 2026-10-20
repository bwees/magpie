package acquisition_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/acquisition/fakes"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/config"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/parse"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

// Seeds #4 and #5 run at a hundredth of their real timing.
const (
	tierHighScore = 2300
	shortWindow   = 150 * time.Millisecond
	longWindow    = 5 * time.Second
	lastAnswer    = 80 * time.Millisecond
	twoHours      = 2 * time.Hour
	halfAnHour    = 30 * time.Minute
	tieredGroupID = 1000

	belowCutoffName = acceptedName
	cutoffName      = "Movie.Title.2024.2160p.WEB-DL.DDP5.1.H.265-LOW"
	bestName        = "Movie.Title.2024.2160p.WEB-DL.DDP5.1.H.265-HIGH"
	movieCategory   = 2040
	tvCategory      = 5040
	showTmdb        = domain.TmdbID(95396)
)

var tvIDCaps = acquisition.TorznabCaps{
	TVSearch:   true,
	TVParams:   []acquisition.TorznabParam{acquisition.ParamQuery, acquisition.ParamSeason, acquisition.ParamEpisode},
	Categories: []int{5000, 5040},
}

func profileQuality(id parse.QualityID) domain.Quality {
	d, _ := parse.QualityByID(id)
	return d.Quality(0)
}

// useTieredProfile gives the title a profile with WEB 2160p as the cutoff above a WEB 1080p group. Only
// the HIGH group scores, so the profile's maximum attainable CF score is tierHighScore.
func (f searchFixture) useTieredProfile(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	high, err := f.quality.CreateFormat(ctx, acquisition.CustomFormat{
		Name: "Tier High", Specs: []api.FormatSpec{{Kind: api.SpecReleaseGroup, Value: `^HIGH$`}},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := f.quality.CreateProfile(ctx, api.Profile{
		Name: "Tiered", UpgradeAllowed: true, PropersMode: api.PropersPreferAndUpgrade,
		Items: []api.ProfileItem{
			{Qualities: []domain.Quality{profileQuality(parse.QualityWEBDL2160p)}, Allowed: true},
			{
				Group:     &api.QualityGroup{ID: tieredGroupID, Name: "WEB 1080p"},
				Qualities: []domain.Quality{profileQuality(parse.QualityWEBDL1080p), profileQuality(parse.QualityWEBRip1080p)},
				Allowed:   true,
			},
		},
		FormatScores: []api.FormatScore{{Format: high.ID, Name: high.Name, Score: tierHighScore}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.title.Profile = profile.ID
	f.catalog.PutTitle(f.title)
}

func (f searchFixture) setSettleWindow(t *testing.T, window time.Duration) {
	t.Helper()
	if err := acquisition.SettleWindowKey.Set(context.Background(), f.store, settings.Duration(window)); err != nil {
		t.Fatal(err)
	}
}

func (f searchFixture) setDelay(t *testing.T, delay time.Duration) {
	t.Helper()
	if err := acquisition.DefaultDelayKey.Set(context.Background(), f.store, settings.Duration(delay)); err != nil {
		t.Fatal(err)
	}
}

func (f searchFixture) startAuto(t *testing.T, trigger api.SearchTrigger) acquisition.SearchSnapshot {
	t.Helper()
	s, err := f.searches.Start(context.Background(), acquisition.SearchRequest{Title: f.title.ID, Trigger: trigger})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// settleQueued returns the search whose automatic grab was queued, if any.
func (f searchFixture) settleQueued() (domain.SearchID, bool) {
	for _, task := range f.jobs.queued() {
		if id, ok := acquisition.SettleTaskOf(task); ok {
			return id, true
		}
	}
	return "", false
}

func (f searchFixture) waitForSettle(t *testing.T) domain.SearchID {
	t.Helper()
	deadline := time.Now().Add(waitForEvent)
	for time.Now().Before(deadline) {
		if id, ok := f.settleQueued(); ok {
			return id
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no automatic grab was queued")
	return ""
}

// settle runs the queued automatic grab, as its job would.
func (f searchFixture) settle(t *testing.T) {
	t.Helper()
	id := f.waitForSettle(t)
	f.jobs.drop(func(task backlite.Task) bool { _, ok := acquisition.SettleTaskOf(task); return ok })
	if err := f.downloads.Settle(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}

func (f searchFixture) grabbed(t *testing.T) []api.ReleaseGrabbed {
	t.Helper()
	list := []api.ReleaseGrabbed{}
	for _, ev := range f.events.ofType(api.TypeReleaseGrabbed) {
		list = append(list, payload[api.ReleaseGrabbed](t, ev))
	}
	return list
}

func (f searchFixture) searchTasks() []acquisition.SearchRequest {
	list := []acquisition.SearchRequest{}
	for _, task := range f.jobs.queued() {
		if req, ok := acquisition.SearchTaskOf(task); ok {
			list = append(list, req)
		}
	}
	return list
}

func (f searchFixture) pending(t *testing.T) []acquisition.PendingRelease {
	t.Helper()
	list, err := f.repo.PendingReleases(context.Background(), f.title.ID)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// deliver runs a handler on one event, inside a transaction as the dispatcher does.
func (f searchFixture) deliver(t *testing.T, h bus.Handler, ev bus.Event) {
	t.Helper()
	ctx := context.Background()
	if err := f.db.Write(ctx, func(tx *gorm.DB) error { return h.Handle(ctx, tx, ev) }); err != nil {
		t.Fatal(err)
	}
}

func published(name string, age time.Duration, now time.Time) acquisition.TorznabItem {
	item := release(name)
	item.PublishedAt = now.Add(-age)
	item.Categories = []int{movieCategory}
	return item
}

// Seed acquisition #4: nothing is grabbed before every indexer answered, then the best release is.
func TestAnAutomaticSearchWaitsForEveryIndexerThenGrabsTheBest(t *testing.T) {
	f := newSearchFixture(t)
	f.useTieredProfile(t)
	f.setSettleWindow(t, longWindow)
	f.indexer(t, fastID, "Fast", idCaps, fastAnswer, release(belowCutoffName))
	f.indexer(t, slowID, "Slow", idCaps, slowAnswer, release(cutoffName))
	f.indexer(t, stuckID, "Last", idCaps, lastAnswer)

	started := f.startAuto(t, api.TriggerAuto)
	f.events.waitFor(t, api.TypeSearchResultsReceived, 1)
	if _, queued := f.settleQueued(); queued {
		t.Fatal("grabbed after the first answer, before the other indexers")
	}
	f.settle(t)

	snapshot, err := f.searches.Search(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range snapshot.Indexers {
		if run.Status == acquisition.IndexerPending {
			t.Errorf("%s still pending when the grab ran", run.Indexer.Name)
		}
	}
	grabs := f.grabbed(t)
	if len(grabs) != 1 || grabs[0].Release != cutoffName || grabs[0].Trigger != api.TriggerAuto {
		t.Fatalf("grabs = %+v, want only %s", grabs, cutoffName)
	}
	done := f.completed(t)
	if done.Outcome != api.OutcomeGrabbed || done.Grabbed == nil || *done.Grabbed != grabs[0].Download {
		t.Errorf("SearchCompleted = %+v, want GRABBED with %s", done, grabs[0].Download)
	}
}

// Seed acquisition #4: once the settle window passes, the best release so far is grabbed.
func TestAnAutomaticSearchGrabsWhenTheSettleWindowPasses(t *testing.T) {
	f := newSearchFixture(t)
	f.useTieredProfile(t)
	f.setSettleWindow(t, shortWindow)
	f.indexer(t, fastID, "Fast", idCaps, fastAnswer, release(belowCutoffName))
	f.indexer(t, stuckID, "Stuck", idCaps, neverAnswers)

	started := f.startAuto(t, api.TriggerAuto)
	f.settle(t)

	if snapshot, _ := f.searches.Search(started.ID); snapshot.Completed {
		t.Error("the search completed before its stuck indexer timed out")
	}
	if grabs := f.grabbed(t); len(grabs) != 1 || grabs[0].Release != belowCutoffName {
		t.Fatalf("grabs = %+v, want %s", grabs, belowCutoffName)
	}
	if done := f.completed(t); done.Outcome != api.OutcomeGrabbed || !slices.Equal(done.FailedIndexers, []string{"Stuck"}) {
		t.Errorf("SearchCompleted = %+v, want GRABBED with Stuck failed", done)
	}
}

// Seed acquisition #5.
func TestAReleaseAtCutoffWithTheMaximumScoreIsGrabbedEarly(t *testing.T) {
	f := newSearchFixture(t)
	f.useTieredProfile(t)
	f.setSettleWindow(t, longWindow)
	f.indexer(t, fastID, "Fast", idCaps, fastAnswer, release(bestName))
	f.indexer(t, stuckID, "Stuck", idCaps, neverAnswers)

	begin := time.Now()
	f.startAuto(t, api.TriggerAuto)
	f.settle(t)

	if waited := time.Since(begin); waited >= longWindow {
		t.Errorf("grabbed after %s, want before the settle window", waited)
	}
	if grabs := f.grabbed(t); len(grabs) != 1 || grabs[0].Release != bestName || grabs[0].CFScore != tierHighScore {
		t.Errorf("grabs = %+v, want %s at %d", grabs, bestName, tierHighScore)
	}
}

func TestAUserAutomaticSearchIgnoresTheDelay(t *testing.T) {
	f := newSearchFixture(t)
	f.useTieredProfile(t)
	f.setDelay(t, twoHours)
	f.indexer(t, fastID, "Fast", idCaps, 0, published(belowCutoffName, 0, time.Now()))

	f.startAuto(t, api.TriggerUserAuto)
	f.settle(t)

	if grabs := f.grabbed(t); len(grabs) != 1 || grabs[0].Trigger != api.TriggerUserAuto {
		t.Errorf("grabs = %+v, want one USER_AUTO grab", grabs)
	}
	if done := f.completed(t); done.Outcome != api.OutcomeGrabbed {
		t.Errorf("outcome = %s, want GRABBED", done.Outcome)
	}
}

// Seed acquisition #32.
func TestSearchCompletedOutcome(t *testing.T) {
	t.Run("nothing accepted", func(t *testing.T) {
		f := newSearchFixture(t)
		f.indexer(t, fastID, "Fast", idCaps, 0, release(rejectedName))
		f.startAuto(t, api.TriggerAuto)
		f.settle(t)
		if done := f.completed(t); done.Outcome != api.OutcomeNothingAccepted || done.Grabbed != nil {
			t.Errorf("SearchCompleted = %+v, want NOTHING_ACCEPTED", done)
		}
	})
	t.Run("no releases", func(t *testing.T) {
		f := newSearchFixture(t)
		f.indexer(t, fastID, "Fast", idCaps, 0)
		f.startAuto(t, api.TriggerAuto)
		f.settle(t)
		if done := f.completed(t); done.Outcome != api.OutcomeNoReleases || done.Grabbed != nil {
			t.Errorf("SearchCompleted = %+v, want NO_RELEASES", done)
		}
	})
	t.Run("pending delay", func(t *testing.T) {
		f := newSearchFixture(t)
		f.useTieredProfile(t)
		f.setDelay(t, twoHours)
		f.indexer(t, fastID, "Fast", idCaps, 0, published(belowCutoffName, halfAnHour, time.Now()))
		f.startAuto(t, api.TriggerAuto)
		f.settle(t)
		if done := f.completed(t); done.Outcome != api.OutcomePendingDelay || done.Grabbed != nil {
			t.Errorf("SearchCompleted = %+v, want PENDING_DELAY", done)
		}
		if held := f.pending(t); len(held) != 1 || held[0].Release.Title != belowCutoffName {
			t.Errorf("pending = %+v, want the delayed release", held)
		}
		if grabs := f.grabbed(t); len(grabs) != 0 {
			t.Errorf("grabbed %+v during the delay", grabs)
		}
	})
	t.Run("grabbed", func(t *testing.T) {
		f := newSearchFixture(t)
		f.indexer(t, fastID, "Fast", idCaps, 0, release(acceptedName))
		f.startAuto(t, api.TriggerAuto)
		f.settle(t)
		done := f.completed(t)
		if grabs := f.grabbed(t); done.Outcome != api.OutcomeGrabbed || len(grabs) != 1 || *done.Grabbed != grabs[0].Download {
			t.Errorf("SearchCompleted = %+v with grabs %+v, want GRABBED with the grab", done, grabs)
		}
	})
	t.Run("interactive with accepted releases", func(t *testing.T) {
		f := newSearchFixture(t)
		f.indexer(t, fastID, "Fast", idCaps, 0, release(acceptedName))
		f.start(t)
		if done := f.completed(t); done.Outcome != api.OutcomeReleasesFound || done.Accepted != 1 {
			t.Errorf("SearchCompleted = %+v, want RELEASES_FOUND", done)
		}
	})
}

// Seed acquisition #12.
func TestTheDelayHoldsAReleaseUntilItsTimeThenGrabsTheBestPending(t *testing.T) {
	f := newSearchFixture(t)
	f.useTieredProfile(t)
	f.setDelay(t, twoHours)
	now := time.Now()
	f.indexer(t, fastID, "Fast", idCaps, 0)
	f.prowlarr.SetRSS(fastID, published(belowCutoffName, halfAnHour, now))

	if err := f.searches.SyncFeed(context.Background(), fastID); err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	if held := f.pending(t); len(held) != 1 || held[0].Trigger != api.TriggerRSS {
		t.Fatalf("pending = %+v, want the RSS release held", held)
	}
	if grabs := f.grabbed(t); len(grabs) != 0 {
		t.Fatalf("grabbed %+v during the delay", grabs)
	}

	acquisition.SetSearchClock(f.searches, func() time.Time { return now.Add(twoHours - halfAnHour - time.Minute) })
	if err := f.searches.ReviewPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, queued := f.settleQueued(); queued || len(f.pending(t)) != 1 {
		t.Fatal("the pending release was taken before its delay passed")
	}

	acquisition.SetSearchClock(f.searches, func() time.Time { return now.Add(twoHours - halfAnHour + time.Minute) })
	if err := f.searches.ReviewPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	if grabs := f.grabbed(t); len(grabs) != 1 || grabs[0].Release != belowCutoffName || grabs[0].Trigger != api.TriggerRSS {
		t.Errorf("grabs = %+v, want the pending release grabbed by RSS", grabs)
	}
	if held := f.pending(t); len(held) != 0 {
		t.Errorf("pending = %+v, want it emptied", held)
	}
	if n := len(f.events.ofType(api.TypeSearchStarted)); n != 0 {
		t.Errorf("RSS published %d SearchStarted", n)
	}
}

// Seed acquisition #12: a release meeting the cutoff skips the delay.
func TestAReleaseMeetingTheCutoffBypassesTheDelay(t *testing.T) {
	f := newSearchFixture(t)
	f.useTieredProfile(t)
	f.setDelay(t, twoHours)
	f.indexer(t, fastID, "Fast", idCaps, 0)
	f.prowlarr.SetRSS(fastID, published(cutoffName, halfAnHour, time.Now()))

	if err := f.searches.SyncFeed(context.Background(), fastID); err != nil {
		t.Fatal(err)
	}
	f.settle(t)

	if grabs := f.grabbed(t); len(grabs) != 1 || grabs[0].Release != cutoffName {
		t.Errorf("grabs = %+v, want %s at once", grabs, cutoffName)
	}
	if held := f.pending(t); len(held) != 0 {
		t.Errorf("pending = %+v, want none", held)
	}
}

func TestRSSIgnoresReleasesForTitlesNobodyWants(t *testing.T) {
	f := newSearchFixture(t)
	f.indexer(t, fastID, "Fast", idCaps, 0)
	other := published("Another.Film.2024.1080p.WEB-DL.DDP5.1.H.264-GRP", 0, time.Now())
	show := published("Movie.Title.S01E01.1080p.WEB-DL.DDP5.1.H.264-GRP", 0, time.Now())
	show.Categories = []int{tvCategory}
	f.prowlarr.SetRSS(fastID, other, show)

	if err := f.searches.SyncFeed(context.Background(), fastID); err != nil {
		t.Fatal(err)
	}
	if _, queued := f.settleQueued(); queued {
		t.Error("RSS matched a release to no wanted title")
	}
}

func (f searchFixture) addTitle(t *testing.T, origin domain.Origin) bus.Event {
	t.Helper()
	f.title.Origin = origin
	f.catalog.PutTitle(f.title)
	return bus.EventOf(catalogapi.TitleAdded{Title: f.title.ID, Kind: f.title.Kind, Released: f.title.Released, Origin: origin})
}

// Seed acquisition #14: a redelivered TitleAdded doesn't queue a second search.
func TestTitleAddedQueuesOneAutomaticSearch(t *testing.T) {
	f := newSearchFixture(t)
	handler := acquisition.NewTitleAddedHandler(f.searches)
	added := f.addTitle(t, domain.OriginRequest)

	f.deliver(t, handler, added)
	f.deliver(t, handler, added)

	tasks := f.searchTasks()
	if len(tasks) != 1 || tasks[0].Title != f.title.ID || tasks[0].Trigger != api.TriggerAuto || len(tasks[0].Episodes) != 0 {
		t.Errorf("search jobs = %+v, want one AUTO search for the movie", tasks)
	}
}

// Seed acquisition #29: a title found by a library scan is never searched on add.
func TestTitlesFromAScanOrMigrationAreNotSearchedOnAdd(t *testing.T) {
	for _, origin := range []domain.Origin{domain.OriginLibraryScan, domain.OriginMigration} {
		f := newSearchFixture(t)
		f.deliver(t, acquisition.NewTitleAddedHandler(f.searches), f.addTitle(t, origin))
		if tasks := f.searchTasks(); len(tasks) != 0 {
			t.Errorf("%s: search jobs = %+v, want none", origin, tasks)
		}
	}
}

func TestTitleAddedRespectsTheSearchOnAddSetting(t *testing.T) {
	f := newSearchFixture(t)
	if err := acquisition.SearchOnAddKey.Set(context.Background(), f.store, false); err != nil {
		t.Fatal(err)
	}
	f.deliver(t, acquisition.NewTitleAddedHandler(f.searches), f.addTitle(t, domain.OriginManual))
	if tasks := f.searchTasks(); len(tasks) != 0 {
		t.Errorf("search jobs = %+v, want none with search on add off", tasks)
	}
}

// Seed acquisition #13.
func TestAnUnreleasedMovieIsSearchedOnceItIsReleased(t *testing.T) {
	f := newSearchFixture(t)
	f.title.Released = false
	f.deliver(t, acquisition.NewTitleAddedHandler(f.searches), f.addTitle(t, domain.OriginRequest))
	if tasks := f.searchTasks(); len(tasks) != 0 {
		t.Fatalf("search jobs = %+v before the release date", tasks)
	}

	f.title.Released = true
	f.catalog.PutTitle(f.title)
	f.deliver(t, acquisition.NewWantedReleasedHandler(f.searches), bus.EventOf(catalogapi.WantedReleased{Title: f.title.ID}))

	if tasks := f.searchTasks(); len(tasks) != 1 || tasks[0].Trigger != api.TriggerAuto {
		t.Errorf("search jobs = %+v, want one AUTO search", tasks)
	}
}

func TestASearchJobLetsTheSameSearchBeQueuedAgain(t *testing.T) {
	f := newSearchFixture(t)
	handler := acquisition.NewWantedReleasedHandler(f.searches)
	f.deliver(t, handler, bus.EventOf(catalogapi.WantedReleased{Title: f.title.ID}))

	if err := f.searches.RunSearchJob(context.Background(), f.searchTasks()[0]); err != nil {
		t.Fatal(err)
	}
	f.deliver(t, handler, bus.EventOf(catalogapi.WantedReleased{Title: f.title.ID}))

	if tasks := f.searchTasks(); len(tasks) != 2 {
		t.Errorf("search jobs = %+v, want a second one after the first ran", tasks)
	}
}

type seriesFixture struct {
	searchFixture
	episodes []catalogapi.Episode
}

// newSeriesFixture tracks a series with a three-episode first season that has aired.
func newSeriesFixture(t *testing.T) seriesFixture {
	t.Helper()
	f := newSearchFixture(t)
	f.title = catalogapi.Title{
		ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindSeries, Name: "Severance", Year: 2022,
		External: domain.ExternalIDs{Tmdb: showTmdb}, RuntimeMinutes: 50, Monitored: true, Released: true,
	}
	f.catalog.PutTitle(f.title)
	aired := time.Now().AddDate(0, -1, 0)
	s := seriesFixture{searchFixture: f}
	for number := 1; number <= 3; number++ {
		ep := catalogapi.Episode{
			ID: domain.NewID[domain.EpisodeID](), Title: f.title.ID, Season: 1, Number: number,
			AiredSeason: 1, AiredNumber: number, AirDate: &aired, RuntimeMinutes: 50, Monitored: true, Released: true,
		}
		f.catalog.PutEpisode(ep)
		s.episodes = append(s.episodes, ep)
	}
	return s
}

func (s seriesFixture) searchEpisodes(t *testing.T, eps ...catalogapi.Episode) {
	t.Helper()
	ids := []domain.EpisodeID{}
	for _, ep := range eps {
		ids = append(ids, ep.ID)
	}
	req := acquisition.SearchRequest{Title: s.title.ID, Episodes: ids, Trigger: api.TriggerAuto}
	if _, err := s.searches.Start(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}

func TestAPackCoversItsEpisodesAndEachEpisodeIsGrabbedOnce(t *testing.T) {
	const (
		pack   = "Severance.S01.1080p.WEB-DL.DDP5.1.H.264-GRP"
		first  = "Severance.S01E01.1080p.WEB-DL.DDP5.1.H.264-GRP"
		second = "Severance.S01E02.1080p.WEB-DL.DDP5.1.H.264-GRP"
	)
	t.Run("a pack", func(t *testing.T) {
		s := newSeriesFixture(t)
		s.indexer(t, fastID, "Fast", tvIDCaps, 0, release(first), release(pack), release(second))
		s.searchEpisodes(t, s.episodes[0], s.episodes[1])
		s.settle(t)
		if grabs := s.grabbed(t); len(grabs) != 1 || grabs[0].Release != pack || len(grabs[0].Episodes) != 3 {
			t.Errorf("grabs = %+v, want only the pack", grabs)
		}
	})
	t.Run("singles", func(t *testing.T) {
		s := newSeriesFixture(t)
		s.indexer(t, fastID, "Fast", tvIDCaps, 0, release(first), release(second))
		s.searchEpisodes(t, s.episodes[0], s.episodes[1])
		s.settle(t)
		names := []string{}
		for _, g := range s.grabbed(t) {
			names = append(names, g.Release)
		}
		slices.Sort(names)
		if !slices.Equal(names, []string{first, second}) {
			t.Errorf("grabbed %v, want each episode once", names)
		}
	})
}

func TestMonitoringChangesSearchNewItemsAndDropPendingReleases(t *testing.T) {
	s := newSeriesFixture(t)
	unaired := s.episodes[2]
	unaired.Released = false
	s.catalog.PutEpisode(unaired)
	handler := acquisition.NewMonitoringChangedHandler(s.searches)

	added := []domain.EpisodeID{s.episodes[0].ID, unaired.ID}
	s.deliver(t, handler, bus.EventOf(catalogapi.MonitoringChanged{Title: s.title.ID, Added: added}))
	tasks := s.searchTasks()
	if len(tasks) != 1 || !slices.Equal(tasks[0].Episodes, []domain.EpisodeID{s.episodes[0].ID}) {
		t.Fatalf("search jobs = %+v, want the released episode only", tasks)
	}

	s.useTieredProfile(t)
	s.setDelay(t, twoHours)
	s.indexer(t, fastID, "Fast", tvIDCaps, 0, published("Severance.S01E02.1080p.WEB-DL.DDP5.1.H.264-GRP", 0, time.Now()))
	s.searchEpisodes(t, s.episodes[1])
	s.settle(t)
	if held := s.pending(t); len(held) != 1 {
		t.Fatalf("pending = %+v, want the delayed episode release", held)
	}
	s.deliver(t, handler, bus.EventOf(catalogapi.MonitoringChanged{Title: s.title.ID, Removed: []domain.EpisodeID{s.episodes[1].ID}}))
	if held := s.pending(t); len(held) != 0 {
		t.Errorf("pending = %+v, want the unmonitored episode's release dropped", held)
	}
}

func TestTitleRemovedCancelsSearchesDropsPendingAndRemovesDownloads(t *testing.T) {
	f := newSearchFixture(t)
	f.useTieredProfile(t)
	f.setDelay(t, twoHours)
	f.indexer(t, fastID, "Fast", idCaps, 0, published(belowCutoffName, 0, time.Now()))
	f.startAuto(t, api.TriggerAuto)
	f.settle(t)
	if held := f.pending(t); len(held) != 1 {
		t.Fatalf("pending = %+v, want one", held)
	}
	f.indexer(t, slowID, "Other", idCaps, 0, release(cutoffName))
	f.start(t)
	snapshot, err := f.searches.Search(payload[api.SearchCompleted](t, f.events.waitFor(t, api.TypeSearchCompleted, 2)[1]).Search)
	if err != nil {
		t.Fatal(err)
	}
	grabbed := f.grab(t, snapshot, cutoffName)
	f.indexer(t, stuckID, "Stuck", idCaps, neverAnswers)
	running := f.start(t)

	f.deliver(t, acquisition.NewTitleRemovedHandler(f.searches), bus.EventOf(catalogapi.TitleRemoved{Title: f.title.ID, DeleteFiles: true}))

	cancelled := f.events.waitFor(t, api.TypeSearchCompleted, 3)[2]
	if done := payload[api.SearchCompleted](t, cancelled); done.Search != running.ID || !slices.Contains(done.FailedIndexers, "Stuck") {
		t.Errorf("SearchCompleted = %+v, want the running search cancelled", done)
	}
	if held := f.pending(t); len(held) != 0 {
		t.Errorf("pending = %+v, want none", held)
	}
	var removal *acquisition.TitleRemoval
	for _, task := range f.jobs.queued() {
		if r, ok := acquisition.TitleRemovalOf(task); ok {
			removal = &r
		}
	}
	if removal == nil || removal.Title != f.title.ID || !removal.DeleteFiles {
		t.Fatalf("removal job = %+v, want one deleting files", removal)
	}
	if err := f.downloads.RemoveTitle(context.Background(), removal.Title, removal.DeleteFiles); err != nil {
		t.Fatal(err)
	}
	if d := f.download(t, *grabbed.Download); d.RemovedAt == nil || f.qbit.Called(fakes.QBittorrentDelete) != 1 {
		t.Errorf("download = %+v, want it removed from qBittorrent", d)
	}
}

func TestBacklogSweepsQueueMissingAndCutoffUnmetItemsTwoSecondsPerQuery(t *testing.T) {
	s := newSeriesFixture(t)
	file := libraryapi.MediaFile{
		ID: domain.NewID[domain.MediaFileID](), Title: s.title.ID, Path: "/tv/Severance/S01E01.mkv",
		Episodes:    []domain.EpisodeRef{{Episode: s.episodes[0].ID, Season: 1, Number: 1}},
		ReleaseName: "Severance.S01E01.720p.WEB-DL.DDP5.1.H.264-GRP", Quality: profileQuality(parse.QualityWEBDL720p),
		Origin: domain.OriginDownload,
	}
	s.library.Put(librarytest.StoredFile{File: file})
	s.deliverScores(t, libraryapi.FileImported{
		MediaFile: file.ID, Title: file.Title, Episodes: file.Episodes, Path: file.Path, Quality: file.Quality,
		ReleaseName: file.ReleaseName, Origin: file.Origin,
	})

	if err := s.searches.SweepMissing(context.Background()); err != nil {
		t.Fatal(err)
	}
	missing := s.jobs.laterTasks()
	if len(missing) != 2 {
		t.Fatalf("missing sweep queued %d searches, want the movie and the two missing episodes", len(missing))
	}
	if gap := missing[1].at.Sub(missing[0].at); gap < acquisition.MinSweepGap {
		t.Errorf("searches %s apart, want at least %s", gap, acquisition.MinSweepGap)
	}
	if err := s.searches.SweepCutoffUnmet(context.Background()); err != nil {
		t.Fatal(err)
	}
	cutoff := s.jobs.laterTasks()[len(missing):]
	if len(cutoff) != 1 {
		t.Fatalf("cutoff sweep queued %d searches, want the 720p episode", len(cutoff))
	}
	if req, _ := acquisition.SearchTaskOf(cutoff[0].task); !slices.Equal(req.Episodes, []domain.EpisodeID{s.episodes[0].ID}) {
		t.Errorf("cutoff search = %+v, want episode 1", req)
	}
}

func TestStartingANonInteractiveSearchRunsAUserAutomaticSearch(t *testing.T) {
	f := newSearchFixture(t)
	f.indexer(t, stuckID, "Stuck", idCaps, neverAnswers)
	handler := serveSearches(f, admin())

	rec := do(handler, http.MethodPost, "/api/acquisition/searches", `{"title":"`+string(f.title.ID)+`","interactive":false}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	if got := decode[acquisition.SearchResponse](t, rec); got.Trigger != api.TriggerUserAuto {
		t.Errorf("trigger = %s, want USER_AUTO", got.Trigger)
	}
}

func serveAutomaticSearch(f searchFixture, principal *domain.Principal) http.Handler {
	controllers := []httpserver.Controller{acquisition.NewAutomaticSearchController(f.searches)}
	mux := httpserver.Build(config.Env{}, httpserver.ModeServe, controllers).Mux
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal != nil {
			r = r.WithContext(domain.WithPrincipal(r.Context(), *principal))
		}
		mux.ServeHTTP(w, r)
	})
}

func TestAutomaticSearchSettingsAreAdminOnly(t *testing.T) {
	f := newSearchFixture(t)
	requester := &domain.Principal{User: domain.NewID[domain.UserID]()}
	handler := serveAutomaticSearch(f, requester)
	for _, path := range []string{"/api/acquisition/schedules", "/api/acquisition/search-options"} {
		if rec := do(handler, http.MethodGet, path, ""); rec.Code != http.StatusForbidden {
			t.Errorf("GET %s = %d, want 403", path, rec.Code)
		}
	}
}

func TestSchedulesAndSearchOptionsAreValidated(t *testing.T) {
	f := newSearchFixture(t)
	handler := serveAutomaticSearch(f, admin())

	if rec := do(handler, http.MethodGet, "/api/acquisition/schedules", ""); rec.Code != http.StatusOK {
		t.Fatalf("get schedules = %d", rec.Code)
	} else if got := decode[acquisition.SchedulesResponse](t, rec); got.RSSIntervalMinutes != 15 || got.MissingSweepDays != 1 ||
		got.CutoffSweepDays != 7 || got.PendingCheckMinutes != 1 {
		t.Errorf("schedules = %+v, want the defaults", got)
	}
	for _, body := range []string{
		`{"rssIntervalMinutes":5,"missingSweepDays":1,"cutoffSweepDays":7}`,
		`{"rssIntervalMinutes":15,"missingSweepDays":8,"cutoffSweepDays":7}`,
		`{"rssIntervalMinutes":15,"missingSweepDays":1,"cutoffSweepDays":31}`,
	} {
		if rec := do(handler, http.MethodPut, "/api/acquisition/schedules", body); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d, want 400", body, rec.Code)
		}
	}
	rec := do(handler, http.MethodPut, "/api/acquisition/schedules", `{"rssIntervalMinutes":30,"missingSweepDays":2,"cutoffSweepDays":14}`)
	if got := decode[acquisition.SchedulesResponse](t, rec); rec.Code != http.StatusOK || got.RSSIntervalMinutes != 30 {
		t.Errorf("PUT = %d %+v", rec.Code, got)
	}

	if got := decode[acquisition.SearchOptionsResponse](t, do(handler, http.MethodGet, "/api/acquisition/search-options", "")); !got.SearchOnAdd {
		t.Error("search on add is off by default")
	}
	rec = do(handler, http.MethodPut, "/api/acquisition/search-options", `{"searchOnAdd":false}`)
	if got := decode[acquisition.SearchOptionsResponse](t, rec); rec.Code != http.StatusOK || got.SearchOnAdd {
		t.Errorf("PUT search options = %d %+v", rec.Code, got)
	}
}
