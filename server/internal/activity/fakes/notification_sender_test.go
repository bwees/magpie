package fakes

import (
	"context"
	"errors"
	"testing"

	"magpie/internal/activity"
)

const (
	validURL   = "discord://token@channel"
	invalidURL = "gopher://nowhere"
)

func TestNotificationSenderValidatesAndRecords(t *testing.T) {
	f := NewNotificationSender()
	if err := f.Validate(validURL); err != nil {
		t.Errorf("valid url: %v", err)
	}
	if err := f.Validate(invalidURL); !errors.Is(err, activity.ErrInvalidURL) {
		t.Errorf("invalid url err = %v", err)
	}

	msg := activity.Message{Title: "Dune is available", Link: "/titles/1"}
	if err := f.Send(context.Background(), validURL, msg); err != nil {
		t.Fatal(err)
	}
	if sent := f.Sent(); len(sent) != 1 || sent[0].Message != msg || sent[0].URL != validURL {
		t.Errorf("Sent = %+v", sent)
	}
}

func TestNotificationSenderScriptedOutage(t *testing.T) {
	f := NewNotificationSender()
	f.FailNext(NotificationSenderSend, activity.ErrUnavailable, activity.ErrUnavailable)

	for range 2 {
		if err := f.Send(context.Background(), validURL, activity.Message{}); !errors.Is(err, activity.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	}
	if err := f.Send(context.Background(), validURL, activity.Message{}); err != nil {
		t.Errorf("third send: %v", err)
	}
	if len(f.Sent()) != 1 {
		t.Errorf("failed sends were recorded: %+v", f.Sent())
	}
}
