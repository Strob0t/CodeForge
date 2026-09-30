package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-17: a trigger answers with the plan it started or an error, never
// "triggered" when nothing started (the service used to report success with
// no orchestrator wired, and recorded the trigger before starting anything).

type mockReviewTriggerStore struct {
	recentExists bool
	createdIDs   []string
	// projects maps projectID -> tenantID for tenant isolation testing.
	projects map[string]string
}

func (m *mockReviewTriggerStore) FindRecentReviewTrigger(_ context.Context, _, _ string, _ time.Duration) (bool, error) {
	return m.recentExists, nil
}

func (m *mockReviewTriggerStore) CreateReviewTrigger(_ context.Context, _, _, _ string) (string, error) {
	id := "trigger-1"
	m.createdIDs = append(m.createdIDs, id)
	return id, nil
}

func (m *mockReviewTriggerStore) GetProject(ctx context.Context, id string) (*project.Project, error) {
	tenantID := tenantctx.FromContext(ctx)
	ownerTenant, ok := m.projects[id]
	if !ok || ownerTenant != tenantID {
		return nil, domain.ErrNotFound
	}
	return &project.Project{ID: id, TenantID: ownerTenant, Name: "test-project"}, nil
}

// fakePipelineStarter records the pipelines it starts; err fails every start.
type fakePipelineStarter struct {
	started []string
	err     error
}

func (f *fakePipelineStarter) start(kind string) (*plan.ExecutionPlan, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.started = append(f.started, kind)
	return &plan.ExecutionPlan{ID: "plan-" + kind, Status: plan.StatusRunning}, nil
}

func (f *fakePipelineStarter) StartReviewPipeline(context.Context, string) (*plan.ExecutionPlan, error) {
	return f.start("review")
}

func (f *fakePipelineStarter) StartBoundaryAnalysis(context.Context, string) (*plan.ExecutionPlan, error) {
	return f.start("boundaries")
}

func newTriggerTest(recent bool) (*mockReviewTriggerStore, *fakePipelineStarter, *ReviewTriggerService) {
	store := &mockReviewTriggerStore{recentExists: recent, projects: map[string]string{"proj-1": tenantctx.DefaultTenantID}}
	starter := &fakePipelineStarter{}
	return store, starter, NewReviewTriggerService(store, starter, 30*time.Minute)
}

func TestReviewTriggerService_DedupSkipsRecentSHA(t *testing.T) {
	store, starter, svc := newTriggerTest(true)

	p, err := svc.TriggerReview(context.Background(), "proj-1", "abc123", "pipeline-completion")
	if err != nil || p != nil {
		t.Fatalf("TriggerReview = %+v, %v, want deduplicated (no plan, no error)", p, err)
	}
	if len(starter.started) != 0 || len(store.createdIDs) != 0 {
		t.Fatalf("started %v, recorded %v, want nothing", starter.started, store.createdIDs)
	}
}

func TestReviewTriggerService_ManualBypassesDedup(t *testing.T) {
	_, starter, svc := newTriggerTest(true)

	p, err := svc.TriggerReview(context.Background(), "proj-1", "abc123", "manual")
	if err != nil || p == nil || p.ID != "plan-review" {
		t.Fatalf("TriggerReview = %+v, %v, want the started review plan", p, err)
	}
	if len(starter.started) != 1 {
		t.Fatalf("started %v, want the review pipeline", starter.started)
	}
}

func TestReviewTriggerService_NewSHATriggersReview(t *testing.T) {
	store, starter, svc := newTriggerTest(false)

	p, err := svc.TriggerReview(context.Background(), "proj-1", "newsha", "branch-merge")
	if err != nil || p == nil {
		t.Fatalf("TriggerReview = %+v, %v, want a plan", p, err)
	}
	if len(starter.started) != 1 || len(store.createdIDs) != 1 {
		t.Fatalf("started %v, recorded %d triggers, want one each", starter.started, len(store.createdIDs))
	}
}

