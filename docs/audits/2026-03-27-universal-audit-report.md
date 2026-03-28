# Universal Audit Report — 2026-03-27

**Date:** 2026-03-27
**Auditor:** Claude Opus 4.6 (Universal Audit Prompt v2)
**Branch:** `staging`
**Scope:** Full monorepo — Go core, Python workers, TypeScript frontend, PostgreSQL, Docker, CI/CD, configs (~280K LOC, 2,074 files)
**Method:** 5 parallel specialist agents (Security, Code Quality, Architecture, Infrastructure, Compliance)

---

## Audit Summary

| Metric               | Value                              |
|-----------------------|------------------------------------|
| Target                | `/workspaces/CodeForge` (monorepo) |
| Input Type            | Mixed / Monorepo (Go + Python + TypeScript + SQL + Docker + CI/CD) |
| Files Analyzed        | ~2,074                             |
| Total Findings        | 77                                 |
| Critical              | 6                                  |
| High                  | 19                                 |
| Medium                | 37                                 |
| Low                   | 12                                 |
| Informational         | 3                                  |
| **Overall Risk Score**| **52 / 100**                       |

---

## Risk Heatmap

| Dimension          | Weight | Score /100 | Top Issue                                              |
|--------------------|--------|-----------|--------------------------------------------------------|
| Security           | 30%    | 55        | Live API keys on disk (.env)                           |
| Code Quality       | 25%    | 50        | Python workers have near-zero unit test coverage       |
| Architecture       | 20%    | 45        | All 48 services accept monolithic `database.Store` ISP violation |
| Infrastructure     | 15%    | 60        | Blue-green Traefik port mismatch — deployment broken   |
| Compliance         | 10%    | 55        | Audit log anonymization missing tenant isolation (GDPR) |
| **Weighted Total** |        | **52**    | **Blue-green deployment is non-functional**            |

*Score = risk level (higher = worse). 0 = no issues, 100 = critical systemic failures.*

---

## CRITICAL Findings (6)

### SEC-001 · Live API keys committed to `.env` on disk
| Field | Value |
|---|---|
| Severity | CRITICAL |
| Dimension | Security |
| Location | `.env:36-44`, `.env:51`, `data/.env` |
| Evidence | `ANTHROPIC_API_KEY=sk-ant-api03-...`, `GITHUB_TOKEN=github_pat_11AMQKT6A0...`, `OPENROUTER_API_KEY=sk-or-v1-...`, plus 7 more provider keys. While `.env` is in `.gitignore`, these are real credentials on disk. |
| Risk | Anyone with filesystem access (container escape, backup leak, accidental copy) obtains live keys for 10+ LLM providers and GitHub. |
| Remediation | (1) Rotate ALL exposed keys immediately. (2) Use a secrets manager or Docker secrets. (3) Add a pre-commit hook scanning for high-entropy strings. |
| Reference | CWE-798, OWASP A07:2021 |
| Confidence | high |

### INFRA-001 · Traefik blue-green frontend port mismatch — deployment broken
| Field | Value |
|---|---|
| Severity | CRITICAL |
| Dimension | Infrastructure |
| Location | `docker-compose.blue-green.yml:59,80` |
| Evidence | Traefik labels set `server.port=80` but `Dockerfile.frontend` exposes 8080 and `nginx.conf` listens on 8080. |
| Risk | Blue-green deployment is completely non-functional. All frontend requests fail with connection refused. |
| Remediation | Change both `frontend-blue` and `frontend-green` Traefik labels to `server.port=8080`. |
| Confidence | high |

### INFRA-002 · PostgreSQL production SSL certificates not provisioned
| Field | Value |
|---|---|
| Severity | CRITICAL |
| Dimension | Infrastructure |
| Location | `docker-compose.prod.yml:56-60` |
| Evidence | `ssl=on` with cert paths pointing to the data volume, but no volume mount, init script, or documentation for cert generation. |
| Risk | PostgreSQL will fail to start in production. Blocks all production deployments. |
| Remediation | Add Docker secret or mounted volume for TLS certs. Add an init script that generates self-signed certs if none exist. Document the provisioning step. |
| Confidence | high |

