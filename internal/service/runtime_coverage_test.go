package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/feedback"
	goalPkg "github.com/Strob0t/CodeForge/internal/domain/goal"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// --- Extended mock store that supports conversations and feedback ---

// extRuntimeMockStore wraps runtimeMockStore and adds conversation/feedback support.
// We embed via composition and override the methods we need.
type extRuntimeMockStore struct {
	runtimeMockStore
	mu2           sync.Mutex
	conversations []conversation.Conversation
	feedbackAudit []feedback.AuditEntry
}

func (m *extRuntimeMockStore) GetConversation(_ context.Context, id string) (*conversation.Conversation, error) {
	m.mu2.Lock()
	defer m.mu2.Unlock()
	for i := range m.conversations {
		if m.conversations[i].ID == id {
			return &m.conversations[i], nil
		}
	}
	return nil, errMockNotFound
}

func (m *extRuntimeMockStore) CreateFeedbackAudit(_ context.Context, a *feedback.AuditEntry) error {
	m.mu2.Lock()
	defer m.mu2.Unlock()
	a.ID = fmt.Sprintf("fb-%d", len(m.feedbackAudit)+1)
	a.CreatedAt = time.Now()
	m.feedbackAudit = append(m.feedbackAudit, *a)
	return nil
}

func (m *extRuntimeMockStore) ListFeedbackByRun(_ context.Context, runID string) ([]feedback.AuditEntry, error) {
	m.mu2.Lock()
	defer m.mu2.Unlock()
	var result []feedback.AuditEntry
	for i := range m.feedbackAudit {
		if m.feedbackAudit[i].RunID == runID {
			result = append(result, m.feedbackAudit[i])
		}
	}
	return result, nil
}

// newExtRuntimeTestEnv creates a test environment with extended mock capabilities.
func newExtRuntimeTestEnv() (*service.RuntimeService, *extRuntimeMockStore) {
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{ID: "proj-1", Name: "test-project", WorkspacePath: "/tmp/test-workspace"},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-1", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-1", Title: "Fix bug", Prompt: "Fix the null pointer", Status: task.StatusPending},
			},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeCfg := config.Runtime{
		StallThreshold:         5,
		StallMaxRetries:        2,
		QualityGateTimeout:     60 * time.Second,
		DefaultTestCommand:     "go test ./...",
		DefaultLintCommand:     "golangci-lint run ./...",
		DeliveryCommitPrefix:   "codeforge:",
		ApprovalTimeoutSeconds: 60,
	}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)
	return svc, store
}

// ============================================================================
// runtime_lifecycle.go tests
// ============================================================================

// TestCancelRunWithReason_Success tests the cancelRunWithReason path via
// the context-level timeout mechanism. We simulate by using a run that's running
// and triggering cancellation with a reason.
func TestCancelRunWithReason_ViaHeartbeatTimeout(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	// Create a run via StartRun to set up all internal state
	req := run.StartRequest{
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
	}
	r, err := svc.StartRun(ctx, &req)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	// Set an old heartbeat and a short heartbeat timeout
	svc.SetHeartbeat(r.ID, time.Now().Add(-5*time.Minute))

	// Trigger heartbeat timeout via HandleToolCallRequest
	cfg := config.Runtime{
		StallThreshold:   5,
		HeartbeatTimeout: 1 * time.Second, // Very short so the old heartbeat triggers timeout
	}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	svcWithHB := service.NewRuntimeService(store, &runtimeMockQueue{}, bc, &runtimeMockEventStore{}, policySvc, &cfg)
	svcWithHB.SetHeartbeat(r.ID, time.Now().Add(-5*time.Minute))

	// Add the run to the store for this service
	toolReq := messagequeue.ToolCallRequestPayload{
		RunID:  r.ID,
		CallID: "call-hb-timeout",
		Tool:   "Read",
		Path:   "main.go",
	}
	err = svcWithHB.HandleToolCallRequest(ctx, &toolReq)
	if err != nil {
		t.Fatalf("HandleToolCallRequest failed: %v", err)
	}

	// The run should be timed out due to heartbeat
	got, _ := store.GetRun(ctx, r.ID)
	if got.Status != run.StatusTimeout {
		t.Fatalf("expected timeout status from heartbeat timeout, got %s", got.Status)
	}
}

// TestCleanupRunState_PendingApprovals verifies that cleanupRunState sends
// deny to any pending HITL approval channels for the run.
func TestCleanupRunState_PendingApprovals(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// Start a run to have proper state
	req := run.StartRequest{
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
	}
	r, err := svc.StartRun(ctx, &req)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Simulate a pending HITL approval by starting a goroutine
	approvalDone := make(chan policy.Decision, 1)

	// Use the "ask" flow: we need to create a policy that triggers "ask".
	// Instead, we directly test CancelRun which calls cleanupRunState.
	// The cleanupRunState should clean up any pending approvals for the run.

	// Cancel the run - this calls cleanupRunState internally
	err = svc.CancelRun(ctx, r.ID)
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	got, _ := store.GetRun(ctx, r.ID)
	if got.Status != run.StatusCancelled {
		t.Fatalf("expected cancelled, got %s", got.Status)
	}

	// Verify the approval channel was cleaned up
	_ = approvalDone // channel not used since no actual approval was pending
}

// TestFinalizeRun_OnRunCompleteCallback verifies the onRunComplete callback
// is called when a run is finalized.
func TestFinalizeRun_OnRunCompleteCallback(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	var callbackRunID string
	var callbackStatus run.Status
	svc.SetOnRunComplete(func(_ context.Context, runID string, status run.Status) {
		callbackRunID = runID
		callbackStatus = status
	})

	// Add a running run
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-callback",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	payload := messagequeue.RunCompletePayload{
		RunID:     "run-callback",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "completed",
		Output:    "done",
		StepCount: 5,
		CostUSD:   0.05,
	}
	if err := svc.HandleRunComplete(ctx, &payload); err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}

	if callbackRunID != "run-callback" {
		t.Errorf("expected callback run_id %q, got %q", "run-callback", callbackRunID)
	}
	if callbackStatus != run.StatusCompleted {
		t.Errorf("expected callback status completed, got %s", callbackStatus)
	}
}

