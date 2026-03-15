-- +goose Up
-- Add error_message column to benchmark_runs for storing failure reasons.
ALTER TABLE benchmark_runs
    ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE benchmark_runs
    DROP COLUMN IF EXISTS error_message;
