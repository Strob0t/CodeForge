# Universal Audit Report — 2026-03-27

**Date:** 2026-03-27
**Auditor:** Claude Opus 4.6 (Universal Audit Prompt)
**Branch:** `staging`
**Scope:** Full monorepo — Go core, Python workers, TypeScript frontend, PostgreSQL migrations
**Excluded:** `.env` files, `docker-compose` files (filtered by request)

---

## Audit Summary

| Metric               | Value                                                     |
|-----------------------|-----------------------------------------------------------|
| Target                | `/workspaces/CodeForge`                                   |
| Input Type            | Mixed / Monorepo (Go + Python + TypeScript + SQL + Infra) |
| Files Analyzed        | ~1,516 source + 88 SQL + 30 config/infra                  |
| Total Findings        | 45                                                        |
| Critical              | 3                                                         |
| High                  | 8                                                         |
| Medium                | 20                                                        |
| Low                   | 10                                                        |
| Informational         | 5                                                         |
| Overall Risk Score    | 31/100                                                    |

---

## Findings

### CRITICAL

> **[F-001] Hardcoded JWT Secret in `codeforge.yaml`**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | CRITICAL                                                                     |
> | Dimension      | Security + Infrastructure                                                    |
> | Location       | `codeforge.yaml:102`                                                         |
> | Evidence       | `jwt_secret: "e2e-test-secret-key-minimum-32-bytes-long"` — predictable test string passes entropy validation |
> | Risk           | Attacker who discovers this value can forge arbitrary JWT tokens with any role/tenant, achieving full admin access |
> | Remediation    | Remove hardcoded value; add this string to the blocked defaults list in `internal/config/loader.go:399`; use env var `CODEFORGE_AUTH_JWT_SECRET` |
> | Reference      | CWE-321, OWASP A02:2021                                                     |
> | Confidence     | high                                                                         |

> **[F-002] Duplicate NATS Subscription: Trajectory Events Processed Twice**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | CRITICAL                                                                     |
> | Dimension      | Architecture + Code Quality                                                  |
> | Location       | `internal/service/runtime.go:574` and `:591`                                 |
> | Evidence       | Two subscriptions for `SubjectTrajectoryEvent`: one via the subs table calling `s.handleTrajectoryEvent`, and a 174-line inline handler. Both call `s.events.Append` |
> | Risk           | Every trajectory event is persisted to DB twice, doubles WS broadcasts, doubles AG-UI notifications. The inline handler has 2 event types (`roadmap_proposed`, `subagent_requested`) NOT in the extracted handler — removing the duplicate without migrating these will silently break roadmap proposals and sub-agent spawning |
> | Remediation    | Remove inline handler (lines 591-763). Add `roadmap_proposed` and `subagent_requested` cases to `handleTrajectoryEvent` in `runtime_subscribers.go` |
> | Reference      | DRY, SRP                                                                     |
> | Confidence     | high                                                                         |

> **[F-003] Hardcoded LiteLLM Master Key in Config**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | CRITICAL                                                                     |
> | Dimension      | Security                                                                     |
> | Location       | `codeforge.yaml:22`                                                          |
> | Evidence       | `master_key: "sk-codeforge-dev"` — predictable default credential            |
> | Risk           | Known-default credential gates access to all LLM provider proxying. Anyone who discovers this can make API calls through LiteLLM consuming provider credits |
> | Remediation    | Set `master_key: ""` in `codeforge.yaml` and require it via `LITELLM_MASTER_KEY` env var |
> | Reference      | CWE-798                                                                      |
> | Confidence     | high                                                                         |

---

### HIGH

> **[F-004] JetStream Stream Has No Retention Limits**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | HIGH                                                                         |
> | Dimension      | Infrastructure                                                               |
> | Location       | `internal/adapter/nats/nats.go:74-78`                                        |
> | Evidence       | `StreamConfig` has no `MaxAge`, `MaxBytes`, `MaxMsgs` — unlimited retention  |
> | Risk           | Messages accumulate indefinitely, exhausting disk, crashing NATS, cascading failures across Go Core and Python workers |
> | Remediation    | Add `MaxAge: 7*24*time.Hour`, `MaxBytes: 1<<30`, `Retention: jetstream.LimitsPolicy` |
> | Reference      | NATS JetStream Operations Guide                                              |
> | Confidence     | high                                                                         |

