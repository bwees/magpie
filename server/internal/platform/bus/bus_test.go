package bus

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"magpie/internal/platform/db"
	"magpie/internal/platform/db/dbtest"

	"gorm.io/gorm"
)

const (
	typeThingHappened  = "test.ThingHappened"
	typeThingProgress  = "test.ThingProgressed"
	typeOtherHappened  = "test.OtherHappened"
	aggregateA         = "A"
	aggregateB         = "B"
	maxStepsPerDrain   = 100
	waitForDelivery    = 5 * time.Second
	handlerRecorder    = "test.recorder"
	handlerSecondInput = "test.second"
)

var errHandler = errors.New("handler failed")

type thingHappened struct {
	Thing string `json:"thing"`
}

type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time      { return c.at }
func (c *fakeClock) add(d time.Duration) { c.at = c.at.Add(d) }

// recorder is a handler that logs what it saw and fails for aggregates in failing.
type recorder struct {
	mu      sync.Mutex
	name    string
	seen    []string
	failing map[string]bool
}

func newRecorder(name string) *recorder {
	return &recorder{name: name, failing: map[string]bool{}}
}

func (r *recorder) handler() Handler {
	return Handler{
		Name:       r.name,
		EventTypes: []string{typeThingHappened},
		Handle: func(_ context.Context, _ *gorm.DB, ev Event) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.failing[ev.AggregateID] {
				return errHandler
			}
			payload, err := Decode[thingHappened](ev)
			if err != nil {
				return err
			}
			r.seen = append(r.seen, payload.Thing)
			return nil
		},
	}
}

func (r *recorder) setFailing(aggregate string, failing bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failing[aggregate] = failing
}

func (r *recorder) things() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.seen)
}

type harness struct {
	t     *testing.T
	db    *db.DB
	bus   *Bus
	disp  *Dispatcher
	clock *fakeClock
}

func newHarness(t *testing.T, d *db.DB, handlers ...Handler) *harness {
	t.Helper()
	b := New()
	disp, err := NewDispatcher(DispatcherParams{DB: d, Bus: b, Handlers: handlers})
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{at: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}
	disp.now = clock.now
	if err := disp.ensureSubscriptions(context.Background()); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, db: d, bus: b, disp: disp, clock: clock}
}

