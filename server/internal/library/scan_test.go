package library_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/library"
	"magpie/internal/library/api"
	"magpie/internal/library/api/contracttest"
)

const (
	duneFolder    = "Dune (2021) [tmdbid-438631]"
	duneVideo     = "Dune.2021.1080p.BluRay.x264-GRP.mkv"
	duneSubtitles = "Dune.2021.1080p.BluRay.x264-GRP.en.srt"
	showFolder    = "Show (2020) [tmdbid-1399]"
)

// libraryFile writes a video or subtitle that is already in the library, with the given runtime.
func (f *importFixture) libraryFile(t *testing.T, path string, runtime time.Duration) string {
	t.Helper()
	if err := f.files.MkdirAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := f.files.WriteFile(path, []byte("content of "+path)); err != nil {
		t.Fatal(err)
	}
	if runtime > 0 {
		f.prober.Set(path, api.MediaInfo{Duration: runtime, Container: "matroska", Width: 1920, Height: 1080})
	}
	return path
}

func (f *importFixture) scan(t *testing.T) {
	t.Helper()
	if _, err := f.service.StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.runJobs(t)
}

func (f *importFixture) foldersFound(t *testing.T) []api.FolderFound {
	t.Helper()
	var out []api.FolderFound
	for _, ev := range f.events.ofType(api.TypeFolderFound) {
		out = append(out, payload[api.FolderFound](t, ev))
	}
	return out
}

func (f *importFixture) matchFolder(t *testing.T, folder string, title domain.TitleID) {
	t.Helper()
	matched := catalogapi.ScanFolderMatched{Folder: folder, Title: title}
	if err := library.Handle(context.Background(), f.db, library.NewScanFolderMatchedHandler(f.service), matched); err != nil {
		t.Fatal(err)
	}
	f.runJobs(t)
}

// Seed library #15.
func TestAScannedFolderIsReportedThenLinkedInPlace(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	dir := filepath.Join(f.layout.movies, duneFolder)
	video := f.libraryFile(t, filepath.Join(dir, duneVideo), movieRuntime)
	subtitles := f.libraryFile(t, filepath.Join(dir, duneSubtitles), 0)

	f.scan(t)

	found := f.foldersFound(t)
	year := 2021
	want := api.FolderFound{Path: dir, Kind: domain.MediaKindMovie, ParsedTitle: "Dune", Year: &year, TmdbHint: ptr(duneTmdb)}
	if len(found) != 1 || found[0].Path != want.Path || found[0].Kind != want.Kind || found[0].ParsedTitle != want.ParsedTitle ||
		found[0].Year == nil || *found[0].Year != year || found[0].TmdbHint == nil || *found[0].TmdbHint != duneTmdb {
		t.Fatalf("FolderFound = %+v, want %+v", found, want)
	}

	f.matchFolder(t, dir, f.movie.ID)
	f.matchFolder(t, dir, f.movie.ID)

	f.stat(t, video)
	imported := f.imported(t)
	if len(imported) != 1 {
		t.Fatalf("%d FileImported, want 1", len(imported))
	}
	got := imported[0]
	if got.Origin != domain.OriginLibraryScan || got.Download != nil || got.Path != video || got.OSHash == "" || got.Title != f.movie.ID {
		t.Errorf("FileImported = %+v, want the file linked in place from a scan", got)
	}
	if want := []api.Sidecar{{Path: subtitles, Language: "en"}}; !slices.Equal(got.Sidecars, want) {
		t.Errorf("Sidecars = %+v, want %+v", got.Sidecars, want)
	}
	if got.Quality.Name != "Bluray-1080p" {
		t.Errorf("quality = %+v, want the name's Bluray-1080p", got.Quality)
	}
	files, err := f.repo.FilesForTitle(ctx, f.movie.ID)
	if err != nil || len(files) != 1 || files[0].InfoHash != "" || files[0].Download != nil {
		t.Errorf("media files = %+v, %v; want one row without a download", files, err)
	}
	if !slices.Contains(f.jellyfin.Refreshed(), dir) {
		t.Errorf("Jellyfin refreshed %v, want %s", f.jellyfin.Refreshed(), dir)
	}

	f.scan(t)
	if n := len(f.foldersFound(t)); n != 1 {
		t.Errorf("%d FolderFound after a rescan, want the linked folder left out", n)
	}
}