> **[F-005] Business Logic in HTTP Handler: AllowAlwaysPolicy**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | HIGH                                                                         |
> | Dimension      | Architecture                                                                 |
> | Location       | `internal/adapter/http/handlers_policy_crud.go:126-214`                      |
> | Evidence       | 89 lines of business logic (clone profile, construct rule, prepend, filesystem persist) in HTTP handler instead of service layer |
> | Risk           | Violates hexagonal architecture — handler should delegate to `PolicyService` |
> | Remediation    | Extract `AllowAlways(ctx, projectID, tool, command)` method on `PolicyService` |
> | Reference      | Hexagonal Architecture — adapters should be thin                             |
> | Confidence     | high                                                                         |

> **[F-006] God Object: database.Store Interface (278 methods)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | HIGH                                                                         |
> | Dimension      | Architecture                                                                 |
> | Location       | `internal/port/database/store.go`                                            |
> | Evidence       | 34 embedded interfaces composing ~278 methods; every service gets access to all |
> | Risk           | Violates Interface Segregation — impossible to understand which data ops a service actually needs |
> | Remediation    | Services should accept specific sub-interfaces (e.g., `ProjectStore`, `RunStore`) instead of composite `Store` |
> | Reference      | Interface Segregation Principle                                              |
> | Confidence     | high                                                                         |

> **[F-007] Swallowed Database Errors in Dashboard Store (6 occurrences)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | HIGH                                                                         |
> | Dimension      | Code Quality                                                                 |
> | Location       | `internal/adapter/postgres/store_dashboard.go:85-91,131-154`                 |
> | Evidence       | `_ = s.pool.QueryRow(...)` — errors silently discarded, returns zero-valued metrics with HTTP 200 |
> | Risk           | Infrastructure failures invisible; dashboard shows incorrect data silently   |
> | Remediation    | Use `logBestEffort` pattern (already in `internal/service/log_best_effort.go`) or return errors |
> | Reference      | CLAUDE.md "Errors should never pass silently"                                |
> | Confidence     | high                                                                         |

> **[F-008] GDPR Service Has Zero Tests (151 LOC, Compliance-Critical)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | HIGH                                                                         |
> | Dimension      | Code Quality + Compliance                                                    |
> | Location       | `internal/service/gdpr.go`                                                   |
> | Evidence       | No `gdpr_test.go` exists. Handles GDPR Art. 17 (erasure) and Art. 20 (portability) |
> | Risk           | Untested deletion may leave orphaned PII; regulatory fines up to 4% annual turnover |
> | Remediation    | Create `gdpr_test.go` with table-driven tests for export completeness, partial failures, deletion cascade |
> | Reference      | GDPR Art. 17, 20; CLAUDE.md TDD                                             |
> | Confidence     | high                                                                         |

> **[F-009] Auth Token Manager Has Zero Tests (251 LOC, Security-Critical)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | HIGH                                                                         |
> | Dimension      | Code Quality                                                                 |
> | Location       | `internal/service/auth_token.go`                                             |
> | Evidence       | No `auth_token_test.go`. Implements JWT signing, refresh rotation, revocation, HMAC-SHA256 |
> | Risk           | Token rotation bugs could allow session fixation; HMAC bugs could allow token forgery |
> | Remediation    | Create `auth_token_test.go` covering refresh, expiration, rotation, revocation, concurrent race, HMAC verification |
> | Reference      | OWASP Session Management; CLAUDE.md TDD                                     |
> | Confidence     | high                                                                         |

> **[F-010] AGPL Section 13: No Source Code Offer in Web UI**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | HIGH                                                                         |
> | Dimension      | Compliance                                                                   |
> | Location       | Project-wide (no source link anywhere in frontend)                           |
> | Evidence       | Project licensed AGPL-3.0. No "Source Code" link in sidebar, footer, or About page |
> | Risk           | AGPL non-compliance for any deployment modifying the source                  |
> | Remediation    | Add visible source code link in sidebar footer or About page                 |
> | Reference      | AGPL-3.0 Section 13                                                         |
> | Confidence     | high                                                                         |

> **[F-011] Traefik Security Middlewares Defined But Never Applied**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | HIGH                                                                         |
> | Dimension      | Infrastructure                                                               |
> | Location       | `traefik/dynamic/middleware.yaml:1-17`                                       |
> | Evidence       | `rate-limit` and `security-headers` middlewares defined but no router references them |
> | Risk           | HSTS, X-Frame-Options, rate limiting never applied at proxy layer            |
> | Remediation    | Add `middlewares=rate-limit@file,security-headers@file` to router labels     |
> | Reference      | OWASP Security Headers                                                       |
> | Confidence     | high                                                                         |