func (h *harness) publish(aggregate string, things ...string) {
	h.t.Helper()
	err := h.db.Write(context.Background(), func(tx *gorm.DB) error {
		for _, thing := range things {
			if err := h.bus.Publish(tx, NewEvent(typeThingHappened, aggregate, thingHappened{Thing: thing})); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

// drain steps every handler until none has due work.
func (h *harness) drain() {
	h.t.Helper()
	for range maxStepsPerDrain {
		busy := false
		for _, handler := range h.disp.handlers {
			worked, err := h.disp.step(context.Background(), handler)
			if err != nil {
				h.t.Fatalf("step %s: %v", handler.Name, err)
			}
			busy = busy || worked
		}
		if !busy {
			return
		}
	}
	h.t.Fatal("dispatcher never settled")
}

func (h *harness) queued(handler, thingEventAggregate string) []queueRow {
	h.t.Helper()
	var rows []queueRow
	if err := h.db.Read(context.Background()).Where("handler = ? AND aggregate_id = ?", handler, thingEventAggregate).Order("seq").Find(&rows).Error; err != nil {
		h.t.Fatal(err)
	}
	return rows
}

func (h *harness) count(table string) int64 {
	h.t.Helper()
	var n int64
	if err := h.db.Read(context.Background()).Table(table).Count(&n).Error; err != nil {
		h.t.Fatal(err)
	}
	return n
}

func assertThings(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("handled %v, want %v", got, want)
	}
}

func TestPublishWritesOutboxInCallerTransaction(t *testing.T) {
	h := newHarness(t, dbtest.Open(t))
	h.publish(aggregateA, "a1", "a2")

	if got := h.count("platform_outbox"); got != 2 {
		t.Fatalf("outbox rows = %d, want 2", got)
	}
}

func TestRolledBackTransactionPublishesNothing(t *testing.T) {
	h := newHarness(t, dbtest.Open(t))
	var delivered []Event
	h.bus.Subscribe(func(ev Event) { delivered = append(delivered, ev) })
	errAbort := errors.New("abort")

	err := h.db.Write(context.Background(), func(tx *gorm.DB) error {
		if err := h.bus.Publish(tx, NewEvent(typeThingHappened, aggregateA, thingHappened{Thing: "a1"})); err != nil {
			return err
		}
		return errAbort
	})

	if !errors.Is(err, errAbort) {
		t.Fatal(err)
	}
	if got := h.count("platform_outbox"); got != 0 {
		t.Errorf("outbox rows = %d after rollback", got)
	}
	if len(delivered) != 0 {
		t.Errorf("in-process subscribers saw %d rolled-back events", len(delivered))
	}
}

func TestPublishNeedsWriteTransaction(t *testing.T) {
	d := dbtest.Open(t)
	b := New()
	conn := d.Read(context.Background())

	if err := b.Publish(conn, NewEvent(typeThingHappened, aggregateA, thingHappened{})); !errors.Is(err, db.ErrNotInWrite) {
		t.Fatalf("err = %v, want ErrNotInWrite", err)
	}
}

func TestCommittedEventsReachSubscribersTyped(t *testing.T) {
	h := newHarness(t, dbtest.Open(t))
	var delivered []Event
	h.bus.Subscribe(func(ev Event) { delivered = append(delivered, ev) })

	h.publish(aggregateA, "a1")

	if len(delivered) != 1 {
		t.Fatalf("subscribers saw %d events, want 1", len(delivered))
	}
	if payload, ok := delivered[0].Payload.(thingHappened); !ok || payload.Thing != "a1" {
		t.Errorf("payload = %#v", delivered[0].Payload)
	}
}

func TestDeliversInOutboxOrderPerAggregate(t *testing.T) {
	r := newRecorder(handlerRecorder)
	h := newHarness(t, dbtest.Open(t), r.handler())

	h.publish(aggregateA, "a1")
	h.publish(aggregateB, "b1")
	h.publish(aggregateA, "a2", "a3")
	h.publish(aggregateB, "b2")
	h.drain()

	assertThings(t, r.things(), "a1", "b1", "a2", "a3", "b2")
}

func TestFailingHandlerBacksOffAndHoldsOnlyItsAggregate(t *testing.T) {
	r := newRecorder(handlerRecorder)
	h := newHarness(t, dbtest.Open(t), r.handler())
	r.setFailing(aggregateA, true)

	h.publish(aggregateA, "a1", "a2")
	h.publish(aggregateB, "b1")
	h.drain()

	assertThings(t, r.things(), "b1")
	queued := h.queued(handlerRecorder, aggregateA)
	if len(queued) != 2 || queued[0].Attempts != 1 || queued[1].Attempts != 0 {
		t.Fatalf("queue = %+v, want a1 failed once and a2 held", queued)
	}

	wantDelays := []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i, want := range wantDelays {
		head := h.queued(handlerRecorder, aggregateA)[0]
		if got := head.NextAttemptAt.Sub(h.clock.now()); got != want {
			t.Fatalf("after failure %d: retry in %v, want %v", i+1, got, want)
		}
		h.clock.add(want - time.Millisecond)
		h.drain()
		if got := h.queued(handlerRecorder, aggregateA)[0].Attempts; got != i+1 {
			t.Fatalf("retried before the backoff passed: attempts = %d", got)
		}
		h.clock.add(time.Millisecond)
		h.drain()
	}

	r.setFailing(aggregateA, false)
	h.clock.add(10 * time.Minute)
	h.drain()

	assertThings(t, r.things(), "b1", "a1", "a2")
	if got := h.queued(handlerRecorder, aggregateA); len(got) != 0 {
		t.Errorf("queue not empty after recovery: %+v", got)
	}
}

func TestOtherHandlersAndAggregatesKeepFlowing(t *testing.T) {
	failing := newRecorder(handlerRecorder)
	healthy := newRecorder(handlerSecondInput)
	h := newHarness(t, dbtest.Open(t), failing.handler(), healthy.handler())
	failing.setFailing(aggregateA, true)

	h.publish(aggregateA, "a1")
	h.publish(aggregateB, "b1")
	h.publish(aggregateA, "a2")
	h.publish(aggregateB, "b2")
	h.drain()

	assertThings(t, healthy.things(), "a1", "b1", "a2", "b2")
	assertThings(t, failing.things(), "b1", "b2")
}

func TestParksAfterMaxAttemptsAndRetryRequeues(t *testing.T) {
	r := newRecorder(handlerRecorder)
	h := newHarness(t, dbtest.Open(t), r.handler())
	ctx := context.Background()
	r.setFailing(aggregateA, true)

	h.publish(aggregateA, "a1")
	h.drain()
	for attempt := 1; attempt < MaxAttempts; attempt++ {
		h.clock.add(RetryDelay(attempt))
		h.drain()
	}

	parked, err := h.disp.Parked(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(parked) != 1 || parked[0].Attempts != MaxAttempts || parked[0].EventType != typeThingHappened {
		t.Fatalf("parked = %+v, want a1 after %d attempts", parked, MaxAttempts)
	}

	h.clock.add(time.Hour)
	h.drain()
	if got := h.queued(handlerRecorder, aggregateA)[0].Attempts; got != MaxAttempts {
		t.Fatalf("parked event was retried on its own: attempts = %d", got)
	}

	r.setFailing(aggregateA, false)
	h.publish(aggregateA, "a2")
	h.drain()
	assertThings(t, r.things(), "a2")

	if err := h.disp.Retry(ctx, handlerRecorder, parked[0].EventID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	h.drain()
	assertThings(t, r.things(), "a2", "a1")

	if err := h.disp.Retry(ctx, handlerRecorder, parked[0].EventID); !errors.Is(err, ErrNotParked) {
		t.Errorf("second retry err = %v, want ErrNotParked", err)
	}
}

func TestRedeliveryAfterCrashIsDeduped(t *testing.T) {
	d := dbtest.Open(t)
	calls := 0
	handler := Handler{
		Name:       handlerRecorder,
		EventTypes: []string{typeThingHappened},
		Handle: func(context.Context, *gorm.DB, Event) error {
			calls++
			return nil
		},
	}
	errCrash := errors.New("process died")

	crashing := newHarness(t, d, handler)
	crashing.disp.afterHandle = func(Handler, Event) error { return errCrash }
	crashing.publish(aggregateA, "a1")
	if _, err := crashing.disp.step(context.Background(), handler); !errors.Is(err, errCrash) {
		t.Fatalf("step err = %v, want simulated crash", err)
	}

	restarted := newHarness(t, d, handler)
	restarted.drain()

	if calls != 1 {
		t.Errorf("handler ran %d times, want 1", calls)
	}
	var sub subscriptionRow
	if err := d.Read(context.Background()).Where("handler = ?", handlerRecorder).Take(&sub).Error; err != nil {
		t.Fatal(err)
	}
	if sub.Cursor == 0 {
		t.Error("cursor did not advance past the redelivered event")
	}
}

func TestPruneFollowsRetention(t *testing.T) {
	behind := newRecorder(handlerSecondInput)
	r := newRecorder(handlerRecorder)
	d := dbtest.Open(t)
	h := newHarness(t, d, r.handler())
	ctx := context.Background()
	old := h.clock.now().Add(-OutboxRetention - time.Hour)

	publishAt := func(aggregate, thing string, at time.Time) {
		ev := NewEvent(typeThingHappened, aggregate, thingHappened{Thing: thing})
		ev.OccurredAt = at
		if err := d.Write(ctx, func(tx *gorm.DB) error { return h.bus.Publish(tx, ev) }); err != nil {
			t.Fatal(err)
		}
	}
	r.setFailing(aggregateB, true)
	publishAt(aggregateA, "old-delivered", old)
	publishAt(aggregateB, "old-queued", old)
	publishAt(aggregateA, "recent", h.clock.now())
	h.drain()

	oldProcessed := processedRow{Handler: handlerRecorder, EventID: "old", ProcessedAt: h.clock.now().Add(-ProcessedRetention - time.Hour)}
	if err := d.Write(ctx, func(tx *gorm.DB) error { return tx.Create(&oldProcessed).Error }); err != nil {
		t.Fatal(err)
	}

	if err := h.disp.Prune(ctx); err != nil {
		t.Fatal(err)
	}

	var things []string
	if err := d.Read(ctx).Model(&outboxRow{}).Order("seq").Pluck("json_extract(payload, '$.thing')", &things).Error; err != nil {
		t.Fatal(err)
	}
	assertThings(t, things, "old-queued", "recent")
	if got := h.count("platform_processed_events"); got != 2 {
		t.Errorf("processed rows = %d, want the 2 recent ones", got)
	}

	// A handler that has not passed an old row keeps it.
	lagging := newHarness(t, d, r.handler(), behind.handler())
	publishAt(aggregateA, "old-unseen", old)
	if err := lagging.disp.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.Read(ctx).Model(&outboxRow{}).Order("seq").Pluck("json_extract(payload, '$.thing')", &things).Error; err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(things, "old-unseen") {
		t.Errorf("pruned a row a handler has not seen: %v", things)
	}
}

func TestEphemeralEventsStayInProcess(t *testing.T) {
	d := dbtest.Open(t)
	calls := 0
	handler := Handler{
		Name:       handlerRecorder,
		EventTypes: []string{typeThingProgress},
		Handle:     func(context.Context, *gorm.DB, Event) error { calls++; return nil },
	}
	h := newHarness(t, d, handler)
	var delivered []Event
	h.bus.Subscribe(func(ev Event) { delivered = append(delivered, ev) })

	h.bus.Emit(NewEvent(typeThingProgress, aggregateA, thingHappened{Thing: "50%"}))

	if len(delivered) != 1 {
		t.Fatalf("subscribers saw %d ephemeral events, want 1", len(delivered))
	}
	if got := h.count("platform_outbox"); got != 0 {
		t.Errorf("ephemeral event written to the outbox (%d rows)", got)
	}

	restarted := newHarness(t, d, handler)
	restarted.drain()
	if calls != 0 {
		t.Errorf("a durable handler received %d ephemeral events after restart", calls)
	}
}

func TestStartedDispatcherDeliversOnCommit(t *testing.T) {
	got := make(chan string, 1)
	handler := Handler{
		Name:       handlerRecorder,
		EventTypes: []string{typeThingHappened},
		Handle: func(_ context.Context, _ *gorm.DB, ev Event) error {
			payload, err := Decode[thingHappened](ev)
			got <- payload.Thing
			return err
		},
	}
	h := newHarness(t, dbtest.Open(t), handler)
	h.disp.now = time.Now
	if err := h.disp.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.disp.Stop)

	h.publish(aggregateA, "live")

	select {
	case thing := <-got:
		if thing != "live" {
			t.Errorf("handled %q", thing)
		}
	case <-time.After(waitForDelivery):
		t.Fatal("event never delivered")
	}
}

func TestDispatcherRejectsBadHandlers(t *testing.T) {
	valid := newRecorder(handlerRecorder).handler()
	cases := map[string][]Handler{
		"duplicate": {valid, valid},
		"no types":  {{Name: "x", Handle: valid.Handle}},
		"no name":   {{EventTypes: []string{typeOtherHappened}, Handle: valid.Handle}},
	}
	for name, handlers := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewDispatcher(DispatcherParams{DB: dbtest.Open(t), Bus: New(), Handlers: handlers}); err == nil {
				t.Fatal("NewDispatcher accepted bad handlers")
			}
		})
	}
}

func TestDecode(t *testing.T) {
	typed := NewEvent(typeThingHappened, aggregateA, thingHappened{Thing: "x"})
	raw := typed
	raw.Payload = []byte(`{"thing":"x"}`)

	if got, err := Decode[thingHappened](typed); err != nil || got.Thing != "x" {
		t.Errorf("typed decode = %+v, %v", got, err)
	}
	if _, err := Decode[thingHappened](raw); err == nil {
		t.Error("decoded a payload that is neither the struct nor JSON")
	}
	raw.Payload = json.RawMessage(`{"thing":"y"}`)
	if got, err := Decode[thingHappened](raw); err != nil || got.Thing != "y" {
		t.Errorf("raw decode = %+v, %v", got, err)
	}
}
