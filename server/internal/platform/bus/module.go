package bus

import (
	"context"
	"time"

	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/jobs"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
)

const (
	pruneQueueName   = "platform.prune-events"
	pruneInterval    = 24 * time.Hour
	pruneMaxAttempts = 3
	pruneBackoff     = 10 * time.Minute
)

var Module = fx.Module("bus",
	fx.Provide(
		New,
		NewDispatcher,
		httpserver.AsController(NewController),
		jobs.AsQueue(newPruneQueue),
		jobs.AsPeriodic(newPrunePeriodic),
	),
	fx.Invoke(func(lc fx.Lifecycle, d *Dispatcher) {
		lc.Append(fx.Hook{OnStart: d.Start, OnStop: func(context.Context) error { d.Stop(); return nil }})
	}),
)

type pruneTask struct{}

func (pruneTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: pruneQueueName, MaxAttempts: pruneMaxAttempts, Backoff: pruneBackoff}
}

func newPruneQueue(d *Dispatcher) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ pruneTask) error { return d.Prune(ctx) })
}

func newPrunePeriodic() jobs.Periodic {
	return jobs.Periodic{
		Name:     pruneQueueName,
		Schedule: func(context.Context) (jobs.Schedule, error) { return jobs.Every(pruneInterval), nil },
		Task:     func() backlite.Task { return pruneTask{} },
	}
}
