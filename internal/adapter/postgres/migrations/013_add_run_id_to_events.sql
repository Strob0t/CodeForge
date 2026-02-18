-- +goose Up
ALTER TABLE agent_events ADD COLUMN run_id UUID;
CREATE INDEX idx_agent_events_run_id ON agent_events (run_id);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_events_run_id;
ALTER TABLE agent_events DROP COLUMN IF EXISTS run_id;
