package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// S6-F review 4: a run the control plane ended (cancel, timeout, stall) is
// terminal at once, but its worker only got runs.cancel and may still write
// to the workspace until its completion arrives. The runtime remembers such
// runs until the worker confirms the stop, and tells WorkerStopped
// listeners when it does.
func TestRuntime_WorkerMayStillWriteUntilItConfirmsTheStop(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID: "run-stop", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1", Status: run.StatusRunning, StartedAt: time.Now(),
	})
	store.mu.Unlock()
	var confirmed []string
	svc.SetOnWorkerStopped(func(_ context.Context, runID string) { confirmed = append(confirmed, runID) })

	if svc.WorkerMayStillWrite("run-stop") {
		t.Fatal("a running run is not a stopped one")
	}
	if err := svc.CancelRun(ctx, "run-stop"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if !svc.WorkerMayStillWrite("run-stop") || svc.WorkerStopGrace() <= 0 {
		t.Fatal("the worker of a cancelled run may still write until it confirms the stop")
	}

	// The worker's completion of the ended run confirms the stop, once.
	payload := &messagequeue.RunCompletePayload{RunID: "run-stop", Status: string(run.StatusCancelled)}
	if err := svc.HandleRunComplete(ctx, payload); err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}
	if err := svc.HandleRunComplete(ctx, payload); err != nil {
		t.Fatalf("HandleRunComplete again: %v", err)
	}
	if svc.WorkerMayStillWrite("run-stop") || len(confirmed) != 1 || confirmed[0] != "run-stop" {
		t.Fatalf("after the worker's completion: may write %v, confirmations %v, want no and one", svc.WorkerMayStillWrite("run-stop"), confirmed)
	}
}
