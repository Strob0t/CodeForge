package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/goal"
)

// GoalStore defines database operations for project goals (Phase 28).
type GoalStore interface {
	CreateProjectGoal(ctx context.Context, g *goal.ProjectGoal) error
	GetProjectGoal(ctx context.Context, id string) (*goal.ProjectGoal, error)
	ListProjectGoals(ctx context.Context, projectID string) ([]goal.ProjectGoal, error)
	ListEnabledGoals(ctx context.Context, projectID string) ([]goal.ProjectGoal, error)
	UpdateProjectGoal(ctx context.Context, g *goal.ProjectGoal) error
	DeleteProjectGoal(ctx context.Context, id string) error
	DeleteProjectGoalsBySource(ctx context.Context, projectID, source string) error
}
