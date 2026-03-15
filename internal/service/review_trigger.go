package service

import (
	"context"
	"time"
)

// ReviewTriggerStore is the subset of the store needed by ReviewTriggerService.
type ReviewTriggerStore interface {
	FindRecentReviewTrigger(ctx context.Context, projectID, commitSHA string, within time.Duration) (bool, error)
	CreateReviewTrigger(ctx context.Context, projectID, commitSHA, source string) (string, error)
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
func (s *ReviewTriggerService) TriggerReview(ctx context.Context, projectID, commitSHA, source string) (bool, error) {
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
