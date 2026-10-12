package acquisition_test

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	"magpie/internal/acquisition"
	"magpie/internal/acquisition/api"
	"magpie/internal/acquisition/fakes"
	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/settings"
)

// Seed #3 runs at a hundredth of its real timing: answers at 10 ms and 50 ms, a 600 ms timeout.
const (
	fastAnswer    = 10 * time.Millisecond
	slowAnswer    = 50 * time.Millisecond
	searchTimeout = 600 * time.Millisecond
	neverAnswers  = time.Hour
	waitForEvent  = 5 * time.Second
	quietPeriod   = 100 * time.Millisecond

	fastID, slowID, stuckID = 11, 12, 13
	gigabyte                = 1 << 30
	releaseSize             = 8 * gigabyte
	releaseSeeders          = 40
	movieImdb               = "tt15239678"
	movieTmdb               = domain.TmdbID(693134)
	acceptedName            = "Movie.Title.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
	rejectedName            = "Movie.Title.2024.720p.WEB-DL.DDP5.1.H.264-GRP"
)

var (
	idCaps = acquisition.TorznabCaps{
		MovieSearch: true,
		MovieParams: []acquisition.TorznabParam{acquisition.ParamQuery, acquisition.ParamIMDb, acquisition.ParamTMDB},
		Categories:  []int{2000, 2040, 5000},
	}
	imdbOnlyCaps = acquisition.TorznabCaps{
		MovieSearch: true,
		MovieParams: []acquisition.TorznabParam{acquisition.ParamQuery, acquisition.ParamIMDb},
	}
	titleOnlyCaps = acquisition.TorznabCaps{
		MovieSearch: true,
		MovieParams: []acquisition.TorznabParam{acquisition.ParamQuery, acquisition.ParamYear},
	}
)

type recorder struct {
	mu     sync.Mutex
	events []bus.Event
	added  chan struct{}
}

func newRecorder(b *bus.Bus) *recorder {
	r := &recorder{added: make(chan struct{}, 1)}
	b.Subscribe(func(ev bus.Event) {
		r.mu.Lock()
		r.events = append(r.events, ev)
		r.mu.Unlock()
		select {
		case r.added <- struct{}{}:
		default:
		}
	})
	return r
}

func (r *recorder) ofType(eventType string) []bus.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(r.events), func(ev bus.Event) bool { return ev.Type != eventType })
}

// waitFor blocks until n events of the type arrived.
func (r *recorder) waitFor(t *testing.T, eventType string, n int) []bus.Event {
	t.Helper()
	deadline := time.After(waitForEvent)
	for {
		if got := r.ofType(eventType); len(got) >= n {
			return got
		}
		select {
		case <-r.added:
		case <-deadline:
			t.Fatalf("got %d %s events, want %d", len(r.ofType(eventType)), eventType, n)
		}
	}
}

