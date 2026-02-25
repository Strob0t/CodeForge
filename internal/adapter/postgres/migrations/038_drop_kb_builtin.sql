-- +goose Up
DELETE FROM knowledge_bases WHERE builtin = true;
ALTER TABLE knowledge_bases DROP COLUMN builtin;

-- +goose Down
ALTER TABLE knowledge_bases ADD COLUMN builtin BOOLEAN NOT NULL DEFAULT false;
