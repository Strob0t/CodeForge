-- +goose Up
ALTER TABLE conversations ADD COLUMN mode TEXT NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN model TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE conversations DROP COLUMN IF EXISTS model;
ALTER TABLE conversations DROP COLUMN IF EXISTS mode;
