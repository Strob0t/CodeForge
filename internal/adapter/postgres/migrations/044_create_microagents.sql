-- +goose Up
CREATE TABLE IF NOT EXISTS microagents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    project_id      TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('knowledge', 'repo', 'task')),
    trigger_pattern TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    prompt          TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, project_id, name)
);

CREATE INDEX idx_microagents_project ON microagents(tenant_id, project_id);
CREATE INDEX idx_microagents_enabled ON microagents(tenant_id, enabled) WHERE enabled = TRUE;

-- +goose Down
DROP TABLE IF EXISTS microagents;
