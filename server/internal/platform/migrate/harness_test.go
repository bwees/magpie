package migrate_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	acquisitiontest "magpie/internal/acquisition/api/contracttest"
	activitytest "magpie/internal/activity/api/contracttest"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/migrate"
	"magpie/internal/platform/migrate/fakes"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	sonarrVersion = "4.0.20.3014"
	radarrVersion = "6.4.4.10685"

	conclaveTmdb domain.TmdbID = 974576
	blueyTmdb    domain.TmdbID = 82728
	fireflyTmdb  domain.TmdbID = 1437
	blueyTvdb                  = 371980
	fireflyTvdb                = 78874
	fireflyDVD                 = "firefly-dvd"

	conclaveID = 40
	blueyID    = 1
	fireflyID  = 2

	sonarrRemux1080p = 20
	sonarrWebDL1080p = 3
	sonarrHDTV1080p  = 9
	bluray1080p      = 7
	blurayRawSource  = "7"

	hdProfileID   = 4
	hdProfileName = "HD-1080p"
	remuxFormatID = 3

	blockedRelease = "Bluey.2018.S01E02.1080p.WEB-DL-BAD"
	grabbedHash    = "ABCDEF0123456789ABCDEF0123456789ABCDEF01"
	prowlarrSource = "Tracker (Prowlarr)"

	tvFrom     = "/tv"
	tvTo       = "/data/tv"
	moviesFrom = "/movies"
	moviesTo   = "/data/movies"
)

var addedAt = time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)

// queued stands in for backlite: tasks wait until run processes them.
type queued struct {
	mu    sync.Mutex
	tasks []backlite.Task
}

func (q *queued) Enqueue(_ context.Context, task backlite.Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, task)
	return nil
}

func (q *queued) EnqueueTx(tx *gorm.DB, task backlite.Task) error {
	return db.AfterCommit(tx, func() { _ = q.Enqueue(tx.Statement.Context, task) })
}

func (q *queued) take() []backlite.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	tasks := q.tasks
	q.tasks = nil
	return tasks
}

type harness struct {
	t           *testing.T
	service     *migrate.Service
	arr         *fakes.Arr
	tmdb        *fakes.TMDB
	disk        *fakes.Disk
	jobs        *queued
	catalog     *catalogtest.ImporterFake
	acquisition *acquisitiontest.ImporterFake
	library     *librarytest.ImporterFake
	activity    *activitytest.ImporterFake
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t: t, arr: fakes.NewArr(), tmdb: fakes.NewTMDB(), disk: fakes.NewDisk(), jobs: &queued{},
		catalog: catalogtest.NewImporterFake(catalogtest.ImporterWorld{
			Movies: []domain.TmdbID{conclaveTmdb},
			Series: []catalogtest.ImporterSeries{{Tmdb: blueyTmdb, Seasons: map[int]int{1: 2}}, {Tmdb: fireflyTmdb, Seasons: map[int]int{1: 4, 2: 4}}},
		}),
		acquisition: acquisitiontest.NewImporterFake(),
		library: librarytest.NewImporterFake(librarytest.ImporterWorld{
			Folders: []string{tvTo, moviesTo},
			Videos:  []string{conclaveVideoTo, blueyVideoTo, fireflyVideoTo},
		}),
		activity: activitytest.NewImporterFake(),
	}
	h.service = migrate.NewService(migrate.Params{
		DB: dbtest.Open(t), Bus: bus.New(), Jobs: h.jobs, Arr: h.arr, TMDB: h.tmdb, Disk: h.disk,
		Importers: migrate.Importers{Catalog: h.catalog, Acquisition: h.acquisition, Library: h.library, Activity: h.activity},
	})
	h.arr.Put(sonarrLibrary())
	h.arr.Put(radarrLibrary())
	h.putTMDB()
	for _, folder := range []string{tvTo, moviesTo} {
		h.disk.AddFolder(folder)
	}
	for _, file := range []string{conclaveVideoTo, blueyVideoTo, fireflyVideoTo} {
		h.disk.AddFile(file)
	}
	return h
}

var (
	conclaveVideo   = moviesFrom + "/Conclave (2024)/Conclave.2024.1080p.BluRay.x264-GRP.mkv"
	conclaveVideoTo = moviesTo + "/Conclave (2024)/Conclave.2024.1080p.BluRay.x264-GRP.mkv"
	blueyVideo      = tvFrom + "/Bluey/Season 01/Bluey.S01E01.mkv"
	blueyVideoTo    = tvTo + "/Bluey/Season 01/Bluey.S01E01.mkv"
	fireflyVideo    = tvFrom + "/Firefly/Season 02/Firefly.S02E01.mkv"
	fireflyVideoTo  = tvTo + "/Firefly/Season 02/Firefly.S02E01.mkv"
)

func quality(id int) migrate.ArrProfileItem {
	return migrate.ArrProfileItem{QualityID: id, Allowed: true}
}

