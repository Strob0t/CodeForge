-- +goose Up
-- Add monotonic sequence number for trajectory event ordering and dedup.

ALTER TABLE agent_events ADD COLUMN sequence_number BIGINT;

-- Create a sequence for monotonic numbering.
CREATE SEQUENCE agent_events_seq_number_seq;

-- Backfill existing rows with monotonic values.
UPDATE agent_events SET sequence_number = nextval('agent_events_seq_number_seq');

-- Make NOT NULL after backfill.
ALTER TABLE agent_events ALTER COLUMN sequence_number SET NOT NULL;
ALTER TABLE agent_events ALTER COLUMN sequence_number SET DEFAULT nextval('agent_events_seq_number_seq');

-- Index for efficient range queries (gap-fill on reconnect).
CREATE INDEX idx_agent_events_run_seq ON agent_events (run_id, sequence_number);

-- +goose Down
DROP INDEX IF EXISTS idx_agent_events_run_seq;
ALTER TABLE agent_events DROP COLUMN IF EXISTS sequence_number;
DROP SEQUENCE IF EXISTS agent_events_seq_number_seq;
