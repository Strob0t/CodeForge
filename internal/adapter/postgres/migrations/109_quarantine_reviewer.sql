-- +goose Up
-- KI-79: the reviewer of a quarantined message is the logged-in user.
-- reviewed_by keeps the reviewer's name at the time; the GDPR erasure of the
-- user replaces it with a placeholder before the user row is deleted, which
-- then unlinks the ID. Reviews recorded before this migration keep their
-- free-text name and no ID.
ALTER TABLE quarantine_messages
    ADD COLUMN reviewed_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX idx_quarantine_reviewed_by_user
    ON quarantine_messages(reviewed_by_user_id)
    WHERE reviewed_by_user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_quarantine_reviewed_by_user;
ALTER TABLE quarantine_messages DROP COLUMN IF EXISTS reviewed_by_user_id;