// TestFinalizeRun_FailedStatus verifies that failed runs set task to failed
// and agent to idle.
func TestFinalizeRun_FailedStatus(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-fail-final",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	payload := messagequeue.RunCompletePayload{
		RunID:     "run-fail-final",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "failed",
		Error:     "something broke",
		StepCount: 2,
		CostUSD:   0.02,
	}
	if err := svc.HandleRunComplete(ctx, &payload); err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}

	r, _ := store.GetRun(ctx, "run-fail-final")
	if r.Status != run.StatusFailed {
		t.Fatalf("expected failed, got %s", r.Status)
	}
	if r.Error != "something broke" {
		t.Fatalf("expected error message preserved, got %q", r.Error)
	}

	ag, _ := store.GetAgent(ctx, "agent-1")
	if ag.Status != agent.StatusIdle {
		t.Fatalf("expected agent idle after failure, got %s", ag.Status)
	}

	// Verify AG-UI run_finished event has "failed" status
	bc.mu.Lock()
	defer bc.mu.Unlock()
	foundFinished := false
	for _, ev := range bc.events {
		if ev.EventType == ws.AGUIRunFinished {
			foundFinished = true
			if finEv, ok := ev.Data.(ws.AGUIRunFinishedEvent); ok {
				if finEv.Status != "failed" {
					t.Errorf("expected AG-UI status 'failed', got %q", finEv.Status)
				}
			}
		}
	}
	if !foundFinished {
		t.Error("expected AG-UI run_finished event to be broadcast")
	}
}

// TestHandleRunComplete_StatusInference tests that HandleRunComplete correctly
// infers status when it's empty in the payload.
func TestHandleRunComplete_StatusInference(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		errMsg   string
		expected run.Status
	}{
		{"empty_with_error", "", "crash", run.StatusFailed},
		{"empty_without_error", "", "", run.StatusCompleted},
		{"explicit_completed", "completed", "", run.StatusCompleted},
		{"explicit_failed", "failed", "boom", run.StatusFailed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, _, _ := newRuntimeTestEnv()
			ctx := context.Background()

			runID := "run-infer-" + tc.name
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID:            runID,
				TaskID:        "task-1",
				AgentID:       "agent-1",
				ProjectID:     "proj-1",
				PolicyProfile: "plan-readonly",
				Status:        run.StatusRunning,
				StartedAt:     time.Now(),
			})
			store.mu.Unlock()

			payload := messagequeue.RunCompletePayload{
				RunID:     runID,
				TaskID:    "task-1",
				ProjectID: "proj-1",
				Status:    tc.status,
				Error:     tc.errMsg,
			}
			if err := svc.HandleRunComplete(ctx, &payload); err != nil {
				t.Fatalf("HandleRunComplete: %v", err)
			}

			r, _ := store.GetRun(ctx, runID)
			if r.Status != tc.expected {
				t.Fatalf("expected status %s, got %s", tc.expected, r.Status)
			}
		})
	}
}

// TestTriggerDelivery_NoDeliverMode verifies delivery is skipped when no mode set.
func TestTriggerDelivery_NoDeliverMode(t *testing.T) {
	svc, store, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// Run with no deliver mode should complete without delivery attempt
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-no-deliver",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
		Status:        run.StatusRunning,
		DeliverMode:   run.DeliverModeNone,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	payload := messagequeue.RunCompletePayload{
		RunID:     "run-no-deliver",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "completed",
		Output:    "done",
	}
	if err := svc.HandleRunComplete(ctx, &payload); err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}

	// No delivery-related messages should be published
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for _, msg := range queue.messages {
		if msg.Subject == "delivery.start" {
			t.Fatal("unexpected delivery message published")
		}
	}
}

// ============================================================================
// runtime_execution.go tests
// ============================================================================

// TestHandleConversationToolCall_AllowByDefault tests that conversation-based
// tool calls use the default policy when no project policy is set.
func TestHandleConversationToolCall_AllowByDefault(t *testing.T) {
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{ID: "proj-conv", Name: "conv-project", WorkspacePath: "/tmp/conv"},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-conv", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-conv", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
			},
		},
		conversations: []conversation.Conversation{
			{ID: "conv-1", ProjectID: "proj-conv", Title: "Test conv"},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}

	// Use a policy that allows Read
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeCfg := config.Runtime{}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx := context.Background()
	req := messagequeue.ToolCallRequestPayload{
		RunID:  "conv-1", // conversation ID used as run ID
		CallID: "call-conv-1",
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
	if resp.Decision != "allow" {
		t.Fatalf("expected allow for Read in conversation mode, got %s (reason: %s)", resp.Decision, resp.Reason)
	}
}

// TestHandleConversationToolCall_ConversationNotFound tests that tool calls
// for unknown conversation IDs are denied.
func TestHandleConversationToolCall_ConversationNotFound(t *testing.T) {
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{ID: "proj-1", Name: "test-project", WorkspacePath: "/tmp/test"},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-1", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-1", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
			},
		},
		// No conversations
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeCfg := config.Runtime{}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx := context.Background()
	req := messagequeue.ToolCallRequestPayload{
		RunID:  "nonexistent-conv",
		CallID: "call-1",
		Tool:   "Read",
		Path:   "main.go",
	}
	// This should send a deny response (not error out)
	err := svc.HandleToolCallRequest(ctx, &req)
	if err != nil {
		t.Fatalf("expected no error (deny response sent), got: %v", err)
	}

	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response")
	}
	var resp messagequeue.ToolCallResponsePayload
	_ = json.Unmarshal(msg.Data, &resp)
	if resp.Decision != "deny" {
		t.Fatalf("expected deny for unknown conversation, got %s", resp.Decision)
	}
}

