# CodeForge Universal Audit Report

**Date:** 2026-03-26
**Version:** v0.8.0 (branch: staging, commit 57335a28)
**Scope:** Full codebase — Go Core (~135K LOC), Python Workers (~66K LOC), TypeScript Frontend (~72K LOC), 88 SQL migrations, Docker infrastructure, CI/CD
**Auditor:** Claude Opus 4.6 — 5 parallel dimension agents
**Mode:** READ-ONLY

---

## Audit Summary

| Metric               | Value                                     |
|-----------------------|-------------------------------------------|
| Target                | /workspaces/CodeForge (full monorepo)     |
| Input Type            | Mixed / Monorepo (App + Infra + DB + CI)  |
| Files Analyzed        | ~1,570 source files                       |
| Total Findings        | 95                                        |
| Critical              | 5                                         |
| High                  | 23                                        |
| Medium                | 34                                        |
| Low                   | 21                                        |
| Informational         | 12                                        |
| **Overall Risk Score**| **60 / 100**                              |

---

## Risk Heatmap

| Dimension          | Weight | Score /100 | Top Issue                                          |
|--------------------|--------|------------|-----------------------------------------------------|
| Security           | 30%    | 65         | Live API keys on disk (F-SEC-001)                   |
| Code Quality       | 25%    | 55         | 5-way backend executor DRY violation (F-QUA-002)    |
| Architecture       | 20%    | 50         | 303-method god interface (F-ARC-001)                |
| Infrastructure     | 15%    | 72         | No NATS retention limits (F-INF-003)                |
| Compliance         | 10%    | 58         | Incomplete GDPR data export (F-COM-004)             |
| **Weighted Total** |        | **60**     | **Infrastructure is the weakest dimension**         |

---

## Top 3 Priorities

1. **Rotate all API keys immediately** (F-SEC-001 / F-INF-001) — Live keys for Anthropic, Google, Groq, Mistral, OpenRouter, GitHub, HuggingFace are in plaintext `.env`. Rotate now regardless of whether they were ever committed to git.

2. **Add NATS JetStream retention limits** (F-INF-003) — No MaxAge, MaxBytes, or MaxMsgSize. Unbounded stream growth will eventually cause an outage. A single malformed large message could exhaust container memory.

3. **Fix blue-green TLS entrypoints** (F-INF-002) — All 4 Traefik routers in the blue-green overlay bind to HTTP port 80, bypassing TLS. Production traffic (including JWTs, API keys) would transit in cleartext.

---

## CRITICAL Findings (5)

---

> **[F-SEC-001] Live API Keys on Disk**
> | Field          | Value                                        |
> |----------------|----------------------------------------------|
> | Severity       | CRITICAL                                      |
> | Dimension      | Security                                      |
> | Location       | `.env:36-50`, `data/.env`                     |
> | Evidence       | 10+ real API keys (Anthropic, Google, Groq, Mistral, OpenRouter, GitHub, HuggingFace, etc.) in plaintext |
> | Risk           | Key leakage via volume mount, backup, or accidental `git add -f`. Financial abuse, data exfiltration |
> | Remediation    | Rotate ALL keys immediately. Use secrets manager. Add `detect-secrets` pre-commit hook |
> | Reference      | CWE-798, CWE-312, OWASP A07:2021             |
> | Confidence     | high                                          |

---

> **[F-INF-001] Live API Keys in `.env` (duplicate of F-SEC-001)**
> Same finding from Infrastructure perspective — confirmed from both agents.

---

> **[F-INF-002] Blue-Green Routers Use HTTP-only (No TLS)**
> | Field          | Value                                        |
> |----------------|----------------------------------------------|
> | Severity       | CRITICAL                                      |
> | Dimension      | Infrastructure                                |
> | Location       | `docker-compose.blue-green.yml:28,39,50,61`  |
> | Evidence       | `traefik.http.routers.*.entrypoints=web` (port 80, no TLS) on all 4 routers |
> | Risk           | All production traffic in cleartext. MITM trivially possible |
> | Remediation    | Change entrypoints to `websecure`. Add `tls.certresolver=letsencrypt` |
> | Reference      | CIS Docker 5.8, OWASP Transport Layer Protection |
> | Confidence     | high                                          |

