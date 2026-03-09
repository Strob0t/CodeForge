package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// DashboardService provides aggregated dashboard data.
type DashboardService struct {
	store database.Store
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(store database.Store) *DashboardService {
	return &DashboardService{store: store}
}

// Stats returns the 7 KPI values for the dashboard header.
func (s *DashboardService) Stats(ctx context.Context) (*dashboard.DashboardStats, error) {
	return s.store.DashboardStats(ctx)
}

// ProjectHealth returns health score + stats for a single project.
func (s *DashboardService) ProjectHealth(ctx context.Context, projectID string) (*dashboard.ProjectHealth, error) {
	return s.store.ProjectHealth(ctx, projectID)
}

// RunOutcomes returns run status counts for the donut chart.
func (s *DashboardService) RunOutcomes(ctx context.Context, days int) ([]dashboard.RunOutcome, error) {
	return s.store.DashboardRunOutcomes(ctx, days)
}

// AgentPerformance returns agent success rates for the bar chart.
func (s *DashboardService) AgentPerformance(ctx context.Context) ([]dashboard.AgentPerf, error) {
	return s.store.DashboardAgentPerformance(ctx)
}

// ModelUsage returns cost per model for the pie chart.
func (s *DashboardService) ModelUsage(ctx context.Context) ([]dashboard.ModelUsage, error) {
	return s.store.DashboardModelUsage(ctx)
}

// CostByProject returns per-project costs for the horizontal bar chart.
func (s *DashboardService) CostByProject(ctx context.Context) ([]dashboard.ProjectCost, error) {
	return s.store.DashboardCostByProject(ctx)
}

// CostTrend returns daily cost aggregated across all projects.
func (s *DashboardService) CostTrend(ctx context.Context, days int) ([]cost.DailyCost, error) {
	return s.store.DashboardCostTrend(ctx, days)
}
