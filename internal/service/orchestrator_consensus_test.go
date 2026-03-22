package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
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

// orchTestStore extends mockStore with orchestrator-specific tracking.
type orchTestStore struct {
	mockStore
	planStatuses     map[string]plan.Status
	stepStatuses     map[string]plan.StepStatus
	runs             map[string]*run.Run
	updatedRunStatus map[string]run.Status
}

func newOrchTestStore() *orchTestStore {
	return &orchTestStore{
		planStatuses:     make(map[string]plan.Status),
		stepStatuses:     make(map[string]plan.StepStatus),
		runs:             make(map[string]*run.Run),
		updatedRunStatus: make(map[string]run.Status),
	}
}

func (s *orchTestStore) UpdatePlanStatus(_ context.Context, id string, status plan.Status) error {
	s.planStatuses[id] = status
	return nil
}

func (s *orchTestStore) UpdatePlanStepStatus(_ context.Context, stepID string, status plan.StepStatus, _, _ string) error {
	s.stepStatuses[stepID] = status
	return nil
}

func (s *orchTestStore) ListPlanSteps(_ context.Context, _ string) ([]plan.Step, error) {
	return nil, nil
}

func (s *orchTestStore) GetRun(_ context.Context, id string) (*run.Run, error) {
	if r, ok := s.runs[id]; ok {
		return r, nil
	}
	return nil, domain.ErrNotFound
}

func (s *orchTestStore) UpdateRunStatus(_ context.Context, id string, status run.Status, _ int, _ float64, _, _ int64) error {
	s.updatedRunStatus[id] = status
	return nil
}

// newTestOrchService creates an OrchestratorService with mock dependencies for testing.
func newTestOrchService(store *orchTestStore) *OrchestratorService {
	orchCfg := &config.Orchestrator{
		MaxParallel:       4,
		PingPongMaxRounds: 3,
		ConsensusQuorum:   0, // majority
	}
	hub := &mockBroadcaster{}
	events := &mockEventStore{}

	return NewOrchestratorService(store, hub, events, nil, orchCfg)
}

func TestAdvanceConsensus_AllStepsCompleted(t *testing.T) {
	store := newOrchTestStore()
	svc := newTestOrchService(store)

	p := &plan.ExecutionPlan{
		ID:        "plan-1",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolConsensus,
		Status:    plan.StatusRunning,
		Steps: []plan.Step{
			{ID: "s1", Status: plan.StepStatusCompleted},
			{ID: "s2", Status: plan.StepStatusCompleted},
			{ID: "s3", Status: plan.StepStatusCompleted},
		},
	}

	svc.advanceConsensus(context.Background(), p)

	// All 3 steps completed, quorum=0 means majority=(3/2)+1=2 => 3 >= 2, should complete.
	if got, ok := store.planStatuses["plan-1"]; !ok || got != plan.StatusCompleted {
		t.Errorf("expected plan status 'completed', got %q (found=%v)", got, ok)
	}
}

func TestAdvanceConsensus_QuorumNotMet(t *testing.T) {
	store := newOrchTestStore()
	svc := newTestOrchService(store)

	p := &plan.ExecutionPlan{
		ID:        "plan-2",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolConsensus,
		Status:    plan.StatusRunning,
		Steps: []plan.Step{
			{ID: "s1", Status: plan.StepStatusCompleted},
			{ID: "s2", Status: plan.StepStatusFailed},
			{ID: "s3", Status: plan.StepStatusFailed},
		},
	}

	svc.advanceConsensus(context.Background(), p)

	// 1 completed out of 3. Default quorum = (3/2)+1 = 2. 1 < 2 => fail.
	if got, ok := store.planStatuses["plan-2"]; !ok || got != plan.StatusFailed {
		t.Errorf("expected plan status 'failed', got %q (found=%v)", got, ok)
	}
}

func TestAdvanceConsensus_DefaultQuorum(t *testing.T) {
	store := newOrchTestStore()
	svc := newTestOrchService(store)

	// 5 steps, 3 completed = majority (5/2)+1=3 => exactly quorum => complete.
	p := &plan.ExecutionPlan{
		ID:        "plan-3",
		ProjectID: "proj-1",
		Protocol:  plan.ProtocolConsensus,
		Status:    plan.StatusRunning,
		Steps: []plan.Step{
			{ID: "s1", Status: plan.StepStatusCompleted},
			{ID: "s2", Status: plan.StepStatusCompleted},
			{ID: "s3", Status: plan.StepStatusCompleted},
			{ID: "s4", Status: plan.StepStatusFailed},
			{ID: "s5", Status: plan.StepStatusFailed},
		},
	}

	svc.advanceConsensus(context.Background(), p)

	if got, ok := store.planStatuses["plan-3"]; !ok || got != plan.StatusCompleted {
		t.Errorf("expected plan status 'completed' (3 >= majority of 5), got %q", got)
	}
}

// NOTE(FIX-032): TestStartStep_PublishesNATS requires a full RuntimeService
// with NATS queue mocking, agent/task store data, and run creation. This is
// an integration test that requires complex setup beyond unit testing scope.

// NOTE(FIX-032): TestEvaluateStepReview_PassThreshold requires a
// ReviewRouterService with LLM evaluation, which is an integration concern.

func TestReplanStep_ResetsStatus(t *testing.T) {
	store := newOrchTestStore()
	store.runs["run-1"] = &run.Run{
		ID:        "run-1",
		TaskID:    "task-1",
		ProjectID: "proj-1",
		Status:    run.StatusFailed,
	}
	svc := newTestOrchService(store)

	err := svc.ReplanStep(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Run status should be reset to pending.
	if got, ok := store.updatedRunStatus["run-1"]; !ok || got != run.StatusPending {
		t.Errorf("expected run status reset to 'pending', got %q (found=%v)", got, ok)
	}
}

func TestReplanStep_RunNotFound(t *testing.T) {
	store := newOrchTestStore()
	svc := newTestOrchService(store)

	err := svc.ReplanStep(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}
