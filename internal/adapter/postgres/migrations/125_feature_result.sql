-- +goose Up
-- KI-152: the auto-agent verifies each feature it ran (change check, test
-- and lint commands) and records how that ended: what was checked, or why
-- the feature failed.
ALTER TABLE features ADD COLUMN result TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE features DROP COLUMN result;
