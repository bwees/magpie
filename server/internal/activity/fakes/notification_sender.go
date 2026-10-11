// Package fakes holds in-memory fakes of Activity's ports, for tests and for booting without real adapters.
package fakes

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"sync"

	"magpie/internal/activity"
	"magpie/internal/platform/scripted"
)

const (
	NotificationSenderValidate scripted.Method = "NotificationSender.Validate"
	NotificationSenderSend     scripted.Method = "NotificationSender.Send"
)

// Schemes the fake accepts; shoutrrr supports more.
const (
	SchemeDiscord  = "discord"
	SchemeTelegram = "telegram"
	SchemeNtfy     = "ntfy"
	SchemePushover = "pushover"
	SchemeSMTP     = "smtp"
)

var knownSchemes = []string{SchemeDiscord, SchemeTelegram, SchemeNtfy, SchemePushover, SchemeSMTP}

type Sent struct {
	URL     string
	Message activity.Message
}

// NotificationSender accepts URLs with a known scheme and a host, and records every sent message.
type NotificationSender struct {
	scripted.Script

	mu   sync.Mutex
	sent []Sent
}

var _ activity.NotificationSender = (*NotificationSender)(nil)

func NewNotificationSender() *NotificationSender {
	return &NotificationSender{}
}

// Sent returns every delivered message, in send order.
func (f *NotificationSender) Sent() []Sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent)
}

func (f *NotificationSender) Validate(raw string) error {
	if err := f.Enter(context.Background(), NotificationSenderValidate); err != nil {
		return err
	}
	return validate(raw)
}

func (f *NotificationSender) Send(ctx context.Context, raw string, m activity.Message) error {
	if err := f.Enter(ctx, NotificationSenderSend); err != nil {
		return err
	}
	if err := validate(raw); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, Sent{URL: raw, Message: m})
	return nil
}

func validate(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || !slices.Contains(knownSchemes, u.Scheme) {
		return fmt.Errorf("%w: %q", activity.ErrInvalidURL, raw)
	}
	return nil
}
