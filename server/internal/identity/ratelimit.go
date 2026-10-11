package identity

import (
	"sync"
	"time"
)

const (
	// MaxFailedSignIns failures from one IP inside SignInWindow block that IP's next sign-in.
	MaxFailedSignIns = 10
	SignInWindow     = 10 * time.Minute
)

// signInLimiter keeps the times of recent failed sign-ins per client IP.
type signInLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	failures map[string][]time.Time
}

func newSignInLimiter() *signInLimiter {
	return &signInLimiter{now: time.Now, failures: map[string][]time.Time{}}
}

func (l *signInLimiter) Allowed(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(ip)) < MaxFailedSignIns
}

func (l *signInLimiter) Failed(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[ip] = append(l.recent(ip), l.now())
}

// recent drops failures older than the window and forgets IPs with none left.
func (l *signInLimiter) recent(ip string) []time.Time {
	cutoff := l.now().Add(-SignInWindow)
	kept := l.failures[ip][:0]
	for _, at := range l.failures[ip] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, ip)
		return nil
	}
	l.failures[ip] = kept
	return kept
}
