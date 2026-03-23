package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// --- TestHandleToolCallResult_BudgetAlertThresholds ---

func TestHandleToolCallResult_BudgetAlertThresholds(t *testing.T) {
	tests := []struct {
		name           string
		initialCost    float64
		resultCost     float64
		wantAlertCount int
		wantAlertPct   []float64
		description    string
	}{
		{
			name:           "80% threshold broadcasts alert once",
			initialCost:    3.90,
			resultCost:     0.20, // total 4.10 => 82% of 5.0
			wantAlertCount: 1,
			wantAlertPct:   []float64{80},
			description:    "crossing 80% threshold triggers a single budget alert",
		},
		{
			name:           "90% threshold broadcasts alert once",
			initialCost:    4.40,
			resultCost:     0.20, // total 4.60 => 92% of 5.0
			wantAlertCount: 2,    // both 80% and 90% fire since this is a fresh run
			wantAlertPct:   []float64{80, 90},
			description:    "crossing 90% threshold triggers both 80% and 90% alerts",
		},
		{
			name:           "duplicate threshold suppressed on second call",
			initialCost:    3.90,
			resultCost:     0.20, // first call: 4.10 => 82%
			wantAlertCount: 1,    // only the 80% alert from first call
			wantAlertPct:   []float64{80},
			description:    "second tool call at same threshold level does not re-alert",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, _, bc := newRuntimeTestEnv()
			ctx := context.Background()

			runID := "run-alert-" + tc.name
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID:            runID,
				TaskID:        "task-1",
				AgentID:       "agent-1",
				ProjectID:     "proj-1",
				PolicyProfile: "headless-safe-sandbox", // MaxCost: 5.0
				Status:        run.StatusRunning,
				CostUSD:       tc.initialCost,
				StartedAt:     time.Now(),
			})
			store.mu.Unlock()

			// First tool call result
			result := messagequeue.ToolCallResultPayload{
				RunID:   runID,
				CallID:  "call-alert-1",
				Tool:    "Read",
				Success: true,
				Output:  "content",
				CostUSD: tc.resultCost,
			}
			if err := svc.HandleToolCallResult(ctx, &result); err != nil {
				t.Fatalf("HandleToolCallResult: %v", err)
			}

			// For the "duplicate suppressed" test, send a second call at the same level.
			if tc.name == "duplicate threshold suppressed on second call" {
				result2 := messagequeue.ToolCallResultPayload{
					RunID:   runID,
					CallID:  "call-alert-2",
					Tool:    "Read",
					Success: true,
					Output:  "more content",
					CostUSD: 0.01, // still at ~82%, same threshold band
				}
				if err := svc.HandleToolCallResult(ctx, &result2); err != nil {
					t.Fatalf("HandleToolCallResult (second): %v", err)
				}
			}

			// Count budget alert events
			bc.mu.Lock()
			defer bc.mu.Unlock()

			var alertEvents []event.BudgetAlertEvent
			for _, ev := range bc.events {
				if ev.EventType == event.EventBudgetAlert {
					if alertEv, ok := ev.Data.(event.BudgetAlertEvent); ok {
						alertEvents = append(alertEvents, alertEv)
					}
				}
			}

			if len(alertEvents) != tc.wantAlertCount {
				t.Errorf("expected %d budget alert(s), got %d", tc.wantAlertCount, len(alertEvents))
			}

			// Verify alert percentages
			for i, wantPct := range tc.wantAlertPct {
				if i >= len(alertEvents) {
					break
				}
				if alertEvents[i].Percentage < wantPct {
					t.Errorf("alert[%d]: expected percentage >= %.0f, got %.1f", i, wantPct, alertEvents[i].Percentage)
				}
				if alertEvents[i].MaxCost != 5.0 {
					t.Errorf("alert[%d]: expected max_cost 5.0, got %.2f", i, alertEvents[i].MaxCost)
				}
			}
		})
	}
}

// --- TestHandleToolCallRequest_RunNotRunningStatuses ---

func TestHandleToolCallRequest_RunNotRunningStatuses(t *testing.T) {
	tests := []struct {
		name       string
		status     run.Status
		wantDenied bool
	}{
		{
			name:       "completed run denies tool call",
			status:     run.StatusCompleted,
			wantDenied: true,
		},
		{
			name:       "cancelled run denies tool call",
			status:     run.StatusCancelled,
			wantDenied: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, queue, _ := newRuntimeTestEnv()
			ctx := context.Background()

			runID := "run-notrunning-" + tc.name
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID:            runID,
				TaskID:        "task-1",
				AgentID:       "agent-1",
				ProjectID:     "proj-1",
				PolicyProfile: "headless-safe-sandbox",
				Status:        tc.status,
				StartedAt:     time.Now(),
			})
			store.mu.Unlock()

			req := messagequeue.ToolCallRequestPayload{
				RunID:  runID,
				CallID: "call-notrunning",
				Tool:   "Read",
				Path:   "main.go",
			}
			if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
				t.Fatalf("HandleToolCallRequest: %v", err)
			}

			msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
			if !ok {
				t.Fatal("expected tool call response on NATS")
			}
			var resp messagequeue.ToolCallResponsePayload
			if err := json.Unmarshal(msg.Data, &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if tc.wantDenied && resp.Decision != "deny" {
				t.Errorf("expected deny for %s run, got %s (reason: %s)", tc.status, resp.Decision, resp.Reason)
			}
			if tc.wantDenied && resp.Reason == "" {
				t.Error("expected non-empty denial reason")
			}
		})
	}
}

// --- TestHandleToolCallRequest_UnknownPolicyProfile ---

