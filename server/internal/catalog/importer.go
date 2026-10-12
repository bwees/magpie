package catalog

import (
	"context"
	"errors"
	"log/slog"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type importSourceKind string

const (
	importSourceTitle   importSourceKind = "TITLE"
	importSourceEpisode importSourceKind = "EPISODE"
)

type importSourceRow struct {
	SourceKey domain.SourceKey `gorm:"primaryKey"`
	Kind      importSourceKind
	TargetID  string
}

func (importSourceRow) TableName() string { return "catalog_import_sources" }

var _ api.Importer = (*Service)(nil)

type importKey struct {
	kind domain.MediaKind
	tmdb domain.TmdbID
}

// importMetadata is what TMDB says about a title the migration creates, fetched before the transaction.
type importMetadata struct {
	movie  TmdbMovie
	series seriesSnapshot
}

func (m importMetadata) images(kind domain.MediaKind) TmdbImages {
	if kind == domain.MediaKindSeries {
		return m.series.series.Images
	}
	return m.movie.Images
}

// ImportTitles implements api.Importer. Titles a source or (kind, tmdbId) already names are reused as they
// are; new ones are fetched from TMDB and stored in one transaction with Origin MIGRATION, each publishing
// TitleAdded{Origin: MIGRATION}. Episodes map by their Magpie numbering; one TMDB lacks is left out.
func (s *Service) ImportTitles(ctx context.Context, in []api.ImportTitle) (api.ImportedTitles, error) {
	meta, err := metadataSetting.Get(ctx, s.settings)
	if err != nil {
		return api.ImportedTitles{}, err
	}
	fetched := map[importKey]importMetadata{}
	conn := s.db.Read(ctx)
	for _, title := range in {
		key := importKey{kind: title.Kind, tmdb: title.Tmdb}
		if _, done := fetched[key]; done {
			continue
		}
		known, err := s.importedTitle(conn, title)
		if err != nil {
			return api.ImportedTitles{}, err
		}
		if known != nil {
			continue
		}
		if fetched[key], err = s.fetchImport(ctx, title, meta.Language); err != nil {
			return api.ImportedTitles{}, err
		}
	}

	var out api.ImportedTitles
	var created []api.ImportTitle
	err = s.db.Write(ctx, func(tx *gorm.DB) error {
		out = api.ImportedTitles{Titles: map[domain.SourceKey]domain.TitleID{}, Episodes: map[domain.SourceKey]domain.EpisodeID{}}
		created = nil
		for _, title := range in {
			id, isNew, err := s.importTitle(tx, title, fetched[importKey{kind: title.Kind, tmdb: title.Tmdb}], meta.ReleaseRegion)
			if err != nil {
				return err
			}
			if isNew {
				created = append(created, title)
			}
			out.Titles[title.Source] = id
			if err := s.importEpisodes(tx, id, title.Episodes, out.Episodes); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return api.ImportedTitles{}, err
	}
	for _, title := range created {
		id := out.Titles[title.Source]
		images := fetched[importKey{kind: title.Kind, tmdb: title.Tmdb}].images(title.Kind)
		if err := s.cacheImages(ctx, id, images); err != nil {
			slog.WarnContext(ctx, "caching title images failed; the refresh job retries", "title", id, "error", err)
		}
	}
	return out, nil
}

// importedTitle is the Title a source or its (kind, tmdbId) already names, or nil.
func (s *Service) importedTitle(conn *gorm.DB, in api.ImportTitle) (*titleRow, error) {
	var sources []importSourceRow
	if err := conn.Where("source_key = ? AND kind = ?", in.Source, importSourceTitle).Limit(1).Find(&sources).Error; err != nil {
		return nil, err
	}
	if len(sources) > 0 {
		row, err := s.repo.title(conn, domain.TitleID(sources[0].TargetID))
		if err == nil {
			return &row, nil
		}
		if !errors.Is(err, api.ErrNotFound) {
			return nil, err
		}
	}
	row, err := s.repo.titleByTmdb(conn, in.Kind, in.Tmdb)
	if errors.Is(err, api.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) fetchImport(ctx context.Context, in api.ImportTitle, lang string) (importMetadata, error) {
	if in.Kind == domain.MediaKindSeries {
		group := ""
		if in.EpisodeGroup != nil {
			group = *in.EpisodeGroup
		}
		snap, err := s.fetchSeries(ctx, in.Tmdb, group, lang)
		return importMetadata{series: snap}, err
	}
	movie, err := s.tmdb.Movie(ctx, in.Tmdb, lang)
	return importMetadata{movie: movie}, err
}

// importTitle finds or creates the Title for one source and records the source.
func (s *Service) importTitle(tx *gorm.DB, in api.ImportTitle, meta importMetadata, region string) (domain.TitleID, bool, error) {
	known, err := s.importedTitle(tx, in)
	if err != nil {
		return "", false, err
	}
	if known != nil {
		id := domain.TitleID(known.ID)
		return id, false, saveImportSource(tx, in.Source, importSourceTitle, string(id))
	}
	var row titleRow
	var monitored []domain.EpisodeID
	if in.Kind == domain.MediaKindSeries {
		row, monitored, err = s.createImportedSeries(tx, in, meta.series)
	} else {
		row, err = s.createImportedMovie(tx, in, meta.movie, region)
	}
	if err != nil {
		return "", false, err
	}
	id := domain.TitleID(row.ID)
	if err := saveImportSource(tx, in.Source, importSourceTitle, row.ID); err != nil {
		return "", false, err
	}
	return id, true, s.publish(tx, api.TitleAdded{
		Title:     id,
		Kind:      in.Kind,
		External:  domain.ExternalIDs{Tmdb: in.Tmdb, Tvdb: row.TvdbID, Imdb: row.ImdbID},
		Profile:   in.Profile,
		Monitored: monitored,
		Released:  row.Released,
		Origin:    domain.OriginMigration,
	})
}

// applyImport sets what the source app said about a title. External IDs are kept only when TMDB has none.
func applyImport(row *titleRow, in api.ImportTitle) {
	row.ProfileID = string(in.Profile)
	row.Monitored = in.Monitored
	row.Origin = string(domain.OriginMigration)
	if !in.AddedAt.IsZero() {
		row.AddedAt = in.AddedAt.UTC()
	}
	tmdbHasNone := row.TvdbID == nil && row.ImdbID == nil
	if tmdbHasNone && (in.External.Tvdb != nil || in.External.Imdb != nil) {
		row.TvdbID, row.ImdbID, row.ExternalIDSource = in.External.Tvdb, in.External.Imdb, ExternalIDSourceImport
	}
}

func (s *Service) createImportedMovie(tx *gorm.DB, in api.ImportTitle, movie TmdbMovie, region string) (titleRow, error) {
	row, err := s.movieRow(movie, region)
	if err != nil {
		return titleRow{}, err
	}
	applyImport(&row, in)
	return row, s.repo.createTitle(tx, row)
}

// createImportedSeries stores a series with the source app's episode monitoring. A season is monitored when
// any of its imported episodes is.
func (s *Service) createImportedSeries(tx *gorm.DB, in api.ImportTitle, snap seriesSnapshot) (titleRow, []domain.EpisodeID, error) {
	row := s.newSeriesRow(in.Tmdb)
	row.EpisodeGroupID = snap.groupID
	row.MonitorFutureSeasons = in.MonitorNewSeasons
	row, _, err := s.createSeries(tx, row, snap, noSeason)
	if err != nil {
		return titleRow{}, nil, err
	}
	applyImport(&row, in)
	if err := s.repo.saveTitle(tx, row); err != nil {
		return titleRow{}, nil, err
	}
	id := domain.TitleID(row.ID)
	episodes, err := s.repo.titleEpisodes(tx, id)
	if err != nil {
		return titleRow{}, nil, err
	}
	wanted := map[episodeNumber]bool{}
	for _, e := range in.Episodes {
		wanted[episodeNumber{Season: e.Season, Number: e.Number}] = e.Monitored
	}
	monitoredSeasons := map[int]bool{}
	monitored := []domain.EpisodeID{}
	for i := range episodes {
		e := &episodes[i]
		e.Monitored = wanted[e.number()]
		if e.Monitored {
			monitoredSeasons[e.SeasonNumber] = true
			monitored = append(monitored, domain.EpisodeID(e.ID))
		}
	}
	if err := s.repo.saveEpisodes(tx, episodes); err != nil {
		return titleRow{}, nil, err
	}
	seasons, err := s.repo.seasonRows(tx, id)
	if err != nil {
		return titleRow{}, nil, err
	}
	for _, season := range seasons {
		season.Monitored = monitoredSeasons[season.Number]
		if err := s.repo.saveSeason(tx, season); err != nil {
			return titleRow{}, nil, err
		}
	}
	return row, monitored, nil
}

// importEpisodes maps each episode source to the title's episode at that numbering, keeping a source's
// earlier mapping.
func (s *Service) importEpisodes(tx *gorm.DB, title domain.TitleID, in []api.ImportEpisode, out map[domain.SourceKey]domain.EpisodeID) error {
	if len(in) == 0 {
		return nil
	}
	episodes, err := s.repo.titleEpisodes(tx, title)
	if err != nil {
		return err
	}
	byNumber := map[episodeNumber]domain.EpisodeID{}
	owned := map[domain.EpisodeID]bool{}
	for _, e := range episodes {
		byNumber[e.number()] = domain.EpisodeID(e.ID)
		owned[domain.EpisodeID(e.ID)] = true
	}
	sources := make([]domain.SourceKey, 0, len(in))
	for _, e := range in {
		sources = append(sources, e.Source)
	}
	var known []importSourceRow
	if err := tx.Where("source_key IN ? AND kind = ?", sources, importSourceEpisode).Find(&known).Error; err != nil {
		return err
	}
	earlier := map[domain.SourceKey]domain.EpisodeID{}
	for _, row := range known {
		earlier[row.SourceKey] = domain.EpisodeID(row.TargetID)
	}
	for _, e := range in {
		if id, ok := earlier[e.Source]; ok && owned[id] {
			out[e.Source] = id
			continue
		}
		id, ok := byNumber[episodeNumber{Season: e.Season, Number: e.Number}]
		if !ok {
			continue
		}
		if err := saveImportSource(tx, e.Source, importSourceEpisode, string(id)); err != nil {
			return err
		}
		out[e.Source] = id
	}
	return nil
}

func saveImportSource(tx *gorm.DB, source domain.SourceKey, kind importSourceKind, target string) error {
	row := importSourceRow{SourceKey: source, Kind: kind, TargetID: target}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}
