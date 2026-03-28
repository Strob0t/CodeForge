package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// ============================================================================
// HandleToolCallRequest tests
// ============================================================================

func TestHandleToolCallRequest_PolicyAllows(t *testing.T) {
	tests := []struct {
		name         string
		tool         string
		path         string
		wantDecision string
	}{
		{
			name:         "Read tool is allowed by headless-safe-sandbox",
			tool:         "Read",
			path:         "main.go",
			wantDecision: "allow",
		},
		{
			name:         "Glob tool is allowed by headless-safe-sandbox",
			tool:         "Glob",
			path:         "*.go",
			wantDecision: "allow",
		},
		{
			name:         "Grep tool is allowed by headless-safe-sandbox",
			tool:         "Grep",
			path:         "",
			wantDecision: "allow",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, queue, bc := newRuntimeTestEnv()
			ctx := context.Background()

			runID := "run-allow-" + tc.tool
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID:            runID,
				TaskID:        "task-1",
				AgentID:       "agent-1",
				ProjectID:     "proj-1",
				PolicyProfile: "headless-safe-sandbox",
				Status:        run.StatusRunning,
				StepCount:     0,
				StartedAt:     time.Now(),
			})
			store.mu.Unlock()

			req := messagequeue.ToolCallRequestPayload{
				RunID:  runID,
				CallID: "call-allow-" + tc.tool,
				Tool:   tc.tool,
				Path:   tc.path,
			}
			if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
				t.Fatalf("HandleToolCallRequest: %v", err)
			}

			// Verify NATS response
			msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
			if !ok {
				t.Fatal("expected tool call response on NATS")
			}
			var resp messagequeue.ToolCallResponsePayload
			if err := json.Unmarshal(msg.Data, &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if resp.Decision != tc.wantDecision {
				t.Errorf("expected decision %q, got %q (reason: %s)", tc.wantDecision, resp.Decision, resp.Reason)
			}
			if resp.RunID != runID {
				t.Errorf("expected run_id %q, got %q", runID, resp.RunID)
			}
			if resp.CallID != req.CallID {
				t.Errorf("expected call_id %q, got %q", req.CallID, resp.CallID)
			}

			// Verify step count was incremented
			r, _ := store.GetRun(ctx, runID)
			if r.StepCount != 1 {
				t.Errorf("expected step_count 1, got %d", r.StepCount)
			}

			// Verify WS broadcast includes tool_call_status
			bc.mu.Lock()
			defer bc.mu.Unlock()
			foundToolStatus := false
			for _, ev := range bc.events {
				if ev.EventType == event.EventToolCallStatus {
					foundToolStatus = true
				}
			}
			if !foundToolStatus {
				t.Error("expected tool_call_status broadcast event")
			}
		})
	}
}

func TestHandleToolCallRequest_PolicyDenies(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		command string
	}{
		{
			name:    "Bash denied without matching command allow",
			tool:    "Bash",
			command: "rm -rf /",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, queue, _ := newRuntimeTestEnv()
			ctx := context.Background()

			runID := "run-deny-" + tc.tool
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID:            runID,
				TaskID:        "task-1",
				AgentID:       "agent-1",
				ProjectID:     "proj-1",
				PolicyProfile: "headless-safe-sandbox",
				Status:        run.StatusRunning,
				StepCount:     0,
				StartedAt:     time.Now(),
			})
			store.mu.Unlock()

			req := messagequeue.ToolCallRequestPayload{
				RunID:   runID,
				CallID:  "call-deny-" + tc.tool,
				Tool:    tc.tool,
				Command: tc.command,
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
				t.Errorf("expected deny, got %q", resp.Decision)
			}
		})
	}
}

