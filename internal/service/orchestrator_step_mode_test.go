package service_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// A plan step keeps the mode it was created with: pipeline templates (e.g.
// the review-refactor pipeline, KI-17) choose each step's mode, and the step's
// run starts in it.
func TestCreatePlan_StepsKeepTheirMode(t *testing.T) {
	store, orchSvc := newOrchTestSetup()
	p, err := orchSvc.CreatePlan(context.Background(), &plan.CreatePlanRequest{
		Name: "modes", ProjectID: "proj-1", Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{
			{TaskID: "t1", AgentID: "a1", ModeID: "boundary_analyzer"},
			{TaskID: "t2", AgentID: "a2", ModeID: "refactorer", DependsOn: []string{"0"}},
		},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	stored, err := store.GetPlan(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if stored.Steps[0].ModeID != "boundary_analyzer" || stored.Steps[1].ModeID != "refactorer" {
		t.Fatalf("step modes = %q, %q, want boundary_analyzer, refactorer", stored.Steps[0].ModeID, stored.Steps[1].ModeID)
	}
}
