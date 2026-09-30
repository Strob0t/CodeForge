-- +goose Up
-- Retention ages sessions by their last use. updated_at cannot tell: nothing
-- writes a conversation session while it is reused, and the foreign key
-- actions that NULL a purged run or conversation reset it through the
-- updated_at trigger. last_activity_at is written only when a session is used.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMPTZ;

-- Backfill from updated_at without the trigger bumping updated_at itself.
ALTER TABLE sessions DISABLE TRIGGER sessions_updated_at;
UPDATE sessions SET last_activity_at = updated_at WHERE last_activity_at IS NULL;
ALTER TABLE sessions ENABLE TRIGGER sessions_updated_at;

ALTER TABLE sessions ALTER COLUMN last_activity_at SET DEFAULT now();
ALTER TABLE sessions ALTER COLUMN last_activity_at SET NOT NULL;

-- +goose Down
ALTER TABLE sessions DROP COLUMN IF EXISTS last_activity_at;
