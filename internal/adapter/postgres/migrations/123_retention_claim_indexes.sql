-- +goose Up
-- +goose NO TRANSACTION
-- KI-90: the retention job deletes handoff claims whose stage was done and
-- webhook delivery claims past their dedup window, across all tenants and in
-- batches; these indexes let each batch find them without a table scan.
--
-- CONCURRENTLY (like migration 096), so the build does not block the claim
-- writes while the application starts. A build that fails leaves an invalid
-- index; the DROP removes it when the migration runs again.
DROP INDEX CONCURRENTLY IF EXISTS idx_handoff_claims_done_at;
CREATE INDEX CONCURRENTLY idx_handoff_claims_done_at ON handoff_claims (done_at)
    WHERE done_at IS NOT NULL;

DROP INDEX CONCURRENTLY IF EXISTS idx_webhook_deliveries_received_at;
CREATE INDEX CONCURRENTLY idx_webhook_deliveries_received_at ON webhook_deliveries (received_at);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_webhook_deliveries_received_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_handoff_claims_done_at;
