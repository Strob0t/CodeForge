package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// PlanStore defines database operations for execution plans and steps.
type PlanStore interface {
	CreatePlan(ctx context.Context, p *plan.ExecutionPlan) error
	GetPlan(ctx context.Context, id string) (*plan.ExecutionPlan, error)
	ListPlansByProject(ctx context.Context, projectID string) ([]plan.ExecutionPlan, error)
	UpdatePlanStatus(ctx context.Context, id string, status plan.Status) error
	CreatePlanStep(ctx context.Context, step *plan.Step) error
	ListPlanSteps(ctx context.Context, planID string) ([]plan.Step, error)
	UpdatePlanStepStatus(ctx context.Context, stepID string, status plan.StepStatus, runID string, errMsg string) error
	GetPlanStepByRunID(ctx context.Context, runID string) (*plan.Step, error)
	UpdatePlanStepRound(ctx context.Context, stepID string, round int) error
}
