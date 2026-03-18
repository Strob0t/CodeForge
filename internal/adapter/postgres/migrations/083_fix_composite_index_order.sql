-- +goose Up
CREATE INDEX IF NOT EXISTS idx_agent_events_tenant_run_seq
    ON agent_events(tenant_id, run_id, sequence_number);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_events_tenant_run_seq;