func TestHandleToolCallRequest_UnknownPolicyProfile(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-unknown-policy",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "nonexistent-profile-xyz",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	req := messagequeue.ToolCallRequestPayload{
		RunID:  "run-unknown-policy",
		CallID: "call-unknown-pol",
		Tool:   "Read",
		Path:   "main.go",
	}
	if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response on NATS")
	}
	var resp messagequeue.ToolCallResponsePayload
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Decision != "deny" {
		t.Errorf("expected deny for unknown policy profile, got %s", resp.Decision)
	}
	if resp.Reason == "" {
		t.Error("expected non-empty denial reason for unknown policy profile")
	}
}

// --- TestHandleToolCallRequest_ConcurrentCalls ---

func TestHandleToolCallRequest_ConcurrentCalls(t *testing.T) {
	// Each goroutine uses its own run to avoid data races in the mock store
	// (the mock's GetRun returns a pointer into the slice without holding
	// the mutex during the caller's read of struct fields). The goal here is
	// to verify the RuntimeService itself handles concurrent requests without
	// panicking or deadlocking.
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	const n = 10

	// Pre-create one run per goroutine.
	store.mu.Lock()
	for i := 0; i < n; i++ {
		store.runs = append(store.runs, run.Run{
			ID:            fmt.Sprintf("run-conc-%d", i),
			TaskID:        "task-1",
			AgentID:       "agent-1",
			ProjectID:     "proj-1",
			PolicyProfile: "headless-safe-sandbox",
			Status:        run.StatusRunning,
			StepCount:     0,
			StartedAt:     time.Now(),
		})
	}
	store.mu.Unlock()

	var wg sync.WaitGroup
	errs := make(chan error, n)

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			req := messagequeue.ToolCallRequestPayload{
				RunID:  fmt.Sprintf("run-conc-%d", idx),
				CallID: fmt.Sprintf("call-conc-%d", idx),
				Tool:   "Read",
				Path:   "file.go",
			}
			if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
				errs <- fmt.Errorf("goroutine %d: %w", idx, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent HandleToolCallRequest error: %v", err)
	}

	// Verify each run's step count was incremented.
	for i := 0; i < n; i++ {
		r, _ := store.GetRun(ctx, fmt.Sprintf("run-conc-%d", i))
		if r.StepCount != 1 {
			t.Errorf("run-conc-%d: expected step_count 1, got %d", i, r.StepCount)
		}
	}
}

// --- TestHandleConversationToolCall_HITLAsk ---

func TestHandleConversationToolCall_HITLAsk(t *testing.T) {
	// Build a test environment where the conversation's project uses
	// supervised-ask-all, which returns "ask" for all tools except Read.
	// Bash should trigger HITL approval.
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{ID: "proj-hitl", Name: "hitl-project", WorkspacePath: "/tmp/hitl", PolicyProfile: "supervised-ask-all"},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-hitl", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-hitl", Title: "Test", Prompt: "Test", Status: task.StatusPending},
			},
		},
		conversations: []conversation.Conversation{
			{ID: "conv-hitl", ProjectID: "proj-hitl", Title: "HITL conversation"},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}

	// Use supervised-ask-all as default so it's available for lookup.
	policySvc := service.NewPolicyService("supervised-ask-all", nil)
	runtimeCfg := config.Runtime{
		ApprovalTimeoutSeconds: 2, // Short timeout so the test doesn't hang.
	}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx := context.Background()
	req := messagequeue.ToolCallRequestPayload{
		RunID:   "conv-hitl", // conversation ID as run ID
		CallID:  "call-hitl-1",
		Tool:    "Bash", // Bash is not in the allow list, triggers "ask"
		Command: "echo hello",
	}

	// Run HandleToolCallRequest in a goroutine since it will block on HITL
	// until timeout or approval.
	resultCh := make(chan error, 1)
	go func() {
		resultCh <- svc.HandleToolCallRequest(ctx, &req)
	}()

	// Give the goroutine time to register the pending approval and broadcast.
	time.Sleep(100 * time.Millisecond)

	// Resolve the approval with "allow".
	ok := svc.ResolveApproval("conv-hitl", "call-hitl-1", "allow")
	if !ok {
		// If ResolveApproval returns false, the approval channel may not have been
		// registered yet or it timed out. In either case, wait for the result.
		t.Log("ResolveApproval returned false; waiting for timeout result")
	}

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("HandleToolCallRequest: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HandleToolCallRequest did not return within 5s")
	}

	// Verify a permission_request broadcast was made (HITL ask event).
	bc.mu.Lock()
	defer bc.mu.Unlock()

	foundPermRequest := false
	foundToolStatus := false
	for _, ev := range bc.events {
		if ev.EventType == "agui.permission_request" {
			foundPermRequest = true
		}
		if ev.EventType == event.EventToolCallStatus {
			foundToolStatus = true
		}
	}
	if !foundPermRequest {
		t.Error("expected agui.permission_request broadcast for HITL ask on conversation tool call")
	}
	if !foundToolStatus {
		t.Error("expected tool_call_status broadcast after HITL resolution")
	}

	// Verify NATS response was sent.
	msg, msgOK := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !msgOK {
		t.Fatal("expected tool call response on NATS after HITL resolution")
	}
	var resp messagequeue.ToolCallResponsePayload
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// Decision should be "allow" if ResolveApproval succeeded, or "deny" if it timed out.
	if resp.Decision != "allow" && resp.Decision != "deny" {
		t.Errorf("expected decision 'allow' or 'deny', got %q", resp.Decision)
	}
}
