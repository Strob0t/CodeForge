-- +goose Up

-- Missing foreign key indexes: JOINs and cascading deletes may do full table scans without these.
CREATE INDEX IF NOT EXISTS idx_tasks_agent_id ON tasks(agent_id);
CREATE INDEX IF NOT EXISTS idx_execution_plans_team_id ON execution_plans(team_id);
CREATE INDEX IF NOT EXISTS idx_plan_steps_task_id ON plan_steps(task_id);
CREATE INDEX IF NOT EXISTS idx_plan_steps_agent_id ON plan_steps(agent_id);
CREATE INDEX IF NOT EXISTS idx_team_members_agent_id ON team_members(agent_id);
CREATE INDEX IF NOT EXISTS idx_context_packs_project_id ON context_packs(project_id);
CREATE INDEX IF NOT EXISTS idx_shared_contexts_project_id ON shared_contexts(project_id);
CREATE INDEX IF NOT EXISTS idx_sessions_parent_session_id ON sessions(parent_session_id);
CREATE INDEX IF NOT EXISTS idx_sessions_parent_run_id ON sessions(parent_run_id);
CREATE INDEX IF NOT EXISTS idx_sessions_current_run_id ON sessions(current_run_id);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_retrieval_scope_projects_scope_id ON retrieval_scope_projects(scope_id);
CREATE INDEX IF NOT EXISTS idx_scope_knowledge_bases_scope_id ON scope_knowledge_bases(scope_id);
CREATE INDEX IF NOT EXISTS idx_project_mcp_servers_project_id ON project_mcp_servers(project_id);
CREATE INDEX IF NOT EXISTS idx_project_mcp_servers_mcp_server_id ON project_mcp_servers(mcp_server_id);
CREATE INDEX IF NOT EXISTS idx_mcp_server_tools_server_id ON mcp_server_tools(server_id);
CREATE INDEX IF NOT EXISTS idx_project_goals_tenant_id ON project_goals(tenant_id);

-- +goose Down

DROP INDEX IF EXISTS idx_project_goals_tenant_id;
DROP INDEX IF EXISTS idx_mcp_server_tools_server_id;
DROP INDEX IF EXISTS idx_project_mcp_servers_mcp_server_id;
DROP INDEX IF EXISTS idx_project_mcp_servers_project_id;
DROP INDEX IF EXISTS idx_scope_knowledge_bases_scope_id;
DROP INDEX IF EXISTS idx_retrieval_scope_projects_scope_id;
DROP INDEX IF EXISTS idx_users_tenant_id;
DROP INDEX IF EXISTS idx_sessions_current_run_id;
DROP INDEX IF EXISTS idx_sessions_parent_run_id;
DROP INDEX IF EXISTS idx_sessions_parent_session_id;
DROP INDEX IF EXISTS idx_shared_contexts_project_id;
DROP INDEX IF EXISTS idx_context_packs_project_id;
DROP INDEX IF EXISTS idx_team_members_agent_id;
DROP INDEX IF EXISTS idx_plan_steps_agent_id;
DROP INDEX IF EXISTS idx_plan_steps_task_id;
DROP INDEX IF EXISTS idx_execution_plans_team_id;
DROP INDEX IF EXISTS idx_tasks_agent_id;
