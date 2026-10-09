package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// KI-94: an agent works on one plan at a time. The review pipeline checked
// it when it picked its agent (S6-F 7), but any other plan could start with
// an agent a running plan - a review pipeline's included - still used.

func TestStartPlan_AgentOfARunningPlanIsRefused(t *testing.T) {
	ctx := context.Background()
	_, orchSvc, _ := newOrchRuntimeSetup()
	create := func(name string, agents ...string) *plan.ExecutionPlan {
		t.Helper()
		var steps []plan.CreateStepRequest
		for _, a := range agents {
			steps = append(steps, plan.CreateStepRequest{TaskID: "t1", AgentID: a})
		}
		p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{Name: name, ProjectID: "proj-1", Protocol: plan.ProtocolParallel, Steps: steps})
		if err != nil {
			t.Fatalf("CreatePlan: %v", err)
		}
		return p
	}

	running := create("running", "a1")
	pendingToo := create("pending", "a2") // pending plans do not hold their agents
	if _, err := orchSvc.StartPlan(ctx, running.ID); err != nil {
		t.Fatalf("StartPlan: %v", err)
	}

	for _, agents := range [][]string{{"a1"}, {"a3", "a1"}} {
		p := create("shares", agents...)
		if _, err := orchSvc.StartPlan(ctx, p.ID); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("StartPlan with agents %v = %v, want a conflict (a1 runs plan %s)", agents, err, running.ID)
		}
		if got, _ := orchSvc.GetPlan(ctx, p.ID); got.Status != plan.StatusPending {
			t.Fatalf("refused plan is %s, want pending", got.Status)
		}
	}
	if _, err := orchSvc.StartPlan(ctx, pendingToo.ID); err != nil {
		t.Fatalf("StartPlan with an agent no running plan uses: %v", err)
	}

	// Once the running plan ended, its agent is free again.
	if err := orchSvc.CancelPlan(ctx, running.ID); err != nil {
		t.Fatalf("CancelPlan: %v", err)
	}
	later := create("later", "a1")
	if _, err := orchSvc.StartPlan(ctx, later.ID); err != nil {
		t.Fatalf("StartPlan after the plan ended: %v", err)
	}
}
