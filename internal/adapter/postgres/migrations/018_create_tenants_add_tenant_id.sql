-- +goose Up

-- Tenants table for multi-tenancy support.
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Default tenant (matches DefaultTenantID in middleware/tenant.go).
INSERT INTO tenants (id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000000', 'Default', 'default');

-- Add tenant_id to tables that are missing it.
-- Tables that already have tenant_id: projects, tasks, agents, runs, agent_events, execution_plans, agent_teams, roadmaps.

ALTER TABLE plan_steps ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE team_members ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE context_packs ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE context_entries ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE shared_contexts ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE shared_context_items ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE repo_maps ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE milestones ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE features ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE graph_nodes ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE graph_edges ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

-- Fix roadmaps.tenant_id type: was TEXT in migration 017, should be UUID.
-- Drop old column and add new UUID column.
ALTER TABLE roadmaps DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE roadmaps ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

-- Indexes for tenant-scoped queries on newly added columns.
CREATE INDEX idx_plan_steps_tenant ON plan_steps (tenant_id);
CREATE INDEX idx_context_packs_tenant ON context_packs (tenant_id);
CREATE INDEX idx_shared_contexts_tenant ON shared_contexts (tenant_id);
CREATE INDEX idx_repo_maps_tenant ON repo_maps (tenant_id);
CREATE INDEX idx_milestones_tenant ON milestones (tenant_id);
CREATE INDEX idx_features_tenant ON features (tenant_id);
CREATE INDEX idx_roadmaps_tenant ON roadmaps (tenant_id);
CREATE INDEX idx_graph_nodes_tenant ON graph_nodes (tenant_id);
CREATE INDEX idx_graph_edges_tenant ON graph_edges (tenant_id);

-- +goose Down
DROP INDEX IF EXISTS idx_graph_edges_tenant;
DROP INDEX IF EXISTS idx_graph_nodes_tenant;
DROP INDEX IF EXISTS idx_roadmaps_tenant;
DROP INDEX IF EXISTS idx_features_tenant;
DROP INDEX IF EXISTS idx_milestones_tenant;
DROP INDEX IF EXISTS idx_repo_maps_tenant;
DROP INDEX IF EXISTS idx_shared_contexts_tenant;
DROP INDEX IF EXISTS idx_context_packs_tenant;
DROP INDEX IF EXISTS idx_plan_steps_tenant;

-- Restore roadmaps.tenant_id as TEXT.
ALTER TABLE roadmaps DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE roadmaps ADD COLUMN tenant_id TEXT NOT NULL DEFAULT '';

ALTER TABLE graph_edges DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE graph_nodes DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE features DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE milestones DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE repo_maps DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE shared_context_items DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE shared_contexts DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE context_entries DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE context_packs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE team_members DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE plan_steps DROP COLUMN IF EXISTS tenant_id;

DROP TABLE IF EXISTS tenants;
