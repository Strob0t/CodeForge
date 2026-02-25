-- +goose Up

CREATE TABLE prompt_sections (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    name        TEXT NOT NULL,
    scope       TEXT NOT NULL DEFAULT 'global',
    content     TEXT NOT NULL DEFAULT '',
    priority    INT NOT NULL DEFAULT 50,
    sort_order  INT NOT NULL DEFAULT 0,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    merge       TEXT NOT NULL DEFAULT 'replace' CHECK (merge IN ('replace', 'prepend', 'append')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, scope, name)
);

CREATE INDEX idx_prompt_sections_scope ON prompt_sections (tenant_id, scope);

-- +goose Down

DROP TABLE IF EXISTS prompt_sections;
