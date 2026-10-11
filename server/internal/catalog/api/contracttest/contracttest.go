// Package contracttest holds the catalog.Reader contract suite and the in-memory Fake that passes it.
// The real repository runs the same suite in its own tests.
package contracttest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"
)

// Fixture is the stored state a Reader starts with. Titles and episodes carry their stored flags,
// including Released, as the release clock last set them.
type Fixture struct {
	Titles            []api.Title
	Episodes          []api.Episode
	Images            []Image
	GlobalLanguages   []api.WantedLanguage
	LanguageOverrides map[domain.TitleID][]api.WantedLanguage
}

// Image is one cached image file.
type Image struct {
	Title domain.TitleID
	Kind  string // api.ImageKind*
	Path  string
}

// Factory returns a Reader holding exactly the fixture's state.
type Factory func(t *testing.T, f Fixture) api.Reader

const (
	duneName      = "Dune"
	arrivalName   = "Arrival"
	severanceName = "Severance"
	duneOldYear   = 1984
	duneNewYear   = 2021
	duneOldTmdb   = 841
	duneNewTmdb   = 438631
	arrivalTmdb   = 329865
	arrivalYear   = 2016
	severanceTmdb = 95396
	severanceYear = 2022
	pluribusName  = "Pluribus"
	pluribusTmdb  = 225171
	pluribusYear  = 2025
	posterPath    = "/data/images/dune-2021/poster.jpg"

	languageEnglish  domain.Language = "en"
	languageSpanish  domain.Language = "es"
	languageJapanese domain.Language = "ja"
)

type world struct {
	fixture                Fixture
	duneNew, duneOld       api.Title
	arrival, severance     api.Title
	pluribus               api.Title
	s1e1, s1e2, s2e1, s2e2 api.Episode
	pluribusPilot          api.Episode
	globalLangs, override  []api.WantedLanguage
}

// newWorld builds the shared fixture. Severance uses an episode group: S2E1 aired as S1E3.
func newWorld() world {
	var w world
	w.duneNew = title(domain.MediaKindMovie, duneName, duneNewYear, duneNewTmdb)
	w.duneNew.Monitored, w.duneNew.Released = true, true
	w.duneOld = title(domain.MediaKindMovie, duneName, duneOldYear, duneOldTmdb)
	w.arrival = title(domain.MediaKindMovie, arrivalName, arrivalYear, arrivalTmdb)
	w.arrival.Monitored = true
	w.severance = title(domain.MediaKindSeries, severanceName, severanceYear, severanceTmdb)
	w.severance.Monitored, w.severance.Released = true, true

	w.s1e1 = episode(w.severance.ID, 1, 1, 1, 1, true, true)
	w.s1e2 = episode(w.severance.ID, 1, 2, 1, 2, false, true)
	w.s2e1 = episode(w.severance.ID, 2, 1, 1, 3, true, true)
	w.s2e2 = episode(w.severance.ID, 2, 2, 1, 4, true, false)

	w.pluribus = title(domain.MediaKindSeries, pluribusName, pluribusYear, pluribusTmdb)
	w.pluribus.Released = true
	w.pluribusPilot = episode(w.pluribus.ID, 1, 1, 1, 1, true, true)

	w.globalLangs = []api.WantedLanguage{{Language: languageEnglish}, {Language: languageSpanish, Forced: true}}
	w.override = []api.WantedLanguage{{Language: languageJapanese}, {Language: languageEnglish, HearingImpaired: true}}

	w.fixture = Fixture{
		Titles:            []api.Title{w.severance, w.duneNew, w.arrival, w.duneOld, w.pluribus},
		Episodes:          []api.Episode{w.s2e2, w.s1e2, w.s2e1, w.s1e1, w.pluribusPilot},
		Images:            []Image{{Title: w.duneNew.ID, Kind: api.ImageKindPoster, Path: posterPath}},
		GlobalLanguages:   w.globalLangs,
		LanguageOverrides: map[domain.TitleID][]api.WantedLanguage{w.arrival.ID: w.override},
	}
	return w
}