---

### MEDIUM

> **[F-012] Weak JWT Secret Passes Validation**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Security                                                                     |
> | Location       | `codeforge.yaml:102`, `internal/config/loader.go:399`                        |
> | Evidence       | The string `e2e-test-secret-key-minimum-32-bytes-long` passes the 32-char minimum and entropy checks. Only the original default `codeforge-dev-jwt-secret-change-in-production` is blocked |
> | Risk           | If config is accidentally used in staging/production, JWT tokens can be forged |
> | Remediation    | Maintain a blocklist of known test secrets in `loader.go`                    |
> | Reference      | CWE-321                                                                      |
> | Confidence     | medium                                                                       |

> **[F-013] PostgreSQL `sslmode=disable` in Non-Production Config**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Security                                                                     |
> | Location       | `codeforge.yaml:10`                                                          |
> | Evidence       | `sslmode=disable` overrides the default `sslmode=prefer`. Production rejects this, but staging does not |
> | Risk           | Credentials and data transmitted in cleartext in shared environments         |
> | Remediation    | Extend `sslmode=disable` rejection to staging environments                   |
> | Reference      | CWE-319                                                                      |
> | Confidence     | medium                                                                       |

> **[F-014] Bash Tool Blocklist Trivially Bypassable**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Security                                                                     |
> | Location       | `workers/codeforge/tools/bash.py:67-101`                                     |
> | Evidence       | Simple string-contains matching. Bypassable via flag reordering (`rm -r -f /`), long flags (`--recursive --force`), base64 encoding, or interpreter wrapping (`perl -e 'system(...)'`) |
> | Risk           | LLM-directed agent could craft commands bypassing the blocklist              |
> | Remediation    | Document as secondary defense only. Consider command parser or sandboxing (seccomp, namespaces). Go policy engine is the real defense |
> | Reference      | CWE-184                                                                      |
> | Confidence     | high                                                                         |

> **[F-015] HTTP Handler Performs Direct Filesystem I/O**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Architecture                                                                 |
> | Location       | `internal/adapter/http/handlers_goals.go:146`, `handlers_policy_crud.go:86,115,203` |
> | Evidence       | `os.ReadFile(docPath)`, `os.MkdirAll(...)`, `os.Remove(...)` called directly in HTTP handlers |
> | Risk           | Breaks Ports & Adapters pattern; makes handlers harder to unit test          |
> | Remediation    | Move filesystem operations into respective services via `filesystem.Provider` port |
> | Reference      | Hexagonal Architecture                                                       |
> | Confidence     | high                                                                         |

> **[F-016] God Object: Handlers Struct (77 fields, 62 service dependencies)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Architecture                                                                 |
> | Location       | `internal/adapter/http/handlers.go:53-96`                                    |
> | Evidence       | 62 service dependencies aggregated. Any handler method has access to every service |
> | Risk           | Bloated composition root; violates SRP                                       |
> | Remediation    | Continue existing sub-handler pattern — move remaining methods into domain-specific sub-handler structs |
> | Reference      | Single Responsibility Principle                                              |
> | Confidence     | high                                                                         |

> **[F-017] God Objects: RuntimeService (54 methods), ConversationService (53 methods)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Architecture                                                                 |
> | Location       | `internal/service/runtime.go`, `conversation_agent.go`                       |
> | Evidence       | RuntimeService: 54 methods across 771+618+216 LOC. ConversationService: 53 methods across 934+ LOC |
> | Risk           | Difficult to understand, test, and maintain. Too many responsibilities       |
> | Remediation    | Extract into focused sub-services with narrow interfaces. Continue existing decomposition pattern (runtime_execution.go, runtime_lifecycle.go) |
> | Reference      | Single Responsibility Principle                                              |
> | Confidence     | high                                                                         |

