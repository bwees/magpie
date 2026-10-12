package db

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

const (
	cancelAfter = 100 * time.Millisecond
	// promptCancel is well under the writer's busy timeout, so a wait that ignores ctx fails it.
	promptCancel = 2 * time.Second
)

// holdWriter keeps a write transaction open until the test ends or release is called.
func holdWriter(t *testing.T, d *DB) (release func()) {
	t.Helper()
	held := make(chan struct{})
	released := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- d.Write(context.Background(), func(*gorm.DB) error {
			close(held)
			<-released
			return nil
		})
	}()
	<-held
	release = sync.OnceFunc(func() {
		close(released)
		if err := <-done; err != nil {
			t.Errorf("held write: %v", err)
		}
	})
	t.Cleanup(release)
	return release
}

func writeWithCancel(d *DB) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
	defer cancel()
	begun := time.Now()
	err := d.Write(ctx, func(*gorm.DB) error { return nil })
	return time.Since(begun), err
}

func TestWriteStopsWaitingForTheWriterWhenCancelled(t *testing.T) {
	d := newTestDB(t).open(t)
	release := holdWriter(t, d)
	stuck := time.AfterFunc(promptCancel, release)
	defer stuck.Stop()

	took, err := writeWithCancel(d)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the context's error", err)
	}
	if took > promptCancel {
		t.Errorf("waited %v for a busy writer after the context ended", took)
	}
}

func TestWriteStopsWaitingForAnotherConnectionsLockWhenCancelled(t *testing.T) {
	td := newTestDB(t)
	d := td.open(t)
	other := td.open(t)
	holdWriter(t, other)

	took, err := writeWithCancel(d)
	if err == nil {
		t.Fatal("write succeeded while another connection held the lock")
	}
	if took > promptCancel {
		t.Errorf("waited %v for another connection's lock after the context ended", took)
	}
}
