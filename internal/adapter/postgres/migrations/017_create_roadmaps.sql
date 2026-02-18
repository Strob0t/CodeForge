-- +goose Up

CREATE TABLE roadmaps (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id   TEXT NOT NULL DEFAULT '',
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft',
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_roadmaps_project ON roadmaps(project_id);

CREATE TRIGGER trg_roadmaps_updated_at
    BEFORE UPDATE ON roadmaps
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_roadmaps_version
    BEFORE UPDATE ON roadmaps
    FOR EACH ROW EXECUTE FUNCTION increment_version();

CREATE TABLE milestones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    roadmap_id  UUID NOT NULL REFERENCES roadmaps(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    due_date    TIMESTAMPTZ,
    version     INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_milestones_roadmap_sort ON milestones(roadmap_id, sort_order);

CREATE TRIGGER trg_milestones_updated_at
    BEFORE UPDATE ON milestones
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_milestones_version
    BEFORE UPDATE ON milestones
    FOR EACH ROW EXECUTE FUNCTION increment_version();

CREATE TABLE features (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    milestone_id UUID NOT NULL REFERENCES milestones(id) ON DELETE CASCADE,
    roadmap_id   UUID NOT NULL REFERENCES roadmaps(id) ON DELETE CASCADE,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'backlog',
    labels       TEXT[] NOT NULL DEFAULT '{}',
    spec_ref     TEXT NOT NULL DEFAULT '',
    external_ids JSONB NOT NULL DEFAULT '{}',
    sort_order   INTEGER NOT NULL DEFAULT 0,
    version      INTEGER NOT NULL DEFAULT 1,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_features_milestone_sort ON features(milestone_id, sort_order);
CREATE INDEX idx_features_roadmap ON features(roadmap_id);
CREATE INDEX idx_features_status ON features(status);

CREATE TRIGGER trg_features_updated_at
    BEFORE UPDATE ON features
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_features_version
    BEFORE UPDATE ON features
    FOR EACH ROW EXECUTE FUNCTION increment_version();

-- +goose Down

DROP TRIGGER IF EXISTS trg_features_version ON features;
DROP TRIGGER IF EXISTS trg_features_updated_at ON features;
DROP TRIGGER IF EXISTS trg_milestones_version ON milestones;
DROP TRIGGER IF EXISTS trg_milestones_updated_at ON milestones;
DROP TRIGGER IF EXISTS trg_roadmaps_version ON roadmaps;
DROP TRIGGER IF EXISTS trg_roadmaps_updated_at ON roadmaps;

DROP TABLE IF EXISTS features;
DROP TABLE IF EXISTS milestones;
DROP TABLE IF EXISTS roadmaps;
