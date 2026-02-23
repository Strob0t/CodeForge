-- +goose Up

CREATE TABLE review_policies (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id        TEXT NOT NULL DEFAULT '',
    name             TEXT NOT NULL,
    trigger_type     TEXT NOT NULL CHECK (trigger_type IN ('commit_count', 'pre_merge', 'cron')),
    commit_threshold INT NOT NULL DEFAULT 0,
    cron_expr        TEXT NOT NULL DEFAULT '',
    branch_pattern   TEXT NOT NULL DEFAULT '',
    template_id      TEXT NOT NULL DEFAULT 'review-only',
    enabled          BOOLEAN NOT NULL DEFAULT true,
    commit_counter   INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_review_policies_project ON review_policies (project_id) WHERE enabled = true;
CREATE INDEX idx_review_policies_trigger ON review_policies (trigger_type) WHERE enabled = true;

CREATE TABLE reviews (
    id           TEXT PRIMARY KEY,
    policy_id    TEXT NOT NULL REFERENCES review_policies(id) ON DELETE CASCADE,
    project_id   TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id    TEXT NOT NULL DEFAULT '',
    plan_id      TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    trigger_ref  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_reviews_project ON reviews (project_id, created_at DESC);
CREATE INDEX idx_reviews_policy  ON reviews (policy_id, created_at DESC);
CREATE INDEX idx_reviews_plan    ON reviews (plan_id) WHERE plan_id != '';

-- +goose Down

DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS review_policies;