---

> **[F-INF-003] NATS JetStream Has No Retention Limits**
> | Field          | Value                                        |
> |----------------|----------------------------------------------|
> | Severity       | CRITICAL                                      |
> | Dimension      | Infrastructure                                |
> | Location       | `internal/adapter/nats/nats.go:74-78`        |
> | Evidence       | `StreamConfig` has no `Retention`, `MaxAge`, `MaxBytes`, or `MaxMsgSize` |
> | Risk           | Unbounded stream growth fills disk/memory. Single oversized message can crash NATS |
> | Remediation    | Add `MaxAge: 7d`, `MaxBytes: 1GB`, `MaxMsgSize: 1MB` |
> | Reference      | NATS JetStream Configuration Best Practices   |
> | Confidence     | high                                          |

---

> **[F-ARC-001] God Interface — database.Store Has 303 Methods**
> | Field          | Value                                        |
> |----------------|----------------------------------------------|
> | Severity       | CRITICAL                                      |
> | Dimension      | Architecture                                  |
> | Location       | `internal/port/database/store.go:46`          |
> | Evidence       | Single interface with 303 methods importing 33 domain packages |
> | Risk           | ISP violation. Mocking impossible. Every new entity modifies this file. Merge conflicts guaranteed |
> | Remediation    | Split into role interfaces (`ProjectStore`, `ConversationStore`, etc.) with 5-15 methods each |
> | Reference      | SOLID - Interface Segregation Principle        |
> | Confidence     | high                                          |

---

## HIGH Findings (23)

### Security (4 HIGH)

> **[F-SEC-002] Weak Default JWT Secret**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `internal/config/config.go:572` |
> | Evidence | `JWTSecret: "codeforge-dev-jwt-secret-change-in-production"` — only blocked in `production` AppEnv, not `staging` |
> | Risk | JWT forgery in any non-production deployment |
> | Remediation | Require strong random secret whenever `auth.enabled=true`, regardless of AppEnv |
> | Reference | CWE-1188, OWASP A07:2021 |
> | Confidence | high |

> **[F-SEC-003] PostgreSQL SSL Disabled by Default**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `internal/config/config.go:437` |
> | Evidence | `sslmode=disable` in default DSN |
> | Risk | Cleartext database traffic in any deployment using defaults |
> | Remediation | Default to `sslmode=require` |
> | Reference | CWE-319 |
> | Confidence | high |

> **[F-SEC-004] NATS Unauthenticated in Development**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `docker-compose.yml:119` |
> | Evidence | No `--user`/`--pass` flags, ports 4222+8222 exposed |
> | Risk | Arbitrary NATS publish/subscribe from any local process |
> | Remediation | Add auth to dev compose, bind monitoring to 127.0.0.1 |
> | Reference | CWE-306 |
> | Confidence | high |

> **[F-SEC-005] Default Admin Password in Example Config**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `codeforge.example.yaml:105` |
> | Evidence | `default_admin_pass: "changeme123"` — only warns, does not reject |
> | Risk | Known default credential in non-production deployments |
> | Remediation | Refuse startup with default password when auth is enabled |
> | Reference | CWE-1188, CWE-521 |
> | Confidence | high |

### Code Quality (5 HIGH)

> **[F-QUA-001] SendMessageAgentic vs SendMessageAgenticWithMode — 80% Duplicated**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `internal/service/conversation_agent.go:386-541` + `:570-709` |
> | Evidence | Two 150-line methods sharing ~80% identical logic (fetch conv, fetch project, resolve model, build payload, publish) |
> | Risk | Bug fixes must be applied twice. Divergence already started (dedup vs non-dedup, provider key handling) |
> | Remediation | Extract shared `dispatchAgenticRun()` method |
> | Confidence | high |

