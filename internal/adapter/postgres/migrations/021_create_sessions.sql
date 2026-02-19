-- +goose Up

CREATE TABLE IF NOT EXISTS sessions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    project_id        UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    task_id           UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    parent_session_id UUID REFERENCES sessions(id) ON DELETE SET NULL,
    parent_run_id     UUID REFERENCES runs(id) ON DELETE SET NULL,
    current_run_id    UUID REFERENCES runs(id) ON DELETE SET NULL,
    status            TEXT NOT NULL DEFAULT 'active',
    metadata          JSONB DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sessions_tenant_id ON sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sessions_project_id ON sessions(project_id);
CREATE INDEX IF NOT EXISTS idx_sessions_task_id ON sessions(task_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);

-- Auto-update updated_at
CREATE OR REPLACE TRIGGER sessions_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Audit trail table for high-level actions across the system
CREATE TABLE IF NOT EXISTS audit_trail (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    project_id UUID NOT NULL,
    run_id     UUID,
    agent_id   UUID,
    action     TEXT NOT NULL,
    details    TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_trail_tenant_id ON audit_trail(tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_trail_project_id ON audit_trail(project_id);
CREATE INDEX IF NOT EXISTS idx_audit_trail_run_id ON audit_trail(run_id);
CREATE INDEX IF NOT EXISTS idx_audit_trail_action ON audit_trail(action);
CREATE INDEX IF NOT EXISTS idx_audit_trail_created_at ON audit_trail(created_at DESC);

-- +goose Down

DROP TABLE IF EXISTS audit_trail;
DROP TABLE IF EXISTS sessions;
