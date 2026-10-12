package migrate

import (
	"context"

	"github.com/mikestefanello/backlite"
)

const (
	queueName = "platform.migration"
	// A failed run shows in the wizard and the user starts it again, so it isn't retried.
	queueAttempts = 1
)

func (migrationTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: queueName, MaxAttempts: queueAttempts}
}

func newQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task migrationTask) error { return s.Run(ctx, task) })
}
