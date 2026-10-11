package activity

import (
	"context"
	"errors"
	"time"

	"magpie/internal/platform/jobs"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
)

const (
	defaultHistoryRetentionDays      = 90
	defaultNotificationRetentionDays = 90

	pruneQueueName   = "activity.prune"
	pruneInterval    = 24 * time.Hour
	pruneMaxAttempts = 3
	pruneBackoff     = 15 * time.Minute
)

var (
	historyRetentionDays      = settings.NewKey(owner.Activity, "historyRetentionDays", defaultHistoryRetentionDays)
	notificationRetentionDays = settings.NewKey(owner.Activity, "notificationRetentionDays", defaultNotificationRetentionDays)
)

type pruneTask struct{}

func (pruneTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: pruneQueueName, MaxAttempts: pruneMaxAttempts, Backoff: pruneBackoff}
}

func newPruneQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ pruneTask) error {
		return errors.Join(s.PruneHistory(ctx), s.PruneNotifications(ctx))
	})
}

func newPrunePeriodic() jobs.Periodic {
	return jobs.Periodic{
		Name:     pruneQueueName,
		Schedule: func(context.Context) (jobs.Schedule, error) { return jobs.Every(pruneInterval), nil },
		Task:     func() backlite.Task { return pruneTask{} },
	}
}
