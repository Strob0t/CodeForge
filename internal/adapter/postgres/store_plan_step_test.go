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
// step, only a running step is re-planned, the run ID stays, and another
// tenant's step is not touched (KI-94).
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
	running := func(stepID string) {
		t.Helper()
		if err := f.store.UpdatePlanStepStatus(f.ctx, stepID, plan.StepStatusRunning, runID, "stall detected: x"); err != nil {
			t.Fatalf("UpdatePlanStepStatus: %v", err)
		}
	}
	replan := func(ctx context.Context, stepID string, want bool) {
		t.Helper()
		got, err := f.store.ReplanStalledStep(ctx, stepID, 1)
		if err != nil || got != want {
			t.Fatalf("ReplanStalledStep(%s) = %v, %v; want %v", stepID, got, err, want)
		}
	}

	replan(f.ctx, first, false) // pending, not running
	running(first)
	replan(ctxWithTenant(t, createTestTenant(t, f.store)), first, false) // another tenant
	replan(f.ctx, first, true)
	steps, err := f.store.ListPlanSteps(f.ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range steps {
		if steps[i].ID == first && (steps[i].Status != plan.StepStatusPending || steps[i].RunID != runID || steps[i].Error != "") {
			t.Fatalf("re-planned step = %+v, want pending with its run and no error", steps[i])
		}
	}
	running(first)
	replan(f.ctx, first, false) // its budget is used up
	running(second)
	replan(f.ctx, second, true) // the other step has its own
}