> **[F-QUA-002] 5-Way Backend Executor Duplication (Python)**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `workers/codeforge/backends/{aider,goose,opencode,sweagent,plandex}.py` |
> | Evidence | 5 files × ~120 lines with identical pattern: init, info, check_available, execute, streaming, timeout |
> | Risk | 5-way coordinated changes required for any bug fix or feature |
> | Remediation | Extract `SubprocessBackendExecutor` base class. Each backend becomes ~20 lines |
> | Confidence | high |

> **[F-QUA-003] Missing Type Annotations on Critical Python Methods**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `workers/codeforge/agent_loop.py:510,576,664` |
> | Evidence | `_do_llm_iteration`, `_process_llm_response`, `_execute_tool_call` — no parameter or return type annotations |
> | Risk | No static analysis protection on the most critical agent loop methods |
> | Remediation | Add full type annotations for all parameters and return types |
> | Confidence | high |

> **[F-QUA-004] Pervasive `Any` Usage in Python (26 files)**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | 26 files across `workers/codeforge/` |
> | Evidence | `dict[str, Any]` in tool interfaces, backend interfaces, MCP workbench, models. Violates CLAUDE.md strict type safety rule |
> | Risk | Loss of type safety at API boundaries |
> | Remediation | Replace with typed alternatives (`TypedDict`, unions, specific interfaces) |
> | Confidence | high |

> **[F-QUA-015] Core Agent Loop Has No Unit Tests**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `workers/codeforge/agent_loop.py` (993 lines) |
> | Evidence | Tests only cover backward-compat aliases, NOT `run()`, `_do_llm_iteration`, `_execute_tool_call` |
> | Risk | Zero regression safety for the heart of the AI worker pipeline |
> | Remediation | Add unit tests with mock LiteLLMClient for all primary execution paths |
> | Confidence | high |

### Architecture (3 HIGH)

> **[F-ARC-002] God Struct — Handlers Has 77 Fields**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `internal/adapter/http/handlers.go:22` |
> | Evidence | 77 fields, ~45 distinct service types |
> | Risk | Impossible to instantiate for testing. Every feature adds a field |
> | Remediation | Complete decomposition into focused handler groups (pattern already started) |
> | Confidence | high |

> **[F-ARC-003] God Function — main.go:run() is 1046 Lines**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `cmd/codeforge/main.go:89` |
> | Evidence | Infrastructure setup, service instantiation, 70+ setter calls, subscriber registration, routing, shutdown — all in one function |
> | Risk | Extremely high cognitive load. Missing setter causes runtime nil-pointer panic |
> | Remediation | Extract into phases: `setupInfra()`, `wireServices()`, `startSubscribers()`, `mountRoutes()` |
> | Confidence | high |

> **[F-ARC-004] Service-to-Service Coupling via Concrete Types**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `internal/service/runtime.go:31-59`, `conversation.go:62-92` |
> | Evidence | RuntimeService holds 10 concrete `*XxxService` pointers via 17 setter methods. ConversationService holds 11 |
> | Risk | Cannot test services in isolation. Nil dependencies cause runtime panics |
> | Remediation | Define consumer-side interfaces for dependencies. Use constructor injection |
> | Confidence | high |

### Infrastructure (6 HIGH)

> **[F-INF-004] NATS Unauthenticated in Dev Compose** — same root cause as F-SEC-004
>
> **[F-INF-005] GitHub Actions Not SHA-Pinned**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `.github/workflows/ci.yml`, `docker-build.yml` |
> | Evidence | All 12 third-party actions use mutable tags (`@v4`, `@v5`) instead of commit SHAs |
> | Risk | Supply chain attack via tag reassignment (proven attack vector) |
> | Remediation | Pin all actions to full commit SHA |
> | Reference | OpenSSF Scorecard |
> | Confidence | high |

> **[F-INF-006] Docker Images Use Floating Tags**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `docker-compose.yml:24,135,170` |
> | Evidence | `mcp/playwright:latest`, `docs-mcp-server:latest`, `litellm:main-stable` |
> | Risk | Unpinned images can change behavior silently |
> | Remediation | Pin to specific version tags or SHA digests |
> | Confidence | high |

