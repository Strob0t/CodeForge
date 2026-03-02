-- +goose Up
-- Phase 26: Intelligent model routing — MAB state and routing outcomes.

CREATE TABLE model_performance_stats (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    model_name      TEXT NOT NULL,
    task_type       TEXT NOT NULL,
    complexity_tier TEXT NOT NULL,

    -- MAB statistics
    trial_count     INTEGER NOT NULL DEFAULT 0,
    total_reward    DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    avg_reward      DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    avg_cost_usd    DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    avg_latency_ms  BIGINT NOT NULL DEFAULT 0,
    avg_quality     DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    last_selected   TIMESTAMPTZ,

    -- Model capabilities (populated from LiteLLM metadata)
    supports_tools  BOOLEAN NOT NULL DEFAULT FALSE,
    supports_vision BOOLEAN NOT NULL DEFAULT FALSE,
    max_context     INTEGER NOT NULL DEFAULT 0,
    input_cost_per  DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    output_cost_per DOUBLE PRECISION NOT NULL DEFAULT 0.0,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(tenant_id, model_name, task_type, complexity_tier)
);

CREATE INDEX idx_mps_task_tier ON model_performance_stats(task_type, complexity_tier);
CREATE INDEX idx_mps_model ON model_performance_stats(model_name);
CREATE INDEX idx_mps_tenant ON model_performance_stats(tenant_id);

CREATE TABLE model_routing_outcomes (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    model_name       TEXT NOT NULL,
    task_type        TEXT NOT NULL,
    complexity_tier  TEXT NOT NULL,

    -- Outcome metrics
    success          BOOLEAN NOT NULL DEFAULT TRUE,
    quality_score    DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    cost_usd         DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    latency_ms       BIGINT NOT NULL DEFAULT 0,
    tokens_in        INTEGER NOT NULL DEFAULT 0,
    tokens_out       INTEGER NOT NULL DEFAULT 0,
    reward           DOUBLE PRECISION NOT NULL DEFAULT 0.0,

    -- Context
    routing_layer    TEXT NOT NULL DEFAULT 'mab',
    run_id           TEXT,
    conversation_id  TEXT,
    prompt_hash      TEXT,

    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_mro_model_task ON model_routing_outcomes(model_name, task_type, complexity_tier);
CREATE INDEX idx_mro_created ON model_routing_outcomes(created_at);
CREATE INDEX idx_mro_tenant ON model_routing_outcomes(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS model_routing_outcomes;
DROP TABLE IF EXISTS model_performance_stats;
