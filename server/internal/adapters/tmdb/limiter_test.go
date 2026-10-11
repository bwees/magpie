package tmdb

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock advances only when the bucket sleeps.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	return nil
}

func TestTokenBucketAllowsFortyPerSecond(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}
	start := clock.Now()
	bucket := newTokenBucket(requestsPerSecond, clock.Now, clock.Sleep)

	for range requestsPerSecond {
		if err := bucket.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := clock.Now().Sub(start); elapsed != 0 {
		t.Fatalf("a full bucket waited %v", elapsed)
	}

	if err := bucket.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := clock.Now().Sub(start); elapsed != time.Second/requestsPerSecond {
		t.Errorf("41st request waited %v, want %v", elapsed, time.Second/requestsPerSecond)
	}
}

func TestTokenBucketRefillsOverTime(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}
	bucket := newTokenBucket(requestsPerSecond, clock.Now, clock.Sleep)
	for range requestsPerSecond {
		_ = bucket.Wait(context.Background())
	}

	_ = clock.Sleep(context.Background(), time.Second)
	before := clock.Now()
	for range requestsPerSecond {
		_ = bucket.Wait(context.Background())
	}
	if waited := clock.Now().Sub(before); waited != 0 {
		t.Errorf("a refilled bucket waited %v", waited)
	}
}

func TestTokenBucketStopsWaitingWhenCancelled(t *testing.T) {
	bucket := newTokenBucket(requestsPerSecond, time.Now, sleepContext)
	for range requestsPerSecond {
		_ = bucket.Wait(context.Background())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bucket.Wait(ctx); err == nil {
		t.Error("Wait on a cancelled context returned nil")
	}
}
