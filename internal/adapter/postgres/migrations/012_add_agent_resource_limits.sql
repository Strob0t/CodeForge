-- +goose Up
ALTER TABLE agents ADD COLUMN resource_limits JSONB DEFAULT NULL;

-- +goose Down
ALTER TABLE agents DROP COLUMN IF EXISTS resource_limits;
