-- +goose Up
-- KI-143: access tokens carry the user's token epoch; deleting, erasing or
-- disabling a user and a role change raise it, so the user's earlier access
-- tokens stop working before they expire.
ALTER TABLE users ADD COLUMN token_epoch BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE users DROP COLUMN token_epoch;
