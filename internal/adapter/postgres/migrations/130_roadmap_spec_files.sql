-- +goose Up
-- KI-203: the content hash (SHA-256, hex) of a workspace spec file when the
-- roadmap last imported it or wrote its checkbox markers back. "Sync to
-- file" refuses a file whose content changed since (it must be imported
-- again first), and an import takes a checkbox's state from the file only
-- when the file changed since.
CREATE TABLE roadmap_spec_files (
    tenant_id      UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    roadmap_id     UUID        NOT NULL REFERENCES roadmaps(id) ON DELETE CASCADE,
    path           TEXT        NOT NULL,
    content_sha256 TEXT        NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (roadmap_id, path)
);
CREATE INDEX idx_roadmap_spec_files_tenant ON roadmap_spec_files (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS roadmap_spec_files;
