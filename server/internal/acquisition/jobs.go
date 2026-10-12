package acquisition

import (
	"context"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
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

const (
	searchQueue       = "acquisition.search"
	searchMaxAttempts = 3
	searchBackoff     = time.Minute

	downloadCheckQueue       = "acquisition.download-check"
	downloadCheckMaxAttempts = 3
	downloadCheckBackoff     = 10 * time.Second
	downloadPollTicker       = "acquisition.download-poll"
)

// searchTask runs a search in the background, such as the fresh AUTO search after a rejection
// left nothing cached to fall back to.
type searchTask struct {
	Title    domain.TitleID     `json:"title"`
	Episodes []domain.EpisodeID `json:"episodes"`
	Trigger  api.SearchTrigger  `json:"trigger"`
}

func (searchTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: searchQueue, MaxAttempts: searchMaxAttempts, Backoff: searchBackoff}
}

func newSearchQueue(s *SearchService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t searchTask) error {
		_, err := s.Start(ctx, SearchRequest(t))
		return err
	})
}

// downloadCheckTask acts on what the poll saw for one download: metadata, completion, failure or a timeout.
type downloadCheckTask struct {
	Download domain.DownloadID `json:"download"`
}

func (downloadCheckTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: downloadCheckQueue, MaxAttempts: downloadCheckMaxAttempts, Backoff: downloadCheckBackoff}
}

func newDownloadCheckQueue(s *DownloadService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t downloadCheckTask) error {
		return s.Check(ctx, t.Download)
	})
}

func newDownloadPollTicker(s *DownloadService) jobs.Ticker {
	return jobs.Ticker{Name: downloadPollTicker, Interval: s.PollInterval, Enqueue: s.Poll}
}

// syncIndexersOnStart fills the indexer list right away instead of an hour after the first boot.
func syncIndexersOnStart(lc fx.Lifecycle, q jobs.Enqueuer) {
	lc.Append(fx.StartHook(func(ctx context.Context) error {
		return q.Enqueue(ctx, indexerSyncTask{})
	}))
}
