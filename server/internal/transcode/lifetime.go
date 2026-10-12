package transcode

import (
	"context"
	"sync"
)

// Lifetime is the context of Transcode's background work and agent transfers. It ends when the server
// stops, before the HTTP server waits for open requests, so a long upload can't hold up shutdown.
type Lifetime struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewLifetime() *Lifetime {
	ctx, cancel := context.WithCancel(context.Background())
	return &Lifetime{ctx: ctx, cancel: cancel}
}

// Go runs fn in the background with the lifetime's context.
func (l *Lifetime) Go(fn func(ctx context.Context)) {
	l.wg.Go(func() { fn(l.ctx) })
}

// Bind returns a context that also ends when the lifetime does.
func (l *Lifetime) Bind(ctx context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(l.ctx, cancel)
	return bound, func() {
		stop()
		cancel()
	}
}

// Stop ends the lifetime and waits for the work started with Go.
func (l *Lifetime) Stop() {
	l.cancel()
	l.wg.Wait()
}

// Wait waits for the work started with Go without ending the lifetime. Tests use it to settle.
func (l *Lifetime) Wait() {
	l.wg.Wait()
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
