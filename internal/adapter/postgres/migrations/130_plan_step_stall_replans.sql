-- +goose Up
-- KI-94: a plan step whose run stalled is re-planned at most
-- runtime.stall_max_retries times. The re-plans are counted per step: the
-- steps of a debate (proponent, moderator) and the step they debate run the
-- same task with the same agent, so counting the task's stalled runs made
-- them share one budget.
ALTER TABLE plan_steps ADD COLUMN stall_replans INTEGER NOT NULL DEFAULT 0 CHECK (stall_replans >= 0);

-- +goose Down
ALTER TABLE plan_steps DROP COLUMN stall_replans;
