package acquisition

import (
	"context"
	"time"

	"magpie/internal/platform/jobs"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
)

const (
	indexerSyncQueue       = "acquisition.indexer-sync"
	indexerSyncMaxAttempts = 3
	indexerSyncBackoff     = 5 * time.Minute
)

type indexerSyncTask struct{}

func (indexerSyncTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: indexerSyncQueue, MaxAttempts: indexerSyncMaxAttempts, Backoff: indexerSyncBackoff}
}

func newIndexerSyncQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ indexerSyncTask) error {
		return s.SyncIndexers(ctx)
	})
}

func newIndexerSyncPeriodic(s *Service) jobs.Periodic {
	return jobs.Periodic{
		Name: indexerSyncQueue,
		Schedule: func(ctx context.Context) (jobs.Schedule, error) {
			options, err := s.IndexerOptions(ctx)
			if err != nil {
				return nil, err
			}
			return jobs.Every(options.SyncInterval.Std()), nil
		},
		Task: func() backlite.Task { return indexerSyncTask{} },
	}
}

// syncIndexersOnStart fills the indexer list right away instead of an hour after the first boot.
func syncIndexersOnStart(lc fx.Lifecycle, q jobs.Enqueuer) {
	lc.Append(fx.StartHook(func(ctx context.Context) error {
		return q.Enqueue(ctx, indexerSyncTask{})
	}))
}
