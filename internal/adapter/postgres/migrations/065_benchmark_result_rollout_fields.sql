-- +goose Up
-- Phase 28C: Add rollout fields to benchmark_results and benchmark_runs.

ALTER TABLE benchmark_results ADD COLUMN rollout_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE benchmark_results ADD COLUMN rollout_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE benchmark_results ADD COLUMN is_best_rollout BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE benchmark_results ADD COLUMN diversity_score DOUBLE PRECISION NOT NULL DEFAULT 0.0;

ALTER TABLE benchmark_runs ADD COLUMN hybrid_verification BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE benchmark_runs ADD COLUMN rollout_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE benchmark_runs ADD COLUMN rollout_strategy TEXT NOT NULL DEFAULT 'best';

-- +goose Down
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS rollout_id;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS rollout_count;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS is_best_rollout;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS diversity_score;

ALTER TABLE benchmark_runs DROP COLUMN IF EXISTS hybrid_verification;
ALTER TABLE benchmark_runs DROP COLUMN IF EXISTS rollout_count;
ALTER TABLE benchmark_runs DROP COLUMN IF EXISTS rollout_strategy;
