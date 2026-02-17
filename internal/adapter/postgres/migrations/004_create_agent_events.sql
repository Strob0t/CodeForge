-- +goose Up

CREATE TABLE agent_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id   UUID NOT NULL,
    task_id    UUID NOT NULL,
    project_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload    JSONB NOT NULL DEFAULT '{}',
    request_id TEXT NOT NULL DEFAULT '',
    version    INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_events_task_id ON agent_events (task_id, version);
CREATE INDEX idx_agent_events_agent_id ON agent_events (agent_id, version);
CREATE INDEX idx_agent_events_project_id ON agent_events (project_id, created_at);

-- +goose Down

DROP TABLE IF EXISTS agent_events;
