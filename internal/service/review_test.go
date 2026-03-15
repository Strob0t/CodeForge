package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/review"
	"github.com/Strob0t/CodeForge/internal/service"
)

// ---------------------------------------------------------------------------
// Mock store for review-related database operations
// ---------------------------------------------------------------------------

type reviewMockStore struct {
	runtimeMockStore // embed for all non-review stubs

	policies       []review.ReviewPolicy
	reviews        []review.Review
	commitCounters map[string]int // policyID -> counter
	failCreate     bool
}

func newReviewMockStore() *reviewMockStore {
	return &reviewMockStore{
		commitCounters: make(map[string]int),
	}
}

func (m *reviewMockStore) CreateReviewPolicy(_ context.Context, p *review.ReviewPolicy) error {
	if m.failCreate {
		return errors.New("store: create failed")
	}
	m.policies = append(m.policies, *p)
	return nil
}

func (m *reviewMockStore) GetReviewPolicy(_ context.Context, id string) (*review.ReviewPolicy, error) {
	for i := range m.policies {
		if m.policies[i].ID == id {
			return &m.policies[i], nil
		}
	}
	return nil, fmt.Errorf("policy %q not found", id)
}

func (m *reviewMockStore) ListReviewPoliciesByProject(_ context.Context, projectID string) ([]review.ReviewPolicy, error) {
	var result []review.ReviewPolicy
	for i := range m.policies {
		if m.policies[i].ProjectID == projectID {
			result = append(result, m.policies[i])
		}
	}
	return result, nil
}

func (m *reviewMockStore) UpdateReviewPolicy(_ context.Context, p *review.ReviewPolicy) error {
	for i := range m.policies {
		if m.policies[i].ID == p.ID {
			m.policies[i] = *p
			return nil
		}
	}
	return fmt.Errorf("policy %q not found", p.ID)
}

