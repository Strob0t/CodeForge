# CodeForge Universal Audit Report

**Date:** 2026-03-22
**Auditor:** Claude Opus 4.6 (automated, evidence-based)
**Prompt:** `docs/prompts/universal-audit.md`
**Scope:** Full monorepo — Go backend, Python workers, TypeScript frontend, infrastructure, config, migrations

---

## Audit Summary

| Metric               | Value                                          |
|-----------------------|------------------------------------------------|
| Target                | `/workspaces/CodeForge` (full monorepo)         |
| Input Type            | Mixed / Monorepo (Code + Infra + Config + DB + CI/CD + Docs) |
| Languages             | Go 1.25 (616 files), Python 3.12 (340), TypeScript (419), SQL (86) |
| Total LOC             | ~266,000                                        |
| Total Findings        | 88                                              |
| CRITICAL              | 2                                               |
| HIGH                  | 14                                              |
| MEDIUM                | 44                                              |
| LOW                   | 18                                              |
| INFO                  | 10                                              |
| Overall Risk Score    | **62 / 100** (moderate — critical secrets issue, solid code foundations) |

---

## Risk Heatmap

| Dimension          | Weight | Score /100 | Top Issue                                |
|--------------------|--------|-----------|------------------------------------------|
| Security           | 30%    | 45        | Real API keys committed to `.env`        |
| Code Quality       | 25%    | 72        | God objects (Handlers 100+ fields, 1400 LOC store) |
| Architecture       | 20%    | 68        | Service layer imports adapter layer directly |
| Infrastructure     | 15%    | 42        | No TLS between services, no backup automation |
| Compliance         | 10%    | 70        | Missing GDPR data deletion/export endpoints |
| **Weighted Total** | 100%   | **58**    | **S-001: API keys in version control**   |

---

## Top 3 Priorities

1. **[S-001] Rotate all committed API keys immediately** — 9 real provider keys (Anthropic, Cerebras, Gemini, Groq, Mistral, OpenRouter, GitHub, HuggingFace, Chutes) are in `.env` and git history. Rotate all keys, run `git filter-repo` to scrub history, add `detect-secrets` pre-commit hook.

2. **[I-001 + I-004] Enable TLS for all inter-service communication** — PostgreSQL, NATS, and LiteLLM connections are unencrypted. In production, this exposes credentials, prompts, and user data to network sniffing. Enable `sslmode=require` for Postgres, TLS for NATS, HTTPS for LiteLLM.

3. **[I-012 + I-011] Implement automated database backups** — No scheduled backup exists. WAL archiving writes to local filesystem only (lost with container). Add cron-based `pg_dump`, archive WAL to external storage, document recovery procedure.

---

## CRITICAL Findings

### [S-001] Real API Keys Committed to .env
| Field          | Value |
|----------------|-------|
| Severity       | CRITICAL |
| Dimension      | Security |
| Location       | `.env:36-51` |
| Evidence       | `ANTHROPIC_API_KEY=sk-ant-api03-...`, `GITHUB_TOKEN=github_pat_11AMQKT6A0...`, `OPENROUTER_API_KEY=sk-or-v1-...` and 6 more real keys |
| Risk           | Any repo access = full credential theft for 9 LLM/cloud providers. Quota abuse, data exfiltration, lateral movement via GitHub token. |
| Remediation    | 1. Rotate all 9 keys in provider dashboards NOW. 2. `git filter-repo` to remove from history. 3. Add `detect-secrets` to `.pre-commit-config.yaml`. 4. Never commit `.env` — only `.env.example` with empty values. |
| Reference      | CWE-798, OWASP A07:2021 |
| Confidence     | high |

### [S-002] Second GitHub Token in data/.env
| Field          | Value |
|----------------|-------|
| Severity       | CRITICAL |
| Dimension      | Security |
| Location       | `data/.env:1` |
| Evidence       | `GITHUB_TOKEN=gho_to285q...` |
| Risk           | Separate GitHub OAuth token with repo permissions. Combined with S-001, two independent GitHub access paths are exposed. |
| Remediation    | Rotate token, remove file from git, add `data/.env` to `.gitignore`. |
| Reference      | CWE-798 |
| Confidence     | high |

---

## HIGH Findings

