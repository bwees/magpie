package jobs

import (
	"context"
	"fmt"
	"sync"

	"github.com/mikestefanello/backlite"
)

// runningJobs lets Stop cancel the jobs in progress and wait for them to return.
type runningJobs struct {
	mu       sync.Mutex
	stopping context.Context
	stop     context.CancelFunc
	wg       sync.WaitGroup
}

func newRunningJobs() *runningJobs {
	stopping, stop := context.WithCancel(context.Background())
	return &runningJobs{stopping: stopping, stop: stop}
}

// run calls process with a ctx that also ends when the queue stops.
func (r *runningJobs) run(ctx context.Context, process func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(r.stopping, cancel)()

	r.mu.Lock()
	tracked := r.stopping.Err() == nil
	if tracked {
		r.wg.Add(1)
	}
	r.mu.Unlock()
	if tracked {
		defer r.wg.Done()
	}
	return process(ctx)
}

// cancelAndWait ends every job's ctx and waits until they return or ctx ends.
func (r *runningJobs) cancelAndWait(ctx context.Context) error {
	r.mu.Lock()
	r.stop()
	r.mu.Unlock()
	return waitFor(ctx, &r.wg)
}

// waitFor waits for wg until ctx ends.
func waitFor(ctx context.Context, wg *sync.WaitGroup) error {
	returned := make(chan struct{})
	go func() {
		wg.Wait()
		close(returned)
	}()
	select {
	case <-returned:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("jobs: still running at shutdown: %w", ctx.Err())
	}
}

// cancellableQueue runs a queue's jobs under runningJobs.
type cancellableQueue struct {
	backlite.Queue
	running *runningJobs
}

func (q cancellableQueue) Process(ctx context.Context, payload []byte) error {
	return q.running.run(ctx, func(ctx context.Context) error { return q.Queue.Process(ctx, payload) })
}
