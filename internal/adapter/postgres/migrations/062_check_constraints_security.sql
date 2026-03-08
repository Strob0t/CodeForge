-- +goose Up

-- Defense-in-depth: CHECK constraints on security-critical enum columns.
-- Values verified against Go domain models:
--   internal/domain/user/user.go: RoleAdmin="admin", RoleEditor="editor", RoleViewer="viewer"
--   internal/domain/quarantine/quarantine.go: StatusPending="pending", StatusApproved="approved",
--     StatusRejected="rejected", StatusExpired="expired"
ALTER TABLE users ADD CONSTRAINT chk_users_role
    CHECK (role IN ('admin', 'editor', 'viewer'));
ALTER TABLE quarantine_messages ADD CONSTRAINT chk_quarantine_status
    CHECK (status IN ('pending', 'approved', 'rejected', 'expired'));

-- +goose Down

ALTER TABLE quarantine_messages DROP CONSTRAINT IF EXISTS chk_quarantine_status;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_role;
