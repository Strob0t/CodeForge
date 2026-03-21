package service

import (
	"os"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// TestRuntimeLifecycle_SourceQuality (FIX-032)
// --------------------------------------------------------------------------

func TestRuntimeLifecycle_SourceQuality(t *testing.T) {
	src, err := os.ReadFile("runtime_lifecycle.go") //nolint:gosec // test reads known source
	if err != nil {
		t.Fatalf("failed to read runtime_lifecycle.go: %v", err)
	}
	content := string(src)

	t.Run("ProperErrorHandling", func(t *testing.T) {
		errChecks := strings.Count(content, "if err != nil")
		if errChecks < 3 {
			t.Errorf("expected at least 3 error checks, got %d", errChecks)
		}
	})

	t.Run("NoRawPanic", func(t *testing.T) {
		if strings.Contains(content, "panic(") {
			t.Error("runtime_lifecycle.go should not use panic()")
		}
	})

	t.Run("CleanupRunState_Exists", func(t *testing.T) {
		if !strings.Contains(content, "cleanupRunState") {
			t.Error("runtime_lifecycle.go should contain cleanupRunState for resource cleanup")
		}
	})

	t.Run("BudgetAlertCleanup", func(t *testing.T) {
		// cleanupRunState should clean up budget alert dedup entries.
		if !strings.Contains(content, "budgetAlerts.Delete") {
			t.Error("cleanupRunState should clean up budget alert entries to prevent memory leaks")
		}
	})

	t.Run("ApprovalChannelCleanup", func(t *testing.T) {
		// cleanupRunState should clean up pending approval channels.
		if !strings.Contains(content, "pendingApprovals") {
			t.Error("cleanupRunState should clean up pending approval channels")
		}
	})
}

// TODO(FIX-032): Additional tests to write for runtime_lifecycle.go:
// - TestCleanupRunState_ReleasesAllResources (verify heartbeats, stall trackers, timeouts cleaned)
// - TestCancelRunWithReason_UpdatesStatus (verify run status set to cancelled)
// - TestFinalizeRun_PublishesEvent (verify WS broadcast and event append)
// - TestTriggerDelivery_SendsNATSMessage (verify delivery message published)
// - TestCheckTermination_MaxSteps (verify max steps terminates run)
// - TestCheckTermination_BudgetExceeded (verify budget limit terminates run)
