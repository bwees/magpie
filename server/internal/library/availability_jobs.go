package library

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
)

const (
	reconcileQueueName    = "library.jellyfin-reconcile"
	reconcilePeriodicName = "library.jellyfin-reconcile"
	reconcileAttempts     = 3
	reconcileBackoff      = 5 * time.Minute

	watchBackoffFactor = 2
)

// backoff spaces reconnects to Jellyfin's WebSocket: from initial, doubling up to most.
type backoff struct {
	initial, most time.Duration
}

var (
	watchBackoff     = backoff{initial: time.Second, most: time.Minute}
	errSocketDropped = errors.New("library: Jellyfin's WebSocket closed")
)

type reconcileTask struct{}

func (reconcileTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: reconcileQueueName, MaxAttempts: reconcileAttempts, Backoff: reconcileBackoff}
}

func newReconcileQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ reconcileTask) error {
		return s.ReconcileJellyfin(ctx)
	})
}

// newReconcilePeriodic runs the reconcile on the Schedules setting.
func newReconcilePeriodic(store *settings.Store) jobs.Periodic {
	return jobs.Periodic{
		Name: reconcilePeriodicName,
		Schedule: func(ctx context.Context) (jobs.Schedule, error) {
			s, err := SchedulesKey.Get(ctx, store)
			if err != nil {
				return nil, err
			}
			return jobs.Every(s.JellyfinReconcile.Std()), nil
		},
		Task: func() backlite.Task { return reconcileTask{} },
	}
}

// startJellyfinWatch follows Jellyfin's WebSocket while the server runs.
func startJellyfinWatch(lc fx.Lifecycle, s *Service) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			wg.Go(func() { s.WatchJellyfin(ctx) })
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			wg.Wait()
			return nil
		},
	})
}

// WatchJellyfin confirms files from Jellyfin's LibraryChanged messages until ctx ends. It reconnects
// with backoff whenever the socket can't open or drops; the reconcile covers what it misses meanwhile.
func (s *Service) WatchJellyfin(ctx context.Context) {
	delay := s.watch.initial
	for {
		connected, err := s.followChanges(ctx)
		if ctx.Err() != nil {
			return
		}
		if connected {
			delay = s.watch.initial
		}
		slog.WarnContext(ctx, "library: not following Jellyfin's library changes", "error", err, "retry_in", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*watchBackoffFactor, s.watch.most)
	}
}

// followChanges handles one socket's messages. connected reports whether the socket opened.
func (s *Service) followChanges(ctx context.Context) (connected bool, err error) {
	changes, err := s.deps.Jellyfin.Changes(ctx)
	if err != nil {
		return false, err
	}
	for change := range changes {
		if err := s.HandleChange(ctx, change); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "library: handling a Jellyfin library change failed; the reconcile retries it", "error", err)
		}
	}
	return true, errSocketDropped
}
