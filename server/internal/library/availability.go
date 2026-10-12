package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"

	"gorm.io/gorm"
)

// Defaults and range of the Jellyfin full reconcile, from 12-settings › Schedules.
const (
	DefaultJellyfinReconcile = 24 * time.Hour
	MinJellyfinReconcile     = 6 * time.Hour
	MaxJellyfinReconcile     = 7 * 24 * time.Hour
)

const (
	// overdueAfter is how long Jellyfin may take to confirm a file before the title page warns.
	overdueAfter = 24 * time.Hour
	// reconcileOverlap re-reads a little before the last reconcile, in case Jellyfin's clock runs behind.
	reconcileOverlap = time.Hour
	// pathsPerQuery keeps a path IN (…) list under SQLite's variable limit.
	pathsPerQuery = 500
)

var ErrInvalidSchedules = errors.New("library: invalid schedules")

// Schedules are the Settings → General → Schedules rows Library owns.
type Schedules struct {
	JellyfinReconcile settings.Duration `json:"jellyfinReconcile"`
}

var (
	SchedulesKey = settings.NewKey(owner.Library, "schedules", Schedules{
		JellyfinReconcile: settings.Duration(DefaultJellyfinReconcile),
	})
	// reconciledAtKey is when the last Jellyfin reconcile started, by Magpie's clock.
	reconciledAtKey = settings.NewKey(owner.Library, "jellyfin.reconciledAt", time.Time{})
)

func (s Schedules) Validate() error {
	if r := s.JellyfinReconcile.Std(); r < MinJellyfinReconcile || r > MaxJellyfinReconcile {
		return fmt.Errorf("%w: the Jellyfin reconcile must run every %d–%d hours", ErrInvalidSchedules,
			MinJellyfinReconcile/time.Hour, MaxJellyfinReconcile/time.Hour)
	}
	return nil
}

func (s *Service) Schedules(ctx context.Context) (Schedules, error) {
	return SchedulesKey.Get(ctx, s.deps.Store)
}

func (s *Service) SetSchedules(ctx context.Context, schedules Schedules) error {
	if err := schedules.Validate(); err != nil {
		return err
	}
	return SchedulesKey.Set(ctx, s.deps.Store, schedules)
}

// JellyfinOverdue reports a file Jellyfin still hasn't confirmed a day after it was imported.
func (s *Service) JellyfinOverdue(f api.MediaFile) bool {
	return !f.Available && s.now().Sub(f.ImportedAt) > overdueAfter
}

// refreshFolders asks Jellyfin to rescan each folder once. A failure is only logged: the reconcile asks
// again for every file Jellyfin hasn't confirmed.
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
			slog.WarnContext(ctx, "library: Jellyfin refresh failed; the reconcile retries it", "folder", folder, "error", err)
		}
	}
}

// HandleChange confirms the files of the movies and episodes a LibraryChanged message added or updated.
func (s *Service) HandleChange(ctx context.Context, change LibraryChange) error {
	ids := slices.Compact(slices.Sorted(slices.Values(slices.Concat(change.Added, change.Updated))))
	if len(ids) == 0 {
		return nil
	}
	items, err := s.deps.Jellyfin.Items(ctx, ids)
	if err != nil {
		return err
	}
	return s.confirm(ctx, items)
}

// ReconcileJellyfin confirms what Jellyfin changed since the last run, then asks Jellyfin to rescan the
// folders of every file it still hasn't confirmed.
func (s *Service) ReconcileJellyfin(ctx context.Context) error {
	started := s.now().UTC()
	last, err := reconciledAtKey.Get(ctx, s.deps.Store)
	if err != nil {
		return err
	}
	since := last
	if !last.IsZero() {
		since = last.Add(-reconcileOverlap)
	}
	items, err := s.deps.Jellyfin.ItemsSince(ctx, since)
	if err != nil {
		return err
	}
	if err := s.confirm(ctx, items); err != nil {
		return err
	}
	if err := reconciledAtKey.Set(ctx, s.deps.Store, started); err != nil {
		return err
	}
	var unconfirmed []string
	if err := s.deps.DB.Read(ctx).Model(&mediaFileRow{}).Where("available_at IS NULL").Pluck("path", &unconfirmed).Error; err != nil {
		return err
	}
	s.refreshFolders(ctx, unconfirmed)
	return nil
}

// confirm records each Jellyfin item against the media file at its path and publishes ItemAvailable once
// per (file, Jellyfin item).
func (s *Service) confirm(ctx context.Context, items []JellyfinItem) error {
	byPath := map[string]JellyfinItem{}
	for _, item := range items {
		byPath[filepath.Clean(item.Path)] = item
	}
	for chunk := range slices.Chunk(slices.Sorted(maps.Keys(byPath)), pathsPerQuery) {
		err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return s.confirmPaths(tx, chunk, byPath) })
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) confirmPaths(tx *gorm.DB, paths []string, byPath map[string]JellyfinItem) error {
	var rows []mediaFileRow
	if err := tx.Where("path IN ?", paths).Order("path").Find(&rows).Error; err != nil {
		return err
	}
	now := s.now().UTC()
	for _, row := range rows {
		item := byPath[row.Path]
		result := tx.Model(&mediaFileRow{}).
			Where("id = ? AND (jellyfin_item_id IS NULL OR jellyfin_item_id <> ?)", row.ID, item.ID).
			Updates(map[string]any{"jellyfin_item_id": item.ID, "available_at": gorm.Expr("COALESCE(available_at, ?)", now)})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		episodes, err := fileEpisodes(tx, row.ID)
		if err != nil {
			return err
		}
		event := api.ItemAvailable{Title: row.TitleID, Episodes: episodes, JellyfinItem: item.ID}
		if err := s.deps.Bus.Publish(tx, bus.EventOf(event)); err != nil {
			return err
		}
	}
	return nil
}

func fileEpisodes(tx *gorm.DB, file domain.MediaFileID) ([]domain.EpisodeID, error) {
	episodes := []domain.EpisodeID{}
	err := tx.Model(&mediaFileEpisodeRow{}).Where("media_file_id = ?", file).Order(episodeOrder).Pluck("episode_id", &episodes).Error
	return episodes, err
}
