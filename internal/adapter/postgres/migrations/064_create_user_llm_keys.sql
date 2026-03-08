-- +goose Up
CREATE TABLE user_llm_keys (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id     UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    provider      TEXT NOT NULL,
    label         TEXT NOT NULL,
    encrypted_key BYTEA NOT NULL,
    key_prefix    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, provider)
);
CREATE INDEX idx_user_llm_keys_user ON user_llm_keys (user_id);
CREATE INDEX idx_user_llm_keys_tenant ON user_llm_keys (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS user_llm_keys;
