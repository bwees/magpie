// Package contracttest holds the library.Reader contract suite and the in-memory Fake that passes it.
// The real repository runs the same suite in its own tests.
package contracttest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/library/api"
)

// StoredFile is one media_files row: the file as the Reader returns it, plus the ETag of the file on disk.
type StoredFile struct {
	File api.MediaFile
	ETag string
}

// Fixture is the stored state a Reader starts with.
type Fixture struct {
	Files []StoredFile
}

// Factory returns a Reader holding exactly the fixture's state.
type Factory func(t *testing.T, f Fixture) api.Reader

const (
	moviesRoot   = "/media/movies"
	tvRoot       = "/media/tv"
	movieETag    = "100-2000-1700000000"
	seriesSeason = 1
	// replacedMovieETag is the movie file the upgrade replaced.
	replacedMovieETag = "99-1900-1600000000"
	// Seed 19: a series with 10 episodes, 7 with files and 6 of those confirmed in Jellyfin.
	seriesEpisodes   = 10
	episodesWithFile = 7
	episodesInJelly  = 6
	episodeSize      = 1 << 30
)

var importedAt = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

// Run checks a Reader against the library contract.
func Run(t *testing.T, newReader Factory) {
	ctx := context.Background()

	movie := domain.NewID[domain.TitleID]()
	oldMovieFile := movieFile(movie, importedAt)
	newMovieFile := movieFile(movie, importedAt.Add(time.Hour))

	series := domain.NewID[domain.TitleID]()
	episodes := make([]domain.EpisodeRef, 0, seriesEpisodes)
	for number := 1; number <= seriesEpisodes; number++ {
		episodes = append(episodes, domain.EpisodeRef{Episode: domain.NewID[domain.EpisodeID](), Season: seriesSeason, Number: number})
	}
	fixture := Fixture{Files: []StoredFile{
		{File: oldMovieFile, ETag: replacedMovieETag},
		{File: newMovieFile, ETag: movieETag},
	}}
	for i, ep := range episodes[:episodesWithFile] {
		file := episodeFile(series, ep, importedAt.Add(time.Duration(i)*time.Minute), i < episodesInJelly)
		fixture.Files = append(fixture.Files, StoredFile{File: file, ETag: fmt.Sprintf("etag-%d", i)})
	}
	withoutFiles := domain.NewID[domain.TitleID]()

	t.Run("File returns a stored file and ErrNotFound for an unknown ID", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.File(ctx, newMovieFile.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Path != newMovieFile.Path || got.OSHash != newMovieFile.OSHash || got.Origin != newMovieFile.Origin {
			t.Errorf("File = %+v, want %+v", got, newMovieFile)
		}
		if _, err := r.File(ctx, domain.NewID[domain.MediaFileID]()); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("unknown file err = %v, want ErrNotFound", err)
		}
	})

	t.Run("FilesForTitle lists newest first and is empty for a title without files", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.FilesForTitle(ctx, movie)
		if err != nil {
			t.Fatal(err)
		}
		assertFileIDs(t, got, newMovieFile.ID, oldMovieFile.ID)

		got, err = r.FilesForTitle(ctx, withoutFiles)
		if err != nil || len(got) != 0 {
			t.Errorf("FilesForTitle(no files) = %v, %v; want empty, nil", got, err)
		}
	})

	t.Run("FileForEpisode finds the covering file and ErrNotFound for an episode without one", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.FileForEpisode(ctx, episodes[0].Episode)
		if err != nil {
			t.Fatal(err)
		}
		if got.Title != series || !slices.Contains(got.Episodes, episodes[0]) {
			t.Errorf("FileForEpisode = %+v", got)
		}
		if _, err := r.FileForEpisode(ctx, episodes[seriesEpisodes-1].Episode); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("episode without a file err = %v, want ErrNotFound", err)
		}
	})

	t.Run("SourceForProcessing returns the ETag and a staging dir under the file's root", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.SourceForProcessing(ctx, newMovieFile.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Path != newMovieFile.Path || got.ETag != movieETag || got.Size != newMovieFile.Size {
			t.Errorf("SourceForProcessing = %+v", got)
		}
		if got.StagingDir == "" || !strings.HasPrefix(got.StagingDir, newMovieFile.RootFolder+string(filepath.Separator)) {
			t.Errorf("StagingDir %q is not under root %q", got.StagingDir, newMovieFile.RootFolder)
		}
		if _, err := r.SourceForProcessing(ctx, domain.NewID[domain.MediaFileID]()); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("unknown file err = %v, want ErrNotFound", err)
		}
	})

	t.Run("SourceForProcessing gives each file the ETag Library stored for it, which a swap compares", func(t *testing.T) {
		r := newReader(t, fixture)
		replaced, err := r.SourceForProcessing(ctx, oldMovieFile.ID)
		if err != nil {
			t.Fatal(err)
		}
		current, err := r.SourceForProcessing(ctx, newMovieFile.ID)
		if err != nil {
			t.Fatal(err)
		}
		if replaced.ETag != replacedMovieETag || current.ETag != movieETag || replaced.StagingDir != current.StagingDir {
			t.Errorf("replaced %+v, current %+v: want their own ETags and one staging dir per root", replaced, current)
		}
	})

	t.Run("AvailableEpisodes returns the title's confirmed episodes", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.AvailableEpisodes(ctx, series)
		if err != nil {
			t.Fatal(err)
		}
		want := make([]domain.EpisodeID, 0, episodesInJelly)
		for _, ep := range episodes[:episodesInJelly] {
			want = append(want, ep.Episode)
		}
		if !slices.Equal(got, want) {
			t.Errorf("AvailableEpisodes = %v, want %v", got, want)
		}
		got, err = r.AvailableEpisodes(ctx, withoutFiles)
		if err != nil || len(got) != 0 {
			t.Errorf("AvailableEpisodes(no files) = %v, %v; want empty, nil", got, err)
		}
	})

	t.Run("Coverage counts files and confirmations, zero for titles without files", func(t *testing.T) {
		r := newReader(t, fixture)
		got, err := r.Coverage(ctx, []domain.TitleID{series, withoutFiles})
		if err != nil {
			t.Fatal(err)
		}
		want := []api.TitleCoverage{
			{Title: series, EpisodesWithFile: episodesWithFile, EpisodesAvailable: episodesInJelly, SizeBytes: episodesWithFile * episodeSize},
			{Title: withoutFiles},
		}
		if !slices.Equal(got, want) {
			t.Errorf("Coverage = %+v, want %+v", got, want)
		}
	})
}