func payload[T any](t *testing.T, ev bus.Event) T {
	t.Helper()
	v, err := bus.Decode[T](ev)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

type searchFixture struct {
	fixture
	db        *db.DB
	quality   *acquisition.QualityService
	searches  *acquisition.SearchService
	downloads *acquisition.DownloadService
	cutoff    *acquisition.CutoffService
	jobs      *recordingQueue
	clock     *clock
	catalog   *catalogtest.Fake
	library   *librarytest.Fake
	qbit      *fakes.QBittorrent
	events    *recorder
	title     catalogapi.Title
}

func newSearchFixture(t *testing.T) searchFixture {
	t.Helper()
	ctx := context.Background()
	d := dbtest.Open(t, acquisition.Migrations())
	store := settings.NewStore(d)
	repo := acquisition.NewRepository(d)
	quality := acquisition.NewQualityRepository(d)
	jobs := &recordingQueue{}
	qualityService := acquisition.NewQualityService(d, quality, store, jobs)
	if err := qualityService.SeedPresets(ctx); err != nil {
		t.Fatal(err)
	}
	options := acquisition.IndexerOptions{
		SyncInterval:  settings.Duration(acquisition.DefaultIndexerSyncInterval),
		SearchTimeout: settings.Duration(searchTimeout),
		ResultCache:   settings.Duration(acquisition.DefaultResultCache),
	}
	if err := acquisition.IndexerOptionsKey.Set(ctx, store, options); err != nil {
		t.Fatal(err)
	}

	imdb := movieImdb
	title := catalogapi.Title{
		ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindMovie, Name: "Movie Title", Year: 2024,
		External: domain.ExternalIDs{Tmdb: movieTmdb, Imdb: &imdb}, RuntimeMinutes: 120, Monitored: true, Released: true,
	}
	b := bus.New()
	f := searchFixture{
		fixture: fixture{repo: repo, prowlarr: fakes.NewProwlarr(), store: store},
		db:      d,
		quality: qualityService,
		catalog: catalogtest.NewFake(catalogtest.Fixture{Titles: []catalogapi.Title{title}}),
		library: librarytest.NewFake(librarytest.Fixture{}),
		qbit:    fakes.NewQBittorrent(),
		events:  newRecorder(b),
		title:   title,
		jobs:    jobs,
	}
	f.service = acquisition.NewService(repo, f.prowlarr, store)
	registry := acquisition.NewSearchRegistry()
	f.searches = acquisition.NewSearchService(acquisition.SearchDeps{
		DB: d, Repo: repo, Quality: quality, Catalog: f.catalog, Library: f.library,
		Prowlarr: f.prowlarr, QBittorrent: f.qbit, Store: store, Bus: b, Searches: registry, Jobs: f.jobs,
	})
	t.Cleanup(f.searches.Close)
	f.clock = &clock{now: time.Now().Truncate(time.Second)}
	f.downloads = acquisition.NewDownloadService(acquisition.DownloadDeps{
		DB: d, Repo: repo, Catalog: f.catalog, Prowlarr: f.prowlarr, QBittorrent: f.qbit,
		Store: store, Bus: b, Jobs: f.jobs, Searches: registry,
	})
	acquisition.SetDownloadClock(f.downloads, f.clock.Now)
	f.cutoff = acquisition.NewCutoffService(acquisition.CutoffDeps{DB: d, Repo: repo, Catalog: f.catalog, Bus: b})
	return f
}

// clock is the download service's time, moved by tests to reach timeouts.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (f searchFixture) indexer(t *testing.T, id int, name string, caps acquisition.TorznabCaps, delay time.Duration, items ...acquisition.TorznabItem) {
	t.Helper()
	f.prowlarr.AddIndexer(healthy(id, name, caps))
	f.prowlarr.DelayIndexer(id, delay)
	f.prowlarr.SetResults(id, items...)
	f.sync(t)
}

// release is offered as a magnet; tests that need a .torrent set DownloadURL.
func release(name string) acquisition.TorznabItem {
	return acquisition.TorznabItem{GUID: name, Title: name, MagnetURI: magnet(name), SizeBytes: releaseSize,
		Seeders: releaseSeeders, DownloadVolumeFactor: 1, UploadVolumeFactor: 1}
}

func magnet(name string) string {
	sum := sha1.Sum([]byte(name))
	return "magnet:?xt=urn:btih:" + hex.EncodeToString(sum[:]) + "&dn=" + url.QueryEscape(name)
}

func (f searchFixture) start(t *testing.T) acquisition.SearchSnapshot {
	t.Helper()
	s, err := f.searches.Start(context.Background(), acquisition.SearchRequest{Title: f.title.ID, Trigger: api.TriggerInteractive})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f searchFixture) completed(t *testing.T) api.SearchCompleted {
	t.Helper()
	return payload[api.SearchCompleted](t, f.events.waitFor(t, api.TypeSearchCompleted, 1)[0])
}

func results(t *testing.T, events []bus.Event) []api.SearchResultsReceived {
	t.Helper()
	out := make([]api.SearchResultsReceived, 0, len(events))
	for _, ev := range events {
		out = append(out, payload[api.SearchResultsReceived](t, ev))
	}
	return out
}

