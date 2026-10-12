package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"
	"magpie/internal/platform/owner"
	"magpie/internal/platform/settings"

	"github.com/mikestefanello/backlite"
	"gorm.io/gorm"
)

const (
	testQueueName = "platform.test"
	waitForJob    = 5 * time.Second
	quietWait     = 300 * time.Millisecond
)

type pingTask struct {
	Note string `json:"note"`
}

func (pingTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{Name: testQueueName, MaxAttempts: 1}
}

func newRunningQueue(t *testing.T, d *db.DB) (*Queue, <-chan pingTask) {
	t.Helper()
	ran := make(chan pingTask, 8)
	q, err := Open(d, backlite.NewQueue(func(_ context.Context, task pingTask) error {
		ran <- task
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	q.Start(context.Background())
	t.Cleanup(func() {
		if err := q.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return q, ran
}

func TestBackliteUsesTheAppDatabase(t *testing.T) {
	d := dbtest.Open(t)
	q, ran := newRunningQueue(t, d)

	if !d.Read(context.Background()).Migrator().HasTable("backlite_tasks") {
		t.Fatal("backlite tables missing from the app database")
	}
	if err := q.Enqueue(context.Background(), pingTask{Note: "hello"}); err != nil {
		t.Fatal(err)
	}
	select {
	case task := <-ran:
		if task.Note != "hello" {
			t.Errorf("task = %+v", task)
		}
	case <-time.After(waitForJob):
		t.Fatal("job never ran")
	}
}

func TestEnqueueTxRunsOnlyAfterCommit(t *testing.T) {
	d := dbtest.Open(t)
	q, ran := newRunningQueue(t, d)
	ctx := context.Background()
	errAbort := errors.New("abort")

	err := d.Write(ctx, func(tx *gorm.DB) error {
		if err := q.EnqueueTx(tx, pingTask{Note: "rolled back"}); err != nil {
			return err
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatal(err)
	}
	if err := d.Write(ctx, func(tx *gorm.DB) error { return q.EnqueueTx(tx, pingTask{Note: "committed"}) }); err != nil {
		t.Fatal(err)
	}

	select {
	case task := <-ran:
		if task.Note != "committed" {
			t.Errorf("ran %q, want only the committed task", task.Note)
		}
	case <-time.After(waitForJob):
		t.Fatal("committed job never ran")
	}
}

func TestEnqueueTxAtWaitsUntilItsTime(t *testing.T) {
	const later = time.Hour
	d := dbtest.Open(t)
	q, ran := newRunningQueue(t, d)
	ctx := context.Background()

	err := d.Write(ctx, func(tx *gorm.DB) error {
		if err := q.EnqueueTxAt(tx, pingTask{Note: "later"}, time.Now().Add(later)); err != nil {
			return err
		}
		return q.EnqueueTxAt(tx, pingTask{Note: "now"}, time.Time{})
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case task := <-ran:
		if task.Note != "now" {
			t.Errorf("ran %q first, want the task due now", task.Note)
		}
	case <-time.After(waitForJob):
		t.Fatal("the task due now never ran")
	}
	select {
	case task := <-ran:
		t.Errorf("ran %q an hour early", task.Note)
	case <-time.After(quietWait):
	}
}

type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time { return c.at }

func queuedTasks(t *testing.T, d *db.DB) int64 {
	t.Helper()
	var count int64
	if err := d.Read(context.Background()).Table("backlite_tasks").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestPeriodicFollowsIntervalSetting(t *testing.T) {
	d := dbtest.Open(t)
	q, err := Open(d)
	if err != nil {
		t.Fatal(err)
	}
	store := settings.NewStore(d)
	key := settings.NewKey(owner.Platform, "test.interval", settings.Duration(time.Hour))
	clock := &fakeClock{at: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}
	s := &Scheduler{db: d, queue: q, now: clock.now, periodics: []Periodic{{
		Name:     "platform.test",
		Schedule: EverySetting(store, key),
		Task:     func() backlite.Task { return pingTask{} },
	}}}
	ctx := context.Background()
	step := func(advance time.Duration) {
		t.Helper()
		clock.at = clock.at.Add(advance)
		if err := s.EnqueueDue(ctx); err != nil {
			t.Fatal(err)
		}
	}

	step(0)
	if got := queuedTasks(t, d); got != 0 {
		t.Fatalf("first sight enqueued %d tasks, want 0", got)
	}

	step(59 * time.Minute)
	if got := queuedTasks(t, d); got != 0 {
		t.Fatalf("enqueued %d tasks before the interval passed", got)
	}

	step(time.Minute)
	if got := queuedTasks(t, d); got != 1 {
		t.Fatalf("enqueued %d tasks when due, want 1", got)
	}

	if err := key.Set(ctx, store, settings.Duration(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	step(0)
	step(10 * time.Minute)
	if got := queuedTasks(t, d); got != 2 {
		t.Fatalf("tasks = %d after the shorter interval passed, want 2", got)
	}
}

func TestPausedPeriodicNeverRuns(t *testing.T) {
	d := dbtest.Open(t)
	q, err := Open(d)
	if err != nil {
		t.Fatal(err)
	}
	s := &Scheduler{db: d, queue: q, now: time.Now, periodics: []Periodic{{
		Name:     "platform.paused",
		Schedule: func(context.Context) (Schedule, error) { return nil, nil },
		Task:     func() backlite.Task { return pingTask{} },
	}}}

	if err := s.EnqueueDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var booked int64
	if err := d.Read(context.Background()).Model(&scheduleRow{}).Count(&booked).Error; err != nil {
		t.Fatal(err)
	}
	if booked != 0 {
		t.Errorf("paused job was booked")
	}
}

func TestDailyAtNext(t *testing.T) {
	at := DailyAt{Hour: 3}
	before := time.Date(2026, 10, 11, 2, 0, 0, 0, time.Local)
	after := time.Date(2026, 10, 11, 3, 0, 0, 0, time.Local)

	if got, want := at.Next(before), time.Date(2026, 10, 11, 3, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Errorf("Next(02:00) = %v, want %v", got, want)
	}
	if got, want := at.Next(after), time.Date(2026, 10, 12, 3, 0, 0, 0, time.Local); !got.Equal(want) {
		t.Errorf("Next(03:00) = %v, want %v", got, want)
	}
}

func TestTickerOnlyEnqueues(t *testing.T) {
	d := dbtest.Open(t)
	q, ran := newRunningQueue(t, d)
	s := &Scheduler{db: d, queue: q, now: time.Now, tickers: []Ticker{{
		Name:     "platform.test-ticker",
		Interval: func(context.Context) (time.Duration, error) { return time.Millisecond, nil },
		Enqueue:  func(ctx context.Context, q Enqueuer) error { return q.Enqueue(ctx, pingTask{Note: "tick"}) },
	}}}
	s.Start()
	t.Cleanup(func() {
		if err := s.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})

	select {
	case task := <-ran:
		if task.Note != "tick" {
			t.Errorf("task = %+v", task)
		}
	case <-time.After(waitForJob):
		t.Fatal("ticker never enqueued")
	}
}
