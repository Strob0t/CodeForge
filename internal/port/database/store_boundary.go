package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
)

// EndedReviewRefactoring is a review pipeline whose refactoring is not
// decided although its plan ended (the stuck-work watchdog measures it).
type EndedReviewRefactoring struct {
	PlanID     string
	TenantID   string
	PlanStatus string
}

// BoundaryStore defines database operations for boundaries and review triggers (Phase 31).
type BoundaryStore interface {
	// Boundaries
	GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error)
	UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error
	DeleteProjectBoundaries(ctx context.Context, projectID string) error

	// Review Triggers
	CreateReviewTrigger(ctx context.Context, projectID, commitSHA, source string) (string, error)
}
