package library

import (
	"context"
	"time"

	"magpie/internal/domain"
	"magpie/internal/library/api/contracttest"
	"magpie/internal/platform/bus"
	"magpie/internal/platform/db"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

var (
	NewDownloadCompletedHandler   = newDownloadCompletedHandler
	NewTranscodeVerifiedHandler   = newTranscodeVerifiedHandler
	NewScanFolderMatchedHandler   = newScanFolderMatchedHandler
	NewEpisodeGroupChangedHandler = newEpisodeGroupChangedHandler
	NewTitleRemovedHandler        = newTitleRemovedHandler
)

// RunSwapTask runs a queued swap task. It reports false for a task of another queue.
func RunSwapTask(ctx context.Context, s *Service, task backlite.Task) (bool, error) {
	t, ok := task.(swapTask)
	if !ok {
		return false, nil
	}
	return true, s.Swap(ctx, t.Verified)
}

// RunTask runs a task Library queued; later receives the retries an import books. It reports false for
// a task of another context.
func RunTask(ctx context.Context, s *Service, task backlite.Task, later func(backlite.Task, time.Duration)) (bool, error) {
	switch t := task.(type) {
	case importTask:
		return true, s.runImport(ctx, t, func(_ context.Context, next backlite.Task, wait time.Duration) error {
			later(next, wait)
			return nil
		})
	case scanTask:
		return true, s.RunScan(ctx)
	case linkTask:
		return true, s.LinkFolder(ctx, t.Folder)
	case renameTask:
		return true, s.RenameForGroup(ctx, t.Change)
	case removeTitleTask:
		return true, s.RemoveTitle(ctx, t.Title, t.DeleteFiles)
	}
	return false, nil
}

func SetClock(s *Service, now func() time.Time) { s.now = now }

// SeedFiles stores contract-suite files, with a root folder per distinct root path.
func SeedFiles(ctx context.Context, d *db.DB, files []contracttest.StoredFile) error {
	return d.Write(ctx, func(tx *gorm.DB) error {
		roots := map[string]string{}
		for _, f := range files {
			id, ok := roots[f.File.RootFolder]
			if !ok {
				var rows []rootFolderRow
				if err := tx.Where("path = ?", f.File.RootFolder).Find(&rows).Error; err != nil {
					return err
				}
				if len(rows) > 0 {
					id = rows[0].ID
				} else {
					id = f.File.RootFolder
					if err := insertRootFolder(tx, RootFolder{ID: id, Path: f.File.RootFolder}, f.File.ImportedAt); err != nil {
						return err
					}
				}
				roots[f.File.RootFolder] = id
			}
			if err := insertMediaFile(tx, storedFile{MediaFile: f.File, RootFolderID: id, ETag: f.ETag}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Handle runs a bus handler on one message in its own transaction, as the dispatcher would.
func Handle(ctx context.Context, d *db.DB, h bus.Handler, m domain.Message) error {
	return d.Write(ctx, func(tx *gorm.DB) error { return h.Handle(ctx, tx, bus.EventOf(m)) })
}
