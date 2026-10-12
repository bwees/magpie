package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"magpie/internal/platform/db/dbtest"

	"github.com/mikestefanello/backlite"
	"go.uber.org/fx/fxtest"
)

const (
	// stopDeadline stands in for fx's stop timeout; a stop that reaches it has hung.
	stopDeadline = 10 * time.Second
	promptStop   = 3 * time.Second
	cancelAfter  = 100 * time.Millisecond
)

func openQueue(t *testing.T, process func(context.Context, pingTask) error) (*Queue, *fxtest.Lifecycle) {
	t.Helper()
	lc := fxtest.NewLifecycle(t)
	q, err := New(Params{Lifecycle: lc, DB: dbtest.Open(t)})
	if err != nil {
		t.Fatal(err)
	}
	q.Register(backlite.NewQueue(process))
	return q, lc
}

func stopWithin(t *testing.T, lc *fxtest.Lifecycle, limit time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stopDeadline)
	defer cancel()
	begun := time.Now()
	if err := lc.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if took := time.Since(begun); took > limit {
		t.Fatalf("stop took %v, want under %v", took, limit)
	}
}

func TestStopCancelsARunningJob(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	q, lc := openQueue(t, func(ctx context.Context, _ pingTask) error {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return ctx.Err()
	})
	lc.RequireStart()

	if err := q.Enqueue(context.Background(), pingTask{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(waitForJob):
		t.Fatal("job never ran")
	}

	stopWithin(t, lc, promptStop)
	select {
	case <-cancelled:
	default:
		t.Fatal("stop returned before the running job saw its context end")
	}
}

func TestStopGivesUpOnAJobThatIgnoresCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	q, lc := openQueue(t, func(context.Context, pingTask) error {
		close(started)
		<-release
		return nil
	})
	lc.RequireStart()
	defer close(release)

	if err := q.Enqueue(context.Background(), pingTask{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(waitForJob):
		t.Fatal("job never ran")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
	defer cancel()
	begun := time.Now()
	err := lc.Stop(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("stop = %v, want the context's error", err)
	}
	if took := time.Since(begun); took > promptStop {
		t.Errorf("stop took %v after its context ended", took)
	}
}
