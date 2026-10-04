package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// TestReplanStep_StartsANewRunForTheStep: re-planning gives the step of an
// ended run a new run; the ended run is never reopened (review finding 4).
func TestReplanStep_StartsANewRunForTheStep(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	p := createPlan(t, orchSvc, plan.ProtocolParallel, 2, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"},
	})

	// The first step's run stalls and fails; the second still runs.
	failed := planState(t, store, p.ID).Steps[0].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: failed, Status: run.StatusFailed, Error: "stall detected"}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	orchSvc.HandleRunCompleted(ctx, failed, run.StatusFailed)
	if got := planState(t, store, p.ID); got.Status != plan.StatusRunning {
		t.Fatalf("plan = %s, want running while the second step runs", got.Status)
	}

	if err := orchSvc.ReplanStep(ctx, failed); err != nil {
		t.Fatalf("ReplanStep: %v", err)
	}

	step := planState(t, store, p.ID).Steps[0]
	if step.Status != plan.StepStatusRunning || step.RunID == "" || step.RunID == failed {
		t.Fatalf("step = %s with run %q, want running with a new run", step.Status, step.RunID)
	}
	if r, _ := store.GetRun(ctx, failed); r.Status != run.StatusFailed {
		t.Errorf("ended run = %s, want it to stay failed", r.Status)
	}
	if r, _ := store.GetRun(ctx, step.RunID); r.Status != run.StatusRunning || r.TaskID != "t1" {
		t.Errorf("new run = %s for task %s, want running for t1", r.Status, r.TaskID)
	}

	// Re-planning the same ended run again does nothing: its step moved on.
	if err := orchSvc.ReplanStep(ctx, failed); err == nil {
		t.Error("second ReplanStep of the same run: want an error")
	}

	// The new attempt and the other step complete: the plan completes.
	for _, i := range []int{0, 1} {
		id := planState(t, store, p.ID).Steps[i].RunID
		if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusCompleted}); err != nil {
			t.Fatalf("CompleteRun: %v", err)
		}
		orchSvc.HandleRunCompleted(ctx, id, run.StatusCompleted)
	}
	if got := planState(t, store, p.ID); got.Status != plan.StatusCompleted {
		t.Errorf("plan = %s, want completed", got.Status)
	}
}

// TestReplanStep_EndedPlanIsNotReplanned: a step of a plan that already ended
// gets no new run.
func TestReplanStep_EndedPlanIsNotReplanned(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}})
	failed := planState(t, store, p.ID).Steps[0].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: failed, Status: run.StatusFailed}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	orchSvc.HandleRunCompleted(ctx, failed, run.StatusFailed) // the plan fails

	if err := orchSvc.ReplanStep(ctx, failed); err == nil {
		t.Fatal("ReplanStep in a failed plan: want an error")
	}
	store.runtimeMockStore.mu.Lock()
	runs := len(store.runs)
	store.runtimeMockStore.mu.Unlock()
	if runs != 1 {
		t.Errorf("runs = %d, want no new run", runs)
	}
}
