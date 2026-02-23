-- +goose Up
-- Phase 12E: Artifact-Gated Pipelines
-- Add artifact validation fields to runs table.
ALTER TABLE runs ADD COLUMN artifact_type TEXT NOT NULL DEFAULT '';
ALTER TABLE runs ADD COLUMN artifact_valid BOOLEAN;
ALTER TABLE runs ADD COLUMN artifact_errors JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE runs DROP COLUMN artifact_errors;
ALTER TABLE runs DROP COLUMN artifact_valid;
ALTER TABLE runs DROP COLUMN artifact_type;