> **[F-018] Missing `json:"-"` on 10 Sensitive Config Fields**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Architecture                                                                 |
> | Location       | `internal/config/config.go` (lines 75, 164, 200-202, 213, 225, 360, 407, 424) |
> | Evidence       | `InternalKey`, `APIToken`, `GitHubSecret`, `GitLabToken`, `PlaneSecret`, `SMTPPassword`, `ClientSecret`, `MasterKey`, `APIKeys`, `APIKey` all lack `json:"-"`. `JWTSecret` and `LLMKeyEncryptionSecret` correctly have it |
> | Risk           | Any future `json.Marshal` on Config or containing structs leaks secrets      |
> | Remediation    | Add `json:"-"` to all 10 fields                                             |
> | Reference      | Defense-in-depth, OWASP Sensitive Data Exposure                              |
> | Confidence     | high                                                                         |

> **[F-019] Duplicate Dispatch Logic: SendMessage vs dispatchAgenticRun**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Code Quality                                                                 |
> | Location       | `internal/service/conversation.go:256-323`, `conversation_agent.go:431-565`  |
> | Evidence       | Both methods share ~40 lines of identical steps: store message, load history, build prompt, resolve model, marshal, broadcast, publish to NATS |
> | Risk           | Bug fixes applied to one path may not be applied to the other                |
> | Remediation    | Extract common logic into `buildAndPublishRun` method                        |
> | Reference      | DRY                                                                          |
> | Confidence     | high                                                                         |

> **[F-020] Dead Code: Discarded Model Resolution**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Code Quality                                                                 |
> | Location       | `internal/service/conversation_agent.go:612-613`                             |
> | Evidence       | `model, _, modeAutonomy, _ := s.resolveModelAndMode(...)` then `_ = model`. Expensive call with discarded error return |
> | Risk           | Wasted computation; silently ignored mode resolution failures                |
> | Remediation    | Use dedicated `resolveAutonomyLevel()` or check the error return             |
> | Reference      | CLAUDE.md "Errors should never pass silently"                                |
> | Confidence     | high                                                                         |

> **[F-021] Type Safety: `object` Used as Parameter Type in Python (5 occurrences)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Code Quality                                                                 |
> | Location       | `workers/codeforge/consumer/_conversation.py:416,418,472,474,541`            |
> | Evidence       | `routing: object`, `registry: object` — untyped parameters                   |
> | Risk           | No IDE autocompletion, no static type checking, runtime AttributeError       |
> | Remediation    | Define `Protocol` classes with required methods (PEP 544)                    |
> | Reference      | CLAUDE.md type safety policy                                                 |
> | Confidence     | high                                                                         |

> **[F-022] Deep Nesting in Context Optimizer (8 levels)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Code Quality                                                                 |
> | Location       | `internal/service/context_optimizer.go:495-572`                              |
> | Evidence       | `fetchKnowledgeBaseEntries` reaches nesting depth 8: func->for->for->if->if->if->if->for |
> | Risk           | Extremely hard to read, maintain, and debug                                  |
> | Remediation    | Extract inner KB processing into `processKnowledgeBase()` to flatten by 3-4 levels |
> | Reference      | CLAUDE.md "Flat is better than nested"                                       |
> | Confidence     | high                                                                         |

> **[F-023] Frontend Test Coverage: 43 Test Files for 290 Source Files (15%)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Code Quality                                                                 |
> | Location       | `frontend/src/`                                                              |
> | Evidence       | 15 major components (200+ LOC each) have zero tests: ProjectDetailPage (986), FilePanel (934), PlanPanel (795), PolicyPanel (667), RoadmapPanel (647), ChatPanel (537) |
> | Risk           | UI regressions go undetected in complex state management                     |
> | Remediation    | Prioritize tests for ChatPanel, PolicyPanel, ProjectDetailPage               |
> | Reference      | CLAUDE.md TDD requirement                                                    |
> | Confidence     | high                                                                         |

> **[F-024] Go Service Layer: 32 Service Files Without Tests**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Code Quality                                                                 |
> | Location       | `internal/service/` (32 of ~102 production files)                            |
> | Evidence       | Notable untested: `conversation_prompt.go` (351), `lsp.go` (340), `files.go` (332), `runtime_subscribers.go` (216), `runtime_approval.go` (142) |
> | Risk           | Regressions in `runtime_approval.go` could bypass HITL security controls     |
> | Remediation    | Start with `runtime_approval.go` (security-critical) and `runtime_subscribers.go` |
> | Reference      | CLAUDE.md TDD requirement                                                    |
> | Confidence     | high                                                                         |

