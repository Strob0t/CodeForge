-- +goose Up
-- Phase 28C: Multi-rollout test-time scaling columns.

ALTER TABLE benchmark_results
    ADD COLUMN IF NOT EXISTS rollout_id       INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS rollout_count    INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS is_best_rollout  BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS diversity_score  DOUBLE PRECISION NOT NULL DEFAULT 0.0;

CREATE INDEX IF NOT EXISTS idx_bench_results_rollout
    ON benchmark_results(run_id, task_id, rollout_id);

ALTER TABLE benchmark_runs
    ADD COLUMN IF NOT EXISTS rollout_count    INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS rollout_strategy TEXT NOT NULL DEFAULT 'best';

-- +goose Down
ALTER TABLE benchmark_results
    DROP COLUMN IF EXISTS rollout_id,
    DROP COLUMN IF EXISTS rollout_count,
    DROP COLUMN IF EXISTS is_best_rollout,
    DROP COLUMN IF EXISTS diversity_score;

DROP INDEX IF EXISTS idx_bench_results_rollout;

ALTER TABLE benchmark_runs
    DROP COLUMN IF EXISTS rollout_count,
    DROP COLUMN IF EXISTS rollout_strategy;
