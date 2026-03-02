// Package service — RoutingService manages intelligent model routing state.
package service

import (
	"context"
	"fmt"

	"github.com/Strob0t/CodeForge/internal/domain/routing"
	"github.com/Strob0t/CodeForge/internal/port/database"
)

// RoutingService manages model routing state: outcomes, stats, and aggregation.
type RoutingService struct {
	store database.Store
}

// NewRoutingService creates a new RoutingService.
func NewRoutingService(store database.Store) *RoutingService {
	return &RoutingService{store: store}
}

// RecordOutcome persists a routing outcome from a completed LLM call.
func (s *RoutingService) RecordOutcome(ctx context.Context, o *routing.RoutingOutcome) error {
	if o.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	if !o.TaskType.IsValid() {
		return fmt.Errorf("invalid task_type: %q", o.TaskType)
	}
	if !o.ComplexityTier.IsValid() {
		return fmt.Errorf("invalid complexity_tier: %q", o.ComplexityTier)
	}
	return s.store.CreateRoutingOutcome(ctx, o)
}

// GetStats returns model performance stats, optionally filtered.
func (s *RoutingService) GetStats(ctx context.Context, taskType, complexityTier string) ([]routing.ModelPerformanceStats, error) {
	return s.store.ListRoutingStats(ctx, taskType, complexityTier)
}

// RefreshStats triggers aggregation of routing outcomes into stats.
func (s *RoutingService) RefreshStats(ctx context.Context) error {
	return s.store.AggregateRoutingOutcomes(ctx)
}

// ListOutcomes returns recent routing outcomes.
func (s *RoutingService) ListOutcomes(ctx context.Context, limit int) ([]routing.RoutingOutcome, error) {
	return s.store.ListRoutingOutcomes(ctx, limit)
}

// UpsertStats inserts or updates model performance stats.
func (s *RoutingService) UpsertStats(ctx context.Context, st *routing.ModelPerformanceStats) error {
	if st.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	return s.store.UpsertRoutingStats(ctx, st)
}