> **[F-025] Audit Log Stores Plaintext Email (PII)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Compliance                                                                   |
> | Location       | `internal/adapter/postgres/migrations/087_create_audit_log.sql:6`            |
> | Evidence       | `admin_email TEXT NOT NULL` — PII stored in plaintext; `admin_id` UUID would suffice |
> | Risk           | Conflicts with GDPR Art. 5(1)(c) data minimization                          |
> | Remediation    | Remove `admin_email` column, use `admin_id` joined to `users` for display    |
> | Reference      | GDPR Art. 5(1)(c)                                                            |
> | Confidence     | high                                                                         |

> **[F-026] IP Addresses Stored Without Retention Controls**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Compliance                                                                   |
> | Location       | `internal/adapter/postgres/migrations/087_create_audit_log.sql:10`           |
> | Evidence       | `ip_address INET` with default 730-day retention. No documented legal basis  |
> | Risk           | Storing IP addresses (personal data per CJEU Breyer) for 2 years may exceed necessity |
> | Remediation    | Document legal basis for IP retention; consider anonymizing after 90 days    |
> | Reference      | GDPR Art. 5(1)(e), CJEU C-582/14                                            |
> | Confidence     | medium                                                                       |

> **[F-027] GDPR Deletion Does Not Anonymize Audit Log Entries**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Compliance                                                                   |
> | Location       | `internal/service/gdpr.go:145-151`                                           |
> | Evidence       | `DeleteUserData` calls `DeleteUser` via FK cascades. Audit log has no FK to users — `admin_email` persists after deletion. ADR-009 specifies anonymization but implementation does not perform it |
> | Risk           | Deleted user's email persists in audit_log — violates GDPR Art. 17 and contradicts ADR-009 |
> | Remediation    | Before user deletion: `UPDATE audit_log SET admin_email = '[deleted]' WHERE admin_id = $1` |
> | Reference      | GDPR Art. 17, ADR-009                                                        |
> | Confidence     | high                                                                         |

> **[F-028] No Consent Tracking or Privacy Policy Reference**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Compliance                                                                   |
> | Location       | Project-wide                                                                 |
> | Evidence       | Zero results for "consent", "privacy policy", "data processing" across all source |
> | Risk           | No GDPR Art. 6 legal basis infrastructure; no Art. 13 data subject information |
> | Remediation    | Add privacy policy link in login footer; document recommended legal basis in deployment guide |
> | Reference      | GDPR Art. 6, Art. 13                                                         |
> | Confidence     | medium                                                                       |

> **[F-029] No Data Breach Notification Procedure**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Compliance                                                                   |
> | Location       | Project-wide                                                                 |
> | Evidence       | No incident response procedure, notification template, or breach detection   |
> | Risk           | GDPR Art. 33 requires 72-hour notification to supervisory authorities        |
> | Remediation    | Add breach notification procedure document to `docs/`; consider admin notification for security events |
> | Reference      | GDPR Art. 33, Art. 34                                                        |
> | Confidence     | high                                                                         |

> **[F-030] Audit Log Missing Coverage for Data-Modifying Operations**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Compliance                                                                   |
> | Location       | `internal/adapter/http/routes.go`                                            |
> | Evidence       | Only 12 route registrations use `audit()` middleware. Missing: project CRUD, roadmap CRUD, knowledge base CRUD, MCP server CRUD, LLM key CRUD, file operations |
> | Risk           | No accountability trail for most system changes (SOC 2 CC6.1)               |
> | Remediation    | Extend `audit()` to all write endpoints, prioritizing project deletion, file modifications, LLM key management |
> | Reference      | SOC 2 CC6.1, CC7.2                                                           |
> | Confidence     | high                                                                         |

> **[F-031] Prometheus Alerts Config Minimal (3 rules only)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | MEDIUM                                                                       |
> | Dimension      | Infrastructure                                                               |
> | Location       | `configs/prometheus/alerts.yml:1-27`                                         |
> | Evidence       | Only 3 alerts: NATSConsumerLag, HighMemoryUsage, HealthCheckFailing. No alerts for: API error rate, latency, LLM cost anomalies, PostgreSQL pool exhaustion, DLQ growth, disk, cert expiry, auth failures |
> | Risk           | Critical production incidents go undetected until users report them          |
> | Remediation    | Add alerts for HTTP error rate, API latency P95, DLQ growth, PG connections, disk, cert expiry |
> | Reference      | Google SRE Book Ch. 6                                                        |
> | Confidence     | high                                                                         |

