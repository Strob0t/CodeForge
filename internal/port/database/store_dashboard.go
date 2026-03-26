package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/dashboard"
)

// DashboardStore defines database operations for dashboard aggregation.
type DashboardStore interface {
	DashboardStats(ctx context.Context) (*dashboard.DashboardStats, error)
	ProjectHealth(ctx context.Context, projectID string) (*dashboard.ProjectHealth, error)
	DashboardRunOutcomes(ctx context.Context, days int) ([]dashboard.RunOutcome, error)
	DashboardAgentPerformance(ctx context.Context) ([]dashboard.AgentPerf, error)
	DashboardModelUsage(ctx context.Context) ([]dashboard.ModelUsage, error)
	DashboardCostByProject(ctx context.Context) ([]dashboard.ProjectCost, error)
	DashboardCostTrend(ctx context.Context, days int) ([]cost.DailyCost, error)
}
