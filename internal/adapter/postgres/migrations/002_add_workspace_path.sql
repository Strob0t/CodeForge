-- +goose Up
ALTER TABLE projects ADD COLUMN IF NOT EXISTS workspace_path TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE projects DROP COLUMN IF EXISTS workspace_path;
