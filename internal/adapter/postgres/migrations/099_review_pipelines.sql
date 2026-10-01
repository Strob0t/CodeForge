-- +goose Up
-- Review pipelines (KI-17): the plans the contract-first review pipeline
-- started and the baseline commit of their refactoring. The threshold HITL
-- trusts this record, not the workspace: the baseline ref there
-- (refs/codeforge/review/<plan>) is agent-writable and only keeps the
-- baseline commit from git's garbage collection.
CREATE TABLE IF NOT EXISTS review_pipelines (
    plan_id      UUID PRIMARY KEY REFERENCES execution_plans(id) ON DELETE CASCADE,
    tenant_id    UUID NOT NULL,
    project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    baseline_sha TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_review_pipelines_tenant_project ON review_pipelines (tenant_id, project_id);

-- +goose Down
DROP TABLE IF EXISTS review_pipelines;
