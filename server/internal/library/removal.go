package library

import (
	"context"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/library/api"
	"magpie/internal/platform/bus"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	handlerTitleRemoved = "library.remove-on-title-removed"

	removeTitleQueue    = "library.remove-title"
	removeTitleAttempts = 3
	removeTitleBackoff  = time.Minute
)

// DeleteFile removes a file from disk, then its record, and publishes FileDeleted{REMOVED}. A file
// already gone from disk counts as deleted.
func (s *Service) DeleteFile(ctx context.Context, id domain.MediaFileID) error {
	f, err := s.deps.Repo.File(ctx, id)
	if err != nil {
		return err
	}
	if err := s.removeFiles([]string{f.Path}); err != nil {
		return err
	}
	err = s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return s.forgetFile(tx, f, api.DeleteRemoved) })
	if err != nil {
		return err
	}
	s.refreshFolders(ctx, []string{f.Path})
	return nil
}

// removeTitleTask drops a removed title's files, deleting them from disk when DeleteFiles is set.
type removeTitleTask struct {
	Title       domain.TitleID `json:"title"`
	DeleteFiles bool           `json:"deleteFiles"`
}

func (removeTitleTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: removeTitleQueue, MaxAttempts: removeTitleAttempts, Backoff: removeTitleBackoff}
}

// newTitleRemovedHandler queues dropping the title's files. Handlers can't touch the disk inside their
// transaction, so the removal runs as a job.
func newTitleRemovedHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerTitleRemoved,
		EventTypes: []string{catalogapi.TypeTitleRemoved},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			removed, err := bus.Decode[catalogapi.TitleRemoved](ev)
			if err != nil {
				return err
			}
			return s.deps.Jobs.EnqueueTx(tx, removeTitleTask{Title: removed.Title, DeleteFiles: removed.DeleteFiles})
		},
	}
}

func newRemoveTitleQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t removeTitleTask) error {
		return s.RemoveTitle(ctx, t.Title, t.DeleteFiles)
	})
}

// RemoveTitle forgets every file of a removed title, publishing FileDeleted{TITLE_REMOVED} for each, and
// deletes them from disk first when deleteFiles is set. A rerun picks up the files still recorded.
func (s *Service) RemoveTitle(ctx context.Context, title domain.TitleID, deleteFiles bool) error {
	files, err := s.deps.Repo.FilesForTitle(ctx, title)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if deleteFiles {
			if err := s.removeFiles([]string{f.Path}); err != nil {
				return err
			}
		}
		err := s.deps.DB.Write(ctx, func(tx *gorm.DB) error { return s.forgetFile(tx, f, api.DeleteTitleRemoved) })
		if err != nil {
			return err
		}
		paths = append(paths, f.Path)
	}
	if deleteFiles {
		s.refreshFolders(ctx, paths)
	}
	return nil
}
