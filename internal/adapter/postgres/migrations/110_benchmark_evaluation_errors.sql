-- +goose Up
-- S6-G review (item 3): the dimensions an evaluator could not score are kept
-- apart from the scores (dimension -> error), so no average counts them.
ALTER TABLE benchmark_results
    ADD COLUMN evaluation_errors JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS evaluation_errors;
