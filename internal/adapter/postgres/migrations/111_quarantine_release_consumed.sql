-- +goose Up
-- KI-71 review: Approve replays a held message to its subject; the Go Core
-- carries out a released handoff only when an approved, not yet consumed
-- quarantine message of its tenant has exactly its payload, and records the
-- consumption, so a message that only looks released starts nothing.
ALTER TABLE quarantine_messages ADD COLUMN consumed_at TIMESTAMPTZ;

CREATE INDEX idx_quarantine_unconsumed_releases
    ON quarantine_messages(tenant_id, subject)
    WHERE status = 'approved' AND consumed_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_quarantine_unconsumed_releases;
ALTER TABLE quarantine_messages DROP COLUMN IF EXISTS consumed_at;
