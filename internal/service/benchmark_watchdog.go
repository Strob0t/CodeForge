package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// BenchmarkWatchdog detects and fails orphaned benchmark runs.
type BenchmarkWatchdog struct {
	store database.Store
}

// NewBenchmarkWatchdog creates a watchdog.
func NewBenchmarkWatchdog(store database.Store) *BenchmarkWatchdog {
	return &BenchmarkWatchdog{store: store}
}

// RunWatchdogOnce scans for benchmark runs stuck in "running" state beyond the
// per-type timeout and marks them as "failed". This prevents orphaned runs when
// a Python worker crashes or a NATS message is lost.
func (w *BenchmarkWatchdog) RunWatchdogOnce(ctx context.Context, timeout time.Duration) {
	runs, err := w.store.ListBenchmarkRunsFiltered(ctx, &benchmark.RunFilter{Status: benchmark.StatusRunning})
	if err != nil {
		slog.Warn("watchdog: failed to list running benchmark runs", "error", err)
		return
	}
	now := time.Now()
	for i := range runs {
		run := &runs[i]
		runTimeout := watchdogTimeoutForType(run.BenchmarkType, timeout)
		if run.CreatedAt.Before(now.Add(-runTimeout)) {
			run.Status = benchmark.StatusFailed
			run.ErrorMessage = fmt.Sprintf("watchdog timeout: run exceeded %s without completion", runTimeout)
			if err := w.store.UpdateBenchmarkRun(ctx, run); err != nil {
				slog.Warn("watchdog: failed to update stale run", "run_id", run.ID, "error", err)
				continue
			}
			slog.Warn("watchdog: marked stale benchmark run as failed", "run_id", run.ID, "age", now.Sub(run.CreatedAt))
		}
	}
}

// StartWatchdog launches a background goroutine that periodically calls
// RunWatchdogOnce to detect and fail orphaned benchmark runs. Returns a
// cancel function to stop the goroutine.
func (w *BenchmarkWatchdog) StartWatchdog(interval, timeout time.Duration) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunWatchdogOnce(ctx, timeout)
			}
		}
	}()
	return cancel
}
