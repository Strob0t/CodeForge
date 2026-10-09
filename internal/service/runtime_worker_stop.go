package service

import (
	"context"
	"sync"
	"time"
)

// defaultWorkerStopGrace is how long the worker of a stopped run is assumed
// to possibly still write when no heartbeat timeout is configured.
const defaultWorkerStopGrace = 3 * time.Minute

// workerStops remembers the runs the control plane ended (stopRun: cancel,
// timeout, stall, limits) whose worker has not confirmed the stop yet. Such a
// run is terminal at once, but its worker only got runs.cancel and may still
// change the workspace until its completion arrives (S6-F review 4).
type workerStops struct {
	mu        sync.Mutex
	since     map[string]time.Time
	onStopped func(ctx context.Context, runID string)
}

func (w *workerStops) note(runID string, now time.Time, grace time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.since == nil {
		w.since = make(map[string]time.Time)
	}
	for id, at := range w.since { // a worker that never confirms is assumed gone
		if now.Sub(at) > 10*grace {
			delete(w.since, id)
		}
	}
	w.since[runID] = now
}

// confirm forgets the run and reports whether it was waiting for its worker.
func (w *workerStops) confirm(runID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.since[runID]
	delete(w.since, runID)
	return ok
}

func (w *workerStops) pending(runID string, now time.Time, grace time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	at, ok := w.since[runID]
	return ok && now.Sub(at) <= grace
}

// SetOnWorkerStopped registers fn, called when the worker of a run the
// control plane ended confirms the stop (its completion arrives).
func (s *RuntimeService) SetOnWorkerStopped(fn func(ctx context.Context, runID string)) {
	s.workerStops.mu.Lock()
	s.workerStops.onStopped = fn
	s.workerStops.mu.Unlock()
}

// WorkerStopGrace is how long the worker of a stopped run may still write
// without having confirmed the stop: after it, the worker is taken as gone
// (the lost-worker deadline, LostWorkerAfter).
func (s *RuntimeService) WorkerStopGrace() time.Duration {
	if grace := LostWorkerAfter(s.runtimeCfg); grace > 0 {
		return grace
	}
	return defaultWorkerStopGrace
}

// WorkerMayStillWrite reports whether the control plane ended the run less
// than WorkerStopGrace ago and its worker has not confirmed the stop.
func (s *RuntimeService) WorkerMayStillWrite(runID string) bool {
	return s.workerStops.pending(runID, time.Now(), s.WorkerStopGrace())
}

// noteWorkerStop records that the control plane ends the run.
func (s *RuntimeService) noteWorkerStop(runID string) {
	s.workerStops.note(runID, time.Now(), s.WorkerStopGrace())
}

// confirmWorkerStop handles the worker's completion of a run that already
// ended: a run the control plane stopped is no longer written to.
func (s *RuntimeService) confirmWorkerStop(ctx context.Context, runID string) {
	if !s.workerStops.confirm(runID) {
		return
	}
	s.workerStops.mu.Lock()
	fn := s.workerStops.onStopped
	s.workerStops.mu.Unlock()
	if fn != nil {
		fn(ctx, runID)
	}
}
