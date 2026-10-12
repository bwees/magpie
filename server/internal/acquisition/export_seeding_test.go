package acquisition

import (
	"context"

	"magpie/internal/platform/bus"

	"github.com/mikestefanello/backlite"
)

// LibraryHandlers are Acquisition's handlers of Library's import events.
func LibraryHandlers(s *DownloadService) []bus.Handler {
	return []bus.Handler{newFileImportedHandler(s), newImportFailedHandler(s)}
}

// IsImportJob reports whether a queued task is one of the jobs Library's import events queue.
func IsImportJob(task backlite.Task) bool {
	switch task.(type) {
	case releaseImportedTask, importFailedTask:
		return true
	}
	return false
}

// RunDownloadTask runs a queued import or seeding job; it reports false for any other task.
func RunDownloadTask(ctx context.Context, s *DownloadService, task backlite.Task) (bool, error) {
	switch t := task.(type) {
	case releaseImportedTask:
		return true, s.ReleaseImported(ctx, t.Download)
	case importFailedTask:
		return true, s.DropFailedImport(ctx, t.Download)
	}
	return false, nil
}