func (m *reviewMockStore) DeleteReviewPolicy(_ context.Context, id string) error {
	for i := range m.policies {
		if m.policies[i].ID == id {
			m.policies = append(m.policies[:i], m.policies[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("policy %q not found", id)
}

func (m *reviewMockStore) ListEnabledPoliciesByTrigger(_ context.Context, triggerType review.TriggerType) ([]review.ReviewPolicy, error) {
	var result []review.ReviewPolicy
	for i := range m.policies {
		if m.policies[i].Enabled && m.policies[i].TriggerType == triggerType {
			result = append(result, m.policies[i])
		}
	}
	return result, nil
}

func (m *reviewMockStore) IncrementCommitCounter(_ context.Context, policyID string, count int) (int, error) {
	m.commitCounters[policyID] += count
	return m.commitCounters[policyID], nil
}

func (m *reviewMockStore) ResetCommitCounter(_ context.Context, policyID string) error {
	m.commitCounters[policyID] = 0
	return nil
}

func (m *reviewMockStore) CreateReview(_ context.Context, r *review.Review) error {
	m.reviews = append(m.reviews, *r)
	return nil
}

func (m *reviewMockStore) GetReview(_ context.Context, id string) (*review.Review, error) {
	for i := range m.reviews {
		if m.reviews[i].ID == id {
			return &m.reviews[i], nil
		}
	}
	return nil, fmt.Errorf("review %q not found", id)
}

func (m *reviewMockStore) ListReviewsByProject(_ context.Context, projectID string) ([]review.Review, error) {
	var result []review.Review
	for i := range m.reviews {
		if m.reviews[i].ProjectID == projectID {
			result = append(result, m.reviews[i])
		}
	}
	return result, nil
}

func (m *reviewMockStore) UpdateReviewStatus(_ context.Context, id string, status review.Status, completedAt *time.Time) error {
	for i := range m.reviews {
		if m.reviews[i].ID == id {
			m.reviews[i].Status = status
			m.reviews[i].CompletedAt = completedAt
			return nil
		}
	}
	return fmt.Errorf("review %q not found", id)
}

func (m *reviewMockStore) GetReviewByPlanID(_ context.Context, planID string) (*review.Review, error) {
	for i := range m.reviews {
		if m.reviews[i].PlanID == planID {
			return &m.reviews[i], nil
		}
	}
	return nil, fmt.Errorf("review with plan %q not found", planID)
}

// ---------------------------------------------------------------------------
// Test environment factory
// ---------------------------------------------------------------------------

func newReviewTestEnv() (*service.ReviewService, *reviewMockStore) {
	store := newReviewMockStore()
	hub := &runtimeMockBroadcaster{}
	events := &runtimeMockEventStore{}

	// Use real PipelineService + ModeService with built-in modes/templates.
	modes := service.NewModeService()
	pipelineSvc := service.NewPipelineService(modes)

	// OrchestratorService needs a non-nil config for MaxParallel default.
	// Runtime is nil — trigger tests don't exercise full orchestration.
	orchestrator := service.NewOrchestratorService(store, hub, events, nil, &config.Orchestrator{})

	svc := service.NewReviewService(store, pipelineSvc, orchestrator, hub, events)
	return svc, store
}

// mustCreatePolicy is a test helper that creates a policy and fails the test on error.
func mustCreatePolicy(t *testing.T, svc *service.ReviewService, projectID string, req *review.CreatePolicyRequest) *review.ReviewPolicy {
	t.Helper()
	p, err := svc.CreatePolicy(context.Background(), projectID, "tenant-1", req)
	if err != nil {
		t.Fatalf("mustCreatePolicy: %v", err)
	}
	return p
}

// ---------------------------------------------------------------------------
// Tests: CreatePolicy validation
// ---------------------------------------------------------------------------

func TestReviewService_CreatePolicy_Validation(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	tests := []struct {
		name    string
		req     review.CreatePolicyRequest
		wantErr string
	}{
		{
			name:    "missing name",
			req:     review.CreatePolicyRequest{TriggerType: review.TriggerCommitCount, CommitThreshold: 5},
			wantErr: "policy name is required",
		},
		{
			name:    "invalid trigger type",
			req:     review.CreatePolicyRequest{Name: "test", TriggerType: "invalid"},
			wantErr: "invalid trigger type",
		},
		{
			name:    "commit_count without threshold",
			req:     review.CreatePolicyRequest{Name: "test", TriggerType: review.TriggerCommitCount, CommitThreshold: 0},
			wantErr: "commit_threshold must be >= 1",
		},
		{
			name:    "pre_merge without branch pattern",
			req:     review.CreatePolicyRequest{Name: "test", TriggerType: review.TriggerPreMerge},
			wantErr: "branch_pattern is required",
		},
		{
			name:    "cron without expression",
			req:     review.CreatePolicyRequest{Name: "test", TriggerType: review.TriggerCron},
			wantErr: "cron_expr is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreatePolicy(ctx, "proj-1", "tenant-1", &tt.req)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestReviewService_CreatePolicy_ValidCommitCount(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	req := &review.CreatePolicyRequest{
		Name:            "nightly-review",
		TriggerType:     review.TriggerCommitCount,
		CommitThreshold: 10,
	}
	p, err := svc.CreatePolicy(ctx, "proj-1", "tenant-1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID == "" {
		t.Error("expected non-empty ID")
	}
	if p.Name != "nightly-review" {
		t.Errorf("got name=%q, want %q", p.Name, "nightly-review")
	}
	if p.TemplateID != "review-only" {
		t.Errorf("got template=%q, want default %q", p.TemplateID, "review-only")
	}
	if !p.Enabled {
		t.Error("expected policy to be enabled by default")
	}
	if len(store.policies) != 1 {
		t.Errorf("store should have 1 policy, got %d", len(store.policies))
	}
}

func TestReviewService_CreatePolicy_DefaultTemplate(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	req := &review.CreatePolicyRequest{
		Name:            "test",
		TriggerType:     review.TriggerCommitCount,
		CommitThreshold: 5,
	}
	p, err := svc.CreatePolicy(ctx, "proj-1", "tenant-1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.TemplateID != "review-only" {
		t.Errorf("expected default template %q, got %q", "review-only", p.TemplateID)
	}
}

func TestReviewService_CreatePolicy_CustomTemplate(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	req := &review.CreatePolicyRequest{
		Name:            "custom",
		TriggerType:     review.TriggerCommitCount,
		CommitThreshold: 3,
		TemplateID:      "full-review",
	}
	p, err := svc.CreatePolicy(ctx, "proj-1", "tenant-1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.TemplateID != "full-review" {
		t.Errorf("expected template %q, got %q", "full-review", p.TemplateID)
	}
}

// ---------------------------------------------------------------------------
// Tests: UpdatePolicy
// ---------------------------------------------------------------------------

func TestReviewService_UpdatePolicy_PartialFields(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	// Create a base policy.
	req := &review.CreatePolicyRequest{
		Name:            "original",
		TriggerType:     review.TriggerCommitCount,
		CommitThreshold: 5,
	}
	p, err := svc.CreatePolicy(ctx, "proj-1", "tenant-1", req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update only the name.
	newName := "updated-name"
	updated, err := svc.UpdatePolicy(ctx, p.ID, review.UpdatePolicyRequest{Name: &newName})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "updated-name" {
		t.Errorf("name not updated: got %q", updated.Name)
	}
	if updated.CommitThreshold != 5 {
		t.Errorf("threshold should be unchanged: got %d", updated.CommitThreshold)
	}
}

func TestReviewService_UpdatePolicy_InvalidStateRejected(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	req := &review.CreatePolicyRequest{
		Name:            "test",
		TriggerType:     review.TriggerCommitCount,
		CommitThreshold: 5,
	}
	p, err := svc.CreatePolicy(ctx, "proj-1", "tenant-1", req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Try setting commit_threshold to 0 — should fail re-validation.
	zero := 0
	_, err = svc.UpdatePolicy(ctx, p.ID, review.UpdatePolicyRequest{CommitThreshold: &zero})
	if err == nil {
		t.Fatal("expected validation error for threshold=0")
	}
}

func TestReviewService_UpdatePolicy_NotFound(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	newName := "x"
	_, err := svc.UpdatePolicy(ctx, "nonexistent", review.UpdatePolicyRequest{Name: &newName})
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

// ---------------------------------------------------------------------------
// Tests: CRUD — Get, Delete, List
// ---------------------------------------------------------------------------

func TestReviewService_GetPolicy(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	req := &review.CreatePolicyRequest{Name: "p1", TriggerType: review.TriggerCommitCount, CommitThreshold: 3}
	p, _ := svc.CreatePolicy(ctx, "proj-1", "tenant-1", req)

	got, err := svc.GetPolicy(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "p1" {
		t.Errorf("got name=%q, want %q", got.Name, "p1")
	}
}

func TestReviewService_DeletePolicy(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	req := &review.CreatePolicyRequest{Name: "del", TriggerType: review.TriggerCommitCount, CommitThreshold: 1}
	p, _ := svc.CreatePolicy(ctx, "proj-1", "tenant-1", req)

	if err := svc.DeletePolicy(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(store.policies) != 0 {
		t.Errorf("expected 0 policies after delete, got %d", len(store.policies))
	}
}

func TestReviewService_ListPolicies(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	mustCreatePolicy(t, svc, "proj-1", &review.CreatePolicyRequest{Name: "a", TriggerType: review.TriggerCommitCount, CommitThreshold: 1})
	mustCreatePolicy(t, svc, "proj-1", &review.CreatePolicyRequest{Name: "b", TriggerType: review.TriggerCommitCount, CommitThreshold: 2})
	mustCreatePolicy(t, svc, "proj-2", &review.CreatePolicyRequest{Name: "c", TriggerType: review.TriggerCommitCount, CommitThreshold: 3})

	list, err := svc.ListPolicies(ctx, "proj-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 policies for proj-1, got %d", len(list))
	}
}

// ---------------------------------------------------------------------------
// Tests: HandlePush — commit threshold trigger
// ---------------------------------------------------------------------------

func TestReviewService_HandlePush_BelowThreshold(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	// Create a commit_count policy with threshold=5.
	mustCreatePolicy(t, svc, "proj-1", &review.CreatePolicyRequest{
		Name:            "commit-review",
		TriggerType:     review.TriggerCommitCount,
		CommitThreshold: 5,
	})

	// Push 3 commits — below threshold.
	if err := svc.HandlePush(ctx, "proj-1", "main", 3); err != nil {
		t.Fatalf("push: %v", err)
	}

	// No review should be created.
	if len(store.reviews) != 0 {
		t.Errorf("expected 0 reviews below threshold, got %d", len(store.reviews))
	}
	// Counter should be 3.
	policyID := store.policies[0].ID
	if store.commitCounters[policyID] != 3 {
		t.Errorf("counter: got %d, want 3", store.commitCounters[policyID])
	}
}

func TestReviewService_HandlePush_AtThreshold(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	mustCreatePolicy(t, svc, "proj-1", &review.CreatePolicyRequest{
		Name:            "commit-review",
		TriggerType:     review.TriggerCommitCount,
		CommitThreshold: 5,
	})

	// Push 5 commits — at threshold.
	if err := svc.HandlePush(ctx, "proj-1", "main", 5); err != nil {
		t.Fatalf("push: %v", err)
	}

	// Review should be created (even if triggerReview fails on pipeline/orchestrator,
	// the review record is created first).
	if len(store.reviews) != 1 {
		t.Errorf("expected 1 review at threshold, got %d", len(store.reviews))
	}

	// Counter should be reset to 0.
	policyID := store.policies[0].ID
	if store.commitCounters[policyID] != 0 {
		t.Errorf("counter should be reset: got %d", store.commitCounters[policyID])
	}
}

func TestReviewService_HandlePush_DisabledPolicySkipped(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	disabled := false
	mustCreatePolicy(t, svc, "proj-1", &review.CreatePolicyRequest{
		Name:            "disabled",
		TriggerType:     review.TriggerCommitCount,
		CommitThreshold: 1,
		Enabled:         &disabled,
	})

	if err := svc.HandlePush(ctx, "proj-1", "main", 10); err != nil {
		t.Fatalf("push: %v", err)
	}

	if len(store.reviews) != 0 {
		t.Errorf("disabled policy should not trigger review, got %d", len(store.reviews))
	}
}

// ---------------------------------------------------------------------------
// Tests: HandlePreMerge — branch pattern matching
// ---------------------------------------------------------------------------

func TestReviewService_HandlePreMerge_MatchingBranch(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	mustCreatePolicy(t, svc, "proj-1", &review.CreatePolicyRequest{
		Name:          "pre-merge",
		TriggerType:   review.TriggerPreMerge,
		BranchPattern: "main",
	})

	r, err := svc.HandlePreMerge(ctx, "proj-1", "main")
	if err != nil {
		t.Fatalf("pre-merge: %v", err)
	}
	if r == nil {
		t.Fatal("expected review for matching branch")
	}
	if len(store.reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(store.reviews))
	}
}

func TestReviewService_HandlePreMerge_NonMatchingBranch(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	mustCreatePolicy(t, svc, "proj-1", &review.CreatePolicyRequest{
		Name:          "pre-merge",
		TriggerType:   review.TriggerPreMerge,
		BranchPattern: "main",
	})

	r, err := svc.HandlePreMerge(ctx, "proj-1", "develop")
	if err != nil {
		t.Fatalf("pre-merge: %v", err)
	}
	if r != nil {
		t.Error("expected nil review for non-matching branch")
	}
	if len(store.reviews) != 0 {
		t.Error("no review should be created for non-matching branch")
	}
}

func TestReviewService_HandlePreMerge_GlobPattern(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	mustCreatePolicy(t, svc, "proj-1", &review.CreatePolicyRequest{
		Name:          "release-gate",
		TriggerType:   review.TriggerPreMerge,
		BranchPattern: "release-*",
	})

	r, err := svc.HandlePreMerge(ctx, "proj-1", "release-v2")
	if err != nil {
		t.Fatalf("pre-merge: %v", err)
	}
	if r == nil {
		t.Fatal("expected review for glob-matching branch release-v2")
	}
}

func TestReviewService_HandlePreMerge_DisabledSkipped(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	disabled := false
	mustCreatePolicy(t, svc, "proj-1", &review.CreatePolicyRequest{
		Name:          "disabled",
		TriggerType:   review.TriggerPreMerge,
		BranchPattern: "main",
		Enabled:       &disabled,
	})

	r, err := svc.HandlePreMerge(ctx, "proj-1", "main")
	if err != nil {
		t.Fatalf("pre-merge: %v", err)
	}
	if r != nil {
		t.Error("disabled policy should not trigger review")
	}
	if len(store.reviews) != 0 {
		t.Error("no review should be created for disabled policy")
	}
}

// ---------------------------------------------------------------------------
// Tests: HandlePlanComplete — status mapping
// ---------------------------------------------------------------------------

func TestReviewService_HandlePlanComplete_Completed(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	// Seed a review with a plan ID.
	store.reviews = append(store.reviews, review.Review{
		ID:        "rev-1",
		PolicyID:  "pol-1",
		ProjectID: "proj-1",
		TenantID:  "tenant-1",
		PlanID:    "plan-abc",
		Status:    review.StatusRunning,
	})

	svc.HandlePlanComplete(ctx, "plan-abc", "completed")

	r, _ := store.GetReview(ctx, "rev-1")
	if r.Status != review.StatusCompleted {
		t.Errorf("expected status=%q, got %q", review.StatusCompleted, r.Status)
	}
	if r.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestReviewService_HandlePlanComplete_Failed(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	store.reviews = append(store.reviews, review.Review{
		ID:        "rev-2",
		PolicyID:  "pol-1",
		ProjectID: "proj-1",
		TenantID:  "tenant-1",
		PlanID:    "plan-fail",
		Status:    review.StatusRunning,
	})

	svc.HandlePlanComplete(ctx, "plan-fail", "failed")

	r, _ := store.GetReview(ctx, "rev-2")
	if r.Status != review.StatusFailed {
		t.Errorf("expected status=%q, got %q", review.StatusFailed, r.Status)
	}
}

func TestReviewService_HandlePlanComplete_UnknownPlanIgnored(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	// No reviews in store — should not panic.
	svc.HandlePlanComplete(ctx, "unknown-plan", "completed")

	if len(store.reviews) != 0 {
		t.Error("no review should be created for unknown plan")
	}
}

// ---------------------------------------------------------------------------
// Tests: ManualTrigger
// ---------------------------------------------------------------------------

func TestReviewService_ManualTrigger_ValidPolicy(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	p, _ := svc.CreatePolicy(ctx, "proj-1", "tenant-1", &review.CreatePolicyRequest{
		Name:            "manual",
		TriggerType:     review.TriggerCommitCount,
		CommitThreshold: 100,
	})

	r, err := svc.ManualTrigger(ctx, p.ID)
	if err != nil {
		t.Fatalf("manual trigger: %v", err)
	}
	if r == nil {
		t.Fatal("expected review from manual trigger")
	}
	if r.TriggerRef != "manual" {
		t.Errorf("trigger_ref: got %q, want %q", r.TriggerRef, "manual")
	}
	if len(store.reviews) != 1 {
		t.Errorf("expected 1 review, got %d", len(store.reviews))
	}
}

func TestReviewService_ManualTrigger_UnknownPolicy(t *testing.T) {
	svc, _ := newReviewTestEnv()
	ctx := context.Background()

	_, err := svc.ManualTrigger(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown policy")
	}
}

// ---------------------------------------------------------------------------
// Tests: GetReview, ListReviews
// ---------------------------------------------------------------------------

func TestReviewService_GetReview(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	store.reviews = append(store.reviews, review.Review{
		ID:        "rev-get",
		PolicyID:  "pol-1",
		ProjectID: "proj-1",
		Status:    review.StatusPending,
	})

	r, err := svc.GetReview(ctx, "rev-get")
	if err != nil {
		t.Fatalf("get review: %v", err)
	}
	if r.ID != "rev-get" {
		t.Errorf("got id=%q, want %q", r.ID, "rev-get")
	}
}

func TestReviewService_ListReviews(t *testing.T) {
	svc, store := newReviewTestEnv()
	ctx := context.Background()

	store.reviews = append(store.reviews,
		review.Review{ID: "r1", ProjectID: "proj-1"},
		review.Review{ID: "r2", ProjectID: "proj-1"},
		review.Review{ID: "r3", ProjectID: "proj-2"},
	)

	list, err := svc.ListReviews(ctx, "proj-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 reviews for proj-1, got %d", len(list))
	}
}
