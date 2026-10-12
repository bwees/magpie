package subtitles_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	librarytest "magpie/internal/library/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/settings"
	"magpie/internal/subtitles"
	"magpie/internal/subtitles/api"
	"magpie/internal/subtitles/fakes"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	english  domain.Language = "en"
	spanish  domain.Language = "es"
	japanese domain.Language = "ja"

	showTmdb domain.TmdbID = 1399
	showYear               = 2011
	duneTmdb domain.TmdbID = 438631
	duneYear               = 2021

	showRelease = "Show.2011.S01E02.1080p.WEB-DL.DDP5.1.H.264-GRP"
	duneRelease = "Dune.2021.1080p.WEB-DL.DDP5.1.H.264-FLUX"
	showHash    = "8e245d9679d31e12"
	duneHash    = "61f7751fc2a72bfb"

	episodeVideo = "/tv/Show (2011)/Season 01/Show S01E02.mkv"
	movieVideo   = "/movies/Dune (2021)/Dune (2021).mkv"

	syncOffset = 1200 * time.Millisecond
)

var (
	start     = time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	srt       = []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n")
	betterSRT = []byte("1\n00:00:01,500 --> 00:00:02,500\nHello there\n")
)

// queued is a task and the earliest time it may run.
type queued struct {
	task backlite.Task
	at   time.Time
}

// queue records tasks instead of running them.
type queue struct {
	mu    sync.Mutex
	tasks []queued
}

func (q *queue) add(task backlite.Task, at time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, queued{task: task, at: at})
}

func (q *queue) Enqueue(_ context.Context, task backlite.Task) error {
	q.add(task, time.Time{})
	return nil
}

func (q *queue) EnqueueTx(_ *gorm.DB, task backlite.Task) error {
	q.add(task, time.Time{})
	return nil
}

func (q *queue) EnqueueTxAt(_ *gorm.DB, task backlite.Task, at time.Time) error {
	q.add(task, at)
	return nil
}

// takeDue removes and returns the tasks that may run at now.
func (q *queue) takeDue(now time.Time) []backlite.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	var due []backlite.Task
	q.tasks = slices.DeleteFunc(q.tasks, func(t queued) bool {
		if t.at.After(now) {
			return false
		}
		due = append(due, t.task)
		return true
	})
	return due
}

func (q *queue) all() []queued {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.tasks)
}

type recorder struct {
	mu     sync.Mutex
	events []bus.Event
}

func newRecorder(b *bus.Bus) *recorder {
	r := &recorder{}
	b.Subscribe(func(ev bus.Event) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.events = append(r.events, ev)
	})
	return r
}

func (r *recorder) all() []bus.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

