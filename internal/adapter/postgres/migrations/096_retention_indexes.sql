-- +goose Up
-- +goose NO TRANSACTION
-- Indexes for the retention job's age predicates on audit entries and consent
-- records (store_retention.go): they age by their creation, which never
-- changes. Sessions, conversations and runs get no index on the activity
-- timestamp they age by: it changes on every update, and an index on it would
-- make those updates non-HOT; the daily sweep scans them instead.
-- Named idx_retention_* so they do not clash with feature indexes.
--
-- CONCURRENTLY, so the build does not block writes while the application
-- starts. A build that fails leaves an invalid index; the DROP removes it
-- when the migration runs again.
DROP INDEX CONCURRENTLY IF EXISTS idx_retention_audit_log_created;
CREATE INDEX CONCURRENTLY idx_retention_audit_log_created ON audit_log (created_at);

DROP INDEX CONCURRENTLY IF EXISTS idx_retention_audit_log_ip_created;
CREATE INDEX CONCURRENTLY idx_retention_audit_log_ip_created ON audit_log (created_at)
    WHERE ip_address IS NOT NULL;

DROP INDEX CONCURRENTLY IF EXISTS idx_retention_user_consents_client_created;
CREATE INDEX CONCURRENTLY idx_retention_user_consents_client_created ON user_consents (created_at)
    WHERE ip_address IS NOT NULL OR user_agent IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_retention_user_consents_client_created;
DROP INDEX CONCURRENTLY IF EXISTS idx_retention_audit_log_ip_created;
DROP INDEX CONCURRENTLY IF EXISTS idx_retention_audit_log_created;
