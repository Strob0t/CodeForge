# Universal Audit Report v2 — CodeForge v0.8.0

**Date:** 2026-03-23
**Target:** `/workspaces/CodeForge` (monorepo)
**Branch:** `staging`
**Auditor:** Claude Opus 4.6 (5 parallel agents, read-only)
**Methodology:** `docs/prompts/universal-audit.md` — 5 weighted dimensions + Strategic Advisor overlay

---

## Audit Summary

| Metric               | Value                              |
|-----------------------|------------------------------------|
| Target                | /workspaces/CodeForge (monorepo)   |
| Input Type            | Mixed / Monorepo (App + Infra + DB + CI/CD + Docs) |
| Files Analyzed        | ~1,700 source files                |
| Lines of Code         | ~270,000 LOC (Go 132K, TS 71K, Python 65K, SQL 2.3K) |
| Total Findings        | 64                                 |
| Critical              | 3                                  |
| High                  | 12                                 |
| Medium                | 33                                 |
| Low                   | 13                                 |
| Informational         | 3                                  |
| Overall Risk Score    | **52 / 100**                       |

---

## Risk Heatmap

| Dimension          | Weight | Score /100 | Top Issue                                      |
|--------------------|--------|------------|------------------------------------------------|
| Security           | 30%    | 65         | Real API keys committed in `.env` (S-001)      |
| Code Quality       | 25%    | 45         | 79/86 store methods untested (Q-009)           |
| Architecture       | 20%    | 40         | God objects: 11 services >15 methods (A-001)   |
| Infrastructure     | 15%    | 55         | No audit trail for admin ops (I-015)           |
| Compliance         | 10%    | 50         | No GDPR data deletion capability (C-010)       |
| **Weighted Total** | 100%   | **52**     | **Secrets exposure in version control (S-001)** |

> Score interpretation: 0 = no risk, 100 = maximum risk. Weighted by dimension importance.

---

## Top 3 Priorities

| # | Finding | Impact | Fix Effort |
|---|---------|--------|------------|
| 1 | **S-001/S-002/S-003: Secrets in Git** — Real API keys (Anthropic, Gemini, GitHub PAT, etc.) committed in `.env`, hardcoded dev secrets in `codeforge.yaml` and `docker-compose.yml` | CRITICAL: Key theft, account takeover, financial loss | Immediate: revoke keys, filter git history, add secret scanning |
| 2 | **C-010: No GDPR data deletion** — No `DELETE /users/{id}` endpoint, no data export, no right-to-erasure workflow | HIGH: Regulatory non-compliance, potential fines | Medium: implement user deletion cascade + export endpoint |
| 3 | **A-001/A-002: God objects** — Handlers struct with 61 deps, BenchmarkService with 30 methods, 11 services >15 methods | HIGH: Coupling, untestability, cognitive overload | Large: decompose incrementally |

---

## Findings by Dimension

### Dimension 1: Security (30%) — 8 Findings

**F-001 (S-001): Real API Keys Committed in .env** — CRITICAL
- Location: `.env:36-51`
- Evidence: `ANTHROPIC_API_KEY=sk-ant-api03-...`, `GITHUB_TOKEN=github_pat_11AMQKT6A0l...`, `GEMINI_API_KEY=AIzaSy...`, `HF_TOKEN=hf_ODvm...` + 6 more
- Risk: Attacker clones repo, extracts all keys, makes LLM calls at victim's expense, accesses GitHub repos via PAT
- Remediation: IMMEDIATE — Revoke all keys. `git filter-repo --path .env --invert-paths`. Add `detect-secrets` to pre-commit.
- Reference: CWE-798, OWASP A07:2021
- Confidence: high

**F-002 (S-002): Development Secrets in Configuration Files** — CRITICAL
- Location: `codeforge.yaml:22,102,145`
- Evidence: `litellm.master_key: "sk-codeforge-dev"`, `auth.jwt_secret: "e2e-test-secret-key-minimum-32-bytes-long"`
- Risk: JWT forgery with known secret, LiteLLM bypass with known master key
- Remediation: Remove all secret defaults. Require env vars with validation.
- Reference: CWE-798, CWE-321
- Confidence: high

