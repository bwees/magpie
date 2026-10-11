package tmdb

import (
	"context"
	"sync"
	"time"
)

// tokenBucket allows bursts up to its rate and then one request per 1/rate.
type tokenBucket struct {
	mu     sync.Mutex
	rate   float64
	tokens float64
	last   time.Time
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
}

func newTokenBucket(perSecond int, now func() time.Time, sleep func(context.Context, time.Duration) error) *tokenBucket {
	return &tokenBucket{rate: float64(perSecond), tokens: float64(perSecond), last: now(), now: now, sleep: sleep}
}

// Wait blocks until a token is free or ctx ends.
func (b *tokenBucket) Wait(ctx context.Context) error {
	for {
		wait := b.take()
		if wait == 0 {
			return nil
		}
		if err := b.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

// take spends a token and returns 0, or returns how long until one is free.
func (b *tokenBucket) take() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	b.tokens = min(b.rate, b.tokens+now.Sub(b.last).Seconds()*b.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	return time.Duration((1 - b.tokens) / b.rate * float64(time.Second))
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
