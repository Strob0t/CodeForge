-- +goose Up
CREATE TABLE mcp_servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    name VARCHAR(128) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    transport VARCHAR(16) NOT NULL CHECK (transport IN ('stdio', 'sse')),
    command VARCHAR(512) NOT NULL DEFAULT '',
    args JSONB NOT NULL DEFAULT '[]',
    url VARCHAR(2048) NOT NULL DEFAULT '',
    env JSONB NOT NULL DEFAULT '{}',
    headers JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    status VARCHAR(32) NOT NULL DEFAULT 'registered',
    last_health_check TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE project_mcp_servers (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    mcp_server_id UUID NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
    PRIMARY KEY (project_id, mcp_server_id)
);

CREATE TABLE mcp_server_tools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    input_schema JSONB NOT NULL DEFAULT '{}',
    UNIQUE (server_id, name)
);

-- +goose Down
DROP TABLE IF EXISTS mcp_server_tools;
DROP TABLE IF EXISTS project_mcp_servers;
DROP TABLE IF EXISTS mcp_servers;
