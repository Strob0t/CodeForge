-- +goose Up
-- KI-96 D11: a deleted project's workspace is removed by the worker as the
-- tenant's tool UID. The project row's removal and this record are one
-- transaction; the Go Core publishes workspace.delete.request, and again for
-- pending rows every 10 minutes, until the worker reports the deletion done.
-- No foreign key to projects: the project row is gone.
CREATE TABLE workspace_deletions (
    id             UUID        PRIMARY KEY,
    tenant_id      UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id     UUID        NOT NULL,
    workspace_path TEXT        NOT NULL,
    tool_uid       INTEGER     NOT NULL CHECK (tool_uid BETWEEN 20000 AND 29999),
    requested_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    done_at        TIMESTAMPTZ,
    attempts       INTEGER     NOT NULL DEFAULT 0,
    last_error     TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX idx_workspace_deletions_pending ON workspace_deletions (requested_at) WHERE done_at IS NULL;
CREATE INDEX idx_workspace_deletions_tenant ON workspace_deletions (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS workspace_deletions;