> **[F-INF-007] Worker Dockerfile Copies Entire Build Directory**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `Dockerfile.worker:42-43` |
> | Evidence | `COPY --from=build /app /app` after already copying `.venv` — redundant, pulls build artifacts |
> | Remediation | Copy only `VERSION`, `workers/` explicitly |
> | Confidence | high |

> **[F-INF-008] No CPU Limits in Dev Compose**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `docker-compose.yml` (all services) |
> | Evidence | Memory limits set but no `cpus` limits on any service |
> | Risk | Runaway process starves all containers and host |
> | Remediation | Add `cpus` limits matching production compose |
> | Confidence | high |

> **[F-INF-009] No Container Image Scanning in CI/CD**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `.github/workflows/docker-build.yml` |
> | Evidence | Builds and pushes 3 images with no scanning step. Source-level vuln checks exist but no image scanning |
> | Risk | OS-level CVEs in base images deployed undetected |
> | Remediation | Add `trivy-action` or `grype-action` after build |
> | Confidence | high |

### Compliance (5 HIGH)

> **[F-COM-001] OpenAPI Declares "Proprietary" Instead of AGPL-3.0**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `docs/api/openapi.yaml:9-10` |
> | Evidence | `license: name: Proprietary` — but LICENSE file is AGPL-3.0-or-later |
> | Risk | License misrepresentation, AGPL Section 5(d) non-compliance |
> | Remediation | Change to `name: AGPL-3.0-or-later` |
> | Confidence | high |

> **[F-COM-002] OpenAPI Covers ~10 of ~301 Endpoints**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `docs/api/openapi.yaml` |
> | Evidence | Self-declared stub. ~3% endpoint coverage |
> | Risk | SOC 2 CC6.1 documentation gap. Integration/security reviews impossible |
> | Remediation | Generate complete spec from routes.go |
> | Confidence | high |

> **[F-COM-003] Password Reset Tokens Lack Tenant Isolation**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `internal/adapter/postgres/store_user.go:91-127` |
> | Evidence | 4 CRUD operations on `password_reset_tokens` have no `tenant_id` in WHERE clauses despite column existing |
> | Risk | Cross-tenant token validation theoretically possible |
> | Remediation | Add `tenant_id` to all password_reset_tokens queries |
> | Confidence | high |

> **[F-COM-004] GDPR Data Export Incomplete**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `internal/service/gdpr.go:31-56` |
> | Evidence | Export includes only User, APIKeys, LLMKeys. Missing: conversations, messages, cost records, feedback, memories |
> | Risk | GDPR Article 20 non-compliance (right to data portability) |
> | Remediation | Expand `UserDataExport` to include all personal data categories |
> | Confidence | high |

> **[F-COM-005] Data Retention Policy Not Enforced in Code**
> | Field | Value |
> |---|---|
> | Severity | HIGH |
> | Location | `docs/data-retention.md` vs codebase |
> | Evidence | Policy document exists but zero `retention` config, zero cleanup jobs for agent events (90d), conversations (1y), sessions (30d) |
> | Risk | GDPR Article 5(1)(e) storage limitation non-compliance |
> | Remediation | Implement `retention` config section and scheduled cleanup goroutine |
> | Confidence | high |

---

## MEDIUM Findings (34)

<details>
<summary>Security — 5 MEDIUM</summary>

| ID | Title | Location | Key Risk |
|---|---|---|---|
| F-SEC-006 | LIKE wildcard injection in GetProjectByRepoName | `store_project.go:43` | `%` matches all projects in tenant |
| F-SEC-007 | Initial setup endpoint race condition (TOCTOU) | `handlers_auth.go:311-347` | Concurrent setup could create two admins |
| F-SEC-008 | Modular bias in random password generation | `crypto/crypto.go:44` | First 8 charset chars over-represented by ~25% |
| F-SEC-009 | A2A CreateA2APushConfig missing tenant check | `store_a2a.go:249-260` | Cross-tenant push webhook attachment |
| F-SEC-010 | Hardcoded internal service key default | `.env:20` | `codeforge-internal-dev` grants admin access |