// Run checks a Reader against the catalog contract.
func Run(t *testing.T, newReader Factory) {
	ctx := context.Background()
	w := newWorld()
	unknownTitle := domain.NewID[domain.TitleID]()

	t.Run("Title and Episodes return ErrNotFound for an unknown Title", func(t *testing.T) {
		r := newReader(t, w.fixture)
		if _, err := r.Title(ctx, unknownTitle); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("Title err = %v, want ErrNotFound", err)
		}
		if _, err := r.Episodes(ctx, unknownTitle); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("Episodes err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Title returns a stored Title", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, err := r.Title(ctx, w.duneNew.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.External.Tmdb != duneNewTmdb || got.Year != duneNewYear || !got.Released {
			t.Errorf("Title = %+v", got)
		}
	})

	t.Run("TitleByTmdb matches TMDB ID and kind", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, err := r.TitleByTmdb(ctx, domain.MediaKindMovie, duneNewTmdb)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != w.duneNew.ID {
			t.Errorf("TitleByTmdb = %s, want %s", got.ID, w.duneNew.ID)
		}
		if _, err := r.TitleByTmdb(ctx, domain.MediaKindSeries, duneNewTmdb); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("wrong kind err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Titles with an empty filter lists all by name then year", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, err := r.Titles(ctx, api.TitleFilter{})
		if err != nil {
			t.Fatal(err)
		}
		assertTitles(t, got, w.arrival, w.duneOld, w.duneNew, w.pluribus, w.severance)
	})

	t.Run("Titles applies every filter field", func(t *testing.T) {
		r := newReader(t, w.fixture)
		movie, yes, no := domain.MediaKindMovie, true, false
		cases := []struct {
			name   string
			filter api.TitleFilter
			want   []api.Title
		}{
			{"kind", api.TitleFilter{Kind: &movie}, []api.Title{w.arrival, w.duneOld, w.duneNew}},
			{"monitored", api.TitleFilter{Monitored: &yes}, []api.Title{w.arrival, w.duneNew, w.severance}},
			{"unreleased", api.TitleFilter{Released: &no}, []api.Title{w.arrival, w.duneOld}},
			{"ids", api.TitleFilter{IDs: []domain.TitleID{w.severance.ID, w.duneOld.ID}}, []api.Title{w.duneOld, w.severance}},
			{"combined", api.TitleFilter{Kind: &movie, Monitored: &yes, Released: &yes}, []api.Title{w.duneNew}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := r.Titles(ctx, tc.filter)
				if err != nil {
					t.Fatal(err)
				}
				assertTitles(t, got, tc.want...)
			})
		}
	})

	t.Run("Episodes are ordered by season and number in the current numbering", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, err := r.Episodes(ctx, w.severance.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertEpisodes(t, got, w.s1e1, w.s1e2, w.s2e1, w.s2e2)
	})

	t.Run("Episodes of a movie is an empty slice", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, err := r.Episodes(ctx, w.duneNew.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("movie episodes = %v", got)
		}
	})

	t.Run("Episode returns ErrNotFound for an unknown ID", func(t *testing.T) {
		r := newReader(t, w.fixture)
		if _, err := r.Episode(ctx, domain.NewID[domain.EpisodeID]()); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		got, err := r.Episode(ctx, w.s2e1.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.AiredSeason != w.s2e1.AiredSeason || got.AiredNumber != w.s2e1.AiredNumber {
			t.Errorf("Episode = %+v", got)
		}
	})

	t.Run("EpisodeByNumber uses the current numbering", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, err := r.EpisodeByNumber(ctx, w.severance.ID, w.s2e1.Season, w.s2e1.Number)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != w.s2e1.ID {
			t.Errorf("EpisodeByNumber = %s, want %s", got.ID, w.s2e1.ID)
		}
		if _, err := r.EpisodeByNumber(ctx, w.severance.ID, w.s2e1.AiredSeason, w.s2e1.AiredNumber); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("aired numbering err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Wanted lists a series' monitored released episodes", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, ok, err := r.Wanted(ctx, w.severance.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("ok = false")
		}
		assertEpisodes(t, got, w.s1e1, w.s2e1)
	})

	t.Run("Wanted is empty for an unmonitored series", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, ok, err := r.Wanted(ctx, w.pluribus.ID)
		if err != nil {
			t.Fatal(err)
		}
		if ok || len(got) != 0 {
			t.Errorf("Wanted = %v, %v; want empty, false", got, ok)
		}
	})

	t.Run("Wanted reports a movie as an empty slice with ok when monitored and released", func(t *testing.T) {
		r := newReader(t, w.fixture)
		for _, tc := range []struct {
			title api.Title
			want  bool
		}{{w.duneNew, true}, {w.arrival, false}, {w.duneOld, false}} {
			got, ok, err := r.Wanted(ctx, tc.title.ID)
			if err != nil {
				t.Fatal(err)
			}
			if ok != tc.want || len(got) != 0 {
				t.Errorf("Wanted(%s) = %v, %v; want empty, %v", tc.title.ID, got, ok, tc.want)
			}
		}
		if _, _, err := r.Wanted(ctx, unknownTitle); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("unknown err = %v, want ErrNotFound", err)
		}
	})

	t.Run("ImagePath returns cached images and ErrNotFound otherwise", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, err := r.ImagePath(ctx, w.duneNew.ID, api.ImageKindPoster)
		if err != nil {
			t.Fatal(err)
		}
		if got != posterPath {
			t.Errorf("ImagePath = %q, want %q", got, posterPath)
		}
		if _, err := r.ImagePath(ctx, w.duneNew.ID, api.ImageKindLogo); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("uncached err = %v, want ErrNotFound", err)
		}
	})

	t.Run("WantedLanguages returns the override, else the global list", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, err := r.WantedLanguages(ctx, w.arrival.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertLanguages(t, got, w.override)
		got, err = r.WantedLanguages(ctx, w.duneNew.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertLanguages(t, got, w.globalLangs)
		if _, err := r.WantedLanguages(ctx, unknownTitle); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("unknown err = %v, want ErrNotFound", err)
		}
	})

	t.Run("GlobalLanguages ignores overrides", func(t *testing.T) {
		r := newReader(t, w.fixture)
		got, err := r.GlobalLanguages(ctx)
		if err != nil {
			t.Fatal(err)
		}
		assertLanguages(t, got, w.globalLangs)
	})
}

