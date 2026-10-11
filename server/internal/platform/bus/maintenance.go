package bus

import (
	"context"
	"time"

	"gorm.io/gorm"
)

const (
	// OutboxRetention is how long a delivered outbox row is kept after every handler has passed it.
	OutboxRetention = 7 * 24 * time.Hour
	// ProcessedRetention is how long handler idempotency records are kept.
	ProcessedRetention = 30 * 24 * time.Hour
)

type ParkedEvent struct {
	Handler     string
	EventID     string
	EventType   string
	AggregateID string
	Attempts    int
	ParkedAt    time.Time
	LastError   string
}

// Parked lists events that exhausted their attempts, newest first.
func (d *Dispatcher) Parked(ctx context.Context) ([]ParkedEvent, error) {
	parked := []ParkedEvent{}
	err := d.db.Read(ctx).Table("platform_handler_queue AS q").
		Select("q.handler, q.event_id, o.type AS event_type, q.aggregate_id, q.attempts, q.parked_at, q.last_error").
		Joins("JOIN platform_outbox AS o ON o.id = q.event_id").
		Where("q.parked_at IS NOT NULL").
		Order("q.parked_at DESC").
		Scan(&parked).Error
	return parked, err
}

// Retry re-queues a parked event for its handler with a fresh set of attempts.
func (d *Dispatcher) Retry(ctx context.Context, handler, eventID string) error {
	err := d.db.Write(ctx, func(tx *gorm.DB) error {
		result := tx.Model(&queueRow{}).
			Where("handler = ? AND event_id = ? AND parked_at IS NOT NULL", handler, eventID).
			Updates(map[string]any{"parked_at": nil, "attempts": 0, "next_attempt_at": d.now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotParked
		}
		return nil
	})
	if err == nil {
		d.Notify()
	}
	return err
}

// Prune drops outbox rows every registered handler has passed once they are older than OutboxRetention,
// keeping any row still queued for a handler, and drops processed-event records older than ProcessedRetention.
func (d *Dispatcher) Prune(ctx context.Context) error {
	now := d.now().UTC()
	names := make([]string, 0, len(d.handlers))
	for _, h := range d.handlers {
		names = append(names, h.Name)
	}
	return d.db.Write(ctx, func(tx *gorm.DB) error {
		var passed int64
		lowestCursor := tx.Model(&subscriptionRow{}).Select("COALESCE(MIN(cursor), 0)").Where("handler IN ?", names)
		if len(names) == 0 {
			lowestCursor = tx.Model(&outboxRow{}).Select("COALESCE(MAX(seq), 0)")
		}
		if err := lowestCursor.Scan(&passed).Error; err != nil {
			return err
		}
		queued := tx.Model(&queueRow{}).Select("event_id")
		if err := tx.Where("seq <= ? AND occurred_at < ? AND id NOT IN (?)", passed, now.Add(-OutboxRetention), queued).
			Delete(&outboxRow{}).Error; err != nil {
			return err
		}
		return tx.Where("processed_at < ?", now.Add(-ProcessedRetention)).Delete(&processedRow{}).Error
	})
}
