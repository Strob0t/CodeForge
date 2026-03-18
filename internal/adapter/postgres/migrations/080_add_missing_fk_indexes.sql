-- +goose Up
CREATE INDEX IF NOT EXISTS idx_channels_tenant_id ON channels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_channels_project_id ON channels(project_id);
CREATE INDEX IF NOT EXISTS idx_channels_created_by ON channels(created_by);
CREATE INDEX IF NOT EXISTS idx_channel_messages_sender_id ON channel_messages(sender_id);
CREATE INDEX IF NOT EXISTS idx_channel_members_channel_id ON channel_members(channel_id);
CREATE INDEX IF NOT EXISTS idx_channel_members_user_id ON channel_members(user_id);
CREATE INDEX IF NOT EXISTS idx_project_boundaries_project_tenant ON project_boundaries(project_id, tenant_id);

-- +goose Down
DROP INDEX IF EXISTS idx_project_boundaries_project_tenant;
DROP INDEX IF EXISTS idx_channel_members_user_id;
DROP INDEX IF EXISTS idx_channel_members_channel_id;
DROP INDEX IF EXISTS idx_channel_messages_sender_id;
DROP INDEX IF EXISTS idx_channels_created_by;
DROP INDEX IF EXISTS idx_channels_project_id;
DROP INDEX IF EXISTS idx_channels_tenant_id;
