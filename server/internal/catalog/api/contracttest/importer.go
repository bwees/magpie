package contracttest

import (
	"context"
	"maps"
	"sync"
	"testing"
	"time"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
)

const ImporterImportTitles scripted.Method = "catalog.Importer.ImportTitles"

// ImporterWorld is what TMDB knows while the suite imports. The factory makes its TMDB serve exactly this.
type ImporterWorld struct {
	Movies []domain.TmdbID
	Series []ImporterSeries
}

// ImporterSeries is a series TMDB has, in aired order: season number → episode count.
type ImporterSeries struct {
	Tmdb    domain.TmdbID
	Seasons map[int]int
}

// ImporterFactory returns an empty Catalog whose TMDB serves w.
type ImporterFactory func(t *testing.T, w ImporterWorld) api.Importer

const (
	arrivalSource   domain.SourceKey = "radarr:40"
	duneSource      domain.SourceKey = "radarr:41"
	severanceSource domain.SourceKey = "sonarr:12"
	// splitSource is a second Sonarr series that TMDB knows as the same title as severanceSource.
	splitSource domain.SourceKey = "sonarr:13"
	s1e1Source  domain.SourceKey = "sonarr:episode:1"
	s1e2Source  domain.SourceKey = "sonarr:episode:2"
	s2e1Source  domain.SourceKey = "sonarr:episode:3"
	lostSource  domain.SourceKey = "sonarr:episode:4"
	lostSeason                   = 9
)

var importerAddedAt = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)

// importerWorld has two movies and one two-season series.
func importerWorld() ImporterWorld {
	return ImporterWorld{
		Movies: []domain.TmdbID{arrivalTmdb, duneNewTmdb},
		Series: []ImporterSeries{{Tmdb: severanceTmdb, Seasons: map[int]int{1: 2, 2: 1}}},
	}
}

func importedMovie(source domain.SourceKey, tmdb domain.TmdbID) api.ImportTitle {
	return api.ImportTitle{
		Source: source, Kind: domain.MediaKindMovie, Tmdb: tmdb, External: domain.ExternalIDs{Tmdb: tmdb},
		Monitored: true, AddedAt: importerAddedAt,
	}
}

func importedSeries(source domain.SourceKey) api.ImportTitle {
	return api.ImportTitle{
		Source: source, Kind: domain.MediaKindSeries, Tmdb: severanceTmdb, External: domain.ExternalIDs{Tmdb: severanceTmdb},
		Monitored: true, MonitorNewSeasons: true, AddedAt: importerAddedAt,
		Episodes: []api.ImportEpisode{
			{Source: s1e1Source, Season: 1, Number: 1, Monitored: true},
			{Source: s1e2Source, Season: 1, Number: 2, Monitored: false},
			{Source: s2e1Source, Season: 2, Number: 1, Monitored: true},
		},
	}
}

