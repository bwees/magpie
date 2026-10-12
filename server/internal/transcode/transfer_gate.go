package transcode

import (
	"cmp"
	"maps"
	"sync"
	"time"

	"magpie/internal/domain"
)

// TransferRank orders transfers waiting for the server-wide cap: higher goes first.
type TransferRank int

const (
	RankPrefetch TransferRank = iota
	RankDownload
	RankUpload
)

// TransferDirection says which way a job's bytes flow.
type TransferDirection string

const (
	DirectionSource TransferDirection = "SOURCE"
	DirectionOutput TransferDirection = "OUTPUT"
)

// TransferHolder is one job's transfer in one direction.
type TransferHolder struct {
	Job       domain.JobID
	Direction TransferDirection
}

// RetryAfter is what a refused transfer is told to wait. A waiter that hasn't asked again for
// waiterTimeout gave up and stops holding its place.
const (
	RetryAfter    = 5 * time.Second
	waiterTimeout = 3 * RetryAfter
)

type waiter struct {
	rank     TransferRank
	since    time.Time
	lastSeen time.Time
}

// TransferGate caps concurrent source downloads plus output uploads server-wide. A refused transfer
// keeps its place, so when a slot frees the best waiter gets it: uploads, then downloads, then prefetches,
// each in the order they first asked.
type TransferGate struct {
	mu      sync.Mutex
	holders map[TransferHolder]struct{}
	waiting map[TransferHolder]waiter
	now     func() time.Time
}

func NewTransferGate() *TransferGate {
	return &TransferGate{
		holders: map[TransferHolder]struct{}{},
		waiting: map[TransferHolder]waiter{},
		now:     time.Now,
	}
}

// Acquire takes a slot for h, or records it as waiting and returns false. Asking again while holding a
// slot succeeds.
func (g *TransferGate) Acquire(h TransferHolder, rank TransferRank, capacity int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	maps.DeleteFunc(g.waiting, func(_ TransferHolder, w waiter) bool { return now.Sub(w.lastSeen) > waiterTimeout })
	if _, held := g.holders[h]; held {
		return true
	}

	me, ok := g.waiting[h]
	if !ok {
		me = waiter{rank: rank, since: now}
	}
	me.lastSeen = now
	free := capacity - len(g.holders)
	if free > len(g.ahead(h, me)) {
		delete(g.waiting, h)
		g.holders[h] = struct{}{}
		return true
	}
	g.waiting[h] = me
	return false
}

// ahead lists the waiters that go before me.
func (g *TransferGate) ahead(h TransferHolder, me waiter) []TransferHolder {
	var ahead []TransferHolder
	for other, w := range g.waiting {
		if other != h && before(w, me) {
			ahead = append(ahead, other)
		}
	}
	return ahead
}

func before(a, b waiter) bool {
	return cmp.Or(cmp.Compare(b.rank, a.rank), a.since.Compare(b.since)) < 0
}

// Release frees h's slot.
func (g *TransferGate) Release(h TransferHolder) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.holders, h)
}

// ReleaseJob frees every slot a job holds and forgets its waits, for a job that ended or moved on.
func (g *TransferGate) ReleaseJob(job domain.JobID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	keep := func(h TransferHolder) bool { return h.Job == job }
	maps.DeleteFunc(g.holders, func(h TransferHolder, _ struct{}) bool { return keep(h) })
	maps.DeleteFunc(g.waiting, func(h TransferHolder, _ waiter) bool { return keep(h) })
}
