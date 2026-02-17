-- +goose Up
CREATE TABLE repo_maps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    map_text TEXT NOT NULL DEFAULT '',
    token_count INTEGER NOT NULL DEFAULT 0,
    file_count INTEGER NOT NULL DEFAULT 0,
    symbol_count INTEGER NOT NULL DEFAULT 0,
    languages TEXT[] NOT NULL DEFAULT '{}',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_repo_maps_project_id ON repo_maps(project_id);

-- +goose Down
DROP TABLE IF EXISTS repo_maps;