### COMP-001 · Audit log anonymization queries missing tenant isolation
| Field | Value |
|---|---|
| Severity | CRITICAL |
| Dimension | Compliance |
| Location | `internal/adapter/postgres/store_audit_log.go:80,93-95` |
| Evidence | `AnonymizeAuditLogForUser` uses `WHERE admin_id = $1` with no `AND tenant_id`. `AnonymizeExpiredIPAddresses` has no tenant scope at all. |
| Risk | GDPR deletion for Tenant A could anonymize Tenant B's audit entries if admin IDs overlap. Violates tenant data boundaries. |
| Remediation | Add `AND tenant_id = $N` to both queries, passing `tenantFromCtx(ctx)`. |
| Reference | GDPR Art. 5(1)(f), CLAUDE.md tenant isolation rule |
| Confidence | high |

### COMP-002 · No consent mechanism for external LLM data processing
| Field | Value |
|---|---|
| Severity | CRITICAL |
| Dimension | Compliance |
| Location | Absent — no `user_consents` table, no `LLMConsentDialog.tsx`, no `RequireLLMConsent` middleware |
| Evidence | Plans exist in `docs/plans/2026-03-26-wt10-gdpr-compliance-plan.md:69` but no implementation. User prompts and source code are sent to third-party LLM providers without explicit consent. |
| Risk | No lawful processing basis under GDPR Art. 6 for external data transfers. |
| Remediation | Implement planned `user_consents` table, `RequireLLMConsent` middleware, and `LLMConsentDialog.tsx`. Block external LLM calls until consent is recorded. |
| Reference | GDPR Art. 6(1)(a), Art. 7, Art. 44-49 |
| Confidence | high |

### QUAL-001 · Python workers have near-zero unit test coverage
| Field | Value |
|---|---|
| Severity | CRITICAL |
| Dimension | Code Quality |
| Location | `workers/codeforge/` (entire directory) |
| Evidence | 1 test file out of 164 Python source files. Core modules like `agent_loop.py` (1025 LOC), `llm.py` (964 LOC), `retrieval.py` (980 LOC) have zero tests. |
| Risk | Regressions in the most critical AI execution layer are invisible until production. |
| Remediation | Prioritize unit tests for `agent_loop.py`, `llm.py`, `runtime.py`, `executor.py`. |
| Confidence | high |

---

## HIGH Findings (19)

### SEC-002 · Default dev credentials in committed config files
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Security |
| Location | `.devcontainer/devcontainer.json:18-21`, `.vscode/launch.json:28-31,50-53` |
| Evidence | `LITELLM_MASTER_KEY: "sk-codeforge-dev"`, `CODEFORGE_AUTH_ADMIN_PASS: "Changeme123"`, `CODEFORGE_INTERNAL_KEY: "codeforge-internal-dev"` — committed to git. |
| Risk | Predictable credentials in network-exposed dev environments. |
| Remediation | Replace with `${env:VAR}` references or `"REQUIRED"` placeholders. Add startup guard rejecting defaults in non-dev mode. |
| Reference | CWE-798, CWE-1188 |
| Confidence | high |

### SEC-003 · Default PostgreSQL password in docker-compose.yml
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Security |
| Location | `docker-compose.yml:82` |
| Evidence | `POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-codeforge_dev}` with port `5432:5432` exposed. |
| Risk | Any host-network process can connect to the database with known password. |
| Remediation | Remove host port mapping or use randomly-generated password. |
| Reference | CWE-1188, CWE-521 |
| Confidence | high |

### SEC-004 · NATS JetStream exposed without authentication in dev
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Security |
| Location | `docker-compose.yml:107-131` |
| Evidence | No `--user`/`--pass` flags, ports `4222:4222` and `8222:8222` exposed. |
| Risk | Any process can subscribe, publish, or manipulate JetStream — including injecting tool-call approvals. |
| Remediation | Add basic auth or remove host port mappings. |
| Reference | CWE-306, CWE-284 |
| Confidence | high |

