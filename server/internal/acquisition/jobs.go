package acquisition

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
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

	settleQueue       = "acquisition.automatic-grab"
	settleMaxAttempts = 3
	settleBackoff     = 10 * time.Second

	removeTitleQueue       = "acquisition.remove-title"
	removeTitleMaxAttempts = 5
	removeTitleBackoff     = time.Minute

	rssSyncQueue   = "acquisition.rss-sync"
	rssFeedQueue   = "acquisition.rss-feed"
	rssMaxAttempts = 2
	rssBackoff     = time.Minute

	pendingCheckQueue  = "acquisition.pending-check"
	pendingMaxAttempts = 1
	pendingBackoff     = time.Minute

	missingSweepQueue = "acquisition.missing-sweep"
	cutoffSweepQueue  = "acquisition.cutoff-sweep"
	sweepMaxAttempts  = 3
	sweepBackoff      = 10 * time.Minute
)

// searchTask runs a search in the background: on title add, on release, from a sweep, or the fresh AUTO
// search after a rejection left nothing cached to fall back to.
type searchTask struct {
	Title    domain.TitleID     `json:"title"`
	Episodes []domain.EpisodeID `json:"episodes"`
	Trigger  api.SearchTrigger  `json:"trigger"`
	// Removed are downloads the user removed for this search, which may not be dropped yet.
	Removed []domain.DownloadID `json:"removed,omitempty"`
}

func (searchTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: searchQueue, MaxAttempts: searchMaxAttempts, Backoff: searchBackoff}
}

// key is (title, episodes, trigger): a search with the same key that is still queued isn't queued again.
func (t searchTask) key() string {
	episodes := make([]string, 0, len(t.Episodes))
	for _, ep := range t.Episodes {
		episodes = append(episodes, string(ep))
	}
	slices.Sort(episodes)
	return strings.Join([]string{string(t.Title), strings.Join(episodes, ","), string(t.Trigger)}, "|")
}

// queueSearch queues a search in tx unless one with its key is still queued.
func queueSearch(tx *gorm.DB, q jobs.Enqueuer, t searchTask, now time.Time) error {
	claimed, err := claimSearchKey(tx, t.key(), now)
	if err != nil || !claimed {
		return err
	}
	return q.EnqueueTx(tx, t)
}

// queueSearchAt is queueSearch for a search that must not start before at.
func queueSearchAt(tx *gorm.DB, q searchJobs, t searchTask, at time.Time) error {
	claimed, err := claimSearchKey(tx, t.key(), at)
	if err != nil || !claimed {
		return err
	}
	return q.EnqueueTxAt(tx, t, at)
}

func newSearchQueue(s *SearchService) backlite.Queue {
	return backlite.NewQueue(s.runSearchJob)
}

// runSearchJob frees the search's key, so the same search can be queued again while this one runs, then
// starts it. A title or episode removed since it was queued ends the job.
func (s *SearchService) runSearchJob(ctx context.Context, t searchTask) error {
	if err := s.deps.Repo.releaseSearchKey(ctx, t.key()); err != nil {
		return err
	}
	_, err := s.Start(ctx, SearchRequest(t))
	if errors.Is(err, ErrTitleNotFound) || errors.Is(err, ErrInvalidSearch) {
		return nil
	}
	return err
}

// settleTask is a search's automatic grab, queued once the search settled.
type settleTask struct {
	Search domain.SearchID `json:"search"`
}

func (settleTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: settleQueue, MaxAttempts: settleMaxAttempts, Backoff: settleBackoff}
}

func newSettleQueue(s *DownloadService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t settleTask) error {
		return s.Settle(ctx, t.Search)
	})
}

// removeTitleTask takes a removed title's downloads out of qBittorrent.
type removeTitleTask struct {
	Title       domain.TitleID `json:"title"`
	DeleteFiles bool           `json:"deleteFiles"`
}

func (removeTitleTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: removeTitleQueue, MaxAttempts: removeTitleMaxAttempts, Backoff: removeTitleBackoff}
}

