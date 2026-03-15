package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/boundary"
)

// BoundaryStore is the subset of the store needed by BoundaryService.
type BoundaryStore interface {
	GetProjectBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error)
	UpsertProjectBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error
	DeleteProjectBoundaries(ctx context.Context, projectID string) error
}

// BoundaryService manages project boundary configurations.
type BoundaryService struct {
	store BoundaryStore
}

// NewBoundaryService creates a new BoundaryService.
func NewBoundaryService(store BoundaryStore) *BoundaryService {
	return &BoundaryService{store: store}
}

// GetBoundaries returns the boundary config for a project.
func (s *BoundaryService) GetBoundaries(ctx context.Context, projectID string) (*boundary.ProjectBoundaryConfig, error) {
	return s.store.GetProjectBoundaries(ctx, projectID)
}

// UpdateBoundaries validates and persists a boundary config.
func (s *BoundaryService) UpdateBoundaries(ctx context.Context, cfg *boundary.ProjectBoundaryConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	return s.store.UpsertProjectBoundaries(ctx, cfg)
}

// DeleteBoundaries removes the boundary config for a project.
func (s *BoundaryService) DeleteBoundaries(ctx context.Context, projectID string) error {
	return s.store.DeleteProjectBoundaries(ctx, projectID)
}
