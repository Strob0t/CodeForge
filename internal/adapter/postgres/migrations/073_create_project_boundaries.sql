-- +goose Up
CREATE TABLE project_boundaries (
    project_id   UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id    UUID        NOT NULL,
    boundaries   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    last_analyzed TIMESTAMPTZ,
    version      INT         NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id)
);

CREATE INDEX idx_project_boundaries_tenant ON project_boundaries(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS project_boundaries;
