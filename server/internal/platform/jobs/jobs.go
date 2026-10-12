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
	// settleTime is how long backlite gets to record the results of jobs cancelled at shutdown.
	settleTime = time.Second

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
	client  *backlite.Client
	running *runningJobs
	halt    context.CancelFunc
}

var Module = fx.Module("jobs",
	fx.Provide(
		New,
		func(q *Queue) Enqueuer { return q },
		NewScheduler,
	),
	fx.Invoke(registerQueues, func(*Scheduler) {}),
)

type Params struct {
	fx.In

	Lifecycle fx.Lifecycle
	DB        *db.DB
}

type queuesParams struct {
	fx.In

	Queue  *Queue
	Queues []backlite.Queue `group:"job_queues"`
}

// registerQueues runs after construction, so queue processors and event handlers can depend on the
// Enqueuer without a dependency cycle.
func registerQueues(p queuesParams) {
	p.Queue.Register(p.Queues...)
}

// AsQueue provides constructor's backlite.Queue to the job runner.
func AsQueue(constructor any) any {
	return fx.Annotate(constructor, fx.ResultTags(queuesGroup))
}

// New installs backlite's tables and runs the workers while the app runs. Module registers every queue
// before the app starts.
func New(p Params) (*Queue, error) {
	q, err := Open(p.DB)
	if err != nil {
		return nil, err
	}
	p.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			q.Start(context.Background())
			return nil
		},
		OnStop: q.Stop,
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
	q := &Queue{client: client, running: newRunningJobs()}
	q.Register(queues...)
	return q, nil
}

// Register adds queues. Call it before Start.
func (q *Queue) Register(queues ...backlite.Queue) {
	for _, queue := range queues {
		q.client.Register(cancellableQueue{Queue: queue, running: q.running})
	}
}

// Start runs the workers until Stop or until ctx ends. fx-managed queues start themselves.
func (q *Queue) Start(ctx context.Context) {
	ctx, q.halt = context.WithCancel(ctx)
	q.client.Start(ctx)
}

// Stop cancels running jobs and waits for them until ctx ends. A cancelled job counts as a failed
// attempt, like any other error.
func (q *Queue) Stop(ctx context.Context) error {
	defer q.halt()

	// Backlite's own Stop can wait forever: a task its fetcher hands over while the workers exit is
	// never picked up. So it gets only settleTime after the jobs return, to record their results.
	settle, endSettle := context.WithCancel(ctx)
	defer endSettle()
	workersStopped := make(chan bool, 1)
	go func() { workersStopped <- q.client.Stop(settle) }()

	err := q.running.cancelAndWait(ctx)
	settleTimer := time.AfterFunc(settleTime, endSettle)
	defer settleTimer.Stop()
	if !<-workersStopped {
		slog.WarnContext(ctx, "jobs: workers did not all stop; unfinished tasks are released and retried later")
	}
	return err
}

func (q *Queue) Enqueue(ctx context.Context, task backlite.Task) error {
	_, err := q.client.Add(task).Ctx(ctx).Save()
	return err
}

func (q *Queue) EnqueueTx(tx *gorm.DB, task backlite.Task) error {
	return q.EnqueueTxAt(tx, task, time.Time{})
}

// EnqueueTxAt is EnqueueTx for a task that must not run before at. A zero at runs it as soon as possible.
func (q *Queue) EnqueueTxAt(tx *gorm.DB, task backlite.Task, at time.Time) error {
	sqlTx, ok := tx.Statement.ConnPool.(*sql.Tx)
	if !ok {
		return ErrNotInTransaction
	}
	if err := db.AfterCommit(tx, q.client.Notify); err != nil {
		return err
	}
	add := q.client.Add(task).Ctx(tx.Statement.Context).Tx(sqlTx)
	if !at.IsZero() {
		add = add.At(at)
	}
	_, err := add.Save()
	return err
}
