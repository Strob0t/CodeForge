-- +goose Up
ALTER TABLE execution_plans ADD COLUMN team_id UUID REFERENCES agent_teams(id) ON DELETE SET NULL;
ALTER TABLE runs ADD COLUMN team_id UUID REFERENCES agent_teams(id) ON DELETE SET NULL;
ALTER TABLE runs ADD COLUMN output TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_runs_team_id ON runs(team_id);

-- +goose Down
DROP INDEX IF EXISTS idx_runs_team_id;
ALTER TABLE runs DROP COLUMN IF EXISTS output;
ALTER TABLE runs DROP COLUMN IF EXISTS team_id;
ALTER TABLE execution_plans DROP COLUMN IF EXISTS team_id;
