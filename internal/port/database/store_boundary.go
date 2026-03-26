package database

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
)

// BoundaryStore defines database operations for boundaries and review triggers (Phase 31).
type BoundaryStore interface {
	// Boundaries
	GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error)
	UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error
	DeleteProjectBoundaries(ctx context.Context, projectID string) error

	// Review Triggers
	CreateReviewTrigger(ctx context.Context, projectID, commitSHA, source string) (string, error)
	FindRecentReviewTrigger(ctx context.Context, projectID, commitSHA string, within time.Duration) (bool, error)
}