### QUAL-002 · Pervasive `any`/`map[string]any` in Go core (80+ occurrences)
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Code Quality |
| Location | `internal/adapter/lsp/client.go`, `internal/adapter/http/handlers_routing.go:69`, `internal/port/broadcast/broadcaster.go:9`, + 77 more |
| Evidence | `map[string]any{"textDocument": ...}`, `BroadcastEvent(ctx, eventType string, payload any)` |
| Risk | Bypasses compile-time type checking. Contradicts CLAUDE.md rule: "No `any`/`interface{}`". |
| Remediation | Define typed structs for LSP params, HTTP responses, and broadcaster payload. |
| Confidence | high |

### QUAL-003 · Swallowed errors in webhook review trigger
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Code Quality |
| Location | `internal/service/vcs_webhook.go:105,171,229` |
| Evidence | `_ = s.review.HandlePush(...)`, `_, _ = s.review.HandlePreMerge(...)` |
| Risk | Failed review triggers silently discarded — automated reviews may never run. |
| Remediation | Use `logBestEffort` pattern already established in codebase. |
| Confidence | high |

### QUAL-004 · 30+ Go packages lack any test coverage
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Code Quality |
| Location | `internal/adapter/email/`, `internal/adapter/execshell/`, all handler files for quarantine/review/channels/audit/GDPR |
| Evidence | 30+ packages with source but no `_test.go` files. |
| Risk | Handler bugs (wrong status codes, missing auth, bad serialization) undetected. |
| Remediation | Add table-driven tests starting with security-sensitive endpoints. |
| Confidence | high |

### ARCH-001 · All 48 services accept monolithic `database.Store` (ISP violation)
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Architecture |
| Location | `internal/service/*.go` (48 constructors); `internal/port/database/store.go:13` |
| Evidence | All `NewXxxService` constructors accept `database.Store` which embeds 31 sub-interfaces. |
| Risk | Every service can call any store method. Mocking requires implementing all 31 interfaces. |
| Remediation | Each constructor accepts only its relevant sub-interface(s). ADR-014 acknowledges this. |
| Confidence | high |

### ARCH-002 · 77 setter-injection methods create temporal coupling
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Architecture |
| Location | `internal/service/*.go` — 77 `SetYyy()` methods; `conversation.go:111-135` (20 setters) |
| Evidence | 14+ nil-guards in `conversation_agent.go` for partially-initialized services. |
| Risk | Services partially initialized after construction. Silent no-ops or panics. |
| Remediation | Use functional options or builder pattern. Inject all required deps through constructor. |
| Confidence | high |

### ARCH-003 · `Handlers` struct is a god object (80+ fields, 262 routes)
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Architecture |
| Location | `internal/adapter/http/handlers.go:19`, `routes.go` (799 LOC) |
| Evidence | 75+ fields giving every endpoint access to every service. |
| Risk | No structural enforcement of least-privilege at handler level. |
| Remediation | Extend existing pattern (ProjectHandlers, AgentHandlers) to all domains. |
| Confidence | high |

### ARCH-004 · `AgentLoopExecutor` is a 677-LOC god class
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Architecture |
| Location | `workers/codeforge/agent_loop.py:149` |
| Evidence | Single class handles LLM iteration, tool filtering, stall detection, experience caching, model fallback, output validation, workspace snapshots, trajectory publishing, tool execution, and routing. |
| Risk | Impossible to test individual concerns in isolation. |
| Remediation | Extract `ToolExecutor`, `StallDetector`, `ModelFallbackStrategy`, `TrajectoryPublisher`, `OutputValidator`. |
| Confidence | high |

### ARCH-005 · Roadmap/Feature/Milestone CRUD missing `RequireRole` authorization
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Architecture |
| Location | `internal/adapter/http/routes.go:437-462` |
| Evidence | POST/PUT/DELETE for roadmaps, milestones, features have no `RequireRole` middleware. |
| Risk | Any authenticated user (including `viewer`) can create/update/delete roadmap data. |
| Remediation | Add `RequireRole(admin, editor)` to all mutable roadmap endpoints. |
| Confidence | high |

