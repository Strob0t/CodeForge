package database

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/routing"
)

// RoutingStore defines database operations for routing outcomes and statistics.
type RoutingStore interface {
	CreateRoutingOutcome(ctx context.Context, o *routing.RoutingOutcome) error
	ListRoutingStats(ctx context.Context, taskType, complexityTier string) ([]routing.ModelPerformanceStats, error)
	UpsertRoutingStats(ctx context.Context, s *routing.ModelPerformanceStats) error
	AggregateRoutingOutcomes(ctx context.Context) error
	ListRoutingOutcomes(ctx context.Context, limit int) ([]routing.RoutingOutcome, error)
}
