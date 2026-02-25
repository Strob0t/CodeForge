-- +goose Up

CREATE TABLE benchmark_runs (
    id             TEXT PRIMARY KEY,
    dataset        TEXT        NOT NULL,
    model          TEXT        NOT NULL,
    metrics        TEXT[]      NOT NULL DEFAULT '{}',
    status         TEXT        NOT NULL DEFAULT 'running'
                       CHECK (status IN ('running', 'completed', 'failed')),
    summary_scores JSONB       NOT NULL DEFAULT '{}',
    total_cost     DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_tokens   INTEGER     NOT NULL DEFAULT 0,
    total_duration_ms BIGINT   NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at   TIMESTAMPTZ
);

CREATE TABLE benchmark_results (
    id              TEXT PRIMARY KEY,
    run_id          TEXT NOT NULL REFERENCES benchmark_runs(id) ON DELETE CASCADE,
    task_id         TEXT NOT NULL,
    task_name       TEXT NOT NULL,
    scores          JSONB NOT NULL DEFAULT '{}',
    actual_output   TEXT  NOT NULL DEFAULT '',
    expected_output TEXT  NOT NULL DEFAULT '',
    tool_calls      JSONB NOT NULL DEFAULT '[]',
    cost_usd        DOUBLE PRECISION NOT NULL DEFAULT 0,
    tokens_in       INTEGER NOT NULL DEFAULT 0,
    tokens_out      INTEGER NOT NULL DEFAULT 0,
    duration_ms     BIGINT  NOT NULL DEFAULT 0
);

CREATE INDEX idx_benchmark_results_run_id ON benchmark_results(run_id);

-- +goose Down

DROP TABLE IF EXISTS benchmark_results;
DROP TABLE IF EXISTS benchmark_runs;