</details>

<details>
<summary>Code Quality — 8 MEDIUM</summary>

| ID | Title | Location | Key Risk |
|---|---|---|---|
| F-QUA-005 | Complexity hotspot: `_execute_tool_call` (130 LOC, 4 nesting levels) | `agent_loop.py:664-794` | Untestable individual paths |
| F-QUA-006 | Repeated trajectory event dict construction (3x) | `agent_loop.py:678,713,744` | Schema changes require 3-way sync |
| F-QUA-007 | Repeated `RunStatusEvent` broadcast pattern (8x) | `runtime_execution.go` + `runtime.go` + `runtime_lifecycle.go` | Missing fields in some broadcasts |
| F-QUA-008 | Repeated JSON score unmarshaling (3x) | `benchmark.go:519` + 2 more | Score parsing changes need 3 edits |
| F-QUA-009 | Swallowed error in autoagent.go (no logBestEffort) | `autoagent.go:217` | Stale auto-agent status in DB |
| F-QUA-010 | `map[string]any` in non-test Go code (9 locations) | `conversation_agent.go:162` + more | Silent type mismatches |
| F-QUA-011 | `AuthUserCtxKeyForTest` returns `any` | `auth.go:237` | Type safety violation in test helper |
| F-QUA-016 | 28 Go service files without unit tests | `internal/service/` | Missing tests for auth, GDPR, tenant, skill, replay |

</details>

<details>
<summary>Architecture — 6 MEDIUM</summary>

| ID | Title | Location | Key Risk |
|---|---|---|---|
| F-ARC-005 | Domain layer performs direct file I/O (4 packages) | `pipeline/loader.go`, `policy/loader.go`, `microagent/loader.go`, `project/scan.go` | Untestable without real filesystem |
| F-ARC-006 | Business logic in HTTP handler (autoIndex, ListRemoteBranches) | `handlers_project.go:161,401` | Orchestration logic untestable without HTTP |
| F-ARC-007 | Service layer makes direct HTTP calls (4 files) | `project.go:762`, `github_oauth.go`, `a2a.go` | Services coupled to HTTP transport |
| F-ARC-008 | NATS subjects duplicated across Python files (3 locations) | `runtime.py`, `prompt_mutator.py`, `prompt_optimizer.py` | Subject rename requires 3+ file updates |
| F-ARC-009 | RunCompletePayload missing tenant_id in Go | `schemas.go:106-118` vs `models.py:141-156` | Tenant context lost in run-completion processing |
| F-ARC-010 | RuntimeService.StartSubscribers — 223 LOC god function | `runtime.go:574-796` | 7 subscriber handlers in anonymous closures |

</details>

<details>
<summary>Infrastructure — 9 MEDIUM</summary>

| ID | Title | Location | Key Risk |
|---|---|---|---|
| F-INF-010 | `secrets/` not in `.dockerignore` | `.dockerignore` | Secrets baked into Docker image layers |
| F-INF-011 | WAL archives stored inside data volume (dev) | `docker-compose.yml:94` | Volume corruption loses both DB and archives |
| F-INF-012 | No automated backup schedule | Production compose | Backups depend on manual operator action |
| F-INF-013 | Prometheus not deployed (alert rules dead) | `configs/prometheus/alerts.yml` | No operational metrics or alerting |
| F-INF-014 | `sslmode=disable` as default PostgreSQL DSN | `config.go:437` | Cleartext DB traffic with default config |
| F-INF-015 | Nginx does not suppress server version header | `frontend/nginx.conf` | Exposes exact nginx version to attackers |
| F-INF-016 | HSTS header commented out in Nginx | `frontend/nginx.conf:17` | No browser-enforced HTTPS in direct-to-nginx deployments |
| F-INF-017 | PostgreSQL has no statement/connection logging | `docker-compose.yml:86-94` | No DB-level audit trail |
| F-INF-018 | Docker socket exposed to Traefik without proxy | `docker-compose.blue-green.yml:14` | Container env vars readable if Traefik compromised |

