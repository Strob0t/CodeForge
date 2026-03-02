-- +goose Up
CREATE TABLE quarantine_messages (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    TEXT NOT NULL,
    subject       TEXT NOT NULL,
    payload       BYTEA NOT NULL,
    trust_origin  TEXT NOT NULL DEFAULT '',
    trust_level   TEXT NOT NULL DEFAULT '',
    risk_score    DOUBLE PRECISION NOT NULL DEFAULT 0,
    risk_factors  TEXT[] NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'pending',
    reviewed_by   TEXT NOT NULL DEFAULT '',
    review_note   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at   TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_quarantine_status ON quarantine_messages(status);
CREATE INDEX idx_quarantine_project ON quarantine_messages(project_id);

-- +goose Down
DROP TABLE IF EXISTS quarantine_messages;
