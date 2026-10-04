# Known Issues - Fix Plan

> **Status:** In progress (2026-10-03). Progress: **S0 done** (KI-1, KI-2, KI-3); **S1 done** (KI-4 to KI-14); **S2 done** (KI-18 to KI-24, KI-30 to KI-32); **S3 done** (KI-26 to KI-29); **S4 done** (KI-34 to KI-36, KI-43 to KI-51, KI-59, KI-61); **S5 done** (KI-39 to KI-42); **S6 done except KI-25 (real sub-agents)** (KI-71, tool process isolation and NATS authentication, is done: [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md)); the follow-up Known Issues KI-83 to KI-113 are open except **S7-A (KI-97, KI-100, KI-101), S7-B (KI-95, KI-105, KI-106, KI-107) and S7-C (KI-84, KI-85), done 2026-10-02, and S7-H (KI-96, per-tenant tool UIDs and Landlock: [ADR-018](architecture/adr/018-per-tenant-tool-identities-and-landlock.md)), done 2026-10-03**.
> **Scope:** the verified defects KI-1 to KI-113 in [todo.md - Known Issues](todo.md#known-issues), found by the
> docs/code reconciliation of 2026-09-29 on `staging`.
> **Goal:** CI that catches regressions, policy and security layers that actually enforce what the docs and ADRs
> promise, and a run pipeline that completes end to end - without changing the vision or the architecture.

## Principles

- **Reproduce first (TDD).** Every fix starts with a failing test that shows the defect, then the minimal fix
  (AGENTS.md "Testing"). Security fixes get table-driven tests for the bypass variants.
- **CI first.** The defects survived because CI was red since 2026-03-24 and nobody saw new failures. S0 makes
  every existing gate green and adds the missing ones (type check, vitest, integration tests) before feature fixes.
- **Fail closed and fail visibly.** A security check that cannot decide denies; a feature that cannot do what it
  reports returns an error instead of a fake success (see [Decisions](#decisions)).
- **Docs per fix.** When a KI is fixed: mark it `[x] (date)` in todo.md, remove its `KI-n` annotations
  (`grep -rn "KI-n\b" docs AGENTS.md`), update the affected docs.
- **No new runtime dependencies** unless a decision below says so.

## Milestones

| Milestone | Theme | Known Issues | Effort |
|---|---|---|---|
| **S0** | Green CI with complete gates | ~~KI-1~~, ~~KI-2~~, ~~KI-3~~ | M |
| **S1** | Policy and security enforcement | ~~KI-4~~, ~~KI-5~~, ~~KI-6~~, ~~KI-7~~, ~~KI-8~~, ~~KI-9~~, ~~KI-10~~, ~~KI-11~~, ~~KI-12~~, ~~KI-13~~, ~~KI-14~~ | L |
| **S2** | Reliable messaging and runtime | ~~KI-18~~, ~~KI-19~~, ~~KI-20~~, ~~KI-21~~, ~~KI-22~~, ~~KI-23~~, ~~KI-24~~, ~~KI-30~~, ~~KI-31~~, ~~KI-32~~ | L |
| **S3** | Quality gates and delivery | ~~KI-26~~, ~~KI-27~~, ~~KI-28~~, ~~KI-29~~ | M |
| **S4** | Operations and deployment | ~~KI-34~~, ~~KI-35~~, ~~KI-36~~, ~~KI-43~~, ~~KI-44~~, ~~KI-45~~, ~~KI-46~~, ~~KI-47~~, ~~KI-48~~, ~~KI-49~~, ~~KI-50~~, ~~KI-51~~, ~~KI-59~~, ~~KI-61~~ | M |
| **S5** | Frontend correctness | ~~KI-39~~, ~~KI-40~~, ~~KI-41~~, ~~KI-42~~ | M |
| **S6** | Trust, compliance, unwired features | ~~KI-15~~, ~~KI-16~~, ~~KI-17~~, KI-25 (part), ~~KI-33~~, ~~KI-37~~, ~~KI-38~~, ~~KI-52~~, ~~KI-53~~, ~~KI-54~~, ~~KI-55~~, ~~KI-56~~, ~~KI-57~~, ~~KI-58~~, ~~KI-60~~, ~~KI-62~~ | L |
| **S7** | Review follow-ups (KI-83 to KI-113) | KI-83, ~~KI-84~~, ~~KI-85~~, KI-86, KI-87, KI-89, KI-90, KI-91, KI-92, KI-93, KI-94, ~~KI-95~~, ~~KI-96~~, ~~KI-97~~, KI-98, ~~KI-100~~, ~~KI-101~~, ~~KI-105~~, ~~KI-106~~, ~~KI-107~~, KI-108, KI-109 (S7-A, S7-B, S7-C and S7-H done; KI-25, KI-88, KI-99, KI-102, KI-103, KI-104, KI-110 to KI-113: see below) | L |

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
| **KI-26** | **Done (2026-09-30).** Runs without gates deliver; failed gate fails the run and never delivers (D9); terminal-state guard in `HandleRunComplete` | Runtime tests for all three paths |
| **KI-27** | **Done (2026-09-30).** Delivery after checkpoint cleanup, or checkpoints outside the workspace history (shadow repo as documented) | Scratch-repo test for commit-local, branch and patch delivery |
| **KI-28** | **Done (2026-09-30).** Send `CODEFORGE_QG_TIMEOUT`; kill the process group on timeout; watchdog for `quality_gate` status | Worker timeout test; watchdog test |
| **KI-29** | **Done (2026-09-30).** Project/mode gate commands used; missing result fails the gate | Gate tests |

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
| **KI-47** | **Done (2026-10-01, with KI-70).** Traefik frontend service port 8080; the overlay itself works since KI-70 |
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
| **KI-15** | **Done (2026-10-01).** A2A routes outside the JWT group with their own API-key auth (keys map to tenants); inbound A2A prompts quarantined with their task; `HandoffService` constructed and wired (claimed stages, `handoff.status` with `run_id`) |
| **KI-16** | **Done (2026-09-30).** Experience pool tenant-scoped; `experience.enabled` honored |
| **KI-17** | **Done (2026-10-01).** Review pipeline wired end to end (`review_pipelines` record, baseline, impact gate, keep/undo with a HEAD compare-and-swap, pending list); RefactorApproval uses the `review.approval_required` event and the API client |
| **KI-25** | **Part done (2026-10-01).** The tool is no longer registered (D-S3); starting real sub-agent runs stays open |
| **KI-33** | **Done (2026-10-01).** Teams end with their plan; the watchdog check "ended teams" is the backstop |
| **KI-37** | **Done (2026-09-30).** Verifier metrics use the worker's LiteLLM HTTP client instead of the `litellm` package |
| **KI-38** | **Done (2026-09-30).** Allowlist uses `handoff_to`; wire or remove `agent.builtin_tools` / `tool_output_max_chars` |
| **KI-52** | **Done (2026-09-30).** Instantiate `RetentionService`; valid anonymization SQL (`WHERE id IN (SELECT ... LIMIT n)`) |
| **KI-53** | **Done (2026-09-30).** Scan `admin_email` as nullable |
| **KI-54** | **Done (2026-09-30).** Disable deepeval telemetry via env in the worker |
| **KI-55** | **Done (2026-10-01).** Web flow wired when `github.client_id`, `client_secret` and `callback_url` are set; 501 otherwise |
| **KI-56** | **Done (2026-10-01).** Provider names, exact project match, Plane token only to `plane.base_url`, honest status codes (the GitLab token and the VCS lookup: KI-85, S7-C) |
| **KI-57** | **Done (2026-10-01).** Email HITL with configured recipients and web UI URL; the mail links to a web approval page; disabled until configured |
| **KI-58** | **Done (2026-09-30).** `create_skill` uses the run's tenant ID |
| **KI-60** | **Done (2026-10-01).** The tiered cache is removed (ports, adapters, `cache.*` config, ristretto dependency) |
| **KI-62** | **Done (2026-10-01).** Stalled runs are re-planned up to `runtime.stall_max_retries` times |

## S7 - Review Follow-ups (planned 2026-10-02)

The follow-ups found by the S1 to S6 and KI-71 reviews, grouped so each group is one agent round (fix, full
verification, security review, code review, fix round, docs). Security and tenancy first:

| Group | Known Issues | Theme |
|---|---|---|
| **S7-A** | ~~KI-100~~, ~~KI-101~~, ~~KI-97~~ | **Done (2026-10-02).** MCP: outbound policy (SSRF protection) for the connection test and the worker, tenant filter on tool upserts, redaction of the URL and secret-looking arguments |
| **S7-B** | ~~KI-95~~, ~~KI-105~~, ~~KI-106~~, ~~KI-107~~ | **Done (2026-10-02).** In-process workspace access (worker and Go Core) only through `codeforge.workspace_fs` and `internal/workspacefs`, never a symlink out of the workspace and never a blocking FIFO; knowledge-base content in per-tenant areas below `knowledge.content_root`, KB writes admin-only, scope attach tenant-checked; `detect-stack` inside the caller's tenant area; benchmark datasets inside `benchmark.datasets_dir` |
| **S7-C** | ~~KI-85~~, ~~KI-84~~ | **Done (2026-10-02, one review round: findings 1 to 3 and 5 to 8 fixed, 4 not reproduced).** Inbound webhooks are registered per project (own ID, URL and secret naming tenant and project, exact repository match, each delivery handled once, migration 113); PM integrations carry their own API token and the operator's PM credentials serve only the default tenant; the GitLab PM provider goes through `netutil.OutboundPolicy` (`pm.allowed_private_hosts`); Slack approval messages link to the approval page and Slack and email send only the requests of `notification.approval_tenants` |
| **S7-D** | KI-83 | LSP language servers no longer run with the Go Core's environment and rights (design in a plan first) |
| **S7-E** | KI-89, KI-90, KI-91 | Data lifecycle: channel rows without a users row, retention for claim and completion records, quarantine expiry |
| **S7-F** | KI-86, KI-87, KI-94 | Runtime leftovers: single-replica assumption guarded or documented, SVN password off the command line, review pipeline and stall re-plan leftovers |
| **S7-G** | KI-92, KI-93, KI-98 | Frontend: War Room arrows, privacy page links, MCP header editor and admin-only actions |
| **S7-H** | ~~KI-96~~ | **Done (2026-10-03, one review round: four findings fixed; [plan](plans/ki96-tenant-tool-isolation-plan.md), [ADR-018](architecture/adr/018-per-tenant-tool-identities-and-landlock.md)).** Owner decision of 2026-10-02: every tool process runs as its tenant's tool UID (20000-29999, allocated lazily, migration 120) with no supplementary group, tenant directories carry POSIX ACLs (`workspace.tool_acls: required`), Landlock confines every tool call (mandatory in production), the tool environment travels on a memfd, tenant HOMEs live on the `tool_homes` volume, trees from before the upgrade are migrated per tenant without new capabilities, and project workspaces are deleted through the worker (migration 121); stage 3 (network) is KI-110 |

Not scheduled in S7, because each needs a design decision by the project owner first: KI-25 (real sub-agents),
KI-88 (submodule support); KI-96 (per-tenant tool UIDs) was decided on 2026-10-02 and done as S7-H. KI-99 is a residual risk
(documented), KI-102 is a release checklist item and KI-103 is measured; they stay open as notes. KI-104 (the
S7-A leftovers: other callers of the old SSRF filter, 6to4/Teredo, query tokens in other URLs) is open and not yet
scheduled. KI-108 (PM providers outside GitLab have no outbound policy) and KI-109 (no UI for webhook management) are the
S7-C follow-ups, also not yet scheduled. KI-110 (network and local IPC between tenants, stage 3 of S7-H), KI-111 (Claude
Code's Bash inherits the platform's Claude credentials), KI-112 (tenant erasure and tool UID reclaim) and KI-113 (orphaned
tool processes stay zombies) are the S7-H follow-ups, not yet scheduled.

## S8 - Audit and local-model follow-ups (planned 2026-10-03)

Known Issues KI-114 to KI-130, from the owner decisions, the README claim audit and the README screenshot run with a
local model. Order: what breaks a core pillar or a fresh installation first. S7-E, S7-F and S7-G run in the same round
as S8-A and S8-B.

| Group | Known Issues | Scope |
|---|---|---|
| **S8-A** | KI-125 (high), KI-127, KI-130 | Local models run agents with tools: tool capability from model metadata or an operator override, model resolved before the capability check, parallel streamed tool calls kept apart, the Ollama route through its OpenAI-compatible endpoint, no flood of public models without keys; streamed token usage; embedding model by env var and a BM25 fallback; file icons without the network |
| **S8-B** | KI-126, KI-128 | A failed command keeps its output (bounded); Bash rules accept plain leading variable assignments and stay fail closed for loader and shell variables and expansions |
| **S8-C** | KI-116, KI-117, KI-118, KI-124 | A fresh deployment works: published images, `gh`/`svn` replaced by REST APIs or shipped (owner decision S1 for PRs), backend CLIs and the worker's egress, the dev container |
| **S8-D** | KI-119, KI-120 | First-admin setup bound to a one-time token; tenant choice at login and a tenant screen (needs a design) |
| **S8-E** | KI-129, KI-109 | UI defects from the screenshot run; webhook management screen |
| later | KI-104, KI-108, KI-110 to KI-115, KI-121 (backend part), KI-122, KI-123 | Outbound-policy leftovers, stage 3 network isolation, Claude credentials, tenant erasure, zombies, model check, approval window, Copilot, config leftovers |

## S9 - Owner decisions of 2026-10-04 (planned)

Audience: self-hosters and single users first (AGENTS.md section 2), so single-user blockers come before multi-tenant
and GDPR work. Order:

| Group | Known Issues | Scope |
|---|---|---|
| **S9-A** | KI-119, KI-143, KI-131, KI-124 | A safe first start: one-time setup token; token epoch per user; scenario tags on every cloud route; a dev container that starts a working stack |
| **S9-B** | KI-116, KI-117, KI-118 | A deployment that works out of the box: release tags with the version pinned in compose; GitHub PM over REST and `svn` in the Core image; backend CLIs in the standard worker image with a documented egress path |
| **S9-C** | new feature, KI-125 follow-up | Pure-completion models use tools through a text tool protocol parsed by the worker, with grammar-constrained output where the server supports it (Ollama `format`, JSON schema); plan: [text-tool-protocol-plan.md](plans/text-tool-protocol-plan.md) |
| **S9-D** | KI-129 (vision part), KI-94 (decided parts), KI-138, KI-142, KI-146, KI-109 | One "agent work" read model for the dashboard, costs and a new page; refactorer write warning, `approve-partial` removed; small UI gaps |
| **S9-E** (low) | KI-145, KI-144, KI-120, KI-141 | Last-admin rule, export scope and user attribution, tenant from the e-mail at login, never-done claims |
| closed | KI-140 | Decision: keep the current rule (allow rules match the inner command) |

---

## Follow-up Known Issues (found while fixing, 2026-09-30)

Reviews of the S1 to S6 fixes found further defects. They are tracked in [todo.md](todo.md#known-issues) and
scheduled as follows:

| KI | Summary | Milestone |
|---|---|---|
| **KI-63** | Tool-call approvals resolved without a tenant check | S6, **done 2026-09-30** |
| **KI-64** | Tenant propagation over NATS per payload; default-tenant fallback on publish | S6, **done 2026-09-30** |
| **KI-65** | At-most-once work lacks complete Go-side watchdogs (conversations, backend tasks, SIGTERM) | S3 (with the gate watchdog, KI-28), **done 2026-09-30** (review rounds done 2026-10-01) |
| **KI-66** | Worker dedup keys too coarse | S2 follow-up, **done 2026-09-30** |
| **KI-67** | Worker consumer lifecycle gaps (partly fixed) | S2 follow-up, **done 2026-09-30** (core-NATS notifications left) |
| **KI-68** | Policy profiles are one global namespace | S6, **done 2026-09-30** |
| **KI-69** | Policy follow-ups (tools offered despite mode, clone snapshots, run profile, feedback providers, redirections) | S6, **done 2026-09-30** |
| **KI-70** | Blue-green overlay does not work | S4, **done 2026-10-01** |
| **KI-71** | Agent tools can read the worker's secrets (same UID); no NATS authentication | S6, **done 2026-10-01** (tool user uid 10002, secrets tmpfs, NATS users, [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md)) |
| **KI-72** | Claude Code runs bypass the policy layer | S6, **done 2026-09-30** |
| **KI-73** | Channel follow-ups (webhook key, ThreadPanel unmounted, typing/read) | S6, **done 2026-10-01** |
| **KI-74** | Frontend live-update follow-ups | S6, **done 2026-10-01** |
| **KI-75** | LLM models are global across tenants | S6, **done 2026-09-30** (platform admins only) |
| **KI-76** | Runtime follow-ups (plan step ModeID, router under lock, auto-agent race, blocked conversations) | S3 follow-up, **done 2026-09-30** (review rounds done 2026-10-01) |
| **KI-77** | Go core git calls in agent-writable workspaces are not hardened (fsmonitor, filters, hooks, credential helpers) | S3 review round, **done 2026-09-30** |
| **KI-78** | Artifact validation events/audit written before the run's end is decided | S6, **done 2026-10-01** |
| **KI-79** | GDPR residuals (quarantine reviewer free text, privacy page wording) | S6, **done 2026-10-01** |
| **KI-80** | Copilot token handed to every authenticated user | S6, **done 2026-09-30** |
| **KI-81** | Auto-agent runs workspace tests inside the Go Core with the core's environment | S3 follow-ups, **done 2026-10-01** |
| **KI-82** | Git config allowlist refuses common repositories | S3 follow-ups, **done 2026-10-01** |
| **KI-83** | LSP language servers run in the Go Core with the core's environment | S6 follow-up |
| **KI-84** | Slack approval buttons have no interaction endpoint; the Slack provider receives every tenant's requests | S6 follow-up, **done 2026-10-02** (S7-C: the message links to the approval page, no buttons; Slack and email send only `notification.approval_tenants`; agent text escaped) |
| **KI-85** | Inbound webhooks act only in the default tenant; no GitLab PM token | S6 follow-up, **done 2026-10-02** (S7-C: per-project webhooks `/api/v1/webhooks/{vcs,pm}/{provider}/{id}` with their own secret, tenant from the ID, `X-Tenant-ID` never read; own API tokens per PM integration; GitLab PM through `OutboundPolicy`; each delivery handled once; migration 113) |
| **KI-86** | Go Core assumes a single replica (in-memory approval and test waiters) | S6 follow-up |
| **KI-87** | SVN password passed on the command line | S6 follow-up |
| **KI-88** | Submodules and nested repositories are refused in workspaces; very large trees are refused | S3 follow-up (S3-F security review), open |
| **KI-89** | `channel_messages.sender_id` / `channels.created_by` reference users: callers without a users row probably cannot post or create channels | S6 follow-up (S6-H review), open |
| **KI-90** | No retention for `handoff_claims`, `task_result_costs`, `conversation_turn_completions` | S6 follow-up, open |
| **KI-91** | Quarantine messages never expire (a held A2A task waits for a decision) | S6 follow-up, open |
| **KI-92** | War Room arrows of `initiated` handoffs are never removed | S6 follow-up, open |
| **KI-93** | Privacy page links to a Settings > Privacy export/delete screen that does not exist | S6 follow-up, open |
| **KI-94** | Review pipeline and stall re-plan leftovers (edits during the refactorer, agent reservation, debate sub-plan cancel, partial approval, quoted paths, stall prompt, shared stall budget) | S6 follow-up, open |
| **KI-95** | In-process worker readers (repo map, retrieval, GraphRAG collectors, file tools) follow symlinks out of the workspace | KI-71 follow-up, **done 2026-10-02** (S7-B: `internal/workspacefs` and `codeforge.workspace_fs` for the Core and the worker; the Core had no check at all) |
| **KI-96** | All tenants share the tool uid 10002 | KI-71 follow-up, **done 2026-10-03** (S7-H: a tool UID per tenant with POSIX ACLs, Landlock per tool call, the environment on a memfd, deletion through the worker; [ADR-018](architecture/adr/018-per-tenant-tool-identities-and-landlock.md)) |
| **KI-97** | MCP args and URL userinfo are not redacted | KI-71 follow-up, **done 2026-10-02** (S7-A: URL and credential arguments redacted, keep rule by whole-value comparison, no URL secrets in errors and logs) |
| **KI-98** | MCP UI: header editor, "stored, unchanged" hint, admin actions shown to non-admins | KI-71 follow-up, open |
| **KI-99** | NATS does not permission-check deliveries to a known plain subscription (residual) | KI-71 follow-up, open |
| **KI-100** | SSRF in the MCP connection test (sse/streamable_http URLs from the Go Core) | KI-71 follow-up, **done 2026-10-02** (S7-A: `netutil.OutboundPolicy`, `mcp.allowed_private_hosts`, `mcp.use_proxy`, worker `GuardedTransport`) |
| **KI-101** | `UpsertMCPServerTools` has no tenant filter | KI-71 follow-up, **done 2026-10-02** (S7-A) |
| **KI-102** | Re-run the NATS permission tests before a NATS image upgrade (nats-server 2.11 or newer) | KI-71 follow-up, open |
| **KI-103** | Cost of the tool-file sharing pass on very large workspaces; files of processes that outlive the pass | KI-71 follow-up, open |
| **KI-104** | Outbound-policy gaps outside the MCP path: `SafeTransport` / `IsPrivateIP` (A2A, VCS, project-git) lack CGNAT, multicast and NAT64; 6to4 and Teredo count as public; query tokens in other URLs (the GitLab PM provider uses `OutboundPolicy` since KI-85) | S7-A follow-up, open |
| **KI-105** | Knowledge-base `content_path` reads any file (any absolute path; KB routes without a role check) | S7-B review, **done 2026-10-02** (S7-B: per-tenant areas below `knowledge.content_root`, one uniform 400, admin-only writes, tenant-checked scope attach) |
| **KI-106** | `POST /detect-stack` scans other tenants' workspaces | S7-B review, **done 2026-10-02** (S7-B: the caller's tenant area, Adopt's rule) |
| **KI-107** | Benchmark dataset paths read any file (dev mode) | S7-B review, **done 2026-10-02** (S7-B: names or paths inside `benchmark.datasets_dir`) |
| **KI-108** | PM providers outside GitLab (Plane, Gitea/Forgejo/Codeberg) use plain HTTP clients: blind SSRF through a manual sync's `base_url`, response bodies in errors, no size limit | S7-C review, open |
| **KI-109** | No UI for webhook management (per-project webhooks are API only) | S7-C follow-up, open |
| **KI-110** | Tool processes share the network and local IPC across tenants (loopback TCP, internal services, abstract sockets below Landlock ABI 6, SysV and POSIX IPC, `/dev/shm`) | S7-H follow-up (stage 3), open |
| **KI-111** | Claude Code runs expose the platform's Claude credentials to agent commands (to verify) | S7-H review, open |
| **KI-112** | No tenant erasure; tool UIDs of deleted tenants are never reclaimed | S7-H review, open |
| **KI-113** | Orphaned tool processes stay zombies (the worker is PID 1) | S7-H follow-up, open |

## Decisions

Accepted principles from the earlier (main-based) fix plan that the project owner approved on 2026-09-29 apply
unchanged: deny lists win and evaluation fails closed (D6), a failed quality gate fails the run and never delivers
(D9), placeholder features must not pretend to work (D11), `DELETE /api/v1/llm/models/{id}` (D12). New for staging:

| ID | Decision | Rationale |
|---|---|---|
| **D-S1** | Record the deny-list semantics as [ADR-015](architecture/adr/015-policy-deny-lists-and-tool-names.md) (amends ADR-007, written 2026-09-30) and the NATS delivery topology as ADR-016 (refines ADR-001) | ADR-007's own text already says a PathDeny match denies; the code contradicts it |
| **D-S2** | Canonical tool names live in the Go policy domain; presets keep ADR-007 names; workers keep their tool names | One mapping, one place; LLM-facing tool names stay stable |
| **D-S3** | Features that report success without doing anything (sandbox isolation, `spawn_subagent`, review-refactor trigger) fail visibly until implemented; roadmap items stay | Follows D6/D11; silent no-ops are worse than a clear error |