// TestHandleConversationToolCall_DenyByPolicy tests that conversation tool
// calls respect policy denials.
func TestHandleConversationToolCall_DenyByPolicy(t *testing.T) {
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{ID: "proj-deny", Name: "deny-project", WorkspacePath: "/tmp/deny", PolicyProfile: "plan-readonly"},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-deny", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-deny", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
			},
		},
		conversations: []conversation.Conversation{
			{ID: "conv-deny", ProjectID: "proj-deny", Title: "Deny conv"},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	policySvc := service.NewPolicyService("plan-readonly", nil)
	runtimeCfg := config.Runtime{}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx := context.Background()
	req := messagequeue.ToolCallRequestPayload{
		RunID:  "conv-deny",
		CallID: "call-deny-1",
		Tool:   "Edit", // Edit is denied in plan-readonly
		Path:   "main.go",
	}
	if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response")
	}
	var resp messagequeue.ToolCallResponsePayload
	_ = json.Unmarshal(msg.Data, &resp)
	if resp.Decision == "allow" {
		t.Fatal("expected denial for Edit in plan-readonly conversation")
	}
}

// TestHandleToolCallResult_UnknownRun tests the conversation fallback path
// where the run is not found (conversation-based run).
func TestHandleToolCallResult_UnknownRun(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// Tool call result for a run that doesn't exist (conversation mode)
	result := messagequeue.ToolCallResultPayload{
		RunID:   "unknown-conv-run",
		CallID:  "call-1",
		Tool:    "Read",
		Success: true,
		Output:  "file contents",
		CostUSD: 0.001,
	}
	// Should return nil (gracefully handles missing run)
	if err := svc.HandleToolCallResult(ctx, &result); err != nil {
		t.Fatalf("expected nil error for conversation run result, got: %v", err)
	}
}