### INFRA-003 · HTTP clients without timeouts — unbounded connections
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Infrastructure |
| Location | `internal/adapter/slack/notifier.go:27`, `discord/notifier.go:27`, `gitlab/provider.go:30`, `gitea/provider.go:42`, `plane/provider.go:44`, `litellm/client.go:450` |
| Evidence | `http.DefaultClient` (timeout=0) and bare `&http.Client{}` across 7+ adapters. |
| Risk | Outbound HTTP calls hang indefinitely → goroutine exhaustion → core unresponsive. |
| Remediation | Set explicit `Timeout` on all HTTP clients. |
| Confidence | high |

### INFRA-004 · Multiple list queries missing LIMIT clauses
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Infrastructure |
| Location | `store_user.go:101`, `store_mcp.go:57`, `store_review.go:49,160`, `store_skill.go:62`, `store_experience.go:52`, `store_memory.go:41`, + 6 more |
| Evidence | `SELECT ... FROM users WHERE tenant_id = $1 ORDER BY created_at` (no LIMIT) |
| Risk | Unbounded result sets → OOM under data growth. |
| Remediation | Add `LIMIT $N` consistently. Codebase already has `DefaultListLimit` in other queries. |
| Confidence | high |

### INFRA-005 · No automated PostgreSQL backup scheduling in production
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Infrastructure |
| Location | `scripts/backup-postgres.sh` (exists but not scheduled) |
| Evidence | Script exists with encryption and retention, but no cron, no Docker sidecar, no CI schedule. WAL archives go to local Docker volumes. |
| Risk | Data loss if PostgreSQL volume corrupts or host fails. |
| Remediation | Add backup sidecar to prod compose. Configure off-host destination. Test restore procedure. |
| Confidence | high |

### COMP-003 · OpenAPI spec covers only 11% of routes
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Compliance |
| Location | `docs/api/openapi.yaml` (29 paths) vs `routes.go` (262 routes) |
| Evidence | Coverage: ~11%. Missing: benchmarks, A2A, MCP, channels, modes, microagents, routing, prompts, plans, goals, canvas. |
| Risk | Blocks security audits, pen testing, and third-party integrations. |
| Remediation | Add all public endpoints. Prioritize write endpoints (POST/PUT/DELETE). |
| Reference | SOC 2 CC7.1, ISO 27001 A.14.2.5 |
| Confidence | high |

### COMP-004 · Default config ships with SSL disabled for PostgreSQL
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Compliance |
| Location | `codeforge.yaml:10` |
| Evidence | `sslmode=disable` in checked-in config. Example config correctly uses `sslmode=prefer`. |
| Risk | Database traffic unencrypted by default for new installations. |
| Remediation | Change to `sslmode=prefer`. |
| Reference | GDPR Art. 32, CIS PostgreSQL Benchmark 8.1 |
| Confidence | high |

### COMP-005 · LiteLLM master key hardcoded in committed config files
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Compliance |
| Location | `codeforge.yaml:22`, `.env:32`, `docker-compose.yml:181` |
| Evidence | `master_key: "sk-codeforge-dev"` in three committed files. |
| Risk | Known default key allows unauthorized LiteLLM access if not changed. |
| Remediation | Remove hardcoded key. Use env var with no default. Reject `sk-codeforge-dev` outside dev mode. |
| Reference | CWE-798, OWASP A07:2021 |
| Confidence | high |

### COMP-006 · No data-at-rest encryption for conversation content
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Compliance |
| Location | `internal/adapter/postgres/store_conversation.go` |
| Evidence | Messages stored as plaintext. VCS tokens and LLM keys use `crypto.Encrypt`, but conversation content does not. |
| Risk | Database compromise exposes all conversation history (may contain PII, credentials). |
| Remediation | Implement TDE or application-level encryption for `messages.content` using existing `crypto.Encrypt` pattern. |
| Reference | GDPR Art. 32(1)(a), SOC 2 CC6.1 |
| Confidence | high |

