# Audit Remediation Plan — Worktree Organization

> **Source:** `docs/audits/2026-03-22-universal-audit-report.md`
> **Deduplicated against:** `docs/plans/2026-03-22-stub-finder-remediation-plan.md`

---

## Removed Findings

### Ignored by Owner
| ID | Reason |
|---|---|
| S-001, S-002 | Top-3 Priority — owner marked unimportant |
| I-001, I-003, I-004 | Top-3 Priority — owner marked unimportant |
| I-011, I-012 | Top-3 Priority — owner marked unimportant |
| I-005 | Playwright-MCP — explicitly ignored |
| S-008 | Internal key default — explicitly ignored |
| I-024 | Ports on localhost — dev-only, accepted |

### Covered by Stub-Finder Plan
| ID | Covered by |
|---|---|
| Q-001 / A-005 | Worktree D: Handlers decomposition |
| Q-011 | Worktree B, Task B3: SVG icon centralization |
| C-008 | Worktree A, Task A2: ForceSecureCookies |
| Q-004 (partial) | Worktree C, Task C1: structlog migration adds logging to bare excepts |
| Q-013 | Worktree C, Task C1: structlog enforces consistent exception handling |

---

## Remaining Findings: 57 (organized into 7 Worktrees)

---

## Worktree F: `fix/go-security-hardening` (~2h)

Small, safe changes in Go middleware, handlers, and main.go. No API-breaking changes.

| ID | Sev | Title | File(s) | Fix |
|---|---|---|---|---|
| S-009 | HIGH | HSTS Header fehlt | `middleware.go:26-36` | 1 Zeile: `w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")` |
| S-004 | MED | CORS Wildcard bei falschem APP_ENV | `middleware.go:42-70` | Startup-Fail wenn CORS nicht gesetzt in production |
| S-007 | MED | Kein Rate-Limit auf Password-Reset | `handlers_auth.go` | Password-Reset in `authRL` Middleware-Gruppe ziehen |
| I-020 | MED | WebSocket nicht rate-limited | `main.go:819-820` | `/ws` Route in Rate-Limiter-Gruppe + Max-Concurrent pro User |
| I-022 | MED | Auth Rate-Limiter Anbindung pruefen | `main.go:834-837` | Audit + ggf. Fix der authRL-Wiring |
| I-016 | MED | HTTP Timeout-Defaults unklar | `main.go:906-913` | Defaults loggen, `MaxHeaderBytes: 8192` setzen |
| C-001 | MED | Email in Debug-Log bei Auth-Failure | `handlers_auth.go:44` | `"email", req.Email` aus slog.Debug entfernen |
| Q-007 | MED | Bare `return err` ohne Context (6 Stellen) | `git/pool.go:34`, `resilience/breaker.go:58`, 4 weitere | `fmt.Errorf("operation: %w", err)` |
| S-005 | LOW | SQL-Fehler koennten Schema in Logs leaken | `helpers.go:132-135` | Default-Case mit generischer Meldung absichern |

**9 Findings. Alles in Go core, keine Frontend/Python-Abhaengigkeit.**

---

## Worktree G: `refactor/hexagonal-port-interfaces` (~1d)

Architektur-Bereinigung: Fehlende Port-Layer-Abstraktionen erstellen, Service-Layer von Adapter-Imports entkoppeln.

| ID | Sev | Title | Betroffene Files | Fix |
|---|---|---|---|---|
| A-001 | MED | Service importiert LiteLLM Adapter | `service/meta_agent.go`, `service/routing.go`, `service/review_router.go`, `service/model_registry.go` | `port/llm/interface.go` erstellen, Services auf Interface umstellen |
| A-007 | MED | Fehlende Port-Layer LLM Abstraktion | Alle Services mit `adapter/litellm` Import | Gleicher Fix wie A-001 |
| A-010 | MED | Hexagonal-Verletzung (27 Service-Dateien) | `service/routing.go`, `service/review_router.go`, `service/model_registry.go` | Sukzessive Migration auf Port-Interfaces |
| A-002 | MED | Service importiert OTEL Adapter | `service/conversation.go`, `service/runtime.go` | `port/metrics/interface.go` erstellen |
| A-003 | MED | Service importiert Auth Adapter Typen | `service/subscription.go` | Types nach `port/subscription/` verschieben |
| A-004 | LOW | WS Adapter in 27 Services importiert | Mehrere Service-Files | Audit: nur `broadcast.Broadcaster` nutzen, direkte ws-Types eliminieren |
| A-008 | LOW | WS Import-Pattern unklar | 27 Service-Files | Gleicher Fix wie A-004 |

**7 Findings. Rein Go-intern, keine Config/Infra-Abhaengigkeit.**

