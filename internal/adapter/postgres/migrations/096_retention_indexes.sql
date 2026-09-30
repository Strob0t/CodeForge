-- +goose Up
-- Indexes for the retention job's age predicates (store_retention.go,
-- AnonymizeExpired*): without them every sweep batch scans the whole table.
-- Named idx_retention_* so they do not clash with feature indexes on the same
-- columns (e.g. a partial runs(updated_at) index for one status).
CREATE INDEX IF NOT EXISTS idx_retention_sessions_last_activity ON sessions (last_activity_at);
CREATE INDEX IF NOT EXISTS idx_retention_conversations_updated ON conversations (updated_at);
CREATE INDEX IF NOT EXISTS idx_retention_runs_updated ON runs (updated_at);
CREATE INDEX IF NOT EXISTS idx_retention_audit_log_created ON audit_log (created_at);
CREATE INDEX IF NOT EXISTS idx_retention_audit_log_ip_created ON audit_log (created_at)
    WHERE ip_address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_retention_user_consents_client_created ON user_consents (created_at)
    WHERE ip_address IS NOT NULL OR user_agent IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_retention_user_consents_client_created;
DROP INDEX IF EXISTS idx_retention_audit_log_ip_created;
DROP INDEX IF EXISTS idx_retention_audit_log_created;
DROP INDEX IF EXISTS idx_retention_runs_updated;
DROP INDEX IF EXISTS idx_retention_conversations_updated;
DROP INDEX IF EXISTS idx_retention_sessions_last_activity;
