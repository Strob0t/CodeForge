# CodeForge Database Schema Audit Report

**Date:** 2026-03-07
**Tool:** `tools/db_schema_audit.py` (static analysis + Go store cross-reference)
**Scope:** 60 tables, 125 indexes, 58 migrations

---

## Overall Score

| Score | Grade | Status |
|-------|-------|--------|
| **0 / 100** | **F** | SCHEMA HEALTH WARNING |

> **The score of 0 indicates serious structural issues that must be addressed before production deployment.** The primary driver is 17 tables missing `tenant_id` (multi-tenant isolation) and 34 Go store queries operating without tenant scoping. These represent data isolation violations that could leak data between tenants.

---

## Summary by Severity

| Severity | Count | Deduction per Finding | Total Impact |
|----------|-------|-----------------------|--------------|
| Critical | 17 | -5 | -85 |
| High | 51 | -3 | -153 |
| Medium | 22 | -2 | -44 |
| Low | 45 | -1 | -45 |
| **Total** | **135** | | **-327 (capped at -100)** |

---

## Findings by Category

### 1. Multi-Tenant Isolation (Critical + High) — 51 findings

#### 1a. Tables Missing `tenant_id` — 17 Critical

| # | Table | SQL Patch |
|---|-------|-----------|
| 1 | `graph_metadata` | `ALTER TABLE graph_metadata ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 2 | `refresh_tokens` | `ALTER TABLE refresh_tokens ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 3 | `api_keys` | `ALTER TABLE api_keys ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 4 | `revoked_tokens` | `ALTER TABLE revoked_tokens ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 5 | `retrieval_scope_projects` | `ALTER TABLE retrieval_scope_projects ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 6 | `scope_knowledge_bases` | `ALTER TABLE scope_knowledge_bases ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 7 | `conversation_messages` | `ALTER TABLE conversation_messages ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 8 | `project_mcp_servers` | `ALTER TABLE project_mcp_servers ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 9 | `mcp_server_tools` | `ALTER TABLE mcp_server_tools ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 10 | `benchmark_runs` | `ALTER TABLE benchmark_runs ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 11 | `benchmark_results` | `ALTER TABLE benchmark_results ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 12 | `password_reset_tokens` | `ALTER TABLE password_reset_tokens ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 13 | `auto_agents` | `ALTER TABLE auto_agents ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 14 | `quarantine_messages` | `ALTER TABLE quarantine_messages ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 15 | `agent_inbox` | `ALTER TABLE agent_inbox ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 16 | `benchmark_suites` | `ALTER TABLE benchmark_suites ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |
| 17 | `a2a_push_configs` | `ALTER TABLE a2a_push_configs ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-...-000000000000';` |

#### 1b. Go Store Queries Missing `tenant_id` in WHERE — 34 High

| Store File | Table | Count |
|------------|-------|-------|
| `store_refresh_token.go` | `refresh_tokens` | 5 |
| `store_user.go` | `users` | 3 |
| `store_user.go` | `password_reset_tokens` | 2 |
| `store_api_key.go` | `api_keys` | 3 |
| `store_scope.go` | `retrieval_scope_projects` | 3 |
| `store_conversation.go` | `conversations` | 2 |
| `store_conversation.go` | `conversation_messages` | 1 |
| `store_mcp.go` | `project_mcp_servers` | 1 |
| `store_mcp.go` | `mcp_server_tools` | 1 |
| `store_tenant.go` | `tenants` | 2 |
| `store_a2a.go` | `a2a_push_configs` | 2 |
| `store_vcsaccount.go` | `vcs_accounts` | 2 |
| `store_knowledgebase.go` | `scope_knowledge_bases` | 1 |
| `store_knowledgebase.go` | `knowledge_bases` | 1 |
| `store_benchmark.go` | `benchmark_runs` | 1 |
| `store_benchmark_suite.go` | `benchmark_suites` | 1 |
| `store_autoagent.go` | `auto_agents` | 1 |
| `store.go` | `plan_steps` | 1 |

> **AI Assessment:** The `store_tenant.go` queries on `tenants` are **false positives** — the tenants table itself is the root of multi-tenancy and queries by slug/id are inherently correct. The `store_user.go` queries on `users` may also be partially justified for login/auth flows that look up by email before tenant context is established. All other findings are genuine isolation gaps.

---

### 2. Index Strategy — 20 findings (17 High, 10 Low)

#### Missing FK Indexes — 17 High

