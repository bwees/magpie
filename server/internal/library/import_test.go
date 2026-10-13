package library_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	catalogapi "magpie/internal/catalog/api"
	catalogtest "magpie/internal/catalog/api/contracttest"
	"magpie/internal/domain"
	"magpie/internal/library"
	"magpie/internal/library/api"
	"magpie/internal/library/fakes"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	downloadsDevice uint64 = 2
	mediaDevice     uint64 = 3
	// remountedDevice is mediaDevice's share mounted again, as on another node.
	remountedDevice uint64 = 4

	duneTmdb     domain.TmdbID = 438631
	showTmdb     domain.TmdbID = 1399
	movieMinutes               = 155
	showMinutes                = 50
	showEpisodes               = 10

	movieRelease = "Dune.2021.1080p.WEB-DL.DDP5.1.H.264-FLUX"
	packRelease  = "Show.2020.S01.1080p.WEB-DL.DDP5.1.H.264-GRP"
	infoHash     = "3a929e0159bfe761ca2f4b0b3b8dfe0de806c9d9"

	sampleRuntime = 40 * time.Second
	fullRuntime   = 45 * time.Minute
)

var quality = domain.Quality{ID: 3, Name: "WEBDL-1080p", Source: "WEB", Resolution: 1080}

// layout is where downloads and the library live: one device for both, or one each.
type layout struct {
	downloads, movies, tv string
}

var (
	oneDevice  = layout{downloads: "/data/downloads", movies: "/data/media/movies", tv: "/data/media/tv"}
	twoDevices = layout{downloads: "/downloads", movies: "/media/movies", tv: "/media/tv"}
)

type importFixture struct {
	db        *db.DB
	files     *fakes.FileSystem
	prober    *fakes.MediaProber
	hasher    *fakes.Hasher
	jellyfin  *fakes.JellyfinLibrary
	downloads *fakes.DownloadLocation
	catalog   *catalogtest.Fake
	service   *library.Service
	repo      *library.Repository
	queue     *queue
	events    *recorder
	layout    layout
	movie     catalogapi.Title
	show      catalogapi.Title
	// episodes are the show's season 1, in order.
	episodes []catalogapi.Episode
}

