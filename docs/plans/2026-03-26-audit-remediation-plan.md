# Audit Remediation Plan — 2026-03-26

Based on Universal Audit (76 findings across 5 dimensions) + targeted research across GitHub, OWASP, CWE, NIST, academic papers, and production Go/Python/TS codebases.

**Execution Model:** 12 parallel worktrees, 5 phases. Each WT is a self-contained branch.

---

## Phase 1 — Security Critical (parallel: WT-1 + WT-2 + WT-3)

### WT-1: `fix/rbac-sweep` — ~2h

**Findings:** F-ARC-011 (CRIT), F-ARC-012 (CRIT), F-SEC-003 (HIGH), F-ARC-013 (MED), F-ARC-014 (MED), F-COM-007 (MED)

**Approach:** Apply `middleware.RequireRole(...)` via `r.With()` to every unprotected write endpoint. Add `audit()` middleware to security-sensitive operations. Build a `chi.Walk`-based CI test that guarantees 100% RBAC coverage on POST/PUT/DELETE/PATCH routes.

**Key Sources:**
- [OWASP A01:2021 Broken Access Control](https://owasp.org/Top10/2021/A01_2021-Broken_Access_Control/) — "Deny by default except public resources"
- [go-chi/chi `_examples/router-walk/main.go`](https://github.com/go-chi/chi/blob/master/_examples/router-walk/main.go) — Route coverage audit pattern
- [SOC 2 Audit Log Essential Guide](https://hoop.dev/blog/the-essential-guide-to-audit-logging-for-soc-2-compliance/) — "Who, What, When, Outcome" framework

**Steps:**
1. Map all unprotected write endpoints in `routes.go` (~25 endpoints)
2. Apply RBAC per endpoint:
   - `bypass-approvals`, branch-rule DELETE: `RequireRole(Admin)` only
   - All other write endpoints: `RequireRole(Admin, Editor)`
   - Read endpoints: no change (authenticated is sufficient)
3. Add `audit()` middleware to: password change, API key CRUD, bypass-approvals, branch-rule CRUD
4. Enhance `AuditEntry` with: `UserAgent`, `CorrelationID`, `HTTPMethod`, `Path`, `StatusCode`
5. Write `TestAllWriteRoutesHaveRBAC` using `chi.Walk` — test each write route as viewer role, assert 403

**Files:** `internal/adapter/http/routes.go`, `internal/middleware/audit.go`, new `internal/adapter/http/routes_rbac_test.go`

**Edge Cases:**
- `chi.Walk` issue #750: nested group middlewares may not be visible — test with actual HTTP requests
- Auth-disabled mode: RBAC always passes — test must run with auth enabled
- API keys: verify they populate user context with role
- WebSocket endpoints: verify separately (bypass chi middleware after upgrade)

**Verification:** `go test ./internal/adapter/http/... -run TestAllWriteRoutes` + manual curl as viewer role

---

### WT-2: `fix/path-network-security` — ~3h

**Findings:** F-SEC-001 (HIGH), F-SEC-002 (HIGH), F-SEC-004 (MED), F-SEC-005 (MED), F-SEC-007 (MED), F-SEC-010 (LOW), F-SEC-012 (LOW)

#### 2a. Python Path Traversal (F-SEC-001)

**Approach:** Replace `str(target).startswith(str(workspace))` with `target.is_relative_to(workspace)` (Python 3.9+, component-based check).

**Key Sources:**
- [Endor Labs: Path Traversal in OpenClaw via LLM Guardrail Bypass](https://www.endorlabs.com/learn/ai-sast-finding-path-traversal-in-openclaw-via-llm-guardrail-bypass) — GHSA-r5fq-947m-xm57, CVSS 8.8, exact same bug class
- [CWE-22](https://cwe.mitre.org/data/definitions/22.html)
- [Python pathlib docs: `PurePath.is_relative_to()`](https://docs.python.org/3/library/pathlib.html)

**Fix:**
```python
# workers/codeforge/tools/_base.py:68 + glob_files.py:72
# BEFORE: if not str(target).startswith(str(workspace)):
# AFTER:
if not target.is_relative_to(workspace):
```

**Tests:** `test_path_traversal_prefix_confusion()` — workspace `/workspace`, target `/workspace_evil/payload` must be blocked.

#### 2b. Webhook Authentication (F-SEC-002)

**Approach:** Implement HMAC-SHA256 verification with per-channel secrets and replay protection (timestamp + nonce). Follow pattern from existing `middleware/webhook.go`.

**Key Sources:**
- [webhooks.fyi: HMAC Security](https://webhooks.fyi/security/hmac)
- [Hooque: Webhook Security Best Practices](https://hooque.io/guides/webhook-security/)

**Fix:** Replace `X-Webhook-Key` presence check with `X-Webhook-Signature` HMAC-SHA256 validation using `subtle.ConstantTimeCompare`. Add `X-Webhook-Timestamp` for 5-minute replay window.

**Files:** `internal/adapter/http/handlers_channel.go:130-152`

#### 2c. SSRF Prevention (F-SEC-005, F-SEC-007, F-SEC-012)

**Approach:** Use existing `netutil.SafeTransport()` for all HTTP clients with user-controlled URLs.

**Key Sources:**
- [Andrew Ayer: Preventing SSRF in Golang](https://www.agwa.name/blog/post/preventing_server_side_request_forgery_in_golang) — Custom `DialContext` validates IPs at connection time
- [Doyensec safeurl for Go](https://github.com/doyensec/safeurl)
- [OWASP SSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)

**Files:** `service/a2a.go:425`, `service/project.go:701`, `adapter/slack/notifier.go`, `adapter/discord/notifier.go`

#### 2d. Path Restrictions (F-SEC-004, F-SEC-010)

**Fix:** Validate workspace adoption path is within configured `workspaceRoot`. Restrict `DetectStackByPath` to workspace root.

---

### WT-3: `fix/config-hardening` — ~2h

**Findings:** F-INF-001 (CRIT), F-INF-002 (HIGH), F-INF-003 (HIGH), F-INF-004 (HIGH), F-SEC-006 (MED), F-SEC-009 (LOW), F-INF-010 (MED), F-INF-011 (MED), F-INF-012 (MED), F-INF-019 (INFO)

**Approach:** Three-pronged: (1) remove hardcoded secrets from committed configs, (2) auto-generate secrets on first boot, (3) entropy-based validation replacing brittle blocklist.

**Key Sources:**
- [CWE-798: Hard-coded Credentials](https://cwe.mitre.org/data/definitions/798.html) — "Implement a 'first login' mode"
- [Gitea Config Cheat Sheet](https://docs.gitea.com/administration/config-cheat-sheet) — Auto-generates `SECRET_KEY` on first install
- [12-Factor App: Config](https://12factor.net/config) — "Litmus test: could codebase be open source without compromising credentials?"

**Steps:**
1. Clean `codeforge.example.yaml`: secrets = `""`, add documentation comments
2. Add `codeforge.yaml` to `.gitignore` (it's dev config, not template)
3. Implement `ensureSecrets()` in `config/loader.go` using existing `crypto.GenerateRandomToken()`
4. Add `_FILE` suffix support for Docker/K8s secrets (`loadSecretFromFile`)
5. Replace blocklist validation with entropy-based check (Shannon entropy < 3.0 bits/char = reject)
6. Add `json:"-"` to `LLMKeyEncryptionSecret`
7. Set OTEL `insecure: false` default, `sample_rate: 0.1`
8. Set A2A `allow_open: false` default
9. Add TLS config placeholders (`tls_cert_file`, `tls_key_file`)

**Files:** `codeforge.yaml`, `codeforge.example.yaml`, `config/config.go`, `config/loader.go`, `.gitignore`

**Verification:**
```go
TestValidate_RejectsLowEntropySecret  // entropy < 3.0
TestValidate_AcceptsHighEntropySecret // entropy >= 3.0
TestEnsureSecrets_GeneratesJWTOnFirstBoot
TestLoadSecretFromFile
```

---

## Phase 2 — Data Safety (parallel: WT-4 + WT-5 + WT-8)

### WT-4: `fix/query-safety` — ~3h

**Findings:** F-INF-006 (MED), F-INF-007 (MED), F-SEC-008 (MED), F-QUA-005 (MED), F-QUA-006 (MED)

**Approach:** Keyset pagination for high-volume tables, hard LIMIT on all unbounded queries, max-cap on query parameters, `logBestEffort` for silenced errors.

**Key Sources:**
- [Citus Data: Five Ways to Paginate in Postgres](https://www.citusdata.com/blog/2016/03/30/five-ways-to-paginate/) — Keyset is O(1) at any depth
- [OWASP API4:2023 Unrestricted Resource Consumption](https://owasp.org/API-Security/editions/2023/en/0xa4-unrestricted-resource-consumption/)
- [JetBrains: Secure Go Error Handling Best Practices](https://blog.jetbrains.com/go/2026/03/02/secure-go-error-handling-best-practices/)

**Steps:**
1. Define constants: `DefaultListLimit = 50`, `MaxListLimit = 500`
2. Add `LIMIT $N` to 9 unbounded queries (`store_benchmark.go`, `eventstore.go`, `store_a2a.go`)
3. Replace `queryParamInt` with `queryParamIntClamped(r, name, default, min, max)` — 7 handlers
4. Add `WHERE tenant_id = $1` guard + system-context comment to `AggregateRoutingOutcomes`
5. Replace `_ = unmarshalJSONField` with `logBestEffort` — 5 store callsites
6. Replace `_ = json.Decode` with `writeError(w, 400, ...)` — 6 handler callsites

**Files:** `postgres/store_benchmark.go`, `postgres/eventstore.go`, `postgres/store_a2a.go`, `postgres/store_routing.go`, `postgres/store_tenant.go`, `postgres/store_memory.go`, `http/helpers.go`, `http/handlers_cost.go`, `http/handlers_routing.go`, `http/handlers_channel.go`, `http/handlers_session.go`, `http/handlers_dashboard.go`

---

### WT-5: `fix/gdpr-compliance` — ~2 days

**Findings:** F-COM-004 (HIGH), F-COM-003 (HIGH), F-COM-001 (HIGH), F-COM-005 (MED), F-COM-006 (MED), F-COM-008 (LOW)

**Approach:** Self-service GDPR endpoints, PII removal from logs, LLM consent mechanism, tenant-aware retention.

**Key Sources:**
- [ICO: Right to Data Portability](https://ico.org.uk/for-organisations/uk-gdpr-guidance-and-resources/individual-rights/individual-rights/right-to-data-portability/) — JSON/CSV acceptable formats
- [GDPR Article 20](https://gdpr-info.eu/art-20-gdpr/) — Response within 1 month
- [Arcjet: Redacting Sensitive Data with slog](https://blog.arcjet.com/redacting-sensitive-data-from-logs-with-go-log-slog/) — `LogValuer` allowlist pattern

**Steps:**
1. **Self-service endpoints:** Add `/me/export` (GET) and `/me/data` (DELETE) under authenticated non-admin route group. Scope to requesting user's own data via JWT user ID.
2. **PII in logs:** Replace 4 callsites (`auth.go:347,374,409`, `handlers_auth.go:391`) — log `user_id` instead of `email`. Add `Email` type with `LogValue()` → `"***@domain.com"`.
3. **License audit:** `deepeval` is Apache 2.0 (confirmed on PyPI/GitHub) — AGPL-compatible. No action needed. Add `pip-licenses` check to CI for transitive deps.
4. **Consent mechanism:** Create `user_consents` table (migration), add `RequireLLMConsent` middleware, build `LLMConsentDialog.tsx` in frontend. Consent = granular per provider, revocable, auditable.
5. **Retention:** Update `RunCleanup` to iterate all tenants via `ListTenants()` + `tenantctx.WithTenant()`.
6. **Token prefix:** Guard `handlers_auth.go:390` with `if cfg.AppEnv == "development"`.

**Files:** `routes.go`, `service/auth.go`, `handlers_auth.go`, `service/retention.go`, `domain/user/email.go` (new), migration `XXX_create_user_consents.sql`, `frontend/src/features/onboarding/LLMConsentDialog.tsx` (new)

---

### WT-8: `fix/cicd-docker` — ~2h

**Findings:** F-INF-005 (MED), F-INF-008 (MED), F-INF-009 (MED), F-INF-013 (MED), F-INF-014 (LOW), F-INF-015 (LOW), F-INF-016 (LOW), F-INF-017 (LOW), F-INF-018 (LOW)

**Approach:** Pin all CI tools and Docker base images, lower Grype cutoff, enhance worker health check, harden nginx.

**Key Sources:**
- [StepSecurity: Pinning GitHub Actions](https://www.stepsecurity.io/blog/pinning-github-actions-for-enhanced-security-a-complete-guide) — SHA pinning + Renovate
- [tj-actions compromise (March 2025)](https://www.paloaltonetworks.com/blog/cloud-security/trivy-supply-chain-attack/) — 23K repos affected by tag poisoning
- [NATS Monitoring Docs](https://docs.nats.io/running-a-nats-service/nats_admin/monitoring) — `/healthz` endpoint

**Steps:**
1. Replace `pip install poetry` with `snok/install-poetry@<SHA>` (version 1.8.5)
2. Pin `govulncheck@v1.1.4`, `pip-audit`, `poetry-plugin-export`
3. Add SHA256 digest to all `FROM` lines in Dockerfiles
4. Add `renovate.json` with `docker:pinDigests` + `helpers:pinGitHubActionDigests`
5. Change Grype `severity-cutoff: critical` → `high`
6. Replace worker HEALTHCHECK with `healthcheck.py` (checks NATS + LiteLLM connectivity)
7. Worker Dockerfile: replace `COPY --from=build /app /app` with targeted copies
8. nginx: `client_max_body_size 10M` (default), `50M` for uploads; `limit_conn ws_conn 20` for WS; `proxy_read_timeout 300` for WS
9. `.golangci.yml`: replace global `G115` exclude with targeted `nolint:gosec` comments
10. Pre-commit: update `TekWizely/pre-commit-golang` to stable release

**Files:** `.github/workflows/ci.yml`, `.github/workflows/docker-build.yml`, `Dockerfile`, `Dockerfile.worker`, `Dockerfile.frontend`, `frontend/nginx.conf`, `.golangci.yml`, `.pre-commit-config.yaml`, new `renovate.json`, new `workers/codeforge/healthcheck.py`

---

## Phase 3 — Refactoring (parallel: WT-6 + WT-7 + WT-10)

### WT-6: `refactor/python-quality` — ~4h

**Findings:** F-QUA-002 (HIGH), F-QUA-007 (MED), F-QUA-008 (MED), F-QUA-014 (MED), F-QUA-013 (MED)

**Approach:** Decompose agent loop into state-machine style with typed `StepOutcome` union. Add full type annotations. Reduce C901 via dictionary dispatch and helper extraction.

**Key Sources:**
- [OpenHands Software Agent SDK V1](https://arxiv.org/abs/2511.03690) — Step function pattern, typed event-stream
- [LangGraph ReAct Agent](https://github.com/langchain-ai/react-agent) — Two-node graph, ~20 LOC loop body
- [McCabe Cyclomatic Complexity NIST235](https://en.wikipedia.org/wiki/Cyclomatic_complexity) — Limit of 10 confirmed

**Steps:**
1. Define `StepOutcome = IterationStop | IterationContinue | IterationError` (frozen dataclasses with `Literal` kind field)
2. Refactor `run()`: thin orchestrator (~50 LOC) → `_run_iteration()` → `_do_llm_iteration()` / `_process_llm_response()`
3. Add type annotations to all `_do_llm_iteration`, `_process_llm_response`, `_execute_tool_call` parameters
4. Replace `isinstance(result, str)` dispatch with `match` on `StepOutcome`
5. Decompose 7 `noqa: C901` functions via lookup tables and helper extraction
6. Add tests for `executor.py` and `secrets.py` using `pytest-asyncio` + `AsyncMock`

**Files:** `workers/codeforge/agent_loop.py`, `workers/codeforge/consumer/_conversation.py`, `workers/codeforge/repomap.py`, `workers/codeforge/retrieval.py`, new test files

**Verification:** `ruff check --select C901 workers/codeforge/` — no function exceeds 10. All existing tests pass.

---

### WT-7: `refactor/go-service-cleanup` — ~2 days

**Findings:** F-QUA-001 (HIGH), F-QUA-003 (HIGH), F-ARC-001 (HIGH), F-ARC-002 (MED), F-ARC-003 (MED), F-QUA-016 (MED), F-ARC-007 (MED), F-QUA-009 (LOW), F-QUA-010 (LOW)

**Approach:** Decompose `main.go:run()` into bootstrap phases. Remove duplicate `buildSystemPrompt`. Extract `net/http` and `os/exec` behind port interfaces.

**Key Sources:**
- [Gitea Server Startup](https://deepwiki.com/go-gitea/gitea/8.2-server-startup-and-initialization) — 10-phase decomposition pattern
- [Go Project Structure 2025](https://www.glukhov.org/post/2025/12/go-project-structure/) — `bootstrap/` directory for wiring
- [DI in Go: Wire vs fx vs Manual](https://www.glukhov.org/post/2025/12/dependency-injection-in-go/) — Manual DI with phase decomposition
- [Josh Rendek: Go os/exec Interface](https://joshrendek.com/2014/06/go-lang-mocking-exec-dot-command-using-interfaces/)

**Steps:**
1. Decompose `run()` (1054 LOC) → `initInfra()`, `initServices()`, `initHTTP()`, `serve()` — each <200 LOC
2. Make `PromptAssemblyService` a required dependency in `ConversationService` — delete fallback copy in `conversation_agent.go:887-1059`
3. Define `httpDoer` interface in `service/*_deps.go` — replace `net/http` imports in 4 service files
4. Define `commandRunner` interface — replace `os/exec` in 5 service files, route through `port/shell`
5. Replace `min64` with built-in `min` (Go 1.25), delete custom function
6. Remove dead code `writeValidationError` + `validationErrorResponse`
7. `middleware/auth.go`: accept `Authenticator` interface instead of `*service.AuthService`

**Files:** `cmd/codeforge/main.go` (split into `bootstrap.go`), `service/conversation_agent.go`, `service/conversation_prompt.go`, `service/a2a.go`, `service/vcsaccount.go`, `service/project.go`, `service/checkpoint.go`, `service/deliver.go`, `service/autoagent.go`, `service/sandbox.go`, `service/complexity.go`, `middleware/auth.go`, `http/helpers.go`

---

### WT-10: `fix/frontend-a11y` — ~3h

**Findings:** F-COM-012 (MED), F-QUA-004 (MED), F-COM-013 (LOW), F-ARC-010 (LOW)

**Approach:** Keyboard navigation for canvas, `focus-visible` migration, error extraction utility.

**Key Sources:**
- [tldraw WCAG Megathread #5215](https://github.com/tldraw/tldraw/issues/5215) — Tab/Arrow navigation, live regions
- [Figma Keyboard Accessibility](https://www.figma.com/blog/introducing-keyboard-accessibility-features/) — Arrow-key canvas navigation
- [W3C ARIA: Keyboard Interface](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/)

**Steps:**
1. **Canvas a11y:** Add `role="application"`, `aria-label`, `tabIndex={0}`, `onKeyDown` to SVG root. Add `tabIndex={0}`, `role="img"`, `aria-label` to each element. Implement Tab cycling + Arrow-key spatial navigation. Add `aria-live="polite"` region for announcements.
2. **Error utility:** Create `frontend/src/lib/errorUtils.ts` with `extractErrorMessage(e: unknown, fallback?: string): string`. Replace 52 occurrences across 25 files.
3. **Focus-visible:** Replace 16 `focus:ring` with `focus-visible:ring` in `Input.tsx`, `Select.tsx`, `Textarea.tsx`. Add pre-commit grep check.

**Files:** `frontend/src/features/canvas/DesignCanvas.tsx`, `frontend/src/features/canvas/CanvasToolbar.tsx`, `frontend/src/lib/errorUtils.ts` (new), `frontend/src/ui/primitives/Input.tsx`, `frontend/src/ui/primitives/Select.tsx`, 25 files for error extraction

---

## Phase 4 — Architecture (sequential: WT-9)

### WT-9: `refactor/god-objects` — ~5 days (phased)

**Findings:** F-ARC-004 (HIGH), F-ARC-005 (HIGH), F-ARC-006 (HIGH)

**Approach:** Strangler Fig pattern — wrap each god object in a facade that delegates to smaller services. Migrate one responsibility at a time.

**Key Sources:**
- [Martin Fowler: "This class is too large"](https://martinfowler.com/articles/class-too-large.html) — Extract Class, keep facade during migration
- [Shopify: Strangler Fig for God Object](https://shopify.engineering/refactoring-legacy-code-strangler-fig-pattern) — Production-scale decomposition
- [ISP in Go (Redowan 2025)](https://rednafi.com/go/interface-segregation/) — Consumer-defined interfaces
- [Dave Cheney: SOLID Go Design](https://dave.cheney.net/2016/08/20/solid-go-design) — "The bigger the interface, the weaker the abstraction"

**Phases:**

**Phase 4a (day 1-2):** RuntimeService (54 methods → 4 services)
- `RuntimeLifecycleService` — run cleanup, cancellation, finalization, delivery (11 methods)
- `RuntimeExecutionService` — tool call handling, run complete, quality gate (5 methods)
- `RuntimeSubscriberService` — NATS message handlers (9 methods)
- `RuntimeApprovalService` — HITL approval, feedback (4 methods)
- Each service gets narrow consumer-defined interface for DB access

**Phase 4b (day 3):** ConversationService (53 methods → 3 services)
- `ConversationCoreService` — CRUD, search, list (~15 methods)
- `ConversationAgentService` — run dispatch, completion, scoring (~20 methods)
- `ConversationPromptService` — already extracted as `PromptAssemblyService`

**Phase 4c (day 4):** Handlers (67 fields → domain handler groups)
- Continue pattern from existing `ProjectHandlers`, `AgentHandlers` etc.
- Migrate remaining methods from monolithic `Handlers` struct
- `Handlers` becomes thin facade composing sub-handler groups

**Phase 4d (day 5):** Store interface narrowing
- Services accept specific sub-interfaces (`database.ConversationStore`) instead of `database.Store`
- The composite `Store` remains for adapter implementation
- No adapter changes needed — `postgres.Store` already implements all sub-interfaces

**Verification:** After each phase: `go test ./internal/service/... ./internal/adapter/http/...` — zero regressions. Method count per type < 20.

---

## Phase 5 — Coverage & Docs (parallel: WT-11 + WT-12)

### WT-11: `test/coverage-sprint` — ~5 days

**Findings:** F-QUA-011 (HIGH), F-QUA-012 (MED)

**Approach:** Unit tests with testify mocks for service layer, integration tests with testcontainers for adapter layer, `@solidjs/testing-library` + Vitest for frontend.

**Key Sources:**
- [Testcontainers Postgres Module](https://golang.testcontainers.org/modules/postgres/) — Pre-built container with migrations
- [SolidJS Testing Guide](https://docs.solidjs.com/guides/testing) — `render(() => <Component />)` pattern
- [pytest-asyncio + AsyncMock](https://tonybaloney.github.io/posts/async-test-patterns-for-pytest-and-unittest.html)

**Priority order:**
1. **Go critical:** `auth_token.go`, `gdpr.go`, `runtime_approval.go`, `auth_apikey.go`, `retention.go`
2. **Go core:** `experience_pool.go`, `graph.go`, `lsp.go`, `mcp_db.go`, `skill.go`
3. **Frontend:** UI primitives, ChatPanel, ProjectDetailPage (target: 6% → 20%+)
4. **Python:** `executor.py`, `secrets.py` via `pytest-asyncio` + `AsyncMock`

---

### WT-12: `docs/audit-followup` — ~3 days

**Findings:** F-COM-009 (HIGH), F-COM-002 (LOW), F-COM-010 (LOW), F-COM-011 (LOW)

**Approach:** `swaggo/swag` for OpenAPI, FSFE `reuse` for SPDX headers, Keep a Changelog for CHANGELOG, MADR 4.0 for ADR-009.

**Key Sources:**
- [swaggo/swag](https://github.com/swaggo/swag) — Annotation-based OpenAPI generation for Go+chi
- [REUSE Specification v3.3](https://reuse.software/spec-3.3/) — SPDX compliance tooling
- [MADR 4.0](https://adr.github.io/madr/) — Markdown ADR template

**Steps:**
1. **OpenAPI:** Add swaggo annotations to top 50 endpoints (auth, projects, conversations, runs). Generate spec via `swag init`. Add `kin-openapi` validation to CI.
2. **SPDX:** `reuse annotate --recursive --copyright "CodeForge Contributors" --license "AGPL-3.0-or-later"` across all source dirs. Add `reuse lint` to pre-commit.
3. **CHANGELOG:** Add `[Unreleased]` section with all post-0.8.0 changes (GDPR, refactoring, security fixes, CI hardening).
4. **ADR-009:** GDPR architecture decisions — cascade deletion strategy, retention batching, consent management, export format versioning.

---

## Deferred (pragmatic accept)

| Finding | Reason |
|---|---|
| F-ARC-008 (MED) | Domain `json` tags — standard Go pragmatism, 69 files, no ROI until API v2 |
| F-ARC-009 (LOW) | Port-layer concrete types — minimal impact |
| F-QUA-015 (LOW) | LSP `map[string]any` — justified for JSON-RPC interop |
| F-QUA-017 (LOW) | `any` in BackendConfigField — edge case |
| F-SEC-011 (INFO) | Version exposed — debatable, no real risk |
| F-ARC-015 (INFO) | Good practices — no fix needed |

---

## Execution Timeline

```
Week 1:  Phase 1 (WT-1 + WT-2 + WT-3)    — security-critical, ship ASAP
Week 1:  Phase 2 (WT-4 + WT-5 + WT-8)    — data safety, start parallel
Week 2:  Phase 3 (WT-6 + WT-7 + WT-10)   — refactoring
Week 3:  Phase 4 (WT-9)                    — god object decomposition (needs WT-7)
Week 3+: Phase 5 (WT-11 + WT-12)          — coverage + docs (ongoing)
```

## Source Bibliography

### Standards & Frameworks
- OWASP Top 10 2021/2025: [A01 Broken Access Control](https://owasp.org/Top10/2021/A01_2021-Broken_Access_Control/), [API4:2023](https://owasp.org/API-Security/editions/2023/en/0xa4-unrestricted-resource-consumption/), [SSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)
- CWE: [22 (Path Traversal)](https://cwe.mitre.org/data/definitions/22.html), [287 (Auth)](https://cwe.mitre.org/data/definitions/287.html), [798 (Hardcoded Creds)](https://cwe.mitre.org/data/definitions/798.html), [918 (SSRF)](https://cwe.mitre.org/data/definitions/918.html)
- GDPR: [Art. 6](https://gdpr-info.eu/art-6-gdpr/), [Art. 17](https://gdpr-info.eu/art-17-gdpr/), [Art. 20](https://gdpr-info.eu/art-20-gdpr/)
- [SLSA Framework](https://github.blog/security/supply-chain-security/slsa-3-compliance-with-github-actions/)
- [WCAG 2.1](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/)

### GitHub References
- [go-chi/chi router-walk](https://github.com/go-chi/chi/blob/master/_examples/router-walk/main.go)
- [Doyensec safeurl](https://github.com/doyensec/safeurl)
- [tldraw WCAG #5215](https://github.com/tldraw/tldraw/issues/5215)
- [LangGraph ReAct Agent](https://github.com/langchain-ai/react-agent)
- [FSFE reuse-tool](https://github.com/fsfe/reuse-tool)
- [swaggo/swag](https://github.com/swaggo/swag)
- [common-fate/tenancy](https://github.com/common-fate/tenancy)
- [Testcontainers Go](https://golang.testcontainers.org/modules/postgres/)
- [Solid Testing Library](https://github.com/solidjs/solid-testing-library)

### Academic & Industry
- [OpenHands Agent SDK V1 (arXiv:2511.03690)](https://arxiv.org/abs/2511.03690)
- [McCabe Cyclomatic Complexity NIST235](https://en.wikipedia.org/wiki/Cyclomatic_complexity)
- [Type Safety in Python (Authorea)](https://www.authorea.com/users/918281/articles/1290813/)
- [PyTy: Repairing Static Type Errors (UMass)](https://people.cs.umass.edu/~brun/class/2024Fall/CS692P/pyty.pdf)
- [Martin Fowler: Refactoring Large Classes](https://martinfowler.com/articles/class-too-large.html)
- [Dave Cheney: SOLID Go Design](https://dave.cheney.net/2016/08/20/solid-go-design)
- [Shopify: Strangler Fig Pattern](https://shopify.engineering/refactoring-legacy-code-strangler-fig-pattern)

### Production Patterns
- [Gitea Server Startup](https://deepwiki.com/go-gitea/gitea/8.2-server-startup-and-initialization)
- [Citus: Five Ways to Paginate](https://www.citusdata.com/blog/2016/03/30/five-ways-to-paginate/)
- [Arcjet: slog PII Redaction](https://blog.arcjet.com/redacting-sensitive-data-from-logs-with-go-log-slog/)
- [Figma Keyboard Accessibility](https://www.figma.com/blog/introducing-keyboard-accessibility-features/)
