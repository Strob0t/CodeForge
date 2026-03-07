-- +goose Up
-- Composite indexes on agent_events for multi-tenant queries.
-- All queries filter by tenant_id; without it as prefix, PG does a full scan.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_events_tenant_task
    ON agent_events (tenant_id, task_id, version);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_events_tenant_agent
    ON agent_events (tenant_id, agent_id, version);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_events_tenant_run
    ON agent_events (tenant_id, run_id, version);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_agent_events_tenant_project
    ON agent_events (tenant_id, project_id, created_at);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS idx_agent_events_tenant_project;
DROP INDEX CONCURRENTLY IF EXISTS idx_agent_events_tenant_run;
DROP INDEX CONCURRENTLY IF EXISTS idx_agent_events_tenant_agent;
DROP INDEX CONCURRENTLY IF EXISTS idx_agent_events_tenant_task;
