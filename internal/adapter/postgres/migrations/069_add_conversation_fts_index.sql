-- +goose Up
CREATE INDEX idx_conversation_messages_fts
ON conversation_messages USING GIN(to_tsvector('english', content));

-- +goose Down
DROP INDEX IF EXISTS idx_conversation_messages_fts;
