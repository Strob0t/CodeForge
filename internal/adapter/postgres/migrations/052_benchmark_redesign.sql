-- +goose Up

-- Benchmark suites: registered benchmark providers (HumanEval, SWE-bench, custom).
CREATE TABLE benchmark_suites (
    id            TEXT PRIMARY KEY,
    name          TEXT        NOT NULL,
    description   TEXT        NOT NULL DEFAULT '',
    type          TEXT        NOT NULL CHECK (type IN ('simple', 'tool_use', 'agent')),
    provider_name TEXT        NOT NULL,
    task_count    INTEGER     NOT NULL DEFAULT 0,
    config        JSONB       NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_benchmark_suites_type ON benchmark_suites(type);

-- Extend benchmark_runs with Phase 26 columns (nullable for backward compatibility).
ALTER TABLE benchmark_runs
    ADD COLUMN suite_id       TEXT REFERENCES benchmark_suites(id) ON DELETE SET NULL,
    ADD COLUMN benchmark_type TEXT DEFAULT '' CHECK (benchmark_type IN ('', 'simple', 'tool_use', 'agent')),
    ADD COLUMN exec_mode      TEXT DEFAULT '' CHECK (exec_mode IN ('', 'mount', 'sandbox', 'hybrid')),
    ADD COLUMN config         JSONB DEFAULT '{}';

CREATE INDEX idx_benchmark_runs_suite_id ON benchmark_runs(suite_id);
CREATE INDEX idx_benchmark_runs_type ON benchmark_runs(benchmark_type);

-- Extend benchmark_results with Phase 26 columns.
ALTER TABLE benchmark_results
    ADD COLUMN evaluator_scores       JSONB DEFAULT '{}',
    ADD COLUMN files_changed          TEXT[] DEFAULT '{}',
    ADD COLUMN functional_test_output TEXT  DEFAULT '';

-- +goose Down

ALTER TABLE benchmark_results
    DROP COLUMN IF EXISTS functional_test_output,
    DROP COLUMN IF EXISTS files_changed,
    DROP COLUMN IF EXISTS evaluator_scores;

ALTER TABLE benchmark_runs
    DROP COLUMN IF EXISTS config,
    DROP COLUMN IF EXISTS exec_mode,
    DROP COLUMN IF EXISTS benchmark_type,
    DROP COLUMN IF EXISTS suite_id;

DROP INDEX IF EXISTS idx_benchmark_runs_type;
DROP INDEX IF EXISTS idx_benchmark_runs_suite_id;
DROP INDEX IF EXISTS idx_benchmark_suites_type;
DROP TABLE IF EXISTS benchmark_suites;
