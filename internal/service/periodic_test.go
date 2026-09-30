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
	ran := make(chan struct{}, 1)
	stop := startPeriodic(context.Background(), time.Hour, true, func(context.Context) { ran <- struct{}{} })
	defer stop()
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("runAtStart: no run before the first tick")
	}
}

func TestStartPeriodic_NoRunAtStart(t *testing.T) {
	var runs atomic.Int32
	stop := startPeriodic(context.Background(), time.Hour, false, func(context.Context) { runs.Add(1) })
	time.Sleep(20 * time.Millisecond)
	stop()
	if got := runs.Load(); got != 0 {
		t.Fatalf("%d runs before the first tick, want 0", got)
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

// A context that has ended runs nothing, not even the call at start.
func TestStartPeriodic_EndedContextRunsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var runs atomic.Int32
	stop := startPeriodic(ctx, time.Millisecond, true, func(context.Context) { runs.Add(1) })
	stop()
	if got := runs.Load(); got != 0 {
		t.Fatalf("%d runs with an ended context, want 0", got)
	}
}