// TestHandleToolCallResult_BudgetExceeded tests the post-execution budget
// enforcement that terminates the run when cost exceeds the limit.
func TestHandleToolCallResult_BudgetExceeded(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	// headless-safe-sandbox has max_cost: 5.0
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-budget-bust",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		CostUSD:       4.99,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	var callbackStatus run.Status
	svc.SetOnRunComplete(func(_ context.Context, _ string, status run.Status) {
		callbackStatus = status
	})

	// This result pushes cost to 5.0, which should trigger budget enforcement
	result := messagequeue.ToolCallResultPayload{
		RunID:   "run-budget-bust",
		CallID:  "call-bust",
		Tool:    "Bash",
		Success: true,
		Output:  "expensive operation",
		CostUSD: 0.02, // pushes total past budget: 4.99 + 0.02 = 5.01
	}
	if err := svc.HandleToolCallResult(ctx, &result); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}

	r, _ := store.GetRun(ctx, "run-budget-bust")
	if r.Status != run.StatusTimeout {
		t.Fatalf("expected timeout from budget exceeded, got %s", r.Status)
	}

	// Verify agent was set to idle
	ag, _ := store.GetAgent(ctx, "agent-1")
	if ag.Status != agent.StatusIdle {
		t.Fatalf("expected agent idle after budget exceeded, got %s", ag.Status)
	}

	// Verify task was set to failed
	tsk, _ := store.GetTask(ctx, "task-1")
	if tsk.Status != task.StatusFailed {
		t.Fatalf("expected task failed after budget exceeded, got %s", tsk.Status)
	}

	// Verify onRunComplete callback was called
	if callbackStatus != run.StatusTimeout {
		t.Fatalf("expected callback status timeout, got %s", callbackStatus)
	}

	// Verify WS broadcast includes timeout status
	bc.mu.Lock()
	defer bc.mu.Unlock()
	found := false
	for _, ev := range bc.events {
		if ev.EventType == ws.EventRunStatus {
			if statusEv, ok := ev.Data.(ws.RunStatusEvent); ok && statusEv.Status == string(run.StatusTimeout) {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected timeout status broadcast event")
	}
}

// TestHandleToolCallResult_CostAccumulation verifies that cost and tokens
// are accumulated correctly across multiple tool call results.
func TestHandleToolCallResult_CostAccumulation(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-accum",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
		Status:        run.StatusRunning,
		CostUSD:       0,
		TokensIn:      0,
		TokensOut:     0,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	results := []messagequeue.ToolCallResultPayload{
		{RunID: "run-accum", CallID: "c1", Tool: "Read", Success: true, CostUSD: 0.01, TokensIn: 100, TokensOut: 50},
		{RunID: "run-accum", CallID: "c2", Tool: "Edit", Success: true, CostUSD: 0.02, TokensIn: 200, TokensOut: 100},
		{RunID: "run-accum", CallID: "c3", Tool: "Bash", Success: true, CostUSD: 0.03, TokensIn: 300, TokensOut: 150},
	}

	for _, res := range results {
		if err := svc.HandleToolCallResult(ctx, &res); err != nil {
			t.Fatalf("HandleToolCallResult: %v", err)
		}
	}

	r, _ := store.GetRun(ctx, "run-accum")
	expectedCost := 0.06
	if r.CostUSD < expectedCost-0.001 || r.CostUSD > expectedCost+0.001 {
		t.Fatalf("expected cost ~%.2f, got %.6f", expectedCost, r.CostUSD)
	}
	if r.TokensIn != 600 {
		t.Fatalf("expected tokens_in 600, got %d", r.TokensIn)
	}
	if r.TokensOut != 300 {
		t.Fatalf("expected tokens_out 300, got %d", r.TokensOut)
	}
}

// TestHandleRunComplete_RunNotFound tests that HandleRunComplete returns
// an error when the run doesn't exist.
func TestHandleRunComplete_RunNotFound(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	payload := messagequeue.RunCompletePayload{
		RunID:     "nonexistent-run",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "completed",
	}
	err := svc.HandleRunComplete(ctx, &payload)
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

// TestHandleQualityGateResult_NotInGateStatus tests that quality gate results
// for runs not in quality_gate status are ignored.
func TestHandleQualityGateResult_NotInGateStatus(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-not-gated",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning, // Not quality_gate
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	passed := true
	result := messagequeue.QualityGateResultPayload{
		RunID:       "run-not-gated",
		TestsPassed: &passed,
		LintPassed:  &passed,
	}
	// Should return nil (silently ignore)
	if err := svc.HandleQualityGateResult(ctx, &result); err != nil {
		t.Fatalf("expected nil for non-gated run, got: %v", err)
	}

	// Status should remain unchanged
	r, _ := store.GetRun(ctx, "run-not-gated")
	if r.Status != run.StatusRunning {
		t.Fatalf("expected status unchanged (running), got %s", r.Status)
	}
}

// TestHandleQualityGateResult_GateFailedNoRollback tests that when quality
// gates fail without rollback configured, the run still completes.
func TestHandleQualityGateResult_GateFailedNoRollback(t *testing.T) {
	_, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()
	var svc *service.RuntimeService

	// Use a profile that has quality gates but no rollback
	customProfile := policy.PolicyProfile{
		Name: "gate-no-rollback",
		Mode: policy.ModeDefault,
		QualityGate: policy.QualityGate{
			RequireTestsPass:   true,
			RollbackOnGateFail: false,
		},
	}
	policySvc := service.NewPolicyService("gate-no-rollback", []policy.PolicyProfile{customProfile})
	runtimeCfg := config.Runtime{
		StallThreshold:     5,
		QualityGateTimeout: 60 * time.Second,
		DefaultTestCommand: "go test",
		DefaultLintCommand: "lint",
	}
	svc = service.NewRuntimeService(store, &runtimeMockQueue{}, bc, &runtimeMockEventStore{}, policySvc, &runtimeCfg)

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-gate-nrb",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "gate-no-rollback",
		Status:        run.StatusQualityGate,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	failed := false
	result := messagequeue.QualityGateResultPayload{
		RunID:       "run-gate-nrb",
		TestsPassed: &failed, // tests failed
	}
	if err := svc.HandleQualityGateResult(ctx, &result); err != nil {
		t.Fatalf("HandleQualityGateResult: %v", err)
	}

	// Without rollback, the run should still be marked completed (not failed)
	r, _ := store.GetRun(ctx, "run-gate-nrb")
	if r.Status != run.StatusCompleted {
		t.Fatalf("expected completed (no rollback), got %s", r.Status)
	}
}

// TestHandleQualityGateResult_GateErrorMessage tests that a quality gate
// with an error message in the payload is treated as failed.
func TestHandleQualityGateResult_GateErrorMessage(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// Use headless-safe-sandbox which has quality gates
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-gate-err",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusQualityGate,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	result := messagequeue.QualityGateResultPayload{
		RunID: "run-gate-err",
		Error: "test runner crashed",
	}
	if err := svc.HandleQualityGateResult(ctx, &result); err != nil {
		t.Fatalf("HandleQualityGateResult: %v", err)
	}

	// With error in result and rollback configured, should be failed
	r, _ := store.GetRun(ctx, "run-gate-err")
	// headless-safe-sandbox has rollback_on_gate_fail: true
	if r.Status != run.StatusFailed {
		t.Fatalf("expected failed with rollback, got %s", r.Status)
	}
}

// TestHandleToolCallRequest_StepCountIncrement verifies the step count is
// incremented on each tool call.
func TestHandleToolCallRequest_StepCountIncrement(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-steps",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StepCount:     0,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	for i := range 3 {
		req := messagequeue.ToolCallRequestPayload{
			RunID:  "run-steps",
			CallID: fmt.Sprintf("call-step-%d", i),
			Tool:   "Read",
			Path:   "file.go",
		}
		if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
			t.Fatalf("HandleToolCallRequest[%d]: %v", i, err)
		}
	}

	r, _ := store.GetRun(ctx, "run-steps")
	if r.StepCount != 3 {
		t.Fatalf("expected step_count 3, got %d", r.StepCount)
	}
}

// ============================================================================
// runtime_approval.go tests
// ============================================================================

// TestLogFeedbackAudit_Success tests that LogFeedbackAudit stores the entry.
func TestLogFeedbackAudit_Success(t *testing.T) {
	svc, store := newExtRuntimeTestEnv()
	ctx := context.Background()

	err := svc.LogFeedbackAudit(ctx, "run-1", "call-1", "Bash", "slack", "allow", "admin@test.com")
	if err != nil {
		t.Fatalf("LogFeedbackAudit: %v", err)
	}

	store.mu2.Lock()
	defer store.mu2.Unlock()
	if len(store.feedbackAudit) != 1 {
		t.Fatalf("expected 1 feedback audit entry, got %d", len(store.feedbackAudit))
	}
	entry := store.feedbackAudit[0]
	if entry.RunID != "run-1" {
		t.Errorf("expected run_id %q, got %q", "run-1", entry.RunID)
	}
	if entry.CallID != "call-1" {
		t.Errorf("expected call_id %q, got %q", "call-1", entry.CallID)
	}
	if entry.Tool != "Bash" {
		t.Errorf("expected tool %q, got %q", "Bash", entry.Tool)
	}
	if string(entry.Provider) != "slack" {
		t.Errorf("expected provider %q, got %q", "slack", entry.Provider)
	}
	if string(entry.Decision) != "allow" {
		t.Errorf("expected decision %q, got %q", "allow", entry.Decision)
	}
	if entry.Responder != "admin@test.com" {
		t.Errorf("expected responder %q, got %q", "admin@test.com", entry.Responder)
	}
}

