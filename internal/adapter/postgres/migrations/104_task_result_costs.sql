-- +goose Up
-- The worker results recorded per task dispatch (S2-G fix 2, 4). A task's
-- cost is the sum of its dispatches' costs, each counted once: the first
-- result of a dispatch records its cost here, a redelivery of it finds the
-- row and adds nothing. This replaces tasks.result_dispatch_id (101), which
-- remembered only the last dispatch, so a redelivered result counted again
-- after a late result of another dispatch.
CREATE TABLE task_result_costs (
    task_id     UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    dispatch_id TEXT        NOT NULL,
    tenant_id   UUID        NOT NULL,
    cost_usd    NUMERIC(10, 6) NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, dispatch_id)
);
ALTER TABLE tasks DROP COLUMN IF EXISTS result_dispatch_id;

-- +goose Down
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_dispatch_id TEXT;
DROP TABLE IF EXISTS task_result_costs;
