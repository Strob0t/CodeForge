-- +goose Up

-- Account lockout: track failed login attempts and temporary lock timestamp.
ALTER TABLE users ADD COLUMN IF NOT EXISTS failed_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01T00:00:00Z';

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS locked_until;
ALTER TABLE users DROP COLUMN IF EXISTS failed_attempts;