**F-003 (S-003): Secrets in Docker Compose Environment Defaults** — CRITICAL
- Location: `docker-compose.yml:145-157`
- Evidence: `LITELLM_MASTER_KEY: ${LITELLM_MASTER_KEY:-sk-codeforge-dev}`, `DATABASE_URL: ...${POSTGRES_PASSWORD:-codeforge_dev}...`
- Risk: Weak defaults used if env vars not set. Visible via `docker inspect`.
- Remediation: Use `${VAR:?error}` syntax instead of `${VAR:-default}` for secrets.
- Reference: CWE-798
- Confidence: high

**F-004 (S-004): IPv6 SSRF Protection Gap** — HIGH
- Location: `internal/netutil/ssrf.go:12-26`
- Evidence: `IsPrivateIP()` checks IPv4 only. Missing: `fc00::/7` (ULA), `::ffff:127.0.0.1` (IPv4-mapped)
- Risk: Skill import via IPv6 URL bypasses SSRF protection -> access to internal services
- Remediation: Add IPv6 private ranges to `IsPrivateIP()`
- Reference: CWE-918, OWASP A10:2021
- Confidence: medium

**F-005 (S-005): WebSocket Token in URL** — MEDIUM
- Location: `internal/middleware/auth.go:97-124`
- Evidence: `tokenParam := r.URL.Query().Get("token")` — JWT logged by proxies
- Mitigated: HTTPS + 15min expiry in production. Known browser limitation.
- Confidence: medium

**F-006 (S-007): CORS Wildcard When APP_ENV Unset** — MEDIUM
- Location: `internal/adapter/http/middleware.go:44-72`
- Evidence: Empty APP_ENV allows wildcard CORS
- Remediation: Invert logic — require explicit `APP_ENV=development` for wildcard
- Confidence: medium

**F-007 (S-006): XSS Prevention in Markdown** — MEDIUM (mitigated)
- Location: `frontend/src/features/project/Markdown.tsx:108-134`
- Status: Properly escaped and URL-validated. No action needed.
- Confidence: high

**F-008 (S-008): Default Admin Password Pattern** — MEDIUM (mitigated)
- Location: `codeforge.yaml:106-110`
- Status: Setup wizard enforces password change. No action needed.
- Confidence: low

---

### Dimension 2: Code Quality (25%) — 16 Findings

**F-009 (Q-001): Ignored Errors in HTTP Response Reading** — HIGH
- Location: `internal/adapter/auth/anthropic.go:70`, `github.go:76,116`, `discord/notifier.go:94`, `plane/provider.go:268,310`, `slack/notifier.go:95`
- Evidence: `respBody, _ := io.ReadAll(resp.Body)` — 9 occurrences
- Risk: Corrupted response bodies processed silently
- Confidence: high

**F-010 (Q-002): Ignored Errors in JSON Marshaling** — HIGH
- Location: `internal/adapter/gitea/provider.go:152,183`, `gitlab/provider.go:106,134`
- Evidence: `payloadJSON, _ := json.Marshal(payload)` — 4+ occurrences
- Confidence: high

**F-011 (Q-003): Panic in Production Constructors** — HIGH
- Location: `internal/adapter/mcp/server.go:64-72`
- Evidence: `panic("MCP ServerDeps.ProjectLister must not be nil")` — 3 panics
- Remediation: Change to `NewServer(...) (*Server, error)`
- Confidence: high

**F-012 (Q-006): TypeScript Double Casting Through unknown** — MEDIUM
- Location: canvasState.ts, BenchmarkPage.tsx, CodeEditor.tsx, ProjectDetailPage.tsx — 20+ instances
- Confidence: high

**F-013 (Q-004): Dead Store Assignment** — MEDIUM
- Location: `internal/adapter/postgres/store_prompt_variant.go:122`
- Evidence: `_ = argIdx` — blank assignment suppresses unused variable
- Confidence: high

**F-014 (Q-007): Promise Chains Instead of async/await** — MEDIUM
- Location: `frontend/src/api/resources/llm.ts:20`, projects.ts, BenchmarkPage.tsx — 20+ instances
- Confidence: medium

**F-015 (Q-005): Ignored Error in Body Close** — MEDIUM
- Location: `internal/adapter/auth/anthropic.go:68` and similar
- Remediation: Use `logBestEffort` pattern from `internal/service/log_best_effort.go`
- Confidence: medium

**F-016 (Q-009): 79 of 86 Store Methods Untested** — LOW
- Location: `internal/adapter/postgres/`
- Risk: Regressions undetected. Violates CLAUDE.md TDD mandate.
- Confidence: high

