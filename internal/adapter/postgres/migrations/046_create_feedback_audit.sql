-- +goose Up
CREATE TABLE IF NOT EXISTS feedback_audit (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    run_id           TEXT NOT NULL,
    call_id          TEXT NOT NULL,
    tool             TEXT NOT NULL,
    provider         TEXT NOT NULL CHECK (provider IN ('web', 'slack', 'email')),
    decision         TEXT NOT NULL CHECK (decision IN ('allow', 'deny')),
    responder        TEXT NOT NULL DEFAULT '',
    response_time_ms INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_feedback_audit_run ON feedback_audit(run_id);

-- +goose Down
DROP TABLE IF EXISTS feedback_audit;