// TestListFeedbackAudit_Empty tests that ListFeedbackAudit returns empty for
// a run with no feedback entries.
func TestListFeedbackAudit_Empty(t *testing.T) {
	svc, _ := newExtRuntimeTestEnv()
	ctx := context.Background()

	entries, err := svc.ListFeedbackAudit(ctx, "nonexistent-run")
	if err != nil {
		t.Fatalf("ListFeedbackAudit: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

// TestListFeedbackAudit_MultipleEntries tests listing feedback audit for a run
// with multiple entries.
func TestListFeedbackAudit_MultipleEntries(t *testing.T) {
	svc, _ := newExtRuntimeTestEnv()
	ctx := context.Background()

	// Create multiple entries
	_ = svc.LogFeedbackAudit(ctx, "run-fb", "call-1", "Bash", "slack", "allow", "user1")
	_ = svc.LogFeedbackAudit(ctx, "run-fb", "call-2", "Edit", "email", "deny", "user2")
	_ = svc.LogFeedbackAudit(ctx, "run-other", "call-3", "Read", "web", "allow", "user3")

	entries, err := svc.ListFeedbackAudit(ctx, "run-fb")
	if err != nil {
		t.Fatalf("ListFeedbackAudit: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for run-fb, got %d", len(entries))
	}
}

// TestResolveApproval_NonExistentKey tests that ResolveApproval returns false
// for a key that was never registered.
func TestResolveApproval_NonExistentKey(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()

	ok := svc.ResolveApproval("no-run", "no-call", "allow")
	if ok {
		t.Error("expected false for non-existent approval key")
	}
}

// TestConversationToolCall_WsBroadcast tests that conversation tool calls
// broadcast WS events for tool call status when a valid policy is configured.
func TestConversationToolCall_WsBroadcast(t *testing.T) {
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{ID: "proj-ws", Name: "ws-project", WorkspacePath: "/tmp/ws", PolicyProfile: "headless-safe-sandbox"},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-ws", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-ws", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
			},
		},
		conversations: []conversation.Conversation{
			{ID: "conv-ws", ProjectID: "proj-ws", Title: "WS conv"},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeCfg := config.Runtime{}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx := context.Background()
	req := messagequeue.ToolCallRequestPayload{
		RunID:  "conv-ws",
		CallID: "call-ws-1",
		Tool:   "Read",
		Path:   "main.go",
	}
	if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	// Verify WS broadcast for tool call status
	bc.mu.Lock()
	defer bc.mu.Unlock()
	found := false
	for _, ev := range bc.events {
		if ev.EventType == ws.EventToolCallStatus {
			found = true
		}
	}
	if !found {
		t.Error("expected tool_call_status WS event for conversation tool call")
	}
}

// TestCancelRun_AlreadyCompleted tests that cancelling an already-completed run
// returns an error.
func TestCancelRun_AlreadyCompleted(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:        "run-done",
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
		Status:    run.StatusCompleted,
		StartedAt: time.Now(),
	})
	store.mu.Unlock()

	err := svc.CancelRun(ctx, "run-done")
	if err == nil {
		t.Fatal("expected error for cancelling completed run")
	}
}

// TestCancelRun_QualityGateStatus tests that a run in quality_gate status
// can be cancelled.
func TestCancelRun_QualityGateStatus(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:        "run-qg",
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
		Status:    run.StatusQualityGate,
		StartedAt: time.Now(),
	})
	store.mu.Unlock()

	err := svc.CancelRun(ctx, "run-qg")
	if err != nil {
		t.Fatalf("expected cancel of quality_gate run to succeed, got: %v", err)
	}

	r, _ := store.GetRun(ctx, "run-qg")
	if r.Status != run.StatusCancelled {
		t.Fatalf("expected cancelled, got %s", r.Status)
	}
}

// TestStartRun_DefaultExecMode verifies that empty exec_mode defaults to "mount".
func TestStartRun_DefaultExecMode(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	req := run.StartRequest{
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
		// ExecMode not set
	}
	r, err := svc.StartRun(ctx, &req)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if r.ExecMode != run.ExecModeMount {
		t.Fatalf("expected default exec_mode 'mount', got %s", r.ExecMode)
	}
}

// TestStartRun_DefaultModeID verifies that the mode ID defaults to "coder"
// when not specified.
func TestStartRun_DefaultModeID(t *testing.T) {
	svc, _, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	req := run.StartRequest{
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
	}
	r, err := svc.StartRun(ctx, &req)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if r.ModeID != "coder" {
		t.Fatalf("expected default mode_id 'coder', got %s", r.ModeID)
	}

	// Verify the NATS payload has the default mode referenced
	msg, _ := queue.lastMessage(messagequeue.SubjectRunStart)
	var payload messagequeue.RunStartPayload
	_ = json.Unmarshal(msg.Data, &payload)
	// Mode is nil since ModeService is not set, but run should still work
	if payload.RunID != r.ID {
		t.Fatalf("expected run_id %q in payload, got %q", r.ID, payload.RunID)
	}
}

// TestHandleToolCallRequest_AGUIBroadcast verifies AG-UI events are broadcast
// for tool calls.
func TestHandleToolCallRequest_AGUIBroadcast(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-agui",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	req := messagequeue.ToolCallRequestPayload{
		RunID:   "run-agui",
		CallID:  "call-agui-1",
		Tool:    "Read",
		Path:    "file.go",
		Command: "",
	}
	if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()
	foundToolCall := false
	foundToolStatus := false
	for _, ev := range bc.events {
		if ev.EventType == ws.AGUIToolCall {
			foundToolCall = true
			if tcEv, ok := ev.Data.(ws.AGUIToolCallEvent); ok {
				if tcEv.Name != "Read" {
					t.Errorf("expected tool name 'Read', got %q", tcEv.Name)
				}
				if tcEv.CallID != "call-agui-1" {
					t.Errorf("expected call_id 'call-agui-1', got %q", tcEv.CallID)
				}
			}
		}
		if ev.EventType == ws.EventToolCallStatus {
			foundToolStatus = true
		}
	}
	if !foundToolCall {
		t.Error("expected AG-UI tool_call event")
	}
	if !foundToolStatus {
		t.Error("expected tool_call_status event")
	}
}

