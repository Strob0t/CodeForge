// Package service — RoutingService manages intelligent model routing state.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/benchmark"
	"github.com/Strob0t/CodeForge/internal/domain/routing"
	"github.com/Strob0t/CodeForge/internal/port/database"
	"github.com/Strob0t/CodeForge/internal/port/llm"
)

// RoutingService manages model routing state: outcomes, stats, and aggregation.
type RoutingService struct {
	store database.Store

	// In-memory stats cache.
	mu       sync.RWMutex
	cache    map[string][]routing.ModelPerformanceStats // key: "taskType:tier"
	cacheAt  time.Time
	cacheTTL time.Duration
}

// NewRoutingService creates a new RoutingService with a 5-minute stats cache.
func NewRoutingService(store database.Store) *RoutingService {
	return &RoutingService{
		store:    store,
		cache:    make(map[string][]routing.ModelPerformanceStats),
		cacheTTL: 5 * time.Minute,
	}
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
// Results are cached in-memory for the configured TTL.
func (s *RoutingService) GetStats(ctx context.Context, taskType, complexityTier string) ([]routing.ModelPerformanceStats, error) {
	key := taskType + ":" + complexityTier

	s.mu.RLock()
	if time.Since(s.cacheAt) < s.cacheTTL {
		if cached, ok := s.cache[key]; ok {
			s.mu.RUnlock()
			return cached, nil
		}
	}
	s.mu.RUnlock()

	stats, err := s.store.ListRoutingStats(ctx, taskType, complexityTier)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[key] = stats
	s.cacheAt = time.Now()
	s.mu.Unlock()

	return stats, nil
}

// RefreshStats triggers aggregation of routing outcomes into stats
// and invalidates the in-memory cache.
func (s *RoutingService) RefreshStats(ctx context.Context) error {
	err := s.store.AggregateRoutingOutcomes(ctx)
	if err != nil {
		return err
	}
	s.invalidateCache()
	return nil
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
	err := s.store.UpsertRoutingStats(ctx, st)
	if err != nil {
		return err
	}
	s.invalidateCache()
	return nil
}

// SeedFromBenchmarks reads all completed benchmark runs and converts their
// results into routing outcomes for the MAB to learn from.
func (s *RoutingService) SeedFromBenchmarks(ctx context.Context) (int, error) {
	runs, err := s.store.ListBenchmarkRuns(ctx)
	if err != nil {
		return 0, fmt.Errorf("list benchmark runs: %w", err)
	}

	total := 0
	for i := range runs {
		if runs[i].Status != benchmark.StatusCompleted {
			continue
		}
		n, err := s.SeedFromBenchmarkRun(ctx, runs[i].ID)
		if err != nil {
			slog.Warn("seed from benchmark run failed", "run_id", runs[i].ID, "error", err)
			continue
		}
		total += n
	}

	if total > 0 {
		if err := s.store.AggregateRoutingOutcomes(ctx); err != nil {
			return total, fmt.Errorf("aggregate after seed: %w", err)
		}
		s.invalidateCache()
	}

	return total, nil
}

// SeedFromBenchmarkRun converts a single benchmark run's results into routing
// outcomes. Returns the number of outcomes created.
func (s *RoutingService) SeedFromBenchmarkRun(ctx context.Context, runID string) (int, error) {
	run, err := s.store.GetBenchmarkRun(ctx, runID)
	if err != nil {
		return 0, fmt.Errorf("get benchmark run: %w", err)
	}

	results, err := s.store.ListBenchmarkResults(ctx, runID)
	if err != nil {
		return 0, fmt.Errorf("list benchmark results: %w", err)
	}

	count := 0
	for i := range results {
		quality := avgScoreFromScores(results[i].Scores)
		success := quality > 0

		// Estimate complexity tier from benchmark type.
		tier := routing.TierMedium
		switch run.BenchmarkType {
		case benchmark.TypeSimple:
			tier = routing.TierSimple
		case benchmark.TypeToolUse:
			tier = routing.TierComplex
		case benchmark.TypeAgent:
			tier = routing.TierReasoning
		}

		outcome := &routing.RoutingOutcome{
			ModelName:      run.Model,
			TaskType:       routing.TaskCode,
			ComplexityTier: tier,
			Success:        success,
			QualityScore:   quality,
			CostUSD:        results[i].CostUSD,
			LatencyMs:      results[i].DurationMs,
			TokensIn:       results[i].TokensIn,
			TokensOut:      results[i].TokensOut,
			Reward:         computeReward(success, quality, results[i].CostUSD, results[i].DurationMs),
			RoutingLayer:   "benchmark",
			RunID:          runID,
		}

		if err := s.store.CreateRoutingOutcome(ctx, outcome); err != nil {
			slog.Warn("seed routing outcome failed", "run_id", runID, "task_id", results[i].TaskID, "error", err)
			continue
		}
		count++
	}

	return count, nil
}

// SyncModelCapabilities updates model performance stats with capability
// metadata from discovered models (tools, vision, context, cost).
func (s *RoutingService) SyncModelCapabilities(ctx context.Context, models []llm.DiscoveredModel) error {
	for i := range models {
		if models[i].Status != "reachable" {
			continue
		}

		supportsTools := false
		supportsVision := false
		if info := models[i].ModelInfo; info != nil {
			if v, ok := info["supports_function_calling"]; ok {
				supportsTools, _ = v.(bool)
			}
			if v, ok := info["supports_vision"]; ok {
				supportsVision, _ = v.(bool)
			}
		}

		// Upsert a baseline stats record for every known task type.
		for _, taskType := range []routing.TaskType{
			routing.TaskCode, routing.TaskReview, routing.TaskPlan,
			routing.TaskQA, routing.TaskChat, routing.TaskDebug, routing.TaskRefactor,
		} {
			for _, tier := range []routing.ComplexityTier{
				routing.TierSimple, routing.TierMedium, routing.TierComplex, routing.TierReasoning,
			} {
				existing, err := s.store.ListRoutingStats(ctx, string(taskType), string(tier))
				if err != nil {
					continue
				}

				// Check if this model already has stats for this combo.
				found := false
				for j := range existing {
					if existing[j].ModelName != models[i].ModelName {
						continue
					}
					found = true
					// Update capability fields only.
					existing[j].SupportsTools = supportsTools
					existing[j].SupportsVision = supportsVision
					existing[j].MaxContext = models[i].MaxTokens
					existing[j].InputCostPer = models[i].InputCostPer
					existing[j].OutputCostPer = models[i].OutputCostPer
					if err := s.store.UpsertRoutingStats(ctx, &existing[j]); err != nil {
						slog.Warn("sync model capabilities upsert failed", "model", models[i].ModelName, "error", err)
					}
					break
				}

				if !found {
					st := &routing.ModelPerformanceStats{
						ModelName:      models[i].ModelName,
						TaskType:       taskType,
						ComplexityTier: tier,
						SupportsTools:  supportsTools,
						SupportsVision: supportsVision,
						MaxContext:     models[i].MaxTokens,
						InputCostPer:   models[i].InputCostPer,
						OutputCostPer:  models[i].OutputCostPer,
					}
					if err := s.store.UpsertRoutingStats(ctx, st); err != nil {
						slog.Warn("sync model capabilities insert failed", "model", models[i].ModelName, "error", err)
					}
				}
			}
		}
	}

	s.invalidateCache()
	return nil
}

func (s *RoutingService) invalidateCache() {
	s.mu.Lock()
	s.cache = make(map[string][]routing.ModelPerformanceStats)
	s.cacheAt = time.Time{}
	s.mu.Unlock()
}

// avgScoreFromScores computes the average score from a JSON scores map.
func avgScoreFromScores(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var scores map[string]float64
	if err := json.Unmarshal(raw, &scores); err != nil {
		return 0
	}
	if len(scores) == 0 {
		return 0
	}
	var total float64
	for _, v := range scores {
		total += v
	}
	return total / float64(len(scores))
}

// computeReward computes a simple reward signal from quality, cost, and latency.
// Mirrors the Python reward.compute_reward logic with default weights.
func computeReward(success bool, quality, costUSD float64, latencyMs int64) float64 {
	if !success {
		return -0.5
	}

	const (
		qualityWeight = 0.5
		costWeight    = 0.3
		latencyWeight = 0.2
		maxCost       = 0.10
		maxLatency    = 30000.0
	)

	normCost := costUSD / maxCost
	if normCost > 1.0 {
		normCost = 1.0
	}
	normLatency := float64(latencyMs) / maxLatency
	if normLatency > 1.0 {
		normLatency = 1.0
	}

	return qualityWeight*quality - costWeight*normCost - latencyWeight*normLatency
}
