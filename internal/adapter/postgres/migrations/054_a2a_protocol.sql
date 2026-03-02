-- +goose Up

CREATE TABLE IF NOT EXISTS a2a_tasks (
    id TEXT PRIMARY KEY,
    context_id TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'submitted',
    direction TEXT NOT NULL DEFAULT 'inbound',
    skill_id TEXT NOT NULL DEFAULT '',
    trust_origin TEXT NOT NULL DEFAULT 'a2a',
    trust_level TEXT NOT NULL DEFAULT 'untrusted',
    source_addr TEXT NOT NULL DEFAULT '',
    project_id TEXT NOT NULL DEFAULT '',
    remote_agent_id TEXT NOT NULL DEFAULT '',
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    metadata JSONB NOT NULL DEFAULT '{}',
    history JSONB NOT NULL DEFAULT '[]',
    artifacts JSONB NOT NULL DEFAULT '[]',
    error_message TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_a2a_tasks_state ON a2a_tasks (state);
CREATE INDEX IF NOT EXISTS idx_a2a_tasks_context ON a2a_tasks (context_id);
CREATE INDEX IF NOT EXISTS idx_a2a_tasks_project ON a2a_tasks (project_id);
CREATE INDEX IF NOT EXISTS idx_a2a_tasks_tenant ON a2a_tasks (tenant_id);
CREATE INDEX IF NOT EXISTS idx_a2a_tasks_direction ON a2a_tasks (direction);

CREATE TABLE IF NOT EXISTS a2a_remote_agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    trust_level TEXT NOT NULL DEFAULT 'partial',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    skills TEXT[] NOT NULL DEFAULT '{}',
    last_seen TIMESTAMPTZ,
    card_json JSONB,
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_a2a_remote_agents_tenant ON a2a_remote_agents (tenant_id);
CREATE INDEX IF NOT EXISTS idx_a2a_remote_agents_enabled ON a2a_remote_agents (enabled);

CREATE TABLE IF NOT EXISTS a2a_push_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id TEXT NOT NULL REFERENCES a2a_tasks(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    token TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_a2a_push_task ON a2a_push_configs (task_id);

-- +goose Down

DROP TABLE IF EXISTS a2a_push_configs;
DROP TABLE IF EXISTS a2a_remote_agents;
DROP TABLE IF EXISTS a2a_tasks;
