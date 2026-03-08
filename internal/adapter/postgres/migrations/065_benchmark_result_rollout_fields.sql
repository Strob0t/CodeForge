-- +goose Up
-- Phase 28C: Add rollout fields to benchmark_results and benchmark_runs.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_results' AND column_name='rollout_id') THEN
        ALTER TABLE benchmark_results ADD COLUMN rollout_id INTEGER NOT NULL DEFAULT 0;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_results' AND column_name='rollout_count') THEN
        ALTER TABLE benchmark_results ADD COLUMN rollout_count INTEGER NOT NULL DEFAULT 1;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_results' AND column_name='is_best_rollout') THEN
        ALTER TABLE benchmark_results ADD COLUMN is_best_rollout BOOLEAN NOT NULL DEFAULT TRUE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_results' AND column_name='diversity_score') THEN
        ALTER TABLE benchmark_results ADD COLUMN diversity_score DOUBLE PRECISION NOT NULL DEFAULT 0.0;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_runs' AND column_name='hybrid_verification') THEN
        ALTER TABLE benchmark_runs ADD COLUMN hybrid_verification BOOLEAN NOT NULL DEFAULT FALSE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_runs' AND column_name='rollout_count') THEN
        ALTER TABLE benchmark_runs ADD COLUMN rollout_count INTEGER NOT NULL DEFAULT 1;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='benchmark_runs' AND column_name='rollout_strategy') THEN
        ALTER TABLE benchmark_runs ADD COLUMN rollout_strategy TEXT NOT NULL DEFAULT 'best';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS rollout_id;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS rollout_count;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS is_best_rollout;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS diversity_score;

ALTER TABLE benchmark_runs DROP COLUMN IF EXISTS hybrid_verification;
ALTER TABLE benchmark_runs DROP COLUMN IF EXISTS rollout_count;
ALTER TABLE benchmark_runs DROP COLUMN IF EXISTS rollout_strategy;
