package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A plan step follows its run on every path that ends the run, not only on
// the worker's completion message (KI-30).

// startSingleStepPlan creates and starts a sequential plan with one step and
// returns the plan and the ID of the step's run.
func startSingleStepPlan(t *testing.T, store *orchMockStore, orchSvc *service.OrchestratorService) (p *plan.ExecutionPlan, runID string) {
	t.Helper()
	ctx := context.Background()
	p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
		Name:      "single step",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolSequential,
		Steps:     []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("StartPlan: %v", err)
	}
	steps, _ := store.ListPlanSteps(ctx, p.ID)
	if len(steps) != 1 || steps[0].RunID == "" {
		t.Fatalf("expected one started step, got %+v", steps)
	}
	return p, steps[0].RunID
}

func TestPlanStep_FollowsRunEndedByTheRuntime(t *testing.T) {
	ctx := context.Background()
	toolResult := func(rt *service.RuntimeService, runID, callID string, cost float64) error {
		return rt.HandleToolCallResult(ctx, &messagequeue.ToolCallResultPayload{
			RunID: runID, CallID: callID, Tool: "Read", Success: true, Output: "same output", CostUSD: cost,
		})
	}

	tests := []struct {
		name      string
		stop      func(t *testing.T, store *orchMockStore, rt *service.RuntimeService, runID string) error
		wantStep  plan.StepStatus
		wantPlan  plan.Status // "" = not checked
		wantRunAs run.Status
	}{
		{
			name: "termination limit",
			stop: func(_ *testing.T, store *orchMockStore, rt *service.RuntimeService, runID string) error {
				store.runtimeMockStore.mu.Lock()
				for i := range store.runs {
					if store.runs[i].ID == runID {
						store.runs[i].StepCount = 50 // headless-safe-sandbox: MaxSteps 50
					}
				}
				store.runtimeMockStore.mu.Unlock()
				return rt.HandleToolCallRequest(ctx, &messagequeue.ToolCallRequestPayload{
					RunID: runID, CallID: "call-over-limit", Tool: "Read", Path: "main.go",
				})
			},
			wantStep:  plan.StepStatusFailed,
			wantPlan:  plan.StatusFailed,
			wantRunAs: run.StatusTimeout,
		},
		{
			name: "post-execution budget",
			stop: func(_ *testing.T, _ *orchMockStore, rt *service.RuntimeService, runID string) error {
				return toolResult(rt, runID, "call-over-budget", 6.0) // headless-safe-sandbox: MaxCost 5.0
			},
			wantStep:  plan.StepStatusFailed,
			wantPlan:  plan.StatusFailed,
			wantRunAs: run.StatusTimeout,
		},
		{
			name: "stall detection",
			stop: func(_ *testing.T, _ *orchMockStore, rt *service.RuntimeService, runID string) error {
				for i := range 5 { // StallThreshold 5
					if err := toolResult(rt, runID, fmt.Sprintf("call-stall-%d", i), 0); err != nil {
						return err
					}
				}
				return nil
			},
			wantStep:  plan.StepStatusFailed,
			wantPlan:  plan.StatusFailed,
			wantRunAs: run.StatusFailed,
		},
		{
			name: "user cancel",
			stop: func(_ *testing.T, _ *orchMockStore, rt *service.RuntimeService, runID string) error {
				return rt.CancelRun(ctx, runID)
			},
			wantStep:  plan.StepStatusCancelled,
			wantPlan:  plan.StatusFailed, // a cancelled step fails the plan (review finding 3)
			wantRunAs: run.StatusCancelled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, orchSvc, rt := newOrchRuntimeSetup()
			p, runID := startSingleStepPlan(t, store, orchSvc)

			if err := tc.stop(t, store, rt, runID); err != nil {
				t.Fatalf("stop: %v", err)
			}

			if r, _ := store.GetRun(ctx, runID); r.Status != tc.wantRunAs {
				t.Fatalf("run status = %q, want %q", r.Status, tc.wantRunAs)
			}
			steps, _ := store.ListPlanSteps(ctx, p.ID)
			if got := steps[0].Status; got != tc.wantStep {
				t.Errorf("step status = %q, want %q", got, tc.wantStep)
			}
			if tc.wantPlan != "" {
				got, _ := store.GetPlan(ctx, p.ID)
				if got.Status != tc.wantPlan {
					t.Errorf("plan status = %q, want %q", got.Status, tc.wantPlan)
				}
			}
		})
	}
}

// TestCancelPlan_StartsNoFurtherSteps: cancelling a plan cancels its running
// runs; their completion must not start the plan's remaining steps.
func TestCancelPlan_StartsNoFurtherSteps(t *testing.T) {
	tests := []struct {
		name        string
		protocol    plan.Protocol
		maxParallel int
	}{
		{name: "sequential", protocol: plan.ProtocolSequential},
		{name: "parallel", protocol: plan.ProtocolParallel, maxParallel: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, orchSvc, _ := newOrchRuntimeSetup()
			ctx := context.Background()
			p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
				Name:        "cancel " + tc.name,
				ProjectID:   "proj-1",
				Protocol:    tc.protocol,
				MaxParallel: tc.maxParallel,
				Steps: []plan.CreateStepRequest{
					{TaskID: "t1", AgentID: "a1"},
					{TaskID: "t2", AgentID: "a2"},
				},
			})
			if err != nil {
				t.Fatalf("CreatePlan: %v", err)
			}
			if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
				t.Fatalf("StartPlan: %v", err)
			}

			if err := orchSvc.CancelPlan(ctx, p.ID); err != nil {
				t.Fatalf("CancelPlan: %v", err)
			}

			got, _ := store.GetPlan(ctx, p.ID)
			if got.Status != plan.StatusCancelled {
				t.Errorf("plan status = %q, want cancelled", got.Status)
			}
			if got.Steps[0].Status != plan.StepStatusCancelled {
				t.Errorf("first step status = %q, want cancelled", got.Steps[0].Status)
			}
			if got.Steps[1].Status != plan.StepStatusSkipped || got.Steps[1].RunID != "" {
				t.Errorf("second step = %q with run %q, want skipped without a run", got.Steps[1].Status, got.Steps[1].RunID)
			}
			store.runtimeMockStore.mu.Lock()
			runs := len(store.runs)
			store.runtimeMockStore.mu.Unlock()
			if runs != 1 {
				t.Errorf("runs started = %d, want 1", runs)
			}
		})
	}
}
