package utils

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduleRunsAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	done := make(chan struct{})

	go func() {
		Schedule(ctx, 20*time.Millisecond, 0, func(time.Time) {
			if calls.Add(1) >= 2 {
				cancel()
			}
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(800 * time.Millisecond):
		t.Fatal("schedule did not stop after cancel")
	}

	if calls.Load() < 1 {
		t.Fatalf("expected at least one callback invocation, got %d", calls.Load())
	}
}
