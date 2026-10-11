package bus

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"magpie/internal/platform/db"

	"gorm.io/gorm"
)

type outboxRow struct {
	Seq         int64 `gorm:"primaryKey;autoIncrement"`
	ID          string
	Type        string
	AggregateID string
	Payload     string
	OccurredAt  time.Time
}

func (outboxRow) TableName() string { return "platform_outbox" }

func (r outboxRow) event() Event {
	return Event{
		ID:          r.ID,
		Type:        r.Type,
		OccurredAt:  r.OccurredAt,
		AggregateID: r.AggregateID,
		Payload:     json.RawMessage(r.Payload),
	}
}

type subscriber struct {
	fn func(Event)
}

// Bus delivers events in-process. Subscribers see durable events after their transaction commits and
// ephemeral events as they are emitted. A subscriber must not block.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[*subscriber]struct{}
	onDurable   []func()
}

func New() *Bus {
	return &Bus{subscribers: map[*subscriber]struct{}{}}
}

// Publish writes ev to the outbox in tx, so the event exists only if the caller's change commits.
// tx must come from db.Write.
func (b *Bus) Publish(tx *gorm.DB, ev Event) error {
	payload, err := json.Marshal(ev.Payload)
	if err != nil {
		return fmt.Errorf("encoding %s payload: %w", ev.Type, err)
	}
	row := outboxRow{
		ID:          ev.ID,
		Type:        ev.Type,
		AggregateID: ev.AggregateID,
		Payload:     string(payload),
		OccurredAt:  ev.OccurredAt,
	}
	if err := db.AfterCommit(tx, func() { b.committed(ev) }); err != nil {
		return err
	}
	return tx.Create(&row).Error
}

// Emit sends an ephemeral event to in-process subscribers only. It is never stored, so no state may depend on it.
func (b *Bus) Emit(ev Event) {
	b.deliver(ev)
}

// Subscribe receives every durable and ephemeral event until the returned func is called.
func (b *Bus) Subscribe(fn func(Event)) (unsubscribe func()) {
	sub := &subscriber{fn: fn}
	b.mu.Lock()
	b.subscribers[sub] = struct{}{}
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		delete(b.subscribers, sub)
		b.mu.Unlock()
	}
}

// OnDurable calls fn after any transaction that published durable events commits.
func (b *Bus) OnDurable(fn func()) {
	b.mu.Lock()
	b.onDurable = append(b.onDurable, fn)
	b.mu.Unlock()
}

func (b *Bus) committed(ev Event) {
	b.deliver(ev)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, fn := range b.onDurable {
		fn()
	}
}

func (b *Bus) deliver(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for sub := range b.subscribers {
		sub.fn(ev)
	}
}
