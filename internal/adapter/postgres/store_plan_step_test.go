package postgres_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
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
