-- +goose Up
CREATE TABLE IF NOT EXISTS agent_memories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    agent_id    TEXT NOT NULL DEFAULT '',
    run_id      TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'observation'
                CHECK (kind IN ('observation', 'decision', 'error', 'insight')),
    importance  REAL NOT NULL DEFAULT 0.5,
    embedding   BYTEA,
    metadata    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_memories_project ON agent_memories(project_id);
CREATE INDEX idx_agent_memories_kind ON agent_memories(project_id, kind);

-- +goose Down
DROP TABLE IF EXISTS agent_memories;