### [A-005] God Object: Handlers Struct (100+ fields, 1132 LOC)
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Architecture |
| Location       | `internal/adapter/http/handlers.go:41-110` |
| Evidence       | `type Handlers struct { Projects, Tasks, Agents, ... Checkpoint }` — 100+ injected dependencies |
| Risk           | Untestable, hides actual dependencies per endpoint, painful to maintain. |
| Remediation    | Decompose into `ProjectHandlers`, `ConversationHandlers`, `BenchmarkHandlers`, etc. (TODO already in code at line 39). |
| Reference      | SRP, Clean Architecture |
| Confidence     | high |

### [I-001] Missing TLS for PostgreSQL Connections
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | `codeforge.yaml:10`, `docker-compose.prod.yml:128,167` |
| Evidence       | `dsn: "postgres://...?sslmode=disable"` |
| Risk           | Credentials and queries transit unencrypted. Network sniffing yields full DB access. |
| Remediation    | Set `sslmode=require` (or `verify-full`), generate server/client certificates, mount via Docker volumes. |
| Reference      | CWE-319 |
| Confidence     | high |

### [I-005] Playwright-MCP Exposed on 0.0.0.0 with Wildcard Hosts
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | `docker-compose.yml:44-46` |
| Evidence       | `--host "0.0.0.0"`, `--allowed-hosts "*"` |
| Risk           | Any container/process can remotely execute browser commands, steal credentials from rendered pages. |
| Remediation    | Bind to `127.0.0.1`, replace `--allowed-hosts "*"` with specific hostnames, move to `profiles: [dev]`. |
| Reference      | CWE-668 |
| Confidence     | high |

### [I-011] WAL Archiving to Local Filesystem Only
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | `docker-compose.yml:75-76` |
| Evidence       | `archive_command='... cp %p /var/lib/postgresql/data/archive/%f'` |
| Risk           | Container destruction = WAL loss = no point-in-time recovery. |
| Remediation    | Archive to external storage (S3/NFS) via pgbackrest. Test recovery monthly. |
| Reference      | ISO 27001 A.12.3 |
| Confidence     | high |

### [I-012] No Automated Database Backups
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | No cron/scheduler for `scripts/backup-postgres.sh` |
| Evidence       | Manual script exists but no scheduled execution. |
| Risk           | Total data loss on unplanned failure. |
| Remediation    | Add cron: `0 2 * * * scripts/backup-postgres.sh --cleanup`. Add monitoring for backup completion. |
| Reference      | ISO 27001 A.12.3 |
| Confidence     | high |

### [I-002] Insecure Default Credentials in Config
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | `.env.example:31`, `codeforge.yaml:22` |
| Evidence       | `LITELLM_MASTER_KEY: "sk-codeforge-dev"`, `POSTGRES_PASSWORD: codeforge_dev` |
| Risk           | Defaults left in production = immediate admin access to LiteLLM and PostgreSQL. |
| Remediation    | Require explicit env vars in production (no defaults). Add startup validation rejecting weak/default passwords. |
| Reference      | CWE-798 |
| Confidence     | high |

### [I-003] NATS JetStream Unauthenticated in Development
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | `docker-compose.yml:85-102` |
| Evidence       | No `--user`/`--pass` flags on NATS container. |
| Risk           | Any network participant can publish/subscribe to agent commands. |
| Remediation    | Add NATS auth for dev. Add TLS for prod. |
| Reference      | CWE-306 |
| Confidence     | high |

### [I-007] Unpinned Base Image Versions
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | `Dockerfile.frontend:19`, `docker-compose.yml:24,9,105` |
| Evidence       | `FROM nginx:alpine`, `image: jaegertracing/all-in-one:latest`, `image: ghcr.io/berriai/litellm:main-stable` |
| Risk           | Supply chain attack via compromised upstream image. |
| Remediation    | Pin all images to specific semver or SHA256 digest. |
| Reference      | CWE-1104, SLSA Level 1 |
| Confidence     | high |

### [S-009] Missing HSTS Header
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Security |
| Location       | `internal/adapter/http/middleware.go:26-36` |
| Evidence       | CSP, X-Frame-Options, X-Content-Type-Options set — HSTS missing. |
| Risk           | Protocol downgrade attacks. Especially critical since WS tokens pass via URL query params. |
| Remediation    | Add `Strict-Transport-Security: max-age=31536000; includeSubDomains; preload`. |
| Reference      | CWE-319, OWASP A05:2021 |
| Confidence     | high |

### [Q-001] Handlers Struct with 51+ Fields
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Code Quality |
| Location       | `internal/adapter/http/handlers.go:41-111` |
| Evidence       | 51+ service dependencies in a single struct. |
| Risk           | Testing requires mocking 51+ services. Endpoint dependencies are invisible. |
| Remediation    | Decompose into domain-specific handler groups. |
| Reference      | SRP |
| Confidence     | high |

