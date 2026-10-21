// Package scripted lets tests inject errors and latency into in-memory fakes and see which calls were made.
package scripted

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Method names one fake method, e.g. "TMDB.Movie". Each fakes package declares its own.
type Method string

// Script is embedded in every fake. Its zero value adds no latency and no errors.
type Script struct {
	mu        sync.Mutex
	latency   time.Duration
	perMethod map[Method]time.Duration
	queued    map[Method][]error
	always    map[Method]error
	calls     []Method
	entered   map[Method]chan struct{}
}

// Delay makes every call wait d before it runs.
func (s *Script) Delay(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latency = d
}

// DelayMethod makes calls to m wait d, overriding Delay.
func (s *Script) DelayMethod(m Method, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.perMethod == nil {
		s.perMethod = map[Method]time.Duration{}
	}
	s.perMethod[m] = d
}

// FailNext makes the next calls to m return errs, one per call, before any FailAlways error.
func (s *Script) FailNext(m Method, errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queued == nil {
		s.queued = map[Method][]error{}
	}
	s.queued[m] = append(s.queued[m], errs...)
}

// FailAlways makes every call to m return err. A nil err clears it.
func (s *Script) FailAlways(m Method, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.always == nil {
		s.always = map[Method]error{}
	}
	s.always[m] = err
}

// Calls returns every method called so far, in call order.
func (s *Script) Calls() []Method {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// Called reports how many times m was called.
func (s *Script) Called(m Method) int {
	return len(slices.DeleteFunc(s.Calls(), func(c Method) bool { return c != m }))
}

// Entered returns a channel that closes once m has been called.
func (s *Script) Entered(m Method) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entered == nil {
		s.entered = map[Method]chan struct{}{}
	}
	ch, ok := s.entered[m]
	if !ok {
		ch = make(chan struct{})
		s.entered[m] = ch
		if slices.Contains(s.calls, m) {
			close(ch)
		}
	}
	return ch
}

// Enter starts a call to m: it records the call, waits out the latency and returns the scripted error.
// A cancelled ctx ends the wait early with ctx.Err().
func (s *Script) Enter(ctx context.Context, m Method) error {
	s.mu.Lock()
	first := !slices.Contains(s.calls, m)
	s.calls = append(s.calls, m)
	if ch, ok := s.entered[m]; ok && first {
		close(ch)
	}
	wait := s.latency
	if d, ok := s.perMethod[m]; ok {
		wait = d
	}
	err := s.always[m]
	if queued := s.queued[m]; len(queued) > 0 {
		err = queued[0]
		s.queued[m] = queued[1:]
	}
	s.mu.Unlock()

	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}