func TestHandleToolCallRequest_TerminationMaxStepsTriggersTimeout(t *testing.T) {
	svc, store, queue, bc := newRuntimeTestEnv()
	ctx := context.Background()

	// headless-safe-sandbox has MaxSteps=50; set step count at the limit.
	runID := "run-termination-steps"
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            runID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StepCount:     50, // at limit
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	req := messagequeue.ToolCallRequestPayload{
		RunID:  runID,
		CallID: "call-term-steps",
		Tool:   "Read",
		Path:   "file.go",
	}
	if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	// Should be denied due to termination
	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response on NATS")
	}
	var resp messagequeue.ToolCallResponsePayload
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Decision != "deny" {
		t.Errorf("expected deny due to max steps termination, got %q", resp.Decision)
	}

	// Run should be marked as timeout
	r, _ := store.GetRun(ctx, runID)
	if r.Status != run.StatusTimeout {
		t.Errorf("expected run status timeout, got %s", r.Status)
	}

	// Verify run_status broadcast with timeout
	bc.mu.Lock()
	defer bc.mu.Unlock()
	foundTimeout := false
	for _, ev := range bc.events {
		if ev.EventType == event.EventRunStatus {
			if statusEv, ok := ev.Data.(event.RunStatusEvent); ok {
				if statusEv.Status == string(run.StatusTimeout) {
					foundTimeout = true
				}
			}
		}
	}
	if !foundTimeout {
		t.Error("expected run_status broadcast with timeout status")
	}
}

// ============================================================================
// HandleRunComplete tests
// ============================================================================

func TestHandleRunComplete_NoQualityGate(t *testing.T) {
	tests := []struct {
		name           string
		policyProfile  string
		payloadStatus  string
		payloadError   string
		expectedStatus run.Status
	}{
		{
			name:           "completed run with plan-readonly profile (no gates)",
			policyProfile:  "plan-readonly",
			payloadStatus:  "completed",
			payloadError:   "",
			expectedStatus: run.StatusCompleted,
		},
		{
			name:           "failed run is finalized directly",
			policyProfile:  "plan-readonly",
			payloadStatus:  "failed",
			payloadError:   "tool crash",
			expectedStatus: run.StatusFailed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, _, bc := newRuntimeTestEnv()
			ctx := context.Background()

			runID := "run-complete-" + tc.name
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID:            runID,
				TaskID:        "task-1",
				AgentID:       "agent-1",
				ProjectID:     "proj-1",
				PolicyProfile: tc.policyProfile,
				Status:        run.StatusRunning,
				StartedAt:     time.Now(),
			})
			store.mu.Unlock()

			payload := messagequeue.RunCompletePayload{
				RunID:     runID,
				TaskID:    "task-1",
				ProjectID: "proj-1",
				Status:    tc.payloadStatus,
				Error:     tc.payloadError,
				StepCount: 3,
				CostUSD:   0.05,
			}
			if err := svc.HandleRunComplete(ctx, &payload); err != nil {
				t.Fatalf("HandleRunComplete: %v", err)
			}

			r, _ := store.GetRun(ctx, runID)
			if r.Status != tc.expectedStatus {
				t.Errorf("expected status %s, got %s", tc.expectedStatus, r.Status)
			}

			// Agent should be back to idle
			ag, _ := store.GetAgent(ctx, "agent-1")
			if ag.Status != agent.StatusIdle {
				t.Errorf("expected agent idle after finalization, got %s", ag.Status)
			}

			// Verify AG-UI run_finished event
			bc.mu.Lock()
			defer bc.mu.Unlock()
			foundFinished := false
			for _, ev := range bc.events {
				if ev.EventType == event.AGUIRunFinished {
					foundFinished = true
				}
			}
			if !foundFinished {
				t.Error("expected AG-UI run_finished broadcast")
			}
		})
	}
}

func TestHandleRunComplete_AlreadyCompletedRun(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// A run that is already completed should error from GetRun status checks
	// but finalizeRun still proceeds (the store accepts the update).
	// Actually, HandleRunComplete does not check r.Status before finalizing.
	// Let's verify it does not error.
	runID := "run-already-completed"
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            runID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "plan-readonly",
		Status:        run.StatusCompleted,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	payload := messagequeue.RunCompletePayload{
		RunID:     runID,
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "completed",
		Output:    "done again",
	}
	// HandleRunComplete should succeed (idempotent)
	if err := svc.HandleRunComplete(ctx, &payload); err != nil {
		t.Fatalf("HandleRunComplete on already completed run: %v", err)
	}
}

