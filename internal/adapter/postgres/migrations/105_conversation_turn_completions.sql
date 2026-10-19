-- +goose Up
-- The conversation turns whose worker completion was kept (S2-G fix 2, 2).
-- conversation.run.complete is delivered at least once: the first delivery
-- of a turn's completion claims the turn here before its messages and cost
-- are kept, so a redelivery keeps nothing again.
CREATE TABLE conversation_turn_completions (
    conversation_id UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    turn_id         TEXT        NOT NULL,
    tenant_id       UUID        NOT NULL,
    completed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, turn_id)
);

-- +goose Down
DROP TABLE IF EXISTS conversation_turn_completions;