func TestAScanSkipsHiddenAndImportedFoldersAndReadsEveryHint(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	f.libraryFile(t, filepath.Join(f.layout.movies, ".magpie-staging", "x.mkv"), movieRuntime)
	f.libraryFile(t, filepath.Join(f.layout.movies, "Conclave (2024) {tmdb-974576}", "Conclave.mkv"), movieRuntime)
	f.libraryFile(t, filepath.Join(f.layout.movies, "Some.Movie.2019.1080p.WEB-DL", "movie.mkv"), movieRuntime)
	f.libraryFile(t, filepath.Join(f.layout.tv, "The Office (US)", "Season 1", "The.Office.S01E01.mkv"), fullRuntime)
	imported := f.importMovie(t)

	f.scan(t)

	found := f.foldersFound(t)
	paths := make([]string, 0, len(found))
	for _, ff := range found {
		paths = append(paths, ff.Path)
	}
	slices.Sort(paths)
	want := []string{
		filepath.Join(f.layout.movies, "Conclave (2024) {tmdb-974576}"),
		filepath.Join(f.layout.movies, "Some.Movie.2019.1080p.WEB-DL"),
		filepath.Join(f.layout.tv, "The Office (US)"),
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("folders found %v, want %v (not %s)", paths, want, filepath.Dir(imported.Path))
	}
	byPath := map[string]api.FolderFound{}
	for _, ff := range found {
		byPath[ff.Path] = ff
	}
	if c := byPath[want[0]]; c.TmdbHint == nil || *c.TmdbHint != 974576 || c.ParsedTitle != "Conclave" {
		t.Errorf("Conclave = %+v, want its {tmdb-N} hint", c)
	}
	if m := byPath[want[1]]; m.ParsedTitle != "Some Movie" || m.Year == nil || *m.Year != 2019 {
		t.Errorf("release-named folder = %+v, want title and year parsed", m)
	}
	if s := byPath[want[2]]; s.Kind != domain.MediaKindSeries || s.ParsedTitle != "The Office (US)" || s.Year != nil {
		t.Errorf("series folder = %+v, want the full name and no year", s)
	}
	status, err := f.service.ScanStatus(context.Background())
	if err != nil || status.Running || status.FinishedAt == nil || status.Found != len(want) {
		t.Errorf("status = %+v, %v; want a finished scan that found %d", status, err, len(want))
	}
}

func TestAMatchedFolderWithoutAPlayableVideoIsIgnored(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	dir := filepath.Join(f.layout.movies, duneFolder)
	f.libraryFile(t, filepath.Join(dir, "sample.mkv"), sampleRuntime)
	f.libraryFile(t, filepath.Join(dir, "notes.txt"), 0)
	f.scan(t)

	f.matchFolder(t, dir, f.movie.ID)

	if n := len(f.imported(t)); n != 0 {
		t.Errorf("%d FileImported, want none", n)
	}
	status, err := f.service.ScanStatus(context.Background())
	if err != nil || status.Ignored != 1 || status.Linked != 0 {
		t.Errorf("status = %+v, %v; want the folder ignored", status, err)
	}
}

func TestAScannedSeriesFolderLinksEachEpisode(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	dir := filepath.Join(f.layout.tv, "Show")
	for n := 1; n <= 3; n++ {
		f.libraryFile(t, filepath.Join(dir, "Season 1", fmt.Sprintf("Show.S01E%02d.720p.HDTV.x264-GRP.mkv", n)), fullRuntime)
	}
	f.libraryFile(t, filepath.Join(dir, "Extras", "Behind the scenes.mkv"), fullRuntime)
	f.scan(t)

	f.matchFolder(t, dir, f.show.ID)

	imported := f.imported(t)
	if len(imported) != 3 {
		t.Fatalf("%d FileImported, want 3", len(imported))
	}
	for i, ev := range imported {
		if len(ev.Episodes) != 1 || ev.Episodes[0].Episode != f.episodes[i].ID || ev.Quality.Name != "HDTV-720p" {
			t.Errorf("FileImported %d = %+v, want S01E%02d", i, ev, i+1)
		}
	}
}

