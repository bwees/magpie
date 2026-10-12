package app

import (
	"context"
	"errors"

	acquisitionapi "magpie/internal/acquisition/api"
	activityapi "magpie/internal/activity/api"
	"magpie/internal/adapters/arr"
	"magpie/internal/catalog"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/migrate"

	"go.uber.org/fx"
)

// Migration is the Sonarr/Radarr import job with its ports and the contexts' Importers.
var Migration = fx.Options(
	migrate.Module,
	fx.Provide(
		fx.Annotate(arr.New, fx.As(new(migrate.Arr))),
		func(t catalog.TMDB) migrate.TMDB { return migrationTMDB{tmdb: t} },
		func() migrate.Disk { return migrate.OSDisk{} },
		func(c catalogapi.Importer, a acquisitionapi.Importer, l libraryapi.Importer, ac activityapi.Importer) migrate.Importers {
			return migrate.Importers{Catalog: c, Acquisition: a, Library: l, Activity: ac}
		},
	),
)

// matchingLanguage is TMDB's default language; the migration reads names only to match titles.
const matchingLanguage = ""

// migrationTMDB lets the migration use Catalog's TMDB adapter, and its rate limit, through its own port.
type migrationTMDB struct {
	tmdb catalog.TMDB
}

func notFound(err error) error {
	if errors.Is(err, catalog.ErrTmdbNotFound) {
		return errors.Join(migrate.ErrTmdbNotFound, err)
	}
	return err
}

func candidate(item catalog.TmdbSearchItem) migrate.TmdbCandidate {
	return migrate.TmdbCandidate{Tmdb: item.ID, Kind: item.Kind, Name: item.Name, Year: item.Year}
}

func (m migrationTMDB) Series(ctx context.Context, id domain.TmdbID) (migrate.TmdbSeries, error) {
	s, err := m.tmdb.Series(ctx, id, matchingLanguage)
	if err != nil {
		return migrate.TmdbSeries{}, notFound(err)
	}
	out := migrate.TmdbSeries{TmdbCandidate: migrate.TmdbCandidate{Tmdb: s.ID, Kind: domain.MediaKindSeries, Name: s.Name}, Tvdb: s.External.Tvdb}
	if s.FirstAirDate != nil {
		out.Year = s.FirstAirDate.Year()
	}
	for _, season := range s.Seasons {
		out.Seasons = append(out.Seasons, season.Number)
	}
	return out, nil
}

const airDateLayout = "2006-01-02"

func (m migrationTMDB) Season(ctx context.Context, id domain.TmdbID, number int) (migrate.TmdbSeason, error) {
	s, err := m.tmdb.Season(ctx, id, number, matchingLanguage)
	if err != nil {
		return migrate.TmdbSeason{}, notFound(err)
	}
	out := migrate.TmdbSeason{Number: s.Number}
	for _, e := range s.Episodes {
		episode := migrate.TmdbEpisode{ID: int64(e.ID), Season: e.Season, Number: e.Number, Name: e.Name}
		if e.AirDate != nil {
			episode.AirDate = e.AirDate.UTC().Format(airDateLayout)
		}
		out.Episodes = append(out.Episodes, episode)
	}
	return out, nil
}

func (m migrationTMDB) EpisodeGroups(ctx context.Context, id domain.TmdbID) ([]migrate.TmdbGroupSummary, error) {
	groups, err := m.tmdb.EpisodeGroups(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	out := make([]migrate.TmdbGroupSummary, 0, len(groups))
	for _, g := range groups {
		out = append(out, migrate.TmdbGroupSummary{ID: g.ID, Name: g.Name, Type: int(g.Type)})
	}
	return out, nil
}

func (m migrationTMDB) EpisodeGroup(ctx context.Context, groupID string) (migrate.TmdbGroup, error) {
	g, err := m.tmdb.EpisodeGroup(ctx, groupID)
	if err != nil {
		return migrate.TmdbGroup{}, notFound(err)
	}
	out := migrate.TmdbGroup{ID: g.ID, Name: g.Name, Type: int(g.Type)}
	for _, season := range g.Groups {
		s := migrate.TmdbGroupSeason{Order: season.Order}
		for _, entry := range season.Episodes {
			s.Episodes = append(s.Episodes, migrate.TmdbGroupEntry{Episode: int64(entry.Episode), Order: entry.Order})
		}
		out.Seasons = append(out.Seasons, s)
	}
	return out, nil
}

func (m migrationTMDB) Find(ctx context.Context, source, externalID string) (migrate.TmdbFound, error) {
	found, err := m.tmdb.Find(ctx, source, externalID)
	if err != nil {
		return migrate.TmdbFound{}, notFound(err)
	}
	out := migrate.TmdbFound{}
	for _, item := range found.Movies {
		out.Movies = append(out.Movies, candidate(item))
	}
	for _, item := range found.Series {
		out.Series = append(out.Series, candidate(item))
	}
	for _, e := range found.Episodes {
		out.Episodes = append(out.Episodes, migrate.TmdbFoundEpisode{ID: int64(e.ID), Series: e.Series, Season: e.Season, Number: e.Number})
	}
	return out, nil
}

func (m migrationTMDB) Search(ctx context.Context, query string) ([]migrate.TmdbCandidate, error) {
	page, err := m.tmdb.Search(ctx, query, 1)
	if err != nil {
		return nil, notFound(err)
	}
	out := make([]migrate.TmdbCandidate, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, candidate(item))
	}
	return out, nil
}

func (m migrationTMDB) Movie(ctx context.Context, id domain.TmdbID) (migrate.TmdbCandidate, error) {
	movie, err := m.tmdb.Movie(ctx, id, matchingLanguage)
	if err != nil {
		return migrate.TmdbCandidate{}, notFound(err)
	}
	out := migrate.TmdbCandidate{Tmdb: movie.ID, Kind: domain.MediaKindMovie, Name: movie.Title}
	if movie.ReleaseDate != nil {
		out.Year = movie.ReleaseDate.Year()
	}
	return out, nil
}
