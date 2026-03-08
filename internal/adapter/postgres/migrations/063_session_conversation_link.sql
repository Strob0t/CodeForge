-- +goose Up
ALTER TABLE sessions ALTER COLUMN task_id DROP NOT NULL;
ALTER TABLE sessions ADD COLUMN conversation_id UUID REFERENCES conversations(id) ON DELETE SET NULL;
ALTER TABLE sessions ADD CONSTRAINT sessions_task_or_conversation
    CHECK (task_id IS NOT NULL OR conversation_id IS NOT NULL);
CREATE INDEX idx_sessions_conversation_id ON sessions(conversation_id);

-- +goose Down
DROP INDEX IF EXISTS idx_sessions_conversation_id;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_task_or_conversation;
ALTER TABLE sessions DROP COLUMN IF EXISTS conversation_id;
ALTER TABLE sessions ALTER COLUMN task_id SET NOT NULL;