func TestRenameOnScanMovesTheVideoAndItsSidecars(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	if err := f.service.SetScanOptions(ctx, library.ScanOptions{RenameMatched: true}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(f.layout.movies, "dune")
	video := f.libraryFile(t, filepath.Join(dir, duneVideo), movieRuntime)
	inode := f.stat(t, video).Inode
	f.libraryFile(t, filepath.Join(dir, duneSubtitles), 0)
	f.scan(t)

	f.matchFolder(t, dir, f.movie.ID)

	target := f.moviePath()
	if got := f.stat(t, target); got.Inode != inode {
		t.Errorf("renamed video inode %d, want %d (moved, not copied)", got.Inode, inode)
	}
	f.stat(t, filepath.Join(filepath.Dir(target), "Dune (2021).en.srt"))
	if f.files.Exists(video) {
		t.Error("the video is still at its old path")
	}
	if imported := f.imported(t); len(imported) != 1 || imported[0].Path != target {
		t.Errorf("FileImported = %+v, want the new path", imported)
	}
}

// seedEpisodeFile stores a show file for the given episodes at its conventional path, with an English sidecar.
func (f *importFixture) seedEpisodeFile(t *testing.T, episodes ...domain.EpisodeRef) api.MediaFile {
	t.Helper()
	first := episodes[0]
	path := filepath.Join(f.layout.tv, showFolder, fmt.Sprintf("Season %02d", first.Season), fmt.Sprintf("Show S%02dE%02d.mkv", first.Season, first.Number))
	f.libraryFile(t, path, fullRuntime)
	f.libraryFile(t, path[:len(path)-len(filepath.Ext(path))]+".en.srt", 0)
	file := api.MediaFile{
		ID: domain.NewID[domain.MediaFileID](), Title: f.show.ID, Episodes: episodes, RootFolder: f.layout.tv, Path: path,
		Size: 1, Quality: quality, Origin: domain.OriginDownload, ImportedAt: time.Now().UTC(),
	}
	if err := library.SeedFiles(context.Background(), f.db, []contracttest.StoredFile{{File: file, ETag: "etag"}}); err != nil {
		t.Fatal(err)
	}
	return file
}

func (f *importFixture) changeGroup(t *testing.T, moves ...catalogapi.EpisodeMove) {
	t.Helper()
	changed := catalogapi.EpisodeGroupChanged{Title: f.show.ID, GroupID: "group", Moves: moves}
	if err := library.Handle(context.Background(), f.db, library.NewEpisodeGroupChangedHandler(f.service), changed); err != nil {
		t.Fatal(err)
	}
	f.runJobs(t)
}

func episodeAt(id domain.EpisodeID, season, number int) domain.EpisodeRef {
	return domain.EpisodeRef{Episode: id, Season: season, Number: number}
}

// Seed library #14.
func TestAnEpisodeGroupChangeRenamesTheVideoAndItsSidecar(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	episode := domain.NewID[domain.EpisodeID]()
	file := f.seedEpisodeFile(t, episodeAt(episode, 2, 1))
	inode := f.stat(t, file.Path).Inode

	f.changeGroup(t, catalogapi.EpisodeMove{Episode: episode, FromSeason: 2, FromNo: 1, ToSeason: 1, ToNo: 11})

	newDir := filepath.Join(f.layout.tv, showFolder, "Season 01")
	newPath := filepath.Join(newDir, "Show S01E11.mkv")
	if got := f.stat(t, newPath); got.Inode != inode {
		t.Errorf("renamed inode %d, want %d", got.Inode, inode)
	}
	f.stat(t, filepath.Join(newDir, "Show S01E11.en.srt"))
	if f.files.Exists(file.Path) || f.files.Exists(filepath.Join(filepath.Dir(file.Path), "Show S02E01.en.srt")) {
		t.Error("the old video or sidecar is still there")
	}
	stored, err := f.repo.File(ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Path != newPath || !slices.Equal(stored.Episodes, []domain.EpisodeRef{episodeAt(episode, 1, 11)}) {
		t.Errorf("stored file = %+v, want the new path and S01E11", stored)
	}
	if !slices.Contains(f.jellyfin.Refreshed(), newDir) {
		t.Errorf("Jellyfin refreshed %v, want %s", f.jellyfin.Refreshed(), newDir)
	}
}

func TestSwappedEpisodesTradeNames(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	a, b := f.episodes[0].ID, f.episodes[1].ID
	first := f.seedEpisodeFile(t, episodeAt(a, 1, 1))
	second := f.seedEpisodeFile(t, episodeAt(b, 1, 2))
	firstContent, _ := f.files.ReadFile(first.Path)

	f.changeGroup(t,
		catalogapi.EpisodeMove{Episode: a, FromSeason: 1, FromNo: 1, ToSeason: 1, ToNo: 2},
		catalogapi.EpisodeMove{Episode: b, FromSeason: 1, FromNo: 2, ToSeason: 1, ToNo: 1},
	)

	if content, _ := f.files.ReadFile(second.Path); string(content) != string(firstContent) {
		t.Errorf("%s holds %q, want the first file's content", second.Path, content)
	}
	if stored, err := f.repo.File(ctx, first.ID); err != nil || stored.Path != second.Path {
		t.Errorf("first file = %+v, %v; want it at %s", stored, err, second.Path)
	}
}

func TestARenameThatWouldOverwriteAFileKeepsTheOldNameAndFlagsIt(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	episode := f.episodes[0].ID
	file := f.seedEpisodeFile(t, episodeAt(episode, 1, 1))
	stray := f.libraryFile(t, filepath.Join(f.layout.tv, showFolder, "Season 01", "Show S01E05.mkv"), fullRuntime)
	strayContent, _ := f.files.ReadFile(stray)

	f.changeGroup(t, catalogapi.EpisodeMove{Episode: episode, FromSeason: 1, FromNo: 1, ToSeason: 1, ToNo: 5})

	f.stat(t, file.Path)
	if content, _ := f.files.ReadFile(stray); string(content) != string(strayContent) {
		t.Error("the stray file was overwritten")
	}
	problems, err := f.service.RenameProblems(ctx, f.show.ID)
	if err != nil || problems[file.ID] == "" {
		t.Errorf("rename problems = %v, %v; want the file flagged", problems, err)
	}
	if stored, err := f.repo.File(ctx, file.ID); err != nil || stored.Path != file.Path {
		t.Errorf("stored file = %+v, %v; want the old path kept", stored, err)
	}
}

func ptr[T any](v T) *T { return &v }
