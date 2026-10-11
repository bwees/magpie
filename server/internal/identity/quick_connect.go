package identity

import (
	"sync"
	"time"
)

// maxPendingQuickConnects bounds the in-memory handles anonymous callers can create.
const maxPendingQuickConnects = 1000

type pendingQuickConnect struct {
	secret    string
	expiresAt time.Time
}

// quickConnects maps the opaque handles given to browsers to Jellyfin's Quick Connect secrets,
// so the secret never leaves the server. Pending sign-ins are lost on restart.
type quickConnects struct {
	mu      sync.Mutex
	now     func() time.Time
	pending map[string]pendingQuickConnect
}

func newQuickConnects() *quickConnects {
	return &quickConnects{now: time.Now, pending: map[string]pendingQuickConnect{}}
}

func (q *quickConnects) add(secret string, expiresAt time.Time) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.dropExpired()
	if len(q.pending) >= maxPendingQuickConnects {
		return "", ErrTooManySignIns
	}
	handle, err := newSessionToken()
	if err != nil {
		return "", err
	}
	q.pending[handle] = pendingQuickConnect{secret: secret, expiresAt: expiresAt}
	return handle, nil
}

// secret returns the secret behind a live handle.
func (q *quickConnects) secret(handle string) (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.dropExpired()
	p, ok := q.pending[handle]
	return p.secret, ok
}

func (q *quickConnects) remove(handle string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.pending, handle)
}

func (q *quickConnects) dropExpired() {
	now := q.now()
	for handle, p := range q.pending {
		if !now.Before(p.expiresAt) {
			delete(q.pending, handle)
		}
	}
}