// A pipeline that does not start is an error, and the trigger is not
// recorded: the next trigger of the same commit is not deduplicated away.
func TestReviewTriggerService_FailedStartIsNotRecorded(t *testing.T) {
	store, starter, svc := newTriggerTest(false)
	starter.err = ErrReviewNoAgents

	p, err := svc.TriggerReview(context.Background(), "proj-1", "sha", "branch-merge")
	if !errors.Is(err, ErrReviewNoAgents) || p != nil {
		t.Fatalf("TriggerReview = %+v, %v, want the start error", p, err)
	}
	if len(store.createdIDs) != 0 {
		t.Fatalf("recorded %d triggers for a failed start, want 0", len(store.createdIDs))
	}
}

func TestReviewTriggerService_NoPipelineIsAnError(t *testing.T) {
	store := &mockReviewTriggerStore{projects: map[string]string{"proj-1": tenantctx.DefaultTenantID}}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	if p, err := svc.TriggerReview(context.Background(), "proj-1", "sha", "manual"); !errors.Is(err, ErrReviewPipelineUnavailable) || p != nil {
		t.Fatalf("TriggerReview = %+v, %v, want ErrReviewPipelineUnavailable", p, err)
	}
	if p, err := svc.TriggerBoundaryAnalysis(context.Background(), "proj-1"); !errors.Is(err, ErrReviewPipelineUnavailable) || p != nil {
		t.Fatalf("TriggerBoundaryAnalysis = %+v, %v, want ErrReviewPipelineUnavailable", p, err)
	}
}

func TestReviewTriggerService_BoundaryAnalysis(t *testing.T) {
	_, starter, svc := newTriggerTest(true) // no dedup for the analysis

	p, err := svc.TriggerBoundaryAnalysis(context.Background(), "proj-1")
	if err != nil || p == nil || p.ID != "plan-boundaries" {
		t.Fatalf("TriggerBoundaryAnalysis = %+v, %v, want the boundary analysis plan", p, err)
	}
	if len(starter.started) != 1 || starter.started[0] != "boundaries" {
		t.Fatalf("started %v, want the boundary analysis", starter.started)
	}
}

func TestReviewTriggerService_TenantIsolation(t *testing.T) {
	tenantA := "aaaaaaaa-0000-0000-0000-000000000001"
	tenantB := "bbbbbbbb-0000-0000-0000-000000000002"

	store := &mockReviewTriggerStore{projects: map[string]string{"proj-a": tenantA}}
	starter := &fakePipelineStarter{}
	svc := NewReviewTriggerService(store, starter, 30*time.Minute)

	ctxA := tenantctx.WithTenant(context.Background(), tenantA)
	if p, err := svc.TriggerReview(ctxA, "proj-a", "sha1", "pipeline-completion"); err != nil || p == nil {
		t.Fatalf("owner tenant: %+v, %v, want a plan", p, err)
	}

	ctxB := tenantctx.WithTenant(context.Background(), tenantB)
	for name, trigger := range map[string]func() (*plan.ExecutionPlan, error){
		"review":            func() (*plan.ExecutionPlan, error) { return svc.TriggerReview(ctxB, "proj-a", "sha2", "manual") },
		"boundary analysis": func() (*plan.ExecutionPlan, error) { return svc.TriggerBoundaryAnalysis(ctxB, "proj-a") },
		"unknown project": func() (*plan.ExecutionPlan, error) {
			return svc.TriggerReview(ctxA, "proj-nonexistent", "sha3", "manual")
		},
	} {
		p, err := trigger()
		if !errors.Is(err, domain.ErrNotFound) || p != nil || !strings.Contains(err.Error(), "project access check") {
			t.Errorf("%s: %+v, %v, want a not found project access check", name, p, err)
		}
	}
	if len(starter.started) != 1 {
		t.Fatalf("started %v, want only the owner's review", starter.started)
	}
}
