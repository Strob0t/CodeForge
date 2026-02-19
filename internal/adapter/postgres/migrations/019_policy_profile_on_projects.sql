-- +goose Up
ALTER TABLE projects ADD COLUMN IF NOT EXISTS policy_profile TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE projects DROP COLUMN IF EXISTS policy_profile;
