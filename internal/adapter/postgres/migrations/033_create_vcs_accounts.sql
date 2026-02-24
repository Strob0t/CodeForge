-- +goose Up
CREATE TABLE vcs_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    provider TEXT NOT NULL,
    label TEXT NOT NULL,
    server_url TEXT NOT NULL DEFAULT '',
    auth_method TEXT NOT NULL DEFAULT 'token',
    encrypted_token BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_vcs_accounts_tenant ON vcs_accounts (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS vcs_accounts;
