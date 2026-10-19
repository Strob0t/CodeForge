-- +goose Up
-- Worker liveness for the stuck-work watchdog (KI-65). Runs, conversation
-- runs and backend tasks are acked on accept (ADR-016) and never redelivered,
-- so work whose worker died waits forever for its completion. While a worker
-- executes such work it sends a heartbeat every 30 s; the Go Core records the
-- latest one here, and the watchdog ends work whose heartbeats stopped. Work
-- without any heartbeat has not been accepted yet (it waits in NATS for a
-- free worker) and is not ended by the watchdog.
ALTER TABLE runs ADD COLUMN last_heartbeat_at TIMESTAMPTZ;
CREATE INDEX idx_runs_running_heartbeat ON runs (last_heartbeat_at)
    WHERE status = 'running' AND last_heartbeat_at IS NOT NULL;

-- The active run of a conversation (its turn): set before its start is
-- published, cleared when it ends or is stopped.
ALTER TABLE conversations
    ADD COLUMN active_turn_id TEXT,
    ADD COLUMN active_turn_heartbeat_at TIMESTAMPTZ;
CREATE INDEX idx_conversations_active_turn_heartbeat ON conversations (active_turn_heartbeat_at)
    WHERE active_turn_id IS NOT NULL AND active_turn_heartbeat_at IS NOT NULL;

-- Backend tasks get a table of their own: every update of a task row bumps
-- its version (optimistic locking) and updated_at through triggers. A
-- heartbeat names the task version it was sent for: a task that is changed
-- (re-dispatched) afterwards has no heartbeat until its worker sends one.
CREATE TABLE task_heartbeats (
    task_id      UUID PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    tenant_id    UUID NOT NULL,
    task_version INTEGER NOT NULL,
    beat_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_task_heartbeats_beat_at ON task_heartbeats (beat_at);

-- +goose Down
DROP TABLE IF EXISTS task_heartbeats;
DROP INDEX IF EXISTS idx_conversations_active_turn_heartbeat;
ALTER TABLE conversations
    DROP COLUMN IF EXISTS active_turn_heartbeat_at,
    DROP COLUMN IF EXISTS active_turn_id;
DROP INDEX IF EXISTS idx_runs_running_heartbeat;
ALTER TABLE runs DROP COLUMN IF EXISTS last_heartbeat_at;
