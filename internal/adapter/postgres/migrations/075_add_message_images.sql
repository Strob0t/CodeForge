-- +goose Up
ALTER TABLE conversation_messages ADD COLUMN images JSONB;

-- +goose Down
ALTER TABLE conversation_messages DROP COLUMN IF EXISTS images;
