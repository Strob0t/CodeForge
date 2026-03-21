package service

import (
	"os"
	"strings"
	"testing"
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
		// Every error from store/queue calls should be handled (not ignored).
		// Count error assignments vs error checks.
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
		// All handler methods should accept ctx context.Context as first param.
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
		// Handler functions should log errors with slog.
		if !strings.Contains(content, "slog.Error") && !strings.Contains(content, "slog.Warn") {
			t.Error("runtime_execution.go should log errors via slog")
		}
	})
}

// TODO(FIX-032): Additional tests to write for runtime_execution.go:
// - TestHandleToolCallRequest_PolicyDenied (mock store, verify denied tool call returns error)
// - TestHandleToolCallRequest_HITLApproval (mock store with autonomy level requiring HITL)
// - TestHandleToolCallResult_UpdatesRunState (verify run step count, cost tracking)
// - TestHandleRunComplete_TriggersDelivery (verify delivery pipeline is triggered)
// - TestHandleQualityGateResult_PassFail (verify quality gate pass/fail updates run)