### COMP-007 · Privacy policy missing DPO contact, retention periods, subprocessor list
| Field | Value |
|---|---|
| Severity | HIGH |
| Dimension | Compliance |
| Location | `frontend/src/features/legal/PrivacyPolicy.tsx`, `docs/privacy-policy.md` |
| Evidence | Template with placeholder text. Missing: DPO contact, specific retention periods, subprocessor list (OpenAI, Anthropic), legal basis per processing activity. |
| Risk | Violates GDPR Art. 13/14 information requirements. |
| Remediation | Complete privacy policy with all mandatory elements. Link to actual GDPR endpoints. |
| Confidence | high |

---

## MEDIUM Findings (37)

| ID | Title | Dimension | Location |
|----|-------|-----------|----------|
| SEC-005 | Potential command injection via autoagent testFile | Security | `internal/service/autoagent.go:421` |
| SEC-006 | innerHTML usage in Markdown renderer (sanitized but fragile) | Security | `frontend/src/features/project/Markdown.tsx:148` |
| SEC-007 | Default LiteLLM master key fallback in docker-compose | Security | `docker-compose.yml:181` |
| SEC-011 | Default `CODEFORGE_INTERNAL_KEY` in `.env` | Security | `.env:20` |
| QUAL-005 | DRY: GitHub/GitLab push handlers near-identical | Code Quality | `internal/service/vcs_webhook.go:43-176` |
| QUAL-006 | DRY: 18 repetitive SetupStep append blocks | Code Quality | `internal/service/project.go:545-714` |
| QUAL-007 | DRY: duplicated `progressTools` map definition | Code Quality | `internal/domain/run/stall.go:6` + `conversation_agent.go:220` |
| QUAL-008 | Missing type annotations on Python public method | Code Quality | `workers/codeforge/agent_loop.py:821` |
| QUAL-009 | 113 `except Exception` without specific types in Python | Code Quality | 52 files in `workers/codeforge/` |
| QUAL-010 | 20+ empty catch blocks in TypeScript frontend | Code Quality | `VCSSection.tsx`, `App.tsx`, `core.ts`, etc. |
| QUAL-011 | Complexity: `agent_loop.py` run() method 476 LOC | Code Quality | `workers/codeforge/agent_loop.py:350-826` |
| QUAL-012 | Complexity: `RuntimeService.StartRun()` 148 LOC | Code Quality | `internal/service/runtime.go` |
| QUAL-013 | `RuntimeService` god struct with 17 fields, 7 `sync.Map` | Code Quality | `internal/service/runtime.go:31-59` |
| QUAL-014 | DRY: tool call status broadcast pattern repeated 3x | Code Quality | `internal/service/runtime_execution.go:118-387` |
| QUAL-015 | `map[string]any` ad-hoc response types in HTTP handlers | Code Quality | `handlers_routing.go`, `handlers_llm.go`, + 5 more |
| ARCH-006 | Handler contains presentation logic (GetPlanGraph) | Architecture | `handlers_orchestration.go:127-197` |
| ARCH-007 | Handler contains CSV rendering + field-level update logic | Architecture | `handlers_benchmark.go:176-232` |
| ARCH-008 | Cross-adapter import: `copilot` imports `litellm` | Architecture | `internal/adapter/copilot/register.go:8` |
| ARCH-009 | Cross-adapter import: `http` imports `ws` | Architecture | `internal/adapter/http/handlers.go:4` |
| ARCH-010 | `BuiltinModes()` is a 318-LOC data function | Architecture | `internal/domain/mode/presets.go:8` |
| ARCH-011 | `frontend/src/api/types.ts` has 100+ unused exported types | Architecture | `frontend/src/api/types.ts` (2398 LOC) |
| ARCH-012 | Frontend components mix data fetching with rendering (800+ LOC) | Architecture | `FilePanel.tsx`, `A2APage.tsx`, `ProjectDetailPage.tsx` |
| ARCH-013 | `SetupProject` is a 171-LOC orchestration function | Architecture | `internal/service/project.go:545-715` |
| ARCH-014 | `ConversationHandlerMixin` (450 LOC) mixes concerns | Architecture | `workers/codeforge/consumer/_conversation.py:209` |
| INFRA-006 | NATS JetStream stream missing MaxMsgs/MaxConsumers limits | Infrastructure | `internal/adapter/nats/nats.go:74-84` |
| INFRA-007 | Dev compose exposes NATS monitoring port (8222) to host | Infrastructure | `docker-compose.yml:118` |
| INFRA-008 | Dev compose NATS has no authentication | Infrastructure | `docker-compose.yml:119` |
| INFRA-009 | Dev compose missing CPU limits on all services | Infrastructure | `docker-compose.yml` (all services) |
| INFRA-010 | CI NATS services lack JetStream — integration tests incomplete | Infrastructure | `.github/workflows/ci.yml:38-49` |
| INFRA-011 | No DLQ consumer or alerting for dead-lettered messages | Infrastructure | `internal/adapter/nats/nats.go:324-356` |
| INFRA-012 | WAL archives co-located with data in dev compose | Infrastructure | `docker-compose.yml:94` |
| INFRA-013 | Login endpoint missing audit logging | Infrastructure | `internal/adapter/http/routes.go:596-607` |
| COMP-008 | No skip navigation link for keyboard users | Compliance | `frontend/src/App.tsx` |
| COMP-009 | 390 onClick vs 37 onKeyDown — keyboard navigation gap | Compliance | `frontend/src/` (120 files) |
| COMP-010 | No data classification schema or inventory | Compliance | Absent |
| COMP-011 | Audit log stores PII without time-bounded auto-anonymization | Compliance | `internal/middleware/audit.go:34-35` |
| COMP-012 | AGPL Section 5(d) incomplete legal notices | Compliance | `frontend/src/App.tsx:311` |