func TestHandleRunComplete_UnknownRunReturnsError(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	payload := messagequeue.RunCompletePayload{
		RunID:     "run-does-not-exist",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "completed",
	}
	err := svc.HandleRunComplete(ctx, &payload)
	if err == nil {
		t.Fatal("expected error for unknown run ID")
	}
}

func TestHandleRunComplete_WithQualityGates(t *testing.T) {
	svc, store, queue, bc := newRuntimeTestEnv()
	ctx := context.Background()

	// headless-safe-sandbox has quality gates (RequireTestsPass + RequireLintPass)
	runID := "run-quality-gate"
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            runID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	payload := messagequeue.RunCompletePayload{
		RunID:     runID,
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    "completed",
		StepCount: 10,
		CostUSD:   0.50,
		TokensIn:  1000,
		TokensOut: 500,
	}
	if err := svc.HandleRunComplete(ctx, &payload); err != nil {
		t.Fatalf("HandleRunComplete: %v", err)
	}

	// Run should be in quality_gate status, not completed
	r, _ := store.GetRun(ctx, runID)
	if r.Status != run.StatusQualityGate {
		t.Errorf("expected status quality_gate, got %s", r.Status)
	}

	// Quality gate request should be published on NATS
	msg, ok := queue.lastMessage(messagequeue.SubjectQualityGateRequest)
	if !ok {
		t.Fatal("expected quality gate request published on NATS")
	}
	var gateReq messagequeue.QualityGateRequestPayload
	if err := json.Unmarshal(msg.Data, &gateReq); err != nil {
		t.Fatalf("unmarshal quality gate request: %v", err)
	}
	if gateReq.RunID != runID {
		t.Errorf("expected run_id %q in gate request, got %q", runID, gateReq.RunID)
	}
	if !gateReq.RunTests {
		t.Error("expected RunTests=true in quality gate request")
	}
	if !gateReq.RunLint {
		t.Error("expected RunLint=true in quality gate request")
	}

	// Verify quality_gate broadcast events
	bc.mu.Lock()
	defer bc.mu.Unlock()
	foundGateStarted := false
	foundRunStatus := false
	for _, ev := range bc.events {
		if ev.EventType == event.EventQualityGate {
			if qe, ok := ev.Data.(event.QualityGateEvent); ok && qe.Status == "started" {
				foundGateStarted = true
			}
		}
		if ev.EventType == event.EventRunStatus {
			if se, ok := ev.Data.(event.RunStatusEvent); ok && se.Status == string(run.StatusQualityGate) {
				foundRunStatus = true
			}
		}
	}
	if !foundGateStarted {
		t.Error("expected quality_gate 'started' broadcast")
	}
	if !foundRunStatus {
		t.Error("expected run_status broadcast with quality_gate status")
	}
}

// ============================================================================
// HandleQualityGateResult tests
// ============================================================================

func TestHandleQualityGateResult_Passed(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	runID := "run-gate-passed"
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            runID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusQualityGate,
		CostUSD:       0.50,
		StepCount:     10,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	testsPassed := true
	lintPassed := true
	result := messagequeue.QualityGateResultPayload{
		RunID:       runID,
		TestsPassed: &testsPassed,
		LintPassed:  &lintPassed,
	}
	if err := svc.HandleQualityGateResult(ctx, &result); err != nil {
		t.Fatalf("HandleQualityGateResult: %v", err)
	}

	// Run should be completed
	r, _ := store.GetRun(ctx, runID)
	if r.Status != run.StatusCompleted {
		t.Errorf("expected completed after gate passed, got %s", r.Status)
	}

	// Agent should be idle
	ag, _ := store.GetAgent(ctx, "agent-1")
	if ag.Status != agent.StatusIdle {
		t.Errorf("expected agent idle, got %s", ag.Status)
	}

	// Verify quality gate "passed" broadcast
	bc.mu.Lock()
	defer bc.mu.Unlock()
	foundPassed := false
	for _, ev := range bc.events {
		if ev.EventType == event.EventQualityGate {
			if qe, ok := ev.Data.(event.QualityGateEvent); ok && qe.Status == "passed" {
				foundPassed = true
			}
		}
	}
	if !foundPassed {
		t.Error("expected quality_gate 'passed' broadcast")
	}
}

