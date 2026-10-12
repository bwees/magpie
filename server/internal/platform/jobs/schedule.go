package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"magpie/internal/platform/db"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// scheduleCheckInterval bounds how late a periodic job can start.
const scheduleCheckInterval = 30 * time.Second

// Schedule picks the next run after a given time.
type Schedule interface {
	Next(after time.Time) time.Time
}

// Every runs at a fixed interval.
type Every time.Duration

func (e Every) Next(after time.Time) time.Time {
	return after.Add(time.Duration(e))
}

// DailyAt runs once a day at a wall-clock time in the server's time zone.
type DailyAt struct {
	Hour   int
	Minute int
}

func (d DailyAt) Next(after time.Time) time.Time {
	local := after.Local()
	next := time.Date(local.Year(), local.Month(), local.Day(), d.Hour, d.Minute, 0, 0, time.Local)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// Periodic enqueues Task on Schedule. Schedule is read before every check, so it can follow a setting;
// a nil Schedule pauses the job.
type Periodic struct {
	// Name is stable across releases; it keys the next run time.
	Name     string
	Schedule func(ctx context.Context) (Schedule, error)
	Task     func() backlite.Task
}

// AsPeriodic provides constructor's Periodic to the scheduler.
func AsPeriodic(constructor any) any {
	return fx.Annotate(constructor, fx.ResultTags(periodicsGroup))
}

// EverySetting reads a periodic job's interval from a settings key.
func EverySetting(store *settings.Store, key settings.Key[settings.Duration]) func(context.Context) (Schedule, error) {
	return func(ctx context.Context) (Schedule, error) {
		interval, err := key.Get(ctx, store)
		if err != nil {
			return nil, err
		}
		return Every(interval.Std()), nil
	}
}

// Ticker polls on a short, possibly changing interval. Its only job is to enqueue: Enqueue gets nothing
// but an Enqueuer, and the real work runs as a job with retries.
type Ticker struct {
	Name     string
	Interval func(ctx context.Context) (time.Duration, error)
	Enqueue  func(ctx context.Context, q Enqueuer) error
}

// AsTicker provides constructor's Ticker to the scheduler.
func AsTicker(constructor any) any {
	return fx.Annotate(constructor, fx.ResultTags(tickersGroup))
}

type scheduleRow struct {
	Name      string `gorm:"primaryKey"`
	NextRunAt time.Time
}

func (scheduleRow) TableName() string { return "platform_schedules" }

// Scheduler runs the tickers, including its own that enqueues due periodic jobs.
type Scheduler struct {
	db        *db.DB
	queue     *Queue
	periodics []Periodic
	tickers   []Ticker
	now       func() time.Time
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

type SchedulerParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	DB        *db.DB
	Queue     *Queue
	Periodics []Periodic `group:"job_periodics"`
	Tickers   []Ticker   `group:"job_tickers"`
}

func NewScheduler(p SchedulerParams) *Scheduler {
	s := &Scheduler{db: p.DB, queue: p.Queue, periodics: p.Periodics, now: time.Now}
	s.tickers = append(p.Tickers, Ticker{
		Name:     "platform.periodic-jobs",
		Interval: func(context.Context) (time.Duration, error) { return scheduleCheckInterval, nil },
		Enqueue:  func(ctx context.Context, _ Enqueuer) error { return s.EnqueueDue(ctx) },
	})
	p.Lifecycle.Append(fx.StartStopHook(s.Start, s.Stop))
	return s
}

func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	for _, ticker := range s.tickers {
		s.wg.Go(func() { s.runTicker(ctx, ticker) })
	}
}

// Stop cancels the tickers and waits for them until ctx ends.
func (s *Scheduler) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	return waitFor(ctx, &s.wg)
}

func (s *Scheduler) runTicker(ctx context.Context, ticker Ticker) {
	for {
		interval, err := ticker.Interval(ctx)
		if err != nil || interval <= 0 {
			slog.ErrorContext(ctx, "ticker interval unavailable", "ticker", ticker.Name, "interval", interval, "error", err)
			interval = scheduleCheckInterval
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err := ticker.Enqueue(ctx, s.queue); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "ticker failed to enqueue", "ticker", ticker.Name, "error", err)
		}
	}
}

// EnqueueDue enqueues every periodic job whose next run has passed and books its following run.
// A job seen for the first time is booked without running.
func (s *Scheduler) EnqueueDue(ctx context.Context) error {
	var errs []error
	for _, p := range s.periodics {
		if err := s.enqueueIfDue(ctx, p); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Scheduler) enqueueIfDue(ctx context.Context, p Periodic) error {
	schedule, err := p.Schedule(ctx)
	if err != nil || schedule == nil {
		return err
	}
	now := s.now()
	soonest := schedule.Next(now).UTC()

	return s.db.Write(ctx, func(tx *gorm.DB) error {
		var rows []scheduleRow
		if err := tx.Where("name = ?", p.Name).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return tx.Create(&scheduleRow{Name: p.Name, NextRunAt: soonest}).Error
		}
		row := rows[0]
		switch {
		case !row.NextRunAt.After(now):
			if err := s.queue.EnqueueTx(tx, p.Task()); err != nil {
				return err
			}
			row.NextRunAt = soonest
		case row.NextRunAt.After(soonest):
			// The schedule got shorter since the run was booked.
			row.NextRunAt = soonest
		default:
			return nil
		}
		return tx.Save(&row).Error
	})
}
