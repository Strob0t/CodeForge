package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/llm"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A step the review router sends to a debate starts the debate sub-plan and,
// when the debate ends, its own run, all under the one scheduling lock
// without taking it twice (review 2, finding 11: the debate path deadlocked
// in advancePlan, handleDebateComplete and ReplanStep).

// needsReviewLLM answers every review router prompt with "needs review".
type needsReviewLLM struct{}

func needsReview() *llm.ChatCompletionResponse {
	return &llm.ChatCompletionResponse{Content: `{"needs_review": true, "confidence": 0.2, "reason": "risky", "suggested_reviewers": []}`}
}

func (needsReviewLLM) ChatCompletion(context.Context, llm.ChatCompletionRequest) (*llm.ChatCompletionResponse, error) {
	return needsReview(), nil
}

func (needsReviewLLM) ChatCompletionStream(context.Context, llm.ChatCompletionRequest, func(llm.StreamChunk)) (*llm.ChatCompletionResponse, error) {
	return needsReview(), nil
}
func (needsReviewLLM) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }
func (needsReviewLLM) Health(context.Context) (bool, error)            { return true, nil }
func (needsReviewLLM) HealthDetailed(context.Context) (*llm.HealthStatusReport, error) {
	return &llm.HealthStatusReport{}, nil
}

// newDebateSetup is newOrchRuntimeSetup with a review router that sends every
// step to a debate while cfg.ReviewRouterEnabled is set.
func newDebateSetup(routerEnabled bool) (*orchMockStore, *service.OrchestratorService, *config.Orchestrator) {
	return newDebateSetupWithLLM(routerEnabled, needsReviewLLM{})
}

// newDebateSetupWithLLM is newDebateSetup with the review router's LLM.
func newDebateSetupWithLLM(routerEnabled bool, provider llm.Provider) (*orchMockStore, *service.OrchestratorService, *config.Orchestrator) {
	store := newOrchStore()
	bc := &runtimeMockBroadcaster{}
	es := &runtimeMockEventStore{}
	runtimeSvc := service.NewRuntimeService(store, &runtimeMockQueue{}, bc, es,
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{StallThreshold: 5})
	cfg := &config.Orchestrator{
		MaxParallel: 4, PingPongMaxRounds: 3, DebateRounds: 1,
		ReviewRouterEnabled: routerEnabled, ReviewConfidenceThreshold: 0.7, ReviewRouterModel: "test-model",
	}
	orchSvc := service.NewOrchestratorService(store, bc, es, runtimeSvc, cfg)
	runtimeSvc.SetOnRunComplete(orchSvc.HandleRunCompleted)
	orchSvc.SetReviewRouter(service.NewReviewRouterService(provider, cfg, &config.Limits{MaxInputLen: 10000}))
	return store, orchSvc, cfg
}

// within fails the test when fn does not return in time (a deadlock).
func within(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not return: deadlock", what)
	}
}

