-- +goose Up
-- Review pipelines (KI-17): the plans the contract-first review pipeline
-- started and the commits their refactoring is measured and undone against.
-- The threshold HITL trusts this record, not the workspace: the refs there
-- (refs/codeforge/review/<plan>, refs/codeforge/review-result/<plan>) are
-- agent-writable and only keep the commits from git's garbage collection.
--   state: pending (no refactoring yet) -> refactoring (baseline recorded
--   when the refactorer step started) -> awaiting_decision (change measured,
--   keep or undo is up to the user) -> done.
CREATE TABLE IF NOT EXISTS review_pipelines (
    plan_id      UUID PRIMARY KEY REFERENCES execution_plans(id) ON DELETE CASCADE,
    tenant_id    UUID NOT NULL,
    project_id   UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    state        TEXT NOT NULL DEFAULT 'pending'
                 CHECK (state IN ('pending', 'refactoring', 'awaiting_decision', 'done')),
    baseline_sha TEXT NOT NULL DEFAULT '',
    result_sha   TEXT NOT NULL DEFAULT '',
    step_id      TEXT NOT NULL DEFAULT '',
    run_id       TEXT NOT NULL DEFAULT '',
    impact       JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_review_pipelines_tenant_project ON review_pipelines (tenant_id, project_id);

-- +goose Down
DROP TABLE IF EXISTS review_pipelines;