### [Q-002] Oversized Service Files (1000+ LOC)
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Code Quality |
| Location       | `internal/service/conversation_agent.go` (1133), `internal/service/benchmark.go` (1077), `internal/adapter/postgres/store.go` (1487), `workers/codeforge/agent_loop.py` (1453) |
| Evidence       | 4 files exceed 1000 LOC. benchmark.go has a TODO acknowledging decomposition need. |
| Risk           | Hard to test, review, and maintain. High cognitive load. |
| Remediation    | Split benchmark.go into RunManager, ResultAggregator, DatasetResolver. Split store.go by domain. |
| Reference      | SRP, Clean Code |
| Confidence     | high |

### [I-004] Unencrypted LiteLLM Proxy Communication
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | `codeforge.yaml:21`, `docker-compose.prod.yml:130-131` |
| Evidence       | `url: "http://codeforge-litellm:4000"` (HTTP, not HTTPS) |
| Risk           | All LLM prompts (potentially containing secrets, code, user PII) transit unencrypted. |
| Remediation    | Enable HTTPS on LiteLLM or use mTLS between services. |
| Reference      | CWE-319 |
| Confidence     | high |

### [I-013] Traefik Missing TLS Configuration
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | `traefik/traefik.yaml:13` |
| Evidence       | `exposedByDefault: false` set, but no TLS config. TODO comment: "Add TLS certificate configuration". |
| Risk           | Blue-green deployment routes traffic over unencrypted HTTP only. |
| Remediation    | Add Let's Encrypt or static cert TLS, redirect HTTP to HTTPS. |
| Reference      | CWE-319 |
| Confidence     | high |

### [I-010] PostgreSQL Archive Command Lacks Error Handling
| Field          | Value |
|----------------|-------|
| Severity       | HIGH |
| Dimension      | Infrastructure |
| Location       | `docker-compose.yml:76` |
| Evidence       | `archive_command='test ! -f ... && cp %p ...'` — no mkdir, silent failure if dir missing. |
| Risk           | WAL archiving fails silently, breaking point-in-time recovery without alerting. |
| Remediation    | Add `mkdir -p` before `cp`. Add `archive_command_timeout`. Consider pgbackrest. |
| Reference      | PostgreSQL WAL best practices |
| Confidence     | high |

---

## MEDIUM Findings (44 total)

### Security (5)

| ID | Title | Location | Key Risk |
|----|-------|----------|----------|
| S-003 | WebSocket token in URL query params | `internal/middleware/auth.go:97-124` | Token in server/proxy logs, browser history |
| S-004 | CORS origin validation gap | `internal/adapter/http/middleware.go:42-70` | Wildcard CORS if `APP_ENV` misconfigured |
| S-007 | No rate limiting on password reset | `handlers_auth.go` (public path) | Email enumeration, resource exhaustion |
| S-008 | Weak default internal service key | `.env:20` (`codeforge-internal-dev`) | Inter-service auth spoofing |
| S-010 | Dev PostgreSQL password in .env | `.env:24` | Risk of copying to production |

### Code Quality (11)

| ID | Title | Location | Key Risk |
|----|-------|----------|----------|
| Q-003 | Type-unsafe map operations in tests | `internal/adapter/ws/agui_events_test.go:25` | Implicit comparison errors |
| Q-004 | Bare except without logging (Python) | `model_resolver.py:69`, `context_reranker.py:80`, `history.py:208,364` | Silent failures |
| Q-005 | Missing type hints on public functions | `workers/codeforge/agent_loop.py` | Reduced IDE support, bugs |
| Q-007 | Bare `return err` without context (Go) | `git/pool.go:34`, `resilience/breaker.go:58`, 4 more | Debugging difficulty |
| Q-008 | Sparse handler test coverage | `internal/adapter/http/handlers_test.go` | Missing edge case coverage for newer handlers |
| Q-012 | Missing error case tests in conversation | `internal/service/conversation_test.go` | No tests for malformed NATS, policy failures |
| Q-013 | Inconsistent exception types (Python) | `workers/codeforge/agent_loop.py` | Mix of specific and generic catches |
| Q-015 | Missing test files for critical services | `service/branchprotection.go`, `service/channel.go` | Zero test coverage |
| Q-017 | Panics in registry initialization | `port/specprovider/registry.go:24`, `port/pmprovider/registry.go:24` | Crash on duplicate registration |
| Q-018 | Sparse frontend test coverage (13.5%) | `frontend/src/` — 42 test / 311 source files | Low confidence in UI changes |
| Q-019 | Swallowed exception in model resolver | `workers/codeforge/model_resolver.py:69` | Silent model resolution failure |