// Seed acquisition #3.
func TestSearchStreamsOneResultPerIndexerAndTimesOut(t *testing.T) {
	f := newSearchFixture(t)
	f.indexer(t, fastID, "Fast", idCaps, fastAnswer, release(acceptedName))
	f.indexer(t, slowID, "Slow", idCaps, slowAnswer, release(rejectedName))
	f.indexer(t, stuckID, "Stuck", idCaps, neverAnswers)

	started := f.start(t)
	done := f.completed(t)
	time.Sleep(quietPeriod)

	got := results(t, f.events.ofType(api.TypeSearchResultsReceived))
	statuses := map[string]api.IndexerRunStatus{}
	for _, r := range got {
		statuses[r.Indexer] = r.Status
	}
	if len(got) != 3 {
		t.Fatalf("got %d SearchResultsReceived, want 3: %+v", len(got), got)
	}
	want := map[string]api.IndexerRunStatus{"Fast": api.IndexerDone, "Slow": api.IndexerDone, "Stuck": api.IndexerFailed}
	for name, status := range want {
		if statuses[name] != status {
			t.Errorf("%s = %s, want %s", name, statuses[name], status)
		}
	}
	if got[0].Indexer != "Fast" || got[1].Indexer != "Slow" {
		t.Errorf("order = %s, %s, want Fast then Slow", got[0].Indexer, got[1].Indexer)
	}
	if !slices.Equal(done.FailedIndexers, []string{"Stuck"}) || done.Search != started.ID {
		t.Errorf("SearchCompleted = %+v, want FailedIndexers [Stuck]", done)
	}
	if done.Total != 2 || done.Accepted != 1 {
		t.Errorf("counts total=%d accepted=%d, want 2 and 1", done.Total, done.Accepted)
	}

	startedEvents := f.events.ofType(api.TypeSearchStarted)
	if len(startedEvents) != 1 {
		t.Fatalf("got %d SearchStarted, want 1", len(startedEvents))
	}
	if s := payload[api.SearchStarted](t, startedEvents[0]); s.Trigger != api.TriggerInteractive || !s.Interactive || len(s.Indexers) != 3 {
		t.Errorf("SearchStarted = %+v", s)
	}
	if stuck := f.stored(t, stuckID); stuck.Status != acquisition.IndexerFailing || stuck.LastError == "" {
		t.Errorf("timed-out indexer stored as %s %q, want FAILING with the error", stuck.Status, stuck.LastError)
	}
}

func TestReleaseDecisionsCarryQualityFormatsAndRank(t *testing.T) {
	f := newSearchFixture(t)
	freeleech := release(acceptedName)
	freeleech.DownloadVolumeFactor = 0
	f.indexer(t, fastID, "Fast", idCaps, 0, freeleech, release(rejectedName))

	f.start(t)
	got := results(t, f.events.waitFor(t, api.TypeSearchResultsReceived, 1))[0]

	if len(got.Releases) != 2 {
		t.Fatalf("releases = %+v", got.Releases)
	}
	accepted, rejected := got.Releases[0], got.Releases[1]
	if accepted.Rank != 1 || len(accepted.Rejections) != 0 || !accepted.Freeleech || accepted.Quality.Name != "WEBDL-1080p" {
		t.Errorf("accepted = %+v", accepted)
	}
	if rejected.Rank != 0 || len(rejected.Rejections) == 0 || rejected.Rejections[0].Reason != api.RejectQualityNotAllowed {
		t.Errorf("rejected = %+v", rejected)
	}
	if accepted.Indexer != "Fast" || accepted.Release == "" || accepted.Release == rejected.Release {
		t.Errorf("release IDs %q and %q, indexer %q", accepted.Release, rejected.Release, accepted.Indexer)
	}
}

