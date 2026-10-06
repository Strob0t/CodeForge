-- +goose Up
-- KI-203: what the roadmap last saw of a workspace spec file when it
-- imported it or wrote its checkbox markers back. "Sync to file" refuses a
-- file whose content hash (SHA-256, hex) changed since (it must be imported
-- again first). checked holds the state of each imported feature's checkbox
-- ({"<feature id>": true}): an import takes a box's state from the file
-- only when it differs, otherwise the roadmap's status stays.
CREATE TABLE roadmap_spec_files (
    tenant_id      UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    roadmap_id     UUID        NOT NULL REFERENCES roadmaps(id) ON DELETE CASCADE,
    path           TEXT        NOT NULL,
    content_sha256 TEXT        NOT NULL,
    checked        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (roadmap_id, path)
);
CREATE INDEX idx_roadmap_spec_files_tenant ON roadmap_spec_files (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS roadmap_spec_files;