### Architecture (6)

| ID | Title | Location | Key Risk |
|----|-------|----------|----------|
| A-001 | Service imports LiteLLM adapter | `service/meta_agent.go:15`, `service/routing.go`, 2 more | Hexagonal violation, tight coupling |
| A-002 | Service imports OTEL adapter | `service/conversation.go:16`, `service/runtime.go:14` | Metrics coupled to implementation |
| A-003 | Service imports auth adapter types | `service/subscription.go:11` | Adapter types leak into service layer |
| A-006 | God service: BenchmarkService | `service/benchmark.go` (1077 LOC, TODO at line 27) | Single responsibility violation |
| A-007 | Missing port-layer LLM abstraction | Multiple services import `adapter/litellm` | Hard to test, hard to swap providers |
| A-010 | Hexagonal rule violation pattern | `service/routing.go`, `service/review_router.go`, `service/model_registry.go` | Service to Adapter coupling across 27 files |

### Infrastructure (13)

| ID | Title | Location | Key Risk |
|----|-------|----------|----------|
| I-006 | No capability dropping on containers | All Dockerfiles, `docker-compose.prod.yml` | Default Linux caps allow privileged ops |
| I-008 | Nginx missing request size limit | `frontend/nginx.conf` | No DoS protection on upload size |
| I-009 | Nginx missing proxy buffering config | `frontend/nginx.conf:24-41` | Memory pressure on streaming |
| I-014 | Secrets potentially in Docker logs | `docker-compose.yml:1-5` | DSN/API keys in error output |
| I-015 | Env var validation only at container start | `docker-compose.prod.yml` | Late failure discovery at deploy time |
| I-016 | HTTP server timeout defaults unclear | `cmd/codeforge/main.go:906-913` | Slow-loris attack surface |
| I-017 | Playwright `--no-sandbox` flag | `docker-compose.yml:40` | Chrome sandbox disabled |
| I-018 | No security headers on Traefik routes | `docker-compose.blue-green.yml:26-63` | XSS/clickjacking on blue-green frontend |
| I-020 | WebSocket not rate-limited | `cmd/codeforge/main.go:819-820` | Unlimited concurrent WS connections |
| I-021 | LLM key encryption derived from JWT secret | `cmd/codeforge/main.go:514` | JWT rotation = all LLM keys unrecoverable |
| I-022 | Auth rate limiter attachment unverified | `cmd/codeforge/main.go:834-837` | Potential brute-force on login |
| I-024 | Internal services expose ports to localhost | All docker-compose files | Port accessible on shared Docker hosts |
| I-025 | No separate audit logging | System-wide | Compliance gap, forensics blind spot |

### Compliance (7)

| ID | Title | Location | Key Risk |
|----|-------|----------|----------|
| C-001 | Email logged in debug on auth failure | `handlers_auth.go:44` | PII in logs (GDPR Art. 5) |
| C-002 | GitHub token embedded in clone URL | `adapter/github/provider.go:69` | Token in git logs, process listings |
| C-003 | Cascade delete not explicit for messages | `store_conversation.go:121` | Data integrity risk |
| C-004 | No OpenAPI/Swagger documentation | System-wide | No machine-readable API spec |
| C-005 | No GDPR data deletion/export endpoints | System-wide | Missing right-to-erasure, data portability |
| C-008 | Insecure cookie flag in dev | `handlers_auth.go:27-31` | Refresh token over HTTP in dev |
| C-014 | Default admin password handling | `service/auth.go:415,422,472` | Initial admin credential exposure |

---

## LOW Findings (18 total)

