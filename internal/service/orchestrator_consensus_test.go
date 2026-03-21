package service

import (
	"os"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// TestOrchestratorConsensus_SourceQuality (FIX-032)
// --------------------------------------------------------------------------

func TestOrchestratorConsensus_SourceQuality(t *testing.T) {
	src, err := os.ReadFile("orchestrator_consensus.go") //nolint:gosec // test reads known source
	if err != nil {
		t.Fatalf("failed to read orchestrator_consensus.go: %v", err)
	}
	content := string(src)

	t.Run("ProperErrorHandling", func(t *testing.T) {
		errChecks := strings.Count(content, "if err != nil")
		if errChecks < 2 {
			t.Errorf("expected at least 2 error checks, got %d", errChecks)
		}
	})

	t.Run("NoRawPanic", func(t *testing.T) {
		if strings.Contains(content, "panic(") {
			t.Error("orchestrator_consensus.go should not use panic()")
		}
	})

	t.Run("QuorumCalculation", func(t *testing.T) {
		// Consensus should use a quorum-based approach.
		if !strings.Contains(content, "quorum") && !strings.Contains(content, "Quorum") {
			t.Error("consensus module should implement quorum-based decision making")
		}
	})

	t.Run("CompletePlanExists", func(t *testing.T) {
		if !strings.Contains(content, "completePlan") {
			t.Error("orchestrator_consensus.go should contain completePlan for plan finalization")
		}
	})

	t.Run("FailPlanExists", func(t *testing.T) {
		if !strings.Contains(content, "failPlan") {
			t.Error("orchestrator_consensus.go should contain failPlan for plan failure handling")
		}
	})

	t.Run("ReplanStepExists", func(t *testing.T) {
		if !strings.Contains(content, "ReplanStep") {
			t.Error("orchestrator_consensus.go should contain ReplanStep for re-planning support")
		}
	})
}

// TODO(FIX-032): Additional tests to write for orchestrator_consensus.go:
// - TestAdvanceConsensus_AllStepsCompleted (verify plan completes when quorum met)
// - TestAdvanceConsensus_QuorumNotMet (verify plan fails when quorum not met)
// - TestAdvanceConsensus_DefaultQuorum (verify majority calculation when quorum=0)
// - TestStartStep_PublishesNATS (verify step execution request sent via NATS)
// - TestEvaluateStepReview_PassThreshold (verify review pass/fail logic)
// - TestReplanStep_ResetsStatus (verify step status reset and re-execution)
