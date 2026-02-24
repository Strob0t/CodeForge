# Backend QA Test Plan — CodeForge

**Generated:** 2026-02-24
**Backend:** Go Core Service (port 8080)
**Dependencies:** PostgreSQL 17, NATS JetStream, LiteLLM Proxy (port 4000)
**Auth Mode:** Disabled (default admin injected)

---

## MODULE 1: Health & Infrastructure
- `GET /health` — Liveness probe (always 200)
- `GET /health/ready` — Readiness probe (checks Postgres, NATS, LiteLLM)
- `GET /api/v1/` — Version endpoint

## MODULE 2: Project CRUD
- `POST /api/v1/projects` — Create project
- `GET /api/v1/projects` — List projects
- `GET /api/v1/projects/{id}` — Get project by ID
- `PUT /api/v1/projects/{id}` — Update project
- `DELETE /api/v1/projects/{id}` — Delete project
- Edge cases: missing name, invalid provider, duplicate name, nonexistent ID, path traversal

## MODULE 3: Project Operations (Clone/Adopt/Setup/Workspace)
- `POST /api/v1/projects/{id}/clone` — Clone repo
- `POST /api/v1/projects/{id}/adopt` — Adopt local path
- `POST /api/v1/projects/{id}/setup` — Full setup chain
- `GET /api/v1/projects/{id}/workspace` — Workspace info
- `POST /api/v1/parse-repo-url` — Parse repo URL
- `GET /api/v1/projects/remote-branches` — List remote branches

## MODULE 4: Stack Detection
- `GET /api/v1/projects/{id}/detect-stack` — Detect project stack
- `POST /api/v1/detect-stack` — Detect stack by path

## MODULE 5: Git Operations
- `GET /api/v1/projects/{id}/git/status` — Git status
- `POST /api/v1/projects/{id}/git/pull` — Pull latest
- `GET /api/v1/projects/{id}/git/branches` — List branches
- `POST /api/v1/projects/{id}/git/checkout` — Checkout branch

## MODULE 6: Agent Management
- `POST /api/v1/projects/{id}/agents` — Create agent
- `GET /api/v1/projects/{id}/agents` — List agents
- `GET /api/v1/agents/{id}` — Get agent
- `DELETE /api/v1/agents/{id}` — Delete agent
- `POST /api/v1/agents/{id}/dispatch` — Dispatch task
- `POST /api/v1/agents/{id}/stop` — Stop agent

## MODULE 7: Task Management
- `POST /api/v1/projects/{id}/tasks` — Create task
- `GET /api/v1/projects/{id}/tasks` — List tasks
- `GET /api/v1/tasks/{id}` — Get task
- `GET /api/v1/tasks/{id}/events` — List task events
- `GET /api/v1/tasks/{id}/runs` — List task runs
- `GET /api/v1/tasks/{id}/context` — Get context pack
- `POST /api/v1/tasks/{id}/context` — Build context pack

## MODULE 8: Run Management
- `POST /api/v1/runs` — Start run
- `GET /api/v1/runs/{id}` — Get run
- `POST /api/v1/runs/{id}/cancel` — Cancel run
- `GET /api/v1/runs/{id}/events` — List run events

## MODULE 9: Execution Plans (Orchestration)
- `POST /api/v1/projects/{id}/decompose` — Decompose feature
- `POST /api/v1/projects/{id}/plan-feature` — Plan feature
- `POST /api/v1/projects/{id}/plans` — Create plan
- `GET /api/v1/projects/{id}/plans` — List plans
- `GET /api/v1/plans/{id}` — Get plan
- `POST /api/v1/plans/{id}/start` — Start plan
- `POST /api/v1/plans/{id}/cancel` — Cancel plan

## MODULE 10: Agent Teams
- `POST /api/v1/projects/{id}/teams` — Create team
- `GET /api/v1/projects/{id}/teams` — List teams
- `GET /api/v1/teams/{id}` — Get team
- `DELETE /api/v1/teams/{id}` — Delete team
- `GET /api/v1/teams/{id}/shared-context` — Get shared context
- `POST /api/v1/teams/{id}/shared-context` — Add shared context

## MODULE 11: Modes & Scenarios
- `GET /api/v1/modes` — List modes
- `GET /api/v1/modes/scenarios` — List scenarios
- `GET /api/v1/modes/{id}` — Get mode
- `POST /api/v1/modes` — Create mode
- `PUT /api/v1/modes/{id}` — Update mode

