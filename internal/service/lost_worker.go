package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
)

// Work whose worker died (KI-65). Runs, conversation runs and backend tasks
// are acked on accept (ADR-016) and never redelivered, and a worker's own
// timeouts die with it. While a worker executes such work it sends a
// heartbeat every runtime.heartbeat_interval; the Go Core records it in the
// store, and the stuck-work watchdog ends the work whose heartbeats stopped
// through the work's own completion path. Work without any heartbeat waits in
// NATS for a free worker and is not ended; a healthy long run keeps sending
// heartbeats, also while it waits for an LLM or a HITL approval.

// LostWorkerAfter is how long work may go without a worker heartbeat before
// the watchdog ends it: runtime.heartbeat_timeout plus two heartbeat
// intervals for delivery delays (a Go Core catching up on queued heartbeats).
// The interval is the one sent to the worker, but never less than the 30 s
// of a worker that does not read it. Zero (no heartbeat_timeout) disables
// the heartbeat checks.
func LostWorkerAfter(cfg *config.Runtime) time.Duration {
	if cfg == nil || cfg.HeartbeatTimeout <= 0 {
		return 0
	}
	interval := max(cfg.WorkerHeartbeatInterval(), config.DefaultWorkerHeartbeatInterval)
	return cfg.HeartbeatTimeout + 2*interval
}

// heartbeatSeconds is the heartbeat interval sent to the worker with a start.
func heartbeatSeconds(cfg *config.Runtime) int {
	return int(cfg.WorkerHeartbeatInterval() / time.Second)
}

// lostWorkerReason is the error of work the watchdog ends.
func lostWorkerReason(after time.Duration) string {
	return fmt.Sprintf("heartbeat timeout (worker unresponsive): no heartbeat for %s", after)
}

// EndRunsWithLostWorker stops the running runs whose worker stopped sending
// heartbeats, as timed out, through the run stop path (the worker is told to
// stop, the run is finalized in its own tenant); a run that ended meanwhile
// is skipped. It returns how many lost runs it handled.
func (s *RuntimeService) EndRunsWithLostWorker(ctx context.Context) (int, error) {
	after := LostWorkerAfter(s.runtimeCfg)
	if after <= 0 {
		return 0, nil
	}
	lost, err := s.store.ListRunsWithStaleHeartbeat(ctx, after, staleRunBatch)
	if err != nil {
		return 0, fmt.Errorf("list runs with lost worker: %w", err)
	}
	handled := 0
	var errs []error
	for i := range lost {
		r := &lost[i]
		runCtx := withEntityTenant(ctx, r.TenantID)
		slog.WarnContext(runCtx, "run worker heartbeat lost, stopping the run", "run_id", r.ID, "after", after)
		if err := skipEndedRun(runCtx, s.cancelRunWithReason(runCtx, r.ID, lostWorkerReason(after)), "cancelRunWithReason", r.ID); err != nil {
			errs = append(errs, fmt.Errorf("run %s: %w", r.ID, err))
			continue
		}
		handled++
	}
	return handled, errors.Join(errs...)
}
