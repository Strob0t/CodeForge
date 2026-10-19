-- +goose Up
-- Handoffs carried out (S2-G fix, 3). handoff.request and handoff.approved
-- are delivered at least once and start a workspace-changing run: the Go
-- Core claims a handoff's stage (its request, its approval after the
-- quarantine) here before it starts anything, so a redelivered message does
-- nothing, and releases the claim when a transient error lets the message
-- be retried.
CREATE TABLE handoff_claims (
    tenant_id  UUID        NOT NULL,
    handoff_id TEXT        NOT NULL,
    stage      TEXT        NOT NULL CHECK (stage IN ('request', 'approved')),
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, handoff_id, stage)
);

-- +goose Down
DROP TABLE IF EXISTS handoff_claims;
