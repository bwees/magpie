package domain

// Message is an event or command struct published from a context's api package.
type Message interface {
	// Type is "<context>.<Name>", e.g. "catalog.TitleAdded". It is the bus event type.
	Type() string
	// AggregateID is the ordering key: a handler sees one aggregate's messages in publish order.
	AggregateID() string
	// Durable messages go through the outbox. The rest are ephemeral and reach only in-process subscribers.
	Durable() bool
}

// SourceKey identifies a record in the app a migration reads from, e.g. "sonarr:12" or "radarr:40".
// Importers are idempotent on it.
type SourceKey string
