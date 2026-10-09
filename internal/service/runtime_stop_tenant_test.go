package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// The stop paths end runs through the shared completion path (KI-30); every
// event they broadcast stays scoped to the run's tenant (KI-12).
func TestRunStops_ScopeBroadcastsToRunTenant(t *testing.T) {
	tenantCtx := tenantctx.WithTenant(context.Background(), runTenantA)
	toolResult := func(svc *service.RuntimeService, runID, callID string, cost float64) error {
		return svc.HandleToolCallResult(context.Background(), &messagequeue.ToolCallResultPayload{
			RunID: runID, CallID: callID, Tool: "Read", Success: true, Output: "same output", CostUSD: cost, TenantID: runTenantA,
		})
	}

	tests := []struct {
		name       string
		setup      func(store *runtimeMockStore)
		stop       func(svc *service.RuntimeService, runID string) error
		wantStatus run.Status
	}{
		{
			name: "termination limit",
			setup: func(store *runtimeMockStore) {
				store.runs[0].StepCount = 50 // headless-safe-sandbox: MaxSteps 50
			},
			stop: func(svc *service.RuntimeService, runID string) error {
				return svc.HandleToolCallRequest(context.Background(), &messagequeue.ToolCallRequestPayload{
					RunID: runID, CallID: "call-over-limit", Tool: "Read", Path: "main.go", TenantID: runTenantA,
				})
			},
			wantStatus: run.StatusTimeout,
		},
		{
			name: "post-execution budget",
			stop: func(svc *service.RuntimeService, runID string) error {
				return toolResult(svc, runID, "call-over-budget", 6.0) // headless-safe-sandbox: MaxCost 5.0
			},
			wantStatus: run.StatusTimeout,
		},
		{
			name: "stall detection",
			stop: func(svc *service.RuntimeService, runID string) error {
				for i := range 5 { // StallThreshold 5
					if err := toolResult(svc, runID, fmt.Sprintf("call-stall-%d", i), 0); err != nil {
						return err
					}
				}
				return nil
			},
			wantStatus: run.StatusFailed,
		},
		{
			name: "user cancel",
			stop: func(svc *service.RuntimeService, runID string) error {
				return svc.CancelRun(tenantCtx, runID)
			},
			wantStatus: run.StatusCancelled,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, _, bc := newRuntimeTestEnv()
			r, err := svc.StartRun(tenantCtx, &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"})
			if err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			if tc.setup != nil {
				store.mu.Lock()
				tc.setup(store)
				store.mu.Unlock()
			}

			if err := tc.stop(svc, r.ID); err != nil {
				t.Fatalf("stop: %v", err)
			}
			if got, _ := store.GetRun(context.Background(), r.ID); got.Status != tc.wantStatus {
				t.Fatalf("run status = %s, want %s", got.Status, tc.wantStatus)
			}
			if !hasRunStatus(bc, tc.wantStatus) {
				t.Errorf("no run.status %s broadcast", tc.wantStatus)
			}
			assertRunEventsScoped(t, bc, runTenantA)
		})
	}
}
