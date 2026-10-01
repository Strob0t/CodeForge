package service_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/service"
)

// S6-F 2: a step the preparer needs (the review pipeline's refactoring
// step) is prepared before its run starts - the workspace baseline is taken
// then - in the background, without the scheduling lock; a failed
// preparation fails the step.

type testPreparer struct {
	orch     *service.OrchestratorService
	other    string // a pending plan whose start takes the lock
	err      error
	calls    atomic.Int32
	lockFree atomic.Bool
}

func (p *testPreparer) NeedsPreparation(step *plan.Step) bool { return step.ModeID == "refactorer" }

func (p *testPreparer) PrepareStep(context.Context, *plan.Step) error {
	p.calls.Add(1)
	done := make(chan struct{})
	go func() {
		_, _ = p.orch.StartPlan(context.Background(), p.other)
		close(done)
	}()
	select {
	case <-done:
		p.lockFree.Store(true)
	case <-time.After(time.Second):
	}
	return p.err
}

// waitStep polls until step i of the plan has the status.
func waitStep(t *testing.T, store *orchMockStore, planID string, i int, want plan.StepStatus) plan.Step {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		step := planState(t, store, planID).Steps[i]
		if step.Status == want {
			return step
		}
		if time.Now().After(deadline) {
			t.Fatalf("step %d = %s, want %s", i, step.Status, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStepPreparer_RefactoringStepStartsOncePrepared(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	prep := &testPreparer{orch: orchSvc, other: pendingPlan(t, orchSvc).ID}
	orchSvc.SetStepPreparer(prep)
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1", ModeID: "reviewer"},
		{TaskID: "t2", AgentID: "a2", ModeID: "refactorer", DependsOn: []string{"0"}},
	})
	if prep.calls.Load() != 0 {
		t.Fatal("a reviewer step was prepared")
	}

	id := planState(t, store, p.ID).Steps[0].RunID
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusCompleted}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	orchSvc.HandleRunCompleted(ctx, id, run.StatusCompleted)

	step := waitStep(t, store, p.ID, 1, plan.StepStatusRunning)
	if step.RunID == "" || prep.calls.Load() != 1 {
		t.Fatalf("refactorer step run %q after %d preparations, want one preparation and a run", step.RunID, prep.calls.Load())
	}
	if !prep.lockFree.Load() {
		t.Fatal("the step was prepared under the scheduling lock")
	}
}

func TestStepPreparer_FailedPreparationFailsTheStep(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	prep := &testPreparer{orch: orchSvc, other: pendingPlan(t, orchSvc).ID, err: errors.New("no baseline")}
	orchSvc.SetStepPreparer(prep)
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1", ModeID: "refactorer"}})

	step := waitStep(t, store, p.ID, 0, plan.StepStatusFailed)
	if step.RunID != "" {
		t.Fatalf("step ran %q although its preparation failed", step.RunID)
	}
	deadline := time.Now().Add(5 * time.Second)
	for planState(t, store, p.ID).Status != plan.StatusFailed {
		if time.Now().After(deadline) {
			t.Fatalf("plan = %s, want failed", planState(t, store, p.ID).Status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A plan cancelled while its step is prepared starts nothing.
func TestStepPreparer_CancelledWhilePreparing(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	release := make(chan struct{})
	blocking := &blockingPreparer{release: release, entered: make(chan struct{})}
	orchSvc.SetStepPreparer(blocking)
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1", ModeID: "refactorer"}})
	<-blocking.entered
	if err := orchSvc.CancelPlan(context.Background(), p.ID); err != nil {
		t.Fatalf("CancelPlan: %v", err)
	}
	close(release)
	time.Sleep(50 * time.Millisecond)
	if step := planState(t, store, p.ID).Steps[0]; step.RunID != "" || step.Status != plan.StepStatusSkipped {
		t.Fatalf("step %s with run %q, want it skipped without a run", step.Status, step.RunID)
	}
}

type blockingPreparer struct {
	release, entered chan struct{}
}

func (b *blockingPreparer) NeedsPreparation(*plan.Step) bool { return true }

func (b *blockingPreparer) PrepareStep(context.Context, *plan.Step) error {
	close(b.entered)
	<-b.release
	return nil
}

// Review finding 12: a preparation outcome stored for a step that does not
// start then (another step waits for approval) is dropped when the plan
// ends, like the review decisions.
func TestStepPreparer_OutcomeOfAPlanThatEndedIsDropped(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	release := make(chan struct{})
	prep := &refactorerPreparer{blockingPreparer{release: release, entered: make(chan struct{})}}
	orchSvc.SetStepPreparer(prep)
	orchSvc.SetStepGate(func(_ context.Context, step *plan.Step) (plan.StepStatus, func(context.Context)) {
		return plan.StepStatusWaitingApproval, nil
	})
	p := createPlan(t, orchSvc, plan.ProtocolParallel, 2, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1", ModeID: "reviewer"},
		{TaskID: "t2", AgentID: "a2", ModeID: "refactorer"},
	})
	<-prep.entered
	var id string
	for _, st := range planState(t, store, p.ID).Steps {
		if st.ModeID == "reviewer" {
			id = st.RunID
		}
	}
	if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: run.StatusCompleted}); err != nil {
		t.Fatalf("CompleteRun %q: %v", id, err)
	}
	orchSvc.HandleRunCompleted(ctx, id, run.StatusCompleted) // the reviewer step waits for approval
	close(release)

	deadline := time.Now().Add(5 * time.Second)
	for orchSvc.PreparedOutcomeCount() != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("prepared outcomes = %d, want the refactorer's kept while the plan waits", orchSvc.PreparedOutcomeCount())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := orchSvc.CancelPlan(ctx, p.ID); err != nil {
		t.Fatalf("CancelPlan: %v", err)
	}
	if n := orchSvc.PreparedOutcomeCount(); n != 0 {
		t.Fatalf("prepared outcomes after the plan ended = %d, want 0", n)
	}
}

// refactorerPreparer blocks the preparation of refactorer steps only.
type refactorerPreparer struct{ blockingPreparer }

func (r *refactorerPreparer) NeedsPreparation(step *plan.Step) bool {
	return step.ModeID == "refactorer"
}