</details>

<details>
<summary>Compliance — 6 MEDIUM</summary>

| ID | Title | Location | Key Risk |
|---|---|---|---|
| F-COM-006 | No GDPR consent mechanism in frontend | `frontend/src/` (absent) | No lawful processing basis for external LLM calls |
| F-COM-007 | LLM key encryption uses SHA-256 (not KDF) | `internal/crypto/aes.go:15-18` | No salt, no iterations. Low-entropy secrets vulnerable |
| F-COM-008 | CreateToolMessages lacks explicit tenant check | `store_conversation.go:197-235` | Defense-in-depth gap (FK constraint provides some protection) |
| F-COM-009 | AGPL source availability notice missing from frontend | `frontend/src/App.tsx` | AGPL Section 13 non-compliance for network deployment |
| F-COM-010 | Audit log misses GDPR data export events | `routes.go:527` | No accountability trail for data export actions |
| F-COM-011 | Keyboard navigation: 36 keydown vs 384 click handlers | `frontend/src/` | WCAG 2.1.1 gaps in complex components |
| F-COM-012 | No SBOM or third-party license notice file | Project root | AGPL Section 5, enterprise procurement gap |

</details>

---

## LOW Findings (21)

<details>
<summary>All LOW findings</summary>

| ID | Dimension | Title | Location |
|---|---|---|---|
| F-SEC-011 | Security | WebSocket token in URL query parameter | `auth.go:98` |
| F-SEC-012 | Security | Auth disabled by default in example config | `codeforge.example.yaml:99` |
| F-SEC-013 | Security | Docker ports on 0.0.0.0 (all interfaces) | `docker-compose.yml` |
| F-QUA-012 | Quality | Private `_tools` access via `type: ignore` | `_conversation_skill_integration.py:21` |
| F-QUA-013 | Quality | 14 re-export aliases for backward compat | `agent_loop.py:72-85` |
| F-QUA-014 | Quality | `extractKeywords` allocates stop-word map per call | `context_optimizer.go:818` |
| F-QUA-017 | Quality | `_report_rate_info` uses untyped dict with 4 type-ignores | `llm.py:489-503` |
| F-QUA-018 | Quality | `conversation_agent.go` is 1110 LOC | `conversation_agent.go` |
| F-ARC-011 | Architecture | Domain `vcsaccount` imports `internal/crypto` | `vcsaccount/crypto.go:3` |
| F-ARC-012 | Architecture | Broadcaster port uses `any` for payload | `broadcast/broadcaster.go:9` |
| F-ARC-013 | Architecture | 18 service files import `internal/config` directly | `internal/service/` |
| F-ARC-014 | Architecture | Monolithic NATS schemas file (723 lines, 64 structs) | `schemas.go` |
| F-ARC-015 | Architecture | `MountRoutes` is 608 lines | `routes.go:67` |
| F-ARC-016 | Architecture | `SendMessageAgentic` is 156 lines with mixed concerns | `conversation_agent.go:386` |
| F-INF-019 | Infrastructure | `docker-build.yml` missing top-level `permissions` block | `.github/workflows/docker-build.yml` |
| F-INF-020 | Infrastructure | Worker healthcheck only validates module import | `Dockerfile.worker:51` |
| F-INF-021 | Infrastructure | NATS monitoring port (8222) on host in dev | `docker-compose.yml:118` |
| F-INF-022 | Infrastructure | No `pids_limit` on any container | `docker-compose.yml` |
| F-INF-023 | Infrastructure | Log redaction misses several token patterns | `logger/redact.go:10-15` |
| F-COM-013 | Compliance | Inconsistent `alt` attribute pattern on file icons | `FilePanel.tsx:704` |
| F-COM-014 | Compliance | CHANGELOG only covers v0.8.0 | `CHANGELOG.md` |

</details>

---

## INFO / Positive Findings (12)