func TestHandleQualityGateResult_Failed(t *testing.T) {
	tests := []struct {
		name           string
		testsPassed    *bool
		lintPassed     *bool
		errMsg         string
		expectedStatus run.Status
		description    string
	}{
		{
			name:           "tests failed with rollback",
			testsPassed:    boolPtr(false),
			lintPassed:     boolPtr(true),
			errMsg:         "",
			expectedStatus: run.StatusFailed, // headless-safe-sandbox has RollbackOnGateFail=true
			description:    "test failure triggers rollback and sets run to failed",
		},
		{
			name:           "lint failed with rollback",
			testsPassed:    boolPtr(true),
			lintPassed:     boolPtr(false),
			errMsg:         "",
			expectedStatus: run.StatusFailed,
			description:    "lint failure triggers rollback and sets run to failed",
		},
		{
			name:           "error string triggers failure",
			testsPassed:    nil,
			lintPassed:     nil,
			errMsg:         "quality gate execution error",
			expectedStatus: run.StatusFailed,
			description:    "error field in result causes failure even without test/lint results",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, _, bc := newRuntimeTestEnv()
			ctx := context.Background()

			runID := "run-gate-fail-" + tc.name
			store.mu.Lock()
			store.runs = append(store.runs, run.Run{
				ID:            runID,
				TaskID:        "task-1",
				AgentID:       "agent-1",
				ProjectID:     "proj-1",
				PolicyProfile: "headless-safe-sandbox",
				Status:        run.StatusQualityGate,
				CostUSD:       0.50,
				StepCount:     10,
				StartedAt:     time.Now(),
			})
			store.mu.Unlock()

			result := messagequeue.QualityGateResultPayload{
				RunID:       runID,
				TestsPassed: tc.testsPassed,
				LintPassed:  tc.lintPassed,
				Error:       tc.errMsg,
			}
			if err := svc.HandleQualityGateResult(ctx, &result); err != nil {
				t.Fatalf("HandleQualityGateResult: %v", err)
			}

			r, _ := store.GetRun(ctx, runID)
			if r.Status != tc.expectedStatus {
				t.Errorf("expected status %s, got %s", tc.expectedStatus, r.Status)
			}

			// Task should be set to failed
			tsk, _ := store.GetTask(ctx, "task-1")
			if tsk.Status != task.StatusFailed {
				t.Errorf("expected task status failed, got %s", tsk.Status)
			}

			// Verify quality gate "failed" broadcast
			bc.mu.Lock()
			defer bc.mu.Unlock()
			foundFailed := false
			for _, ev := range bc.events {
				if ev.EventType == event.EventQualityGate {
					if qe, ok := ev.Data.(event.QualityGateEvent); ok && qe.Status == "failed" {
						foundFailed = true
					}
				}
			}
			if !foundFailed {
				t.Error("expected quality_gate 'failed' broadcast")
			}
		})
	}
}

func TestHandleQualityGateResult_NonGatedRunIsNoop(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// Run is running (not in quality_gate status) -- result should be ignored.
	runID := "run-not-gated"
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            runID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	testsPassed := true
	result := messagequeue.QualityGateResultPayload{
		RunID:       runID,
		TestsPassed: &testsPassed,
	}
	if err := svc.HandleQualityGateResult(ctx, &result); err != nil {
		t.Fatalf("HandleQualityGateResult: %v", err)
	}

	// Run status should remain unchanged (running)
	r, _ := store.GetRun(ctx, runID)
	if r.Status != run.StatusRunning {
		t.Errorf("expected status to remain running for non-gated run, got %s", r.Status)
	}
}

