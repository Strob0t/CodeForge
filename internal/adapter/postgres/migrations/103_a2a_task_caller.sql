-- +goose Up
-- The A2A key that created an inbound A2A task (S2-G fix, V1). Every A2A key
-- of a tenant used to reach all of the tenant's A2A tasks through the
-- protocol handler; it now sees only the inbound tasks its key created. The
-- column holds the key's ID (a SHA-256 prefix), never the key. Tasks created
-- before have none and are visible to no A2A caller.
ALTER TABLE a2a_tasks ADD COLUMN caller_key_id TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_a2a_tasks_inbound_caller ON a2a_tasks (tenant_id, caller_key_id, created_at DESC)
    WHERE direction = 'inbound';

-- +goose Down
DROP INDEX IF EXISTS idx_a2a_tasks_inbound_caller;
ALTER TABLE a2a_tasks DROP COLUMN IF EXISTS caller_key_id;
