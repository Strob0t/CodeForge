-- +goose Up
-- KI-85: inbound webhooks are registered per project. Each has a random ID
-- (the URL names it) and its own secret; the ID resolves the tenant and the
-- project, so a delivery acts only in the webhook's tenant. The secret (HMAC
-- key or GitLab token) and a PM integration's API token are stored
-- encrypted (AES-256-GCM with a key derived from auth.jwt_secret, like VCS
-- account tokens).
CREATE TABLE webhook_endpoints (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID        NOT NULL REFERENCES tenants(id),
    project_id          UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind                TEXT        NOT NULL CHECK (kind IN ('vcs', 'pm')),
    provider            TEXT        NOT NULL,
    encrypted_secret    BYTEA       NOT NULL,
    encrypted_api_token BYTEA,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    secret_rotated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, kind, provider)
);

CREATE INDEX idx_webhook_endpoints_tenant_project ON webhook_endpoints(tenant_id, project_id);

-- The delivery IDs a webhook handled (X-GitHub-Delivery, X-Gitlab-Event-UUID,
-- X-Plane-Delivery), so a redelivered or replayed event is handled once.
-- Claims older than webhook.delivery_retention are pruned when the webhook
-- receives its next delivery.
CREATE TABLE webhook_deliveries (
    webhook_id  UUID        NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    delivery_id TEXT        NOT NULL,
    tenant_id   UUID        NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (webhook_id, delivery_id)
);

CREATE INDEX idx_webhook_deliveries_received ON webhook_deliveries(webhook_id, received_at);
CREATE INDEX idx_webhook_deliveries_tenant ON webhook_deliveries(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_endpoints;
