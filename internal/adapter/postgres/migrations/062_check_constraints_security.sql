-- +goose Up

ALTER TABLE users ADD CONSTRAINT chk_users_role
    CHECK (role IN ('admin', 'editor', 'viewer'));
ALTER TABLE quarantine_messages ADD CONSTRAINT chk_quarantine_status
    CHECK (status IN ('pending', 'approved', 'rejected', 'expired'));

-- +goose Down

ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_role;
ALTER TABLE quarantine_messages DROP CONSTRAINT IF EXISTS chk_quarantine_status;
