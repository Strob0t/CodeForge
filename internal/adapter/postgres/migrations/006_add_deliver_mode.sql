-- +goose Up
ALTER TABLE runs ADD COLUMN IF NOT EXISTS deliver_mode TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE runs DROP COLUMN IF EXISTS deliver_mode;
