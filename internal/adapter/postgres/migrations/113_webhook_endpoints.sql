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

-- The deliveries a webhook handled, each by two keys: the SHA-256 of its body
-- ("body:<hex>") and its delivery ID ("id:<X-GitHub-Delivery,
-- X-Gitlab-Event-UUID or X-Plane-Delivery>"). A provider's redelivery shares
-- both, a replay of a signed delivery its body (the signature covers only the
-- body, not the delivery-ID header), so either is handled once within
-- webhook.delivery_retention. Claims older than that are pruned when the
-- webhook receives its next delivery.
CREATE TABLE webhook_deliveries (
    webhook_id   UUID        NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    delivery_key TEXT        NOT NULL,
    tenant_id    UUID        NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (webhook_id, delivery_key)
);

CREATE INDEX idx_webhook_deliveries_received ON webhook_deliveries(webhook_id, received_at);
CREATE INDEX idx_webhook_deliveries_tenant ON webhook_deliveries(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_endpoints;