**F-017 (Q-008): God Object — store.go (1487 LOC, 86 methods)** — LOW
- Location: `internal/adapter/postgres/store.go`
- Confidence: high

**F-018 (Q-010): Large Service Functions >200 LOC** — LOW
- Location: `internal/service/conversation_agent.go` — SendMessageAgentic (281 LOC)
- Confidence: high

**F-019 (Q-011): Large Components >800 LOC** — LOW
- Location: ProjectDetailPage.tsx (1036), FilePanel.tsx (929), PolicyPanel.tsx (866), MicroagentsPage.tsx (850), BenchmarkPage.tsx (839)
- Confidence: high

**F-020 (Q-012): interface{} Usage** — LOW
- Location: `internal/adapter/postgres/store_prompt_variant.go:109`
- Confidence: high

**F-021 (Q-013): TODO/FIXME Comments** — LOW
- Location: `internal/adapter/http/routes.go` — FIX-061, FIX-063, FIX-095, FIX-098, FIX-100
- Confidence: high

**F-022 (Q-014): Empty .catch() Handlers** — INFO
- Location: BenchmarkPage.tsx:359 — 5+ instances
- Confidence: medium

**F-023 (Q-015): Large Python Files** — INFO
- Location: agent_loop.py (1580), _conversation.py (1216), _benchmark.py (1040)
- Confidence: medium

**F-024 (Q-016): Ignored filepath.Rel Error** — INFO
- Location: `internal/adapter/autospec/provider.go:59`
- Confidence: medium

---

### Dimension 3: Architecture (20%) — 9 Findings

**F-025 (A-001): God Objects — 11 Services >15 Methods** — HIGH
- Location: benchmark.go (30), auth.go (26), project.go (24), roadmap.go (23), runtime.go (22), a2a.go (19), context_optimizer.go (18), agent.go (18), conversation.go+conversation_agent.go (45 combined)
- Risk: SRP violation, excessive coupling, hard to test
- Confidence: high

**F-026 (A-002): Handlers Struct — 61 Dependencies** — HIGH
- Location: `internal/adapter/http/handlers.go:48-118`
- Evidence: 61 service fields, 51 handler methods, 1139 LOC
- Remediation: Split into domain-specific handler groups
- Confidence: high

**F-027 (A-008): BenchmarkService Decomposition Required** — HIGH
- Location: `internal/service/benchmark.go` (1077 LOC, 30 methods)
- Evidence: Explicit TODO acknowledges the problem
- Confidence: high

**F-028 (A-003): Service Layer Imports Adapter Directly** — MEDIUM
- Location: `internal/service/lsp.go:11`
- Evidence: `import lspAdapter "...internal/adapter/lsp"` — violates hexagonal architecture
- Remediation: Create port interface in `internal/port/codeintel/`
- Confidence: high

**F-029 (A-004): Direct I/O in Service Layer** — MEDIUM
- Location: benchmark.go, files.go, project.go, roadmap.go, checkpoint.go, sandbox.go
- Evidence: `filepath.WalkDir()`, `os.ReadDir()`, `os.Stat()`, `exec.CommandContext()` in service
- Confidence: medium

**F-030 (A-005): Missing RBAC on Write Endpoints** — MEDIUM
- Location: `internal/adapter/http/routes.go:179-230`
- Evidence: No `RequireRole` on: POST /llm/models, DELETE /llm/models, POST /policies, POST /modes, etc.
- Risk: Any authenticated user can manage LLM models, policies, modes
- Reference: OWASP A01:2021
- Confidence: high

**F-031 (A-006): Conversation Service Split Anti-Pattern** — MEDIUM
- Location: conversation.go (405 LOC) + conversation_agent.go (1163 LOC) = 1568 LOC, 45 methods
- Risk: God object disguised as two files
- Confidence: high

**F-032 (A-007): Large Frontend Components** — LOW
- Location: 5 components >600 LOC mixing data fetching and presentation
- Confidence: medium

**F-033 (A-009): Context Budget Scattered** — LOW
- Location: context_budget.go — 3 separate exported functions, no unified contract
- Confidence: medium

---

### Dimension 4: Infrastructure & Operations (15%) — 20 Findings

**F-034 (I-015): No Audit Trail for Admin Operations** — HIGH
- Location: `internal/adapter/http/routes.go:495-500,200-205,638-642`
- Evidence: No audit entry for: user creation/deletion, policy changes, quarantine approval
- Reference: SOC 2 CC6.1, CWE-778
- Confidence: high

