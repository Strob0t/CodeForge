package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/run"
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
		// cleanupRunState delegates to state.CleanupRun which handles budget alert cleanup.
		if !strings.Contains(content, "CleanupRun") {
			t.Error("cleanupRunState should delegate to state.CleanupRun for resource cleanup")
		}
	})

	t.Run("ApprovalChannelCleanup", func(t *testing.T) {
		// cleanupRunState delegates to state.CleanupRun which handles approval channel cleanup.
		if !strings.Contains(content, "CleanupRun") {
			t.Error("cleanupRunState should delegate to state.CleanupRun for approval channel cleanup")
		}
	})
}

// lifecycleTestStore extends mockStore with run tracking for lifecycle tests.
type lifecycleTestStore struct {
	mockStore
	runs          map[string]*run.Run
	completedRuns map[string]run.Status
}

func newLifecycleTestStore() *lifecycleTestStore {
	return &lifecycleTestStore{
		runs:          make(map[string]*run.Run),
		completedRuns: make(map[string]run.Status),
	}
}

func (s *lifecycleTestStore) GetRun(_ context.Context, id string) (*run.Run, error) {
	if r, ok := s.runs[id]; ok {
		return r, nil
	}
	return nil, domain.ErrNotFound
}

func (s *lifecycleTestStore) CompleteRun(_ context.Context, req *run.CompletionRequest) error {
	s.completedRuns[req.ID] = req.Status
	return nil
}

func (s *lifecycleTestStore) UpdateRunStatus(_ context.Context, _ string, _ run.Status, _ int, _ float64, _, _ int64) error {
	return nil
}

// Tests below supplement runtime_internal_test.go which already covers
// TestCleanupRunState_AllFields, TestCleanupRunState_OtherRunsUnaffected,
// and all TestCheckTermination_* variants.

func TestCleanupRunState_ReleasesAllResources(t *testing.T) {
	svc := &RuntimeService{
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}

	runID := "run-lifecycle-cleanup"

	// Populate all resource types.
	svc.state.SetHeartbeat(runID, time.Now())
	svc.state.SetStallTracker(runID, run.NewStallTracker(5, 2))

	cancelCalled := false
	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx
	svc.state.SetRunTimeout(runID, context.CancelFunc(func() {
		cancelCalled = true
		cancel()
	}))
	svc.state.StoreBudgetAlert(runID + ":80")
	svc.state.StoreBudgetAlert(runID + ":90")

	// Add pending approval channels.
	ch1 := make(chan string, 1)
	ch2 := make(chan string, 1)
	svc.state.SetPendingApproval(runID+":call-a", ch1)
	svc.state.SetPendingApproval(runID+":call-b", ch2)

	svc.cleanupRunState(runID)

	// Verify all resources cleaned.
	if _, ok := svc.state.GetHeartbeat(runID); ok {
		t.Error("heartbeat not cleaned up")
	}
	if _, ok := svc.state.GetStallTracker(runID); ok {
		t.Error("stall tracker not cleaned up")
	}
	if _, ok := svc.state.LoadAndDeleteRunTimeout(runID); ok {
		t.Error("run timeout cancel not cleaned up")
	}
	if !cancelCalled {
		t.Error("timeout cancel function not called")
	}
	if alreadySent := svc.state.StoreBudgetAlert(runID + ":80"); alreadySent {
		t.Error("budget alert 80% not cleaned up")
	}
	svc.state.DeleteBudgetAlert(runID + ":80")
	if alreadySent := svc.state.StoreBudgetAlert(runID + ":90"); alreadySent {
		t.Error("budget alert 90% not cleaned up")
	}
	svc.state.DeleteBudgetAlert(runID + ":90")
	if _, ok := svc.state.LoadAndDeletePendingApproval(runID + ":call-a"); ok {
		t.Error("pending approval call-a not cleaned up")
	}
	if _, ok := svc.state.LoadAndDeletePendingApproval(runID + ":call-b"); ok {
		t.Error("pending approval call-b not cleaned up")
	}

	// Both channels should have received "deny".
	for _, ch := range []chan string{ch1, ch2} {
		select {
		case msg := <-ch:
			if msg != "deny" {
				t.Errorf("expected 'deny' on approval channel, got %q", msg)
			}
		default:
			t.Error("expected 'deny' message on approval channel")
		}
	}
}

func TestCancelRunWithReason_UpdatesStatus(t *testing.T) {
	store := newLifecycleTestStore()
	store.runs["run-cancel"] = &run.Run{
		ID:        "run-cancel",
		TaskID:    "task-1",
		AgentID:   "agent-1",
		ProjectID: "proj-1",
		Status:    run.StatusRunning,
		StartedAt: time.Now(),
	}

	q := &internalMockQueue{}
	bc := &internalMockBroadcaster{}
	es := &mockEventStore{}

	svc := &RuntimeService{
		store:      store,
		queue:      q,
		hub:        bc,
		events:     es,
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}

	err := svc.cancelRunWithReason(context.Background(), "run-cancel", "test timeout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify run was completed with timeout status.
	if got, ok := store.completedRuns["run-cancel"]; !ok || got != run.StatusTimeout {
		t.Errorf("expected run status 'timeout', got %q (found=%v)", got, ok)
	}

	// Verify a cancel message was published to NATS.
	q.mu.Lock()
	defer q.mu.Unlock()
	found := false
	for _, msg := range q.messages {
		if msg.subject == "runs.cancel" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected cancel message published to NATS")
	}

	// Verify WS broadcast was sent.
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if len(bc.events) == 0 {
		t.Error("expected at least one WS broadcast event")
	}
}

func TestCancelRunWithReason_AlreadyCompleted(t *testing.T) {
	store := newLifecycleTestStore()
	store.runs["run-done"] = &run.Run{
		ID:        "run-done",
		Status:    run.StatusCompleted,
		StartedAt: time.Now(),
	}

	svc := &RuntimeService{
		store:      store,
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}

	// Should not error for already-completed runs.
	err := svc.cancelRunWithReason(context.Background(), "run-done", "test")
	if err != nil {
		t.Fatalf("expected no error for already-completed run, got: %v", err)
	}

	// Should NOT have been completed again.
	if _, ok := store.completedRuns["run-done"]; ok {
		t.Error("expected already-completed run to be skipped")
	}
}

func TestCancelRunWithReason_NotFound(t *testing.T) {
	store := newLifecycleTestStore()
	svc := &RuntimeService{
		store:      store,
		runtimeCfg: &config.Runtime{},
		state:      NewRunStateManager(),
	}

	err := svc.cancelRunWithReason(context.Background(), "nonexistent", "test")
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
}

// NOTE(FIX-032): TestFinalizeRun_PublishesEvent requires a full RuntimeService
// with a store pre-populated with run, task, agent, and project records,
// plus event store, checkpoint service, and sandbox service. This is an
// integration test.

// NOTE(FIX-032): TestTriggerDelivery_SendsNATSMessage requires a
// DeliverService mock and full run lifecycle setup. This is an integration test.
