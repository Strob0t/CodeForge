package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// PromptScoreStore abstracts persistence for prompt scores.
type PromptScoreStore interface {
	InsertPromptScore(ctx context.Context, score *prompt.PromptScore) error
	GetScoresByFingerprint(ctx context.Context, fingerprint string) ([]prompt.PromptScore, error)
	GetAggregatedScores(ctx context.Context, tenantID, modeID, modelFamily string) (map[string]map[prompt.SignalType]float64, error)
}

// PromptScoreCollector aggregates multi-signal scores for prompt variants.
type PromptScoreCollector struct {
	store PromptScoreStore
	mu    sync.RWMutex
	// In-memory cache of recent scores for fast composite calculation.
	cache map[string][]prompt.PromptScore // key: prompt_fingerprint
}

// NewPromptScoreCollector creates a new score collector.
func NewPromptScoreCollector(store PromptScoreStore) *PromptScoreCollector {
	return &PromptScoreCollector{
		store: store,
		cache: make(map[string][]prompt.PromptScore),
	}
}

// RecordScore records a single signal score for a prompt fingerprint.
func (c *PromptScoreCollector) RecordScore(ctx context.Context, score *prompt.PromptScore) error {
	if score.PromptFingerprint == "" {
		return nil // skip if no fingerprint attached
	}
	if !prompt.ValidSignalType(score.SignalType) {
		slog.Warn("invalid signal type, skipping", "signal_type", score.SignalType)
		return nil
	}
	if score.CreatedAt.IsZero() {
		score.CreatedAt = time.Now()
	}

	if err := c.store.InsertPromptScore(ctx, score); err != nil {
		return err
	}

	// Update in-memory cache.
	c.mu.Lock()
	c.cache[score.PromptFingerprint] = append(c.cache[score.PromptFingerprint], *score)
	c.mu.Unlock()

	return nil
}

// RecordBenchmarkScore is a convenience method for recording benchmark quality scores.
func (c *PromptScoreCollector) RecordBenchmarkScore(ctx context.Context, tenantID, fingerprint, modeID, modelFamily, runID string, score float64) error {
	return c.RecordScore(ctx, &prompt.PromptScore{
		TenantID:          tenantID,
		PromptFingerprint: fingerprint,
		ModeID:            modeID,
		ModelFamily:       modelFamily,
		SignalType:        prompt.SignalBenchmark,
		Score:             score,
		RunID:             runID,
	})
}

// RecordSuccessScore records whether a run succeeded or failed.
func (c *PromptScoreCollector) RecordSuccessScore(ctx context.Context, tenantID, fingerprint, modeID, modelFamily, runID string, succeeded bool) error {
	score := 0.0
	if succeeded {
		score = 1.0
	}
	return c.RecordScore(ctx, &prompt.PromptScore{
		TenantID:          tenantID,
		PromptFingerprint: fingerprint,
		ModeID:            modeID,
		ModelFamily:       modelFamily,
		SignalType:        prompt.SignalSuccess,
		Score:             score,
		RunID:             runID,
	})
}

// RecordCostScore records cost efficiency (quality / cost_usd).
func (c *PromptScoreCollector) RecordCostScore(ctx context.Context, tenantID, fingerprint, modeID, modelFamily, runID string, qualityPerDollar float64) error {
	// Normalize to 0-1 range: tanh(quality_per_dollar / 100) as a soft cap.
	normalized := qualityPerDollar
	if normalized > 1.0 {
		normalized = 1.0
	}
	if normalized < 0 {
		normalized = 0
	}
	return c.RecordScore(ctx, &prompt.PromptScore{
		TenantID:          tenantID,
		PromptFingerprint: fingerprint,
		ModeID:            modeID,
		ModelFamily:       modelFamily,
		SignalType:        prompt.SignalCost,
		Score:             normalized,
		RunID:             runID,
	})
}

// RecordUserFeedback records a user thumbs-up (1.0) or thumbs-down (0.0).
func (c *PromptScoreCollector) RecordUserFeedback(ctx context.Context, tenantID, fingerprint, modeID, modelFamily, runID string, positive bool) error {
	score := 0.0
	if positive {
		score = 1.0
	}
	return c.RecordScore(ctx, &prompt.PromptScore{
		TenantID:          tenantID,
		PromptFingerprint: fingerprint,
		ModeID:            modeID,
		ModelFamily:       modelFamily,
		SignalType:        prompt.SignalUser,
		Score:             score,
		RunID:             runID,
	})
}

// RecordEfficiencyScore records stall/efficiency signal (normalized step count).
func (c *PromptScoreCollector) RecordEfficiencyScore(ctx context.Context, tenantID, fingerprint, modeID, modelFamily, runID string, score float64) error {
	return c.RecordScore(ctx, &prompt.PromptScore{
		TenantID:          tenantID,
		PromptFingerprint: fingerprint,
		ModeID:            modeID,
		ModelFamily:       modelFamily,
		SignalType:        prompt.SignalEfficiency,
		Score:             score,
		RunID:             runID,
	})
}

// CompositeScoreForFingerprint computes the weighted composite score for a given fingerprint
// from in-memory cache. Returns 0 and false if no scores found.
func (c *PromptScoreCollector) CompositeScoreForFingerprint(fingerprint string) (float64, bool) {
	c.mu.RLock()
	scores, ok := c.cache[fingerprint]
	c.mu.RUnlock()
	if !ok || len(scores) == 0 {
		return 0, false
	}

	// Aggregate: average each signal type, then compute weighted composite.
	sums := make(map[prompt.SignalType]float64)
	counts := make(map[prompt.SignalType]int)
	for i := range scores {
		sums[scores[i].SignalType] += scores[i].Score
		counts[scores[i].SignalType]++
	}

	avgs := make(map[prompt.SignalType]float64)
	for signal, sum := range sums {
		avgs[signal] = sum / float64(counts[signal])
	}

	return prompt.CompositeScore(avgs), true
}

// ScoreCountForFingerprint returns the total number of score signals for a fingerprint.
func (c *PromptScoreCollector) ScoreCountForFingerprint(fingerprint string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache[fingerprint])
}