func sonarrLibrary() migrate.ArrLibrary {
	bluey := migrate.ArrSeries{
		ID: blueyID, Title: "Bluey", Year: 2018, TvdbID: blueyTvdb, QualityProfileID: hdProfileID, Monitored: true,
		Path: tvFrom + "/Bluey", SeriesType: migrate.SeriesTypeStandard, Added: addedAt,
		Episodes: []migrate.ArrEpisode{
			{ID: 101, SeasonNumber: 1, EpisodeNumber: 1, Monitored: true, EpisodeFileID: 11},
			{ID: 102, SeasonNumber: 1, EpisodeNumber: 2, Monitored: true},
		},
		Files: []migrate.ArrFile{{ID: 11, Path: blueyVideo, Size: 1, QualityID: sonarrRemux1080p, Revision: 1, SceneName: "Bluey.2018.S01E01.1080p.BluRay.REMUX-GRP"}},
	}
	firefly := migrate.ArrSeries{
		ID: fireflyID, Title: "Firefly", Year: 2002, TmdbID: int(fireflyTmdb), TvdbID: fireflyTvdb, QualityProfileID: hdProfileID,
		Monitored: true, Path: tvFrom + "/Firefly", SeriesType: migrate.SeriesTypeStandard, Added: addedAt,
		Files: []migrate.ArrFile{{ID: 21, Path: fireflyVideo, Size: 1, QualityID: sonarrWebDL1080p, Revision: 1}},
	}
	for i := range 8 {
		season, number := 1+i/4, 1+i%4
		e := migrate.ArrEpisode{ID: 201 + i, SeasonNumber: season, EpisodeNumber: number, Monitored: true}
		if season == 2 && number == 1 {
			e.EpisodeFileID = 21
		}
		firefly.Episodes = append(firefly.Episodes, e)
	}
	return migrate.ArrLibrary{
		App: migrate.AppSonarr, Version: sonarrVersion,
		Series:      []migrate.ArrSeries{bluey, firefly},
		RootFolders: []migrate.ArrRootFolder{{ID: 1, Path: tvFrom}},
		QualityProfiles: []migrate.ArrQualityProfile{{
			ID: hdProfileID, Name: hdProfileName, UpgradeAllowed: true, Cutoff: bluray1080p,
			Items:       []migrate.ArrProfileItem{quality(sonarrHDTV1080p), quality(sonarrWebDL1080p), quality(bluray1080p)},
			FormatItems: []migrate.ArrFormatScore{{Format: remuxFormatID, Score: 100}},
		}},
		CustomFormats: []migrate.ArrCustomFormat{{ID: remuxFormatID, Name: "Remux", Specs: []migrate.ArrSpec{
			{Name: "BlurayRaw", Implementation: migrate.ImplSource, Value: blurayRawSource},
		}}},
		Blocklist: []migrate.ArrBlocklistItem{{
			ID: 601, SeriesID: blueyID, EpisodeIDs: []int{102}, SourceTitle: blockedRelease, Protocol: migrate.ProtocolTorrent,
			Indexer: prowlarrSource, Date: addedAt,
		}},
		History: []migrate.ArrHistoryItem{
			{ID: 501, SeriesID: blueyID, EpisodeID: 102, SourceTitle: blockedRelease, EventType: migrate.HistoryGrabbed, Date: addedAt,
				Data: map[string]string{migrate.HistoryDataTorrentInfoHash: grabbedHash, migrate.HistoryDataIndexer: prowlarrSource}},
			{ID: 502, SeriesID: blueyID, EpisodeID: 102, SourceTitle: blockedRelease, EventType: migrate.HistoryDownloadFailed, Date: addedAt.Add(time.Hour),
				Data: map[string]string{migrate.HistoryDataMessage: "stalled"}},
		},
	}
}

func radarrLibrary() migrate.ArrLibrary {
	return migrate.ArrLibrary{
		App: migrate.AppRadarr, Version: radarrVersion,
		Movies: []migrate.ArrMovie{{
			ID: conclaveID, Title: "Conclave", Year: 2024, TmdbID: int(conclaveTmdb), ImdbID: "tt20215234", Path: moviesFrom + "/Conclave (2024)",
			QualityProfileID: hdProfileID, Monitored: true, Added: addedAt,
			File: &migrate.ArrFile{ID: 41, Path: conclaveVideo, Size: 1, QualityID: bluray1080p, Revision: 1, SceneName: "Conclave.2024.1080p.BluRay.x264-GRP"},
		}},
		RootFolders: []migrate.ArrRootFolder{{ID: 1, Path: moviesFrom}},
		QualityProfiles: []migrate.ArrQualityProfile{{
			ID: hdProfileID, Name: hdProfileName, UpgradeAllowed: true, Cutoff: bluray1080p,
			Items: []migrate.ArrProfileItem{quality(sonarrWebDL1080p), quality(bluray1080p)},
		}},
	}
}