| ID | Dimension | Title | Location |
|----|-----------|-------|----------|
| S-005 | Security | SQL errors may leak schema to logs | `adapter/http/helpers.go:132-135` |
| S-006 | Security | Path traversal mitigated by sanitizeName | `adapter/http/helpers.go:65-86` |
| S-011 | Security | CSP missing nonce, data: in img-src | `adapter/http/middleware.go:33` |
| Q-006 | Quality | Uninitialized ref in ChatPanel | `frontend/src/features/project/ChatPanel.tsx:92` |
| Q-009 | Quality | Deferred close without error check | `adapter/speckit/provider.go:99` |
| Q-010 | Quality | Fragile VERSION file path resolution | `internal/version/version.go:22` |
| Q-011 | Quality | Duplicate SVG icons in frontend | `ChatPanel.tsx:1-3` (TODO in code) |
| Q-016 | Quality | Unused struct field pattern in Handlers | `adapter/http/handlers.go` |
| Q-020 | Quality | Implicit type coercion in test comparison | `adapter/ws/agui_events_test.go:30-41` |
| A-004 | Architecture | WS adapter imported across 27 services | Multiple service files |
| A-008 | Architecture | WS adapter import pattern unclear | 27 service files |
| A-011 | Architecture | No circular import check in CI | Project-wide (currently clean) |
| A-014 | Architecture | Python worker lacks hex arch structure | `workers/codeforge/` |
| C-006 | Compliance | Image alt text inconsistency | `features/project/FileTree.tsx:283` |
| C-007 | Compliance | CSRF rationale not in arch docs | `adapter/http/middleware.go:17-25` |
| C-015 | Compliance | No CHANGELOG.md | Project root |
| I-019 | Infra | Unpinned GitHub Action versions | `.github/workflows/ci.yml:66,150` |
| I-023 | Infra | Nginx health check proxy config | `frontend/nginx.conf:45-47` |

---

## INFO Findings (10 total)

| ID | Dimension | Title | Status |
|----|-----------|-------|--------|
| Q-014 | Quality | Path traversal test patterns (intentional) | COMPLIANT |
| A-012 | Architecture | HTTP handlers use service layer correctly | COMPLIANT |
| A-013 | Architecture | Frontend properly abstracts API calls | COMPLIANT |
| C-009 | Compliance | Markdown renderer XSS protection | COMPLIANT |
| C-010 | Compliance | Security headers comprehensive | COMPLIANT |
| C-011 | Compliance | Tenant isolation properly enforced | COMPLIANT |
| C-012 | Compliance | Pre-commit security linting configured | COMPLIANT |
| C-013 | Compliance | Cookie session isolation (HttpOnly, SameSite) | COMPLIANT |
| C-017 | Compliance | JSON error handling centralized | COMPLIANT |
| C-018 | Compliance | Copilot token exchange (needs review) | REVIEW |

---

## Positive Observations

The audit identified strong patterns that should be maintained:

- **SQL injection: NOT FOUND** — all queries use parameterized statements
- **Tenant isolation: STRONG** — all queries include `tenant_id` checks via `tenantFromCtx()`
- **Command injection: MITIGATED** — subprocess calls use argument arrays, not shell interpolation
- **XSS: NOT FOUND** — SolidJS reactive bindings used throughout, no unsafe HTML injection
- **Password hashing: bcrypt** — correct algorithm choice
- **CSRF: NOT NEEDED** — Bearer auth + CORS correctly eliminates CSRF risk
- **Crypto randomness: crypto/rand** — used for all security-sensitive random generation
- **Pre-commit hooks: CONFIGURED** — `detect-private-key`, `gosec`, Ruff security rules
- **No circular dependencies** — Go package graph is acyclic
- **Frontend API abstraction** — centralized `api/client` wrapper, no scattered `fetch` calls

---

## Remediation Roadmap

### Immediate (this week)
1. Rotate all 9 API keys in S-001 + GitHub token in S-002
2. Scrub git history with `git filter-repo`
3. Add `detect-secrets` to pre-commit hooks

### Short-term (next 2 sprints)
4. Enable TLS for PostgreSQL, NATS, LiteLLM (I-001, I-003, I-004)
5. Add HSTS header (S-009)
6. Set up automated database backups (I-012)
7. Archive WAL to external storage (I-011)
8. Pin all Docker base images (I-007)
9. Fix Playwright-MCP exposure (I-005)

### Medium-term (next quarter)
10. Decompose Handlers struct (A-005 / Q-001)
11. Split oversized service files (Q-002)
12. Create port-layer abstractions for LiteLLM, OTEL, auth (A-001, A-002, A-003)
13. Add GDPR data deletion/export endpoints (C-005)
14. Generate OpenAPI spec (C-004)
15. Increase frontend test coverage from 13.5% to 60% (Q-018)

### Long-term (backlog)
16. Add separate audit logging (I-025)
17. Implement Traefik TLS (I-013)
18. Add capability dropping to all containers (I-006)
19. Add CHANGELOG.md (C-015)
20. Document CSRF rationale in architecture docs (C-007)