---

## LOW Findings (12)

| ID | Title | Dimension |
|----|-------|-----------|
| SEC-008 | WebSocket token-in-URL accepted alongside ticket-based auth | Security |
| SEC-009 | Missing RBAC on several read endpoints | Security |
| SEC-010 | `sslmode=disable` in CI pipeline PostgreSQL DSN | Security |
| QUAL-016 | `fetchJSON` uses `any` for destination parameter | Code Quality |
| QUAL-017 | Python `Any` used in tool interface signatures | Code Quality |
| QUAL-018 | Unchecked git process exit codes in rollout functions | Code Quality |
| QUAL-019 | TODO/FIXME annotations indicating incomplete work | Code Quality |
| ARCH-015 | `context_optimizer.go` (846 LOC) aggregates 6+ sources without boundaries | Architecture |
| INFRA-014 | Traefik security headers missing CSP and Permissions-Policy | Infrastructure |
| INFRA-015 | Worker healthcheck only validates import, not runtime health | Infrastructure |
| COMP-013 | Default admin credentials in README | Compliance |
| COMP-014 | deepeval dependency sends telemetry to external servers | Compliance |

---

## INFORMATIONAL Findings (3)

| ID | Title | Dimension |
|----|-------|-----------|
| SEC-012 | Dynamic SQL construction — safe but brittle pattern | Security |
| QUAL-020 | `project.go` at 945 LOC approaching complexity threshold | Code Quality |
| COMP-015 | CHANGELOG not yet versioned — all under [Unreleased] | Compliance |

---

## Top 3 Priorities

These 3 actions, if executed first, would eliminate the most risk:

1. **Rotate secrets + harden dev defaults (SEC-001, SEC-002, SEC-003, SEC-004, COMP-005)**
   Rotate all live API keys in `.env`. Replace hardcoded dev credentials with env var references. Add NATS auth to dev compose. This closes the entire "known credentials" attack surface across security and compliance dimensions.

