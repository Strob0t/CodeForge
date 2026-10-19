-- +goose Up
-- The dispatch of a task's last recorded worker result (S2-G fix, 6). A
-- tasks.result names its dispatch: only the result of the task's current
-- dispatch ends the task; a result of another dispatch (one the watchdog
-- failed, or that a newer dispatch replaced) only adds its cost. This column
-- makes that cost count once when the result is delivered again.
ALTER TABLE tasks ADD COLUMN result_dispatch_id TEXT;

-- +goose Down
ALTER TABLE tasks DROP COLUMN IF EXISTS result_dispatch_id;
