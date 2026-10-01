package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-62: a plan step whose run stalled gets a new run (MagenticOne stall
// re-planning) instead of failing, at most runtime.stall_max_retries times
// per step.

func newStallReplanSetup(stallMaxRetries int) (*orchMockStore, *service.OrchestratorService) {
	store := newOrchStore()
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	runtimeSvc := service.NewRuntimeService(store, &runtimeMockQueue{}, bc, es,
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5, StallMaxRetries: stallMaxRetries})
	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc,
		&config.Orchestrator{MaxParallel: 4, PingPongMaxRounds: 3})
	runtimeSvc.SetOnRunComplete(orchSvc.HandleRunCompleted)
	return store, orchSvc
}

// endFirstRun ends the run of the plan's first step with status and error,
// as the runtime does.
func endFirstRun(t *testing.T, store *orchMockStore, orchSvc *service.OrchestratorService, planID string, status run.Status, errMsg string) string {
	t.Helper()
	ctx := context.Background()
	id := planState(t, store, planID).Steps[0].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: status, Error: errMsg}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	orchSvc.HandleRunCompleted(ctx, id, status)
	return id
}

func TestStallReplan_StalledStepGetsANewRun(t *testing.T) {
	store, orchSvc := newStallReplanSetup(1)
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
	})

	stalled := endFirstRun(t, store, orchSvc, p.ID, run.StatusFailed, run.StallDetectedError)

	got := planState(t, store, p.ID)
	step := got.Steps[0]
	if got.Status != plan.StatusRunning || step.Status != plan.StepStatusRunning || step.RunID == "" || step.RunID == stalled {
		t.Fatalf("plan %s, step %s with run %q: want the plan running and the step running a new run", got.Status, step.Status, step.RunID)
	}
	if r, _ := store.GetRun(context.Background(), step.RunID); r.TaskID != "t1" || r.Status != run.StatusRunning {
		t.Fatalf("new run = task %s status %s, want a running run of t1", r.TaskID, r.Status)
	}

	// The new attempt completes: the plan goes on.
	endFirstRun(t, store, orchSvc, p.ID, run.StatusCompleted, "")
	if got := planState(t, store, p.ID); got.Steps[0].Status != plan.StepStatusCompleted || got.Steps[1].Status != plan.StepStatusRunning {
		t.Fatalf("steps = %s, %s: want the first completed and the second running", got.Steps[0].Status, got.Steps[1].Status)
	}
}

func TestStallReplan_BoundedPerStep(t *testing.T) {
	store, orchSvc := newStallReplanSetup(1)
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}})

	endFirstRun(t, store, orchSvc, p.ID, run.StatusFailed, run.StallDetectedError) // re-planned
	endFirstRun(t, store, orchSvc, p.ID, run.StatusFailed, run.StallDetectedError) // budget used up

	got := planState(t, store, p.ID)
	if got.Steps[0].Status != plan.StepStatusFailed || got.Status != plan.StatusFailed {
		t.Fatalf("step %s, plan %s: want both failed after the second stall", got.Steps[0].Status, got.Status)
	}
}

func TestStallReplan_OnlyStalls(t *testing.T) {
	tests := []struct {
		name      string
		maxReplan int
		status    run.Status
		errMsg    string
	}{
		{"disabled", 0, run.StatusFailed, run.StallDetectedError},
		{"other failure", 1, run.StatusFailed, "tests failed"},
		{"timeout", 1, run.StatusTimeout, "run timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, orchSvc := newStallReplanSetup(tt.maxReplan)
			p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}})
			endFirstRun(t, store, orchSvc, p.ID, tt.status, tt.errMsg)
			if got := planState(t, store, p.ID); got.Steps[0].Status != plan.StepStatusFailed {
				t.Fatalf("step %s, want failed without a re-plan", got.Steps[0].Status)
			}
		})
	}
}

// S6-F 8: a run the worker's agent loop aborted for a stall is re-planned
// like one the Go Core stopped.
func TestStallReplan_WorkerStallGetsANewRun(t *testing.T) {
	store, orchSvc := newStallReplanSetup(1)
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}})

	stalled := endFirstRun(t, store, orchSvc, p.ID, run.StatusFailed, "stall detected: repeated read_file after 2 escape attempts")

	got := planState(t, store, p.ID)
	if step := got.Steps[0]; got.Status != plan.StatusRunning || step.Status != plan.StepStatusRunning || step.RunID == stalled {
		t.Fatalf("plan %s, step %s with run %q: want the step running a new run", got.Status, step.Status, step.RunID)
	}
}