// debatePlanOf waits until the debate of a step of plan parentID runs its
// first step and returns the debate plan. The review router decides outside
// the scheduling lock, so the debate starts after StartPlan returned.
func debatePlanOf(t *testing.T, store *orchMockStore, parentID string) *plan.ExecutionPlan {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		store.mu.Lock()
		var id string
		for i := range store.plans {
			if strings.HasPrefix(store.plans[i].Name, "debate:"+parentID+":") {
				id = store.plans[i].ID
			}
		}
		store.mu.Unlock()
		if id != "" {
			if p := planState(t, store, id); p.Steps[0].Status == plan.StepStatusRunning && p.Steps[0].RunID != "" {
				return p
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no running debate for %s", parentID)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func completeStepRun(t *testing.T, store *orchMockStore, orchSvc *service.OrchestratorService, planID string, i int) {
	t.Helper()
	step := planState(t, store, planID).Steps[i]
	if step.Status != plan.StepStatusRunning || step.RunID == "" {
		t.Fatalf("step %d of %s is %s with run %q, want running", i, planID, step.Status, step.RunID)
	}
	if err := store.CompleteRun(context.Background(), &run.CompletionRequest{ID: step.RunID, Status: run.StatusCompleted, Output: "synthesis"}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	within(t, "HandleRunCompleted", func() { orchSvc.HandleRunCompleted(context.Background(), step.RunID, run.StatusCompleted) })
}

func TestDebate_RoutedStepRunsAfterItsDebate(t *testing.T) {
	store, orchSvc, _ := newDebateSetup(true)
	ctx := context.Background()
	p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
		Name: "routed", ProjectID: "proj-1", Protocol: plan.ProtocolSequential,
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

	debate := debatePlanOf(t, store, p.ID)
	if debate.Status != plan.StatusRunning || debate.Protocol != plan.ProtocolPingPong {
		t.Fatalf("debate = %s %s, want a running ping_pong plan", debate.Status, debate.Protocol)
	}
	if st := planState(t, store, p.ID).Steps[0]; st.Status != plan.StepStatusRunning || st.RunID != "" {
		t.Fatalf("parent step = %s with run %q, want running without a run while debating", st.Status, st.RunID)
	}

	// Proponent, then moderator: the debate completes and the step runs.
	completeStepRun(t, store, orchSvc, debate.ID, 0)
	completeStepRun(t, store, orchSvc, debate.ID, 1)
	if got := planState(t, store, debate.ID); got.Status != plan.StatusCompleted {
		t.Fatalf("debate = %s, want completed", got.Status)
	}
	completeStepRun(t, store, orchSvc, p.ID, 0)
	if got := planState(t, store, p.ID); got.Status != plan.StatusCompleted {
		t.Errorf("plan = %s, want completed", got.Status)
	}
}

func TestReplanStep_RoutedToADebate(t *testing.T) {
	store, orchSvc, cfg := newDebateSetup(false)
	ctx := context.Background()
	p := createPlan(t, orchSvc, plan.ProtocolParallel, 0, []plan.CreateStepRequest{
		{TaskID: "t1", AgentID: "a1"}, {TaskID: "t2", AgentID: "a2"},
	})
	failed := failStep(t, store, orchSvc, p.ID, 0, run.StatusFailed)

	cfg.ReviewRouterEnabled = true
	within(t, "ReplanStep", func() {
		if err := orchSvc.ReplanStep(ctx, failed); err != nil {
			t.Errorf("ReplanStep: %v", err)
		}
	})
	if debate := debatePlanOf(t, store, p.ID); debate.Status != plan.StatusRunning {
		t.Errorf("debate = %s, want running", debate.Status)
	}
}

// TestDebate_StepsRunInTheirModes: a plan step's mode reaches its run
// (KI-76): CreatePlan dropped it, so the debate's proponent and moderator
// ran without their modes.
func TestDebate_StepsRunInTheirModes(t *testing.T) {
	store, orchSvc, _ := newDebateSetup(true)
	ctx := context.Background()
	p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
		Name: "routed", ProjectID: "proj-1", Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1", ModeID: "coder"}},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if got := planState(t, store, p.ID).Steps[0].ModeID; got != "coder" {
		t.Fatalf("stored step mode = %q, want coder", got)
	}
	within(t, "StartPlan", func() {
		if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
			t.Errorf("StartPlan: %v", err)
		}
	})

	debate := debatePlanOf(t, store, p.ID)
	for i, want := range []string{"proponent", "moderator"} {
		if got := debate.Steps[i].ModeID; got != want {
			t.Errorf("debate step %d mode = %q, want %q", i, got, want)
		}
	}
	r, err := store.GetRun(ctx, debate.Steps[0].RunID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if r.ModeID != "proponent" {
		t.Errorf("proponent run mode = %q, want proponent", r.ModeID)
	}
}

// blockingReviewLLM answers the review router only when released.
type blockingReviewLLM struct {
	entered chan struct{}
	release chan struct{}
}

func newBlockingReviewLLM() *blockingReviewLLM {
	return &blockingReviewLLM{entered: make(chan struct{}, 16), release: make(chan struct{})}
}

func (b *blockingReviewLLM) ChatCompletion(ctx context.Context, _ llm.ChatCompletionRequest) (*llm.ChatCompletionResponse, error) { //nolint:gocritic // llm.Provider takes the request by value
	b.entered <- struct{}{}
	select {
	case <-b.release:
		return needsReview(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *blockingReviewLLM) ChatCompletionStream(ctx context.Context, req llm.ChatCompletionRequest, _ func(llm.StreamChunk)) (*llm.ChatCompletionResponse, error) { //nolint:gocritic // llm.Provider takes the request by value
	return b.ChatCompletion(ctx, req)
}
func (b *blockingReviewLLM) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }
func (b *blockingReviewLLM) Health(context.Context) (bool, error)            { return true, nil }
func (b *blockingReviewLLM) HealthDetailed(context.Context) (*llm.HealthStatusReport, error) {
	return &llm.HealthStatusReport{}, nil
}

// TestReviewRouter_LLMCallDoesNotHoldTheSchedulingLock: the review router's
// LLM call ran under the global plan scheduling lock, so one slow call held
// up every plan (KI-76). The step's review is decided outside the lock and
// the step starts once it is decided.
func TestReviewRouter_LLMCallDoesNotHoldTheSchedulingLock(t *testing.T) {
	router := newBlockingReviewLLM()
	store, orchSvc, _ := newDebateSetupWithLLM(true, router)
	ctx := context.Background()
	// Each plan has its own agent: an agent works on one plan at a time (KI-94).
	newPlan := func(name, agentID string) *plan.ExecutionPlan {
		t.Helper()
		p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
			Name: name, ProjectID: "proj-1", Protocol: plan.ProtocolSequential,
			Steps: []plan.CreateStepRequest{{TaskID: "t1", AgentID: agentID}},
		})
		if err != nil {
			t.Fatalf("CreatePlan: %v", err)
		}
		return p
	}
	first, second := newPlan("first", "a1"), newPlan("second", "a2")

	within(t, "StartPlan while the review router's LLM call runs", func() {
		if _, err := orchSvc.StartPlan(ctx, first.ID); err != nil {
			t.Errorf("StartPlan: %v", err)
		}
	})
	select {
	case <-router.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the review router was not asked")
	}
	within(t, "scheduling another plan while the first plan's review is decided", func() {
		if _, err := orchSvc.StartPlan(ctx, second.ID); err != nil {
			t.Errorf("StartPlan: %v", err)
		}
	})
	if st := planState(t, store, first.ID).Steps[0]; st.Status != plan.StepStatusPending || st.RunID != "" {
		t.Fatalf("step under review = %s with run %q, want pending without a run", st.Status, st.RunID)
	}

	close(router.release)
	for _, p := range []*plan.ExecutionPlan{first, second} {
		if debate := debatePlanOf(t, store, p.ID); debate.Status != plan.StatusRunning {
			t.Errorf("debate of %s = %s, want running", p.Name, debate.Status)
		}
	}
}

// KI-94: cancelling a plan cancels the debate a step of it runs - the
// debate's runs stop - and the cancelled step stays cancelled when the
// debate ends.
func TestDebate_CancelPlanCancelsItsDebate(t *testing.T) {
	store, orchSvc, _ := newDebateSetup(true)
	ctx := context.Background()
	p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
		Name: "cancelled", ProjectID: "proj-1", Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("StartPlan: %v", err)
	}
	debate := debatePlanOf(t, store, p.ID)
	debateRun := debate.Steps[0].RunID

	within(t, "CancelPlan", func() {
		if err := orchSvc.CancelPlan(ctx, p.ID); err != nil {
			t.Errorf("CancelPlan: %v", err)
		}
	})

	if got := planState(t, store, debate.ID); got.Status != plan.StatusCancelled || got.Steps[0].Status != plan.StepStatusCancelled {
		t.Fatalf("debate %s with step %s, want both cancelled", got.Status, got.Steps[0].Status)
	}
	if r, err := store.GetRun(ctx, debateRun); err != nil || r.Status != run.StatusCancelled {
		t.Fatalf("debate run = %+v, %v; want it cancelled", r, err)
	}
	if got := planState(t, store, p.ID); got.Status != plan.StatusCancelled || got.Steps[0].Status != plan.StepStatusCancelled {
		t.Fatalf("plan %s with step %s, want both cancelled (the step not reset by the debate's end)", got.Status, got.Steps[0].Status)
	}
}

