package contracttest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/scripted"
)

const (
	ImporterImportRootFolders scripted.Method = "library.Importer.ImportRootFolders"
	ImporterImportMediaFiles  scripted.Method = "library.Importer.ImportMediaFiles"
)

// ErrInaccessibleRoot is what the fake returns for a root folder that isn't on its disk.
var ErrInaccessibleRoot = errors.New("library: root folder isn't accessible")

// ImporterWorld is the disk the suite imports from (folders that exist and the videos in them) and the
// Catalog it links to: Movie, and Series with Episode as its S01E01.
type ImporterWorld struct {
	Folders []string
	Videos  []string
	Movie   domain.TitleID
	Series  domain.TitleID
	Episode domain.EpisodeID
}

// ImporterFactory returns an empty Library whose disk holds w.
type ImporterFactory func(t *testing.T, w ImporterWorld) api.Importer

const missingRoot = "/media/nowhere"

var (
	importedMovieVideo  = filepath.Join(moviesRoot, "Arrival (2016)", "Arrival.2016.1080p.BluRay.x264-GRP.mkv")
	importedSeriesVideo = filepath.Join(tvRoot, "Severance", "Season 01", "Severance.S01E01.1080p.WEB-DL-GRP.mkv")
	importedAddedAt     = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
)

func importerWorld() ImporterWorld {
	return ImporterWorld{
		Folders: []string{moviesRoot, tvRoot}, Videos: []string{importedMovieVideo, importedSeriesVideo},
		Movie: domain.NewID[domain.TitleID](), Series: domain.NewID[domain.TitleID](), Episode: domain.NewID[domain.EpisodeID](),
	}
}

func importedRoots() []api.ImportRootFolder {
	return []api.ImportRootFolder{
		{Source: "radarr:rootfolder:1", Kind: domain.MediaKindMovie, Path: moviesRoot},
		{Source: "sonarr:rootfolder:1", Kind: domain.MediaKindSeries, Path: tvRoot},
	}
}

func importedFiles(w ImporterWorld) []api.ImportMediaFile {
	return []api.ImportMediaFile{
		{
			Source: "radarr:moviefile:1", Title: w.Movie, Path: importedMovieVideo, Size: 1,
			Quality: domain.Quality{ID: 7, Name: "Bluray-1080p"}, ReleaseName: "Arrival.2016.1080p.BluRay.x264-GRP", AddedAt: importedAddedAt,
		},
		{
			Source: "sonarr:episodefile:1", Title: w.Series, Episodes: []domain.EpisodeID{w.Episode},
			Path: importedSeriesVideo, Size: 1, Quality: domain.Quality{ID: 3, Name: "WEBDL-1080p"}, AddedAt: importedAddedAt,
		},
	}
}

// RunImporter is the library.Importer contract: root folders must be reachable, and files are linked once
// per source, so a resumed migration gets the IDs it got before.
func RunImporter(t *testing.T, factory ImporterFactory) {
	ctx := context.Background()

	t.Run("root folders can be imported again", func(t *testing.T) {
		importer := factory(t, importerWorld())
		for range 2 {
			if err := importer.ImportRootFolders(ctx, importedRoots()); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("an unreachable root folder fails the call", func(t *testing.T) {
		importer := factory(t, importerWorld())
		err := importer.ImportRootFolders(ctx, []api.ImportRootFolder{{Source: "sonarr:rootfolder:9", Kind: domain.MediaKindSeries, Path: missingRoot}})
		if err == nil {
			t.Fatal("importing a missing root folder succeeded")
		}
	})

	t.Run("files keep their IDs on a re-run and a resumed run adds the rest", func(t *testing.T) {
		world := importerWorld()
		importer := factory(t, world)
		if err := importer.ImportRootFolders(ctx, importedRoots()); err != nil {
			t.Fatal(err)
		}
		files := importedFiles(world)
		first, err := importer.ImportMediaFiles(ctx, files[:1])
		if err != nil {
			t.Fatal(err)
		}
		again, err := importer.ImportMediaFiles(ctx, files[:1])
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != 1 || !maps.Equal(first, again) {
			t.Fatalf("re-run = %v, want %v", again, first)
		}
		resumed, err := importer.ImportMediaFiles(ctx, files)
		if err != nil {
			t.Fatal(err)
		}
		movie, series := files[0].Source, files[1].Source
		if resumed[movie] != first[movie] || resumed[series] == "" || resumed[series] == first[movie] {
			t.Fatalf("resumed = %v, want the movie kept as %s and the episode file new", resumed, first[movie])
		}
	})

	t.Run("a file that's gone from disk is left out", func(t *testing.T) {
		world := importerWorld()
		importer := factory(t, world)
		if err := importer.ImportRootFolders(ctx, importedRoots()); err != nil {
			t.Fatal(err)
		}
		gone := importedFiles(world)[0]
		gone.Source, gone.Path = "radarr:moviefile:2", filepath.Join(moviesRoot, "Gone", "Gone.mkv")
		got, err := importer.ImportMediaFiles(ctx, []api.ImportMediaFile{gone})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("got %v, want nothing for a file that isn't there", got)
		}
	})
}

// ImporterFake is the in-memory library.Importer the migration job tests against. It passes RunImporter.
type ImporterFake struct {
	scripted.Script

	mu       sync.Mutex
	world    ImporterWorld
	roots    []api.ImportRootFolder
	files    map[domain.SourceKey]domain.MediaFileID
	imported []api.ImportMediaFile
}

var _ api.Importer = (*ImporterFake)(nil)

func NewImporterFake(w ImporterWorld) *ImporterFake {
	return &ImporterFake{world: w, files: map[domain.SourceKey]domain.MediaFileID{}}
}

func (f *ImporterFake) ImportRootFolders(ctx context.Context, in []api.ImportRootFolder) error {
	if err := f.Enter(ctx, ImporterImportRootFolders); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, root := range in {
		if !slices.Contains(f.world.Folders, root.Path) {
			return fmt.Errorf("%w: %s", ErrInaccessibleRoot, root.Path)
		}
	}
	for _, root := range in {
		if !slices.ContainsFunc(f.roots, func(r api.ImportRootFolder) bool { return r.Path == root.Path }) {
			f.roots = append(f.roots, root)
		}
	}
	return nil
}

func (f *ImporterFake) ImportMediaFiles(ctx context.Context, in []api.ImportMediaFile) (map[domain.SourceKey]domain.MediaFileID, error) {
	if err := f.Enter(ctx, ImporterImportMediaFiles); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[domain.SourceKey]domain.MediaFileID{}
	for _, file := range in {
		if id, ok := f.files[file.Source]; ok {
			out[file.Source] = id
			continue
		}
		if !slices.Contains(f.world.Videos, file.Path) || !f.underRoot(file.Path) {
			continue
		}
		id := domain.NewID[domain.MediaFileID]()
		f.files[file.Source] = id
		f.imported = append(f.imported, file)
		out[file.Source] = id
	}
	return out, nil
}

func (f *ImporterFake) underRoot(path string) bool {
	return slices.ContainsFunc(f.roots, func(r api.ImportRootFolder) bool {
		return strings.HasPrefix(path, r.Path+string(filepath.Separator))
	})
}

// Roots lists the imported root folders, one per path.
func (f *ImporterFake) Roots() []api.ImportRootFolder {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.roots)
}

// Files lists the linked files, one per source, in import order.
func (f *ImporterFake) Files() []api.ImportMediaFile {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.imported)
}
