-- +goose Up
ALTER TABLE conversation_messages ADD COLUMN tool_calls JSONB;
ALTER TABLE conversation_messages ADD COLUMN tool_call_id TEXT NOT NULL DEFAULT '';
ALTER TABLE conversation_messages ADD COLUMN tool_name TEXT NOT NULL DEFAULT '';
ALTER TABLE conversation_messages ALTER COLUMN content DROP NOT NULL;
ALTER TABLE conversation_messages DROP CONSTRAINT conversation_messages_role_check;
ALTER TABLE conversation_messages ADD CONSTRAINT conversation_messages_role_check CHECK (role IN ('user', 'assistant', 'system', 'tool'));

-- +goose Down
ALTER TABLE conversation_messages DROP COLUMN IF EXISTS tool_calls;
ALTER TABLE conversation_messages DROP COLUMN IF EXISTS tool_call_id;
ALTER TABLE conversation_messages DROP COLUMN IF EXISTS tool_name;
ALTER TABLE conversation_messages ALTER COLUMN content SET NOT NULL;
ALTER TABLE conversation_messages DROP CONSTRAINT IF EXISTS conversation_messages_role_check;
ALTER TABLE conversation_messages ADD CONSTRAINT conversation_messages_role_check CHECK (role IN ('user', 'assistant', 'system'));
