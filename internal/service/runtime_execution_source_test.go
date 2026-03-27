package service

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// --------------------------------------------------------------------------
// TestRuntimeExecution_SourceQuality (FIX-032)
// --------------------------------------------------------------------------

func TestRuntimeExecution_SourceQuality(t *testing.T) {
	src, err := os.ReadFile("runtime_execution.go") //nolint:gosec // test reads known source
	if err != nil {
		t.Fatalf("failed to read runtime_execution.go: %v", err)
	}
	content := string(src)

	t.Run("ProperErrorHandling", func(t *testing.T) {
		errAssignments := strings.Count(content, "if err != nil")
		if errAssignments < 5 {
			t.Errorf("expected at least 5 error checks, got %d", errAssignments)
		}
	})

	t.Run("NoRawPanic", func(t *testing.T) {
		if strings.Contains(content, "panic(") {
			t.Error("runtime_execution.go should not use panic() -- use error returns")
		}
	})

	t.Run("ContextPropagation", func(t *testing.T) {
		methods := []string{
			"HandleToolCallRequest",
			"HandleToolCallResult",
			"HandleRunComplete",
			"HandleQualityGateResult",
		}
		for _, m := range methods {
			if !strings.Contains(content, m+"(ctx context.Context") {
				t.Errorf("%s must accept ctx context.Context as first parameter", m)
			}
		}
	})

	t.Run("LoggingOnErrors", func(t *testing.T) {
		if !strings.Contains(content, "slog.Error") && !strings.Contains(content, "slog.Warn") {
			t.Error("runtime_execution.go should log errors via slog")
		}
	})
}

// Tests below supplement the existing runtime_internal_test.go tests.
// Duplicate test names are avoided; see runtime_internal_test.go for
// TestCheckTermination_*, TestIsFileModifyingTool, TestToContextEntryPayloads,
// TestCleanupRunState_*, TestSendToolCallResponse_*, TestWaitForApproval_*.

func TestCheckTermination_BudgetExceeded(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}
	r := &run.Run{
		ID:        "run-budget",
		CostUSD:   5.01,
		StartedAt: time.Now(),
	}
	profile := &policy.PolicyProfile{
		Termination: policy.TerminationCondition{
			MaxCost: 5.0,
		},
	}

	reason := svc.checkTermination(r, profile)
	if reason == "" {
		t.Fatal("expected termination reason when cost exceeds budget")
	}
	if !strings.Contains(reason, "max cost") {
		t.Errorf("expected 'max cost' in reason, got: %s", reason)
	}
}

func TestCheckTermination_NoTermination(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}
	r := &run.Run{
		ID:        "run-ok",
		StepCount: 5,
		CostUSD:   0.10,
		StartedAt: time.Now(),
	}
	profile := &policy.PolicyProfile{
		Termination: policy.TerminationCondition{
			MaxSteps: 100,
			MaxCost:  10.0,
		},
	}

	reason := svc.checkTermination(r, profile)
	if reason != "" {
		t.Errorf("expected no termination, got reason: %s", reason)
	}
}

func TestCheckTermination_CombinedLimits(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}

	// Steps OK, cost exceeded => should terminate on cost.
	r := &run.Run{
		ID:        "run-combo",
		StepCount: 5,
		CostUSD:   10.1,
		StartedAt: time.Now(),
	}
	profile := &policy.PolicyProfile{
		Termination: policy.TerminationCondition{
			MaxSteps: 100,
			MaxCost:  10.0,
		},
	}

	reason := svc.checkTermination(r, profile)
	if reason == "" {
		t.Fatal("expected termination on cost limit")
	}
	if !strings.Contains(reason, "max cost") {
		t.Errorf("expected 'max cost' in reason, got: %s", reason)
	}
}

func TestMarkConversationRunCancelled(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}

	svc.MarkConversationRunCancelled("conv-1")

	if !svc.state.IsConversationCancelled("conv-1") {
		t.Error("expected conv-1 to be marked as cancelled")
	}
}

func TestBypassConversationApprovals(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}

	if svc.IsConversationBypassed("conv-1") {
		t.Error("expected conv-1 to not be bypassed initially")
	}

	svc.BypassConversationApprovals("conv-1")

	if !svc.IsConversationBypassed("conv-1") {
		t.Error("expected conv-1 to be bypassed after calling BypassConversationApprovals")
	}
}

// NOTE(FIX-032): TestHandleToolCallRequest_PolicyDenied,
// TestHandleToolCallRequest_HITLApproval, TestHandleToolCallResult_UpdatesRunState,
// TestHandleRunComplete_TriggersDelivery, and TestHandleQualityGateResult_PassFail
// require a full RuntimeService with PolicyService, mock store with run records,
// mock queue, and event store. These are integration tests that go beyond
// unit-testable scope. See runtime_internal_test.go for the closest unit tests.
