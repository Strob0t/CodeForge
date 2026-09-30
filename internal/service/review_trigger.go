package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// ReviewTriggerStore is the subset of the store needed by ReviewTriggerService.
type ReviewTriggerStore interface {
	FindRecentReviewTrigger(ctx context.Context, projectID, commitSHA string, within time.Duration) (bool, error)
	CreateReviewTrigger(ctx context.Context, projectID, commitSHA, source string) (string, error)
	// GetProject verifies that the project exists and belongs to the tenant
	// embedded in ctx. The postgres implementation already filters by tenant_id.
	GetProject(ctx context.Context, id string) (*project.Project, error)
}

// ReviewTriggerOrchestrator creates and starts review-refactor plans.
type ReviewTriggerOrchestrator interface {
	StartReviewPipeline(ctx context.Context, projectID string) error
}

// ReviewTriggerService manages cascade triggers with deduplication.
type ReviewTriggerService struct {
	store        ReviewTriggerStore
	orchestrator ReviewTriggerOrchestrator
	dedupWindow  time.Duration
}

// NewReviewTriggerService creates a new ReviewTriggerService.
func NewReviewTriggerService(store ReviewTriggerStore, orch ReviewTriggerOrchestrator, dedupWindow time.Duration) *ReviewTriggerService {
	return &ReviewTriggerService{
		store:        store,
		orchestrator: orch,
		dedupWindow:  dedupWindow,
	}
}

// TriggerReview attempts to start a review-refactor pipeline.
// Returns true if a review was triggered, false if deduplicated.
// The project must belong to the tenant extracted from ctx.
func (s *ReviewTriggerService) TriggerReview(ctx context.Context, projectID, commitSHA, source string) (bool, error) {
	// Tenant isolation: verify the project belongs to the calling tenant.
	// GetProject is tenant-scoped (filters by tenant_id from ctx), so
	// a cross-tenant projectID returns ErrNotFound.
	if _, err := s.store.GetProject(ctx, projectID); err != nil {
		return false, fmt.Errorf("project access check: %w", err)
	}

	// Manual triggers bypass dedup
	if source != "manual" {
		exists, err := s.store.FindRecentReviewTrigger(ctx, projectID, commitSHA, s.dedupWindow)
		if err != nil {
			return false, err
		}
		if exists {
			return false, nil
		}
	}

	if _, err := s.store.CreateReviewTrigger(ctx, projectID, commitSHA, source); err != nil {
		return false, err
	}

	if s.orchestrator != nil {
		if err := s.orchestrator.StartReviewPipeline(ctx, projectID); err != nil {
			return true, err
		}
	}

	return true, nil
}