func tmdbSeason(series domain.TmdbID, number, count int, firstID int64) migrate.TmdbSeason {
	season := migrate.TmdbSeason{Number: number}
	for n := 1; n <= count; n++ {
		season.Episodes = append(season.Episodes, migrate.TmdbEpisode{ID: firstID + int64(n-1), Season: number, Number: n})
	}
	return season
}

// putTMDB stores Bluey (one season of two) and Firefly, which TMDB airs as one season of eight but whose
// DVD order is Sonarr's two seasons of four.
func (h *harness) putTMDB() {
	blueyTvdbID := "371980"
	h.tmdb.PutSeries(migrate.TmdbSeries{TmdbCandidate: migrate.TmdbCandidate{Tmdb: blueyTmdb, Kind: domain.MediaKindSeries, Name: "Bluey", Year: 2018}, Tvdb: &blueyTvdbID},
		tmdbSeason(blueyTmdb, 1, 2, 1001))
	h.tmdb.PutFind(migrate.FindTvdb, blueyTvdbID, migrate.TmdbFound{Series: []migrate.TmdbCandidate{{Tmdb: blueyTmdb, Kind: domain.MediaKindSeries, Name: "Bluey"}}})

	fireflyTvdbID := "78874"
	h.tmdb.PutSeries(migrate.TmdbSeries{TmdbCandidate: migrate.TmdbCandidate{Tmdb: fireflyTmdb, Kind: domain.MediaKindSeries, Name: "Firefly", Year: 2002}, Tvdb: &fireflyTvdbID},
		tmdbSeason(fireflyTmdb, 1, 8, 2001))
	dvd := migrate.TmdbGroup{ID: fireflyDVD, Name: "DVD Order", Type: migrate.GroupDVD}
	for order := 1; order <= 2; order++ {
		season := migrate.TmdbGroupSeason{Order: order}
		for i := range 4 {
			season.Episodes = append(season.Episodes, migrate.TmdbGroupEntry{Episode: 2001 + int64((order-1)*4+i), Order: i})
		}
		dvd.Seasons = append(dvd.Seasons, season)
	}
	h.tmdb.PutGroup(fireflyTmdb, dvd)
	h.tmdb.PutGroup(fireflyTmdb, migrate.TmdbGroup{ID: "firefly-tv", Name: "TV Order", Type: migrate.GroupTV, Seasons: []migrate.TmdbGroupSeason{{Order: 1}}})
}

// run processes queued tasks until none are left.
func (h *harness) run() {
	h.t.Helper()
	queue := migrate.NewQueue(h.service)
	for {
		tasks := h.jobs.take()
		if len(tasks) == 0 {
			return
		}
		for _, task := range tasks {
			payload, err := json.Marshal(task)
			if err != nil {
				h.t.Fatal(err)
			}
			_ = queue.Process(context.Background(), payload)
		}
	}
}

func (h *harness) state() migrate.Session {
	h.t.Helper()
	s, err := h.service.State(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return s
}

// connect connects both apps and points their root folders at the server's paths.
func (h *harness) connect() {
	h.t.Helper()
	ctx := context.Background()
	for _, app := range migrate.Apps {
		if _, err := h.service.Connect(ctx, migrate.ConnectRequest{App: app, Connection: migrate.Connection{URL: "http://" + string(app), APIKey: "key"}}); err != nil {
			h.t.Fatal(err)
		}
	}
	paths := []migrate.PathRewrite{{App: migrate.AppSonarr, From: tvFrom, To: tvTo}, {App: migrate.AppRadarr, From: moviesFrom, To: moviesTo}}
	if _, err := h.service.SetPaths(ctx, paths); err != nil {
		h.t.Fatal(err)
	}
}

// dryRun connects and runs the dry run to its report.
func (h *harness) dryRun() *migrate.Plan {
	h.t.Helper()
	h.connect()
	if _, err := h.service.StartDryRun(context.Background()); err != nil {
		h.t.Fatal(err)
	}
	h.run()
	s := h.state()
	if s.Phase != migrate.PhaseReview || s.Plan == nil {
		h.t.Fatalf("dry run ended in %s (%s), want REVIEW with a plan", s.Phase, s.Error)
	}
	return s.Plan
}

// clearReviews settles every queue with its default and confirms the cutover.
func (h *harness) clearReviews() {
	h.t.Helper()
	ctx := context.Background()
	for _, q := range migrate.ReviewQueues {
		if _, err := h.service.SkipQueue(ctx, q); err != nil {
			h.t.Fatal(err)
		}
	}
	if _, err := h.service.SetCutover(ctx, true); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) commit() migrate.Session {
	h.t.Helper()
	if _, err := h.service.StartCommit(context.Background()); err != nil {
		h.t.Fatal(err)
	}
	h.run()
	return h.state()
}

func titleOf(plan *migrate.Plan, source domain.SourceKey) migrate.PlannedTitle {
	for _, t := range plan.Titles {
		if t.Source == source {
			return t
		}
	}
	return migrate.PlannedTitle{}
}
