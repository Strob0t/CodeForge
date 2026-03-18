-- +goose Up
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_project_id_fkey;
ALTER TABLE channels ADD CONSTRAINT channels_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE channel_messages DROP CONSTRAINT IF EXISTS channel_messages_parent_id_fkey;
ALTER TABLE channel_messages ADD CONSTRAINT channel_messages_parent_id_fkey
    FOREIGN KEY (parent_id) REFERENCES channel_messages(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE channel_messages DROP CONSTRAINT IF EXISTS channel_messages_parent_id_fkey;
ALTER TABLE channel_messages ADD CONSTRAINT channel_messages_parent_id_fkey
    FOREIGN KEY (parent_id) REFERENCES channel_messages(id);

ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_project_id_fkey;
ALTER TABLE channels ADD CONSTRAINT channels_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id);