| Table | FK Column | References | Suggested Index |
|-------|-----------|------------|-----------------|
| `tasks` | `agent_id` | `agents` | `CREATE INDEX idx_tasks_agent_id ON tasks(agent_id);` |
| `execution_plans` | `team_id` | `agent_teams` | `CREATE INDEX idx_execution_plans_team_id ON execution_plans(team_id);` |
| `plan_steps` | `task_id` | `tasks` | `CREATE INDEX idx_plan_steps_task_id ON plan_steps(task_id);` |
| `plan_steps` | `agent_id` | `agents` | `CREATE INDEX idx_plan_steps_agent_id ON plan_steps(agent_id);` |
| `team_members` | `agent_id` | `agents` | `CREATE INDEX idx_team_members_agent_id ON team_members(agent_id);` |
| `context_packs` | `project_id` | `projects` | `CREATE INDEX idx_context_packs_project_id ON context_packs(project_id);` |
| `shared_contexts` | `project_id` | `projects` | `CREATE INDEX idx_shared_contexts_project_id ON shared_contexts(project_id);` |
| `sessions` | `parent_session_id` | `sessions` | `CREATE INDEX idx_sessions_parent_session_id ON sessions(parent_session_id);` |
| `sessions` | `parent_run_id` | `runs` | `CREATE INDEX idx_sessions_parent_run_id ON sessions(parent_run_id);` |
| `sessions` | `current_run_id` | `runs` | `CREATE INDEX idx_sessions_current_run_id ON sessions(current_run_id);` |
| `users` | `tenant_id` | `tenants` | `CREATE INDEX idx_users_tenant_id ON users(tenant_id);` |
| `retrieval_scope_projects` | `scope_id` | `retrieval_scopes` | `CREATE INDEX idx_retrieval_scope_projects_scope_id ON retrieval_scope_projects(scope_id);` |
| `scope_knowledge_bases` | `scope_id` | `retrieval_scopes` | `CREATE INDEX idx_scope_knowledge_bases_scope_id ON scope_knowledge_bases(scope_id);` |
| `project_mcp_servers` | `project_id` | `projects` | `CREATE INDEX idx_project_mcp_servers_project_id ON project_mcp_servers(project_id);` |
| `project_mcp_servers` | `mcp_server_id` | `mcp_servers` | `CREATE INDEX idx_project_mcp_servers_mcp_server_id ON project_mcp_servers(mcp_server_id);` |
| `mcp_server_tools` | `server_id` | `mcp_servers` | `CREATE INDEX idx_mcp_server_tools_server_id ON mcp_server_tools(server_id);` |
| `project_goals` | `tenant_id` | `tenants` | `CREATE INDEX idx_project_goals_tenant_id ON project_goals(tenant_id);` |

#### Redundant Indexes — 10 Low

| Table | Redundant Index | Subsumed By |
|-------|----------------|-------------|
| `tasks` | `idx_tasks_project_id (project_id)` | `idx_tasks_project_active (project_id, status)` |
| `tasks` | `idx_tasks_status (status)` | `idx_tasks_stale (status, updated_at)` |
| `agent_events` | `idx_agent_events_run_id (run_id)` | `idx_agent_events_tool_cost (run_id, event_type, tool_name)` |
| `team_members` | `idx_team_members_team_id (team_id)` | `idx_team_members_unique (team_id, agent_id)` |
| `shared_context_items` | `idx_shared_context_items_shared_id (shared_id)` | `idx_shared_context_items_key (shared_id, key)` |
| `conversation_messages` | `idx_conversation_messages_conv (conversation_id)` | `idx_conversation_messages_tool_call_id_unique (conversation_id, tool_call_id)` |
| `benchmark_results` | `idx_benchmark_results_run_id (run_id)` | `idx_bench_results_rollout (run_id, task_id, rollout_id)` |
| `agent_memories` | `idx_agent_memories_project (project_id)` | `idx_agent_memories_kind (project_id, kind)` |
| `experience_entries` | `idx_experience_entries_project (project_id)` | `idx_experience_entries_last_used (project_id, last_used_at)` |

> **Note:** Redundant indexes are not harmful to correctness but waste disk and slow writes. Dropping them is safe but low priority.

---

### 3. Data Type Consistency — 3 Medium

| Table | Issue |
|-------|-------|
| `mcp_servers` | Mixed VARCHAR and TEXT — columns `name`, `transport`, `command`, `url`, `status` use VARCHAR while others use TEXT |
| `mcp_server_tools` | Mixed VARCHAR and TEXT — column `name` uses VARCHAR while others use TEXT |
| (global) | **Inconsistent PK types** — 44 tables use UUID, 11 use TEXT, 1 uses SERIAL |

