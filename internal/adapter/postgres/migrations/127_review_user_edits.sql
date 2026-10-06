-- +goose Up
-- KI-94: the workspace paths users changed through the editor or the file
-- API while a review pipeline's refactoring was not measured yet (state
-- refactoring). The workspace records no writer, so such a change counts as
-- the refactoring's and an undo sets it back too; the approval dialog lists
-- these paths. One row per path (its latest change); the rows are deleted
-- when the pipeline is decided (done) and go with the pipeline's plan. The
-- user is referenced, not copied: a deleted account leaves the row without
-- one.
CREATE TABLE IF NOT EXISTS review_user_edits (
    plan_id   UUID NOT NULL REFERENCES review_pipelines(plan_id) ON DELETE CASCADE,
    path      TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    step_id   TEXT NOT NULL DEFAULT '',
    operation TEXT NOT NULL CHECK (operation IN ('write', 'delete', 'rename')),
    user_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    edited_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (plan_id, path)
);
CREATE INDEX IF NOT EXISTS idx_review_user_edits_user
    ON review_user_edits (user_id)
    WHERE user_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS review_user_edits;
