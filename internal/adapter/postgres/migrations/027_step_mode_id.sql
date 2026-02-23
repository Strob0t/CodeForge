-- +goose Up
-- Phase 12F: Pipeline Templates
-- Add mode_id to plan steps for mode-aware orchestration.
ALTER TABLE plan_steps ADD COLUMN mode_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE plan_steps DROP COLUMN mode_id;
