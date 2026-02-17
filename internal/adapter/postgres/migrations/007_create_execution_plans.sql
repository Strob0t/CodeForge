-- +goose Up

CREATE TABLE execution_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL DEFAULT 'sequential',
    status TEXT NOT NULL DEFAULT 'pending',
    max_parallel INTEGER NOT NULL DEFAULT 4,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_execution_plans_project_id ON execution_plans(project_id);

CREATE TABLE plan_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES execution_plans(id) ON DELETE CASCADE,
    task_id UUID NOT NULL REFERENCES tasks(id),
    agent_id UUID NOT NULL REFERENCES agents(id),
    policy_profile TEXT NOT NULL DEFAULT '',
    deliver_mode TEXT NOT NULL DEFAULT '',
    depends_on UUID[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'pending',
    run_id UUID REFERENCES runs(id),
    round INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_plan_steps_plan_id ON plan_steps(plan_id);
CREATE INDEX idx_plan_steps_run_id ON plan_steps(run_id);

-- Reuse existing trigger function for updated_at
CREATE TRIGGER set_execution_plans_updated_at
    BEFORE UPDATE ON execution_plans
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER set_plan_steps_updated_at
    BEFORE UPDATE ON plan_steps
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- +goose Down

DROP TABLE IF EXISTS plan_steps;
DROP TABLE IF EXISTS execution_plans;
