-- KI-73: a channel's webhook key (only its SHA-256 is stored; the key is
-- shown once when it is generated) and each user's read position per channel.

-- +goose Up
ALTER TABLE channels ADD COLUMN webhook_key_hash BYTEA;

CREATE TABLE channel_read_state (
    channel_id           UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id            UUID NOT NULL REFERENCES tenants(id),
    last_read_message_id UUID REFERENCES channel_messages(id) ON DELETE SET NULL,
    -- created_at of the last read message: later messages are unread.
    last_read_at         TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX idx_channel_read_state_tenant ON channel_read_state(tenant_id);
CREATE INDEX idx_channel_read_state_user ON channel_read_state(user_id);

-- +goose Down
DROP TABLE IF EXISTS channel_read_state;
ALTER TABLE channels DROP COLUMN IF EXISTS webhook_key_hash;