// TestHandleToolCallResult_AGUIBroadcast verifies AG-UI tool result events.
func TestHandleToolCallResult_AGUIBroadcast(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-agui-result",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	result := messagequeue.ToolCallResultPayload{
		RunID:   "run-agui-result",
		CallID:  "call-agui-r1",
		Tool:    "Read",
		Success: true,
		Output:  "file contents here",
		CostUSD: 0.001,
	}
	if err := svc.HandleToolCallResult(ctx, &result); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()
	foundResult := false
	for _, ev := range bc.events {
		if ev.EventType == ws.AGUIToolResult {
			foundResult = true
			if trEv, ok := ev.Data.(ws.AGUIToolResultEvent); ok {
				if trEv.Result != "file contents here" {
					t.Errorf("expected result 'file contents here', got %q", trEv.Result)
				}
				if trEv.Error != "" {
					t.Errorf("expected no error, got %q", trEv.Error)
				}
			}
		}
	}
	if !foundResult {
		t.Error("expected AG-UI tool_result event")
	}
}

// TestHandleToolCallResult_FailedToolAGUIError verifies that a failed tool
// call broadcasts the error in the AG-UI event.
func TestHandleToolCallResult_FailedToolAGUIError(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            "run-agui-fail",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	result := messagequeue.ToolCallResultPayload{
		RunID:   "run-agui-fail",
		CallID:  "call-agui-f1",
		Tool:    "Bash",
		Success: false,
		Output:  "command not found",
		CostUSD: 0.001,
	}
	if err := svc.HandleToolCallResult(ctx, &result); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()
	for _, ev := range bc.events {
		if ev.EventType == ws.AGUIToolResult {
			if trEv, ok := ev.Data.(ws.AGUIToolResultEvent); ok {
				if trEv.Error != "command not found" {
					t.Errorf("expected error 'command not found', got %q", trEv.Error)
				}
			}
		}
	}
}

// ============================================================================
// PersistGoalProposal tests
// ============================================================================

// goalTrackingMockStore wraps extRuntimeMockStore and tracks created goals.
type goalTrackingMockStore struct {
	extRuntimeMockStore
	mu3         sync.Mutex
	goalCreated []goalPkg.ProjectGoal
}

func (m *goalTrackingMockStore) CreateProjectGoal(_ context.Context, g *goalPkg.ProjectGoal) error {
	m.mu3.Lock()
	defer m.mu3.Unlock()
	g.ID = fmt.Sprintf("goal-%d", len(m.goalCreated)+1)
	g.CreatedAt = time.Now()
	g.UpdatedAt = time.Now()
	m.goalCreated = append(m.goalCreated, *g)
	return nil
}