> **AI Assessment:** The VARCHAR/TEXT mixing in MCP tables (migration 036) is a minor consistency issue — PostgreSQL treats them identically for performance. The PK type inconsistency is more concerning: `graph_nodes`, `graph_metadata`, `users`, `refresh_tokens`, `api_keys`, `revoked_tokens`, `benchmark_*`, `password_reset_tokens`, and `a2a_tasks` use TEXT PKs instead of UUID. The TEXT PKs for `users` (email-based) and auth tokens are intentional design choices. The `graph_*` TEXT PKs should be reviewed.

---

### 4. Constraints — 19 Medium

19 tables have TEXT columns named `status`, `role`, `kind`, or `state` without CHECK constraints:

`agents.status`, `tasks.status`, `runs.status`, `execution_plans.status`, `plan_steps.status`, `agent_teams.status`, `team_members.role`, `context_entries.kind`, `graph_nodes.kind`, `graph_edges.kind`, `graph_metadata.status`, `roadmaps.status`, `milestones.status`, `features.status`, `sessions.status`, `users.role`, `auto_agents.status`, `quarantine_messages.status`, `a2a_tasks.state`

> **AI Assessment:** These are enforced at the application layer (Go constants/enums). Adding CHECK constraints would provide defense-in-depth but requires listing all valid values. **Recommended for `users.role`** (security-relevant: admin/user) and **`quarantine_messages.status`** (trust-critical: pending/approved/rejected). Others are lower priority.

---

### 5. JSONB Usage — 24 Low

#### Justified JSONB (keep as-is)

| Table.Column | Justification |
|-------------|---------------|
| `projects.config` | Project-specific settings vary per project type — relational would need EAV pattern |
| `agents.config` | Agent configuration varies by agent type (Aider, OpenHands, etc.) — schema-less is correct |
| `agents.resource_limits` | Small, nested config (CPU, memory, timeout) — not worth a separate table |
| `tasks.result` | Free-form task output from different agent types — heterogeneous structure |
| `agent_events.payload` | Event payload varies by event_type — classic polymorphic data |
| `runs.artifact_errors` | Error arrays vary in structure — rarely queried individually |
| `tenants.settings` | Tenant-specific settings — EAV alternative would be worse |
| `sessions.metadata` | Session-level key-value metadata — flexible by design |
| `settings.value` | Generic settings store — JSONB is the right choice for a KV table |
| `conversation_messages.tool_calls` | LLM tool call arrays — matches OpenAI API format, schema varies per tool |
| `mcp_servers.args` | CLI arguments array — simple list |
| `mcp_servers.env` | Environment variables map — naturally key-value |
| `mcp_servers.headers` | HTTP headers map — naturally key-value |
| `mcp_server_tools.input_schema` | JSON Schema definition — must be JSONB by definition |
| `benchmark_runs.summary_scores` | Evaluation scores vary by evaluator combination |
| `benchmark_results.scores` | Per-evaluator scores — heterogeneous structure |
| `benchmark_results.tool_calls` | Tool call traces — matches LLM API format |
| `benchmark_results.evaluator_scores` | Multi-evaluator results — schema varies |
| `agent_memories.metadata` | Memory-type-specific metadata — varies by kind |
| `benchmark_suites.config` | Suite configuration — varies per suite |
| `a2a_remote_agents.card_json` | A2A Agent Card — external protocol format, must store as-is |
| `a2a_tasks.metadata` | A2A task metadata — protocol-defined flexible structure |

#### Review Recommended

| Table.Column | Concern |
|-------------|---------|
| `features.external_ids` | Could be a `feature_external_ids(feature_id, provider, external_id)` table if queried by provider |
| `a2a_tasks.history` | Message history array — could be `a2a_task_messages` table for queryability |
| `a2a_tasks.artifacts` | Artifact array — could be `a2a_task_artifacts` table |

---

### 6. Partitioning — 3 Low

| Table | AI Assessment |
|-------|---------------|
| `agent_events` | **Recommended.** High-write, append-only event stream. Time-based partitioning (monthly) would significantly improve query performance for recent events and enable efficient archival. This is the highest-value partitioning candidate. |
| `audit_trail` | **Recommended.** Compliance/audit log grows unboundedly. Monthly partitioning enables retention policies (drop old partitions). |
| `feedback_audit` | **Not needed yet.** Low volume table for HITL feedback tracking. Monitor growth before partitioning. |

---

### 7. Naming Conventions — 8 Low

