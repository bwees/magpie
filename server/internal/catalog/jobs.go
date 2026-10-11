package catalog

import (
	"context"
	"errors"
	"strings"
	"time"

	"magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/platform/jobs"
	"magpie/internal/platform/settings"
	requestsapi "magpie/internal/requests/api"

	"github.com/mikestefanello/backlite"
)

const (
	addTitleQueueName      = "catalog.add-title"
	refreshTitleQueueName  = "catalog.refresh-title"
	movieRefreshQueueName  = "catalog.refresh-movies"
	seriesRefreshQueueName = "catalog.refresh-series"
	releaseClockQueueName  = "catalog.release-clock"

	periodicNameSeparator = "."

	// Database errors retry through backlite; TMDB errors in add-title retry on addTitleRetries instead.
	queueAttempts = 3
	queueBackoff  = time.Minute
)

// addTitleRetries is the wait before each retry of a TMDB failure while adding a Title. Once they run
// out, TitleAddFailed is published.
var addTitleRetries = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}

var errNotInJob = errors.New("catalog: a delayed retry needs a running job")

type addTitleTask struct {
	Request requestsapi.RequestApproved `json:"request"`
	// Retries are the waits left before giving up.
	Retries []time.Duration `json:"retries"`
}

func (addTitleTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: addTitleQueueName, MaxAttempts: queueAttempts, Backoff: queueBackoff}
}

type refreshTitleTask struct {
	Title domain.TitleID `json:"title"`
}

func (refreshTitleTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: refreshTitleQueueName, MaxAttempts: queueAttempts, Backoff: queueBackoff}
}

type movieRefreshTask struct{}

func (movieRefreshTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: movieRefreshQueueName, MaxAttempts: queueAttempts, Backoff: queueBackoff}
}

// seriesRefreshTask refreshes the series with one api.SeriesStatus*.
type seriesRefreshTask struct {
	Status string `json:"status"`
}

func (seriesRefreshTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: seriesRefreshQueueName, MaxAttempts: queueAttempts, Backoff: queueBackoff}
}

type releaseClockTask struct{}

func (releaseClockTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: releaseClockQueueName, MaxAttempts: queueAttempts, Backoff: queueBackoff}
}

// delayedEnqueue adds a task that runs after wait.
type delayedEnqueue func(ctx context.Context, task backlite.Task, wait time.Duration) error

// enqueueFromRunningJob uses the backlite client that is running the current job.
func enqueueFromRunningJob(ctx context.Context, task backlite.Task, wait time.Duration) error {
	client := backlite.FromContext(ctx)
	if client == nil {
		return errNotInJob
	}
	_, err := client.Add(task).Ctx(ctx).Wait(wait).Save()
	return err
}

func newAddTitleQueue(s *Service) backlite.Queue {
	return addTitleQueue(s, enqueueFromRunningJob)
}

func addTitleQueue(s *Service, later delayedEnqueue) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task addTitleTask) error {
		return addTitle(ctx, s, later, task)
	})
}

// addTitle retries TMDB failures on the task's own schedule and then reports them; a Title TMDB doesn't
// know fails at once.
func addTitle(ctx context.Context, s *Service, later delayedEnqueue, task addTitleTask) error {
	err := s.AddRequestedTitle(ctx, task.Request)
	if err == nil || !isTmdbError(err) {
		return err
	}
	if !errors.Is(err, ErrTmdbNotFound) && len(task.Retries) > 0 {
		next := addTitleTask{Request: task.Request, Retries: task.Retries[1:]}
		return later(ctx, next, task.Retries[0])
	}
	return s.FailRequestedTitle(ctx, task.Request, err)
}

func newRefreshTitleQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task refreshTitleTask) error {
		return s.pauseOnUnauthorized(ctx, s.RefreshTitle(ctx, task.Title))
	})
}

func newMovieRefreshQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ movieRefreshTask) error { return s.RefreshMovies(ctx) })
}

func newReleaseClockQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, _ releaseClockTask) error { return s.RunReleaseClock(ctx) })
}

func newReleaseClockPeriodic(store *settings.Store) jobs.Periodic {
	return jobs.Periodic{
		Name:     releaseClockQueueName,
		Schedule: jobs.EverySetting(store, releaseClockInterval),
		Task:     func() backlite.Task { return releaseClockTask{} },
	}
}

func newSeriesRefreshQueue(s *Service) backlite.Queue {
	return backlite.NewQueue(func(ctx context.Context, task seriesRefreshTask) error { return s.RefreshSeries(ctx, task.Status) })
}

func newMovieRefreshPeriodic(s *Service) jobs.Periodic {
	return refreshPeriodic(s, movieRefreshQueueName, movieRefreshInterval, movieRefreshTask{})
}

// newSeriesRefreshPeriodic refreshes the series with one api.SeriesStatus* on that status' interval.
func newSeriesRefreshPeriodic(s *Service, status string) jobs.Periodic {
	name := seriesRefreshQueueName + periodicNameSeparator + strings.ToLower(status)
	return refreshPeriodic(s, name, seriesRefreshInterval(status), seriesRefreshTask{Status: status})
}

func newContinuingSeriesRefreshPeriodic(s *Service) jobs.Periodic {
	return newSeriesRefreshPeriodic(s, api.SeriesStatusContinuing)
}

func newEndedSeriesRefreshPeriodic(s *Service) jobs.Periodic {
	return newSeriesRefreshPeriodic(s, api.SeriesStatusEnded)
}

// refreshPeriodic pauses while an invalid TMDB key has paused refreshing.
func refreshPeriodic(s *Service, name string, key settings.Key[settings.Duration], task backlite.Task) jobs.Periodic {
	interval := jobs.EverySetting(s.settings, key)
	return jobs.Periodic{
		Name: name,
		Schedule: func(ctx context.Context) (jobs.Schedule, error) {
			if paused, err := s.RefreshPaused(ctx); err != nil || paused {
				return nil, err
			}
			return interval(ctx)
		},
		Task: func() backlite.Task { return task },
	}
}