func TestALateAnswerStillStreamsIntoAnOpenSearch(t *testing.T) {
	const lateAnswer = searchTimeout + 200*time.Millisecond
	f := newSearchFixture(t)
	f.indexer(t, slowID, "Late", idCaps, lateAnswer, release(acceptedName))

	started := f.start(t)
	got := results(t, f.events.waitFor(t, api.TypeSearchResultsReceived, 2))

	if got[0].Status != api.IndexerFailed || got[1].Status != api.IndexerDone || len(got[1].Releases) != 1 {
		t.Fatalf("events = %+v, want FAILED then a late DONE with the release", got)
	}
	snapshot, err := f.searches.Search(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Indexers[0].Status != api.IndexerDone || len(snapshot.Releases) != 1 {
		t.Errorf("snapshot = %+v, want the late answer cached", snapshot)
	}
	if late := f.stored(t, slowID); late.Status != acquisition.IndexerOK || late.LastOKAt == nil {
		t.Errorf("late indexer stored as %s, last ok %v", late.Status, late.LastOKAt)
	}
}

// Seed acquisition #6.
func TestInteractiveSearchNeverGrabsAndOverridePassesTheGrabCheck(t *testing.T) {
	f := newSearchFixture(t)
	f.indexer(t, fastID, "Fast", idCaps, 0, release(acceptedName), release(rejectedName))

	started := f.start(t)
	done := f.completed(t)
	time.Sleep(quietPeriod)

	if done.Outcome != api.OutcomeReleasesFound || done.Grabbed != nil {
		t.Errorf("SearchCompleted = %+v, want RELEASES_FOUND with no grab", done)
	}
	if n := len(f.events.ofType(api.TypeReleaseGrabbed)); n != 0 || f.qbit.Called(fakes.QBittorrentAddTorrent) != 0 {
		t.Errorf("an interactive search grabbed: %d ReleaseGrabbed", n)
	}

	snapshot, err := f.searches.Search(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	var rejected api.ReleaseDecision
	for _, r := range snapshot.Releases {
		if len(r.Rejections) > 0 && r.Rejections[0].Reason == api.RejectQualityNotAllowed {
			rejected = r
		}
	}
	ctx := context.Background()
	if _, err := f.downloads.Grab(ctx, started.ID, rejected.Release, false); !errors.Is(err, acquisition.ErrReleaseRejected) {
		t.Errorf("grab without override = %v, want ErrReleaseRejected", err)
	}
	if _, err := f.downloads.Grab(ctx, started.ID, "missing", true); !errors.Is(err, acquisition.ErrReleaseNotFound) {
		t.Errorf("grab of an unknown release = %v", err)
	}
	result, err := f.downloads.Grab(ctx, started.ID, rejected.Release, true)
	if err != nil || result.Status != acquisition.GrabChecking {
		t.Fatalf("grab with override = %+v, %v; want a magnet waiting for its check", result, err)
	}
	grabbed := payload[api.ReleaseGrabbed](t, f.events.waitFor(t, api.TypeReleaseGrabbed, 1)[0])
	if !grabbed.Override || grabbed.Download != *result.Download || grabbed.Trigger != api.TriggerInteractive {
		t.Errorf("ReleaseGrabbed = %+v, want Override", grabbed)
	}
	if _, err := f.downloads.Grab(ctx, started.ID, rejected.Release, true); !errors.Is(err, acquisition.ErrAlreadyGrabbed) {
		t.Errorf("second grab = %v, want ErrAlreadyGrabbed", err)
	}
}

// Seed acquisition #23.
func TestQueriesUseSupportedIDsThenTitleAndYear(t *testing.T) {
	const imdbIndexer, titleIndexer = 21, 22
	f := newSearchFixture(t)
	f.indexer(t, imdbIndexer, "IMDb only", imdbOnlyCaps, 0, release(acceptedName))
	f.indexer(t, titleIndexer, "Title only", titleOnlyCaps, 0)

	f.start(t)
	got := results(t, f.events.waitFor(t, api.TypeSearchResultsReceived, 2))

	imdbQueries := f.prowlarr.Queries(imdbIndexer)
	if len(imdbQueries) != 1 || imdbQueries[0].Imdb != movieImdb || imdbQueries[0].Tmdb != 0 || imdbQueries[0].Query != "" {
		t.Errorf("IMDb-only indexer queries = %+v, want one imdbid query", imdbQueries)
	}
	titleQueries := f.prowlarr.Queries(titleIndexer)
	if len(titleQueries) != 1 || titleQueries[0].Query != "Movie Title" || titleQueries[0].Year == nil || *titleQueries[0].Year != 2024 || titleQueries[0].Imdb != "" {
		t.Errorf("title-only indexer queries = %+v, want title + year", titleQueries)
	}
	statuses := map[string]api.IndexerRunStatus{}
	for _, r := range got {
		statuses[r.Indexer] = r.Status
	}
	if statuses["Title only"] != api.IndexerSkipped || statuses["IMDb only"] != api.IndexerDone {
		t.Errorf("statuses = %v, want the title-only indexer SKIPPED", statuses)
	}
}

func TestAnEmptyIDSearchFallsBackToTheTitle(t *testing.T) {
	f := newSearchFixture(t)
	f.prowlarr.AddIndexer(healthy(fastID, "Fast", idCaps))
	f.prowlarr.SetResultsFunc(fastID, func(q acquisition.TorznabQuery) []acquisition.TorznabItem {
		if q.Query == "" {
			return nil
		}
		return []acquisition.TorznabItem{release(acceptedName)}
	})
	f.sync(t)

	f.start(t)
	got := results(t, f.events.waitFor(t, api.TypeSearchResultsReceived, 1))[0]

	queries := f.prowlarr.Queries(fastID)
	if len(queries) != 2 || queries[0].Tmdb != movieTmdb || queries[0].Imdb != movieImdb || queries[1].Query != "Movie Title 2024" {
		t.Errorf("queries = %+v, want ID then title with the year in the text", queries)
	}
	if got.Status != api.IndexerDone || len(got.Releases) != 1 {
		t.Errorf("result = %+v", got)
	}
	if !slices.Equal(queries[0].Categories, []int{2000, 2040}) {
		t.Errorf("categories = %v, want the movie ones from the caps", queries[0].Categories)
	}
}

func TestResultsAreCachedPerIndexerAndQuery(t *testing.T) {
	f := newSearchFixture(t)
	f.indexer(t, fastID, "Fast", idCaps, 0, release(acceptedName))

	f.start(t)
	f.events.waitFor(t, api.TypeSearchCompleted, 1)
	f.start(t)
	second := results(t, f.events.waitFor(t, api.TypeSearchResultsReceived, 2))[1]

	if n := len(f.prowlarr.Queries(fastID)); n != 1 {
		t.Errorf("Prowlarr was asked %d times, want once", n)
	}
	if len(second.Releases) != 1 {
		t.Errorf("second search got %+v, want the cached release", second)
	}
}

func TestCancelStopsInFlightQueries(t *testing.T) {
	f := newSearchFixture(t)
	f.indexer(t, stuckID, "Stuck", idCaps, neverAnswers)

	started := f.start(t)
	if err := f.searches.Cancel(started.ID); err != nil {
		t.Fatal(err)
	}
	done := f.completed(t)

	got := results(t, f.events.ofType(api.TypeSearchResultsReceived))
	if len(got) != 1 || got[0].Status != api.IndexerFailed {
		t.Errorf("events = %+v, want the pending indexer FAILED", got)
	}
	if !slices.Equal(done.FailedIndexers, []string{"Stuck"}) {
		t.Errorf("SearchCompleted = %+v", done)
	}
	snapshot, err := f.searches.Search(started.ID)
	if err != nil || !snapshot.Cancelled || !snapshot.Completed {
		t.Errorf("snapshot = %+v, %v; want cancelled and completed", snapshot, err)
	}
	if stuck := f.stored(t, stuckID); stuck.Status != acquisition.IndexerOK {
		t.Errorf("a cancelled query marked the indexer %s", stuck.Status)
	}
}

func TestFailingIndexerIsRecordedAndReportsItsError(t *testing.T) {
	f := newSearchFixture(t)
	f.indexer(t, fastID, "Broken", idCaps, 0)
	f.prowlarr.FailIndexer(fastID, &acquisition.ErrIndexerFailed{Code: 429, Message: "Request limit reached"})

	f.start(t)
	got := results(t, f.events.waitFor(t, api.TypeSearchResultsReceived, 1))[0]

	if got.Status != api.IndexerFailed || got.Message != "Request limit reached" {
		t.Errorf("result = %+v", got)
	}
	if stored := f.stored(t, fastID); stored.Status != acquisition.IndexerFailing || stored.LastError != "Request limit reached" || stored.FailingSince == nil {
		t.Errorf("stored = %+v", stored)
	}
}

func TestSearchWithoutIndexersCompletesAtOnce(t *testing.T) {
	f := newSearchFixture(t)
	tvOnly := acquisition.TorznabCaps{TVSearch: true, TVParams: []acquisition.TorznabParam{acquisition.ParamQuery}}
	f.indexer(t, fastID, "TV only", tvOnly, 0, release(acceptedName))

	started := f.start(t)
	done := f.completed(t)

	if len(started.Indexers) != 0 || done.Total != 0 || len(f.prowlarr.Queries(fastID)) != 0 {
		t.Errorf("snapshot %+v, completed %+v, want no indexer asked", started, done)
	}
}

func TestStartRejectsUnknownTitlesAndEpisodes(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()

	_, err := f.searches.Start(ctx, acquisition.SearchRequest{Title: "missing", Trigger: api.TriggerInteractive})
	if !errors.Is(err, acquisition.ErrTitleNotFound) {
		t.Errorf("unknown title = %v", err)
	}
	_, err = f.searches.Start(ctx, acquisition.SearchRequest{Title: f.title.ID, Episodes: []domain.EpisodeID{"e1"}, Trigger: api.TriggerInteractive})
	if !errors.Is(err, acquisition.ErrInvalidSearch) {
		t.Errorf("movie with episodes = %v", err)
	}
	if _, err := f.searches.Search("missing"); !errors.Is(err, acquisition.ErrSearchNotFound) {
		t.Errorf("unknown search = %v", err)
	}
}
