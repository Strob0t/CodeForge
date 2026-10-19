-- +goose Up
-- The stuck-work watchdog (KI-28) lists the runs waiting in quality_gate
-- whose last update is oldest (ListStaleRuns) on every sweep, across all
-- tenants. Few runs are ever in quality_gate, so a partial index keeps that
-- query from scanning the whole runs table.
CREATE INDEX IF NOT EXISTS idx_runs_quality_gate_updated_at
    ON runs (updated_at)
    WHERE status = 'quality_gate';

-- +goose Down
DROP INDEX IF EXISTS idx_runs_quality_gate_updated_at;
