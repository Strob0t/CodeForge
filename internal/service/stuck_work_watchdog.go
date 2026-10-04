package service

import (
	"context"
	"log/slog"
	"time"
)

// StuckWorkWatchdog periodically ends work that waits for a worker result
// which no longer comes (KI-28, KI-65): work is acked on accept (ADR-016) or
// its result is lost, and a worker that dies takes its own timeouts with it.
// Each check finds its stuck items in the store, so the watchdog also covers
// work started before a Go Core restart and runs on every replica (the
// store's status predicates let only one replica end an item), and ends them
// through the owning service's completion path.
type StuckWorkWatchdog struct {
	interval time.Duration
	checks   []StuckWorkCheck
}

// StuckWorkCheck is one kind of work the watchdog watches.
type StuckWorkCheck struct {
	Name string
	// EndStuck ends the stuck items and returns how many it handled.
	EndStuck func(ctx context.Context) (int, error)
}

// NewStuckWorkWatchdog creates a watchdog that runs checks every interval.
func NewStuckWorkWatchdog(interval time.Duration, checks ...StuckWorkCheck) *StuckWorkWatchdog {
	return &StuckWorkWatchdog{interval: interval, checks: checks}
}

// RunOnce runs every check once; a failing check does not stop the others.
// Once ctx ends (the watchdog is stopped) no further check starts; a check
// under way ends the item it is ending with a context of its own
// (FailStuckQualityGates) and then returns.
func (w *StuckWorkWatchdog) RunOnce(ctx context.Context) {
	for _, check := range w.checks {
		if ctx.Err() != nil {
			return
		}
		handled, err := check.EndStuck(ctx)
		if err != nil {
			slog.Error("stuck-work watchdog check failed", "check", check.Name, "error", err)
		}
		if handled > 0 {
			slog.Warn("stuck-work watchdog ended stuck work", "check", check.Name, "count", handled)
		}
	}
}

// Start runs the checks every interval until ctx ends or the returned stop
// is called. stop cancels the sweep's context and waits for the sweep: an
// item being ended is finished (bounded by the check), no further item or
// check starts.
func (w *StuckWorkWatchdog) Start(ctx context.Context) (stop func()) {
	return startPeriodic(ctx, w.interval, false, w.RunOnce)
}
