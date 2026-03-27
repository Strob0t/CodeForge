-- +goose Up
-- Consent management for GDPR Art. 6(1)(a) / Art. 7 compliance.
-- Append-only design: every consent change is a new row (proof-of-consent per Art. 7(1)).

CREATE TABLE consent_purposes (
    id          TEXT NOT NULL,
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    label       TEXT NOT NULL,
    description TEXT NOT NULL,
    legal_basis TEXT NOT NULL CHECK (legal_basis IN ('consent', 'legitimate_interest', 'contract')),
    required    BOOLEAN NOT NULL DEFAULT FALSE,
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE user_consents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    user_id         UUID NOT NULL REFERENCES users(id),
    purpose_id      TEXT NOT NULL,
    purpose_version INTEGER NOT NULL,
    granted         BOOLEAN NOT NULL,
    ip_address      INET,
    user_agent      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_user_consents_lookup ON user_consents (tenant_id, user_id, purpose_id, created_at DESC);
CREATE INDEX idx_user_consents_tenant_user ON user_consents (tenant_id, user_id);

-- +goose Down
DROP TABLE IF EXISTS user_consents;
DROP TABLE IF EXISTS consent_purposes;
