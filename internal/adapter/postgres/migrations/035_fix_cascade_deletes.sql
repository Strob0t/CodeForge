-- +goose Up

-- Fix runs table: add ON DELETE CASCADE for project_id, task_id, agent_id
ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_project_id_fkey;
ALTER TABLE runs ADD CONSTRAINT runs_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_task_id_fkey;
ALTER TABLE runs ADD CONSTRAINT runs_task_id_fkey
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;

ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_agent_id_fkey;
ALTER TABLE runs ADD CONSTRAINT runs_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

-- Fix plan_steps: add ON DELETE CASCADE for task_id, agent_id, run_id
ALTER TABLE plan_steps DROP CONSTRAINT IF EXISTS plan_steps_task_id_fkey;
ALTER TABLE plan_steps ADD CONSTRAINT plan_steps_task_id_fkey
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE;

ALTER TABLE plan_steps DROP CONSTRAINT IF EXISTS plan_steps_agent_id_fkey;
ALTER TABLE plan_steps ADD CONSTRAINT plan_steps_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE plan_steps DROP CONSTRAINT IF EXISTS plan_steps_run_id_fkey;
ALTER TABLE plan_steps ADD CONSTRAINT plan_steps_run_id_fkey
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE;

-- +goose Down

-- Revert to original constraints without CASCADE
ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_project_id_fkey;
ALTER TABLE runs ADD CONSTRAINT runs_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id);

ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_task_id_fkey;
ALTER TABLE runs ADD CONSTRAINT runs_task_id_fkey
    FOREIGN KEY (task_id) REFERENCES tasks(id);

ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_agent_id_fkey;
ALTER TABLE runs ADD CONSTRAINT runs_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id);

ALTER TABLE plan_steps DROP CONSTRAINT IF EXISTS plan_steps_task_id_fkey;
ALTER TABLE plan_steps ADD CONSTRAINT plan_steps_task_id_fkey
    FOREIGN KEY (task_id) REFERENCES tasks(id);

ALTER TABLE plan_steps DROP CONSTRAINT IF EXISTS plan_steps_agent_id_fkey;
ALTER TABLE plan_steps ADD CONSTRAINT plan_steps_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id);

ALTER TABLE plan_steps DROP CONSTRAINT IF EXISTS plan_steps_run_id_fkey;
ALTER TABLE plan_steps ADD CONSTRAINT plan_steps_run_id_fkey
    FOREIGN KEY (run_id) REFERENCES runs(id);
