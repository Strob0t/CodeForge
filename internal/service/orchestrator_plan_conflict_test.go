package service

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// TestPlanEndedElsewhere_NotEndedAgain: a plan that already ended (e.g.
// cancelled while its last run completed) is neither completed nor failed
// again; its steps and the plan-complete callbacks are left alone (KI-31).
func TestPlanEndedElsewhere_NotEndedAgain(t *testing.T) {
	tests := []struct {
		name  string
		steps []plan.Step
		end   func(svc *OrchestratorService, ctx context.Context, p *plan.ExecutionPlan)
	}{
		{
			name:  "complete",
			steps: []plan.Step{{ID: "s1", Status: plan.StepStatusCompleted}, {ID: "s2", Status: plan.StepStatusCompleted}},
			end:   (*OrchestratorService).completePlan,
		},
		{
			name:  "fail",
			steps: []plan.Step{{ID: "s1", Status: plan.StepStatusFailed}, {ID: "s2", Status: plan.StepStatusPending}},
			end:   (*OrchestratorService).failPlan,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newOrchTestStore()
			store.planStatuses["plan-ended"] = plan.StatusCancelled
			svc := newTestOrchService(store)
			callbacks := 0
			svc.AddOnPlanComplete(func(context.Context, string, string) { callbacks++ })

			// The caller still sees the plan as running.
			p := &plan.ExecutionPlan{ID: "plan-ended", ProjectID: "proj-1", Protocol: plan.ProtocolSequential, Status: plan.StatusRunning, Steps: tc.steps}
			tc.end(svc, context.Background(), p)

			if got := store.planStatuses["plan-ended"]; got != plan.StatusCancelled {
				t.Errorf("plan status = %s, want cancelled", got)
			}
			if p.Status != plan.StatusRunning {
				t.Errorf("in-memory plan status = %s, want it unchanged", p.Status)
			}
			if len(store.stepStatuses) != 0 {
				t.Errorf("steps changed: %v", store.stepStatuses)
			}
			if callbacks != 0 {
				t.Errorf("plan-complete callbacks = %d, want 0", callbacks)
			}
		})
	}
}
