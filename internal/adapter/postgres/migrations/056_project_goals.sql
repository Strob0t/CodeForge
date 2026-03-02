-- +goose Up
CREATE TABLE project_goals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('vision','requirement','constraint','state','context')),
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'manual',
    source_path TEXT NOT NULL DEFAULT '',
    priority    INTEGER NOT NULL DEFAULT 90,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_project_goals_project ON project_goals(project_id);
CREATE INDEX idx_project_goals_enabled ON project_goals(project_id) WHERE enabled = TRUE;

-- +goose Down
DROP TABLE IF EXISTS project_goals;