// KI-94: a debate that ends after its parent plan was cancelled (it was not
// found to cancel, e.g. created meanwhile) leaves the parent's step as it is.
func TestDebate_EndAfterTheParentWasCancelled(t *testing.T) {
	store, orchSvc, _ := newDebateSetup(true)
	ctx := context.Background()
	p, err := orchSvc.CreatePlan(ctx, &plan.CreatePlanRequest{
		Name: "parent", ProjectID: "proj-1", Protocol: plan.ProtocolSequential,
		Steps: []plan.CreateStepRequest{{TaskID: "t1", AgentID: "a1"}},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if _, err := orchSvc.StartPlan(ctx, p.ID); err != nil {
		t.Fatalf("StartPlan: %v", err)
	}
	debate := debatePlanOf(t, store, p.ID)
	// The parent is cancelled in the store only, as by another replica that
	// does not know the debate yet.
	if err := store.UpdatePlanStatus(ctx, p.ID, plan.StatusCancelled); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePlanStepStatus(ctx, p.Steps[0].ID, plan.StepStatusCancelled, "", "plan cancelled"); err != nil {
		t.Fatal(err)
	}

	completeStepRun(t, store, orchSvc, debate.ID, 0)
	completeStepRun(t, store, orchSvc, debate.ID, 1)

	if got := planState(t, store, p.ID).Steps[0]; got.Status != plan.StepStatusCancelled {
		t.Fatalf("parent step = %s after its debate ended, want it cancelled", got.Status)
	}
}