---

## Worktree H: `fix/docker-infra-hardening` (~3h)

Docker, Nginx, Traefik Haertung. Keine Code-Aenderungen in Go/Python/TS.

| ID | Sev | Title | File(s) | Fix |
|---|---|---|---|---|
| I-002 | HIGH | Default-Credentials in Config | `.env.example`, `codeforge.yaml` | Defaults entfernen, `${VAR:?}` erzwingen |
| I-007 | HIGH | Unpinned Base Images | `Dockerfile.frontend`, `docker-compose.yml` | Alle auf spezifische Semver pinnen |
| I-010 | HIGH | PG archive_command ohne Error-Handling | `docker-compose.yml:76` | `mkdir -p` + Timeout hinzufuegen |
| I-013 | HIGH | Traefik ohne TLS | `traefik/traefik.yaml` | Let's Encrypt oder Static Cert konfigurieren |
| I-006 | MED | Kein cap_drop auf Containern | Alle docker-compose | `cap_drop: ["ALL"]` + selektive `cap_add` |
| I-008 | MED | Nginx ohne client_max_body_size | `frontend/nginx.conf` | `client_max_body_size 100M;` hinzufuegen |
| I-009 | MED | Nginx ohne proxy_buffering off | `frontend/nginx.conf:24-41` | `proxy_buffering off;` fuer WS und Streaming |
| I-014 | MED | Secrets potentiell in Docker-Logs | `docker-compose.yml` | Log-Retention erhoehen, Redaction dokumentieren |
| I-015 | MED | Env-Var-Validierung erst bei Container-Start | `docker-compose.prod.yml` | Pre-deploy Validierungsscript erstellen |
| I-017 | MED | Playwright --no-sandbox | `docker-compose.yml:40` | Entfernen oder in `profiles: [dev]` isolieren |
| I-018 | MED | Keine Security-Headers auf Traefik Routes | `docker-compose.blue-green.yml` | Traefik Header-Middleware konfigurieren |
| I-019 | LOW | Unpinned GitHub Action Versions | `ci.yml:66,150` | Auf spezifische Semver pinnen |

**12 Findings. Nur Config/Docker-Dateien, kein Applikationscode.**

---

## Worktree I: `fix/python-error-handling` (~1h)

Python Worker: Exception-Handling und Type-Safety.

| ID | Sev | Title | File(s) | Fix |
|---|---|---|---|---|
| Q-019 | MED | Geschluckte Exception in model_resolver | `model_resolver.py:69` | `except Exception as exc: logger.warning(...)` |
| Q-005 | MED | Fehlende Type-Hints auf Public Functions | `agent_loop.py` und weitere | Return-Types + Param-Types auf public API |
| Q-004 | MED | Bare except ohne Logging (verbleibend) | `context_reranker.py:80`, `history.py:208,364` | Logging + spezifische Exception-Types |

**3 Findings. Nur Python-Worker, keine Go/Frontend-Abhaengigkeit.**

---

## Worktree J: `fix/test-coverage-gaps` (~1d)

Test-Luecken schliessen: neue Test-Dateien, fehlende Error-Tests, Type-Safety in Tests.

| ID | Sev | Title | File(s) | Fix |
|---|---|---|---|---|
| Q-002 | HIGH | Oversized Service Files (1000+ LOC) | `conversation_agent.go`, `benchmark.go`, `store.go`, `agent_loop.py` | A-006 und Worktree D decken Decomposition ab; hier: Tests fuer bestehenden Code |
| Q-015 | MED | Fehlende Test-Dateien | `branchprotection.go`, `channel.go` | `branchprotection_test.go`, `channel_test.go` erstellen |
| Q-012 | MED | Fehlende Error-Tests Conversation | `conversation_test.go` | Table-driven Error-Tests: malformed NATS, policy failures, context cancellation |
| Q-008 | MED | Lueckenhafte Handler-Tests | `handlers_test.go` | Edge-Case-Tests fuer neuere Handler |
| Q-018 | MED | Frontend-Test-Coverage 13.5% | `frontend/src/` | Testplan + erste Unit-Tests fuer ChatPanel, FilePanel |
| Q-003 | MED | Type-unsichere map[string]any in Tests | `agui_events_test.go:25` | Strongly-typed Structs fuer Unmarshal |
| Q-017 | MED | Panics in Registry Init | `specprovider/registry.go:24`, `pmprovider/registry.go:24` | Validierung-Tests die Duplikat-Registrierung abfangen |
| Q-020 | LOW | Implicit Type-Coercion in Tests | `agui_events_test.go:30-41` | Type-Assertion oder typed Struct |
| Q-006 | LOW | Uninitialisierte Ref in ChatPanel | `ChatPanel.tsx:92` | Explizite Initialisierung |

