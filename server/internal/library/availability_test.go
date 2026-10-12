package library_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/library/fakes"
)

var errJellyfinDown = errors.New("jellyfin: connection refused")

// importMovie imports Dune and returns its FileImported.
func (f *importFixture) importMovie(t *testing.T) api.FileImported {
	t.Helper()
	f.download(t, "Dune.2021.1080p.mkv", 2*time.Hour)
	f.deliver(t, f.movieDownload(acquisitionapi.SeedUntilGoals, "Dune.2021.1080p.mkv"))
	imported := f.imported(t)
	if len(imported) != 1 {
		t.Fatalf("%d FileImported, want 1", len(imported))
	}
	return imported[0]
}

func (f *importFixture) available(t *testing.T) []api.ItemAvailable {
	t.Helper()
	var out []api.ItemAvailable
	for _, ev := range f.events.ofType(api.TypeItemAvailable) {
		out = append(out, payload[api.ItemAvailable](t, ev))
	}
	return out
}

func TestAnImportedMovieIsAvailableAtOnce(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()

	imported := f.importMovie(t)

	got := f.available(t)
	if len(got) != 1 || got[0].Title != f.movie.ID || len(got[0].Episodes) != 0 {
		t.Errorf("ItemAvailable = %+v, want the movie once", got)
	}
	if file, err := f.repo.File(ctx, imported.MediaFile); err != nil || !file.Available {
		t.Errorf("file = %+v, %v; want available", file, err)
	}
}

func TestImportedEpisodeFilesMakeTheirEpisodesAvailable(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	folder := "Show.2020.S01.1080p"

	f.deliver(t, f.completed(f.show.ID, f.episodeIDs(), packRelease, acquisitionapi.SeedUntilGoals, folder, f.packFiles(t, folder)...))

	var want []domain.EpisodeID
	for _, imported := range f.imported(t) {
		for _, ref := range imported.Episodes {
			want = append(want, ref.Episode)
		}
	}
	var got []domain.EpisodeID
	for _, available := range f.available(t) {
		got = append(got, available.Episodes...)
	}
	if !slices.Equal(got, want) {
		t.Errorf("ItemAvailable episodes = %v, want %v", got, want)
	}
	episodes, err := f.repo.AvailableEpisodes(ctx, f.show.ID)
	if err != nil || len(episodes) != len(want) {
		t.Errorf("AvailableEpisodes = %v, %v; want %d", episodes, err, len(want))
	}
}

func TestEachImportRefreshesItsFoldersOnce(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	folder := "Show.2020.S01.1080p"

	f.deliver(t, f.completed(f.show.ID, f.episodeIDs(), packRelease, acquisitionapi.SeedUntilGoals, folder, f.packFiles(t, folder)...))

	want := []string{filepath.Join(f.layout.tv, "Show (2020) [tmdbid-1399]", "Season 01")}
	if got := f.jellyfin.Refreshed(); !slices.Equal(got, want) {
		t.Errorf("refreshed %v, want %v", got, want)
	}
}

// Seed library #16.
func TestARefreshErrorStillLeavesTheImportAvailable(t *testing.T) {
	f := newImportFixture(t, oneDevice)
	ctx := context.Background()
	f.jellyfin.FailAlways(fakes.JellyfinLibraryRefreshPath, errJellyfinDown)

	imported := f.importMovie(t)

	if file, err := f.repo.File(ctx, imported.MediaFile); err != nil || !file.Available {
		t.Errorf("file = %+v, %v; want committed and available", file, err)
	}
	if n := f.jellyfin.Called(fakes.JellyfinLibraryRefreshPath); n != 1 {
		t.Errorf("RefreshPath called %d times, want once", n)
	}
	if len(f.available(t)) != 1 {
		t.Errorf("ItemAvailable = %+v, want once", f.available(t))
	}
}
