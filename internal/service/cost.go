package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// CostService provides cost and token aggregation queries.
type CostService struct {
	store database.Store
}

// NewCostService creates a new CostService.
func NewCostService(store database.Store) *CostService {
	return &CostService{store: store}
}

// GlobalSummary returns cost totals grouped by project.
func (s *CostService) GlobalSummary(ctx context.Context) ([]cost.ProjectSummary, error) {
	return s.store.CostSummaryGlobal(ctx)
}

// ProjectSummary returns aggregate cost for a single project.
func (s *CostService) ProjectSummary(ctx context.Context, projectID string) (*cost.Summary, error) {
	return s.store.CostSummaryByProject(ctx, projectID)
}

// ByModel returns per-model cost breakdown for a project.
func (s *CostService) ByModel(ctx context.Context, projectID string) ([]cost.ModelSummary, error) {
	return s.store.CostByModel(ctx, projectID)
}

// TimeSeries returns daily cost aggregation for a project.
func (s *CostService) TimeSeries(ctx context.Context, projectID string, days int) ([]cost.DailyCost, error) {
	return s.store.CostTimeSeries(ctx, projectID, days)
}

// RecentRuns returns the most recent runs for a project ordered by creation time.
func (s *CostService) RecentRuns(ctx context.Context, projectID string, limit int) ([]run.Run, error) {
	return s.store.RecentRunsWithCost(ctx, projectID, limit)
}

// ByTool returns per-tool cost breakdown for a project.
func (s *CostService) ByTool(ctx context.Context, projectID string) ([]cost.ToolSummary, error) {
	return s.store.CostByTool(ctx, projectID)
}

// ByToolForRun returns per-tool cost breakdown for a single run.
func (s *CostService) ByToolForRun(ctx context.Context, runID string) ([]cost.ToolSummary, error) {
	return s.store.CostByToolForRun(ctx, runID)
}
