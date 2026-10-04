-- +goose Up
-- A handoff claim is done once its stage was carried out or refused (S2-G
-- fix 2, 3). A claim that was never done belongs to a process that may have
-- died between claiming the stage and starting its run: once its lease ran
-- out (claimed_at), a redelivery takes it over instead of being ignored.
ALTER TABLE handoff_claims ADD COLUMN done_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE handoff_claims DROP COLUMN IF EXISTS done_at;