---

### LOW

> **[F-032] WebSocket Token in URL Query String (Accepted Risk)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Security                                                                     |
> | Location       | `internal/middleware/auth.go:98-139`                                         |
> | Evidence       | Token in `?token=` query param — documented as accepted risk with mitigations (short TTL, HTTPS, single-send) |
> | Risk           | Token appears in server access logs, browser history, proxy logs             |
> | Remediation    | Consider single-use ticket exchange pattern for WS upgrade                   |
> | Reference      | CWE-598                                                                      |
> | Confidence     | high                                                                         |

> **[F-033] CORS Origin Reflects Config Without Validating Request**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Security                                                                     |
> | Location       | `internal/adapter/http/middleware.go:44-71`                                  |
> | Evidence       | `Access-Control-Allow-Origin` set to configured value without checking incoming `Origin` header |
> | Risk           | Low — default is `http://localhost:3000`, production should set specific origin |
> | Remediation    | Validate incoming `Origin` against configured allowed origin before reflecting |
> | Reference      | CWE-942                                                                      |
> | Confidence     | medium                                                                       |

> **[F-034] Outdated `golang.org/x/crypto` and `pgx`**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Security                                                                     |
> | Location       | `go.mod`                                                                     |
> | Evidence       | `x/crypto` v0.48.0 (latest v0.49.0), `pgx/v5` v5.8.0 (latest v5.9.1)       |
> | Risk           | Potential unpatched vulnerabilities in crypto and DB driver                   |
> | Remediation    | Update and add `govulncheck` to CI                                           |
> | Reference      | CWE-1104                                                                     |
> | Confidence     | low                                                                          |

> **[F-035] Service Layer Imports `internal/config` (19 files)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Architecture                                                                 |
> | Location       | 19 service files import `internal/config`                                    |
> | Evidence       | Services receive config sub-structs via pointers — pragmatic trade-off       |
> | Risk           | Borderline hexagonal violation, but config contains only value types (no I/O) |
> | Remediation    | Acceptable as-is given current clean usage                                   |
> | Reference      | Hexagonal Architecture                                                       |
> | Confidence     | medium                                                                       |

> **[F-036] Frontend Bypasses Centralized API Client (4 call sites)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Architecture                                                                 |
> | Location       | `frontend/src/features/chat/commandStore.ts:27`, `commandExecutor.ts:47`, `project/RefactorApproval.tsx:45,61` |
> | Evidence       | Direct `fetch()` calls bypass `api/client.ts` with its auth handling and error formatting |
> | Risk           | Inconsistency in auth token handling and error formatting                    |
> | Remediation    | Route through existing API client by adding resource factories               |
> | Reference      | DRY, Single point of entry                                                   |
> | Confidence     | high                                                                         |

> **[F-037] Python `models.py` Monolith (52 classes, 724 lines)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Architecture                                                                 |
> | Location       | `workers/codeforge/models.py`                                                |
> | Evidence       | 52 Pydantic model classes in a single file covering all NATS payload types   |
> | Risk           | Increasingly hard to navigate as project grows                               |
> | Remediation    | Split into `models/run.py`, `models/benchmark.py`, etc.; re-export from `__init__.py` |
> | Reference      | SRP, Python packaging                                                        |
> | Confidence     | medium                                                                       |

> **[F-038] Inconsistent JSON Decode Pattern in HTTP Handlers**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Code Quality                                                                 |
> | Location       | `internal/adapter/http/handlers_agent_features.go:110,225`                   |
> | Evidence       | Raw `json.NewDecoder().Decode()` instead of `readJSON[T]()` helper. Line 110 lacks `MaxBytesReader` |
> | Risk           | Missing body size limit on `StartLSP` handler; inconsistency                 |
> | Remediation    | Convert to `readJSON[T]()` or wrap with `MaxBytesReader`                     |
> | Reference      | CWE-400                                                                      |
> | Confidence     | medium                                                                       |

> **[F-039] `map[string]any` in Go Production Code (15+ locations)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Code Quality                                                                 |
> | Location       | `handlers_agent_features.go:78`, `handlers_backend_health.go:27`, `handlers_llm.go:103`, `handlers_roadmap.go:458`, `handlers_routing.go:69`, `handlers_scope.go:144,185`, `adapter/litellm/client.go:172-173,318,340` |
> | Evidence       | `writeJSON(w, http.StatusOK, map[string]any{...})` for ad-hoc responses      |
> | Risk           | No compile-time checking of response shape. Violates CLAUDE.md type safety   |
> | Remediation    | Define response structs per endpoint                                         |
> | Reference      | CLAUDE.md "No `any`/`interface{}`"                                           |
> | Confidence     | high                                                                         |

