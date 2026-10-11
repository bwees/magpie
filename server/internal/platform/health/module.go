package health

import (
	"context"
	"time"

	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/jobs"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
)

const (
	queueName       = "platform.health-check"
	CheckInterval   = time.Minute
	taskMaxAttempts = 3
	taskBackoff     = 10 * time.Second
)

var Module = fx.Module("health",
	fx.Provide(
		NewChecker,
		jobs.AsQueue(newQueue),
		jobs.AsPeriodic(newPeriodic),
	),
	fx.Invoke(func(lc fx.Lifecycle, q jobs.Enqueuer) {
		lc.Append(fx.StartHook(func(ctx context.Context) error { return q.Enqueue(ctx, CheckTask{}) }))
	}),
)

// CheckTask probes one service, or every service when Service is empty.
type CheckTask struct {
	Service platformapi.Service `json:"service"`
}

func (CheckTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: queueName, MaxAttempts: taskMaxAttempts, Backoff: taskBackoff}
}

func newQueue(c *Checker) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task CheckTask) error {
		if task.Service == "" {
			return c.CheckAll(ctx)
		}
		_, err := c.Check(ctx, task.Service)
		return err
	})
}

func newPeriodic() jobs.Periodic {
	return jobs.Periodic{
		Name:     queueName,
		Schedule: func(context.Context) (jobs.Schedule, error) { return jobs.Every(CheckInterval), nil },
		Task:     func() backlite.Task { return CheckTask{} },
	}
}
