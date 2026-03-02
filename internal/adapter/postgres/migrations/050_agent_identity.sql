-- +goose Up

-- Add identity fields to agents table.
ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS total_runs    INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS total_cost    DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    ADD COLUMN IF NOT EXISTS success_rate  DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    ADD COLUMN IF NOT EXISTS state         JSONB,
    ADD COLUMN IF NOT EXISTS capabilities  TEXT[],
    ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMPTZ;

-- Agent inbox for agent-to-agent messaging.
CREATE TABLE IF NOT EXISTS agent_inbox (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    from_agent TEXT NOT NULL DEFAULT '',
    content    TEXT NOT NULL,
    priority   INTEGER NOT NULL DEFAULT 0,
    read       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_inbox_agent_id ON agent_inbox(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_inbox_unread ON agent_inbox(agent_id) WHERE read = FALSE;

-- +goose Down
DROP TABLE IF EXISTS agent_inbox;
ALTER TABLE agents
    DROP COLUMN IF EXISTS total_runs,
    DROP COLUMN IF EXISTS total_cost,
    DROP COLUMN IF EXISTS success_rate,
    DROP COLUMN IF EXISTS state,
    DROP COLUMN IF EXISTS capabilities,
    DROP COLUMN IF EXISTS last_active_at;
