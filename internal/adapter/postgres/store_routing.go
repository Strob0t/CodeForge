package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Strob0t/CodeForge/internal/domain/routing"
)

// CreateRoutingOutcome inserts a single routing outcome record.
func (s *Store) CreateRoutingOutcome(ctx context.Context, o *routing.RoutingOutcome) error {
	const q = `
		INSERT INTO model_routing_outcomes
			(tenant_id, model_name, task_type, complexity_tier,
			 success, quality_score, cost_usd, latency_ms,
			 tokens_in, tokens_out, reward,
			 routing_layer, run_id, conversation_id, prompt_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, created_at`

	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), o.ModelName, o.TaskType, o.ComplexityTier,
		o.Success, o.QualityScore, o.CostUSD, o.LatencyMs,
		o.TokensIn, o.TokensOut, o.Reward,
		o.RoutingLayer, o.RunID, o.ConversationID, o.PromptHash,
	).Scan(&o.ID, &o.CreatedAt)
}

// ListRoutingStats returns model performance stats, optionally filtered.
// Empty taskType or complexityTier means no filter on that dimension.
func (s *Store) ListRoutingStats(ctx context.Context, taskType, complexityTier string) ([]routing.ModelPerformanceStats, error) {
	q := `
		SELECT id, model_name, task_type, complexity_tier,
		       trial_count, total_reward, avg_reward, avg_cost_usd, avg_latency_ms, avg_quality,
		       last_selected,
		       supports_tools, supports_vision, max_context, input_cost_per, output_cost_per,
		       created_at, updated_at
		FROM model_performance_stats
		WHERE tenant_id = $1`

	args := []any{tenantFromCtx(ctx)}
	argIdx := 2

	if taskType != "" {
		q += fmt.Sprintf(" AND task_type = $%d", argIdx)
		args = append(args, taskType)
		argIdx++
	}
	if complexityTier != "" {
		q += fmt.Sprintf(" AND complexity_tier = $%d", argIdx)
		args = append(args, complexityTier)
	}

	q += " ORDER BY model_name"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list routing stats: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (routing.ModelPerformanceStats, error) {
		var st routing.ModelPerformanceStats
		err := r.Scan(
			&st.ID, &st.ModelName, &st.TaskType, &st.ComplexityTier,
			&st.TrialCount, &st.TotalReward, &st.AvgReward, &st.AvgCostUSD, &st.AvgLatencyMs, &st.AvgQuality,
			&st.LastSelected,
			&st.SupportsTools, &st.SupportsVision, &st.MaxContext, &st.InputCostPer, &st.OutputCostPer,
			&st.CreatedAt, &st.UpdatedAt,
		)
		return st, err
	})
}

// UpsertRoutingStats inserts or updates model performance stats.
func (s *Store) UpsertRoutingStats(ctx context.Context, st *routing.ModelPerformanceStats) error {
	const q = `
		INSERT INTO model_performance_stats
			(tenant_id, model_name, task_type, complexity_tier,
			 trial_count, total_reward, avg_reward, avg_cost_usd, avg_latency_ms, avg_quality,
			 supports_tools, supports_vision, max_context, input_cost_per, output_cost_per)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (tenant_id, model_name, task_type, complexity_tier)
		DO UPDATE SET
			trial_count = EXCLUDED.trial_count,
			total_reward = EXCLUDED.total_reward,
			avg_reward = EXCLUDED.avg_reward,
			avg_cost_usd = EXCLUDED.avg_cost_usd,
			avg_latency_ms = EXCLUDED.avg_latency_ms,
			avg_quality = EXCLUDED.avg_quality,
			supports_tools = EXCLUDED.supports_tools,
			supports_vision = EXCLUDED.supports_vision,
			max_context = EXCLUDED.max_context,
			input_cost_per = EXCLUDED.input_cost_per,
			output_cost_per = EXCLUDED.output_cost_per,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	return s.pool.QueryRow(ctx, q,
		tenantFromCtx(ctx), st.ModelName, st.TaskType, st.ComplexityTier,
		st.TrialCount, st.TotalReward, st.AvgReward, st.AvgCostUSD, st.AvgLatencyMs, st.AvgQuality,
		st.SupportsTools, st.SupportsVision, st.MaxContext, st.InputCostPer, st.OutputCostPer,
	).Scan(&st.ID, &st.CreatedAt, &st.UpdatedAt)
}

// AggregateRoutingOutcomes computes aggregate statistics from routing outcomes
// and upserts them into model_performance_stats.
func (s *Store) AggregateRoutingOutcomes(ctx context.Context) error {
	const q = `
		INSERT INTO model_performance_stats
			(tenant_id, model_name, task_type, complexity_tier,
			 trial_count, total_reward, avg_reward, avg_cost_usd, avg_latency_ms, avg_quality)
		SELECT
			tenant_id,
			model_name,
			task_type,
			complexity_tier,
			COUNT(*),
			SUM(reward),
			AVG(reward),
			AVG(cost_usd),
			AVG(latency_ms)::BIGINT,
			AVG(quality_score)
		FROM model_routing_outcomes
		GROUP BY tenant_id, model_name, task_type, complexity_tier
		ON CONFLICT (tenant_id, model_name, task_type, complexity_tier)
		DO UPDATE SET
			trial_count = EXCLUDED.trial_count,
			total_reward = EXCLUDED.total_reward,
			avg_reward = EXCLUDED.avg_reward,
			avg_cost_usd = EXCLUDED.avg_cost_usd,
			avg_latency_ms = EXCLUDED.avg_latency_ms,
			avg_quality = EXCLUDED.avg_quality,
			updated_at = NOW()`

	_, err := s.pool.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("aggregate routing outcomes: %w", err)
	}
	return nil
}

// ListRoutingOutcomes returns recent routing outcomes, ordered by created_at DESC.
func (s *Store) ListRoutingOutcomes(ctx context.Context, limit int) ([]routing.RoutingOutcome, error) {
	if limit <= 0 {
		limit = 50
	}

	const q = `
		SELECT id, model_name, task_type, complexity_tier,
		       success, quality_score, cost_usd, latency_ms,
		       tokens_in, tokens_out, reward,
		       routing_layer, run_id, conversation_id, prompt_hash,
		       created_at
		FROM model_routing_outcomes
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := s.pool.Query(ctx, q, tenantFromCtx(ctx), limit)
	if err != nil {
		return nil, fmt.Errorf("list routing outcomes: %w", err)
	}
	return scanRows(rows, func(r pgx.Rows) (routing.RoutingOutcome, error) {
		var o routing.RoutingOutcome
		err := r.Scan(
			&o.ID, &o.ModelName, &o.TaskType, &o.ComplexityTier,
			&o.Success, &o.QualityScore, &o.CostUSD, &o.LatencyMs,
			&o.TokensIn, &o.TokensOut, &o.Reward,
			&o.RoutingLayer, &o.RunID, &o.ConversationID, &o.PromptHash,
			&o.CreatedAt,
		)
		return o, err
	})
}
