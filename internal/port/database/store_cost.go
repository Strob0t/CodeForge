package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/cost"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

// CostStore defines database operations for cost aggregation.
type CostStore interface {
	CostSummaryGlobal(ctx context.Context) ([]cost.ProjectSummary, error)
	CostSummaryByProject(ctx context.Context, projectID string) (*cost.Summary, error)
	CostByModel(ctx context.Context, projectID string) ([]cost.ModelSummary, error)
	CostTimeSeries(ctx context.Context, projectID string, days int) ([]cost.DailyCost, error)
	RecentRunsWithCost(ctx context.Context, projectID string, limit int) ([]run.Run, error)
	CostByTool(ctx context.Context, projectID string) ([]cost.ToolSummary, error)
	CostByToolForRun(ctx context.Context, runID string) ([]cost.ToolSummary, error)
}
