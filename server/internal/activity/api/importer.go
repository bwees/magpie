package api

import (
	"context"
	"time"

	"magpie/internal/domain"
)

// ImportKind is the timeline event a Sonarr/Radarr history row becomes.
type ImportKind string

const (
	ImportGrabbed        ImportKind = "GRABBED"
	ImportImported       ImportKind = "IMPORTED"
	ImportDownloadFailed ImportKind = "DOWNLOAD_FAILED"
	ImportFileDeleted    ImportKind = "FILE_DELETED"
)

// ImportTimelineEvent is one Sonarr/Radarr history row, already linked to Magpie IDs.
type ImportTimelineEvent struct {
	Source      domain.SourceKey // "sonarr:history:812"
	Title       domain.TitleID
	Episodes    []domain.EpisodeID // empty for movies
	Kind        ImportKind
	ReleaseName string
	Reason      string // failure or deletion reason, when the row has one
	At          time.Time
}

// Importer is the migration entrypoint (platform migration job only). Idempotent on Source.
// Rows keep their original timestamps; no notifications or attention items are created.
type Importer interface {
	// ImportTimeline writes history rows onto title timelines and the global history.
	ImportTimeline(ctx context.Context, in []ImportTimelineEvent) error
}
