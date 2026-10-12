package library_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/library/api/contracttest"
)

const migratedRelease = "Dune.2021.2160p.UHD.BluRay.REMUX.HDR.HEVC.Atmos-FGT"

var (
	migratedAddedAt = time.Date(2022, 1, 2, 3, 4, 5, 0, time.UTC)
	remux2160p      = domain.Quality{ID: 31, Name: "Remux-2160p", Source: "BLURAY", Resolution: 2160}
)

func TestServicePassesTheImporterContract(t *testing.T) {
	contracttest.RunImporter(t, func(t *testing.T, w contracttest.ImporterWorld) api.Importer {
		f := newImportFixture(t, twoDevices)
		f.catalog.PutTitle(catalogapi.Title{ID: w.Movie, Kind: domain.MediaKindMovie, Name: "Arrival"})
		f.catalog.PutTitle(catalogapi.Title{ID: w.Series, Kind: domain.MediaKindSeries, Name: "Severance"})
		f.catalog.PutEpisode(catalogapi.Episode{ID: w.Episode, Title: w.Series, Season: 1, Number: 1})
		for _, video := range w.Videos {
			f.libraryFile(t, video, fullRuntime)
		}
		return f.service
	})
}

func TestMigratedFileIsLinkedInPlaceWithItsReleaseNameQualityAndSidecars(t *testing.T) {
	f := newImportFixture(t, twoDevices)
	ctx := context.Background()
	dir := filepath.Join(f.layout.movies, "Dune (2021)")
	video := f.libraryFile(t, filepath.Join(dir, "Dune.mkv"), movieRuntime)
	subtitles := f.libraryFile(t, filepath.Join(dir, "Dune.en.srt"), 0)

	ids, err := f.service.ImportMediaFiles(ctx, []api.ImportMediaFile{{
		Source: "radarr:moviefile:7", Title: f.movie.ID, Path: video, Size: 1, Quality: remux2160p,
		ReleaseName: migratedRelease, AddedAt: migratedAddedAt,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ImportMediaFiles(ctx, []api.ImportMediaFile{{Source: "radarr:moviefile:7", Title: f.movie.ID, Path: video}}); err != nil {
		t.Fatal(err)
	}

	f.stat(t, video)
	imported := f.imported(t)
	if len(imported) != 1 {
		t.Fatalf("%d FileImported, want 1", len(imported))
	}
	got := imported[0]
	if got.MediaFile != ids["radarr:moviefile:7"] || got.Origin != domain.OriginMigration || got.Download != nil || got.Path != video {
		t.Errorf("FileImported = %+v, want the file linked in place with origin MIGRATION", got)
	}
	if got.Quality != remux2160p || got.ReleaseName != migratedRelease || got.OSHash == "" {
		t.Errorf("FileImported = %+v, want Remux-2160p from %s", got, migratedRelease)
	}
	if want := []api.Sidecar{{Path: subtitles, Language: "en"}}; !slices.Equal(got.Sidecars, want) {
		t.Errorf("sidecars = %+v, want %+v", got.Sidecars, want)
	}
	files, err := f.repo.FilesForTitle(ctx, f.movie.ID)
	if err != nil || len(files) != 1 || !files[0].ImportedAt.Equal(migratedAddedAt) {
		t.Errorf("files = %+v, %v; want one imported at %s", files, err, migratedAddedAt)
	}
}

func TestMigratedEpisodeFileCarriesTheEpisodesNumbering(t *testing.T) {
	f := newImportFixture(t, twoDevices)
	video := f.libraryFile(t, filepath.Join(f.layout.tv, "Show", "Season 01", "Show - S01E02.mkv"), fullRuntime)

	_, err := f.service.ImportMediaFiles(context.Background(), []api.ImportMediaFile{{
		Source: "sonarr:episodefile:3", Title: f.show.ID, Episodes: []domain.EpisodeID{f.episodes[1].ID}, Path: video, Quality: quality,
	}})
	if err != nil {
		t.Fatal(err)
	}
	imported := f.imported(t)
	want := []domain.EpisodeRef{{Episode: f.episodes[1].ID, Season: 1, Number: 2}}
	if len(imported) != 1 || !slices.Equal(imported[0].Episodes, want) {
		t.Fatalf("FileImported = %+v, want episodes %+v", imported, want)
	}
}

func TestAMigratedFileIsRenamedWithItsSidecarsWhenAsked(t *testing.T) {
	f := newImportFixture(t, twoDevices)
	dir := filepath.Join(f.layout.movies, "dune")
	video := f.libraryFile(t, filepath.Join(dir, "dune.mkv"), movieRuntime)
	f.libraryFile(t, filepath.Join(dir, "dune.en.srt"), 0)

	_, err := f.service.ImportMediaFiles(context.Background(), []api.ImportMediaFile{{
		Source: "radarr:moviefile:8", Title: f.movie.ID, Path: video, Quality: remux2160p, Rename: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(f.layout.movies, "Dune (2021) [tmdbid-438631]", "Dune (2021).mkv")
	f.stat(t, target)
	imported := f.imported(t)
	want := []api.Sidecar{{Path: filepath.Join(f.layout.movies, "Dune (2021) [tmdbid-438631]", "Dune (2021).en.srt"), Language: "en"}}
	if len(imported) != 1 || imported[0].Path != target || !slices.Equal(imported[0].Sidecars, want) {
		t.Fatalf("FileImported = %+v, want the renamed file at %s with its subtitle", imported, target)
	}
	if _, err := f.files.Stat(dir); err == nil {
		t.Errorf("the emptied folder %s is still there", dir)
	}
}

func TestImportedRootFolderMustBeReachable(t *testing.T) {
	f := newImportFixture(t, twoDevices)
	err := f.service.ImportRootFolders(context.Background(), []api.ImportRootFolder{{Source: "sonarr:rootfolder:2", Kind: domain.MediaKindSeries, Path: "/media/anime"}})
	if err == nil {
		t.Fatal("an unreachable root folder was imported")
	}
}
