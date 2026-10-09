package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// ReplanStep gives only the step that its ended run left failed or cancelled
// another attempt, decided under the scheduling lock; the steps it blocked
// can run again, and the ended run's completion no longer moves the step
// (review 2, findings 2 and 3).

// failStep ends step i's run with status and reports it to the orchestrator.
func failStep(t *testing.T, store *orchMockStore, orchSvc interface {
	HandleRunCompleted(context.Context, string, run.Status)
}, planID string, i int, status run.Status) string {
	t.Helper()
	ctx := context.Background()
	runID := planState(t, store, planID).Steps[i].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: runID, Status: status, Error: "stall detected"}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	orchSvc.HandleRunCompleted(ctx, runID, status)
	return runID
}

// TestReplanStep_BlockedDependentsRunAgain: the dependents skipped because
// the step failed are pending again, run after the new attempt completes,
// and the plan completes.
func TestReplanStep_BlockedDependentsRunAgain(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	p := createPlan(t, orchSvc, plan.ProtocolParallel, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"},
		{TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
		{TaskID: "t3", AgentID: "a3", DependsOn: []string{"1"}},
		{TaskID: "t3", AgentID: "a3"}, // keeps the plan running
	})

	failed := failStep(t, store, orchSvc, p.ID, 0, run.StatusFailed)
	if got := planState(t, store, p.ID); got.Steps[1].Status != plan.StepStatusSkipped || got.Steps[2].Status != plan.StepStatusSkipped {
		t.Fatalf("dependents = %s, %s, want skipped", got.Steps[1].Status, got.Steps[2].Status)
	}

	if err := orchSvc.ReplanStep(ctx, failed); err != nil {
		t.Fatalf("ReplanStep: %v", err)
	}
	got := planState(t, store, p.ID)
	if got.Steps[0].Status != plan.StepStatusRunning || got.Steps[1].Status != plan.StepStatusPending || got.Steps[2].Status != plan.StepStatusPending {
		t.Fatalf("steps = %s, %s, %s, want running, pending, pending", got.Steps[0].Status, got.Steps[1].Status, got.Steps[2].Status)
	}
	if got.Steps[1].Error != "" || got.Steps[2].Error != "" {
		t.Errorf("reset dependents keep errors %q, %q", got.Steps[1].Error, got.Steps[2].Error)
	}

	for _, i := range []int{0, 1, 2, 3} {
		st := planState(t, store, p.ID).Steps[i]
		if st.Status != plan.StepStatusRunning {
			t.Fatalf("step %d = %s, want running", i, st.Status)
		}
		failStep(t, store, orchSvc, p.ID, i, run.StatusCompleted)
	}
	if got := planState(t, store, p.ID); got.Status != plan.StatusCompleted {
		t.Errorf("plan = %s, want completed", got.Status)
	}
}

// TestReplanStep_OnlyAnUnsuccessfulStepOfItsRun: a completed step, a step
// whose run ended but whose completion was not processed yet, and a step
// whose run already ended successfully are not re-planned.
func TestReplanStep_OnlyAnUnsuccessfulStepOfItsRun(t *testing.T) {
	ctx := context.Background()
	steps := []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"}}

	t.Run("completed run", func(t *testing.T) {
		store, orchSvc, _ := newOrchRuntimeSetup()
		p := createPlan(t, orchSvc, plan.ProtocolParallel, 0, steps)
		done := failStep(t, store, orchSvc, p.ID, 0, run.StatusCompleted)
		if err := orchSvc.ReplanStep(ctx, done); err == nil {
			t.Error("ReplanStep of a completed run: want an error")
		}
		if st := planState(t, store, p.ID).Steps[0]; st.Status != plan.StepStatusCompleted || st.RunID != done {
			t.Errorf("step = %s with run %s, want completed with its run", st.Status, st.RunID)
		}
	})

	t.Run("completion not processed yet", func(t *testing.T) {
		store, orchSvc, _ := newOrchRuntimeSetup()
		p := createPlan(t, orchSvc, plan.ProtocolParallel, 0, steps)
		runID := planState(t, store, p.ID).Steps[0].RunID
		if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: runID, Status: run.StatusFailed}); err != nil {
			t.Fatalf("CompleteRun: %v", err)
		}
		if err := orchSvc.ReplanStep(ctx, runID); err == nil {
			t.Error("ReplanStep of a step still running: want an error")
		}
		if st := planState(t, store, p.ID).Steps[0]; st.Status != plan.StepStatusRunning || st.RunID != runID {
			t.Errorf("step = %s with run %s, want running with its run", st.Status, st.RunID)
		}
	})
}

// TestReplanStep_ConcurrentReplansStartOneRun: replans of the same ended run
// racing each other start one new run.
func TestReplanStep_ConcurrentReplansStartOneRun(t *testing.T) {
	for range 20 {
		store, orchSvc, _ := newOrchRuntimeSetup()
		ctx := context.Background()
		p := createPlan(t, orchSvc, plan.ProtocolParallel, 0, []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"},
		})
		failed := failStep(t, store, orchSvc, p.ID, 0, run.StatusFailed)
		store.runtimeMockStore.mu.Lock()
		before := len(store.runs)
		store.runtimeMockStore.mu.Unlock()

		var wg sync.WaitGroup
		errs := make(chan error, 3)
		for range 3 {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- orchSvc.ReplanStep(ctx, failed) }()
		}
		wg.Wait()
		close(errs)
		ok := 0
		for err := range errs {
			if err == nil {
				ok++
			}
		}
		store.runtimeMockStore.mu.Lock()
		started := len(store.runs) - before
		store.runtimeMockStore.mu.Unlock()
		if ok != 1 || started != 1 {
			t.Fatalf("%d replans succeeded and %d runs started, want 1 and 1", ok, started)
		}
	}
}

// TestHandleRunCompleted_EndedRunOfAReplannedStep: the completion of the
// step's ended run, delivered again after the step was re-planned, does not
// move the step that now runs a new run.
func TestHandleRunCompleted_EndedRunOfAReplannedStep(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	p := createPlan(t, orchSvc, plan.ProtocolParallel, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"},
	})
	failed := failStep(t, store, orchSvc, p.ID, 0, run.StatusFailed)
	if err := orchSvc.ReplanStep(ctx, failed); err != nil {
		t.Fatalf("ReplanStep: %v", err)
	}
	current := planState(t, store, p.ID).Steps[0].RunID

	orchSvc.HandleRunCompleted(ctx, failed, run.StatusFailed)

	st := planState(t, store, p.ID).Steps[0]
	if st.Status != plan.StepStatusRunning || st.RunID != current {
		t.Errorf("step = %s with run %s, want running with the new run %s", st.Status, st.RunID, current)
	}
	if got := planState(t, store, p.ID); got.Status != plan.StatusRunning {
		t.Errorf("plan = %s, want running", got.Status)
	}
}
