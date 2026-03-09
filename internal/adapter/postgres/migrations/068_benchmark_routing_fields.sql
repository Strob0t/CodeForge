-- +goose Up
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS selected_model TEXT DEFAULT '';
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS routing_reason TEXT DEFAULT '';
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS fallback_chain TEXT DEFAULT '';
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS fallback_count INTEGER DEFAULT 0;
ALTER TABLE benchmark_results ADD COLUMN IF NOT EXISTS provider_errors TEXT DEFAULT '';

-- +goose Down
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS selected_model;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS routing_reason;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS fallback_chain;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS fallback_count;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS provider_errors;
