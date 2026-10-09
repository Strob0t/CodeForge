-- +goose Up
-- The task a handoff stage created for its run (S2-G fix 2, 1). A stage
-- that failed transiently (starting the run) is retried with the same task,
-- instead of every redelivery creating another one.
ALTER TABLE handoff_claims ADD COLUMN task_id TEXT;

-- +goose Down
ALTER TABLE handoff_claims DROP COLUMN IF EXISTS task_id;