**F-035 (I-001): Docker :latest Tags in Production** — HIGH
- Location: `docker-compose.prod.yml:155,197,233`, `docker-compose.yml:24,107`
- Confidence: high

**F-036 (I-005): No Resource Limits in Dev Compose** — HIGH
- Location: `docker-compose.yml` — all services
- Confidence: high

**F-037 (I-026): PostgreSQL Port Exposed on Host** — HIGH
- Location: `docker-compose.yml:61-62` — `ports: - "5432:5432"`
- Remediation: Bind to `127.0.0.1:5432:5432`
- Confidence: high

**F-038 (I-002): Playwright --no-sandbox** — MEDIUM
- Location: `docker-compose.yml:42`
- Confidence: high

**F-039 (I-012): No PII Redaction in Logs** — MEDIUM
- Confidence: medium

**F-040 (I-017): Missing NATS Backlog Metrics** — MEDIUM
- Confidence: medium

**F-041 (I-018): No Alerting Rules** — MEDIUM
- Confidence: medium

**F-042 (I-023): No Archive Retention Policy** — MEDIUM
- Confidence: medium

**F-043 (I-025): NATS Monitoring Exposed Without Auth** — MEDIUM
- Location: `docker-compose.yml:92-94`
- Remediation: Bind to `127.0.0.1:8222:8222`
- Confidence: high

**F-044 (I-028): Traefik Config Incomplete** — MEDIUM
- Evidence: No TLS, no rate limiting, no access logs
- Confidence: medium

**F-045 (I-029): No TLS Core<->LiteLLM** — MEDIUM
- Evidence: `http://litellm:4000` — plain HTTP with API keys
- Confidence: medium

**F-046 (I-030): No TLS Core<->NATS** — MEDIUM
- Evidence: `nats://...@nats:4222` — plain TCP with credentials
- Confidence: medium

**F-047 (I-024): Data Loss on Container Crash** — MEDIUM
- Evidence: `go q.handleMessage()` fire-and-forget — unacked messages redelivered
- Confidence: high

**F-048 (I-006): Insufficient Prod Resource Limits** — MEDIUM
- Evidence: Worker 4GB memory, no per-request timeout on LLM calls
- Confidence: high

**F-049 (I-003): Playwright Missing USER** — MEDIUM
- Evidence: Runs as root by default
- Confidence: medium

**F-050 (I-037): LiteLLM Not Pinned to Digest** — MEDIUM
- Evidence: `:main-stable` rolling tag in dev
- Confidence: medium

**F-051 (I-033): Dev Compose Lacks Hardening** — MEDIUM
- Evidence: No `cap_drop`, `security_opt`, `read_only` (prod has all three)
- Confidence: medium

**F-052 (I-031): NATS Monitoring Unprotected in Prod** — MEDIUM
- Evidence: Port 8222 accessible from any container on internal network
- Confidence: medium

**F-053 (I-013): API Keys via Env Vars** — MEDIUM
- Evidence: Visible via `docker inspect`
- Remediation: Use Docker secrets or Vault
- Confidence: high

---

### Dimension 5: Compliance & Standards (10%) — 11 Findings

**F-054 (C-010): No GDPR Data Deletion Capability** — HIGH
- Location: Codebase-wide (absent)
- Evidence: No DELETE /users/{id}, no data export, no right-to-erasure workflow
- Reference: GDPR Article 17
- Confidence: high

**F-055 (C-011): Data Retention Not Documented** — MEDIUM
- Evidence: No TTL on any table. agent_events, conversations grow indefinitely.
- Confidence: high

**F-056 (C-012): Audit Logging Partial** — MEDIUM
- Evidence: Event sourcing exists. Admin action logging absent.
- Reference: SOC 2 CC6.1
- Confidence: medium

**F-057 (C-013): LLM Consent/Privacy Policy Missing** — MEDIUM
- Evidence: Code sent to 127+ LLM providers. No consent flow. No privacy notice.
- Reference: GDPR Article 13
- Confidence: medium

**F-058 (C-018): Security Documentation Incomplete** — MEDIUM
- Evidence: Missing SECURITY.md, incident reporting, vulnerability disclosure
- Confidence: medium

**F-059 (C-019): WCAG Color Contrast Exception** — MEDIUM
- Evidence: 4.39:1 vs required 4.5:1 — within 0.11 points
- Confidence: high

