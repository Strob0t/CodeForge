-- +goose Up
CREATE TABLE IF NOT EXISTS auto_agents (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id         UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status             TEXT NOT NULL DEFAULT 'idle',
    current_feature_id TEXT NOT NULL DEFAULT '',
    conversation_id    TEXT NOT NULL DEFAULT '',
    features_total     INT NOT NULL DEFAULT 0,
    features_complete  INT NOT NULL DEFAULT 0,
    features_failed    INT NOT NULL DEFAULT 0,
    total_cost_usd     DOUBLE PRECISION NOT NULL DEFAULT 0,
    error              TEXT NOT NULL DEFAULT '',
    started_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id)
);

CREATE INDEX idx_auto_agents_project ON auto_agents(project_id);

-- +goose Down
DROP TABLE IF EXISTS auto_agents;
