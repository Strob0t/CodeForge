-- +goose Up
-- Plan and review events have no agent or task, task results can arrive
-- without an agent (KI-32). The event store wrote an empty string for a
-- missing ID, which the uuid columns rejected, so these events were lost.
-- The columns become nullable and a missing ID is stored as NULL.
ALTER TABLE agent_events ALTER COLUMN agent_id DROP NOT NULL;
ALTER TABLE agent_events ALTER COLUMN task_id DROP NOT NULL;

-- +goose Down
-- Events without agent or task keep the nil UUID, which matches no agent or task.
UPDATE agent_events SET agent_id = '00000000-0000-0000-0000-000000000000' WHERE agent_id IS NULL;
UPDATE agent_events SET task_id = '00000000-0000-0000-0000-000000000000' WHERE task_id IS NULL;
ALTER TABLE agent_events ALTER COLUMN task_id SET NOT NULL;
ALTER TABLE agent_events ALTER COLUMN agent_id SET NOT NULL;
