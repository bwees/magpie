// Package jobs runs backlite on the app's SQLite file, plus periodic schedules and tickers that enqueue into it.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"magpie/internal/platform/db"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

const (
	workers = 4
	// releaseAfter returns a claimed task to its queue if a crash left it unfinished.
	releaseAfter    = time.Hour
	cleanupInterval = time.Hour

	queuesGroup    = `group:"job_queues"`
	periodicsGroup = `group:"job_periodics"`
	tickersGroup   = `group:"job_tickers"`
)

var ErrNotInTransaction = errors.New("jobs: EnqueueTx needs a transaction from db.Write")

// Enqueuer adds tasks. It is all a ticker gets, so tickers can only enqueue.
type Enqueuer interface {
	Enqueue(ctx context.Context, task backlite.Task) error
	// EnqueueTx adds the task in the caller's transaction; it runs only if that transaction commits.
	EnqueueTx(tx *gorm.DB, task backlite.Task) error
}

type Queue struct {
	client *backlite.Client
}

var Module = fx.Module("jobs",
	fx.Provide(
		New,
		func(q *Queue) Enqueuer { return q },
		NewScheduler,
	),
	fx.Invoke(func(*Queue, *Scheduler) {}),
)

type Params struct {
	fx.In

	Lifecycle fx.Lifecycle
	DB        *db.DB
	Queues    []backlite.Queue `group:"job_queues"`
}

// AsQueue provides constructor's backlite.Queue to the job runner.
func AsQueue(constructor any) any {
	return fx.Annotate(constructor, fx.ResultTags(queuesGroup))
}

// New installs backlite's tables, registers every queue and runs the workers while the app runs.
func New(p Params) (*Queue, error) {
	q, err := Open(p.DB, p.Queues...)
	if err != nil {
		return nil, err
	}
	var cancel context.CancelFunc
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var runCtx context.Context
			runCtx, cancel = context.WithCancel(context.Background())
			q.client.Start(runCtx)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			defer cancel()
			if !q.client.Stop(ctx) {
				slog.WarnContext(ctx, "jobs still running at shutdown; they will be released and retried")
			}
			return nil
		},
	})
	return q, nil
}

// Open prepares a queue without starting workers.
func Open(d *db.DB, queues ...backlite.Queue) (*Queue, error) {
	writer, err := d.WriterSQL()
	if err != nil {
		return nil, err
	}
	client, err := backlite.NewClient(backlite.ClientConfig{
		DB:              writer,
		Logger:          slog.Default(),
		NumWorkers:      workers,
		ReleaseAfter:    releaseAfter,
		CleanupInterval: cleanupInterval,
	})
	if err != nil {
		return nil, err
	}
	if err := client.Install(); err != nil {
		return nil, err
	}
	for _, queue := range queues {
		client.Register(queue)
	}
	return &Queue{client: client}, nil
}

// Start runs the workers until ctx ends. fx-managed queues start themselves.
func (q *Queue) Start(ctx context.Context) {
	q.client.Start(ctx)
}

func (q *Queue) Enqueue(ctx context.Context, task backlite.Task) error {
	_, err := q.client.Add(task).Ctx(ctx).Save()
	return err
}

func (q *Queue) EnqueueTx(tx *gorm.DB, task backlite.Task) error {
	sqlTx, ok := tx.Statement.ConnPool.(*sql.Tx)
	if !ok {
		return ErrNotInTransaction
	}
	if err := db.AfterCommit(tx, q.client.Notify); err != nil {
		return err
	}
	_, err := q.client.Add(task).Ctx(tx.Statement.Context).Tx(sqlTx).Save()
	return err
}
