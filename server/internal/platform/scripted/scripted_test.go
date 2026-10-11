package scripted

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

const (
	methodA Method = "Fake.A"
	methodB Method = "Fake.B"

	shortDelay = 20 * time.Millisecond
)

var (
	errFirst  = errors.New("first")
	errSecond = errors.New("second")
	errSticky = errors.New("sticky")
)

func TestZeroScriptPasses(t *testing.T) {
	var s Script
	if err := s.Enter(context.Background(), methodA); err != nil {
		t.Fatal(err)
	}
}

func TestFailNextQueuesErrorsInOrderBeforeFailAlways(t *testing.T) {
	var s Script
	s.FailAlways(methodA, errSticky)
	s.FailNext(methodA, errFirst, errSecond)

	for _, want := range []error{errFirst, errSecond, errSticky, errSticky} {
		if err := s.Enter(context.Background(), methodA); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	}
	if err := s.Enter(context.Background(), methodB); err != nil {
		t.Errorf("other method failed: %v", err)
	}

	s.FailAlways(methodA, nil)
	if err := s.Enter(context.Background(), methodA); err != nil {
		t.Errorf("cleared FailAlways still fails: %v", err)
	}
}

func TestCallsAreRecordedInOrder(t *testing.T) {
	var s Script
	for _, m := range []Method{methodA, methodB, methodA} {
		_ = s.Enter(context.Background(), m)
	}

	if got := s.Calls(); !slices.Equal(got, []Method{methodA, methodB, methodA}) {
		t.Errorf("Calls = %v", got)
	}
	if s.Called(methodA) != 2 || s.Called(methodB) != 1 {
		t.Errorf("Called A = %d, B = %d", s.Called(methodA), s.Called(methodB))
	}
}

func TestDelayWaitsAndHonoursCancellation(t *testing.T) {
	var s Script
	s.DelayMethod(methodA, shortDelay)

	start := time.Now()
	if err := s.Enter(context.Background(), methodA); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < shortDelay {
		t.Errorf("call took %v, want at least %v", elapsed, shortDelay)
	}

	s.Delay(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Enter(ctx, methodB); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
