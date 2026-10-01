package service_test

import (
	"context"
	"fmt"
	"slices"
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

// S6-F 3: in a ping_pong plan (a debate) the stalled step runs again for the
// same round; the stall does not count as a round or hand the turn over.
func TestStallReplan_PingPongRerunsTheSameRound(t *testing.T) {
	store, orchSvc := newStallReplanSetup(1)
	ctx := context.Background()
	p := createPlan(t, orchSvc, plan.ProtocolPingPong, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"},
	})
	endStep := func(i int, status run.Status, errMsg string) string {
		t.Helper()
		id := planState(t, store, p.ID).Steps[i].RunID
		if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: id, Status: status, Error: errMsg}); err != nil {
			t.Fatalf("CompleteRun: %v", err)
		}
		orchSvc.HandleRunCompleted(ctx, id, status)
		return id
	}

	stalled := endStep(0, run.StatusFailed, run.StallDetectedError)

	got := planState(t, store, p.ID)
	s0, s1 := got.Steps[0], got.Steps[1]
	if got.Status != plan.StatusRunning || s0.Status != plan.StepStatusRunning || s0.RunID == stalled || s0.Round != 1 {
		t.Fatalf("plan %s, step 0 %s round %d run %q: want step 0 running round 1 again with a new run", got.Status, s0.Status, s0.Round, s0.RunID)
	}
	if s1.Status != plan.StepStatusPending || s1.Round != 0 {
		t.Fatalf("step 1 %s round %d: want it pending before its first round", s1.Status, s1.Round)
	}

	// The rounds go on alternating from there.
	endStep(0, run.StatusCompleted, "")
	if got := planState(t, store, p.ID); got.Steps[1].Status != plan.StepStatusRunning || got.Steps[1].Round != 1 {
		t.Fatalf("step 1 %s round %d: want it running round 1", got.Steps[1].Status, got.Steps[1].Round)
	}
}

// Review finding 1: a stall in any round re-runs that round of the stalled
// step - round 2 and 3 included - and the rounds then alternate as before:
// each step runs every round once, and the plan completes.
func TestStallReplan_PingPongStallInLaterRounds(t *testing.T) {
	for _, stallRound := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("round %d", stallRound), func(t *testing.T) {
			store, orchSvc := newStallReplanSetup(1)
			ctx := context.Background()
			p := createPlan(t, orchSvc, plan.ProtocolPingPong, 0, []plan.CreateStepRequest{
				{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"},
			})

			// Each completed round as "step:round"; step 0 stalls once in stallRound.
			var rounds []string
			stalled := false
			for range 10 {
				got := planState(t, store, p.ID)
				if got.Status != plan.StatusRunning {
					break
				}
				i := -1
				for j := range got.Steps {
					if got.Steps[j].Status == plan.StepStatusRunning {
						i = j
					}
				}
				if i < 0 {
					t.Fatalf("no running step in a running plan (steps %s / %s)", got.Steps[0].Status, got.Steps[1].Status)
				}
				step := got.Steps[i]
				status, errMsg := run.StatusCompleted, ""
				if i == 0 && step.Round == stallRound && !stalled {
					status, errMsg, stalled = run.StatusFailed, run.StallDetectedError, true
				} else {
					rounds = append(rounds, fmt.Sprintf("%d:%d", i, step.Round))
				}
				if err := store.CompleteRun(ctx, &run.CompletionRequest{ID: step.RunID, Status: status, Error: errMsg}); err != nil {
					t.Fatalf("CompleteRun: %v", err)
				}
				orchSvc.HandleRunCompleted(ctx, step.RunID, status)
			}

			want := []string{"0:1", "1:1", "0:2", "1:2", "0:3", "1:3"}
			if !stalled || !slices.Equal(rounds, want) {
				t.Fatalf("rounds = %v (stalled %v), want %v", rounds, stalled, want)
			}
			if got := planState(t, store, p.ID); got.Status != plan.StatusCompleted {
				t.Fatalf("plan = %s, want completed", got.Status)
			}
		})
	}
}
