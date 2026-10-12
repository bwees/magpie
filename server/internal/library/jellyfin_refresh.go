package library

import (
	"context"
	"log/slog"
	"path/filepath"
	"slices"

	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"

	"gorm.io/gorm"
)

// refreshFolders asks Jellyfin to rescan each folder once. A failure is only logged; the files count as
// available either way.
func (s *Service) refreshFolders(ctx context.Context, files []string) {
	folders := make([]string, 0, len(files))
	for _, f := range files {
		folders = append(folders, filepath.Dir(f))
	}
	for _, folder := range slices.Compact(slices.Sorted(slices.Values(folders))) {
		if err := s.deps.Jellyfin.RefreshPath(ctx, folder); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.WarnContext(ctx, "library: Jellyfin refresh failed", "folder", folder, "error", err)
		}
	}
}

// publishImported publishes a new file's FileImported and its ItemAvailable: a file counts as available
// once it is in the library.
func (s *Service) publishImported(tx *gorm.DB, imported api.FileImported) error {
	episodes := make([]domain.EpisodeID, 0, len(imported.Episodes))
	for _, ref := range imported.Episodes {
		episodes = append(episodes, ref.Episode)
	}
	available := api.ItemAvailable{Title: imported.Title, Episodes: episodes}
	if err := s.deps.Bus.Publish(tx, bus.EventOf(imported)); err != nil {
		return err
	}
	return s.deps.Bus.Publish(tx, bus.EventOf(available))
}
