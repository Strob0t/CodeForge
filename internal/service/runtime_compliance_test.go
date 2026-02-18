package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// newComplianceEnv creates a RuntimeService configured for a specific exec mode.
func newComplianceEnv(_ run.ExecMode) (*service.RuntimeService, *runtimeMockStore, *runtimeMockQueue, *runtimeMockBroadcaster) {
	store := &runtimeMockStore{
		projects: []project.Project{
			{ID: "proj-1", Name: "test-project", WorkspacePath: "/tmp/test-workspace"},
		},
		agents: []agent.Agent{
			{ID: "agent-1", ProjectID: "proj-1", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
		},
		tasks: []task.Task{
			{ID: "task-1", ProjectID: "proj-1", Title: "Fix bug", Prompt: "Fix the null pointer", Status: task.StatusPending},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeCfg := config.Runtime{
		StallThreshold:       5,
		QualityGateTimeout:   60 * time.Second,
		DefaultTestCommand:   "go test ./...",
		DefaultLintCommand:   "golangci-lint run ./...",
		DeliveryCommitPrefix: "codeforge:",
	}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)
	return svc, store, queue, bc
}

// TestRuntimeCompliance runs the same scenarios against both Mount and Sandbox exec modes.
func TestRuntimeCompliance(t *testing.T) {
	modes := []run.ExecMode{run.ExecModeMount, run.ExecModeSandbox}

	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			t.Run("StartRun", func(t *testing.T) {
				svc, _, queue, bc := newComplianceEnv(mode)
				ctx := context.Background()

				req := run.StartRequest{
					TaskID:    "task-1",
					AgentID:   "agent-1",
					ProjectID: "proj-1",
					ExecMode:  mode,
				}
				r, err := svc.StartRun(ctx, &req)
				if err != nil {
					t.Fatalf("StartRun failed: %v", err)
				}
				if r.Status != run.StatusRunning {
					t.Fatalf("expected status running, got %s", r.Status)
				}
				if r.ExecMode != mode {
					t.Fatalf("expected exec_mode %s, got %s", mode, r.ExecMode)
				}

				// Verify NATS message published
				msg, ok := queue.lastMessage(messagequeue.SubjectRunStart)
				if !ok {
					t.Fatal("expected run start message to be published to NATS")
				}
				var payload messagequeue.RunStartPayload
				if err := json.Unmarshal(msg.Data, &payload); err != nil {
					t.Fatalf("failed to unmarshal: %v", err)
				}
				if payload.ExecMode != string(mode) {
					t.Fatalf("expected exec_mode %s in payload, got %s", mode, payload.ExecMode)
				}

				// Verify WS broadcast
				if len(bc.events) == 0 {
					t.Fatal("expected at least one WS event")
				}
			})

			t.Run("ToolCallFlow", func(t *testing.T) {
				svc, store, queue, _ := newComplianceEnv(mode)
				ctx := context.Background()

				// Create a running run
				store.mu.Lock()
				store.runs = append(store.runs, run.Run{
					ID:            "run-tc-" + string(mode),
					TaskID:        "task-1",
					AgentID:       "agent-1",
					ProjectID:     "proj-1",
					PolicyProfile: "headless-safe-sandbox",
					ExecMode:      mode,
					Status:        run.StatusRunning,
					StartedAt:     time.Now(),
				})
				store.mu.Unlock()

				runID := "run-tc-" + string(mode)

				// Request tool call
				req := messagequeue.ToolCallRequestPayload{
					RunID:  runID,
					CallID: "call-tc-1",
					Tool:   "Read",
					Path:   "main.go",
				}
				if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
					t.Fatalf("HandleToolCallRequest failed: %v", err)
				}

				msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
				if !ok {
					t.Fatal("expected tool call response")
				}
				var resp messagequeue.ToolCallResponsePayload
				_ = json.Unmarshal(msg.Data, &resp)
				if resp.Decision != "allow" {
					t.Fatalf("expected allow, got %s", resp.Decision)
				}

				// Report tool result
				result := messagequeue.ToolCallResultPayload{
					RunID:   runID,
					CallID:  "call-tc-1",
					Tool:    "Read",
					Success: true,
					Output:  "file contents",
					CostUSD: 0.001,
				}
				if err := svc.HandleToolCallResult(ctx, &result); err != nil {
					t.Fatalf("HandleToolCallResult failed: %v", err)
				}

				// Verify step count updated
				r, _ := store.GetRun(ctx, runID)
				if r.StepCount != 1 {
					t.Fatalf("expected step_count 1, got %d", r.StepCount)
				}
			})

			t.Run("PolicyEnforcement", func(t *testing.T) {
				svc, store, queue, _ := newComplianceEnv(mode)
				ctx := context.Background()

				// Use plan-readonly which denies Edit/Write/Bash
				store.mu.Lock()
				store.runs = append(store.runs, run.Run{
					ID:            "run-pe-" + string(mode),
					TaskID:        "task-1",
					AgentID:       "agent-1",
					ProjectID:     "proj-1",
					PolicyProfile: "plan-readonly",
					ExecMode:      mode,
					Status:        run.StatusRunning,
					StartedAt:     time.Now(),
				})
				store.mu.Unlock()

				req := messagequeue.ToolCallRequestPayload{
					RunID:  "run-pe-" + string(mode),
					CallID: "call-pe-1",
					Tool:   "Edit",
					Path:   "main.go",
				}
				if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
					t.Fatalf("HandleToolCallRequest failed: %v", err)
				}

				msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
				if !ok {
					t.Fatal("expected tool call response")
				}
				var resp messagequeue.ToolCallResponsePayload
				_ = json.Unmarshal(msg.Data, &resp)
				if resp.Decision == "allow" {
					t.Fatal("expected denial for Edit in plan-readonly")
				}
			})

			t.Run("Termination_MaxSteps", func(t *testing.T) {
				svc, store, queue, _ := newComplianceEnv(mode)
				ctx := context.Background()

				store.mu.Lock()
				store.runs = append(store.runs, run.Run{
					ID:            "run-ms-" + string(mode),
					TaskID:        "task-1",
					AgentID:       "agent-1",
					ProjectID:     "proj-1",
					PolicyProfile: "headless-safe-sandbox",
					ExecMode:      mode,
					Status:        run.StatusRunning,
					StepCount:     200, // max_steps for headless-safe-sandbox
					StartedAt:     time.Now(),
				})
				store.mu.Unlock()

				req := messagequeue.ToolCallRequestPayload{
					RunID:  "run-ms-" + string(mode),
					CallID: "call-ms-1",
					Tool:   "Read",
					Path:   "main.go",
				}
				if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
					t.Fatalf("HandleToolCallRequest failed: %v", err)
				}

				msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
				if !ok {
					t.Fatal("expected response")
				}
				var resp messagequeue.ToolCallResponsePayload
				_ = json.Unmarshal(msg.Data, &resp)
				if resp.Decision != "deny" {
					t.Fatalf("expected deny for max steps, got %s", resp.Decision)
				}

				r, _ := store.GetRun(ctx, "run-ms-"+string(mode))
				if r.Status != run.StatusTimeout {
					t.Fatalf("expected timeout status, got %s", r.Status)
				}
			})

			t.Run("Termination_MaxCost", func(t *testing.T) {
				svc, store, queue, _ := newComplianceEnv(mode)
				ctx := context.Background()

				// headless-safe-sandbox has max_cost: 5.0
				store.mu.Lock()
				store.runs = append(store.runs, run.Run{
					ID:            "run-mc-" + string(mode),
					TaskID:        "task-1",
					AgentID:       "agent-1",
					ProjectID:     "proj-1",
					PolicyProfile: "headless-safe-sandbox",
					ExecMode:      mode,
					Status:        run.StatusRunning,
					CostUSD:       5.01,
					StartedAt:     time.Now(),
				})
				store.mu.Unlock()

				req := messagequeue.ToolCallRequestPayload{
					RunID:  "run-mc-" + string(mode),
					CallID: "call-mc-1",
					Tool:   "Read",
					Path:   "main.go",
				}
				if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
					t.Fatalf("HandleToolCallRequest failed: %v", err)
				}

				msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
				if !ok {
					t.Fatal("expected response")
				}
				var resp messagequeue.ToolCallResponsePayload
				_ = json.Unmarshal(msg.Data, &resp)
				if resp.Decision != "deny" {
					t.Fatalf("expected deny for max cost, got %s", resp.Decision)
				}

				r, _ := store.GetRun(ctx, "run-mc-"+string(mode))
				if r.Status != run.StatusTimeout {
					t.Fatalf("expected timeout status, got %s", r.Status)
				}
			})

			t.Run("CancelRun", func(t *testing.T) {
				svc, store, queue, _ := newComplianceEnv(mode)
				ctx := context.Background()

				store.mu.Lock()
				store.runs = append(store.runs, run.Run{
					ID:        "run-cr-" + string(mode),
					TaskID:    "task-1",
					AgentID:   "agent-1",
					ProjectID: "proj-1",
					ExecMode:  mode,
					Status:    run.StatusRunning,
					StartedAt: time.Now(),
				})
				store.mu.Unlock()

				if err := svc.CancelRun(ctx, "run-cr-"+string(mode)); err != nil {
					t.Fatalf("CancelRun failed: %v", err)
				}

				r, _ := store.GetRun(ctx, "run-cr-"+string(mode))
				if r.Status != run.StatusCancelled {
					t.Fatalf("expected cancelled, got %s", r.Status)
				}

				_, ok := queue.lastMessage(messagequeue.SubjectRunCancel)
				if !ok {
					t.Fatal("expected cancel message on NATS")
				}
			})

			t.Run("Completion", func(t *testing.T) {
				svc, store, _, _ := newComplianceEnv(mode)
				ctx := context.Background()

				// Use plan-readonly to avoid quality gate
				store.mu.Lock()
				store.runs = append(store.runs, run.Run{
					ID:            "run-cp-" + string(mode),
					TaskID:        "task-1",
					AgentID:       "agent-1",
					ProjectID:     "proj-1",
					PolicyProfile: "plan-readonly",
					ExecMode:      mode,
					Status:        run.StatusRunning,
					StartedAt:     time.Now(),
				})
				store.mu.Unlock()

				payload := messagequeue.RunCompletePayload{
					RunID:     "run-cp-" + string(mode),
					TaskID:    "task-1",
					ProjectID: "proj-1",
					Status:    "completed",
					Output:    "done",
					StepCount: 3,
					CostUSD:   0.01,
				}
				if err := svc.HandleRunComplete(ctx, &payload); err != nil {
					t.Fatalf("HandleRunComplete failed: %v", err)
				}

				r, _ := store.GetRun(ctx, "run-cp-"+string(mode))
				if r.Status != run.StatusCompleted {
					t.Fatalf("expected completed, got %s", r.Status)
				}

				ag, _ := store.GetAgent(ctx, "agent-1")
				if ag.Status != agent.StatusIdle {
					t.Fatalf("expected agent idle, got %s", ag.Status)
				}
			})

			t.Run("StallDetection", func(t *testing.T) {
				svc, store, _, _ := newComplianceEnv(mode)
				ctx := context.Background()

				// Use StartRun to properly register the stall tracker
				req := run.StartRequest{
					TaskID:    "task-1",
					AgentID:   "agent-1",
					ProjectID: "proj-1",
					ExecMode:  mode,
				}
				r, err := svc.StartRun(ctx, &req)
				if err != nil {
					t.Fatalf("StartRun failed: %v", err)
				}

				// Feed 5 identical results to trigger stall (threshold is 5)
				for i := range 5 {
					result := messagequeue.ToolCallResultPayload{
						RunID:   r.ID,
						CallID:  "call-sd-" + string(rune('a'+i)),
						Tool:    "Read",
						Success: true,
						Output:  "same output",
						CostUSD: 0.001,
					}
					if err := svc.HandleToolCallResult(ctx, &result); err != nil {
						t.Fatalf("HandleToolCallResult[%d] failed: %v", i, err)
					}
				}

				// After 5 identical results with stall threshold 5, run should be terminated
				stalled, _ := store.GetRun(ctx, r.ID)
				if stalled.Status != run.StatusFailed {
					t.Fatalf("expected failed status from stall detection, got %s", stalled.Status)
				}
			})
		})
	}
}
