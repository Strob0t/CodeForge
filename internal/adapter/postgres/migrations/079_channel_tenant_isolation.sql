-- +goose Up
ALTER TABLE channel_messages
    ADD COLUMN tenant_id UUID;

UPDATE channel_messages SET tenant_id = (
    SELECT c.tenant_id FROM channels c WHERE c.id = channel_messages.channel_id
);

ALTER TABLE channel_messages
    ALTER COLUMN tenant_id SET NOT NULL;

ALTER TABLE channel_messages
    ADD CONSTRAINT channel_messages_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id);

CREATE INDEX idx_channel_messages_tenant ON channel_messages(tenant_id);


ALTER TABLE channel_members
    ADD COLUMN tenant_id UUID;

UPDATE channel_members SET tenant_id = (
    SELECT c.tenant_id FROM channels c WHERE c.id = channel_members.channel_id
);

ALTER TABLE channel_members
    ALTER COLUMN tenant_id SET NOT NULL;

ALTER TABLE channel_members
    ADD CONSTRAINT channel_members_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id);

CREATE INDEX idx_channel_members_tenant ON channel_members(tenant_id);

-- +goose Down
DROP INDEX IF EXISTS idx_channel_members_tenant;
ALTER TABLE channel_members DROP CONSTRAINT IF EXISTS channel_members_tenant_id_fkey;
ALTER TABLE channel_members DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_channel_messages_tenant;
ALTER TABLE channel_messages DROP CONSTRAINT IF EXISTS channel_messages_tenant_id_fkey;
ALTER TABLE channel_messages DROP COLUMN IF EXISTS tenant_id;
