-- +goose Up
-- The dispatch of a backend task (S2-F review). Every dispatch gets an ID
-- that its tasks.agent.* message carries and the worker echoes on its
-- heartbeats: a heartbeat counts only for the dispatch it was sent for, so a
-- late heartbeat of an earlier dispatch never makes the watchdog take a
-- re-dispatch that no worker accepted for a lost one. The dispatch time lets
-- the watchdog fail a dispatch that no worker accepted at all.
ALTER TABLE tasks
    ADD COLUMN dispatch_id TEXT,
    ADD COLUMN dispatched_at TIMESTAMPTZ;
ALTER TABLE task_heartbeats ADD COLUMN dispatch_id TEXT;
CREATE INDEX idx_tasks_queued_dispatched_at ON tasks (dispatched_at)
    WHERE status = 'queued' AND dispatched_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_tasks_queued_dispatched_at;
ALTER TABLE task_heartbeats DROP COLUMN IF EXISTS dispatch_id;
ALTER TABLE tasks
    DROP COLUMN IF EXISTS dispatched_at,
    DROP COLUMN IF EXISTS dispatch_id;