func movieFile(title domain.TitleID, at time.Time) api.MediaFile {
	id := domain.NewID[domain.MediaFileID]()
	return api.MediaFile{
		ID:         id,
		Title:      title,
		Episodes:   []domain.EpisodeRef{},
		RootFolder: moviesRoot,
		Path:       filepath.Join(moviesRoot, "Dune (2021) [tmdbid-438631]", string(id)+".mkv"),
		Size:       episodeSize,
		OSHash:     string(id),
		Origin:     domain.OriginDownload,
		ImportedAt: at,
	}
}

func episodeFile(title domain.TitleID, ep domain.EpisodeRef, at time.Time, available bool) api.MediaFile {
	id := domain.NewID[domain.MediaFileID]()
	return api.MediaFile{
		ID:         id,
		Title:      title,
		Episodes:   []domain.EpisodeRef{ep},
		RootFolder: tvRoot,
		Path:       filepath.Join(tvRoot, "Show (2020) [tmdbid-1]", "Season 01", string(id)+".mkv"),
		Size:       episodeSize,
		Origin:     domain.OriginLibraryScan,
		ImportedAt: at,
		Available:  available,
	}
}

func assertFileIDs(t *testing.T, got []api.MediaFile, want ...domain.MediaFileID) {
	t.Helper()
	gotIDs := make([]domain.MediaFileID, 0, len(got))
	for _, f := range got {
		gotIDs = append(gotIDs, f.ID)
	}
	if !slices.Equal(gotIDs, want) {
		t.Errorf("files = %v, want %v", gotIDs, want)
	}
}