<details>
<summary>All INFO findings</summary>

| ID | Dimension | Title |
|---|---|---|
| F-SEC-014 | Security | LiteLLM master key warns at runtime (good) |
| F-SEC-015 | Security | Subprocess execution governed by policy engine (by design) |
| F-INF-024 | Infrastructure | Pre-commit includes `detect-private-key` (good) |
| F-INF-025 | Infrastructure | Production compose uses Docker secrets + separate networks (good) |
| F-INF-026 | Infrastructure | Backup scripts well-implemented (need automation) |
| F-COM-015 | Compliance | Skip-to-content link present — WCAG 2.4.1 satisfied (good) |
| F-COM-016 | Compliance | Security headers comprehensive — CSP, HSTS, X-Frame-Options (good) |
| F-COM-017 | Compliance | PasswordHash excluded from JSON serialization (good) |
| F-COM-018 | Compliance | LLM keys encrypted at rest with AES-256-GCM (good) |
| F-COM-019 | Compliance | Log redaction handler active for secrets and PII (good) |
| F-COM-020 | Compliance | Tenant isolation consistently applied across 189 files (good) |

**Additional security positives noted by the Security agent:**
- All SQL uses parameterized queries — no string interpolation
- Markdown renderer escapes HTML before formatting (XSS prevention)
- `resolveProjectPath` uses `filepath.EvalSymlinks` + prefix check (path traversal prevention)
- `SafeTransport` blocks private IPs (SSRF prevention)
- bcrypt with cost 12 for password hashing
- JWT with audience/issuer/expiry/JTI revocation
- CORS wildcard rejected in non-development
- Account lockout after 5 failed attempts
- Dedicated auth rate limiter

</details>

---

## Remediation Roadmap (suggested priority order)

### Immediate (this week)
1. Rotate all exposed API keys (F-SEC-001)
2. Add NATS JetStream retention limits (F-INF-003)
3. Fix blue-green TLS entrypoints (F-INF-002)
4. Add `secrets/` to `.dockerignore` (F-INF-010)
5. Fix OpenAPI license field (F-COM-001)

### Short-term (next 2 weeks)
6. Enforce JWT secret strength in all environments (F-SEC-002)
7. Default PostgreSQL to `sslmode=require` (F-SEC-003)
8. Add NATS auth to dev compose (F-SEC-004 / F-INF-004)
9. Reject default admin password at startup (F-SEC-005)
10. SHA-pin GitHub Actions (F-INF-005)
11. Pin Docker image tags (F-INF-006)
12. Add container image scanning to CI (F-INF-009)
13. Add `tenant_id` to password reset token queries (F-COM-003)
14. Fix RunCompletePayload tenant_id mismatch (F-ARC-009)
15. Add tenant check to A2A push config (F-SEC-009)

### Medium-term (next month)
16. Split `database.Store` into role interfaces (F-ARC-001)
17. Decompose `Handlers` struct (F-ARC-002)
18. Extract `main.go:run()` into phases (F-ARC-003)
19. Add unit tests for agent loop (F-QUA-015)
20. Extract `SubprocessBackendExecutor` base class (F-QUA-002)
21. Complete GDPR data export (F-COM-004)
22. Implement data retention enforcement (F-COM-005)
23. Replace SHA-256 key derivation with HKDF (F-COM-007)
24. Deploy Prometheus + configure alerting (F-INF-013)

### Long-term (next quarter)
25. Generate complete OpenAPI spec (F-COM-002)
26. Add consent mechanism for external LLM processing (F-COM-006)
27. Replace `dict[str, Any]` with typed alternatives across Python (F-QUA-004)
28. Move domain file I/O behind `fs.FS` abstraction (F-ARC-005)
29. Extract service HTTP calls behind port interfaces (F-ARC-007)
30. Define consumer-side interfaces for service dependencies (F-ARC-004)
31. AGPL source notice in frontend (F-COM-009)
32. Generate SBOM (F-COM-012)
33. Improve keyboard navigation in complex components (F-COM-011)
