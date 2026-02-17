-- +goose Up

CREATE TABLE agent_teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL DEFAULT 'sequential',
    status TEXT NOT NULL DEFAULT 'initializing',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_teams_project_id ON agent_teams(project_id);

CREATE TABLE team_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES agent_teams(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'coder'
);

CREATE INDEX idx_team_members_team_id ON team_members(team_id);
CREATE UNIQUE INDEX idx_team_members_unique ON team_members(team_id, agent_id);

CREATE TRIGGER set_agent_teams_updated_at
    BEFORE UPDATE ON agent_teams
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- +goose Down

DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS agent_teams;