func ofType[T any](t *testing.T, r *recorder, eventType string) []T {
	t.Helper()
	var out []T
	for _, ev := range r.all() {
		if ev.Type != eventType {
			continue
		}
		v, err := bus.Decode[T](ev)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

type harness struct {
	t        *testing.T
	ctx      context.Context
	db       *db.DB
	store    *settings.Store
	catalog  *catalogtest.Fake
	library  *librarytest.Fake
	os       *fakes.OpenSubtitles
	syncer   *fakes.Syncer
	sidecars *fakes.SidecarFiles
	service  *subtitles.Service
	queue    *queue
	events   *recorder
	now      time.Time

	show, movie catalogapi.Title
	episode     catalogapi.Episode
}

func newHarness(t *testing.T, global ...domain.Language) *harness {
	t.Helper()
	d := dbtest.Open(t, subtitles.Migrations())
	b := bus.New()
	imdb := "tt0944947"
	show := catalogapi.Title{
		ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindSeries, Name: "Show", Year: showYear,
		External: domain.ExternalIDs{Tmdb: showTmdb, Imdb: &imdb}, OriginalLanguage: english,
	}
	movie := catalogapi.Title{
		ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindMovie, Name: "Dune", Year: duneYear,
		External: domain.ExternalIDs{Tmdb: duneTmdb}, OriginalLanguage: english,
	}
	episode := catalogapi.Episode{
		ID: domain.NewID[domain.EpisodeID](), Title: show.ID, Season: 1, Number: 2, AiredSeason: 1, AiredNumber: 2,
	}
	var langs []catalogapi.WantedLanguage
	for _, l := range global {
		langs = append(langs, catalogapi.WantedLanguage{Language: l})
	}
	h := &harness{
		t: t, ctx: context.Background(), db: d, store: settings.NewStore(d),
		catalog: catalogtest.NewFake(catalogtest.Fixture{
			Titles: []catalogapi.Title{show, movie}, Episodes: []catalogapi.Episode{episode}, GlobalLanguages: langs,
		}),
		library: librarytest.NewFake(librarytest.Fixture{}),
		os:      fakes.NewOpenSubtitles(), syncer: fakes.NewSyncer(), sidecars: fakes.NewSidecarFiles(),
		queue: &queue{}, events: newRecorder(b), now: start,
		show: show, movie: movie, episode: episode,
	}
	h.service = subtitles.NewService(subtitles.Deps{
		DB: d, Repo: subtitles.NewRepository(d), Store: h.store, Catalog: h.catalog, Library: h.library,
		OpenSubtitles: h.os, Syncer: h.syncer, Sidecars: h.sidecars, Bus: b, Jobs: h.queue,
	})
	subtitles.SetClock(h.service, func() time.Time { return h.now })
	h.syncer.SetOffset(episodeVideo, syncOffset)
	h.syncer.SetOffset(movieVideo, syncOffset)
	return h
}

func (h *harness) advance(d time.Duration) { h.now = h.now.Add(d) }

// episodeFile is S01E02 of the show, as Library stores it.
func (h *harness) episodeFile(streams ...libraryapi.Stream) libraryapi.MediaFile {
	return libraryapi.MediaFile{
		ID: domain.NewID[domain.MediaFileID](), Title: h.show.ID, Path: episodeVideo, ReleaseName: showRelease,
		OSHash: showHash, Episodes: []domain.EpisodeRef{{Episode: h.episode.ID, Season: 1, Number: 2}},
		MediaInfo: libraryapi.MediaInfo{Streams: streams}, ImportedAt: h.now,
	}
}

func (h *harness) movieFile(streams ...libraryapi.Stream) libraryapi.MediaFile {
	return libraryapi.MediaFile{
		ID: domain.NewID[domain.MediaFileID](), Title: h.movie.ID, Path: movieVideo, ReleaseName: duneRelease,
		OSHash: duneHash, MediaInfo: libraryapi.MediaInfo{Streams: streams}, ImportedAt: h.now,
	}
}

func (h *harness) episodeQuery(lang domain.Language) subtitles.SearchQuery {
	return subtitles.SearchQuery{MovieHash: showHash, Tmdb: showTmdb, Imdb: "tt0944947", Season: 1, Episode: 2, Language: lang}
}

func (h *harness) movieQuery(lang domain.Language) subtitles.SearchQuery {
	return subtitles.SearchQuery{MovieHash: duneHash, Tmdb: duneTmdb, Language: lang}
}

// perfectEpisode is a hash-verified English subtitle for S01E02.
func perfectEpisode(fileID int64, lang domain.Language) subtitles.Candidate {
	return subtitles.Candidate{
		FileID: fileID, Language: lang, ReleaseName: showRelease, HashMatch: true,
		Tmdb: showTmdb, Year: showYear, Season: 1, Episode: 2,
	}
}

// imported stores the file in Library and delivers FileImported.
func (h *harness) imported(f libraryapi.MediaFile, sidecars ...libraryapi.Sidecar) {
	h.t.Helper()
	h.library.Put(librarytest.StoredFile{File: f})
	h.handle(subtitles.NewLibraryHandler(h.service), libraryapi.FileImported{
		MediaFile: f.ID, Title: f.Title, Episodes: f.Episodes, Path: f.Path, ReleaseName: f.ReleaseName,
		OSHash: f.OSHash, MediaInfo: f.MediaInfo, Sidecars: sidecars, Origin: domain.OriginDownload,
	})
}

func (h *harness) handle(handler bus.Handler, m domain.Message) {
	h.t.Helper()
	if err := subtitles.Handle(h.ctx, h.db, handler, m); err != nil {
		h.t.Fatal(err)
	}
}

// runDue runs queued tasks that are due, and the tasks they queue, until none is due.
func (h *harness) runDue() {
	h.t.Helper()
	for {
		due := h.queue.takeDue(h.now)
		if len(due) == 0 {
			return
		}
		for _, task := range due {
			if _, err := subtitles.RunTask(h.ctx, h.service, task); err != nil {
				h.t.Fatalf("task %T: %v", task, err)
			}
		}
	}
}

func (h *harness) sweep() {
	h.t.Helper()
	h.queue.add(subtitles.SweepTask(), time.Time{})
	h.runDue()
}

func (h *harness) view(file domain.MediaFileID) subtitles.FileView {
	h.t.Helper()
	v, err := h.service.File(h.ctx, file)
	if err != nil {
		h.t.Fatal(err)
	}
	return v
}

// coverage returns the status of each wanted track, keyed by language (forced tracks get a "+forced" suffix).
func (h *harness) coverage(file domain.MediaFileID) map[string]api.CoverageStatus {
	h.t.Helper()
	got, err := subtitles.NewRepository(h.db).Coverage(h.ctx, file)
	if err != nil {
		h.t.Fatal(err)
	}
	out := map[string]api.CoverageStatus{}
	for _, c := range got {
		key := string(c.Language)
		if c.Forced {
			key += "+forced"
		}
		out[key] = c.Status
	}
	return out
}

func (h *harness) searchesQueued(file domain.MediaFileID) int {
	return len(slices.DeleteFunc(h.queue.all(), func(q queued) bool { return !subtitles.IsSearch(q.task, file) }))
}
