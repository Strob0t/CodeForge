-- +goose Up
CREATE TABLE context_packs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    token_budget INTEGER NOT NULL DEFAULT 4096,
    tokens_used INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_context_packs_task_id ON context_packs(task_id);

CREATE TABLE context_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pack_id UUID NOT NULL REFERENCES context_packs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL DEFAULT 'file',
    path TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL,
    tokens INTEGER NOT NULL DEFAULT 0,
    priority INTEGER NOT NULL DEFAULT 50
);

CREATE INDEX idx_context_entries_pack_id ON context_entries(pack_id);

CREATE TABLE shared_contexts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES agent_teams(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_shared_contexts_team_id ON shared_contexts(team_id);

CREATE TABLE shared_context_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    shared_id UUID NOT NULL REFERENCES shared_contexts(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    author UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    tokens INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_shared_context_items_shared_id ON shared_context_items(shared_id);
CREATE UNIQUE INDEX idx_shared_context_items_key ON shared_context_items(shared_id, key);

CREATE TRIGGER set_shared_contexts_updated_at
    BEFORE UPDATE ON shared_contexts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- +goose Down
DROP TABLE IF EXISTS shared_context_items;
DROP TABLE IF EXISTS shared_contexts;
DROP TABLE IF EXISTS context_entries;
DROP TABLE IF EXISTS context_packs;