8 columns named `--` detected in `graph_nodes`, `graph_edges`, `graph_metadata`, `model_performance_stats`, `model_routing_outcomes`. These are SQL comment artifacts being parsed as column names — **false positives from the parser** (the `--` is a SQL comment prefix being incorrectly tokenized). No action needed.

---

### 8. Migration Quality — 0 findings

All 58 migrations have both Up and Down sections. No issues detected.

---

## Top 5 Prioritized Recommendations

### 1. Add `tenant_id` to 17 tables (Critical, Score Impact: -85)

**Why:** Without `tenant_id`, these tables have no mechanism for row-level tenant isolation. In a multi-tenant deployment, any query on these tables returns data from ALL tenants. This is a data leak vulnerability.

**Migration patch:**

```sql
-- +goose Up
ALTER TABLE graph_metadata ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE refresh_tokens ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE api_keys ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE revoked_tokens ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE retrieval_scope_projects ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE scope_knowledge_bases ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE conversation_messages ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE project_mcp_servers ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE mcp_server_tools ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE benchmark_runs ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE benchmark_results ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE password_reset_tokens ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE auto_agents ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE quarantine_messages ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE agent_inbox ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE benchmark_suites ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE a2a_push_configs ADD COLUMN tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

CREATE INDEX idx_graph_metadata_tenant ON graph_metadata(tenant_id);
CREATE INDEX idx_refresh_tokens_tenant ON refresh_tokens(tenant_id);
CREATE INDEX idx_api_keys_tenant ON api_keys(tenant_id);
CREATE INDEX idx_revoked_tokens_tenant ON revoked_tokens(tenant_id);
CREATE INDEX idx_retrieval_scope_projects_tenant ON retrieval_scope_projects(tenant_id);
CREATE INDEX idx_scope_knowledge_bases_tenant ON scope_knowledge_bases(tenant_id);
CREATE INDEX idx_conversation_messages_tenant ON conversation_messages(tenant_id);
CREATE INDEX idx_project_mcp_servers_tenant ON project_mcp_servers(tenant_id);
CREATE INDEX idx_mcp_server_tools_tenant ON mcp_server_tools(tenant_id);
CREATE INDEX idx_benchmark_runs_tenant ON benchmark_runs(tenant_id);
CREATE INDEX idx_benchmark_results_tenant ON benchmark_results(tenant_id);
CREATE INDEX idx_password_reset_tokens_tenant ON password_reset_tokens(tenant_id);
CREATE INDEX idx_auto_agents_tenant ON auto_agents(tenant_id);
CREATE INDEX idx_quarantine_messages_tenant ON quarantine_messages(tenant_id);
CREATE INDEX idx_agent_inbox_tenant ON agent_inbox(tenant_id);
CREATE INDEX idx_benchmark_suites_tenant ON benchmark_suites(tenant_id);
CREATE INDEX idx_a2a_push_configs_tenant ON a2a_push_configs(tenant_id);
```

### 2. Add `tenant_id` to Go store WHERE clauses (High, Score Impact: -102)

**Why:** Even tables WITH `tenant_id` columns are not protected if Go queries don't filter by it. 34 queries across 13 store files need tenant scoping.

**Action:** For each store file listed in section 1b, add `AND tenant_id = $N` to every WHERE clause. Exclude `store_tenant.go` (false positive) and auth-flow queries in `store_user.go` that intentionally query cross-tenant.

### 3. Add missing FK indexes (High, Score Impact: -51)

**Why:** 17 FK columns lack indexes. Without indexes, JOINs and cascading deletes perform full table scans. This gets progressively worse as tables grow.

**Action:** Apply the 17 `CREATE INDEX` statements from section 2.

### 4. Add CHECK constraints to security-critical enum columns (Medium)

**Why:** `users.role` and `quarantine_messages.status` govern access control and trust decisions. Application bugs could insert invalid values.

```sql
ALTER TABLE users ADD CONSTRAINT chk_users_role CHECK (role IN ('admin', 'user'));
ALTER TABLE quarantine_messages ADD CONSTRAINT chk_quarantine_status CHECK (status IN ('pending', 'approved', 'rejected'));
```

### 5. Partition `agent_events` by time (Low, but high operational value)

**Why:** `agent_events` is the highest-write table (every tool call, every LLM response). Without partitioning, historical queries degrade as the table grows.

```sql
-- Requires recreating the table with PARTITION BY RANGE (created_at)
-- and migrating existing data. Plan for a maintenance window.
```

---

## Appendix: Statistics

- Tables analyzed: 60
- Indexes analyzed: 125
- Migrations analyzed: 58
- Go store files cross-referenced: 18
- False positive rate: ~5% (tenants table queries, parser comment artifacts)
