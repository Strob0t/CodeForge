-- +goose Up

ALTER TABLE projects ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agents   ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE tasks    ADD COLUMN version INTEGER NOT NULL DEFAULT 1;

-- Trigger to auto-increment version on each UPDATE
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION increment_version()
RETURNS TRIGGER AS $$
BEGIN
    NEW.version = OLD.version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_projects_version
    BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION increment_version();

CREATE TRIGGER trg_agents_version
    BEFORE UPDATE ON agents
    FOR EACH ROW EXECUTE FUNCTION increment_version();

CREATE TRIGGER trg_tasks_version
    BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION increment_version();

-- +goose Down

DROP TRIGGER IF EXISTS trg_tasks_version ON tasks;
DROP TRIGGER IF EXISTS trg_agents_version ON agents;
DROP TRIGGER IF EXISTS trg_projects_version ON projects;
DROP FUNCTION IF EXISTS increment_version;

ALTER TABLE tasks    DROP COLUMN IF EXISTS version;
ALTER TABLE agents   DROP COLUMN IF EXISTS version;
ALTER TABLE projects DROP COLUMN IF EXISTS version;
