package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// startPeriodic is the ticker lifecycle shared by the stuck-work watchdog and
// the retention job.

func TestStartPeriodic_RunAtStart(t *testing.T) {
	for _, runAtStart := range []bool{true, false} {
		var runs atomic.Int32
		stop := startPeriodic(context.Background(), time.Hour, runAtStart, func(context.Context) { runs.Add(1) })
		stop()
		want := int32(0)
		if runAtStart {
			want = 1
		}
		if got := runs.Load(); got != want {
			t.Errorf("runAtStart %v: %d runs before the first tick, want %d", runAtStart, got, want)
		}
	}
}

func TestStartPeriodic_RunsOnEveryTick(t *testing.T) {
	ticks := make(chan struct{}, 16)
	stop := startPeriodic(context.Background(), 10*time.Millisecond, false, func(context.Context) { ticks <- struct{}{} })
	defer stop()
	for i := range 3 {
		select {
		case <-ticks:
		case <-time.After(5 * time.Second):
			t.Fatalf("tick %d did not run", i+1)
		}
	}
}

func TestStartPeriodic_StopCancelsAndWaits(t *testing.T) {
	started := make(chan struct{})
	var finished atomic.Bool
	stop := startPeriodic(context.Background(), time.Hour, true, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		finished.Store(true)
	})
	<-started
	stop()
	if !finished.Load() {
		t.Fatal("stop returned before the running call ended")
	}
}

func TestStartPeriodic_EndsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var runs atomic.Int32
	stop := startPeriodic(ctx, time.Millisecond, false, func(context.Context) { runs.Add(1) })
	cancel()
	stop() // returns once the loop has seen the cancelled context
	after := runs.Load()
	time.Sleep(20 * time.Millisecond)
	if runs.Load() != after {
		t.Fatal("ran after its context ended")
	}
}
