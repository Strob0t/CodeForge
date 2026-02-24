-- +goose Up
CREATE TABLE settings (
    key TEXT NOT NULL,
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    value JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (key, tenant_id)
);

-- +goose Down
DROP TABLE IF EXISTS settings;