**F-060 (C-009): Default Credentials (Dev-Only)** — LOW (mitigated)
- Confidence: high

**F-061 (C-014): No OpenAPI Specification** — LOW
- Evidence: 344 routes, no machine-readable spec
- Confidence: high

**F-062 (C-021): Form Labels Need Audit** — LOW
- Evidence: 33 labels, 115+ inputs — spot checks pass, full audit needed
- Confidence: medium

**F-063 (C-022): Semantic HTML Mostly Good** — LOW
- Confidence: medium

**F-064 (C-023): Keyboard Navigation Untested** — LOW
- Evidence: No keyboard E2E test, potential keyboard traps in modals
- Confidence: low

---

## Strengths Observed

| Area | Strength |
|---|---|
| Tenant Isolation | 460+ queries with `WHERE tenant_id = $N`, UUID validation |
| Cryptography | bcrypt (configurable cost), SHA256, HMAC-SHA256 JWT |
| Auth | Bearer token -> API key -> WS token fallback, JTI revocation, account lockout |
| Command Injection | All `exec.Command()` annotated with gosec |
| XSS Prevention | HTML entity escaping, URL protocol validation |
| Security Headers | CSP, HSTS, X-Frame-Options, X-Content-Type-Options |
| Prod Hardening | `read_only: true`, `cap_drop: [ALL]`, `no-new-privileges`, non-root USER |
| HTTP Timeouts | Read 30s, Write 60s, Idle 120s, LLM health 10s |
| Query Limits | DefaultListLimit=100, parameterized pagination |
| NATS Resilience | AckWait 90s, MaxAckPending 100, consumer health monitoring |
| Async Logging | Buffered channels, workers, dropped message counter |
| Backup | WAL archiving, JetStream persistence, persistent volumes |
| License | AGPL-3.0, all deps compatible |
| Rate Limiting | Auth rate limiter, account lockout (5 failures / 15min) |
| Accessibility | axe-core E2E, proper alt text, ARIA roles, semantic HTML |
| Changelog | Keep a Changelog format maintained |

---

## Strategic Advisor Assessment

### Assumptions
1. `.env` contains real, active credentials (if expired, S-001 drops to LOW)
2. Production uses same Docker Compose files (if K8s, infra findings may not apply)
3. EU users will be processed (if internal-only, GDPR findings are informational)
4. Project is pre-production v0.8.0 (many findings are appropriate for this stage)

### The Most Common Mistake
**Treating audit findings as a linear TODO.** Teams fix top-to-bottom, spending weeks on MEDIUM items while CRITICALs remain. Correct: fix all CRITICALs in 24h, then prioritize by risk-weighted impact.

### Single Biggest Risk
**S-001** — the only finding where damage is already done. Keys are in git history even if deleted from HEAD. Every other finding is latent. This one is actively exploitable.

---

## Recommended Remediation Order

### Phase 1: Immediate (24h) — CRITICAL
1. Revoke all API keys in `.env`
2. Remove `.env` from git history (`git filter-repo`)
3. Add `detect-secrets` pre-commit hook
4. Replace secret defaults with `${VAR:?required}` syntax

### Phase 2: Short-term (1 week) — HIGH
5. IPv6 SSRF protection (S-004)
6. Handle ignored errors: `io.ReadAll`, `json.Marshal` (Q-001/Q-002)
7. Replace panics with error returns (Q-003)
8. Add `RequireRole` middleware to write endpoints (A-005)
9. Bind dev ports to localhost (I-026, I-025)
10. Admin audit log table + middleware (I-015)

### Phase 3: Medium-term (1 month) — MEDIUM
11. GDPR data deletion + export (C-010)
12. Data retention policy (C-011)
13. LLM consent flow + privacy policy (C-013)
14. Decompose BenchmarkService + Handlers struct (A-008, A-002)
15. TLS between internal services (I-029, I-030)
16. Resource limits in dev compose (I-005)
17. Pin Docker images (I-001, I-037)
18. SECURITY.md (C-018)

### Phase 4: Ongoing — LOW/INFO
19. Store method test coverage (Q-009)
20. Decompose god objects incrementally (A-001)
21. Frontend component splitting (A-007)
22. OpenAPI spec generation (C-014)
23. Keyboard nav E2E tests (C-064)
24. NATS metrics export (I-017)
25. Alerting rules (I-018)
