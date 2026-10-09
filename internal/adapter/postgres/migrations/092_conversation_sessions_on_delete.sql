-- +goose Up
-- Deleting a conversation failed once it had a session without a task: the
-- foreign key sets sessions.conversation_id to NULL, which the
-- sessions_task_or_conversation check rejects. One foreign key action cannot
-- delete conversation-only sessions and keep the others, so conversation-only
-- sessions are deleted by this trigger before the conversation, and sessions
-- that also belong to a task keep existing, detached by the unchanged
-- ON DELETE SET NULL (task_id NOT NULL satisfies the check).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_conversation_only_sessions()
RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM sessions WHERE conversation_id = OLD.id AND task_id IS NULL;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_conversations_delete_sessions ON conversations;
CREATE TRIGGER trg_conversations_delete_sessions
    BEFORE DELETE ON conversations
    FOR EACH ROW EXECUTE FUNCTION delete_conversation_only_sessions();

-- +goose Down
DROP TRIGGER IF EXISTS trg_conversations_delete_sessions ON conversations;
DROP FUNCTION IF EXISTS delete_conversation_only_sessions();
