package subtitles

import (
	"context"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
)

const (
	searchQueueName         = "subtitles.search"
	downloadsQueueName      = "subtitles.downloads"
	sweepQueueName          = "subtitles.sweep"
	recomputeQueueName      = "subtitles.recompute-wanted"
	removeSidecarsQueueName = "subtitles.remove-sidecars"
	sweepPeriodicName       = "subtitles.sweep"

	queueAttempts = 3
	queueBackoff  = time.Minute
)

func queueConfig(name string) backlite.QueueConfig {
	return backlite.QueueConfig{Name: name, MaxAttempts: queueAttempts, Backoff: queueBackoff}
}

// searchTask searches a file's open tracks.
type searchTask struct {
	MediaFile domain.MediaFileID `json:"mediaFile"`
	Priority  Priority           `json:"priority"`
}

func (searchTask) Config() backlite.QueueConfig { return queueConfig(searchQueueName) }

// downloadsTask downloads the candidates that waited for quota.
type downloadsTask struct{}

func (downloadsTask) Config() backlite.QueueConfig { return queueConfig(downloadsQueueName) }

// sweepTask is the periodic pass: waiting downloads, upgrades, then the missing backlog.
type sweepTask struct{}

func (sweepTask) Config() backlite.QueueConfig { return queueConfig(sweepQueueName) }

// recomputeTask recomputes the wanted tracks of one batch of files after After, of one title or of all.
type recomputeTask struct {
	Title *domain.TitleID    `json:"title"`
	After domain.MediaFileID `json:"after"`
}

func (recomputeTask) Config() backlite.QueueConfig { return queueConfig(recomputeQueueName) }

// removeSidecarsTask deletes sidecar files no subtitle record points at any more.
type removeSidecarsTask struct {
	Paths []string `json:"paths"`
}

func (removeSidecarsTask) Config() backlite.QueueConfig { return queueConfig(removeSidecarsQueueName) }

func newSearchQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t searchTask) error { return s.searchFile(ctx, t.MediaFile, t.Priority) })
}

func newDownloadsQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ downloadsTask) error { return s.drainDownloads(ctx) })
}

func newSweepQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ sweepTask) error { return s.sweep(ctx) })
}

func newRecomputeQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t recomputeTask) error { return s.recompute(ctx, t) })
}

func newRemoveSidecarsQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, t removeSidecarsTask) error { return s.removeSidecars(ctx, t.Paths) })
}

// newSweepPeriodic runs the sweep on the upgrade-check interval (6 h by default).
func newSweepPeriodic(store *settings.Store) jobs.Periodic {
	return jobs.Periodic{
		Name: sweepPeriodicName,
		Schedule: func(ctx context.Context) (jobs.Schedule, error) {
			cfg, err := SettingsKey.Get(ctx, store)
			if err != nil {
				return nil, err
			}
			return jobs.Every(cfg.UpgradeCheckInterval.Std()), nil
		},
		Task: func() backlite.Task { return sweepTask{} },
	}
}
