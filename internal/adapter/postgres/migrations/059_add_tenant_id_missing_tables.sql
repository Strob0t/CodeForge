-- +goose Up

-- Critical: 6 tables with no tenant isolation (security risk in multi-tenant deployments)
ALTER TABLE quarantine_messages ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE benchmark_runs ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE benchmark_results ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE benchmark_suites ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE auto_agents ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE agent_inbox ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

-- Defense-in-depth: 10 join/child/auth tables (inherit tenant via FK but direct column enables filtering without JOIN)
ALTER TABLE a2a_push_configs ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE graph_metadata ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE retrieval_scope_projects ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE scope_knowledge_bases ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE conversation_messages ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE project_mcp_servers ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE mcp_server_tools ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE refresh_tokens ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE api_keys ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE password_reset_tokens ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

-- Indexes for all new tenant_id columns
CREATE INDEX idx_quarantine_messages_tenant ON quarantine_messages(tenant_id);
CREATE INDEX idx_benchmark_runs_tenant ON benchmark_runs(tenant_id);
CREATE INDEX idx_benchmark_results_tenant ON benchmark_results(tenant_id);
CREATE INDEX idx_benchmark_suites_tenant ON benchmark_suites(tenant_id);
CREATE INDEX idx_auto_agents_tenant ON auto_agents(tenant_id);
CREATE INDEX idx_agent_inbox_tenant ON agent_inbox(tenant_id);
CREATE INDEX idx_a2a_push_configs_tenant ON a2a_push_configs(tenant_id);
CREATE INDEX idx_graph_metadata_tenant ON graph_metadata(tenant_id);
CREATE INDEX idx_retrieval_scope_projects_tenant ON retrieval_scope_projects(tenant_id);
CREATE INDEX idx_scope_knowledge_bases_tenant ON scope_knowledge_bases(tenant_id);
CREATE INDEX idx_conversation_messages_tenant ON conversation_messages(tenant_id);
CREATE INDEX idx_project_mcp_servers_tenant ON project_mcp_servers(tenant_id);
CREATE INDEX idx_mcp_server_tools_tenant ON mcp_server_tools(tenant_id);
CREATE INDEX idx_refresh_tokens_tenant ON refresh_tokens(tenant_id);
CREATE INDEX idx_api_keys_tenant ON api_keys(tenant_id);
CREATE INDEX idx_password_reset_tokens_tenant ON password_reset_tokens(tenant_id);

-- +goose Down

-- Drop indexes
DROP INDEX IF EXISTS idx_password_reset_tokens_tenant;
DROP INDEX IF EXISTS idx_api_keys_tenant;
DROP INDEX IF EXISTS idx_refresh_tokens_tenant;
DROP INDEX IF EXISTS idx_mcp_server_tools_tenant;
DROP INDEX IF EXISTS idx_project_mcp_servers_tenant;
DROP INDEX IF EXISTS idx_conversation_messages_tenant;
DROP INDEX IF EXISTS idx_scope_knowledge_bases_tenant;
DROP INDEX IF EXISTS idx_retrieval_scope_projects_tenant;
DROP INDEX IF EXISTS idx_graph_metadata_tenant;
DROP INDEX IF EXISTS idx_a2a_push_configs_tenant;
DROP INDEX IF EXISTS idx_agent_inbox_tenant;
DROP INDEX IF EXISTS idx_auto_agents_tenant;
DROP INDEX IF EXISTS idx_benchmark_suites_tenant;
DROP INDEX IF EXISTS idx_benchmark_results_tenant;
DROP INDEX IF EXISTS idx_benchmark_runs_tenant;
DROP INDEX IF EXISTS idx_quarantine_messages_tenant;

-- Drop columns (reverse order)
ALTER TABLE password_reset_tokens DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE mcp_server_tools DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE project_mcp_servers DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE conversation_messages DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE scope_knowledge_bases DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE retrieval_scope_projects DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE graph_metadata DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE a2a_push_configs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE agent_inbox DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE auto_agents DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE benchmark_suites DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE benchmark_results DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE benchmark_runs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE quarantine_messages DROP COLUMN IF EXISTS tenant_id;
