package library

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"slices"

	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"

	"gorm.io/gorm"
)

// titleFiles lists a title's files with their episodes, newest first, inside a transaction.
func titleFiles(tx *gorm.DB, title domain.TitleID) ([]api.MediaFile, error) {
	var rows []mediaFileRow
	if err := tx.Where("title_id = ?", title).Order(newestFirst).Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]domain.MediaFileID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var episodes []mediaFileEpisodeRow
	if err := tx.Where("media_file_id IN ?", ids).Order(episodeOrder).Find(&episodes).Error; err != nil {
		return nil, err
	}
	files := make([]api.MediaFile, 0, len(rows))
	for _, row := range rows {
		refs := []domain.EpisodeRef{}
		for _, e := range episodes {
			if e.MediaFileID == row.ID {
				refs = append(refs, domain.EpisodeRef{Episode: e.EpisodeID, Season: e.Season, Number: e.Number})
			}
		}
		files = append(files, api.MediaFile{ID: row.ID, Title: row.TitleID, Path: row.Path, Episodes: refs})
	}
	return files, nil
}

// replacedBy are the files a new video takes the place of: for a movie every file of the title, for a
// series every file sharing an episode with it.
func replacedBy(existing []api.MediaFile, kind domain.MediaKind, episodes []domain.EpisodeRef) []api.MediaFile {
	if kind != domain.MediaKindSeries {
		return existing
	}
	return slices.DeleteFunc(slices.Clone(existing), func(f api.MediaFile) bool {
		return !slices.ContainsFunc(f.Episodes, func(old domain.EpisodeRef) bool {
			return slices.ContainsFunc(episodes, func(ref domain.EpisodeRef) bool { return ref.Episode == old.Episode })
		})
	})
}

func deleteMediaFile(tx *gorm.DB, id domain.MediaFileID) error {
	if err := tx.Where("media_file_id = ?", id).Delete(&mediaFileEpisodeRow{}).Error; err != nil {
		return err
	}
	return tx.Where("id = ?", id).Delete(&mediaFileRow{}).Error
}

// forgetFile deletes a file's record and publishes FileDeleted, the one signal other contexts clean up on.
func (s *Service) forgetFile(tx *gorm.DB, f api.MediaFile, reason api.DeleteReason) error {
	if err := deleteMediaFile(tx, f.ID); err != nil {
		return err
	}
	return s.deps.Bus.Publish(tx, bus.EventOf(api.FileDeleted{
		MediaFile: f.ID, Title: f.Title, Episodes: f.Episodes, Path: f.Path, Reason: reason,
	}))
}

// removeFiles deletes files from disk. A file already gone counts as removed.
func (s *Service) removeFiles(paths []string) error {
	var errs []error
	for _, path := range paths {
		if err := s.deps.Files.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// retireReplaced removes the files an upgrade replaced. They are no longer in the library, so a failure
// only leaves a stray file behind.
func (s *Service) retireReplaced(ctx context.Context, paths []string) {
	if err := s.removeFiles(paths); err != nil {
		slog.WarnContext(ctx, "library: couldn't remove a replaced file", "error", err)
	}
}
