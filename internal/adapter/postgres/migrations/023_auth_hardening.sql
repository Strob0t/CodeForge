-- +goose Up

-- JWT revocation blacklist (P1-6)
CREATE TABLE IF NOT EXISTS revoked_tokens (
    jti         TEXT PRIMARY KEY,
    expires_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_revoked_tokens_expires ON revoked_tokens (expires_at);

-- API key scopes column (P2-1)
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS scopes TEXT[] DEFAULT NULL;

-- Forced password change (P2-2)
ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
DROP TABLE IF EXISTS revoked_tokens;
ALTER TABLE api_keys DROP COLUMN IF EXISTS scopes;
ALTER TABLE users DROP COLUMN IF EXISTS must_change_password;
