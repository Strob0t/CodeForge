package postgres_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// TestStore_PlanStepsKeepTheirMode: a plan step's mode is stored, whether the
// step is created with its plan or on its own (KI-76).
func TestStore_PlanStepsKeepTheirMode(t *testing.T) {
	f := newStatusFixture(t)
	p := &plan.ExecutionPlan{
		ProjectID: f.project.ID, Name: "step-mode", Protocol: plan.ProtocolPingPong,
		Status: plan.StatusPending, MaxParallel: 1,
		Steps: []plan.Step{
			{TaskID: f.task.ID, AgentID: f.agent.ID, ModeID: "proponent", Status: plan.StepStatusPending},
		},
	}
	if err := f.store.CreatePlan(f.ctx, p); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	added := &plan.Step{PlanID: p.ID, TaskID: f.task.ID, AgentID: f.agent.ID, ModeID: "moderator", Status: plan.StepStatusPending, DependsOn: []string{}}
	if err := f.store.CreatePlanStep(f.ctx, added); err != nil {
		t.Fatalf("CreatePlanStep: %v", err)
	}

	steps, err := f.store.ListPlanSteps(f.ctx, p.ID)
	if err != nil {
		t.Fatalf("ListPlanSteps: %v", err)
	}
	modes := map[string]string{}
	for i := range steps {
		modes[steps[i].ID] = steps[i].ModeID
	}
	if modes[p.Steps[0].ID] != "proponent" || modes[added.ID] != "moderator" {
		t.Fatalf("stored step modes = %v, want proponent and moderator", modes)
	}
}

// TestStore_ReplanStalledStepCountsPerStep: stall re-plans are counted per
// step, only a step running the stalled run is re-planned, the run ID stays,
// and another tenant's step is not touched (KI-94). A step that no longer
// runs the stalled run (another replica re-planned or ended it) is reported
// as moved, not as out of re-plans (S7-F review).
func TestStore_ReplanStalledStepCountsPerStep(t *testing.T) {
	f := newStatusFixture(t)
	p := &plan.ExecutionPlan{
		ProjectID: f.project.ID, Name: "stall-budget", Protocol: plan.ProtocolPingPong,
		Status: plan.StatusRunning, MaxParallel: 1,
		Steps: []plan.Step{
			{TaskID: f.task.ID, AgentID: f.agent.ID, ModeID: "proponent", Status: plan.StepStatusPending},
			{TaskID: f.task.ID, AgentID: f.agent.ID, ModeID: "moderator", Status: plan.StepStatusPending},
		},
	}
	if err := f.store.CreatePlan(f.ctx, p); err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	first, second := p.Steps[0].ID, p.Steps[1].ID
	runID := f.newRun(t, run.StatusFailed).ID
	otherRun := f.newRun(t, run.StatusRunning).ID
	runningRun := func(stepID, id string) {
		t.Helper()
		if err := f.store.UpdatePlanStepStatus(f.ctx, stepID, plan.StepStatusRunning, id, "stall detected: x"); err != nil {
			t.Fatalf("UpdatePlanStepStatus: %v", err)
		}
	}
	running := func(stepID string) { t.Helper(); runningRun(stepID, runID) }
	replan := func(ctx context.Context, stepID string, want plan.ReplanOutcome) {
		t.Helper()
		got, err := f.store.ReplanStalledStep(ctx, stepID, runID, 1)
		if err != nil || got != want {
			t.Fatalf("ReplanStalledStep(%s) = %v, %v; want %v", stepID, got, err, want)
		}
	}
	stepOf := func(stepID string) plan.Step {
		t.Helper()
		steps, err := f.store.ListPlanSteps(f.ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		for i := range steps {
			if steps[i].ID == stepID {
				return steps[i]
			}
		}
		t.Fatalf("step %s not found", stepID)
		return plan.Step{}
	}

	replan(f.ctx, first, plan.ReplanStepMoved) // pending, not running
	running(first)
	replan(ctxWithTenant(t, createTestTenant(t, f.store)), first, plan.ReplanStepMoved) // another tenant
	replan(f.ctx, first, plan.Replanned)
	if st := stepOf(first); st.Status != plan.StepStatusPending || st.RunID != runID || st.Error != "" {
		t.Fatalf("re-planned step = %+v, want pending with its run and no error", st)
	}
	// A duplicated completion of the stalled run on another replica: the
	// step is pending again, then runs a new run; neither is re-planned or
	// counted again.
	replan(f.ctx, first, plan.ReplanStepMoved)
	runningRun(first, otherRun)
	replan(f.ctx, first, plan.ReplanStepMoved)
	if st := stepOf(first); st.Status != plan.StepStatusRunning || st.RunID != otherRun {
		t.Fatalf("step = %+v, want running the other run", st)
	}
	running(first)
	replan(f.ctx, first, plan.ReplanBudgetUsedUp) // its budget is used up
	running(second)
	replan(f.ctx, second, plan.Replanned) // the other step has its own
}