## MODULE 12: Pipeline Templates
- `GET /api/v1/pipelines` — List pipelines
- `POST /api/v1/pipelines` — Register pipeline
- `GET /api/v1/pipelines/{id}` — Get pipeline
- `POST /api/v1/pipelines/{id}/instantiate` — Instantiate pipeline

## MODULE 13: Roadmap & Features
- `GET /api/v1/projects/{id}/roadmap` — Get roadmap
- `POST /api/v1/projects/{id}/roadmap` — Create roadmap
- `PUT /api/v1/projects/{id}/roadmap` — Update roadmap
- `DELETE /api/v1/projects/{id}/roadmap` — Delete roadmap
- `POST /api/v1/projects/{id}/roadmap/detect` — Detect specs
- `POST /api/v1/projects/{id}/roadmap/import` — Import specs
- `POST /api/v1/projects/{id}/roadmap/sync-to-file` — Sync to file
- `POST /api/v1/projects/{id}/roadmap/milestones` — Create milestone
- `GET /api/v1/milestones/{id}` — Get milestone
- `PUT /api/v1/milestones/{id}` — Update milestone
- `DELETE /api/v1/milestones/{id}` — Delete milestone
- `POST /api/v1/milestones/{id}/features` — Create feature
- `GET /api/v1/features/{id}` — Get feature
- `PUT /api/v1/features/{id}` — Update feature
- `DELETE /api/v1/features/{id}` — Delete feature

## MODULE 14: Policy Management
- `GET /api/v1/policies` — List policies
- `POST /api/v1/policies` — Create policy
- `GET /api/v1/policies/{name}` — Get policy
- `DELETE /api/v1/policies/{name}` — Delete policy
- `POST /api/v1/policies/{name}/evaluate` — Evaluate policy

## MODULE 15: Cost Tracking
- `GET /api/v1/costs` — Global cost summary
- `GET /api/v1/projects/{id}/costs` — Project cost summary
- `GET /api/v1/projects/{id}/costs/by-model` — Cost by model
- `GET /api/v1/projects/{id}/costs/by-tool` — Cost by tool
- `GET /api/v1/projects/{id}/costs/daily` — Daily cost time series
- `GET /api/v1/projects/{id}/costs/runs` — Recent runs with cost
- `GET /api/v1/runs/{id}/costs/by-tool` — Run cost by tool

## MODULE 16: Retrieval & Search
- `POST /api/v1/projects/{id}/search` — Search project
- `POST /api/v1/projects/{id}/search/agent` — Agent search
- `POST /api/v1/projects/{id}/index` — Index project
- `GET /api/v1/projects/{id}/index` — Get index status

## MODULE 17: GraphRAG
- `POST /api/v1/projects/{id}/graph/build` — Build graph
- `GET /api/v1/projects/{id}/graph/status` — Graph status
- `POST /api/v1/projects/{id}/graph/search` — Search graph

## MODULE 18: Scopes (Cross-Project)
- `POST /api/v1/scopes` — Create scope
- `GET /api/v1/scopes` — List scopes
- `GET /api/v1/scopes/{id}` — Get scope
- `PUT /api/v1/scopes/{id}` — Update scope
- `DELETE /api/v1/scopes/{id}` — Delete scope
- `POST /api/v1/scopes/{id}/projects` — Add project to scope
- `DELETE /api/v1/scopes/{id}/projects/{pid}` — Remove project
- `POST /api/v1/scopes/{id}/search` — Search scope
- `POST /api/v1/scopes/{id}/graph/search` — Graph search scope

## MODULE 19: Knowledge Bases
- `GET /api/v1/knowledge-bases` — List KBs
- `POST /api/v1/knowledge-bases` — Create KB
- `GET /api/v1/knowledge-bases/{id}` — Get KB
- `PUT /api/v1/knowledge-bases/{id}` — Update KB
- `DELETE /api/v1/knowledge-bases/{id}` — Delete KB
- `POST /api/v1/knowledge-bases/{id}/index` — Index KB
- `POST /api/v1/scopes/{id}/knowledge-bases` — Attach KB to scope
- `DELETE /api/v1/scopes/{id}/knowledge-bases/{kbid}` — Detach KB
- `GET /api/v1/scopes/{id}/knowledge-bases` — List scope KBs

## MODULE 20: Conversations (Chat)
- `POST /api/v1/projects/{id}/conversations` — Create conversation
- `GET /api/v1/projects/{id}/conversations` — List conversations
- `GET /api/v1/conversations/{id}` — Get conversation
- `DELETE /api/v1/conversations/{id}` — Delete conversation
- `GET /api/v1/conversations/{id}/messages` — List messages
- `POST /api/v1/conversations/{id}/messages` — Send message

