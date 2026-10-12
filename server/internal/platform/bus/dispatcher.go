package bus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"magpie/internal/platform/db"

	"go.uber.org/fx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// MaxAttempts is how many times a handler may fail one event before it is parked.
	MaxAttempts = 10

	// The dispatcher wakes on every commit; polling only covers writes made outside db.Write.
	pollInterval = 5 * time.Second

	handlersGroup = `group:"bus_handlers"`
)

// retryDelays is the wait after each failure; the last entry repeats.
var retryDelays = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}

var (
	ErrNotParked        = errors.New("bus: event is not parked for that handler")
	ErrDuplicateHandler = errors.New("bus: handler name registered twice")
	ErrInvalidHandler   = errors.New("bus: handler needs a name, event types and a func")
)

// HandleFunc runs inside a write transaction. Its changes, any events it publishes and the
// processed-event record commit together. It must not do network I/O; enqueue a job for that.
type HandleFunc func(ctx context.Context, tx *gorm.DB, ev Event) error

// Handler is one durable subscription with its own cursor and delivery state.
type Handler struct {
	// Name is stable across releases; it keys the cursor, e.g. "acquisition.search-on-title-added".
	Name       string
	EventTypes []string
	Handle     HandleFunc
}

// AsHandler provides constructor's Handler to the dispatcher.
func AsHandler(constructor any) any {
	return fx.Annotate(constructor, fx.ResultTags(handlersGroup))
}

type subscriptionRow struct {
	Handler string `gorm:"primaryKey"`
	Cursor  int64
}

func (subscriptionRow) TableName() string { return "platform_subscriptions" }

type queueRow struct {
	Handler       string `gorm:"primaryKey"`
	EventID       string `gorm:"primaryKey"`
	Seq           int64
	AggregateID   string
	Attempts      int
	NextAttemptAt time.Time
	ParkedAt      *time.Time
	LastError     string
}

func (queueRow) TableName() string { return "platform_handler_queue" }

type processedRow struct {
	Handler     string `gorm:"primaryKey"`
	EventID     string `gorm:"primaryKey"`
	ProcessedAt time.Time
}

func (processedRow) TableName() string { return "platform_processed_events" }

// Dispatcher delivers outbox events to each handler at least once, in order per aggregate.
// A failing event holds back only that handler's later events for the same aggregate.
type Dispatcher struct {
	db       *db.DB
	handlers []Handler
	now      func() time.Time
	wakes    []chan struct{}
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// afterHandle runs between handling an event and moving the cursor. Tests use it to simulate a crash.
	afterHandle func(Handler, Event) error
}

type DispatcherParams struct {
	fx.In

	DB       *db.DB
	Bus      *Bus
	Handlers []Handler `group:"bus_handlers"`
}

func NewDispatcher(p DispatcherParams) (*Dispatcher, error) {
	names := make(map[string]bool, len(p.Handlers))
	for _, h := range p.Handlers {
		if h.Name == "" || len(h.EventTypes) == 0 || h.Handle == nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidHandler, h.Name)
		}
		if names[h.Name] {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateHandler, h.Name)
		}
		names[h.Name] = true
	}

	d := &Dispatcher{db: p.DB, handlers: p.Handlers, now: time.Now}
	for range p.Handlers {
		d.wakes = append(d.wakes, make(chan struct{}, 1))
	}
	p.Bus.OnDurable(d.Notify)
	return d, nil
}

// Start registers every handler's cursor and runs one delivery loop per handler.
func (d *Dispatcher) Start(ctx context.Context) error {
	if err := d.ensureSubscriptions(ctx); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	d.cancel = cancel
	for i, h := range d.handlers {
		d.wg.Go(func() { d.run(runCtx, h, d.wakes[i]) })
	}
	return nil
}

// Stop cancels the delivery loops and waits for them until ctx ends.
func (d *Dispatcher) Stop(ctx context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	stopped := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("bus: handlers still running at shutdown: %w", ctx.Err())
	}
}

