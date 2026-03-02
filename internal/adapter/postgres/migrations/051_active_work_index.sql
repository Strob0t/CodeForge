-- +goose Up

-- Partial composite index for active-work queries (ListActiveWork).
CREATE INDEX IF NOT EXISTS idx_tasks_project_active
    ON tasks(project_id, status) WHERE status IN ('queued', 'running');

-- Partial index for stale-task detection (ReleaseStaleWork).
CREATE INDEX IF NOT EXISTS idx_tasks_stale
    ON tasks(status, updated_at) WHERE status IN ('queued', 'running');

-- +goose Down

DROP INDEX IF EXISTS idx_tasks_stale;
DROP INDEX IF EXISTS idx_tasks_project_active;
