package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/project"
)

// ErrReviewPipelineUnavailable: no review pipeline is wired to start.
var ErrReviewPipelineUnavailable = errors.New("review pipeline is not available")

// ReviewTriggerStore is the subset of the store needed by ReviewTriggerService.
type ReviewTriggerStore interface {
	CreateReviewTrigger(ctx context.Context, projectID, commitSHA, source string) (string, error)
	// GetProject verifies that the project exists and belongs to the tenant
	// embedded in ctx. The postgres implementation already filters by tenant_id.
	GetProject(ctx context.Context, id string) (*project.Project, error)
}

// ReviewPipelineStarter starts the contract-first review pipelines
// (ReviewPipelineService); an error means nothing was started.
type ReviewPipelineStarter interface {
	StartReviewPipeline(ctx context.Context, projectID string) (*plan.ExecutionPlan, error)
	StartBoundaryAnalysis(ctx context.Context, projectID string) (*plan.ExecutionPlan, error)
}

// ReviewTriggerService is the entry point of the review triggers: it checks
// the project and starts the pipelines.
type ReviewTriggerService struct {
	store     ReviewTriggerStore
	pipelines ReviewPipelineStarter
}

// NewReviewTriggerService creates a new ReviewTriggerService.
func NewReviewTriggerService(store ReviewTriggerStore, pipelines ReviewPipelineStarter) *ReviewTriggerService {
	return &ReviewTriggerService{store: store, pipelines: pipelines}
}

// reviewTriggerSource is the source recorded for a review trigger: the only
// trigger is the manual one (POST /projects/{id}/review-refactor).
const reviewTriggerSource = "manual"

// TriggerReview starts the review-refactor pipeline and returns its plan.
// The trigger is recorded once the pipeline started. The project must belong
// to the tenant in ctx.
func (s *ReviewTriggerService) TriggerReview(ctx context.Context, projectID, commitSHA string) (*plan.ExecutionPlan, error) {
	if err := s.checkProject(ctx, projectID); err != nil {
		return nil, err
	}

	p, err := s.pipelines.StartReviewPipeline(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.CreateReviewTrigger(ctx, projectID, commitSHA, reviewTriggerSource); err != nil {
		// The pipeline runs; only the dedup record is missing.
		slog.Warn("review trigger not recorded", "project_id", projectID, "plan_id", p.ID, "error", err)
	}
	return p, nil
}

// TriggerBoundaryAnalysis starts the boundary analysis of the project and
// returns its plan. The project must belong to the tenant in ctx.
func (s *ReviewTriggerService) TriggerBoundaryAnalysis(ctx context.Context, projectID string) (*plan.ExecutionPlan, error) {
	if err := s.checkProject(ctx, projectID); err != nil {
		return nil, err
	}
	return s.pipelines.StartBoundaryAnalysis(ctx, projectID)
}

// checkProject enforces tenant isolation (GetProject is tenant-scoped, so a
// project of another tenant is not found) and that a pipeline is wired.
func (s *ReviewTriggerService) checkProject(ctx context.Context, projectID string) error {
	if _, err := s.store.GetProject(ctx, projectID); err != nil {
		return fmt.Errorf("project access check: %w", err)
	}
	if s.pipelines == nil {
		return ErrReviewPipelineUnavailable
	}
	return nil
}
