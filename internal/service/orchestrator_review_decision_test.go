package service_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A step's review decision is applied even when its plan cannot be read at
// once (S2-F review, F8). decideReview dropped the decision when it could
// not read the plan, and nothing advanced the plan again: the step stayed
// pending and the plan running. Decisions of steps that left pending are
// forgotten instead of being kept for a later start.

// flakyPlanStore fails the next failGetPlan plan reads.
type flakyPlanStore struct {
	*orchMockStore
	failGetPlan atomic.Int32
}

func (s *flakyPlanStore) GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error) {
	if s.failGetPlan.Add(-1) >= 0 {
		return nil, errors.New("database unavailable")
	}
	s.failGetPlan.Store(0)
	return s.orchMockStore.GetPlan(ctx, id)
}

func newFlakyReviewSetup(router *blockingReviewLLM) (*flakyPlanStore, *service.OrchestratorService) {
	store := &flakyPlanStore{orchMockStore: newOrchStore()}
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	runtimeSvc := service.NewRuntimeService(store, &runtimeMockQueue{}, bc, es,
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5})
	cfg := &config.Orchestrator{
		MaxParallel: 4, PingPongMaxRounds: 3, DebateRounds: 1,
		ReviewRouterEnabled: true, ReviewConfidenceThreshold: 0.7, ReviewRouterModel: "test-model",
	}
	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, cfg)
	runtimeSvc.SetOnRunComplete(orchSvc.HandleRunCompleted)
	orchSvc.SetReviewRouter(service.NewReviewRouterService(router, cfg, &config.Limits{MaxInputLen: 10000}))
	return store, orchSvc
}

// startReviewedPlan starts a one-step sequential plan and waits until its
// step's review asks the router.
func startReviewedPlan(t *testing.T, router *blockingReviewLLM, orchSvc *service.OrchestratorService) *plan.ExecutionPlan {
	t.Helper()
	ctx := context.Background()
	p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
		Name: "reviewed", ProjectID: "proj-1", Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	within(t, "StartPlan", func() {
		if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
			t.Errorf("StartPlan: %v", err)
		}
	})
	select {
	case <-router.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the review router was not asked")
	}
	return p
}

func TestReviewDecision_AppliedAfterThePlanCouldNotBeRead(t *testing.T) {
	router := newBlockingReviewLLM()
	store, orchSvc := newFlakyReviewSetup(router)
	p := startReviewedPlan(t, router, orchSvc)

	store.failGetPlan.Store(2)
	close(router.release)

	if debate := debatePlanOf(t, store.orchMockStore, p.ID); debate.Status != plan.StatusRunning {
		t.Fatalf("debate = %s, want running: the decision was applied", debate.Status)
	}
}

func TestReviewDecision_ForgottenWhenItsStepLeftPending(t *testing.T) {
	router := newBlockingReviewLLM()
	store, orchSvc := newFlakyReviewSetup(router)
	p := startReviewedPlan(t, router, orchSvc)

	// The plan cannot be read while the decision is applied: it is kept
	// for the plan's next advance.
	store.failGetPlan.Store(1 << 20)
	close(router.release)
	within(t, "the review", orchSvc.WaitForReviews)
	if n := orchSvc.ReviewDecisionCount(); n != 1 {
		t.Fatalf("review decisions = %d, want the one kept", n)
	}
	store.failGetPlan.Store(0)

	if err := orchSvc.CancelPlan(context.Background(), p.ID); err != nil {
		t.Fatalf("CancelPlan: %v", err)
	}
	if n := orchSvc.ReviewDecisionCount(); n != 0 {
		t.Fatalf("review decisions after the plan was cancelled = %d, want 0", n)
	}
}