// Notify wakes every handler loop to look for new events.
func (d *Dispatcher) Notify() {
	for _, wake := range d.wakes {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func (d *Dispatcher) ensureSubscriptions(ctx context.Context) error {
	return d.db.Write(ctx, func(tx *gorm.DB) error {
		for _, h := range d.handlers {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&subscriptionRow{Handler: h.Name}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *Dispatcher) run(ctx context.Context, h Handler, wake <-chan struct{}) {
	for {
		worked, err := d.step(ctx, h)
		if err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "event dispatch failed", "handler", h.Name, "error", err)
		}
		if worked && err == nil {
			continue
		}

		wait := pollInterval
		if due, ok := d.nextDue(ctx, h); ok {
			wait = min(wait, max(due.Sub(d.now()), 0))
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-wake:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// step delivers at most one event to h: a due retry first, then the next outbox event.
// It reports whether it did any work.
func (d *Dispatcher) step(ctx context.Context, h Handler) (bool, error) {
	now := d.now().UTC()

	head, found, err := d.dueHead(ctx, h, now)
	if err != nil {
		return false, err
	}
	if found {
		return true, d.retry(ctx, h, head, now)
	}

	var sub subscriptionRow
	if err := d.db.Read(ctx).Where("handler = ?", h.Name).Take(&sub).Error; err != nil {
		return false, err
	}
	var latest int64
	if err := d.db.Read(ctx).Model(&outboxRow{}).Select("COALESCE(MAX(seq), 0)").Scan(&latest).Error; err != nil {
		return false, err
	}
	var next []outboxRow
	if err := d.db.Read(ctx).
		Where("seq > ? AND seq <= ? AND type IN ?", sub.Cursor, latest, h.EventTypes).
		Order("seq").Limit(1).Find(&next).Error; err != nil {
		return false, err
	}
	if len(next) == 0 {
		if latest > sub.Cursor {
			return false, d.advance(ctx, h, latest)
		}
		return false, nil
	}
	return true, d.deliverNext(ctx, h, next[0], now)
}

func (d *Dispatcher) deliverNext(ctx context.Context, h Handler, row outboxRow, now time.Time) error {
	var held int64
	if err := d.db.Read(ctx).Model(&queueRow{}).
		Where("handler = ? AND aggregate_id = ? AND parked_at IS NULL", h.Name, row.AggregateID).
		Count(&held).Error; err != nil {
		return err
	}
	if held > 0 {
		return d.db.Write(ctx, func(tx *gorm.DB) error {
			waiting := queueRow{Handler: h.Name, EventID: row.ID, Seq: row.Seq, AggregateID: row.AggregateID, NextAttemptAt: now}
			if err := tx.Create(&waiting).Error; err != nil {
				return err
			}
			return setCursor(tx, h, row.Seq)
		})
	}

	ev := row.event()
	handleErr := d.handle(ctx, h, ev)
	if err := d.checkpoint(h, ev); err != nil {
		return err
	}
	return d.db.Write(ctx, func(tx *gorm.DB) error {
		if handleErr != nil {
			failed := queueRow{Handler: h.Name, EventID: row.ID, Seq: row.Seq, AggregateID: row.AggregateID}
			recordFailure(&failed, handleErr, now)
			logFailure(ctx, h, ev, failed)
			if err := tx.Create(&failed).Error; err != nil {
				return err
			}
		}
		return setCursor(tx, h, row.Seq)
	})
}

func (d *Dispatcher) retry(ctx context.Context, h Handler, queued queueRow, now time.Time) error {
	var row outboxRow
	if err := d.db.Read(ctx).Where("id = ?", queued.EventID).Take(&row).Error; err != nil {
		return fmt.Errorf("loading queued event %s: %w", queued.EventID, err)
	}
	ev := row.event()
	handleErr := d.handle(ctx, h, ev)
	if err := d.checkpoint(h, ev); err != nil {
		return err
	}
	return d.db.Write(ctx, func(tx *gorm.DB) error {
		if handleErr == nil {
			return tx.Delete(&queued).Error
		}
		recordFailure(&queued, handleErr, now)
		logFailure(ctx, h, ev, queued)
		return tx.Save(&queued).Error
	})
}

func (d *Dispatcher) checkpoint(h Handler, ev Event) error {
	if d.afterHandle == nil {
		return nil
	}
	return d.afterHandle(h, ev)
}

// handle runs h once per event: an event already in processed_events is skipped.
func (d *Dispatcher) handle(ctx context.Context, h Handler, ev Event) error {
	return d.db.Write(ctx, func(tx *gorm.DB) (err error) {
		var done int64
		if err := tx.Model(&processedRow{}).Where("handler = ? AND event_id = ?", h.Name, ev.ID).Count(&done).Error; err != nil {
			return err
		}
		if done > 0 {
			return nil
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("handler panicked: %v", recovered)
			}
		}()
		if err := h.Handle(ctx, tx, ev); err != nil {
			return err
		}
		return tx.Create(&processedRow{Handler: h.Name, EventID: ev.ID, ProcessedAt: d.now().UTC()}).Error
	})
}

func (d *Dispatcher) advance(ctx context.Context, h Handler, seq int64) error {
	return d.db.Write(ctx, func(tx *gorm.DB) error { return setCursor(tx, h, seq) })
}

func setCursor(tx *gorm.DB, h Handler, seq int64) error {
	return tx.Model(&subscriptionRow{}).Where("handler = ? AND cursor < ?", h.Name, seq).Update("cursor", seq).Error
}

// headCondition keeps only the oldest unparked queued event of each aggregate.
const headCondition = "parked_at IS NULL AND seq = (SELECT MIN(q.seq) FROM platform_handler_queue q " +
	"WHERE q.handler = platform_handler_queue.handler AND q.aggregate_id = platform_handler_queue.aggregate_id AND q.parked_at IS NULL)"

func (d *Dispatcher) dueHead(ctx context.Context, h Handler, now time.Time) (queueRow, bool, error) {
	var heads []queueRow
	err := d.db.Read(ctx).
		Where("handler = ? AND next_attempt_at <= ?", h.Name, now).
		Where(headCondition).
		Order("seq").Limit(1).Find(&heads).Error
	if err != nil || len(heads) == 0 {
		return queueRow{}, false, err
	}
	return heads[0], true, nil
}

func (d *Dispatcher) nextDue(ctx context.Context, h Handler) (time.Time, bool) {
	var heads []queueRow
	err := d.db.Read(ctx).Where("handler = ?", h.Name).Where(headCondition).
		Order("next_attempt_at").Limit(1).Find(&heads).Error
	if err != nil || len(heads) == 0 {
		return time.Time{}, false
	}
	return heads[0].NextAttemptAt, true
}

func recordFailure(row *queueRow, err error, now time.Time) {
	row.Attempts++
	row.LastError = err.Error()
	row.NextAttemptAt = now.Add(RetryDelay(row.Attempts))
	if row.Attempts >= MaxAttempts {
		row.ParkedAt = &now
	}
}

func logFailure(ctx context.Context, h Handler, ev Event, row queueRow) {
	if row.ParkedAt != nil {
		slog.ErrorContext(ctx, "event parked", "handler", h.Name, "event", ev.ID, "type", ev.Type, "attempts", row.Attempts, "error", row.LastError)
		return
	}
	slog.WarnContext(ctx, "event handler failed", "handler", h.Name, "event", ev.ID, "type", ev.Type, "attempts", row.Attempts, "retryAt", row.NextAttemptAt, "error", row.LastError)
}

// RetryDelay is the wait after the given number of failed attempts.
func RetryDelay(attempts int) time.Duration {
	return retryDelays[min(max(attempts, 1), len(retryDelays))-1]
}
