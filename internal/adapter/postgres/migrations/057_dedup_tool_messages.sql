-- +goose Up
-- Prevent duplicate tool result messages for the same conversation + tool_call_id.
-- This partial index only covers rows with a non-null, non-empty tool_call_id.
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_messages_tool_call_id_unique
    ON conversation_messages (conversation_id, tool_call_id)
    WHERE tool_call_id IS NOT NULL AND tool_call_id != '';

-- +goose Down
DROP INDEX IF EXISTS idx_conversation_messages_tool_call_id_unique;
