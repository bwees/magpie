// Package bus carries events between contexts: a transactional outbox for durable events,
// a per-handler dispatcher, and an in-process bus for ephemeral events and the live gateway.
package bus

import (
	"encoding/json"
	"fmt"
	"time"

	"magpie/internal/domain"
)

type Event struct {
	// ID is a ULID; handlers dedupe on it.
	ID string
	// Type names the payload, e.g. "acquisition.DownloadCompleted".
	Type       string
	OccurredAt time.Time
	// AggregateID is the ordering key: one handler sees one aggregate's events in publish order.
	AggregateID string
	// Payload is one of the structs in a context's api package. Events read back from the outbox carry
	// json.RawMessage; use Decode to get the struct.
	Payload any
}

type eventID string

func NewEvent(eventType, aggregateID string, payload any) Event {
	return Event{
		ID:          string(domain.NewID[eventID]()),
		Type:        eventType,
		OccurredAt:  time.Now().UTC(),
		AggregateID: aggregateID,
		Payload:     payload,
	}
}

// EventOf wraps an api message, taking its type and aggregate ID from the message itself.
func EventOf(m domain.Message) Event {
	return NewEvent(m.Type(), m.AggregateID(), m)
}

// Decode returns the payload as T, whether it is still the struct or was read back as JSON.
func Decode[T any](ev Event) (T, error) {
	var payload T
	switch raw := ev.Payload.(type) {
	case T:
		return raw, nil
	case json.RawMessage:
		if err := json.Unmarshal(raw, &payload); err != nil {
			return payload, fmt.Errorf("decoding %s payload: %w", ev.Type, err)
		}
		return payload, nil
	default:
		return payload, fmt.Errorf("%s payload is %T, not %T", ev.Type, ev.Payload, payload)
	}
}
