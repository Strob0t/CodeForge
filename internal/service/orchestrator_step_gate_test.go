package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// KI-17: the step gate decides the status of a step whose run completed; the
// review pipeline holds a high-impact refactoring for approval with it.
func TestStepGate_HoldsAStepForApproval(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	var gated []string
	orchSvc.SetStepGate(func(_ context.Context, step *plan.Step) plan.StepStatus {
		gated = append(gated, step.ModeID)
		if step.ModeID == "refactorer" {
			return plan.StepStatusWaitingApproval
		}
		return plan.StepStatusCompleted
	})
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1", ModeID: "reviewer"},
		{TaskID: "t2", AgentID: "a2", ModeID: "refactorer", DependsOn: []string{"0"}},
	})

	complete := func(i int, status run.Status) {
		t.Helper()
		id := planState(t, store, p.ID).Steps[i].RunID
		if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: status}); err != nil {
			t.Fatalf("CompleteRun: %v", err)
		}
		orchSvc.HandleRunCompleted(ctx, id, status)
	}

	complete(0, run.StatusCompleted)
	if st := planState(t, store, p.ID).Steps[0].Status; st != plan.StepStatusCompleted {
		t.Fatalf("reviewer step = %s, want completed through the gate", st)
	}
	complete(1, run.StatusCompleted)
	got := planState(t, store, p.ID)
	if got.Steps[1].Status != plan.StepStatusWaitingApproval || got.Status != plan.StatusRunning {
		t.Fatalf("refactorer step = %s, plan = %s: want the step waiting for approval, the plan running", got.Steps[1].Status, got.Status)
	}
	if len(gated) != 2 || gated[0] != "reviewer" || gated[1] != "refactorer" {
		t.Fatalf("gated steps = %v, want reviewer, refactorer", gated)
	}

	if err := orchSvc.ApproveStep(ctx, p.ID, got.Steps[1].ID); err != nil {
		t.Fatalf("ApproveStep: %v", err)
	}
	if got := planState(t, store, p.ID); got.Status != plan.StatusCompleted {
		t.Fatalf("plan = %s after approval, want completed", got.Status)
	}
}

// Failed runs do not pass the gate: the step fails as before.
func TestStepGate_OnlyCompletedRuns(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	calls := 0
	orchSvc.SetStepGate(func(context.Context, *plan.Step) plan.StepStatus {
		calls++
		return plan.StepStatusWaitingApproval
	})
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1", ModeID: "refactorer"}})
	id := planState(t, store, p.ID).Steps[0].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusFailed}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	orchSvc.HandleRunCompleted(ctx, id, run.StatusFailed)
	if got := planState(t, store, p.ID); calls != 0 || got.Steps[0].Status != plan.StepStatusFailed {
		t.Fatalf("gate calls %d, step %s: want no gate and a failed step", calls, got.Steps[0].Status)
	}
}
