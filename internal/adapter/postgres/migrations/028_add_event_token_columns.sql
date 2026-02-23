-- +goose Up
-- Phase 12H: Per-tool token tracking on agent_events.
ALTER TABLE agent_events ADD COLUMN tool_name  TEXT             NOT NULL DEFAULT '';
ALTER TABLE agent_events ADD COLUMN model      TEXT             NOT NULL DEFAULT '';
ALTER TABLE agent_events ADD COLUMN tokens_in  BIGINT           NOT NULL DEFAULT 0;
ALTER TABLE agent_events ADD COLUMN tokens_out BIGINT           NOT NULL DEFAULT 0;
ALTER TABLE agent_events ADD COLUMN cost_usd   DOUBLE PRECISION NOT NULL DEFAULT 0;

-- Partial indexes for per-tool cost aggregation (only tool result events).
CREATE INDEX idx_agent_events_tool_cost ON agent_events (run_id, event_type, tool_name)
    WHERE event_type = 'run.toolcall.result';
CREATE INDEX idx_agent_events_project_tool ON agent_events (project_id, event_type, tool_name)
    WHERE event_type = 'run.toolcall.result';

-- +goose Down
DROP INDEX IF EXISTS idx_agent_events_project_tool;
DROP INDEX IF EXISTS idx_agent_events_tool_cost;
ALTER TABLE agent_events DROP COLUMN IF EXISTS cost_usd;
ALTER TABLE agent_events DROP COLUMN IF EXISTS tokens_out;
ALTER TABLE agent_events DROP COLUMN IF EXISTS tokens_in;
ALTER TABLE agent_events DROP COLUMN IF EXISTS model;
ALTER TABLE agent_events DROP COLUMN IF EXISTS tool_name;