**9 Findings. Mixed Go + Frontend, aber alles Test-fokussiert.**

---

## Worktree K: `feat/compliance-and-docs` (~1d)

Compliance-Luecken, Dokumentation, GDPR.

| ID | Sev | Title | File(s) | Fix |
|---|---|---|---|---|
| C-002 | MED | GitHub Token in Clone-URL | `adapter/github/provider.go:69` | Credential-Helper oder SSH statt Token-in-URL |
| C-005 | MED | Keine GDPR Daten-Loesch/Export-Endpoints | System-weit | `DELETE /users/me`, `GET /users/me/export` Endpoints designen |
| C-004 | MED | Keine OpenAPI/Swagger Dokumentation | System-weit | OpenAPI 3.0 Spec generieren (chi-docgen oder manuell) |
| C-003 | MED | CASCADE DELETE nicht explizit | `store_conversation.go:121` | Schema pruefen, ggf. Migration fuer explizites CASCADE |
| C-014 | MED | Default Admin Passwort Handling | `service/auth.go:415,422,472` | Mandatory Password-Change bei Erst-Login erzwingen |
| I-025 | MED | Kein separates Audit-Logging | System-weit | Audit-Log-Tabelle + Service designen |
| I-021 | MED | LLM Key Encryption vom JWT Secret abgeleitet | `main.go:514` | Separaten Encryption-Key einfuehren |
| S-010 | MED | Dev Postgres-Passwort in .env | `.env:24` | Startup-Warnung wenn Default-Passwort in production |
| C-015 | LOW | Kein CHANGELOG.md | Projekt-Root | CHANGELOG.md erstellen |
| C-007 | LOW | CSRF Rationale nicht in Arch-Docs | `middleware.go:17-25` | Absatz in `docs/architecture.md` hinzufuegen |
| C-006 | LOW | Image Alt-Text inkonsistent | `FileTree.tsx:283` | `alt="file icon"` / `alt="directory icon"` |

**11 Findings. Mix aus Code + Docs + Schema.**

---

## Worktree L: `chore/low-priority-cleanup` (Backlog)

Niedrige Prioritaet, kann opportunistisch abgearbeitet werden.

| ID | Sev | Title | File(s) |
|---|---|---|---|
| S-006 | LOW | Path-Traversal (bereits mitigiert) | `helpers.go:65-86` |
| S-011 | LOW | CSP Nonce + data: in img-src | `middleware.go:33` |
| Q-009 | LOW | Deferred Close ohne Error-Check | `speckit/provider.go:99` |
| Q-010 | LOW | Fragile VERSION Pfadsuche | `version/version.go:22` |
| Q-016 | LOW | Potentiell ungenutzte Handlers-Felder | `handlers.go` |
| A-011 | LOW | Kein Circular-Import-Check in CI | CI config |
| A-014 | LOW | Python Worker ohne Hex-Arch Struktur | `workers/codeforge/` |
| I-023 | LOW | Nginx Health-Check Config | `nginx.conf:45-47` |

**8 Findings. Alles LOW, kein Zeitdruck.**

---

## Zusammenfassung

| Worktree | Branch | Scope | Findings | Effort |
|----------|--------|-------|----------|--------|
| **F** | `fix/go-security-hardening` | Go Middleware, Handlers, main.go | 9 | ~2h |
| **G** | `refactor/hexagonal-port-interfaces` | Port-Layer Abstraktionen | 7 | ~1d |
| **H** | `fix/docker-infra-hardening` | Docker, Nginx, Traefik, CI | 12 | ~3h |
| **I** | `fix/python-error-handling` | Python Worker Exception-Handling | 3 | ~1h |
| **J** | `fix/test-coverage-gaps` | Go + Frontend Tests | 9 | ~1d |
| **K** | `feat/compliance-and-docs` | GDPR, OpenAPI, Audit-Logging, Docs | 11 | ~1d |
| **L** | `chore/low-priority-cleanup` | Backlog LOW-Findings | 8 | opportunistisch |
| | | **Gesamt** | **59** | |

**Abhaengigkeiten:**
- F, H, I, L: voellig unabhaengig, sofort parallelisierbar
- G: unabhaengig, aber vor Worktree D (Stub-Finder) erledigen fuer saubere Basis
- J: teilweise nach G (Tests fuer neue Port-Interfaces)
- K: teilweise nach F (Security-Headers muessen stehen bevor Compliance-Audit)

**Empfohlene Reihenfolge:** F + H + I parallel -> G -> J + K parallel -> L (Backlog)
