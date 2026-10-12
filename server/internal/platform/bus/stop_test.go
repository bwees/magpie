package bus

import (
	"context"
	"errors"
	"testing"
	"time"

	"magpie/internal/platform/db/dbtest"

	"gorm.io/gorm"
)

const (
	promptStop  = 2 * time.Second
	cancelAfter = 100 * time.Millisecond
)

func TestStopIsPromptWhileAnotherWriteHoldsTheWriter(t *testing.T) {
	d := dbtest.Open(t)
	h := newHarness(t, d, newRecorder(handlerRecorder).handler())
	h.disp.now = time.Now
	handled := make(chan struct{})
	writerTaken := make(chan struct{})
	h.disp.afterHandle = func(Handler, Event) error {
		close(handled)
		<-writerTaken
		return nil
	}
	if err := h.disp.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.publish(aggregateA, "busy")
	select {
	case <-handled:
	case <-time.After(waitForDelivery):
		t.Fatal("event never delivered")
	}

	released := make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- d.Write(context.Background(), func(*gorm.DB) error {
			close(writerTaken)
			<-released
			return nil
		})
	}()
	<-writerTaken

	ctx, cancel := context.WithTimeout(context.Background(), promptStop)
	defer cancel()
	stopErr := h.disp.Stop(ctx)
	close(released)
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if stopErr != nil {
		t.Errorf("stop waited for another transaction to release the writer: %v", stopErr)
	}
}

func TestStopGivesUpOnAHandlerThatIgnoresCancellation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	stuck := Handler{
		Name:       handlerRecorder,
		EventTypes: []string{typeThingHappened},
		Handle: func(context.Context, *gorm.DB, Event) error {
			close(entered)
			<-release
			return nil
		},
	}
	h := newHarness(t, dbtest.Open(t), stuck)
	h.disp.now = time.Now
	if err := h.disp.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.publish(aggregateA, "stuck")
	select {
	case <-entered:
	case <-time.After(waitForDelivery):
		t.Fatal("event never delivered")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
	defer cancel()
	err := h.disp.Stop(ctx)
	close(release)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop = %v, want the context's error", err)
	}
	if err := h.disp.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
