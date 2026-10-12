package transcode

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Lifetime is the context of Transcode's background work and agent transfers. It ends when the server
// stops, before the HTTP server waits for open requests, so a long upload can't hold up shutdown.
type Lifetime struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	running map[chan struct{}]struct{}
}

func NewLifetime() *Lifetime {
	ctx, cancel := context.WithCancel(context.Background())
	return &Lifetime{ctx: ctx, cancel: cancel, running: map[chan struct{}]struct{}{}}
}

// Go runs fn in the background with the lifetime's context. Once the lifetime has ended, fn doesn't run.
func (l *Lifetime) Go(fn func(ctx context.Context)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ctx.Err() != nil {
		return
	}
	done := make(chan struct{})
	l.running[done] = struct{}{}
	go func() {
		defer l.finish(done)
		fn(l.ctx)
	}()
}

func (l *Lifetime) finish(done chan struct{}) {
	l.mu.Lock()
	delete(l.running, done)
	l.mu.Unlock()
	close(done)
}

// Bind returns the request's context, which also ends when the lifetime does. Ending the lifetime also
// breaks off a read or write the request is stuck in, since a stalled agent never lets it check the
// context again.
func (l *Lifetime) Bind(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancel(r.Context())
	control := http.NewResponseController(w)
	interrupted := make(chan struct{})
	stop := context.AfterFunc(l.ctx, func() {
		defer close(interrupted)
		cancel()
		now := time.Now()
		_ = control.SetReadDeadline(now)
		_ = control.SetWriteDeadline(now)
	})
	return bound, func() {
		if !stop() {
			<-interrupted
		}
		cancel()
	}
}

// Stop ends the lifetime and waits for the work started with Go.
func (l *Lifetime) Stop() {
	l.mu.Lock()
	l.cancel()
	l.mu.Unlock()
	l.Wait()
}

// Wait waits until no work started with Go is running, without ending the lifetime. Tests use it to
// settle.
func (l *Lifetime) Wait() {
	for {
		done, ok := l.anyRunning()
		if !ok {
			return
		}
		<-done
	}
}

func (l *Lifetime) anyRunning() (chan struct{}, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for done := range l.running {
		return done, true
	}
	return nil, false
}

// Wake asks the scheduler for a pass now instead of at its next tick.
type Wake struct {
	ch chan struct{}
}

func NewWake() *Wake {
	return &Wake{ch: make(chan struct{}, 1)}
}

// Kick never blocks; kicks that arrive while one is pending merge into it.
func (w *Wake) Kick() {
	select {
	case w.ch <- struct{}{}:
	default:
	}
}

func (w *Wake) C() <-chan struct{} { return w.ch }
