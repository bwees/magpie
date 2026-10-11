-- +goose Up
CREATE TABLE platform_outbox (
    seq          INTEGER PRIMARY KEY AUTOINCREMENT,
    id           TEXT NOT NULL UNIQUE,
    type         TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload      TEXT NOT NULL CHECK (json_valid(payload)),
    occurred_at  DATETIME NOT NULL
);
CREATE INDEX platform_outbox_type_seq ON platform_outbox (type, seq);

CREATE TABLE platform_subscriptions (
    handler TEXT PRIMARY KEY,
    cursor  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE platform_handler_queue (
    handler         TEXT NOT NULL,
    event_id        TEXT NOT NULL,
    seq             INTEGER NOT NULL,
    aggregate_id    TEXT NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at DATETIME NOT NULL,
    parked_at       DATETIME,
    last_error      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (handler, event_id)
);
CREATE INDEX platform_handler_queue_aggregate ON platform_handler_queue (handler, aggregate_id, seq);

CREATE TABLE platform_processed_events (
    handler      TEXT NOT NULL,
    event_id     TEXT NOT NULL,
    processed_at DATETIME NOT NULL,
    PRIMARY KEY (handler, event_id)
);
CREATE INDEX platform_processed_events_processed_at ON platform_processed_events (processed_at);

-- +goose Down
DROP TABLE platform_processed_events;
DROP TABLE platform_handler_queue;
DROP TABLE platform_subscriptions;
DROP TABLE platform_outbox;
