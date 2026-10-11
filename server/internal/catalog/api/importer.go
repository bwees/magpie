package api

import (
	"context"
	"time"

	"magpie/internal/domain"
)

// Importer is the migration entrypoint, called only by the platform migration job. It is idempotent on
// the source key and on (kind, tmdbId). Imported Titles get Origin MIGRATION. Spec: docs/spec/11-migration.md.
type Importer interface {
	// ImportTitles creates or finds each Title and its episodes, and returns their Magpie IDs keyed by source.
	ImportTitles(ctx context.Context, in []ImportTitle) (ImportedTitles, error)
}

type ImportTitle struct {
	Source domain.SourceKey // "sonarr:12" | "radarr:40"
	Kind   domain.MediaKind
	Tmdb   domain.TmdbID
	// External keeps Sonarr's tvdbId / imdbId even when TMDB lacks the link.
	External domain.ExternalIDs
	// Profile was already imported by Acquisition.
	Profile           domain.ProfileID
	Monitored         bool
	MonitorNewSeasons bool
	// EpisodeGroup is the TMDB episode group ID, when the user confirmed one.
	EpisodeGroup *string
	// Episodes is set for series only, already translated to Magpie numbering.
	Episodes []ImportEpisode
	Tags     []string
	AddedAt  time.Time
}

type ImportEpisode struct {
	Source         domain.SourceKey
	Season, Number int
	Monitored      bool
}

// ImportedTitles maps each ImportTitle.Source and ImportEpisode.Source to its Magpie ID.
type ImportedTitles struct {
	Titles   map[domain.SourceKey]domain.TitleID
	Episodes map[domain.SourceKey]domain.EpisodeID
}
