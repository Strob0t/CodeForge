-- +goose Up

CREATE TABLE runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         UUID NOT NULL REFERENCES tasks(id),
    agent_id        UUID NOT NULL REFERENCES agents(id),
    project_id      UUID NOT NULL REFERENCES projects(id),
    policy_profile  TEXT NOT NULL DEFAULT 'headless-safe-sandbox',
    exec_mode       TEXT NOT NULL DEFAULT 'mount',
    status          TEXT NOT NULL DEFAULT 'pending',
    step_count      INTEGER NOT NULL DEFAULT 0,
    cost_usd        NUMERIC(12,6) NOT NULL DEFAULT 0,
    error           TEXT NOT NULL DEFAULT '',
    version         INTEGER NOT NULL DEFAULT 1,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_runs_task_id ON runs (task_id);
CREATE INDEX idx_runs_agent_id ON runs (agent_id);
CREATE INDEX idx_runs_project_id ON runs (project_id, created_at);

-- +goose Down

DROP TABLE IF EXISTS runs;
