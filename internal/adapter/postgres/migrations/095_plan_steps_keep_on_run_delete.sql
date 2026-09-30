-- +goose Up
-- Deleting a run (cost-record retention) deleted the plan steps that ran it
-- (ON DELETE CASCADE). A plan step is part of its execution plan, not of the
-- run's cost record: it keeps its status and error and only loses the run
-- reference.
ALTER TABLE plan_steps DROP CONSTRAINT IF EXISTS plan_steps_run_id_fkey;
ALTER TABLE plan_steps ADD CONSTRAINT plan_steps_run_id_fkey
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE plan_steps DROP CONSTRAINT IF EXISTS plan_steps_run_id_fkey;
ALTER TABLE plan_steps ADD CONSTRAINT plan_steps_run_id_fkey
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE;
