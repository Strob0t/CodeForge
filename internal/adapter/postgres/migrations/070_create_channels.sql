-- +goose Up
CREATE TABLE channels (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    project_id  UUID REFERENCES projects(id),
    name        VARCHAR(100) NOT NULL,
    type        VARCHAR(20) NOT NULL CHECK (type IN ('project', 'bot')),
    description TEXT DEFAULT '',
    created_by  UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE channel_messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    sender_id   UUID REFERENCES users(id),
    sender_type VARCHAR(20) NOT NULL CHECK (sender_type IN ('user', 'agent', 'bot', 'webhook')),
    sender_name VARCHAR(100) NOT NULL,
    content     TEXT NOT NULL,
    metadata    JSONB DEFAULT '{}',
    parent_id   UUID REFERENCES channel_messages(id),
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE channel_members (
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id),
    role        VARCHAR(20) DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    notify      VARCHAR(20) DEFAULT 'all' CHECK (notify IN ('all', 'mentions', 'nothing')),
    joined_at   TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX idx_channel_messages_channel ON channel_messages(channel_id, created_at DESC);
CREATE INDEX idx_channel_messages_thread ON channel_messages(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_channel_messages_fts ON channel_messages USING GIN(to_tsvector('english', content));

-- +goose Down
DROP TABLE IF EXISTS channel_members;
DROP TABLE IF EXISTS channel_messages;
DROP TABLE IF EXISTS channels;
