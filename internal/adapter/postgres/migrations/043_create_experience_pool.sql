-- +goose Up
CREATE TABLE IF NOT EXISTS experience_entries (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_description TEXT NOT NULL,
    task_embedding  BYTEA,
    result_output   TEXT NOT NULL,
    result_cost     REAL NOT NULL DEFAULT 0,
    result_status   TEXT NOT NULL DEFAULT 'success',
    run_id          TEXT NOT NULL DEFAULT '',
    confidence      REAL NOT NULL DEFAULT 0,
    hit_count       INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_experience_entries_project ON experience_entries(project_id);
CREATE INDEX idx_experience_entries_last_used ON experience_entries(project_id, last_used_at DESC);

-- +goose Down
DROP TABLE IF EXISTS experience_entries;