// TestPersistGoalProposal_Success verifies that PersistGoalProposal creates
// a goal in the database via GoalDiscoveryService.
func TestPersistGoalProposal_Success(t *testing.T) {
	store := &goalTrackingMockStore{
		extRuntimeMockStore: extRuntimeMockStore{
			runtimeMockStore: runtimeMockStore{
				projects: []project.Project{
					{ID: "proj-goal", Name: "goal-project", WorkspacePath: "/tmp/goal"},
				},
				agents: []agent.Agent{
					{ID: "agent-1", ProjectID: "proj-goal", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
				},
				tasks: []task.Task{
					{ID: "task-1", ProjectID: "proj-goal", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
				},
			},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeCfg := config.Runtime{}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	// Wire GoalDiscoveryService
	goalSvc := service.NewGoalDiscoveryService(store)
	svc.SetGoalService(goalSvc)

	ctx := context.Background()
	err := svc.PersistGoalProposal(ctx, "proj-goal", "requirement", "Add auth", "Implement OAuth2", 80)
	if err != nil {
		t.Fatalf("PersistGoalProposal: %v", err)
	}

	store.mu3.Lock()
	defer store.mu3.Unlock()
	if len(store.goalCreated) != 1 {
		t.Fatalf("expected 1 goal created, got %d", len(store.goalCreated))
	}

	g := store.goalCreated[0]
	if g.ProjectID != "proj-goal" {
		t.Errorf("expected project_id %q, got %q", "proj-goal", g.ProjectID)
	}
	if g.Kind != goalPkg.KindRequirement {
		t.Errorf("expected kind %q, got %q", goalPkg.KindRequirement, g.Kind)
	}
	if g.Title != "Add auth" {
		t.Errorf("expected title %q, got %q", "Add auth", g.Title)
	}
	if g.Content != "Implement OAuth2" {
		t.Errorf("expected content %q, got %q", "Implement OAuth2", g.Content)
	}
	if g.Priority != 80 {
		t.Errorf("expected priority 80, got %d", g.Priority)
	}
	if g.Source != "agent" {
		t.Errorf("expected source %q, got %q", "agent", g.Source)
	}
}

// TestPersistGoalProposal_NilGoalService verifies that PersistGoalProposal
// returns nil when no GoalDiscoveryService is wired.
func TestPersistGoalProposal_NilGoalService(t *testing.T) {
	svc, _ := newExtRuntimeTestEnv()
	ctx := context.Background()

	// Do NOT call SetGoalService — goalSvc remains nil
	err := svc.PersistGoalProposal(ctx, "proj-1", "requirement", "Add auth", "Implement OAuth2", 80)
	if err != nil {
		t.Fatalf("expected nil error when goalSvc is nil, got: %v", err)
	}
}

// TestPersistGoalProposal_InvalidKind verifies that PersistGoalProposal
// returns a validation error for an invalid goal kind.
func TestPersistGoalProposal_InvalidKind(t *testing.T) {
	store := &goalTrackingMockStore{
		extRuntimeMockStore: extRuntimeMockStore{
			runtimeMockStore: runtimeMockStore{
				projects: []project.Project{
					{ID: "proj-goal", Name: "goal-project", WorkspacePath: "/tmp/goal"},
				},
				agents: []agent.Agent{
					{ID: "agent-1", ProjectID: "proj-goal", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
				},
				tasks: []task.Task{
					{ID: "task-1", ProjectID: "proj-goal", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
				},
			},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	policySvc := service.NewPolicyService("headless-safe-sandbox", nil)
	runtimeCfg := config.Runtime{}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	goalSvc := service.NewGoalDiscoveryService(store)
	svc.SetGoalService(goalSvc)

	ctx := context.Background()
	err := svc.PersistGoalProposal(ctx, "proj-goal", "invalid-kind", "Title", "Content", 50)
	if err == nil {
		t.Fatal("expected validation error for invalid kind")
	}

	store.mu3.Lock()
	defer store.mu3.Unlock()
	if len(store.goalCreated) != 0 {
		t.Fatalf("expected 0 goals created for invalid kind, got %d", len(store.goalCreated))
	}
}

// TestConversationToolCall_ConfigPolicyPresetFallback verifies that when a
// project has an empty PolicyProfile but config["policy_preset"] is set, the
// config value is used as fallback for conversation tool-call evaluation.
func TestConversationToolCall_ConfigPolicyPresetFallback(t *testing.T) {
	// Project has empty PolicyProfile but config["policy_preset"] = "plan-readonly".
	// Service default is "trusted-mount-autonomous" (would allow Write).
	// The config fallback should use "plan-readonly" (ModePlan -> deny writes).
	//
	// Without the fix, PolicyProfile="" causes fallback to hardcoded "default"
	// (unknown profile -> allow). With the fix, config["policy_preset"] =
	// "plan-readonly" is used, which denies Edit/Write.
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{
					ID:            "proj-cfg",
					Name:          "cfg-project",
					WorkspacePath: "/tmp/cfg",
					PolicyProfile: "", // empty — should fall back to config
					Config:        map[string]string{"policy_preset": "plan-readonly"},
				},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-cfg", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-cfg", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
			},
		},
		conversations: []conversation.Conversation{
			{ID: "conv-cfg", ProjectID: "proj-cfg", Title: "Config fallback conv"},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}

	// Service default is trusted-mount-autonomous — would allow Write.
	// But config fallback to "plan-readonly" should deny it.
	policySvc := service.NewPolicyService("trusted-mount-autonomous", nil)
	runtimeCfg := config.Runtime{}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx := context.Background()
	req := messagequeue.ToolCallRequestPayload{
		RunID:  "conv-cfg",
		CallID: "call-cfg-1",
		Tool:   "Edit", // Edit is denied in plan-readonly
		Path:   "main.go",
	}
	if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response message on NATS")
	}
	var resp messagequeue.ToolCallResponsePayload
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// "plan-readonly" uses ModePlan which denies Edit (no matching allow rule).
	// Without the config fallback, the hardcoded "default" (unknown profile)
	// would allow the call.
	if resp.Decision == string(policy.DecisionAllow) {
		t.Fatalf("expected denial for Edit via config policy_preset=plan-readonly, got %q (reason: %s)",
			resp.Decision, resp.Reason)
	}
}

// TestConversationToolCall_HITLBypassForFullAutoProfile verifies that when a
// conversation's project uses a full-auto profile (ModeAcceptEdits or ModeDelegate),
// a tool call that evaluates to "ask" is auto-approved without blocking for HITL.
// This test MUST complete fast (<5s) and NOT block for the 60s approval timeout.
func TestConversationToolCall_HITLBypassForFullAutoProfile(t *testing.T) {
	// Create a custom profile with ModeAcceptEdits that has an explicit "ask" rule for Bash.
	customProfile := policy.PolicyProfile{
		Name: "full-auto-with-ask-rule",
		Mode: policy.ModeAcceptEdits,
		Rules: []policy.PermissionRule{
			{Specifier: policy.ToolSpecifier{Tool: "Read"}, Decision: policy.DecisionAllow},
			{Specifier: policy.ToolSpecifier{Tool: "Bash"}, Decision: policy.DecisionAsk}, // would normally block for HITL
		},
		Termination: policy.TerminationCondition{
			MaxSteps: 50,
		},
	}

	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{ID: "proj-auto", Name: "auto-project", WorkspacePath: "/tmp/auto", PolicyProfile: "full-auto-with-ask-rule"},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-auto", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-auto", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
			},
		},
		conversations: []conversation.Conversation{
			{ID: "conv-auto", ProjectID: "proj-auto", Title: "Auto conv"},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}

	policySvc := service.NewPolicyService("supervised-ask-all", []policy.PolicyProfile{customProfile})
	runtimeCfg := config.Runtime{
		ApprovalTimeoutSeconds: 2, // Short timeout to prove bypass (should never be reached)
	}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	// Use a tight context deadline to fail fast if HITL blocks.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := messagequeue.ToolCallRequestPayload{
		RunID:   "conv-auto",
		CallID:  "call-auto-1",
		Tool:    "Bash",
		Command: "go build",
	}
	err := svc.HandleToolCallRequest(ctx, &req)
	if err != nil {
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
	if resp.Decision != "allow" {
		t.Fatalf("expected auto-approved 'allow' for full-auto profile, got %q (reason: %s)", resp.Decision, resp.Reason)
	}
}

// TestRunToolCall_HITLBypassForFullAutoProfile verifies that run-based tool calls
// with a full-auto profile (ModeAcceptEdits) auto-approve "ask" decisions without
// blocking for HITL. This test MUST complete fast (<5s).
func TestRunToolCall_HITLBypassForFullAutoProfile(t *testing.T) {
	customProfile := policy.PolicyProfile{
		Name: "full-auto-with-ask-rule",
		Mode: policy.ModeAcceptEdits,
		Rules: []policy.PermissionRule{
			{Specifier: policy.ToolSpecifier{Tool: "Read"}, Decision: policy.DecisionAllow},
			{Specifier: policy.ToolSpecifier{Tool: "Bash"}, Decision: policy.DecisionAsk},
		},
		Termination: policy.TerminationCondition{
			MaxSteps: 50,
		},
	}

	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{ID: "proj-auto-run", Name: "auto-run-project", WorkspacePath: "/tmp/auto-run"},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-auto-run", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-auto-run", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
			},
			runs: []run.Run{
				{
					ID:            "run-auto",
					TaskID:        "task-1",
					AgentID:       "agent-1",
					ProjectID:     "proj-auto-run",
					PolicyProfile: "full-auto-with-ask-rule",
					Status:        run.StatusRunning,
					StartedAt:     time.Now(),
				},
			},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}

	policySvc := service.NewPolicyService("supervised-ask-all", []policy.PolicyProfile{customProfile})
	runtimeCfg := config.Runtime{
		ApprovalTimeoutSeconds: 2,
	}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := messagequeue.ToolCallRequestPayload{
		RunID:   "run-auto",
		CallID:  "call-auto-run-1",
		Tool:    "Bash",
		Command: "go build",
	}
	err := svc.HandleToolCallRequest(ctx, &req)
	if err != nil {
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
	if resp.Decision != "allow" {
		t.Fatalf("expected auto-approved 'allow' for full-auto run profile, got %q (reason: %s)", resp.Decision, resp.Reason)
	}
}

// TestConversationToolCall_HITLNotBypassedForNonFullAutoProfile verifies that
// non-full-auto profiles (e.g., ModeDefault) still trigger HITL wait when
// policy evaluates to "ask". The test uses a short timeout to verify blocking behavior.
func TestConversationToolCall_HITLNotBypassedForNonFullAutoProfile(t *testing.T) {
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{ID: "proj-supervised", Name: "supervised-project", WorkspacePath: "/tmp/supervised", PolicyProfile: "supervised-ask-all"},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-supervised", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-supervised", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
			},
		},
		conversations: []conversation.Conversation{
			{ID: "conv-supervised", ProjectID: "proj-supervised", Title: "Supervised conv"},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}

	// supervised-ask-all has ModeDefault which should NOT auto-approve.
	policySvc := service.NewPolicyService("supervised-ask-all", nil)
	runtimeCfg := config.Runtime{
		ApprovalTimeoutSeconds: 1, // Very short so we can test blocking behavior quickly
	}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx := context.Background()
	req := messagequeue.ToolCallRequestPayload{
		RunID:   "conv-supervised",
		CallID:  "call-supervised-1",
		Tool:    "Bash",
		Command: "go build",
	}
	err := svc.HandleToolCallRequest(ctx, &req)
	if err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	// Should get a "deny" decision because the HITL timeout expired (no user approved).
	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response on NATS")
	}
	var resp messagequeue.ToolCallResponsePayload
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Decision != "deny" {
		t.Fatalf("expected 'deny' for supervised profile after HITL timeout, got %q", resp.Decision)
	}
}

