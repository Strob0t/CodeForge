-- +goose Up
-- KI-90: the retention job deletes handoff claims whose stage was done and
-- webhook delivery claims past their dedup window, across all tenants and in
-- batches; these indexes let each batch find them without a table scan.
CREATE INDEX IF NOT EXISTS idx_handoff_claims_done_at
    ON handoff_claims(done_at) WHERE done_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_received_at
    ON webhook_deliveries(received_at);

-- +goose Down
DROP INDEX IF EXISTS idx_webhook_deliveries_received_at;
DROP INDEX IF EXISTS idx_handoff_claims_done_at;
