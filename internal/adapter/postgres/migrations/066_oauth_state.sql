-- +goose Up
CREATE TABLE oauth_states (
    state      TEXT        PRIMARY KEY,
    provider   TEXT        NOT NULL,
    tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_oauth_states_tenant_id ON oauth_states(tenant_id);
CREATE INDEX idx_oauth_states_expires_at ON oauth_states(expires_at);

-- +goose Down
DROP TABLE IF EXISTS oauth_states;