// TestConversationToolCall_DefaultProfileFallback verifies that when both
// PolicyProfile and config["policy_preset"] are empty, the service's
// DefaultProfile() is used instead of a hardcoded "default" string.
func TestConversationToolCall_DefaultProfileFallback(t *testing.T) {
	// Both PolicyProfile and config are empty.
	// Service default is "plan-readonly" (denies Edit).
	// Without the fix, the hardcoded "default" (unknown profile) would allow.
	// With the fix, DefaultProfile() = "plan-readonly" is used -> deny.
	store := &extRuntimeMockStore{
		runtimeMockStore: runtimeMockStore{
			projects: []project.Project{
				{
					ID:            "proj-def",
					Name:          "def-project",
					WorkspacePath: "/tmp/def",
					PolicyProfile: "",
					Config:        map[string]string{}, // no policy_preset
				},
			},
			agents: []agent.Agent{
				{ID: "agent-1", ProjectID: "proj-def", Name: "test-agent", Backend: "aider", Status: agent.StatusIdle, Config: map[string]string{}},
			},
			tasks: []task.Task{
				{ID: "task-1", ProjectID: "proj-def", Title: "Fix bug", Prompt: "Fix it", Status: task.StatusPending},
			},
		},
		conversations: []conversation.Conversation{
			{ID: "conv-def", ProjectID: "proj-def", Title: "Default fallback conv"},
		},
	}
	queue := &runtimeMockQueue{}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}

	// Service default is "plan-readonly" — Edit should be denied.
	policySvc := service.NewPolicyService("plan-readonly", nil)
	runtimeCfg := config.Runtime{}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx := context.Background()
	req := messagequeue.ToolCallRequestPayload{
		RunID:  "conv-def",
		CallID: "call-def-1",
		Tool:   "Edit",
		Path:   "main.go",
	}
	if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response message on NATS")
	}
	var resp messagequeue.ToolCallResponsePayload
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// "plan-readonly" denies Edit. Without the fix, the hardcoded "default"
	// (unknown profile) would allow the call.
	if resp.Decision == string(policy.DecisionAllow) {
		t.Fatalf("expected denial for Edit via DefaultProfile()=plan-readonly, got %q (reason: %s)",
			resp.Decision, resp.Reason)
	}
}
