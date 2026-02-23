-- +goose Up

-- Retrieval scopes: named boundaries for cross-project RAG search.
CREATE TABLE retrieval_scopes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    name        TEXT NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('shared', 'global')),
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_retrieval_scopes_name ON retrieval_scopes(tenant_id, name);
CREATE INDEX idx_retrieval_scopes_type ON retrieval_scopes(type);

-- Join table: which projects belong to which scope (explicit opt-in).
CREATE TABLE retrieval_scope_projects (
    scope_id    UUID NOT NULL REFERENCES retrieval_scopes(id) ON DELETE CASCADE,
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope_id, project_id)
);

CREATE INDEX idx_scope_projects_project ON retrieval_scope_projects(project_id);

CREATE TRIGGER set_retrieval_scopes_updated_at
    BEFORE UPDATE ON retrieval_scopes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- +goose Down
DROP TABLE IF EXISTS retrieval_scope_projects;
DROP TABLE IF EXISTS retrieval_scopes;