// RunImporter is the catalog.Importer contract: idempotent on source keys and (kind, tmdbId), so a resumed
// migration gets the IDs it got before and creates nothing twice.
func RunImporter(t *testing.T, factory ImporterFactory) {
	ctx := context.Background()

	t.Run("returns a Magpie ID for every title and episode source", func(t *testing.T) {
		importer := factory(t, importerWorld())
		got, err := importer.ImportTitles(ctx, []api.ImportTitle{importedMovie(arrivalSource, arrivalTmdb), importedSeries(severanceSource)})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Titles) != 2 || got.Titles[arrivalSource] == "" || got.Titles[severanceSource] == "" || got.Titles[arrivalSource] == got.Titles[severanceSource] {
			t.Fatalf("titles = %v, want two distinct IDs", got.Titles)
		}
		episodes := []domain.SourceKey{s1e1Source, s1e2Source, s2e1Source}
		seen := map[domain.EpisodeID]bool{}
		for _, source := range episodes {
			id := got.Episodes[source]
			if id == "" || seen[id] {
				t.Fatalf("episodes = %v, want a distinct ID for %s", got.Episodes, source)
			}
			seen[id] = true
		}
	})

	t.Run("a re-run returns the same IDs", func(t *testing.T) {
		importer := factory(t, importerWorld())
		in := []api.ImportTitle{importedMovie(arrivalSource, arrivalTmdb), importedSeries(severanceSource)}
		first, err := importer.ImportTitles(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		again, err := importer.ImportTitles(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if !maps.Equal(first.Titles, again.Titles) || !maps.Equal(first.Episodes, again.Episodes) {
			t.Fatalf("re-run = %+v, want %+v", again, first)
		}
	})

	t.Run("a resumed run keeps earlier IDs and adds only new sources", func(t *testing.T) {
		importer := factory(t, importerWorld())
		first, err := importer.ImportTitles(ctx, []api.ImportTitle{importedMovie(arrivalSource, arrivalTmdb)})
		if err != nil {
			t.Fatal(err)
		}
		resumed, err := importer.ImportTitles(ctx, []api.ImportTitle{importedMovie(arrivalSource, arrivalTmdb), importedMovie(duneSource, duneNewTmdb)})
		if err != nil {
			t.Fatal(err)
		}
		if resumed.Titles[arrivalSource] != first.Titles[arrivalSource] || resumed.Titles[duneSource] == "" || resumed.Titles[duneSource] == first.Titles[arrivalSource] {
			t.Fatalf("resumed = %v, want Arrival kept as %s and Dune new", resumed.Titles, first.Titles[arrivalSource])
		}
	})

	t.Run("two sources for one TMDB title share it", func(t *testing.T) {
		importer := factory(t, importerWorld())
		split := importedSeries(splitSource)
		split.Episodes = nil
		got, err := importer.ImportTitles(ctx, []api.ImportTitle{importedSeries(severanceSource), split})
		if err != nil {
			t.Fatal(err)
		}
		if got.Titles[splitSource] != got.Titles[severanceSource] {
			t.Fatalf("titles = %v, want both sources on one title", got.Titles)
		}
	})

	t.Run("an episode TMDB doesn't have is left unmapped", func(t *testing.T) {
		importer := factory(t, importerWorld())
		series := importedSeries(severanceSource)
		series.Episodes = append(series.Episodes, api.ImportEpisode{Source: lostSource, Season: lostSeason, Number: 1, Monitored: true})
		got, err := importer.ImportTitles(ctx, []api.ImportTitle{series})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got.Episodes[lostSource]; ok || len(got.Episodes) != len(series.Episodes)-1 {
			t.Fatalf("episodes = %v, want every episode but %s", got.Episodes, lostSource)
		}
	})
}

type importedKey struct {
	kind domain.MediaKind
	tmdb domain.TmdbID
}

type episodeKey struct {
	title          domain.TitleID
	season, number int
}

// ImporterFake is the in-memory catalog.Importer the migration job tests against. It passes RunImporter.
type ImporterFake struct {
	scripted.Script

	mu       sync.Mutex
	world    ImporterWorld
	sources  map[domain.SourceKey]domain.TitleID
	titles   map[importedKey]domain.TitleID
	episodes map[episodeKey]domain.EpisodeID
	imported []api.ImportTitle
}

var _ api.Importer = (*ImporterFake)(nil)

func NewImporterFake(w ImporterWorld) *ImporterFake {
	return &ImporterFake{
		world:    w,
		sources:  map[domain.SourceKey]domain.TitleID{},
		titles:   map[importedKey]domain.TitleID{},
		episodes: map[episodeKey]domain.EpisodeID{},
	}
}

func (f *ImporterFake) ImportTitles(ctx context.Context, in []api.ImportTitle) (api.ImportedTitles, error) {
	if err := f.Enter(ctx, ImporterImportTitles); err != nil {
		return api.ImportedTitles{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := api.ImportedTitles{Titles: map[domain.SourceKey]domain.TitleID{}, Episodes: map[domain.SourceKey]domain.EpisodeID{}}
	for _, title := range in {
		id, created := f.title(title)
		if created {
			f.imported = append(f.imported, title)
		}
		out.Titles[title.Source] = id
		for _, e := range title.Episodes {
			if episode, ok := f.episode(title.Tmdb, id, e); ok {
				out.Episodes[e.Source] = episode
			}
		}
	}
	return out, nil
}

// Created lists the titles the fake created, in order; a title found again isn't listed twice.
func (f *ImporterFake) Created() []api.ImportTitle {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]api.ImportTitle(nil), f.imported...)
}

func (f *ImporterFake) title(in api.ImportTitle) (domain.TitleID, bool) {
	if id, ok := f.sources[in.Source]; ok {
		return id, false
	}
	key := importedKey{kind: in.Kind, tmdb: in.Tmdb}
	id, ok := f.titles[key]
	if !ok {
		id = domain.NewID[domain.TitleID]()
		f.titles[key] = id
	}
	f.sources[in.Source] = id
	return id, !ok
}

func (f *ImporterFake) episode(tmdb domain.TmdbID, title domain.TitleID, e api.ImportEpisode) (domain.EpisodeID, bool) {
	if !f.hasEpisode(tmdb, e.Season, e.Number) {
		return "", false
	}
	key := episodeKey{title: title, season: e.Season, number: e.Number}
	id, ok := f.episodes[key]
	if !ok {
		id = domain.NewID[domain.EpisodeID]()
		f.episodes[key] = id
	}
	return id, true
}

func (f *ImporterFake) hasEpisode(tmdb domain.TmdbID, season, number int) bool {
	for _, s := range f.world.Series {
		if s.Tmdb == tmdb {
			return number >= 1 && number <= s.Seasons[season]
		}
	}
	return false
}
