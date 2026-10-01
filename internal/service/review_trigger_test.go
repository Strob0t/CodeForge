package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-17: a trigger answers with the plan it started or an error, never
// "triggered" when nothing started (the service used to report success with
// no orchestrator wired, and recorded the trigger before starting anything).

type mockReviewTriggerStore struct {
	createdIDs []string
	sources    []string
	// projects maps projectID -> tenantID for tenant isolation testing.
	projects map[string]string
}

func (m *mockReviewTriggerStore) CreateReviewTrigger(_ context.Context, _, _, source string) (string, error) {
	id := "trigger-1"
	m.createdIDs = append(m.createdIDs, id)
	m.sources = append(m.sources, source)
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

func newTriggerTest() (*mockReviewTriggerStore, *fakePipelineStarter, *ReviewTriggerService) {
	store := &mockReviewTriggerStore{projects: map[string]string{"proj-1": tenantctx.DefaultTenantID}}
	starter := &fakePipelineStarter{}
	return store, starter, NewReviewTriggerService(store, starter)
}

// S6-F 15: the only trigger is the manual one; every trigger starts the
// pipeline (the dedup branch for other sources was unreachable) and is
// recorded once it started.
func TestReviewTriggerService_StartsAndRecords(t *testing.T) {
	store, starter, svc := newTriggerTest()

	for range 2 {
		p, err := svc.TriggerReview(context.Background(), "proj-1", "abc123")
		if err != nil || p == nil || p.ID != "plan-review" {
			t.Fatalf("TriggerReview = %+v, %v, want the started review plan", p, err)
		}
	}
	if len(starter.started) != 2 || len(store.createdIDs) != 2 {
		t.Fatalf("started %v, recorded %d triggers, want two each", starter.started, len(store.createdIDs))
	}
	for _, src := range store.sources {
		if src != "manual" {
			t.Fatalf("recorded source %q, want manual", src)
		}
	}
}

// A pipeline that does not start is an error, and the trigger is not
// recorded: the next trigger of the same commit is not deduplicated away.
func TestReviewTriggerService_FailedStartIsNotRecorded(t *testing.T) {
	store, starter, svc := newTriggerTest()
	starter.err = ErrReviewNoAgents

	p, err := svc.TriggerReview(context.Background(), "proj-1", "sha")
	if !errors.Is(err, ErrReviewNoAgents) || p != nil {
		t.Fatalf("TriggerReview = %+v, %v, want the start error", p, err)
	}
	if len(store.createdIDs) != 0 {
		t.Fatalf("recorded %d triggers for a failed start, want 0", len(store.createdIDs))
	}
}

func TestReviewTriggerService_NoPipelineIsAnError(t *testing.T) {
	store := &mockReviewTriggerStore{projects: map[string]string{"proj-1": tenantctx.DefaultTenantID}}
	svc := NewReviewTriggerService(store, nil)

	if p, err := svc.TriggerReview(context.Background(), "proj-1", "sha"); !errors.Is(err, ErrReviewPipelineUnavailable) || p != nil {
		t.Fatalf("TriggerReview = %+v, %v, want ErrReviewPipelineUnavailable", p, err)
	}
	if p, err := svc.TriggerBoundaryAnalysis(context.Background(), "proj-1"); !errors.Is(err, ErrReviewPipelineUnavailable) || p != nil {
		t.Fatalf("TriggerBoundaryAnalysis = %+v, %v, want ErrReviewPipelineUnavailable", p, err)
	}
}

func TestReviewTriggerService_BoundaryAnalysis(t *testing.T) {
	_, starter, svc := newTriggerTest()

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
	svc := NewReviewTriggerService(store, starter)

	ctxA := tenantctx.WithTenant(context.Background(), tenantA)
	if p, err := svc.TriggerReview(ctxA, "proj-a", "sha1"); err != nil || p == nil {
		t.Fatalf("owner tenant: %+v, %v, want a plan", p, err)
	}

	ctxB := tenantctx.WithTenant(context.Background(), tenantB)
	for name, trigger := range map[string]func() (*plan.ExecutionPlan, error){
		"review":            func() (*plan.ExecutionPlan, error) { return svc.TriggerReview(ctxB, "proj-a", "sha2") },
		"boundary analysis": func() (*plan.ExecutionPlan, error) { return svc.TriggerBoundaryAnalysis(ctxB, "proj-a") },
		"unknown project": func() (*plan.ExecutionPlan, error) {
			return svc.TriggerReview(ctxA, "proj-nonexistent", "sha3")
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