2. **Fix blue-green deployment blockers (INFRA-001, INFRA-002)**
   Fix the Traefik port mismatch (80→8080) and provision PostgreSQL SSL certs. Without these, production deployment via blue-green is impossible. Two-line config fix + one init script.

3. **Add Python worker tests + GDPR consent mechanism (QUAL-001, COMP-002)**
   The Python AI workers are the most critical and least tested layer. Adding unit tests for `agent_loop.py`, `llm.py`, and `runtime.py` prevents silent regressions. Implementing the planned GDPR consent mechanism (`user_consents` table + middleware + dialog) closes the most impactful compliance gap.

---

## Positive Findings

The audit identified strong engineering practices across the codebase:

### Security
- **SQL injection prevention** — all queries use parameterized `$N` placeholders via pgx/psycopg3
- **SSRF protection** — `SafeTransport()` blocks private/reserved IPs on all user-facing HTTP clients
- **Path traversal protection** — both Go (`resolveProjectPath`) and Python (`resolve_safe_path`) properly guard
- **Password security** — bcrypt (cost 12), account lockout (5 attempts/15 min), complexity enforcement
- **JWT implementation** — auto-generated secrets, entropy validation, token revocation table
- **Security headers** — full set (CSP, HSTS, X-Frame-Options: DENY, X-Content-Type-Options: nosniff)
- **Rate limiting** — global + stricter auth-specific limiter
- **Tenant isolation** — consistent `AND tenant_id = $N` across all store queries
- **Error handling** — internal errors return generic messages; actual errors logged server-side only
- **Production Docker hardening** — cap_drop ALL, no-new-privileges, read_only, network segregation
- **Webhook security** — HMAC-SHA256 verification (GitHub), token verification (GitLab)

### Architecture
- **Domain purity** — zero hexagonal violations in `internal/domain/`
- **Port purity** — `internal/port/` imports only `internal/domain/` types
- **Services never import adapters** — clean dependency direction
- **Consumer-defined interfaces** — `conversation_deps.go`, `runtime_deps.go` show good narrow interfaces
- **ISP-ready store decomposition** — 31 sub-interfaces already defined, awaiting wiring
- **No TypeScript `any`** — zero occurrences across the entire frontend
- **Well-structured `logBestEffort`** pattern for non-fatal store errors (47 occurrences)

### Compliance
- **GDPR self-service** — `/me/export` and `/me/data` endpoints implemented
- **Data retention service** — configurable batched cleanup citing GDPR Art. 5(1)(e)
- **License compatibility** — all dependencies use permissive licenses (MIT, BSD, Apache-2.0)
- **CI security** — all GitHub Actions pinned to full commit SHAs
- **Breach notification procedure** — document exists in `docs/security/`
- **Focus-visible ring styles** — applied to buttons, inputs, and interactive elements

---

## Methodology

- **Phase 1 (Discovery):** Automated classification of input types via file pattern detection
- **Phase 2 (Audit):** 5 parallel specialist agents, each scanning the full codebase for their dimension's checklist
- **Phase 3 (Report):** Cross-agent deduplication, severity calibration, and weighted risk scoring
- **Exclusions:** `.worktrees/`, `node_modules/`, `.venv/`, `.git/` directories

---

## Comparison with Prior Audit (2026-03-26)

| Metric | 2026-03-26 | 2026-03-27 | Delta |
|--------|------------|------------|-------|
| Scope | ~1,516 files (code+SQL+config) | ~2,074 files (full stack) | +37% broader |
| Total Findings | 45 | 77 | +32 (wider scope) |
| Critical | 3 | 6 | +3 new (INFRA-001/002, COMP-001/002) |
| Risk Score | 31/100 | 52/100 | +21 (infra + compliance coverage) |

Key differences: This audit added full Docker/CI/CD infrastructure analysis and deep GDPR compliance checking, which the prior audit excluded. The prior audit's CRITICAL findings (JWT secret, NATS duplicate subscription, LiteLLM key) remain relevant and are captured as HIGH in this report's overlap areas.