> **[F-040] PostgreSQL Backup Script Lacks Encryption**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Infrastructure                                                               |
> | Location       | `scripts/backup-postgres.sh:23-27`                                           |
> | Evidence       | `pg_dump --format=custom --compress=6` — compressed but not encrypted        |
> | Risk           | Backups on shared storage expose all data including credential hashes        |
> | Remediation    | Add GPG encryption: `pg_dump ... | gpg --symmetric --batch --passphrase-file "$KEY_FILE"` |
> | Reference      | CIS PostgreSQL 8.2                                                           |
> | Confidence     | high                                                                         |

> **[F-041] Multiple Forms Lack Proper Label Associations**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | LOW                                                                          |
> | Dimension      | Compliance                                                                   |
> | Location       | `frontend/src/features/dashboard/ProjectCard.tsx:40-44`, `project/TrajectoryPanel.tsx:398` |
> | Evidence       | Checkbox in batch mode has no label or `aria-label`                          |
> | Risk           | Screen readers cannot identify unlabeled controls                            |
> | Remediation    | Add `aria-label` attributes to all unlabeled inputs                          |
> | Reference      | WCAG 2.1 SC 1.3.1, SC 4.1.2                                                 |
> | Confidence     | high                                                                         |

---

### INFO

> **[F-042] CHANGELOG Only Covers v0.8.0**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | INFO                                                                         |
> | Dimension      | Compliance                                                                   |
> | Location       | `CHANGELOG.md`                                                               |
> | Evidence       | Only `[Unreleased]` and `[0.8.0]` entries. No history for 0.1.0-0.7.x       |
> | Remediation    | Retroactively populate major version entries for security-relevant changes    |
> | Reference      | SOC 2 CC8.1                                                                  |
> | Confidence     | high                                                                         |

> **[F-043] Missing ADRs for 7+ Architectural Decisions**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | INFO                                                                         |
> | Dimension      | Compliance                                                                   |
> | Location       | `docs/architecture/adr/`                                                     |
> | Evidence       | 9 ADRs exist (001-009). Missing: AG-UI, A2A, trust/quarantine, MCP, hybrid routing, contract-first review, visual canvas |
> | Remediation    | Create ADRs for A2A, trust/quarantine, and hybrid routing at minimum         |
> | Reference      | SOC 2 CC8.1                                                                  |
> | Confidence     | medium                                                                       |

> **[F-044] OpenAPI Spec Covers 3% of API Surface**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | INFO                                                                         |
> | Dimension      | Compliance                                                                   |
> | Location       | `docs/api/openapi.yaml`                                                      |
> | Evidence       | Self-described "foundation stub" with ~7 endpoints. Actual routes.go has 263 handlers |
> | Remediation    | Generate spec from routes.go or maintain complete spec                       |
> | Reference      | SOC 2 CC7.1, CC8.1                                                           |
> | Confidence     | high                                                                         |

> **[F-045] Large Frontend Component: ProjectDetailPage (986 LOC)**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | INFO                                                                         |
> | Dimension      | Code Quality                                                                 |
> | Location       | `frontend/src/features/project/ProjectDetailPage.tsx`                        |
> | Evidence       | Orchestrates 30+ panels with embedded PanelSelector component               |
> | Remediation    | Extract PanelSelector; consider panel registry pattern                       |
> | Reference      | SRP                                                                          |
> | Confidence     | low                                                                          |

> **[F-046] Repeated Pydantic Validator Patterns**
> | Field          | Value                                                                        |
> |----------------|------------------------------------------------------------------------------|
> | Severity       | INFO                                                                         |
> | Dimension      | Code Quality                                                                 |
> | Location       | `workers/codeforge/models.py:126,493,252,299`                                |
> | Evidence       | `_coerce_list_fields` duplicated in 2 models; `_clamp_top_k` duplicated in 2 models |
> | Remediation    | Acceptable as-is given Pydantic per-model validator constraints              |
> | Reference      | DRY (mitigated by framework)                                                 |
> | Confidence     | medium                                                                       |

