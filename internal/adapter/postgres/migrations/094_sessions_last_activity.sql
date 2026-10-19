-- +goose Up
-- Retention ages sessions by their last use. updated_at cannot tell: nothing
-- writes a conversation session while it is reused, and the foreign key
-- actions that NULL a purged run or conversation reset it through the
-- updated_at trigger. last_activity_at is written only when a session is used.
--
-- The column starts at the time of this migration for existing sessions: a
-- constant default is stored in the catalog (no table rewrite, no backfill
-- that would fire the updated_at trigger on every row), so NOT NULL needs no
-- scan either. Existing sessions are therefore purged at the earliest one
-- retention period (retention.sessions) after the upgrade.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- +goose Down
ALTER TABLE sessions DROP COLUMN IF EXISTS last_activity_at;
