package service_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/service"
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

// --- S6-F 12: the gate and the plan-end callbacks run without the lock ---

// lockFree reports whether the orchestrator's scheduling lock can be taken
// within a second: StartPlan of another, pending plan takes it.
func lockFree(t *testing.T, orchSvc *service.OrchestratorService, otherPlanID string) bool {
	t.Helper()
	done := make(chan struct{})
	go func() {
		_, _ = orchSvc.StartPlan(context.Background(), otherPlanID)
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(time.Second):
		return false
	}
}

func pendingPlan(t *testing.T, orchSvc *service.OrchestratorService) *plan.ExecutionPlan {
	t.Helper()
	p, err := orchSvc.CreatePlan(context.Background(), &plan.CreatePlanRequest{
		Name: "other", ProjectID: "proj-1", Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{{TaskID: "t9", AgentID: "a9"}},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	return p
}

func TestStepGate_RunsWithoutTheSchedulingLock(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	other := pendingPlan(t, orchSvc)
	free := false
	orchSvc.SetStepGate(func(context.Context, *plan.Step) plan.StepStatus {
		free = lockFree(t, orchSvc, other.ID)
		return plan.StepStatusCompleted
	})
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}})
	id := planState(t, store, p.ID).Steps[0].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusCompleted}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	orchSvc.HandleRunCompleted(ctx, id, run.StatusCompleted)

	if !free {
		t.Fatal("the step gate ran under the scheduling lock")
	}
	if got := planState(t, store, p.ID); got.Status != plan.StatusCompleted {
		t.Fatalf("plan = %s, want completed", got.Status)
	}
}

func TestPlanEndCallbacks_RunWithoutTheSchedulingLock(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	other := pendingPlan(t, orchSvc)
	var ended []string
	free := false
	orchSvc.AddOnPlanComplete(func(_ context.Context, planID, status string) {
		if planID == other.ID {
			return
		}
		ended = append(ended, status)
		free = lockFree(t, orchSvc, other.ID)
	})
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}})
	id := planState(t, store, p.ID).Steps[0].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusFailed}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	orchSvc.HandleRunCompleted(ctx, id, run.StatusFailed)

	if len(ended) != 1 || ended[0] != string(plan.StatusFailed) {
		t.Fatalf("plan-end callbacks = %v, want one for the failed plan", ended)
	}
	if !free {
		t.Fatal("the plan-end callback ran under the scheduling lock")
	}
}

// A completion delivered again while its step is being gated is skipped: the
// gate runs once and the plan goes on once.
func TestStepGate_DuplicateCompletionWhileGating(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	orchSvc.SetStepGate(func(_ context.Context, step *plan.Step) plan.StepStatus {
		if step.TaskID == "t1" && calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return plan.StepStatusCompleted
	})
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
	})
	id := planState(t, store, p.ID).Steps[0].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusCompleted}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	done := make(chan struct{})
	go func() {
		orchSvc.HandleRunCompleted(ctx, id, run.StatusCompleted)
		close(done)
	}()
	<-entered
	orchSvc.HandleRunCompleted(ctx, id, run.StatusCompleted) // redelivered: returns at once
	close(release)
	<-done

	got := planState(t, store, p.ID)
	if calls.Load() != 1 || got.Steps[0].Status != plan.StepStatusCompleted || got.Steps[1].Status != plan.StepStatusRunning {
		t.Fatalf("gate calls %d, steps %s / %s: want one gate call, the first completed and the second running",
			calls.Load(), got.Steps[0].Status, got.Steps[1].Status)
	}
}

// A plan cancelled while its step is being gated stays cancelled: the gate's
// answer is not applied and no further step starts.
func TestStepGate_PlanCancelledWhileGating(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	orchSvc.SetStepGate(func(_ context.Context, step *plan.Step) plan.StepStatus {
		if step.TaskID == "t1" {
			close(entered)
			<-release
		}
		return plan.StepStatusCompleted
	})
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
	})
	id := planState(t, store, p.ID).Steps[0].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusCompleted}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	done := make(chan struct{})
	go func() {
		orchSvc.HandleRunCompleted(ctx, id, run.StatusCompleted)
		close(done)
	}()
	<-entered
	if err := orchSvc.CancelPlan(ctx, p.ID); err != nil {
		t.Fatalf("CancelPlan: %v", err)
	}
	close(release)
	<-done

	got := planState(t, store, p.ID)
	if got.Status != plan.StatusCancelled || got.Steps[0].Status != plan.StepStatusCancelled || got.Steps[1].Status != plan.StepStatusSkipped {
		t.Fatalf("plan %s, steps %s / %s: want the plan cancelled, the gated step cancelled, the next skipped",
			got.Status, got.Steps[0].Status, got.Steps[1].Status)
	}
}
