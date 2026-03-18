-- +goose Up
-- These single-column indexes are subsumed by tenant-prefixed composite indexes from migration 058.
DROP INDEX IF EXISTS idx_agent_events_task_id;
DROP INDEX IF EXISTS idx_agent_events_agent_id;
DROP INDEX IF EXISTS idx_agent_events_project_id;
DROP INDEX IF EXISTS idx_agent_events_run_id;
-- These are subsumed by composite indexes from migrations 042/043.
DROP INDEX IF EXISTS idx_agent_memories_project;
DROP INDEX IF EXISTS idx_experience_entries_project;

-- +goose Down
CREATE INDEX IF NOT EXISTS idx_agent_events_task_id ON agent_events(task_id, version);
CREATE INDEX IF NOT EXISTS idx_agent_events_agent_id ON agent_events(agent_id, version);
CREATE INDEX IF NOT EXISTS idx_agent_events_project_id ON agent_events(project_id, created_at);
CREATE INDEX IF NOT EXISTS idx_agent_events_run_id ON agent_events(run_id);
CREATE INDEX IF NOT EXISTS idx_agent_memories_project ON agent_memories(project_id);
CREATE INDEX IF NOT EXISTS idx_experience_entries_project ON experience_entries(project_id);