---

## Risk Heatmap

| Dimension          | Score /100 | Weight | Weighted | Top Issue                                      |
|--------------------|-----------|--------|----------|-------------------------------------------------|
| Security           | 25        | 30%    | 7.5      | Bash tool blocklist bypassable (F-014)          |
| Code Quality       | 40        | 25%    | 10.0     | Duplicate NATS subscription (F-002)             |
| Architecture       | 30        | 20%    | 6.0      | 278-method god interface (F-006)                |
| Infrastructure     | 25        | 15%    | 3.75     | JetStream no retention limits (F-004)           |
| Compliance         | 35        | 10%    | 3.5      | GDPR deletion doesn't anonymize audit log (F-027) |
| **Weighted Total** |           |        | **30.75** | **Duplicate NATS subscription + hardcoded secrets** |

*Score = risk level (lower = better). 0 = no issues, 100 = critical systemic failures.*

---

## Top 3 Priorities

1. **F-002 — Fix duplicate NATS trajectory subscription.** Remove inline handler `runtime.go:591-763`, migrate `roadmap_proposed` + `subagent_requested` to `runtime_subscribers.go`. Fixes double DB writes and double WS broadcasts.

2. **F-001 + F-003 — Remove hardcoded secrets from `codeforge.yaml`.** Replace JWT secret with auto-generated value, add `e2e-test-secret-key-minimum-32-bytes-long` to blocked defaults, clear LiteLLM `master_key`.

3. **F-008 + F-027 — Fix GDPR compliance gaps.** Anonymize `admin_email` in audit_log before user deletion (implement ADR-009's design), add test coverage for GDPR service.

---

## Positive Findings

The audit identified the following well-implemented security and quality controls:

### Security
- **SQL injection prevention:** All queries use parameterized placeholders via pgx/psycopg3
- **Path traversal protection:** Both Go and Python validate and contain file paths
- **XSS prevention:** Markdown renderer escapes HTML; link URLs protocol-whitelisted
- **Authentication:** JWT with HMAC-SHA256, bcrypt >= cost 10, account lockout, refresh rotation
- **Authorization:** `RequireRole` middleware consistently applied across routes
- **Security headers:** CSP, HSTS, X-Frame-Options: DENY, X-Content-Type-Options: nosniff
- **Secrets at rest:** LLM keys encrypted with AES-256-GCM/HKDF; API keys stored as SHA-256 hashes
- **Tenant isolation:** 481 occurrences of `tenant_id` filtering across 59 store files
- **SSRF protection:** Webhook URL validation checks private IPs, requires HTTPS
- **Rate limiting:** Per-IP with separate auth endpoint limiter
- **Error handling:** Generic messages in responses; internal errors logged server-side only
- **Log redaction:** RedactHandler strips API keys, tokens, emails from log output

### Architecture
- **Clean dependency direction:** No production imports from domain/port into adapter
- **Adapter-agnostic ports:** Zero adapter-specific types in port interfaces
- **Consumer-defined interfaces:** Narrow caller-side interfaces in deps files
- **No circular dependencies:** Verified across Go, Python, and TypeScript
- **Proper secret handling:** `User.PasswordHash` has `json:"-"`
- **Frontend WebSocket abstraction:** All components use `useWebSocket()` hook

### Compliance
- **GDPR self-service:** `/me/export` and `/me/data` endpoints implemented
- **Data retention service:** Configurable batched cleanup citing GDPR Art. 5(1)(e)
- **Password security:** Hashed passwords, excluded from list queries
- **License compatibility:** All dependencies use permissive licenses (MIT, BSD, Apache-2.0)
- **Frontend accessibility:** Skip-link, reduced-motion, semantic landmarks, focus-visible rings, ARIA on most elements
- **CI security:** All GitHub Actions pinned to full commit SHAs
- **Docker security:** Non-root users, cap_drop ALL, no-new-privileges, multi-stage builds, health checks

---

## Methodology

- **Phase 1 (Discovery):** Automated classification of input types via file pattern detection
- **Phase 2 (Audit):** 5 parallel specialized agents, one per dimension, analyzing actual source code with evidence-based findings
- **Phase 3 (Report):** Cross-agent deduplication and severity calibration
- **Exclusions:** `.env` files, `docker-compose` files (per user request); `.worktrees/`, `node_modules/`, `.venv/`, `.git/` directories