func newImportFixture(t *testing.T, l layout) *importFixture {
	t.Helper()
	ctx := context.Background()
	files := fakes.NewFileSystem()
	if l == oneDevice {
		files.Mount("/data", mediaDevice)
	} else {
		files.Mount("/downloads", downloadsDevice)
		files.Mount("/media", mediaDevice)
	}
	for _, dir := range []string{l.downloads, l.movies, l.tv} {
		if err := files.MkdirAll(dir); err != nil {
			t.Fatal(err)
		}
	}

	movie := catalogapi.Title{
		ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindMovie, Name: "Dune", Year: 2021,
		External: domain.ExternalIDs{Tmdb: duneTmdb}, RuntimeMinutes: movieMinutes,
	}
	show := catalogapi.Title{
		ID: domain.NewID[domain.TitleID](), Kind: domain.MediaKindSeries, Name: "Show", Year: 2020,
		External: domain.ExternalIDs{Tmdb: showTmdb}, RuntimeMinutes: showMinutes,
	}
	var episodes []catalogapi.Episode
	for n := 1; n <= showEpisodes; n++ {
		episodes = append(episodes, catalogapi.Episode{
			ID: domain.NewID[domain.EpisodeID](), Title: show.ID, Season: 1, Number: n,
			AiredSeason: 1, AiredNumber: n, RuntimeMinutes: showMinutes, Monitored: true, Released: true,
		})
	}

	d := dbtest.Open(t, library.Migrations())
	b := bus.New()
	f := &importFixture{
		db: d, files: files, prober: fakes.NewMediaProber(), hasher: fakes.NewHasher(files), jellyfin: fakes.NewJellyfinLibrary(),
		downloads: fakes.NewDownloadLocation(),
		catalog:   catalogtest.NewFake(catalogtest.Fixture{Titles: []catalogapi.Title{movie, show}, Episodes: episodes}),
		repo:      library.NewRepository(d), queue: &queue{}, events: newRecorder(b), layout: l,
		movie: movie, show: show, episodes: episodes,
	}
	f.service = library.NewService(library.Deps{
		DB: d, Repo: f.repo, Catalog: f.catalog, Files: files, Prober: f.prober, Hasher: f.hasher,
		Downloads: f.downloads, Bus: b, Jobs: f.queue,
		Jellyfin: f.jellyfin, Store: settings.NewStore(d),
	})
	for kind, path := range map[domain.MediaKind]string{domain.MediaKindMovie: l.movies, domain.MediaKindSeries: l.tv} {
		if _, err := f.service.AddRootFolder(ctx, kind, path); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// download writes a video of the given runtime into the downloads folder.
func (f *importFixture) download(t *testing.T, rel string, runtime time.Duration) string {
	t.Helper()
	path := filepath.Join(f.layout.downloads, rel)
	if err := f.files.MkdirAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := f.files.WriteFile(path, []byte("content of "+rel)); err != nil {
		t.Fatal(err)
	}
	if runtime > 0 {
		f.prober.Set(path, api.MediaInfo{Duration: runtime, Container: "matroska"})
	}
	return path
}

func (f *importFixture) movieDownload(mode acquisitionapi.SeedingMode, content string, files ...string) acquisitionapi.DownloadCompleted {
	return f.completed(f.movie.ID, nil, movieRelease, mode, content, files...)
}

func (f *importFixture) completed(title domain.TitleID, episodes []domain.EpisodeID, release string, mode acquisitionapi.SeedingMode, content string, files ...string) acquisitionapi.DownloadCompleted {
	c := acquisitionapi.DownloadCompleted{
		Download: domain.NewID[domain.DownloadID](), Title: title, Episodes: episodes,
		ContentPath: filepath.Join(f.layout.downloads, content), ReleaseName: release, InfoHash: infoHash,
		Quality: quality, SeedingMode: mode, Files: []acquisitionapi.DownloadFile{},
	}
	for _, file := range files {
		c.Files = append(c.Files, acquisitionapi.DownloadFile{Path: file, Size: 1})
	}
	return c
}

// deliver hands DownloadCompleted to Library's handler and runs the import jobs it queued.
func (f *importFixture) deliver(t *testing.T, c acquisitionapi.DownloadCompleted) {
	t.Helper()
	ctx := context.Background()
	if err := library.Handle(ctx, f.db, library.NewDownloadCompletedHandler(f.service), c); err != nil {
		t.Fatal(err)
	}
	f.runJobs(t)
}

func (f *importFixture) runJobs(t *testing.T) {
	t.Helper()
	for _, task := range f.queue.drain() {
		ran, err := library.RunTask(context.Background(), f.service, task, func(next backlite.Task, _ time.Duration) {
			f.queue.add(next)
		})
		if !ran {
			t.Fatalf("unexpected task %T", task)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func (f *importFixture) imported(t *testing.T) []api.FileImported {
	t.Helper()
	var out []api.FileImported
	for _, ev := range f.events.ofType(api.TypeFileImported) {
		out = append(out, payload[api.FileImported](t, ev))
	}
	return out
}

func (f *importFixture) imp(t *testing.T, id domain.DownloadID) library.Import {
	t.Helper()
	imp, err := f.repo.Import(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return imp
}

func (f *importFixture) moviePath() string {
	return filepath.Join(f.layout.movies, "Dune (2021) [tmdbid-438631]", "Dune (2021).mkv")
}

func (f *importFixture) stat(t *testing.T, path string) library.FileInfo {
	t.Helper()
	info, err := f.files.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info
}

// Seed library #1.
func TestASeedingMovieOnOneDeviceIsHardlinked(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	source := f.download(t, "Dune.2021.1080p/Dune.2021.1080p.mkv", 2*time.Hour)
	c := f.movieDownload(acquisitionapi.SeedUntilGoals, "Dune.2021.1080p", "Dune.2021.1080p/Dune.2021.1080p.mkv")

	f.deliver(t, c)

	if src, dst := f.stat(t, source), f.stat(t, f.moviePath()); src.Inode != dst.Inode {
		t.Errorf("library file inode %d, source %d: want a hardlink", dst.Inode, src.Inode)
	}
	if n := f.files.Called(fakes.FileSystemCopy); n != 0 {
		t.Errorf("Copy called %d times on one device", n)
	}
	events := f.imported(t)
	if len(events) != 1 {
		t.Fatalf("%d FileImported, want 1", len(events))
	}
	wantHash, _ := f.hasher.OSHash(f.moviePath())
	got := events[0]
	if got.OSHash == "" || got.OSHash != wantHash || got.ReleaseName != movieRelease || got.Path != f.moviePath() ||
		got.Origin != domain.OriginDownload || got.Download == nil || *got.Download != c.Download || len(got.Episodes) != 0 {
		t.Errorf("FileImported = %+v", got)
	}
	file, err := f.repo.File(context.Background(), got.MediaFile)
	if err != nil {
		t.Fatal(err)
	}
	if file.InfoHash != infoHash || file.OSHash != wantHash || file.ReleaseName != movieRelease || file.RootFolder != f.layout.movies {
		t.Errorf("stored file = %+v", file)
	}
	if imp := f.imp(t, c.Download); imp.Status != library.ImportImported {
		t.Errorf("import status = %s", imp.Status)
	}
}

// Seed library #2.
func TestASeedingMovieAcrossDevicesIsCopied(t *testing.T) {
	f := newImportFixture(t, twoDevices)
	source := f.download(t, "Dune.2021.1080p.mkv", 2*time.Hour)

	f.deliver(t, f.movieDownload(acquisitionapi.SeedUntilGoals, "Dune.2021.1080p.mkv"))

	src, dst := f.stat(t, source), f.stat(t, f.moviePath())
	if dst.Device != mediaDevice || src.Inode == dst.Inode {
		t.Errorf("library file %+v, source %+v: want a copy on the media device", dst, src)
	}
	if n := f.files.Called(fakes.FileSystemCopy); n != 1 {
		t.Errorf("Copy called %d times, want once", n)
	}
	if len(f.imported(t)) != 1 {
		t.Error("want one FileImported")
	}
}

// Seed library #3.
func TestADontSeedMovieIsMoved(t *testing.T) {
	f := newImportFixture(t, twoDevices)
	source := f.download(t, "Dune.2021.1080p/Dune.2021.1080p.mkv", 2*time.Hour)

	f.deliver(t, f.movieDownload(acquisitionapi.DontSeed, "Dune.2021.1080p", "Dune.2021.1080p/Dune.2021.1080p.mkv"))

	if f.files.Exists(source) {
		t.Error("the source still exists after a move")
	}
	f.stat(t, f.moviePath())
	if len(f.imported(t)) != 1 {
		t.Error("want one FileImported")
	}
}

func (f *importFixture) packFiles(t *testing.T, folder string) []string {
	t.Helper()
	var rels []string
	for _, e := range f.episodes {
		rel := fmt.Sprintf("%s/Show.2020.S01E%02d.1080p.WEB-DL-GRP.mkv", folder, e.Number)
		f.download(t, rel, fullRuntime)
		rels = append(rels, rel)
	}
	return rels
}

func (f *importFixture) episodeIDs() []domain.EpisodeID {
	ids := make([]domain.EpisodeID, 0, len(f.episodes))
	for _, e := range f.episodes {
		ids = append(ids, e.ID)
	}
	return ids
}

// Seed library #4.
func TestASeasonPackImportsEachEpisodeAndSkipsTheSample(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	folder := "Show.2020.S01.1080p"
	rels := f.packFiles(t, folder)
	sample := folder + "/sample.mkv"
	f.download(t, sample, sampleRuntime)

	f.deliver(t, f.completed(f.show.ID, f.episodeIDs(), packRelease, acquisitionapi.SeedUntilGoals, folder, append(rels, sample)...))

	events := f.imported(t)
	if len(events) != showEpisodes {
		t.Fatalf("%d FileImported, want %d", len(events), showEpisodes)
	}
	for _, e := range f.episodes {
		path := filepath.Join(f.layout.tv, "Show (2020) [tmdbid-1399]", "Season 01", fmt.Sprintf("Show S01E%02d.mkv", e.Number))
		f.stat(t, path)
		i := slices.IndexFunc(events, func(ev api.FileImported) bool { return ev.Path == path })
		if i < 0 || len(events[i].Episodes) != 1 || events[i].Episodes[0].Episode != e.ID {
			t.Errorf("no FileImported for %s mapped to E%02d", path, e.Number)
		}
	}
	for _, ev := range events {
		if filepath.Base(ev.Path) == "sample.mkv" {
			t.Error("the sample was imported")
		}
	}
}

// Seed library #5.
func TestAPackWithAnUnmappableVideoImportsNothing(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	folder := "Show.2020.S01.1080p"
	rels := f.packFiles(t, folder)
	extra := folder + "/Behind.The.Scenes.mkv"
	f.download(t, extra, fullRuntime)
	c := f.completed(f.show.ID, f.episodeIDs(), packRelease, acquisitionapi.SeedUntilGoals, folder, append(rels, extra)...)

	f.deliver(t, c)

	if n := len(f.imported(t)); n != 0 {
		t.Errorf("%d FileImported, want none", n)
	}
	if f.files.Exists(filepath.Join(f.layout.tv, "Show (2020) [tmdbid-1399]")) {
		t.Error("files were placed for a partial import")
	}
	imp := f.imp(t, c.Download)
	if imp.Status != library.ImportManual {
		t.Errorf("import status = %s, want MANUAL", imp.Status)
	}
	failed := f.events.ofType(api.TypeImportFailed)
	if len(failed) != 1 {
		t.Fatalf("%d ImportFailed, want 1", len(failed))
	}
	if got := payload[api.ImportFailed](t, failed[0]); got.Download != c.Download || got.Retryable || !got.Manual || got.Reason == "" {
		t.Errorf("ImportFailed = %+v, want manual and not retryable", got)
	}
	i := slices.IndexFunc(imp.Files, func(file library.ImportFile) bool { return file.Path == extra })
	if i < 0 || imp.Files[i].Kind != library.FileVideo || len(imp.Files[i].Episodes) != 0 {
		t.Errorf("import files = %+v, want the unmapped video listed", imp.Files)
	}
	first := slices.IndexFunc(imp.Files, func(file library.ImportFile) bool { return file.Path == rels[0] })
	if first < 0 || len(imp.Files[first].Episodes) != 1 || imp.Files[first].Episodes[0].Episode != f.episodes[0].ID {
		t.Errorf("import files = %+v, want the mapped videos' episodes kept for the picker", imp.Files)
	}
}

// Seed library #6.
func TestARedeliveredDownloadCompletedImportsOnce(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	f.download(t, "Dune.2021.1080p.mkv", 2*time.Hour)
	c := f.movieDownload(acquisitionapi.SeedUntilGoals, "Dune.2021.1080p.mkv")

	f.deliver(t, c)
	f.deliver(t, c)

	if n := len(f.imported(t)); n != 1 {
		t.Errorf("%d FileImported, want 1", n)
	}
	files, err := f.repo.FilesForTitle(context.Background(), f.movie.ID)
	if err != nil || len(files) != 1 {
		t.Errorf("media files = %d, %v; want one row", len(files), err)
	}
}

// Seed library #8.
func TestReleaseSubtitlesBecomeSidecarsAndExtrasAreDropped(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	folder := "Dune.2021.1080p"
	f.download(t, folder+"/Movie.mkv", 2*time.Hour)
	for _, extra := range []string{"Movie.en.srt", "Movie.nfo", "Movie.srt"} {
		f.download(t, folder+"/"+extra, 0)
	}

	f.deliver(t, f.movieDownload(acquisitionapi.SeedUntilGoals, folder,
		folder+"/Movie.mkv", folder+"/Movie.en.srt", folder+"/Movie.nfo", folder+"/Movie.srt"))

	dir := filepath.Dir(f.moviePath())
	sidecar := filepath.Join(dir, "Dune (2021).en.srt")
	f.stat(t, sidecar)
	if f.files.Exists(filepath.Join(dir, "Movie.nfo")) || f.files.Exists(filepath.Join(dir, "Dune (2021).nfo")) {
		t.Error("the nfo was copied")
	}
	events := f.imported(t)
	if len(events) != 1 {
		t.Fatalf("%d FileImported, want 1", len(events))
	}
	want := []api.Sidecar{{Path: sidecar, Language: "en"}}
	if !slices.Equal(events[0].Sidecars, want) {
		t.Errorf("Sidecars = %+v, want %+v", events[0].Sidecars, want)
	}
}

func TestARetryUsesThePathMappingSetSinceTheDownloadFinished(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	const qbittorrentDownloads = "/qbittorrent/downloads"
	f.download(t, "Dune.2021.1080p.mkv", 2*time.Hour)
	c := f.movieDownload(acquisitionapi.SeedUntilGoals, "Dune.2021.1080p.mkv")
	c.ContentPath = filepath.Join(qbittorrentDownloads, "Dune.2021.1080p.mkv")
	f.deliver(t, c)
	for f.imp(t, c.Download).Status == library.ImportPending {
		f.runJobs(t)
	}

	f.downloads.Map(qbittorrentDownloads, f.layout.downloads)
	if _, err := f.service.RetryImport(ctx, c.Download); err != nil {
		t.Fatal(err)
	}
	f.runJobs(t)

	if imp := f.imp(t, c.Download); imp.Status != library.ImportImported {
		t.Errorf("import = %+v, want IMPORTED after the mapping was fixed", imp)
	}
	if len(f.imported(t)) != 1 {
		t.Errorf("%d FileImported, want 1", len(f.imported(t)))
	}
}

func TestAFileErrorIsRetriedThenFailsTheImport(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	f.download(t, "Dune.2021.1080p.mkv", 2*time.Hour)
	f.files.FailAlways(fakes.FileSystemHardlink, errors.New("disk on fire"))
	c := f.movieDownload(acquisitionapi.SeedUntilGoals, "Dune.2021.1080p.mkv")

	f.deliver(t, c)
	for range 3 {
		if imp := f.imp(t, c.Download); imp.Status != library.ImportPending {
			t.Fatalf("status %s before the retries ran out", imp.Status)
		}
		f.runJobs(t)
	}

	imp := f.imp(t, c.Download)
	if imp.Status != library.ImportFailed || imp.LastError == "" {
		t.Errorf("import = %+v, want FAILED with the error", imp)
	}
	failed := f.events.ofType(api.TypeImportFailed)
	if len(failed) != 1 {
		t.Fatalf("%d ImportFailed, want 1", len(failed))
	}
	if got := payload[api.ImportFailed](t, failed[0]); got.Manual {
		t.Errorf("ImportFailed = %+v, want a failed import not to wait for the user", got)
	}
}

// Seed library #5, the failed part.
func TestAPackWithOnlyASampleFailsWithoutWaitingForTheUser(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	f.download(t, "Dune.2021.1080p/sample.mkv", sampleRuntime)
	c := f.movieDownload(acquisitionapi.SeedUntilGoals, "Dune.2021.1080p", "Dune.2021.1080p/sample.mkv")

	f.deliver(t, c)

	if imp := f.imp(t, c.Download); imp.Status != library.ImportFailed {
		t.Errorf("import status = %s, want FAILED", imp.Status)
	}
	failed := f.events.ofType(api.TypeImportFailed)
	if len(failed) != 1 {
		t.Fatalf("%d ImportFailed, want 1", len(failed))
	}
	if got := payload[api.ImportFailed](t, failed[0]); got.Manual || got.Retryable {
		t.Errorf("ImportFailed = %+v, want neither manual nor retryable", got)
	}
}

// Seed library #20.
func TestRemovingAManualImportTellsAcquisitionOnce(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	folder := "Show.2020.S01.1080p"
	rels := f.packFiles(t, folder)
	extra := folder + "/Behind.The.Scenes.mkv"
	f.download(t, extra, fullRuntime)
	c := f.completed(f.show.ID, f.episodeIDs(), packRelease, acquisitionapi.SeedUntilGoals, folder, append(rels, extra)...)
	f.deliver(t, c)
	ctx := context.Background()

	if err := f.service.RemoveImport(ctx, c.Download); err != nil {
		t.Fatal(err)
	}

	if imp := f.imp(t, c.Download); imp.Status != library.ImportRemoved {
		t.Errorf("import status = %s, want REMOVED", imp.Status)
	}
	queue, err := f.service.Imports(ctx, []library.ImportStatus{library.ImportFailed, library.ImportManual})
	if err != nil || len(queue) != 0 {
		t.Errorf("imports queue = %+v, %v; want it empty", queue, err)
	}
	if err := f.service.RemoveImport(ctx, c.Download); !errors.Is(err, library.ErrImportNotStopped) {
		t.Errorf("second remove = %v, want ErrImportNotStopped", err)
	}
	removed := f.events.ofType(api.TypeImportRemoved)
	if len(removed) != 1 {
		t.Fatalf("%d ImportRemoved, want 1", len(removed))
	}
	want := api.ImportRemoved{Download: c.Download, Title: f.show.ID, ReleaseName: packRelease}
	if got := payload[api.ImportRemoved](t, removed[0]); got != want {
		t.Errorf("ImportRemoved = %+v, want %+v", got, want)
	}
}

func TestAManualMappingImportsTheChosenEpisodes(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	folder := "Show.2020.S01.1080p"
	odd := folder + "/Episode.One.mkv"
	f.download(t, odd, fullRuntime)
	c := f.completed(f.show.ID, f.episodeIDs()[:2], packRelease, acquisitionapi.SeedUntilGoals, folder, odd, folder+"/Episode.Two.mkv")
	f.download(t, folder+"/Episode.Two.mkv", fullRuntime)
	f.deliver(t, c)
	if imp := f.imp(t, c.Download); imp.Status != library.ImportManual {
		t.Fatalf("status %s, want MANUAL", imp.Status)
	}

	_, err := f.service.ImportManually(context.Background(), c.Download, []library.FileMapping{{Path: odd, Episodes: []domain.EpisodeID{f.episodes[0].ID}}})
	if err != nil {
		t.Fatal(err)
	}
	f.runJobs(t)

	events := f.imported(t)
	if len(events) != 1 || events[0].Episodes[0].Episode != f.episodes[0].ID {
		t.Fatalf("FileImported = %+v, want only S01E01", events)
	}
	f.stat(t, filepath.Join(f.layout.tv, "Show (2020) [tmdbid-1399]", "Season 01", "Show S01E01.mkv"))
}

func TestRootFoldersMustExistAndReportTheHardlinkCheck(t *testing.T) {
	f := newImportFixture(t, twoDevices)
	ctx := context.Background()
	if _, err := f.service.AddRootFolder(ctx, domain.MediaKindMovie, "/media/missing"); !errors.Is(err, library.ErrInvalidRootFolder) {
		t.Errorf("missing folder err = %v", err)
	}
	if _, err := f.service.AddRootFolder(ctx, domain.MediaKindMovie, f.layout.movies); !errors.Is(err, library.ErrRootFolderExists) {
		t.Errorf("duplicate err = %v", err)
	}

	check, err := f.service.HardlinkCheck(ctx)
	if err != nil || check.Problem == "" {
		t.Errorf("check = %+v, %v; want a problem while qBittorrent's path is unknown", check, err)
	}
}

// queue records enqueued tasks; tests run them.
type queue struct {
	mu    sync.Mutex
	tasks []backlite.Task
}

func (q *queue) add(task backlite.Task) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tasks = append(q.tasks, task)
}

func (q *queue) drain() []backlite.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	tasks := q.tasks
	q.tasks = nil
	return tasks
}

func (q *queue) Enqueue(_ context.Context, task backlite.Task) error {
	q.add(task)
	return nil
}

func (q *queue) EnqueueTx(_ *gorm.DB, task backlite.Task) error {
	q.add(task)
	return nil
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

func (r *recorder) ofType(eventType string) []bus.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(r.events), func(ev bus.Event) bool { return ev.Type != eventType })
}

func payload[T any](t *testing.T, ev bus.Event) T {
	t.Helper()
	v, err := bus.Decode[T](ev)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
