-- +goose Up

-- Prompt score signals collected from benchmark results, user feedback, run outcomes.
CREATE TABLE prompt_scores (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    prompt_fingerprint  TEXT NOT NULL,
    mode_id             TEXT NOT NULL,
    model_family        TEXT NOT NULL,
    signal_type         TEXT NOT NULL CHECK (signal_type IN ('benchmark', 'success', 'cost', 'user', 'efficiency')),
    score               DOUBLE PRECISION NOT NULL,
    run_id              UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_prompt_scores_fingerprint ON prompt_scores (prompt_fingerprint);
CREATE INDEX idx_prompt_scores_mode_model ON prompt_scores (tenant_id, mode_id, model_family);

-- Extend prompt_sections with evolution tracking columns.
ALTER TABLE prompt_sections ADD COLUMN IF NOT EXISTS version          INT NOT NULL DEFAULT 1;
ALTER TABLE prompt_sections ADD COLUMN IF NOT EXISTS parent_id        TEXT;
ALTER TABLE prompt_sections ADD COLUMN IF NOT EXISTS mutation_source   TEXT;
ALTER TABLE prompt_sections ADD COLUMN IF NOT EXISTS promotion_status  TEXT NOT NULL DEFAULT 'candidate';
ALTER TABLE prompt_sections ADD COLUMN IF NOT EXISTS trial_count       INT NOT NULL DEFAULT 0;
ALTER TABLE prompt_sections ADD COLUMN IF NOT EXISTS avg_score         DOUBLE PRECISION NOT NULL DEFAULT 0.0;
ALTER TABLE prompt_sections ADD COLUMN IF NOT EXISTS mode_id           TEXT NOT NULL DEFAULT '';
ALTER TABLE prompt_sections ADD COLUMN IF NOT EXISTS model_family      TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_prompt_sections_evolution ON prompt_sections (tenant_id, mode_id, model_family, promotion_status);

-- Track which prompt assembly was used for each run.
ALTER TABLE runs ADD COLUMN IF NOT EXISTS prompt_fingerprint TEXT;

-- User feedback on messages (thumbs up/down).
ALTER TABLE conversation_messages ADD COLUMN IF NOT EXISTS feedback_score INT;

-- +goose Down

DROP INDEX IF EXISTS idx_prompt_scores_fingerprint;
DROP INDEX IF EXISTS idx_prompt_scores_mode_model;
DROP TABLE IF EXISTS prompt_scores;

ALTER TABLE prompt_sections DROP COLUMN IF EXISTS version;
ALTER TABLE prompt_sections DROP COLUMN IF EXISTS parent_id;
ALTER TABLE prompt_sections DROP COLUMN IF EXISTS mutation_source;
ALTER TABLE prompt_sections DROP COLUMN IF EXISTS promotion_status;
ALTER TABLE prompt_sections DROP COLUMN IF EXISTS trial_count;
ALTER TABLE prompt_sections DROP COLUMN IF EXISTS avg_score;
ALTER TABLE prompt_sections DROP COLUMN IF EXISTS mode_id;
ALTER TABLE prompt_sections DROP COLUMN IF EXISTS model_family;

DROP INDEX IF EXISTS idx_prompt_sections_evolution;

ALTER TABLE runs DROP COLUMN IF EXISTS prompt_fingerprint;
ALTER TABLE conversation_messages DROP COLUMN IF EXISTS feedback_score;
