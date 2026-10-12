package library

import (
	"context"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/platform/bus"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	handlerScanFolderMatched   = "library.link-on-scan-folder-matched"
	handlerEpisodeGroupChanged = "library.rename-on-episode-group-changed"

	scanQueueName   = "library.scan"
	linkQueueName   = "library.link-folder"
	renameQueueName = "library.rename-episodes"

	// A failed scan shows in its status and the admin starts another, so it isn't retried.
	scanQueueAttempts = 1
)

type scanTask struct{}

func (scanTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: scanQueueName, MaxAttempts: scanQueueAttempts}
}

// linkTask links one matched scan folder; the scan folder's status says whether it ran.
type linkTask struct {
	Folder string `json:"folder"`
}

func (linkTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: linkQueueName, MaxAttempts: importQueueAttempts, Backoff: importQueueBackoff}
}

type renameTask struct {
	Change catalogapi.EpisodeGroupChanged `json:"change"`
}

func (renameTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: renameQueueName, MaxAttempts: importQueueAttempts, Backoff: importQueueBackoff}
}

func newScanQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ scanTask) error { return s.RunScan(ctx) })
}

func newLinkQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task linkTask) error { return s.LinkFolder(ctx, task.Folder) })
}

func newRenameQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task renameTask) error { return s.RenameForGroup(ctx, task.Change) })
}

// newScanFolderMatchedHandler queues linking the files of a folder Catalog matched; probing them is disk work.
func newScanFolderMatchedHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerScanFolderMatched,
		EventTypes: []string{catalogapi.TypeScanFolderMatched},
		Handle: func(ctx context.Context, tx *gorm.DB, ev bus.Event) error {
			matched, err := bus.Decode[catalogapi.ScanFolderMatched](ev)
			if err != nil {
				return err
			}
			return s.QueueLink(ctx, tx, matched)
		},
	}
}

// newEpisodeGroupChangedHandler queues renaming the files of the episodes a renumbering moved.
func newEpisodeGroupChangedHandler(s *Service) bus.Handler {
	return bus.Handler{
		Name:       handlerEpisodeGroupChanged,
		EventTypes: []string{catalogapi.TypeEpisodeGroupChanged},
		Handle: func(_ context.Context, tx *gorm.DB, ev bus.Event) error {
			changed, err := bus.Decode[catalogapi.EpisodeGroupChanged](ev)
			if err != nil || len(changed.Moves) == 0 {
				return err
			}
			return s.deps.Jobs.EnqueueTx(tx, renameTask{Change: changed})
		},
	}
}
