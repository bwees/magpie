package live

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
)

const smallQueue = 4

// queuedClient is a client attached to h with no socket behind it, so nothing drains its queue.
func queuedClient(t *testing.T, h *Hub, p domain.Principal) (*client, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := newClient(h, nil, p, cancel)
	h.attach(c, "", 0)
	if reset := <-c.queue; reset.msg.Type != TypeReset {
		t.Fatalf("attach queued %+v, want a reset", reset.msg)
	}
	return c, ctx
}

func drain(c *client) []entry {
	var entries []entry
	for len(c.queue) > 0 {
		entries = append(entries, <-c.queue)
	}
	return entries
}

func TestBurstLargerThanTheQueueResetsTheSocket(t *testing.T) {
	cfg := testConfig()
	cfg.ClientQueue = smallQueue
	h := newHub(cfg, RoleFilter{})
	c, ctx := queuedClient(t, h, admin)
	const afterOverflow = 2

	for range smallQueue + 1 + afterOverflow {
		publish(h, typeFileImported, "t1", fileImported{Title: "t1"})
	}

	if ctx.Err() != nil {
		t.Fatal("a socket that fell behind was dropped")
	}
	got := drain(c)
	if len(got) != 1+afterOverflow || got[0].msg.Type != TypeReset || got[0].msg.Seq != smallQueue+1 {
		t.Fatalf("queued %+v, want a reset at seq %d and the events after it", got, smallQueue+1)
	}
	for i, e := range got[1:] {
		if want := uint64(smallQueue + 2 + i); e.msg.Seq != want || e.msg.Type != typeFileImported {
			t.Errorf("queued[%d] = %+v, want seq %d", i+1, e.msg, want)
		}
	}
}

// countingIdentity counts the user lookups the live filter makes.
type countingIdentity struct {
	identityapi.Reader
	lookups atomic.Int32
}

func (c *countingIdentity) User(ctx context.Context, id domain.UserID) (identityapi.User, error) {
	c.lookups.Add(1)
	return c.Reader.User(ctx, id)
}

func TestSocketRereadsViewerAccessOnlyAfterTTL(t *testing.T) {
	roles := newRoleFixture()
	identity := &countingIdentity{Reader: roles.identity}
	h := newHub(testConfig(), RoleFilter{Identity: identity, Requests: roles.filter.Requests})
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	h.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	c, _ := queuedClient(t, h, admin)
	const burst = 50

	for range burst {
		publish(h, typeFileImported, "t1", fileImported{Title: roles.myTitle})
	}
	for _, e := range drain(c) {
		if !c.allows(context.Background(), e) {
			t.Fatalf("admin was filtered from %+v", e.msg)
		}
	}
	if n := identity.lookups.Load(); n != 1 {
		t.Fatalf("%d user lookups for %d messages, want 1", n, burst)
	}

	roles.identity.Put(identityapi.User{ID: admin.User, Username: "admin", Role: identityapi.RoleAdmin, Disabled: true})
	mu.Lock()
	clock = clock.Add(accessTTL)
	mu.Unlock()
	publish(h, typeFileImported, "t1", fileImported{Title: roles.myTitle})
	if e := <-c.queue; c.allows(context.Background(), e) {
		t.Error("a disabled admin still got messages after the access TTL")
	}
	if n := identity.lookups.Load(); n != 2 {
		t.Errorf("%d user lookups after the TTL, want 2", n)
	}
}
