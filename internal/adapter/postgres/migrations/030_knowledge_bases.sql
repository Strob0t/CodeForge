-- +goose Up

-- Knowledge bases: curated knowledge modules for agent context retrieval.
CREATE TABLE knowledge_bases (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    category     TEXT NOT NULL CHECK (category IN ('framework', 'paradigm', 'language', 'security', 'custom')),
    tags         TEXT[] NOT NULL DEFAULT '{}',
    builtin      BOOLEAN NOT NULL DEFAULT false,
    content_path TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'indexed', 'error')),
    chunk_count  INTEGER NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_knowledge_bases_name ON knowledge_bases(tenant_id, name);
CREATE INDEX idx_knowledge_bases_category ON knowledge_bases(category);

-- Join table: which knowledge bases belong to which scope.
CREATE TABLE scope_knowledge_bases (
    scope_id          UUID NOT NULL REFERENCES retrieval_scopes(id) ON DELETE CASCADE,
    knowledge_base_id UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    added_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope_id, knowledge_base_id)
);

CREATE INDEX idx_scope_kb_kb ON scope_knowledge_bases(knowledge_base_id);

-- +goose StatementBegin
CREATE TRIGGER set_knowledge_bases_updated_at
    BEFORE UPDATE ON knowledge_bases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS scope_knowledge_bases;
DROP TABLE IF EXISTS knowledge_bases;
