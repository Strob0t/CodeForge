package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// S6-F 5: a cancelled plan ends like a failed or completed one: the plan-end
// callbacks run (team cleanup, the review pipeline's keep/undo), once, after
// the scheduling lock is released.
func TestCancelPlan_RunsThePlanEndCallbacks(t *testing.T) {
	store, orchSvc, _ := newOrchRuntimeSetup()
	ctx := context.Background()
	other := pendingPlan(t, orchSvc)
	type end struct{ planID, status string }
	var ends []end
	free := false
	orchSvc.AddOnPlanComplete(func(_ context.Context, planID, status string) {
		if planID == other.ID {
			return
		}
		ends = append(ends, end{planID, status})
		free = lockFree(t, orchSvc, other.ID)
	})
	p := createPlan(t, orchSvc, plan.ProtocolSequential, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2", DependsOn: []string{"0"}},
	})

	if err := orchSvc.CancelPlan(ctx, p.ID); err != nil {
		t.Fatalf("CancelPlan: %v", err)
	}
	if len(ends) != 1 || ends[0] != (end{p.ID, string(plan.StatusCancelled)}) {
		t.Fatalf("plan-end callbacks = %+v, want one for the cancelled plan", ends)
	}
	if !free {
		t.Fatal("the plan-end callback ran under the scheduling lock")
	}
	got := planState(t, store, p.ID)
	if got.Steps[0].Status != plan.StepStatusCancelled || got.Steps[1].Status != plan.StepStatusSkipped {
		t.Fatalf("steps = %s / %s when the callback ran, want cancelled / skipped", got.Steps[0].Status, got.Steps[1].Status)
	}

	// A plan that already ended is not cancelled, and does not end again.
	if err := orchSvc.CancelPlan(ctx, p.ID); err == nil {
		t.Fatal("CancelPlan of a cancelled plan succeeded")
	}
	if len(ends) != 1 {
		t.Fatalf("plan-end callbacks = %+v after a second cancel, want still one", ends)
	}
}