func TestHandleQualityGateResult_WithoutRollback(t *testing.T) {
	// headless-permissive-sandbox has RequireTestsPass but RollbackOnGateFail=false
	// So gate failure should result in StatusCompleted (not failed).
	policySvc := service.NewPolicyService("headless-permissive-sandbox", nil)
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
	runtimeCfg := config.Runtime{
		StallThreshold:       5,
		DefaultTestCommand:   "go test ./...",
		DefaultLintCommand:   "golangci-lint run ./...",
		DeliveryCommitPrefix: "codeforge:",
	}
	svc := service.NewRuntimeService(store, queue, bc, es, policySvc, &runtimeCfg)

	ctx := context.Background()

	runID := "run-gate-no-rollback"
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            runID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-permissive-sandbox",
		Status:        run.StatusQualityGate,
		CostUSD:       0.30,
		StepCount:     5,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	testsFailed := false
	result := messagequeue.QualityGateResultPayload{
		RunID:       runID,
		TestsPassed: &testsFailed,
	}
	if err := svc.HandleQualityGateResult(ctx, &result); err != nil {
		t.Fatalf("HandleQualityGateResult: %v", err)
	}

	// Without rollback, gate failure results in completed (not failed)
	r, _ := store.GetRun(ctx, runID)
	if r.Status != run.StatusCompleted {
		t.Errorf("expected completed (no rollback), got %s", r.Status)
	}
}

// ============================================================================
// HandleToolCallResult tests (non-budget-alert; budget alerts in
// runtime_execution_extended_test.go)
// ============================================================================

func TestHandleToolCallResult_AccumulatesCostAndTokens(t *testing.T) {
	svc, store, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	runID := "run-cost-acc"
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            runID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "trusted-mount-autonomous", // high budget (50.0)
		Status:        run.StatusRunning,
		CostUSD:       1.00,
		TokensIn:      100,
		TokensOut:     50,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	result := messagequeue.ToolCallResultPayload{
		RunID:     runID,
		CallID:    "call-cost-1",
		Tool:      "Read",
		Success:   true,
		Output:    "file contents",
		CostUSD:   0.25,
		TokensIn:  200,
		TokensOut: 100,
	}
	if err := svc.HandleToolCallResult(ctx, &result); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}

	r, _ := store.GetRun(ctx, runID)
	expectedCost := 1.25
	if r.CostUSD != expectedCost {
		t.Errorf("expected cost %.2f, got %.2f", expectedCost, r.CostUSD)
	}
	if r.TokensIn != 300 {
		t.Errorf("expected tokens_in 300, got %d", r.TokensIn)
	}
	if r.TokensOut != 150 {
		t.Errorf("expected tokens_out 150, got %d", r.TokensOut)
	}
}

func TestHandleToolCallResult_UnknownRunIsNoop(t *testing.T) {
	svc, _, _, _ := newRuntimeTestEnv()
	ctx := context.Background()

	// Conversation-based runs don't have a run record -- should return nil.
	result := messagequeue.ToolCallResultPayload{
		RunID:   "conv-unknown-xyz",
		CallID:  "call-conv-1",
		Tool:    "Read",
		Success: true,
		Output:  "ok",
		CostUSD: 0.01,
	}
	if err := svc.HandleToolCallResult(ctx, &result); err != nil {
		t.Fatalf("HandleToolCallResult for unknown run should not error, got: %v", err)
	}
}

func TestHandleToolCallResult_BroadcastsToolCallStatusAndAGUI(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	runID := "run-bc-events"
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            runID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "trusted-mount-autonomous",
		Status:        run.StatusRunning,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	result := messagequeue.ToolCallResultPayload{
		RunID:   runID,
		CallID:  "call-bc-1",
		Tool:    "Write",
		Success: true,
		Output:  "written",
		CostUSD: 0.01,
	}
	if err := svc.HandleToolCallResult(ctx, &result); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()

	foundToolStatus := false
	foundAGUIResult := false
	for _, ev := range bc.events {
		if ev.EventType == event.EventToolCallStatus {
			if tce, ok := ev.Data.(event.ToolCallStatusEvent); ok {
				if tce.Phase == "result" && tce.CallID == "call-bc-1" {
					foundToolStatus = true
				}
			}
		}
		if ev.EventType == event.AGUIToolResult {
			foundAGUIResult = true
		}
	}
	if !foundToolStatus {
		t.Error("expected tool_call_status broadcast with phase=result")
	}
	if !foundAGUIResult {
		t.Error("expected AG-UI tool_result broadcast")
	}
}

