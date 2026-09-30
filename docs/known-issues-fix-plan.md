# Known Issues - Fix Plan

> **Status:** In progress (2026-09-30). Progress: **S0 done** (KI-1, KI-2, KI-3); **S1 done** (KI-4 to KI-14); **S2 in progress** (KI-18 to KI-20, KI-24, KI-30 to KI-32 done); **S4 in progress** (KI-43 to KI-46, KI-48 to KI-50, KI-59 done); **S5 done** (KI-39 to KI-42).
> **Scope:** the verified defects KI-1 to KI-62 in [todo.md - Known Issues](todo.md#known-issues), found by the
> docs/code reconciliation of 2026-09-29 on `staging`.
> **Goal:** CI that catches regressions, policy and security layers that actually enforce what the docs and ADRs
> promise, and a run pipeline that completes end to end - without changing the vision or the architecture.

## Principles

- **Reproduce first (TDD).** Every fix starts with a failing test that shows the defect, then the minimal fix
  (CLAUDE.md "TDD"). Security fixes get table-driven tests for the bypass variants.
- **CI first.** The defects survived because CI was red since 2026-03-24 and nobody saw new failures. S0 makes
  every existing gate green and adds the missing ones (type check, vitest, integration tests) before feature fixes.
- **Fail closed and fail visibly.** A security check that cannot decide denies; a feature that cannot do what it
  reports returns an error instead of a fake success (see [Decisions](#decisions)).
- **Docs per fix.** When a KI is fixed: mark it `[x] (date)` in todo.md, remove its `KI-n` annotations
  (`grep -rn "KI-n\b" docs CLAUDE.md`), update the affected docs.
- **No new runtime dependencies** unless a decision below says so.

## Milestones

| Milestone | Theme | Known Issues | Effort |
|---|---|---|---|
| **S0** | Green CI with complete gates | KI-1, KI-2, KI-3 | M |
| **S1** | Policy and security enforcement | ~~KI-4~~, ~~KI-5~~, ~~KI-6~~, ~~KI-7~~, ~~KI-8~~, ~~KI-9~~, ~~KI-10~~, ~~KI-11~~, ~~KI-12~~, ~~KI-13~~, ~~KI-14~~ | L |
| **S2** | Reliable messaging and runtime | ~~KI-18~~, ~~KI-19~~, ~~KI-20~~, ~~KI-21~~, ~~KI-22~~, ~~KI-23~~, ~~KI-24~~, ~~KI-30~~, ~~KI-31~~, ~~KI-32~~ | L |
| **S3** | Quality gates and delivery | KI-26, KI-27, KI-28, KI-29 | M |
| **S4** | Operations and deployment | ~~KI-34~~, ~~KI-35~~, ~~KI-36~~, ~~KI-43~~, ~~KI-44~~, ~~KI-45~~, ~~KI-46~~, KI-47, ~~KI-48~~, ~~KI-49~~, ~~KI-50~~, ~~KI-51~~, ~~KI-59~~, ~~KI-61~~ | M |
| **S5** | Frontend correctness | ~~KI-39~~, ~~KI-40~~, ~~KI-41~~, ~~KI-42~~ | M |
| **S6** | Trust, compliance, unwired features | KI-15, KI-16, KI-17, KI-25, KI-33, KI-37, KI-38, ~~KI-52~~, ~~KI-53~~, ~~KI-54~~, KI-55, KI-56, KI-57, KI-58, KI-60, KI-62 | L |

Order rationale: S0 first because every later fix needs trustworthy tests. S1 next because KI-4/KI-5 make every
policy preset ineffective (permissive presets allow `curl` and `.env` edits, `plan-readonly` cannot run at all) -
the most severe defects in the list. S2 before S3 because gate and delivery fixes depend on runs completing
exactly once. S4 makes the production compose start; S5 and S6 are independent of each other.

---

## S0 - Green CI With Complete Gates

> **Done (2026-09-30).** KI-1, KI-2 and KI-3 fixed: Python (2541 tests), Go unit (`-race`) and `integration`-tagged
> tests, frontend `tsc` and vitest (398 tests) are green and gated in CI; smoke tests pass locally with the admin seeded.
> KI-11 was fixed here as well, because chi v5.3.0 (KI-3) deprecates `RealIP`.

| KI | Fix | Proof |
|---|---|---|
| **KI-1** | Relock Python deps (psutil), pin `ruff` 0.15.1 as Poetry dev dependency, golangci-lint v2.11.4 (v2.1.6 cannot lint go 1.25; v2.10+ knows the G706 exclude) + fix its findings, run CI for PRs to `staging` | Each CI job green on the PR |
| **KI-3** | Bump vulnerable Go modules (chi 5.3.0, pgx 5.9.2, otel 1.44.0, grpc 1.83.1, x/net, x/text), `@unovis` 1.7.1 + in-range npm updates, Python updates; drop the obsolete `setuptools<82` pin | govulncheck, `npm audit --omit=dev --audit-level=high`, pip-audit clean |
| **KI-2** | Repair the rotted suites: root-cause every failing test (stale test vs. code bug vs. non-hermetic vs. order-dependent), fix code bugs the tests expose, never skip or weaken; Go contract fixtures keep the committed format; then add the missing gates: `tsc --noEmit` and `vitest run` in the Frontend job, `-tags=integration` tests in CI | Full `pytest`, `go test -race ./...`, `vitest`, `tsc` green locally and in CI |

## S1 - Policy and Security Enforcement

| KI | Root cause | Fix | Proof |
|---|---|---|---|
| **KI-4** | Worker tool names (`bash`, `read_file`, ...) never equal preset names (`Bash`, `Read`, ...); no path/command extracted | **Done (2026-09-30).** One canonical tool-name table in the Go policy domain (worker names and Claude Code names -> ADR-007 names); the worker sends `path` / `command` extracted from the tool arguments; unknown tools are evaluated under their own name and fall to the mode default (see D-S2) | Table test: every built-in worker tool maps; end-to-end evaluate for each preset: permissive denies `curl` and `.env` edits, `plan-readonly` allows reads and the LLM call |
| **KI-5** | Deny lists only skip the rule | **Done (2026-09-30).** Two-pass evaluation: any matching deny list denies regardless of rule order; paths normalized relative to the workspace, escapes denied; deny lists fail closed on calls without a value (ADR-015, amends ADR-007) | Rule-order and path-variant tables (`./.env`, `a/../.env`, absolute) |
| **KI-6** | Prefix matching on raw `bash -c` strings | **Done (2026-09-30).** Split the command into segments (`;`, `&&`, `\|\|`, `\|`, newline, `$()`, backticks, subshells); every segment must pass; match on the basename of the executable; unparsable constructs fail closed | Bypass table (`go test ./... ; curl x`, `/usr/bin/curl`, `$(curl ...)`) |
| **KI-7** | Conversation path allows on unknown profile, ignores mode profile, Allow-Always not persisted, glob from JSON args | **Done (2026-09-30).** Unknown profile denies (same as run path); mode-derived profile used; `PolicyDir` wired so Allow-Always rules persist; Allow-Always pattern built from the canonical command | Handler/service tests incl. restart (reload from dir) |
| **KI-8** | Unsynchronized map | **Done (2026-09-30).** `sync.RWMutex` around `PolicyService.profiles` | `go test -race` with concurrent evaluate + update |
| **KI-9** | Built-in presets writable | **Done (2026-09-30).** `POST /policies` rejects built-in names (409) | Handler test |
| **KI-10** | Mode tool lists not enforced | **Done (2026-09-30).** Mode `Tools`/`DeniedTools` use canonical names and are enforced in the Go evaluation (deny wins) | Read-only modes cannot write or run bash |
| **KI-11** | `chimw.RealIP` before limiters | **Done (S0).** `middleware.ClientIP` trusts forwarding headers only from `server.trusted_proxies`; IPv6 keyed by /64 | `clientip_test.go`, `TestRateLimitKeyGroupsIPv6By64`, config tests |
| **KI-12** | Tenant-blind broadcast, blocking writes, ticket endpoint unwired | **Done (2026-09-30).** `BroadcastToTenant` for all tenant events; per-client buffered send queue with write timeout (slow client dropped, not blocking); wire `WSTickets` and stop sending the JWT in the URL | Hub tests with two tenants and a stalled client; ticket handler test |
| **KI-13** | Sandbox container unused | **Done (2026-09-30).** Fail closed: runs in `sandbox`/`hybrid` exec mode are rejected with a clear error until tools execute inside the container (see D-S3); fix `--cpus` float formatting | Run-start test; sandbox args test |
| **KI-14** | Dev compose publishes DB/NATS on 0.0.0.0 | **Done (2026-09-30).** Bind to `127.0.0.1` in `docker-compose.yml` | Compose config check |

## S2 - Reliable Messaging and Runtime

| KI | Fix | Proof |
|---|---|---|
| **KI-18** | **Done (2026-09-30, [ADR-016](architecture/adr/016-nats-delivery-semantics.md)).** Durable work consumers without inactivity threshold and with `DeliverNew` on first creation; notification subjects via core NATS; long work acked on accept + Go run watchdog (ADR-016, adapting the main-based D1-D4); per-run cancel listeners unsubscribed | Integration test against real NATS: restart does not replay; two workers do not double-execute |
| **KI-19** | **Done (2026-09-30, [ADR-016](architecture/adr/016-nats-delivery-semantics.md)).** Python retry via JetStream `num_delivered`, `max_deliver`, DLQ publish on the last attempt; invalid payloads terminated, not NAK'd | Worker tests with a fake message |
| **KI-20** | **Done (2026-09-30, [ADR-016](architecture/adr/016-nats-delivery-semantics.md)).** Validate `runs.*`, `context.*`, `repomap.*` payloads against their structs | Validator table test |
| **KI-21** | **Done (2026-09-30).** Worker policy wait >= Go HITL timeout (derive both from one config value); run path uses the agent loop instead of one completion (follow-up item if large) | Timeout test; run-path integration test |
| **KI-22** | **Done (2026-09-30).** `tasks.cancel` stops backend processes; remove or publish `review.trigger.request` | Backend cancel test |
| **KI-23** | **Done (2026-09-30).** `workspace_path` and backend in run/backend-task payloads; shared workspace volume in prod compose | Contract fixture round trip |
| **KI-24** | **Done (2026-09-30).** Clear the conversation cancel flag when a new run starts | Service test |
| **KI-30** | **Done (2026-09-30).** Termination, stall and cancel paths call `onRunComplete` | Orchestrator test per path |
| **KI-31** | **Done (2026-09-30).** Status predicates / version checks on run, plan and team updates; no write-back from `cancelled` | Store integration test |
| **KI-32** | **Done (2026-09-30).** Persist result and plan events with real IDs or nullable columns; do not discard errors | Event store integration test |

## S3 - Quality Gates and Delivery

| KI | Fix | Proof |
|---|---|---|
| **KI-26** | Runs without gates deliver; failed gate fails the run and never delivers (D9); terminal-state guard in `HandleRunComplete` | Runtime tests for all three paths |
| **KI-27** | Delivery after checkpoint cleanup, or checkpoints outside the workspace history (shadow repo as documented) | Scratch-repo test for commit-local, branch and patch delivery |
| **KI-28** | Send `CODEFORGE_QG_TIMEOUT`; kill the process group on timeout; watchdog for `quality_gate` status | Worker timeout test; watchdog test |
| **KI-29** | Project/mode gate commands used; missing result fails the gate | Gate tests |

## S4 - Operations and Deployment

| KI | Fix |
|---|---|
| **KI-34** | **Done (2026-09-30).** Worker health: HTTP `/health` + `/health/ready` from the worker (port of the main-based fix), or sentinel on a tmpfs; compose healthchecks use it |
| **KI-35** | **Done (2026-09-30).** Worker logs in the Go schema (`time`, `level`, `msg`, `service`) for structlog and stdlib loggers (port of the main-based fix) |
| **KI-36** | **Done (2026-09-30).** Go OTEL exporter honors `CODEFORGE_OTEL_INSECURE`; Python metrics exporter wired; trace context injected on publish |
| **KI-43** | **Done (2026-09-30).** Mount PG 18 data at `/var/lib/postgresql` in both compose files (with a migration note for existing volumes) |
| **KI-44** | **Done (2026-09-30).** Prod Postgres: either provision certificates or `ssl=off` behind the internal network, documented |
| **KI-45** | **Done (2026-09-30).** Writable workspace volume for the prod core (read-only rootfs stays) |
| **KI-46** | **Done (2026-09-30).** Go reads `*_FILE` secrets via the existing file provider; prod compose passes files instead of env; JWT secret and internal key wired; `validate-env.sh` checks the real names |
| **KI-47** | **Port done (2026-09-30); overlay blockers: KI-70.** Traefik frontend service port 8080 |
| **KI-48** | **Done (2026-09-30).** Image scan uses a tag that is pushed |
| **KI-49** | **Done (2026-09-30).** Restore script terminates connections with a working psql invocation |
| **KI-50** | **Done (2026-09-30).** Devcontainer sets `LITELLM_BASE_URL` |
| **KI-51** | **Done (2026-09-30).** Config drift items fixed individually (Ollama api_base from env, DOCS_MCP vars, logs.sh name, example yaml comments, SMTP default 587, resolve-docker-ips.sh safe to source, one history token default) |
| **KI-59** | **Done (2026-09-30).** Replace non-ASCII characters in the listed config files and scripts |
| **KI-61** | **Done (2026-09-30), secrets-only.** SIGHUP reloads the secrets vault and logs the names of changed settings that need a restart; the unused `ConfigHolder` is removed |

## S5 - Frontend Correctness

| KI | Fix |
|---|---|
| **KI-39** | **Done (2026-09-30);** live output is filtered by the project's task IDs instead of adding `project_id` to `task.output` (both emitters only know run/task/tenant; a task's `task.status`/`run.status` event precedes its output). Refetch run/plan/agent panels on their WebSocket events; include `project_id` in `task.output` (or filter by task IDs); AgentLane filters by task |
| **KI-40** | **Done (2026-09-30).** `DELETE /api/v1/llm/models/{id}` in Go (replaces the POST route, D12 of the main-based plan); remove or implement the non-existent MCP client methods |
| **KI-41** | **Done (2026-09-30).** Project update merges config keys (PATCH semantics); `autonomy_level` either wired or removed from the popover |
| **KI-42** | **Done (2026-09-30).** Broadcast channel events (tenant-scoped, see KI-12) |

## S6 - Trust, Compliance and Unwired Features

| KI | Fix |
|---|---|
| **KI-15** | A2A routes outside the JWT group with their own API-key auth; quarantine inbound A2A; construct and wire `HandoffService` |
| **KI-16** | Experience pool tenant-scoped; `experience.enabled` honored |
| **KI-17** | Wire the review pipeline orchestrator and `DiffImpactScorer`, or return 501 until wired (D-S3); fix the event name, payload and auth in RefactorApproval |
| **KI-25** | `spawn_subagent` starts a real sub-agent run, or the tool is removed from BASE_TOOLS until it does (D-S3) |
| **KI-33** | Clean up teams when their plan finishes |
| **KI-37** | Verifier metrics use the worker's LiteLLM HTTP client instead of the `litellm` package |
| **KI-38** | Allowlist uses `handoff_to`; wire or remove `agent.builtin_tools` / `tool_output_max_chars` |
| **KI-52** | **Done (2026-09-30).** Instantiate `RetentionService`; valid anonymization SQL (`WHERE id IN (SELECT ... LIMIT n)`) |
| **KI-53** | **Done (2026-09-30).** Scan `admin_email` as nullable |
| **KI-54** | **Done (2026-09-30).** Disable deepeval telemetry via env in the worker |
| **KI-55** | Wire `GitHubOAuthService` into the handlers (config present) or keep 501 and document it |
| **KI-56** | Provider names and per-provider config for webhook sync |
| **KI-57** | Email HITL with configured recipients, public callback URL and POST-safe links (or disabled until configured) |
| **KI-58** | `create_skill` uses the run's tenant ID |
| **KI-60** | Inject the tiered cache where the docs say it is used, or document it as unused (D-S3) |
| **KI-62** | Call `ReplanStep` from stall detection, or document re-planning as planned |

## Follow-up Known Issues (found while fixing, 2026-09-30)

Reviews of the S1, S2 and S4 fixes found further defects. They are tracked in [todo.md](todo.md#known-issues) and
scheduled as follows:

| KI | Summary | Milestone |
|---|---|---|
| **KI-63** | Tool-call approvals resolved without a tenant check | S6 |
| **KI-64** | Tenant propagation over NATS per payload; default-tenant fallback on publish | S6 |
| **KI-65** | At-most-once work lacks complete Go-side watchdogs (conversations, backend tasks, SIGTERM) | S3 (with the gate watchdog, KI-28) |
| **KI-66** | Worker dedup keys too coarse | S2 follow-up |
| **KI-67** | Worker consumer lifecycle gaps (partly fixed) | S2 follow-up |
| **KI-68** | Policy profiles are one global namespace | S6 |
| **KI-69** | Policy follow-ups (tools offered despite mode, clone snapshots, run profile, feedback providers, redirections) | S6 |
| **KI-70** | Blue-green overlay does not work | S4 |
| **KI-71** | Agent tools can read the worker's secrets (same UID) | S6 (with KI-13) |
| **KI-72** | Claude Code runs bypass the policy layer | S6, **done 2026-09-30** |
| **KI-73** | Channel follow-ups (webhook key, ThreadPanel unmounted, typing/read) | S6 |
| **KI-74** | Frontend live-update follow-ups | S6 |
| **KI-75** | LLM models are global across tenants | S6 |
| **KI-76** | Runtime follow-ups (plan step ModeID, router under lock, auto-agent race, blocked conversations) | S3 follow-up |
| **KI-77** | Go core git calls in agent-writable workspaces are not hardened (fsmonitor, filters, hooks, credential helpers) | S3 review round |

## Decisions

Accepted principles from the earlier (main-based) fix plan that the project owner approved on 2026-09-29 apply
unchanged: deny lists win and evaluation fails closed (D6), a failed quality gate fails the run and never delivers
(D9), placeholder features must not pretend to work (D11), `DELETE /api/v1/llm/models/{id}` (D12). New for staging:

| ID | Decision | Rationale |
|---|---|---|
| **D-S1** | Record the deny-list semantics as [ADR-015](architecture/adr/015-policy-deny-lists-and-tool-names.md) (amends ADR-007, written 2026-09-30) and the NATS delivery topology as ADR-016 (refines ADR-001) | ADR-007's own text already says a PathDeny match denies; the code contradicts it |
| **D-S2** | Canonical tool names live in the Go policy domain; presets keep ADR-007 names; workers keep their tool names | One mapping, one place; LLM-facing tool names stay stable |
| **D-S3** | Features that report success without doing anything (sandbox isolation, `spawn_subagent`, review-refactor trigger) fail visibly until implemented; roadmap items stay | Follows D6/D11; silent no-ops are worse than a clear error |
