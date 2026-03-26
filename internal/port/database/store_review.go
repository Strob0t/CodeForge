package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/review"
)

// ReviewStore defines database operations for review policies and reviews.
type ReviewStore interface {
	// Review Policies
	CreateReviewPolicy(ctx context.Context, p *review.ReviewPolicy) error
	GetReviewPolicy(ctx context.Context, id string) (*review.ReviewPolicy, error)
	ListReviewPoliciesByProject(ctx context.Context, projectID string) ([]review.ReviewPolicy, error)
	UpdateReviewPolicy(ctx context.Context, p *review.ReviewPolicy) error
	DeleteReviewPolicy(ctx context.Context, id string) error
	ListEnabledPoliciesByTrigger(ctx context.Context, triggerType review.TriggerType) ([]review.ReviewPolicy, error)
	IncrementCommitCounter(ctx context.Context, policyID string, count int) (int, error)
	ResetCommitCounter(ctx context.Context, policyID string) error

	// Reviews
	CreateReview(ctx context.Context, r *review.Review) error
	GetReview(ctx context.Context, id string) (*review.Review, error)
	ListReviewsByProject(ctx context.Context, projectID string) ([]review.Review, error)
	UpdateReviewStatus(ctx context.Context, id string, status review.Status, completedAt *time.Time) error
	GetReviewByPlanID(ctx context.Context, planID string) (*review.Review, error)
}