func TestHandleToolCallResult_PostExecutionBudgetExceeded(t *testing.T) {
	svc, store, _, bc := newRuntimeTestEnv()
	ctx := context.Background()

	// headless-safe-sandbox has MaxCost=5.0
	runID := "run-post-budget"
	store.mu.Lock()
	store.runs = append(store.runs, run.Run{
		ID:            runID,
		TaskID:        "task-1",
		AgentID:       "agent-1",
		ProjectID:     "proj-1",
		PolicyProfile: "headless-safe-sandbox",
		Status:        run.StatusRunning,
		CostUSD:       4.80,
		StartedAt:     time.Now(),
	})
	store.mu.Unlock()

	// This tool call pushes cost over the 5.0 budget
	result := messagequeue.ToolCallResultPayload{
		RunID:   runID,
		CallID:  "call-over-budget",
		Tool:    "Read",
		Success: true,
		Output:  "expensive result",
		CostUSD: 0.30, // total: 5.10 >= 5.0
	}
	if err := svc.HandleToolCallResult(ctx, &result); err != nil {
		t.Fatalf("HandleToolCallResult: %v", err)
	}

	// Run should be terminated with timeout status
	r, _ := store.GetRun(ctx, runID)
	if r.Status != run.StatusTimeout {
		t.Errorf("expected timeout from post-execution budget, got %s", r.Status)
	}

	// Agent should be idle
	ag, _ := store.GetAgent(ctx, "agent-1")
	if ag.Status != agent.StatusIdle {
		t.Errorf("expected agent idle after budget exceeded, got %s", ag.Status)
	}

	// Task should be failed
	tsk, _ := store.GetTask(ctx, "task-1")
	if tsk.Status != task.StatusFailed {
		t.Errorf("expected task failed after budget exceeded, got %s", tsk.Status)
	}

	// Verify run_status broadcast with timeout
	bc.mu.Lock()
	defer bc.mu.Unlock()
	foundTimeout := false
	for _, ev := range bc.events {
		if ev.EventType == event.EventRunStatus {
			if se, ok := ev.Data.(event.RunStatusEvent); ok && se.Status == string(run.StatusTimeout) {
				foundTimeout = true
			}
		}
	}
	if !foundTimeout {
		t.Error("expected run_status broadcast with timeout after budget exceeded")
	}
}

// ============================================================================
// Cancelled conversation fast-reject (HITL)
// ============================================================================

func TestHandleConversationToolCall_Cancelled_Denies(t *testing.T) {
	svc, _, queue, _ := newRuntimeTestEnv()
	ctx := context.Background()

	convID := "conv-cancelled-1"

	// Mark the conversation as cancelled before sending a tool call.
	svc.MarkConversationRunCancelled(convID)

	// Send a tool call request using the conversation ID as run_id.
	// Since no run record exists for this ID, HandleToolCallRequest falls
	// through to handleConversationToolCall, which should fast-reject.
	req := messagequeue.ToolCallRequestPayload{
		RunID:  convID,
		CallID: "call-cancelled-1",
		Tool:   "Read",
		Path:   "main.go",
	}
	if err := svc.HandleToolCallRequest(ctx, &req); err != nil {
		t.Fatalf("HandleToolCallRequest: %v", err)
	}

	// Verify NATS response is deny with correct reason.
	msg, ok := queue.lastMessage(messagequeue.SubjectRunToolCallResponse)
	if !ok {
		t.Fatal("expected tool call response on NATS")
	}
	var resp messagequeue.ToolCallResponsePayload
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Decision != "deny" {
		t.Errorf("expected deny for cancelled conversation, got %q", resp.Decision)
	}
	if resp.Reason != "conversation run cancelled" {
		t.Errorf("expected reason %q, got %q", "conversation run cancelled", resp.Reason)
	}
	if resp.CallID != req.CallID {
		t.Errorf("expected call_id %q, got %q", req.CallID, resp.CallID)
	}
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}