func title(kind domain.MediaKind, name string, year int, tmdb domain.TmdbID) api.Title {
	return api.Title{
		ID:       domain.NewID[domain.TitleID](),
		Kind:     kind,
		External: domain.ExternalIDs{Tmdb: tmdb},
		Name:     name,
		Year:     year,
		Origin:   domain.OriginManual,
	}
}

func episode(title domain.TitleID, season, number, airedSeason, airedNumber int, monitored, released bool) api.Episode {
	return api.Episode{
		ID:          domain.NewID[domain.EpisodeID](),
		Title:       title,
		Season:      season,
		Number:      number,
		AiredSeason: airedSeason,
		AiredNumber: airedNumber,
		Monitored:   monitored,
		Released:    released,
	}
}

func assertTitles(t *testing.T, got []api.Title, want ...api.Title) {
	t.Helper()
	id := func(x api.Title) domain.TitleID { return x.ID }
	if g, w := mapIDs(got, id), mapIDs(want, id); !slices.Equal(g, w) {
		t.Errorf("titles = %v, want %v", g, w)
	}
}

func assertEpisodes(t *testing.T, got []api.Episode, want ...api.Episode) {
	t.Helper()
	id := func(x api.Episode) domain.EpisodeID { return x.ID }
	if g, w := mapIDs(got, id), mapIDs(want, id); !slices.Equal(g, w) {
		t.Errorf("episodes = %v, want %v", g, w)
	}
}

func assertLanguages(t *testing.T, got, want []api.WantedLanguage) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("languages = %+v, want %+v", got, want)
	}
}

func mapIDs[T any, ID any](items []T, id func(T) ID) []ID {
	ids := make([]ID, 0, len(items))
	for _, item := range items {
		ids = append(ids, id(item))
	}
	return ids
}
