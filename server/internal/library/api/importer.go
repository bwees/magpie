package api

import (
	"context"
	"time"

	"magpie/internal/domain"
)

// Importer is the migration entrypoint (platform migration job only). Idempotent on the source key.
// Spec: docs/spec/11-migration.md.
type Importer interface {
	// ImportRootFolders stores root folders after checking access and running the hardlink test.
	// An inaccessible root fails the call.
	ImportRootFolders(ctx context.Context, in []ImportRootFolder) error
	// ImportMediaFiles links existing files to Titles without moving or renaming them and publishes
	// FileImported{Origin: MIGRATION} per new file. Returns Magpie IDs keyed by source.
	ImportMediaFiles(ctx context.Context, in []ImportMediaFile) (map[domain.SourceKey]domain.MediaFileID, error)
}

type ImportRootFolder struct {
	Source domain.SourceKey
	Kind   domain.MediaKind
	Path   string // after path rewrite
}

type ImportMediaFile struct {
	Source       domain.SourceKey
	Title        domain.TitleID
	Episodes     []domain.EpisodeID
	Path         string // after path rewrite
	Size         int64
	Quality      domain.Quality // Magpie (Radarr) quality IDs
	ReleaseName  string         // *arr sceneName
	ReleaseGroup string
	Languages    []domain.Language
	AddedAt      time.Time
}
