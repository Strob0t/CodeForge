package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

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

func TestReviewTriggerService_DedupSkipsRecentSHA(t *testing.T) {
	store := &mockReviewTriggerStore{
		recentExists: true,
		projects:     map[string]string{"proj-1": tenantctx.DefaultTenantID},
	}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	triggered, err := svc.TriggerReview(context.Background(), "proj-1", "abc123", "pipeline-completion")
	if err != nil {
		t.Fatal(err)
	}
	if triggered {
		t.Error("expected dedup to skip, but review was triggered")
	}
}

func TestReviewTriggerService_ManualBypassesDedup(t *testing.T) {
	store := &mockReviewTriggerStore{
		recentExists: true,
		projects:     map[string]string{"proj-1": tenantctx.DefaultTenantID},
	}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	triggered, err := svc.TriggerReview(context.Background(), "proj-1", "abc123", "manual")
	if err != nil {
		t.Fatal(err)
	}
	if !triggered {
		t.Error("manual trigger should bypass dedup")
	}
}

func TestReviewTriggerService_NewSHATriggersReview(t *testing.T) {
	store := &mockReviewTriggerStore{
		recentExists: false,
		projects:     map[string]string{"proj-1": tenantctx.DefaultTenantID},
	}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	triggered, err := svc.TriggerReview(context.Background(), "proj-1", "newsha", "branch-merge")
	if err != nil {
		t.Fatal(err)
	}
	if !triggered {
		t.Error("new SHA should trigger review")
	}
	if len(store.createdIDs) != 1 {
		t.Errorf("expected 1 trigger created, got %d", len(store.createdIDs))
	}
}

func TestReviewTriggerService_TenantIsolation(t *testing.T) {
	tenantA := "aaaaaaaa-0000-0000-0000-000000000001"
	tenantB := "bbbbbbbb-0000-0000-0000-000000000002"

	store := &mockReviewTriggerStore{
		recentExists: false,
		projects:     map[string]string{"proj-a": tenantA},
	}
	svc := NewReviewTriggerService(store, nil, 30*time.Minute)

	// Tenant A owns proj-a -- should succeed.
	ctxA := tenantctx.WithTenant(context.Background(), tenantA)
	triggered, err := svc.TriggerReview(ctxA, "proj-a", "sha1", "pipeline-completion")
	if err != nil {
		t.Fatalf("expected success for owner tenant, got: %v", err)
	}
	if !triggered {
		t.Error("expected review to be triggered for owner tenant")
	}

	// Tenant B tries proj-a -- should be denied.
	ctxB := tenantctx.WithTenant(context.Background(), tenantB)
	triggered, err = svc.TriggerReview(ctxB, "proj-a", "sha2", "pipeline-completion")
	if err == nil {
		t.Fatal("expected error for cross-tenant access, got nil")
	}
	if triggered {
		t.Error("expected no trigger for cross-tenant access")
	}
	if !strings.Contains(err.Error(), "project access check") {
		t.Errorf("expected 'project access check' in error, got: %v", err)
	}

	// Non-existent project -- should be denied.
	triggered, err = svc.TriggerReview(ctxA, "proj-nonexistent", "sha3", "manual")
	if err == nil {
		t.Fatal("expected error for non-existent project, got nil")
	}
	if triggered {
		t.Error("expected no trigger for non-existent project")
	}
}
