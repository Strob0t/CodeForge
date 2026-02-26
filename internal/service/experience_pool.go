package service

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/experience"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// ExperiencePoolService manages cached successful agent runs for reuse.
type ExperiencePoolService struct {
	db database.Store
}

// NewExperiencePoolService creates a new ExperiencePoolService.
func NewExperiencePoolService(db database.Store) *ExperiencePoolService {
	return &ExperiencePoolService{db: db}
}

// ListByProject returns all experience entries for a project.
func (s *ExperiencePoolService) ListByProject(ctx context.Context, projectID string) ([]experience.Entry, error) {
	return s.db.ListExperienceEntries(ctx, projectID)
}

// Get returns a single experience entry.
func (s *ExperiencePoolService) Get(ctx context.Context, id string) (*experience.Entry, error) {
	return s.db.GetExperienceEntry(ctx, id)
}

// Delete removes an experience entry.
func (s *ExperiencePoolService) Delete(ctx context.Context, id string) error {
	return s.db.DeleteExperienceEntry(ctx, id)
}

// Store creates a new experience entry.
func (s *ExperiencePoolService) Store(ctx context.Context, req *experience.CreateRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	entry := &experience.Entry{
		ProjectID:       req.ProjectID,
		TaskDescription: req.TaskDescription,
		ResultOutput:    req.ResultOutput,
		ResultCost:      req.ResultCost,
		ResultStatus:    req.ResultStatus,
		RunID:           req.RunID,
	}
	if err := s.db.CreateExperienceEntry(ctx, entry); err != nil {
		return fmt.Errorf("create experience entry: %w", err)
	}
	return nil
}