func newRemoveTitleQueue(s *DownloadService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t removeTitleTask) error {
		return s.RemoveTitle(ctx, t.Title, t.DeleteFiles)
	})
}

// rssSyncTask queues one feed task per indexer.
type rssSyncTask struct{}

func (rssSyncTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: rssSyncQueue, MaxAttempts: rssMaxAttempts, Backoff: rssBackoff}
}

func newRSSSyncQueue(s *SearchService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ rssSyncTask) error {
		return s.QueueFeeds(ctx)
	})
}

func newRSSSyncPeriodic(store *settings.Store) jobs.Periodic {
	return jobs.Periodic{
		Name:     rssSyncQueue,
		Schedule: scheduleOf(store, func(s Schedules) settings.Duration { return s.RSSInterval }),
		Task:     func() backlite.Task { return rssSyncTask{} },
	}
}

// rssFeedTask reads one indexer's feed.
type rssFeedTask struct {
	Indexer int `json:"indexer"`
}

func (rssFeedTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: rssFeedQueue, MaxAttempts: rssMaxAttempts, Backoff: rssBackoff}
}

func newRSSFeedQueue(s *SearchService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t rssFeedTask) error {
		return s.SyncFeed(ctx, t.Indexer)
	})
}

// pendingCheckTask grabs the held releases whose delay passed.
type pendingCheckTask struct{}

func (pendingCheckTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: pendingCheckQueue, MaxAttempts: pendingMaxAttempts, Backoff: pendingBackoff}
}

func newPendingCheckQueue(s *SearchService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ pendingCheckTask) error {
		return s.ReviewPending(ctx)
	})
}

func newPendingCheckTicker(r *Repository) jobs.Ticker {
	return jobs.Ticker{
		Name:     pendingCheckQueue,
		Interval: func(context.Context) (time.Duration, error) { return PendingCheckInterval, nil },
		Enqueue: func(ctx context.Context, q jobs.Enqueuer) error {
			held, err := r.hasPending(ctx)
			if err != nil || !held {
				return err
			}
			return q.Enqueue(ctx, pendingCheckTask{})
		},
	}
}

type missingSweepTask struct{}

func (missingSweepTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: missingSweepQueue, MaxAttempts: sweepMaxAttempts, Backoff: sweepBackoff}
}

func newMissingSweepQueue(s *SearchService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ missingSweepTask) error {
		return s.SweepMissing(ctx)
	})
}

func newMissingSweepPeriodic(store *settings.Store) jobs.Periodic {
	return jobs.Periodic{
		Name:     missingSweepQueue,
		Schedule: scheduleOf(store, func(s Schedules) settings.Duration { return s.MissingSweepInterval }),
		Task:     func() backlite.Task { return missingSweepTask{} },
	}
}

type cutoffSweepTask struct{}

func (cutoffSweepTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: cutoffSweepQueue, MaxAttempts: sweepMaxAttempts, Backoff: sweepBackoff}
}

func newCutoffSweepQueue(s *SearchService) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ cutoffSweepTask) error {
		return s.SweepCutoffUnmet(ctx)
	})
}

func newCutoffSweepPeriodic(store *settings.Store) jobs.Periodic {
	return jobs.Periodic{
		Name:     cutoffSweepQueue,
		Schedule: scheduleOf(store, func(s Schedules) settings.Duration { return s.CutoffSweepInterval }),
		Task:     func() backlite.Task { return cutoffSweepTask{} },
	}
}

// scheduleOf follows one of the Schedules settings.
func scheduleOf(store *settings.Store, interval func(Schedules) settings.Duration) func(context.Context) (jobs.Schedule, error) {
	return func(ctx context.Context) (jobs.Schedule, error) {
		s, err := SchedulesKey.Get(ctx, store)
		if err != nil {
			return nil, err
		}
		return jobs.Every(interval(s).Std()), nil
	}
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
