-- +goose Up
CREATE TABLE review_triggers (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id    UUID        NOT NULL,
    commit_sha   TEXT        NOT NULL,
    source       TEXT        NOT NULL CHECK (source IN ('pipeline-completion', 'branch-merge', 'manual')),
    plan_id      UUID,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_review_triggers_dedup
    ON review_triggers(project_id, commit_sha, triggered_at DESC);
CREATE INDEX idx_review_triggers_tenant
    ON review_triggers(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS review_triggers;