## MODULE 21: RepoMap
- `GET /api/v1/projects/{id}/repomap` — Get repomap
- `POST /api/v1/projects/{id}/repomap` — Generate repomap

## MODULE 22: Trajectory & History
- `GET /api/v1/runs/{id}/trajectory` — Get trajectory
- `GET /api/v1/runs/{id}/trajectory/export` — Export trajectory
- `GET /api/v1/runs/{id}/checkpoints` — List checkpoints
- `POST /api/v1/runs/{id}/replay` — Replay run

## MODULE 23: Session Management
- `POST /api/v1/runs/{id}/resume` — Resume run
- `POST /api/v1/runs/{id}/fork` — Fork run
- `POST /api/v1/runs/{id}/rewind` — Rewind run
- `GET /api/v1/projects/{id}/sessions` — List sessions
- `GET /api/v1/sessions/{id}` — Get session

## MODULE 24: Audit Trail
- `GET /api/v1/audit` — Global audit trail
- `GET /api/v1/projects/{id}/audit` — Project audit trail

## MODULE 25: Review Policies
- `POST /api/v1/projects/{id}/review-policies` — Create review policy
- `GET /api/v1/projects/{id}/review-policies` — List review policies
- `GET /api/v1/review-policies/{id}` — Get review policy
- `PUT /api/v1/review-policies/{id}` — Update review policy
- `DELETE /api/v1/review-policies/{id}` — Delete review policy
- `POST /api/v1/review-policies/{id}/trigger` — Trigger review
- `GET /api/v1/projects/{id}/reviews` — List reviews
- `GET /api/v1/reviews/{id}` — Get review

## MODULE 26: Branch Protection
- `POST /api/v1/projects/{id}/branch-rules` — Create rule
- `GET /api/v1/projects/{id}/branch-rules` — List rules
- `GET /api/v1/branch-rules/{id}` — Get rule
- `PUT /api/v1/branch-rules/{id}` — Update rule
- `DELETE /api/v1/branch-rules/{id}` — Delete rule

## MODULE 27: Settings
- `GET /api/v1/settings` — Get settings
- `PUT /api/v1/settings` — Update settings

## MODULE 28: Authentication
- `POST /api/v1/auth/login` — Login
- `POST /api/v1/auth/refresh` — Refresh token
- `POST /api/v1/auth/logout` — Logout
- `GET /api/v1/auth/me` — Get current user
- `POST /api/v1/auth/change-password` — Change password

## MODULE 29: API Key Management
- `POST /api/v1/auth/api-keys` — Create API key
- `GET /api/v1/auth/api-keys` — List API keys
- `DELETE /api/v1/auth/api-keys/{id}` — Delete API key

## MODULE 30: VCS Accounts
- `GET /api/v1/vcs-accounts` — List VCS accounts
- `POST /api/v1/vcs-accounts` — Create VCS account
- `DELETE /api/v1/vcs-accounts/{id}` — Delete VCS account
- `POST /api/v1/vcs-accounts/{id}/test` — Test VCS account

## MODULE 31: LLM Management
- `GET /api/v1/llm/models` — List models
- `POST /api/v1/llm/models` — Add model
- `POST /api/v1/llm/models/delete` — Delete model
- `GET /api/v1/llm/health` — LLM health

## MODULE 32: Provider Registries
- `GET /api/v1/providers/git` — Git providers
- `GET /api/v1/providers/agent` — Agent backends
- `GET /api/v1/providers/spec` — Spec providers
- `GET /api/v1/providers/pm` — PM providers

## MODULE 33: Tenant Management (Admin)
- `GET /api/v1/tenants` — List tenants
- `POST /api/v1/tenants` — Create tenant
- `GET /api/v1/tenants/{id}` — Get tenant
- `PUT /api/v1/tenants/{id}` — Update tenant

## MODULE 34: User Management (Admin)
- `GET /api/v1/users` — List users
- `POST /api/v1/users` — Create user
- `PUT /api/v1/users/{id}` — Update user
- `DELETE /api/v1/users/{id}` — Delete user

## MODULE 35: WebSocket
- `GET /ws?token=<jwt>` — WebSocket upgrade

## MODULE 36: Middleware & Security
- CORS headers
- Security headers
- Rate limiting
- Idempotency keys
- Request ID tracking

## MODULE 37: Error Handling & Edge Cases
- 404 for nonexistent resources
- 400 for invalid payloads
- 409 for version conflicts
- Response time baseline (<2000ms)
- SQL injection attempts
- XSS payload rejection

---

**TOTAL: 37 modules, ~160 endpoints/functions identified.**
