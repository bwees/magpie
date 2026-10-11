package activity

import (
	"context"
	"errors"
)

// NotificationSender is implemented by internal/adapters/shoutrrr (the nicholas-fedor fork).
type NotificationSender interface {
	// Validate parses a shoutrrr URL (discord://, telegram://, ntfy://, pushover://, smtp://, …).
	// ErrInvalidURL for unknown schemes or malformed tokens. No network call.
	Validate(url string) error
	// Send delivers one message. Returns ErrRejected (4xx from the service) or ErrUnavailable (network/5xx).
	Send(ctx context.Context, url string, m Message) error
}

type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Link  string `json:"link"`
}

var (
	ErrInvalidURL  = errors.New("notify: invalid notification url")
	ErrRejected    = errors.New("notify: rejected by the service")
	ErrUnavailable = errors.New("notify: service unavailable")
)
