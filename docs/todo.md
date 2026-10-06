# CodeForge — TODO Tracker

> LLM Agents: This is your **primary** task reference.
> Always read this file before starting work; the **Current work** line below states the current priority.
>
> **Current work (2026-10-03):** the [Known Issues](#known-issues) are being fixed milestone by milestone
> ([fix plan](known-issues-fix-plan.md)): S0 to S6 are done except [KI-25](#known-issues) (real sub-agents);
> [KI-71](#known-issues) (agent tools run as a separate tool user, NATS authentication) is fixed; the follow-up
> Known Issues KI-83 to KI-109 found by the fix reviews are scheduled as S7 in the fix plan; S7-A (KI-97, KI-100,
> KI-101: MCP outbound policy, redaction, tenant-scoped tool upserts), S7-B (KI-95, KI-105, KI-106, KI-107:
> symlink-safe workspace access, knowledge areas), S7-C (KI-84, KI-85: per-project webhooks, Slack approval links)
> and S7-H (KI-96: per-tenant tool UIDs, POSIX ACLs and Landlock, ADR-018) are done, the rest is open.
> The 2026-03-28 agent pipeline TODOs live in `docs/plans/2026-03-28-agent-improvement-todos.md` (all checked off).

### How to Use This File

- Before starting work: Read this file to understand what needs to be done
- After completing a task: Mark it `[x]`, add completion date, move to "Recently Completed" if needed
- When discovering new work: Add items to the appropriate section with context
- Format: `- [ ]` for open/pending, `- [x]` for done (with date)
- Cross-reference: Link to feature docs, architecture.md sections, or issues where relevant

---

- [x] (2026-10-03) `AGENTS.md` holds only the rules; the mechanics it listed moved into the linked docs (completeness check of every removed statement): versioning and release to [dev-setup](dev-setup.md#versioning-and-release), tenant isolation rules to [architecture.md](architecture.md#tenant-isolation), the handler checklist (settling helpers, at-most-once registration, duplicate guards, quality-gate verdicts, `workspace.delete.request.dlq`) to [ADR-016](architecture/adr/016-nats-delivery-semantics.md), the tool-call payload fields and profile resolution to [ADR-015](architecture/adr/015-policy-deny-lists-and-tool-names.md), platform admins and model credential stripping to [features/03](features/03-multi-llm-provider.md#shared-models-and-platform-admins), the docs tree and feature-doc structure to [README](README.md). Stale status notes fixed on the way (KI-10, KI-21, KI-22, KI-23, KI-38, KI-65 to KI-69 are fixed, but architecture.md and features/04 still described them as open).
- [x] (2026-10-03) S7-H landed (per-tenant tool identities and Landlock, KI-96; [ADR-018](architecture/adr/018-per-tenant-tool-identities-and-landlock.md), [plan](plans/ki96-tenant-tool-isolation-plan.md), [SECURITY.md](SECURITY.md#agent-tool-isolation), [dev-setup](dev-setup.md#upgrading-to-per-tenant-tool-users-ki-96); one review round): every tool process of a tenant runs as the tenant's tool UID (20000-29999, allocated lazily, `tenants.tool_uid`, migration 120) with no supplementary group, umask 007 and Landlock (ABI 2+, mandatory in production); tenant directories get POSIX ACLs instead of the shared group; the tool environment travels on a memfd, never on argv; tenant HOMEs live on the `tool_homes` volume; trees from before the upgrade are migrated per tenant without new capabilities; project workspaces are deleted by the worker as the tenant (migration 121, `workspace.delete.request`/`.result`); `/health/ready` answers 503 with the reason when isolation is not ready; `scripts/check-host.sh` is the preflight. New config: `workspace.tool_acls`/`CODEFORGE_WORKSPACE_TOOL_ACLS` (Core), `CODEFORGE_TOOL_LANDLOCK`, `_LANDLOCK_MIN_ABI`, `_HOME_BASE`, `_PATH`, `_READ_PATHS`, `_CACHE_MAX_MB` (worker); `CODEFORGE_TOOL_UID`/`GID`/`HOME` are gone. Operators will notice: the upgrade order (backup with ACLs, check-host.sh, stop every worker, core, new workers); host requirements (Landlock ABI 2+, Docker 23+, POSIX ACLs on both volumes, all workers on one host); tenant creation is platform-admin only; project deletion answers 409 while work runs; tools cannot use `ps`/`pkill`/`df`/`ss` or `/tmp`; background processes end when the tenant's work in a worker ends; in required mode an operator's `CLAUDE_CONFIG_DIR` no longer reaches Claude Code. Fixed along the way: `.dockerignore` excluded `scripts/worker-entrypoint.sh`, so `Dockerfile.worker` had not built since KI-71. The KI-25 and KI-88 plans now name ADR-019 and ADR-020. New open follow-ups: [KI-110](#known-issues), [KI-111](#known-issues), [KI-112](#known-issues), [KI-113](#known-issues).
- [x] (2026-10-03) S7-H review round (KI-96): the worker's own commands that run as a tool UID (sharing pass, removals, cache measurement) end within 600 s and the migration's owner walks within 3600 s, then their process group is killed and the step counts as failed (a tenant's leftover process could stop them below Landlock ABI 6 and hold the subject's message loop forever); the tenant's last work item in a worker kills its leftover processes before its end-of-work steps; project deletion also removes the Go Core's own private entries (`.git/codeforge/patches`, 0700/0600, out of the tool UID's reach) by descriptor, so it no longer fails with ENOTEMPTY after every patch delivery; the worker image installs pytest and ruff into the system interpreter of the tool PATH (`workers/tool-requirements.txt`, hash-pinned to poetry.lock; the default Python gate commands and the auto-agent's workspace test failed as "test failed" without them), and the isolation probe runs `pytest --version` when the tool PATH has pytest; temporary entries on the shared volumes get random names instead of the PID (every replica is PID 1), and a rollback detected by two replicas at once no longer leaves one not ready.
- [x] (2026-10-03) S7-H review round 3 (KI-96): the worker's walk over its own leftovers at project deletion is bounded (60 s, 100,000 entries, 128 levels, read as a stream, at most 258 descriptors), so a tenant process that keeps adding entries can no longer stall `workspace.delete.request` for every tenant or exhaust the worker's descriptors; a limit fails the deletion with ENOTEMPTY and the reason, and the Go Core publishes it again every 10 minutes. A tenant is idle in a worker only when none of its work items runs or is still in its end-of-work steps (before, the last running item's reap could kill another item's helpers and leave its Claude Code config behind). The reaper never collects the worker's own helpers, so a killed helper is never reported as exit status 0. State files on the shared volumes (UID bindings, stamps) are written completely or removed again; a failed binding write no longer binds the UID to an empty tenant, and the migration's unshare copy no longer replaces a file with a cut-off copy. CI: the Landlock helper sets `no_new_privs` itself (unprivileged callers got EPERM), tests that need root skip without it, and the root isolation step makes the runner's HOME traversable for the tool users. Residual: a deletion blocked by a live tenant process costs up to 60 s per retry (no attempt limit); a leftover that recreates `claude/<work_id>` after its item ended is not removed; a helper of a cancelled end-of-work step can be reaped (never reported as success).
- [x] (2026-10-02) S7-C landed (per-project inbound webhooks and Slack approval links, KI-84, KI-85; [SECURITY.md](SECURITY.md#security-measures), [dev-setup](dev-setup.md#upgrading-to-per-project-webhooks-ki-85); one review round): VCS and PM webhooks are registered per project (`POST /api/v1/projects/{id}/webhooks`, admins) with their own ID, URL (`/api/v1/webhooks/{vcs|pm}/{provider}/{id}`) and secret, which name the tenant and the project; `X-Tenant-ID` is never read on these routes; the Slack approval message links to the approval page instead of posting buttons; Slack and approval emails send only the requests of `notification.approval_tenants`. Migration 113 (`webhook_endpoints`, `webhook_deliveries`); new config keys `notification.approval_tenants`, `pm.allowed_private_hosts` and `webhook.delivery_retention` (the three global `webhook.*` secrets are ignored). Operators will notice: the old global webhook URLs answer 410, so every webhook must be registered per project and entered at the provider with its new URL and secret (for Plane: register Plane's own secret); a project's `repo_url` must name the exact host and owner/name, or its events are ignored; rotating `auth.jwt_secret` makes stored webhook secrets unreadable (rotate them); Slack approvals need `notification.web_ui_url` as well; a self-hosted GitLab on a private network or on loopback must be listed in `pm.allowed_private_hosts`, and GitLab PM requests no longer use `HTTP(S)_PROXY`; other tenants' github-issues and plane imports and syncs need the integration's own token (the operator's Plane token and the Core's gh login serve only the default tenant). New open follow-ups: [KI-108](#known-issues), [KI-109](#known-issues).
- [x] (2026-10-02) S7-B landed (symlink-safe in-process workspace access and knowledge areas, KI-95, KI-105, KI-106, KI-107; [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md) note of 2026-10-02, [SECURITY.md](SECURITY.md#agent-tool-isolation); three review rounds): the Go Core and the worker read and write workspace files only through `internal/workspacefs` (os.Root) and `codeforge.workspace_fs` (descriptor walks with `O_NOFOLLOW`); knowledge-base content lives in per-tenant areas `<knowledge.content_root>/<tenant_id>/`, benchmark datasets only inside `benchmark.datasets_dir`, and `POST /detect-stack` stays inside the caller's tenant area. New config key `knowledge.content_root` / `CODEFORGE_KNOWLEDGE_CONTENT_ROOT` (default `data/knowledge`; prod: a read-only `knowledge` volume at `/data/knowledge` in core and worker). No migration. Operators will notice: the file API answers 400 (was 500) for a path that leaves the workspace, a special file or a file over 10 MiB, delete and rename act on a symlink itself and the workspace root cannot be deleted or renamed; absolute symlinks are no longer followed by the Core or the worker process; a workspace whose directory is a symlink or FIFO is refused; creating, changing, deleting and indexing a knowledge base need the admin role (403 otherwise), `content_path` is relative to the tenant's area and every refusal is one and the same 400, and existing knowledge bases pointing anywhere else stop indexing and no longer feed agent context (fill `/data/knowledge/<tenant_id>/` and re-create them; logged once per knowledge base); attaching a knowledge base to a scope across tenants answers 404; `detect-stack` outside the caller's tenant area is 400 (outside the workspace root only platform admins inside `workspace.adopt_roots`); benchmark datasets must be names or paths inside the datasets directory (dev mode; `.yml` names resolve now; a worker whose working directory, its parent and `CODEFORGE_WORKSPACE` all lack `configs/benchmarks` needs an absolute `CODEFORGE_BENCHMARK_DATASETS_DIR`); `read_file` streams (output capped at 10 MiB, offset and limit reach any line) and `edit_file` keeps the file's line endings; a rolling upgrade shows an old Core's `kb:` index request as an index error in a new worker. Internal API: `NewFileService(store)`, `NewGoalDiscoveryService(store)`, `NewKnowledgeBaseService(store, contentRoot)`, `NewContextOptimizerService(store, orchCfg, limits)` plus `SetKnowledgeBases`; the filesystem port and the `osfs` adapter are removed.
- [x] (2026-10-02) Core defects fixed along the way in S7-B (all found while auditing what read workspace files): goal discovery (project setup, `goals/detect`) stored the content of a symlinked file such as `CLAUDE.md -> /run/secrets/jwt-secret` as a project goal every user could read; the context optimizer and the AI goal discovery put such content into agent context; the spec providers and the roadmap keyword scan imported it as roadmap features, and the markdown provider's `ReadSpec` even accepted `../`; `SyncToSpecFile` and the markdown `WriteSpec` wrote `ROADMAP.md` through a symlink, so any file the Core could write; the file browser checked with `EvalSymlinks` and opened the path again (a race), its delete and rename acted on a symlink's target, and `DELETE path=.` (or `x/..`) deleted the whole workspace; every Core reader, index seeding and the `.gitattributes` check included, blocked forever on a FIFO (the filter-attribute check now also fails closed, when the repository cannot be opened and when listing fails); `writePatch` opened `.git` with a plain `os.OpenRoot`, which a FIFO in the `.git` directory's place could block; and, found in the review, `AddKnowledgeBaseToScope` attached a knowledge base across tenants (the link carried the default tenant; attach and detach are tenant-checked now and scope listings show only the caller's tenant's knowledge bases).
- [x] (2026-10-02) S7-A landed (MCP outbound policy and redaction, KI-97, KI-100, KI-101; [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md) decision 9, [SECURITY.md](SECURITY.md#mcp-servers); three review rounds): sse and streamable_http MCP URLs follow `netutil.OutboundPolicy` in the Go Core (create, update, both tests) and in the worker (`GuardedTransport`), configured by `mcp.allowed_private_hosts` / `CODEFORGE_MCP_ALLOWED_PRIVATE_HOSTS` and `mcp.use_proxy` / `CODEFORGE_MCP_USE_PROXY`; MCP reads redact the URL (userinfo, credential query values) and credential arguments; tool upserts are tenant-scoped. No migration. Operators will notice: link-local, metadata and (by default) loopback addresses are refused (400 on create, update and test; the worker skips the server with a logged reason), loopback opens only by an explicit allowlist entry (`localhost`, `127.0.0.1`, `::1`, a loopback CIDR; dev: `127.0.0.1` for docs-mcp), private addresses (compose service names such as `docs-mcp` included) need `mcp.allowed_private_hosts`, `servers_dir` servers may use private and loopback addresses without a listing, invalid allowlist entries stop startup, MCP URLs must be http or https with a host (a `servers_dir` file with an invalid URL is logged at error level and skipped), connection tests and worker connections ignore `HTTP(S)_PROXY` unless `mcp.use_proxy` is on (the Core then logs a startup warning), reads show `https://***@host/...?api_key=***` and `--token=***`, an edit that keeps `***` in the URL or arguments must send them exactly as read, a server saved on a now-refused address stays readable and can be disabled, renamed or deleted (its test answers 400, runs skip it), the URL placeholder is `https://mcp.example.com/sse`. New open follow-up: [KI-104](#known-issues).
- [x] (2026-10-02) Bugs fixed along the way in S7-A: conversation runs dropped the MCP servers' headers and description (runs and conversations now share `MCPService.RunServerPayloads`); streamable_http servers never connected in the worker (it unpacked two values from an SDK call that yields three in the locked mcp 1.30; it now uses `streamable_http_client` with the guarded client); the Core's sse connection test never connected (`TestConnection` called `Initialize` without `Start`, so every sse test failed with "transport not started yet" and sent no request; it now starts the client first, a failed start is reported as "connect failed: ...").
- [x] (2026-10-01) S6 and the S3 follow-ups landed (PR branch `claude/busy-dijkstra-q0oxi9`): KI-15, KI-17, KI-33, KI-55, KI-56, KI-57, KI-60, KI-62, KI-70, KI-73, KI-74, KI-78, KI-79, KI-81 and KI-82 are fixed, KI-25 partly (`spawn_subagent` is no longer offered), KI-47 completely. The S3 review follow-ups 1a to 1f and P2/P3 are done (see KI-77, KI-82). The reviews of that work added: S2-F review F1 to F14 and S2-G (a dispatch ID on every backend task dispatch, heartbeats and `heartbeat_seconds` on every start, `runtime.task_accept_timeout`, dead-letter subscribers for task, handoff and benchmark starts, cancel listeners that replay from their start message, handoff claims, conversation completions kept once per turn, task cost counted once per dispatch), S6-D (account deletion erases like the GDPR endpoints, the retention sweep runs on the advisory-lock connection and creates its indexes `CONCURRENTLY`, calendar-year cutoffs with 29 February clamped to 28 February), S6-F (review pipeline, stall re-planning, team cleanup), S6-G (evaluation errors never score 0.0, `tool_output_max_chars` on `runs.start`, exact credential-name redaction, `handoff_to` always offered when registered) and S6-H (blue-green, channels, quarantine reviewer, live updates). New Known Issues: KI-88 to KI-94. Still open: KI-25 (sub-agents), KI-83 to KI-94 (KI-71 followed the same day, see below).
- [x] (2026-10-01) KI-71 landed (tool isolation, NATS authentication, [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md); four review rounds and a security-review round): agent tool processes run as the tool user (uid 10002), the worker's secrets sit in a tmpfs only the worker can enter, and NATS requires a user per service with per-subject permissions. New operator steps: run `scripts/generate-secrets.sh` and `scripts/validate-env.sh` after the upgrade, deploy `configs/nats/`, apply migrations 111 and 112. Details in the KI-71 entry; open follow-ups KI-95 to KI-103.
- [x] (2026-10-01) Stdio MCP servers could never start in the worker: `mcp_workbench` passed `errlog=io.StringIO()` to the MCP SDK's `stdio_client`, which needs a real file descriptor, so every stdio server failed to connect. Found while moving the servers to the tool user (KI-71); the error log now goes to `/dev/null` and a failed connection closes its server process and log handle.
- [x] (2026-10-01) Checkpoints and delivery could miss a same-size change made in the same second as the index they copy: the private index copy got a new modification time, so git trusted racily-clean entries (`TestDeliver_Patch` failed about 1 in 50 runs). The copy keeps the source index's time (`seedIndex`), restoring git's racy-git re-check; 0 failures in 400 runs.
- [x] (2026-10-01) Dependencies: OpenTelemetry Go modules v1.42/v1.44 -> v1.45.0 (GO-2026-6505: exporter config logging could leak endpoint URLs in info logs; govulncheck in the Security Scanning job). `go mod tidy` also raised golang.org/x/{crypto,net,sync,sys,term,text}, grpc-gateway and genproto to the versions the new modules require.
- [x] (2026-10-01) CI: `test_performance_10k_chars` failed on a shared runner (10.2ms against a 10ms budget; the regex-bound analyzer needs about 7ms locally). Budget 25ms: it guards against order-of-magnitude regressions, not runner speed.
- [x] (2026-10-01) CI secret scanning: gitleaks reported the synthetic secrets of `internal/config/secret_files_test.go` once the PR commit window reached them; `.gitleaks.toml` (default rules plus an allowlist of those exact fixture values) keeps the scan strict for everything else.
- [x] (2026-10-01) Agent instructions follow the AGENTS.md convention: `CLAUDE.md` is replaced by [`AGENTS.md`](../AGENTS.md) (structure after the Scavengarr `AGENTS.md`: workflow, overview, architecture, dependencies, language rules, testing, agent system and cross-language rules, subagents, dev container, navigation). Descriptive catalogues moved to [architecture/project-reference.md](architecture/project-reference.md), the E2E startup procedure to [testing/e2e-setup.md](testing/e2e-setup.md). Claude Code reads `AGENTS.md` when no `CLAUDE.md` exists (v2.1.277 or newer).
- [x] (2026-09-30) SessionStart hook for Claude Code on the web (`.claude/hooks/session-start.sh`, registered in `.claude/settings.json`): installs the CI toolchains and dependencies and starts PostgreSQL 18 + NATS JetStream for the Go tests, see [dev-setup](dev-setup.md#claude-code-on-the-web-sessionstart-hook)

### Handover (2026-10-06, session end)

The owner asked the session of 2026-10-03..06 to finish its in-flight work and end. State at the end (branch `claude/busy-dijkstra-q0oxi9`, PR into `staging`):

- **Landed:** S9-B, S9-C, KI-94, webhooks UI, MCP servers, S10-G, S10-E1, S10-I, S10-D, S10-E2, the S9-D UI round, the model switch (`qwen3.5:4b-q4_K_M`), the AGENTS.md update. Every landed round went through a review and a fix round; the residuals are Known Issues (KI-224 to KI-227).
- **Next, in order** (fix rounds of the [2026-10-06 code review](audits/2026-10-06-code-review/README.md), one agent per round in its own worktree, reviewed and landed by the lead): S10-A (HTTP authorization and audit), S10-H (agent runtime state), then S10-F (store, A2A, data lifecycle), S10-C (secrets, SSRF, tenant mixing), S10-B (authentication and sessions), then S10-J (frontend) and S10-K (evaluation and leftovers). The audit README lists the KIs of each round.
- **Agent-work read model** ([plan](plans/2026-10-06-agent-work-read-model.md)): 2 of 5 commits exist in worktree `.claude/worktrees/agent-a7510158b2681edfb` (branch `s9d-agentwork`, base `5e280e44`): the per-turn result columns with the completion claim and the `agent_work` view with its store. Before landing, renumber its migrations 128/129 to 129/130 (127 review_user_edits and 128 roadmap_spec_files landed since) and rebase onto the branch head; then the remaining commits (dashboard, cost page and activity timeline reading `agent_work`, the new page).
- **Benchmark run 2** with the new default model ([autonomous-goal-benchmark.md](testing/autonomous-goal-benchmark.md)), then the S9-C live check; the Ollama container of the benchmark host is `codeforge-shots-ollama`.
- **Open items of recent rounds:** KI-214 (build, CI and deployment hygiene: PostgreSQL roles, provider keys in `docker inspect`, the Claude Code policy test in CI), KI-225 (text tool protocol residuals), KI-227 (S10-D git and SVN residuals), KI-228 (S10-E2 worker residuals).
- **Session practice that worked:** at most two agents at a time; heavy test runs serialized through one flock wrapper; `go clean -cache` whenever the disk goes under 3 GB free; private databases per round; reviews on read-only snapshots; the lead writes the docs (see AGENTS.md section 8).

### Known Issues

> Verified defects found in the docs/code reconciliation of 2026-09-29 on `staging` (HEAD `cb9b63ce`).
> IDs (KI-1..KI-229) are stable and never renumbered; other docs link here (`todo.md#known-issues`) by ID.
> Every unchecked item is an open task: when its fix lands, check it `[x]` with the date and keep the entry.

#### CI and tooling

- [x] (2026-09-29) **KI-1 CI red on `staging`** (high): CI has been red on `staging` since 2026-03-24: `poetry.lock` is stale (psutil added without re-locking), `ruff` is not a Poetry dependency although CI runs `poetry run ruff`, golangci-lint v2.1.6 (built with go1.24) refuses the go 1.25 module (v2.5.0 reports 11 findings), and CI does not run for pull requests to `staging`. Evidence: `.github/workflows/ci.yml:7`, `.github/workflows/ci.yml:62`, `.github/workflows/ci.yml:85`, `pyproject.toml:33`. **Fixed (S0):** `poetry.lock` regenerated; `ruff` 0.15.1 pinned as Poetry dev dependency (in sync with the ruff-pre-commit rev); golangci-lint pinned to v2.11.4 in CI and the devcontainer (v2.1.6 cannot lint the go 1.25 module; v2.10+ is needed for the G706 exclude in `.golangci.yml`) plus fixes for its findings; CI also runs for pull requests to `staging`.
- [x] (2026-09-30) **KI-2 Test suites rotted while CI was red** (high): Python has 2 collection errors (`tests/evaluation.py` is shadowed by the `tests/evaluation/` package; `test_score_key_normalization.py` imports names moved to `_benchmark_gemmas.py`) and 127 failing tests in 43 files; in Go, TestCORSWildcardRestriction, TestHandleListPolicies (nil-pointer panic), TestAPIKeyStore_TenantIsolation and TestOAuthState_GetExpired fail, `internal/service` hangs until the 10 min timeout, and the contract test rewrites `testdata/contracts/*.json` without a trailing newline. The frontend has 13 `tsc --noEmit` errors (some are runtime bugs, see KI-40) and 4 failing vitest tests; neither the type check, vitest nor any `-tags=integration` test runs in CI. Evidence: `workers/tests/test_score_key_normalization.py:16`, `scripts/test.sh:73`, `.github/workflows/ci.yml:114-120`. **Fixed (S0):** every failing test root-caused and repaired (none skipped or weakened); real bugs found on the way fixed (logger kwargs in context_reranker/history/skills safety, tool router cap, over-matching `rm -rf` blocklist, `_clear_processed`, Quarantine/Routing page crashes, missing MCP client methods, canvas image export, a 10-minute deadlock and a data race in `internal/service` tests); contract fixtures keep their committed format; CI gains `go vet -tags=integration`, the `integration`-tagged Go tests, `npm run typecheck` and `npm run test`; the smoke job seeds the admin. Result: pytest 2541 passed, `go test -race ./...` and integration tests green, vitest 398 passed, `tsc` clean. Remaining nit: the provider registry tests in `internal/port/*` are not repeatable with `-count>1` (global registries; CI uses `-count=1`).
- [x] (2026-09-29) **KI-3 Dependencies with known vulnerabilities** (high): The Security Scanning job is red: govulncheck finds 8 reachable vulnerabilities in 5 Go modules (chi v5.2.5 RealIP spoofing, pgx v5.9.1 placeholder SQL injection, otel sdk v1.42.0, grpc v1.79.2, x/net v0.51.0, x/text v0.35.0), `npm audit --omit=dev` reports 4 critical and 1 high (seroval, maplibre-gl via @unovis 1.6.4, lodash-es, protocol-buffers-schema, yaml) and pip-audit 30+ advisories (cryptography, pyjwt, starlette, mcp, python-multipart, requests, urllib3, aiohttp, pytest, ...). `setuptools<82` is still pinned for agentneo, which was removed on 2026-03-05. Evidence: `go.mod:9`, `go.mod:11`, `frontend/package.json:24-25`, `pyproject.toml:34`. **Fixed (S0):** Go chi v5.3.0, pgx v5.9.2, OpenTelemetry v1.44.0, grpc v1.83.1, x/net v0.55.0, x/text v0.39.0; `@unovis` ^1.7.1 plus in-range npm updates; Python updates (cryptography, pyjwt, starlette, mcp, python-multipart, aiohttp, ...), obsolete `setuptools<82` pin removed. govulncheck (module findings), `npm audit --omit=dev --audit-level=high` and pip-audit are clean.

#### Security and policy

- [x] (2026-09-30) **KI-4 Policy rules never match real tool calls** (critical): The agent loop requests permission with snake_case tool names (`bash`, `read_file`, `edit_file`, ...), the raw JSON arguments as `command` and no `path`, and the Claude Code executor sends `file:read`-style names, while every preset matches `Read`/`Edit`/`Write`/`Bash`/`Grep` exactly. Decisions therefore come only from the mode default: `headless-permissive-sandbox` and `trusted-mount-autonomous` allow everything (curl/wget/ssh, `.env` edits), `plan-readonly` denies even the LLM call, and the default `headless-safe-sandbox` asks for every call. Evidence: `workers/codeforge/tool_executor.py:75-77`, `internal/service/policy.go:257`, `internal/domain/policy/presets.go:40-49`. **Fixed (S1, [ADR-015](architecture/adr/015-policy-deny-lists-and-tool-names.md)):** one canonical tool-name table in the policy domain (`internal/domain/policy/toolnames.go`, `CanonicalTool`: worker, Claude Code and legacy category names map to `Read`/`Write`/`Edit`/`Bash`/`Grep`/`Glob`/`ListDir`); every evaluation path canonicalizes. The worker sends the full bash command and the real `file_path`/`path` (`tool_executor.policy_request_args`), plus a display-only `arguments_preview`; presets allow `ListDir` wherever they allow Read/Glob/Grep, `plan-readonly` allows the LLM call.
- [x] (2026-09-30) **KI-5 `path_deny` / `command_deny` do not deny** (high): A matching deny list only skips that rule, so evaluation falls through to later rules or the mode default (allow under `acceptEdits`, so `.env` edits pass in `headless-permissive-sandbox`), contrary to ADR-007. Paths are only `filepath.Clean`ed (not normalized to the workspace), and calls without a path skip deny lists entirely. Evidence: `internal/service/policy.go:270-296`, `internal/domain/policy/presets.go:79-83`, `internal/service/policy_test.go:42-73`. **Fixed (S1):** two-pass evaluation (any matching deny list denies regardless of rule order, then first-match-wins); a deny list also denies a call without a value for it; paths are normalized against the workspace and escapes are denied; deny matching ignores case; malformed patterns count as a match and are rejected by validation. Presets express their protections as deny lists (`.env`, `**/.env`, `secrets/**`, `**/credentials.*`; the permissive network ban as `command_deny`). Legacy tool globs (`file:*`, `*_file`) still match raw and canonical names for deny/ask rules.
- [x] (2026-09-30) **KI-6 Command rules use plain prefix matching** (high): `matchCommandPattern` is a string prefix check while the bash tool runs `bash -c`, so `go test ./... ; curl x | sh` passes the `headless-safe-sandbox` allow list and `/usr/bin/curl` or `sh -c "curl ..."` bypass the permissive curl deny. Evidence: `internal/service/policy.go:326-328`, `internal/domain/policy/presets.go:85-89`, `workers/codeforge/tools/bash.py:118-121`. **Fixed (S1):** `internal/domain/policy/command.go` parses the command (quotes, escapes, ANSI-C quoting, comments, here-doc bodies, `;`/`&`/`&&`/`|`/`||`/newlines/parentheses) and matches every simple command by executable basename; allow lists need every part to match, deny lists deny on any part. Constructs that cannot be analysed are opaque (never allowed by an allow list, denied by every `command_deny`): substitutions, `$[..]`, non-plain `${..}`, assignment prefixes and `env`, shells/`eval`/`source`, inline interpreter code, `/dev/tcp`, code-running options of git/go/sed/awk/find/make/npm/tar/rsync, unknown wrapper options. Safe wrappers (`time`, `timeout`, `nice`, `nohup`, `command`, `xargs`) are unwrapped. Fuzzed (`FuzzParseShellCommand`) and covered by a 29-string bypass table (`TestShellBypassRegressions`). Residual risk: scripts the agent writes and runs, symlinks and redirection targets are not path-checked; real containment needs the sandbox (KI-13).
- [x] (2026-09-30) **KI-7 Conversation tool calls fail open and ignore the intended profile** (high): An unknown policy profile allows the call (the run path denies), the mode-derived profile in the payload is ignored (the project/default profile always applies), and Allow-Always clones live only in memory because `PolicyHandlers.PolicyDir` is never set, so after a restart the project points at an unknown profile and every call is allowed. The Allow-Always glob is built from the first token of the JSON arguments (`{"command":*`) and matches almost any later bash command (affects the "Allow Always" entry under Frontend UI Bug Fixes & i18n). Evidence: `internal/service/runtime_execution.go:201-205`, `cmd/codeforge/main.go:813-817`, `internal/service/policy.go:177-181`. **Fixed (S1):** an unknown profile, unknown mode or missing project denies on both the run and the conversation path, with the reason sent to the worker. One resolver (`conversationPolicyProfile`) serves dispatch and evaluation: project `policy_profile`, then `config["policy_preset"]`, then the mode-derived preset, then the default. The worker reports the turn's mode (`mode_id` on `runs.toolcall.request`). Allow-Always extends the profile that decided the call (`agui.permission_request.profile`) in a per-project clone `{profile}-custom-{projectID}` that replaces its base only for that project and never re-pins the project; Bash rules list every executable of the approved command. Persistence: `policy.custom_dir` (default `data/policies`) is loaded at start and written back atomically to each profile's source file; two files defining one profile fail startup; `trust_minimum` allow rules need a trust annotation that meets them.
- [x] (2026-09-30) **KI-8 Policy profile map is not synchronized** (high): `PolicyService.profiles` is a plain map written by HTTP handlers (`SaveProfile`, `DeleteProfile`, `PrependRule` via Allow Always) and read by NATS tool-call handlers without a lock, so a policy edit during a run can crash the Go Core with a fatal concurrent map read/write. Evidence: `internal/service/policy.go:18-21`, `internal/service/policy.go:98`. **Fixed (S1):** a `sync.RWMutex` guards the map; changes and their file writes are serialized; profiles are replaced, never modified in place (race test).
- [x] (2026-09-30) **KI-9 Editors can overwrite built-in presets** (medium): `POST /api/v1/policies` (admin/editor) calls `SaveProfile`, which has no preset check, so an editor can replace `headless-safe-sandbox` (the default profile) with an allow-all profile that persists via the policy directory; `DeleteProfile` and `PrependRule` do reject presets. Evidence: `internal/adapter/http/handlers_policy_crud.go:67-91`, `internal/service/policy.go:94-100`. **Fixed (S1):** `SaveProfile` rejects preset names with `ErrConflict` (HTTP 409); a new profile never overwrites an existing policy file (409); validation errors return 400.
- [x] (2026-09-30) **KI-10 Mode tool restrictions are not enforced** (medium): Modes declare PascalCase `Tools`/`DeniedTools` that never match the worker's tool names, and `denied_tools` is only parsed and rendered into prompts, so read-only modes (architect, reviewer, security) are still offered `write_file`, `edit_file` and `bash`. Evidence: `workers/codeforge/models.py:79`, `internal/service/mode_prompt.go:83-84`, `workers/codeforge/agent_loop.py:226-252`. **Fixed (S1):** mode tool lists use canonical names (`mode.BuiltinToolNames` = `policy.BuiltinTools()`) and are enforced on the run and conversation paths (`policy.WithModeTools`): `DeniedTools` denies any tool, a non-empty `Tools` list denies built-in tools it does not list (MCP tools, `LLM`, skills, handoff are only restricted by `DeniedTools`). Read-only modes can no longer write or run bash. Checkpoints cover Edit, Write and Bash (canonical names).
- [x] (2026-09-29) **KI-11 Rate limiting and audit IPs use spoofable headers** (high): `chimw.RealIP` runs before the rate limiters, so `True-Client-IP` / `X-Real-IP` / `X-Forwarded-For` choose the bucket (bypassing the login brute-force limiter) and the audit-log IP, and rotating values fills the 100k bucket cap so new clients get 429. Evidence: `cmd/codeforge/main.go:924`, `internal/middleware/ratelimit.go:165-170`, `internal/middleware/audit.go:28`. **Fixed (S0):** `chimw.RealIP` (deprecated in chi v5.3.0) replaced by `middleware.ClientIP` (`internal/middleware/clientip.go`), which honours forwarding headers only from `server.trusted_proxies` / `CODEFORGE_TRUSTED_PROXIES` and runs first in the chain; rate limit keys group IPv6 by /64. Fixed early in S0 because the chi upgrade made `RealIP` a lint error.
- [x] (2026-09-30) **KI-12 WebSocket fan-out and ticket auth are broken** (high): Every event is broadcast to every tenant's clients (`BroadcastToTenant` has no callers), and broadcasts write to all clients under the hub lock without a write timeout, so one stalled client blocks all live updates. `POST /api/v1/ws/ticket` panics because `WSTickets` is never wired, so the JWT still travels in the WebSocket URL (audit SEC-008 / WT-12 not in effect). Evidence: `internal/adapter/ws/events.go:10-21`, `internal/adapter/ws/handler.go:91-111`, `internal/adapter/http/handlers_auth.go:485`. **Fixed (S1):** `BroadcastEvent` delivers only to clients of the tenant in ctx (`tenantctx.Lookup`, no default fallback); an event without a tenant is dropped and logged (fail closed). `BroadcastGlobal` (port `broadcast.GlobalBroadcaster`) is the one explicit all-tenant path, used only for `model.health` of the shared LiteLLM proxy. NATS requests carry `tenant_id` and the worker echoes it on every result/stream subject (`withPayloadTenant`; a stored run/conversation/review/A2A task wins via `withEntityTenant` / `RuntimeService.loadRunScoped`); background work keeps only the request tenant (`detachTenant`); handoff requests and runs carry the source run's tenant. Each client has a bounded send queue (1024) drained by its own writer with a 10 s write timeout; a full queue or failed write drops only that client. `main.go` wires the ticket store: `POST /api/v1/ws/ticket` issues single-use tickets (30 s TTL) bound to user and tenant, `GET /ws?ticket=` redeems them in the hub, `?token=` is rejected (401). The frontend fetches a ticket per connection attempt, keeps the socket across token refreshes, closes it on logout and while the password must be changed. Core and worker must be deployed together (events from an old worker without `tenant_id` are dropped).
- [x] (2026-09-30) **KI-13 Sandbox and hybrid modes provide no isolation** (high): A container is created per run, but `SandboxService.Exec` has no callers and the worker runs tools as local subprocesses (`bash -c` with the worker's filesystem, network and credentials). `--cpus` uses integer division, so a `cpu_quota` below 1000 becomes `--cpus=0` (unlimited). Evidence: `internal/service/sandbox.go:217`, `internal/service/sandbox.go:91`, `workers/codeforge/tools/bash.py:118-124`. **Fixed (S1):** fail closed (D-S3): runs, agentic conversations and benchmark runs in `sandbox`/`hybrid` mode (explicit or via the project config `execution_mode`) are rejected with HTTP 400 before anything is created; the worker completes any non-`mount` `runs.start` as failed; handoffs declare `mount`; the benchmark UI shows sandbox/hybrid as not available; `--cpus` is a decimal and non-positive quotas are rejected. Running tools inside the container stays a roadmap item. Follow-ups: goal discovery still creates an empty conversation before the rejection; preset descriptions still say "sandbox".
- [x] (2026-09-30) **KI-14 Dev compose exposes PostgreSQL and NATS** (medium): `docker-compose.yml` publishes PostgreSQL (default password `codeforge_dev`) and NATS 4222/8222 without authentication on all interfaces, so anyone on the host's network can read the database or publish forged NATS messages (audit SEC-003, SEC-004, INFRA-007, INFRA-008 still open). Evidence: `docker-compose.yml:77-82`, `docker-compose.yml:116-119`. **Fixed (S1):** all 10 published dev ports in `docker-compose.yml` bind to 127.0.0.1 (remote access via SSH tunnel).
- [x] (2026-09-30) **KI-63 Tool-call approvals are not tenant-checked** (medium): `POST /runs/{id}/approve/{callId}` and the conversation approval route call `RuntimeService.ResolveApproval(runID, callID, decision)`, which resolves the pending in-memory approval by IDs only, without checking that the run belongs to the caller's tenant. Knowing a run ID and call ID of another tenant is enough to approve or deny its tool call. Evidence: `internal/service/runtime_approval.go:106`, `internal/adapter/http/handlers_agent_features.go:486`, `internal/adapter/http/handlers_conversation.go:147`. Found during the KI-12 fix (2026-09-30). **Fixed (S2):** pending approvals are keyed by run, call and tenant; `ResolveApproval` resolves only in the caller's tenant (another tenant gets 404 and the approval stays pending); bypass-approvals and stop load the conversation tenant-scoped first (404 otherwise).
- [x] (2026-09-30) **KI-64 Tenant propagation over NATS is per payload** (low): since KI-12 every NATS request struct carries `tenant_id`, the worker echoes it and each Go subscriber scopes its context by hand (`withPayloadTenant`, `loadRunScoped`); a new subject that misses one of the three edits silently loses its live events. Outgoing payloads fill `tenant_id` from `tenantctx.FromContext`, which falls back to the default tenant instead of failing closed. Follow-up: stamp the tenant as a NATS header on publish and restore it in the adapter (like `X-Request-ID`), and use `tenantctx.Lookup` for outgoing payloads. Evidence: `internal/service/tenant_scope.go`, `internal/service/agent.go:95`. Found in the KI-12 code review (2026-09-30). **Fixed (S6):** every Go publish carries the tenant as the `X-Tenant-ID` header; handlers use it when neither request nor payload sets a tenant (request > payload > header, `tenantctx.Explicit`); the worker binds the header tenant while handling a message and echoes it on everything it publishes; outgoing payloads take the tenant from the context (`outgoingTenant`; a missing tenant is logged as an error, the publish is not failed yet). **A2A follow-up fixed (S6):** the inbound A2A executor's Execute and Cancel require the tenant of the request (`ErrValidation` without one) instead of falling back to the default tenant.
- [x] (2026-09-30) **KI-68 Policy profiles are one global namespace** (medium): policy profiles are not tenant-scoped; any editor of any tenant can list profiles and replace any non-preset profile via `POST /policies` (since S1 the change is also written to the policy directory). Evidence: `internal/service/policy.go`, `internal/adapter/http/handlers_policy_crud.go`. Found in the S1 security review (2026-09-30). **Fixed (S6):** custom profiles live in `<policy.custom_dir>/<tenant_id>/` (tenant must be a UUID, atomic writes); lookup is the caller's tenant, then legacy flat files (default tenant only, read-only), then the global read-only presets; the listing shows presets plus the caller's own profiles; save/replace/delete/Allow-Always touch only the caller's tenant (an Allow-Always clone goes to the project's tenant); runs, conversations, `EvaluatePolicy` and Claude Code resolve profiles in the run's tenant. A flat Allow-Always clone made for another tenant's project must be moved to `<dir>/<tenant>/` to apply again.
- [x] (2026-09-30) **KI-69 Policy follow-ups from S1** (low): the worker still offers tools a mode denies to the LLM (they are refused at call time); Allow-Always clones are snapshots of their base (later preset changes do not reach them); runs ignore the project's policy profile (request or default profile only); the Slack and email approval providers do not receive `profile` or `arguments_preview`; Bash redirection targets are not checked against `path_deny`. Found during the KI-4..KI-10 fixes (2026-09-30). **Fixed (S6):** (a) the worker offers the LLM only the tools its mode allows (`ToolRegistry.restrict_to_mode`, same rule as `policy.WithModeTools`, canonical names via `policy_args.canonical_tool`, kept equal to Go by a test); (b) runs resolve the profile request > project `policy_profile` / config `policy_preset` > default (conversations additionally pick a preset from the mode's autonomy); (c) Slack and email approvals show `profile` and `arguments_preview` (Slack escapes `&<>` and backticks, email HTML-escapes values and keeps line breaks out of the subject); (d) Bash redirection targets are checked against `path_deny` (see ADR-015; unknown targets fail closed); (e) Allow-Always clones stay snapshots of their base profile (documented, no change). **Follow-up fixed (S6-E):** Bash redirection targets are checked against the workspace path and its real path (`policy.WithWorkspaceRealPath`), so a retargeted symlink does not move the check.
- [x] (2026-09-30) **KI-77 Go core runs git in agent-writable workspaces without hardening** (high): core and worker share the workspace volume and UID, and agent tools can write `.git/config`, `.gitattributes` and hooks (Bash, and Write/Edit since `resolve_safe_path` does not block `.git/`). Every Go git call in a workspace honours that config: a planted `core.fsmonitor` runs on `git add -A`, `write-tree` and `diff --cached`, clean/smudge filters on `add`/`read-tree -u`, external diff drivers on `diff`, hooks such as `reference-transaction` on `update-ref`, credential helpers and `core.sshCommand` on push. Checkpoints run on every allowed file-changing tool call, so an agent can execute code in the core container (JWT secret, LLM key encryption secret). Affected: checkpoint/rewind/patch delivery (`internal/service/checkpoint.go`, `git_worktree.go`, `deliver.go`), the gitlocal provider (`GET /projects/{id}/git/*`). Found in the S2-B and S3 security reviews (2026-09-30); being fixed with the S3 review round. **Fixed (S3 review):** every Go git call in a workspace (checkpoints, delivery incl. `gh`, gitlocal/GitHub providers, workspace init, ls-remote) goes through `internal/git/workspace.go`: sanitised environment (no global/system config or attributes, inherited `GIT_*` dropped, no prompts/pager/editor), `GIT_CONFIG_COUNT` overrides (hooks, fsmonitor, credential helpers, signing, gc/maintenance, submodule recursion off; filter drivers neutralised; https/http/ssh/git transports only) and pre-checks that execute nothing (`.git` a real directory without `commondir`/alternates or symlinked config/refs/logs/objects; config keys on an allowlist, fail closed). Worker file tools refuse `.git` components; presets deny Write/Edit on `**/.git/**`. Behaviour: global git config is ignored for workspace git; repositories with config keys outside the allowlist are refused; pull/push via local-path remotes are refused. Residual: a still-running agent process could rewrite `.git/config` between check and git's read (KI-71 gave tools their own UID, but the workspaces are group-writable for them, so the window stays open); the SVN provider is not hardened yet. **Follow-ups fixed (S3-F, 2026-10-01):** (1a) commit delivery commits only the run's own change (a three-way merge of the run's base checkpoint, HEAD and the working tree): the user's uncommitted pre-run work stays uncommitted, pre-run staged changes in files the run edited stay staged (`git merge-file`; on a conflict the index takes the delivered content and a warning names the path); (1b) a failed checkpoint denies file-changing calls of runs that need rollback or delivery; (1c) the SVN provider is hardened (non-interactive, no credential cache, a private empty configuration directory, `--ignore-externals`, every contacted URL must lie within the project's repository URL, `file://` only with the provider key `allow_file_urls`); (1d) `runtime.default_test_command` / `default_lint_command` are validated at load; (1e) private temporary directories live in one per-process directory (`internal/proctemp`), stale ones are removed at startup; (1f) adopt and local clone take the caller's tenant area of the workspace root, `workspace.adopt_roots` (platform admins for adoption; outside the workspace root only) extends it; (P2/P3) see KI-82. Workspaces with submodules or a nested `.git` are refused for every Go git operation: [KI-88](#known-issues).
- [x] (2026-09-30) **KI-80 Copilot token handed to every user** (high): `POST /api/v1/copilot/exchange` returns the platform's GitHub Copilot token (with its expiry) to any authenticated user of any tenant and role. Evidence: `internal/adapter/http/handlers_llm.go` (`HandleCopilotExchange`), `internal/adapter/http/routes.go`. Found in the S6 tenancy work (2026-09-30); fix in progress. **Fixed (S6):** `/copilot/exchange` is platform-admin only and returns `{status, expires_at}` (the token never leaves the server; failures are logged server-side); `GET /llm/models` strips credential parameters (key, secret, token, password, credential, authorization, also nested); subscription provider connect/disconnect (the shared `.env` keys) are platform-admin only; the frontend hides shared LLM actions from other users (`is_platform_admin`). **Review round (S6-G, credential names):** any key ending in `_key` and the camelCase spellings count as credentials (`GET /llm/models` redaction, `RedactURL` in logs, which also redacts credential query parameters).
- [x] (2026-10-01) **KI-81 Auto-agent runs workspace tests inside the Go Core** (high): `AutoAgentService.runWorkspaceTest` (`internal/service/autoagent.go`) runs `python -m pytest` in the agent-writable workspace from the Go Core process with the core's full environment (JWT secret, database URL, LLM key encryption secret), so an agent-written `conftest.py` or pytest plugin runs as the core. Found in the KI-77 security review (2026-09-30); fix in progress (route the test run through the worker). **Fixed (S3-F):** the test run goes through the worker. The Go Core publishes `conversation.test.request` (workspace path, test file, timeout) and waits for `conversation.test.result`; the worker accepts only a `test_<name>.py` in the workspace root and runs it like a quality-gate command (allowlisted executable, tool environment without the worker's credentials, own process group, timeout). The worker sends the last 64 KiB of the output, the Go fix prompt keeps the last 16 KiB. Follow-up: [KI-86](#known-issues) (the waiter lives in the asking replica's memory).
- [x] (2026-10-01) **KI-82 Git config allowlist refuses common repositories** (medium): the KI-77 allowlist refuses whole repositories for benign keys such as `core.excludesfile`, `commit.template`, `rerere.enabled`, `log.date`, `tag.sort`; linked worktrees and submodule working directories are refused too (their `.git` is a file). Found in the KI-77 review (2026-09-30); allowlist extension in progress. **Fixed (S3-F):** the allowlist covers the inert keys of the common sections (identity, display, branch, LFS, credential, gc, rerere, ...) and, per key, `core`, `merge`, `diff`, `push`, `fetch`, `pull` and `checkout`; `core.excludesFile`, `commit.template`, `rerere.enabled`, `log.date` and `tag.sort` pass and a refusal names the key. Keys that name a transport program or make a remote a promisor are refused in every repository (`core.sshCommand`, `core.gitProxy`, `remote.*.uploadpack` / `receivepack` / `vcs` / `promisor` / `partialCloneFilter`, `extensions.partialClone`; operators configure ssh with `GIT_SSH_COMMAND`), `http.*`, `protocol.*` and `url.*` only for network operations; `GIT_NO_LAZY_FETCH=1` is set and every push runs with `push.recurseSubmodules=no` and `--no-recurse-submodules`. Linked worktrees and submodule working directories (a `.git` file) stay refused, see [KI-88](#known-issues). Files behind a filter driver (git-lfs, git-crypt) keep their content in checkpoints (the index is renormalised after `add -A`) and commit delivery refuses changes to filtered paths.
- [ ] **KI-83 LSP language servers run in the Go Core** (medium; high when `lsp.enabled`): language servers are started by the Go Core in the agent-writable workspace with the core's environment and are not contained; servers that load project plugins or run project tooling (TypeScript server plugins from `node_modules`, `go list` with workspace `go.env`/`GOFLAGS`) can execute agent-written code next to the JWT secret and DB credentials. Same class as KI-77/KI-81: run them in the worker or a separate container. Found in the KI-81 grep for core-side program starts (2026-10-01). The tool-user isolation of KI-71 covers only processes the worker starts; LSP servers are its open D5 follow-up.
- [x] (2026-10-02) **KI-84 Slack approval buttons do nothing** (low): the Slack feedback provider posted Approve/Deny buttons with no Slack interaction endpoint, and received every tenant's approval requests. Found in the KI-57 work and in S2-G (2026-10-01). **Fixed (S7-C):** the message has no buttons; it shows what the approval page shows (run, tool, command, path, profile, arguments preview) and links to `<notification.web_ui_url>/approvals/<run>/<call>`. The provider is registered only when `slack_webhook_url` and `web_ui_url` are both set. Slack and approval emails send only the requests of the tenants in `notification.approval_tenants` (`CODEFORGE_NOTIFICATION_APPROVAL_TENANTS`, tenant UUIDs; default: the default tenant); a request without a tenant is never sent. Agent text is inert in mrkdwn: `& < >` are escaped and backticks replaced; `@`, `://` and `www.` are broken by a word joiner, so no mention or link resolves; line breaks are flattened and values cut at 500 characters; link unfurling is off.
- [x] (2026-10-02) **KI-85 Inbound webhooks act only in the default tenant; no GitLab PM token** (medium): the VCS and PM routes checked one global secret per provider and ran in the default tenant, or in the tenant an `X-Tenant-ID` header named (the header was honoured on these unauthenticated routes). VCS webhooks found the project by a substring of the repository URL, and GitLab PM sync had no token. Found in the KI-56 work (2026-10-01). **Fixed (S7-C, with a review round):**
  - **Per-project webhooks.** Registration is `POST /api/v1/projects/{id}/webhooks` (admins). Editors can list webhooks, without secrets. Rotate, API token and delete are admin-only and audited. Migration 113 adds `webhook_endpoints` and `webhook_deliveries`.
  - **Own ID, URL and secret.** Each webhook has a random ID and its own URL, `/api/v1/webhooks/{vcs|pm}/{provider}/{id}`. Its secret is random and shown once; for Plane it is the secret Plane generates. It is stored AES-256-GCM encrypted with an HKDF key from `auth.jwt_secret`, like VCS account tokens.
  - **Tenant from the ID.** The ID names tenant and project, and the signature is checked with that webhook's secret in constant time. Unknown IDs and wrong signatures get one uniform 401. `X-Tenant-ID` is never read on webhook routes.
  - **Exact repository match.** An event must name the project's repository exactly (host and path, case-insensitive; Plane: `plane_project_id`). Other events are ignored and logged. The repository-name lookups are removed from the store.
  - **Own API tokens.** PM integrations carry their own API token (GitLab `PRIVATE-TOKEN`, GitHub `GH_TOKEN`, Plane `api_token`). The operator's Plane token and the Core's gh login serve only the default tenant, on every path: webhook syncs; `POST /projects/{id}/roadmap/import/pm` (github-issues and plane get 400 outside the default tenant); `POST /projects/{id}/roadmap/sync` (github-issues needs `provider_config.token` outside the default tenant). Errors shown to a tenant never name the operator's `plane.base_url`.
  - **GitLab outbound policy.** The GitLab PM provider's base URL is chosen by tenants: a project's `repo_url` host, or a manual sync's `base_url`. It connects through `netutil.OutboundPolicy` (`pm.allowed_private_hosts`, default none; cloud metadata never reached; loopback only with an explicit entry; no proxy; set once at startup, `cmd/codeforge/pm_outbound.go`). It follows redirects only within the origin (`netutil.SameOriginRedirect`, shared with the MCP connection test) and reads at most 10 MiB. Its errors name only the method, the origin and the status code or kind of failure: never the body, the token or the URL's userinfo or query.
  - **Each delivery handled once.** Within `webhook.delivery_retention` (default 168h), a delivery is handled once. It claims its body hash and its delivery ID (`X-GitHub-Delivery`, `X-Gitlab-Event-UUID`, `X-Plane-Delivery`), so both a provider's redelivery and a replay of a signed delivery under any delivery ID are duplicates. A delivery that failed, or that named another repository, is forgotten, so its redelivery runs. VCS and PM deliveries bypass the `Idempotency-Key` response cache. After the retention, a stored signed delivery could be replayed: GitHub and Plane sign no timestamp.
  - **Old routes.** The old global routes answer 410. The old `webhook.*` secret settings are ignored with a startup warning.
  - **Review round.** Fixed: replay of a signed delivery under another delivery ID, the GitLab outbound policy, the operator's PM credentials kept to the default tenant, deliveries and the tenant middleware in the idempotency chain (findings 1 to 3 and 5 to 8); finding 4 did not reproduce.
- [x] (2026-10-04) **KI-86 Go Core assumes a single replica** (low): pending HITL approvals and auto-agent workspace-test waiters live in memory of the Go process that asked, while results arrive on shared durable consumers; with two replicas a decision or test result can land on the other replica and is dropped (the run then times out). Run one Go Core replica, or move these waits to per-request subjects / shared state. Found in the S3-F review (2026-10-01). **Fixed (S7-F):** Go Core replicas hand results and decisions to the replica that waits, over core-NATS request-reply on `core.relay.<sha256(key)>` (`messagequeue.Relay`, implemented by the NATS queue, outside the stream; only the `core` user may use it). This covers HITL approvals (decide and the approval page), the auto-agent workspace test and conversation completion, and every syncWaiter result (retrieval, sub-agent and graph search, memory recall, context rerank, backend health). Approval keys contain the tenant; only `allow`/`deny` are relayed. Still per replica: KI-139. **Review fixes (S7-F R2, 2026-10-04):** a replica acts only on relay requests whose reply is a Go Core inbox (`_INBOX_core.`). The NATS server checks neither a push consumer's deliver subject nor the reply subject of `MSG.NEXT`/`DIRECT.GET` requests, and the relay subject follows from the key, so the worker could have a message it published to `runs.output` delivered to `core.relay.<hash>`, where the waiting replica took it for an `allow` decision. Such deliveries carry `$JS.ACK.*` or no reply and are now dropped with a warning (regression tests against the production config for all three ways). A relayed result is the worker's message as it came (re-encoded without HTML escaping otherwise), so it fits the payload limit like the original.
- [x] (2026-10-04) **KI-87 SVN password on the command line** (low): the SVN provider passes `--password` as an argument, visible in `/proc/<pid>/cmdline` to processes of the same container (agent tools; the separate tool UID of KI-71 does not hide it, `/proc/<pid>/cmdline` is world-readable). Pass it via `--password-from-stdin` (svn 1.10+) or an auth file in the private config directory. Found in the S3-F work (2026-10-01). **Fixed (S7-F):** the password reaches svn on stdin (`--password-from-stdin`, svn 1.10+), never in argv; a password with a line break is refused. Tested against a real svnserve.
- [x] (2026-10-01) **KI-15 A2A and handoff trust gates are bypassed** (medium): The A2A endpoints and AgentCard sit behind the global JWT middleware, so A2A API keys are rejected, and inbound A2A prompts are stored as untrusted but published without quarantine. `HandoffService` is never constructed, so Python handoffs skip quarantine, inbox delivery and `handoff.status` events (War Room handoff arrows never render). Evidence: `cmd/codeforge/main.go:921`, `internal/middleware/auth.go:27-38`, `internal/adapter/a2a/executor.go:64-84`, `internal/service/handoff.go:36`. **Fixed (S6):** A2A callers authenticate with A2A API keys instead of a user's JWT (`middleware.A2AAuth`; `/a2a` and the AgentCard are exempt from the JWT middleware): an `a2a.api_keys` entry is `<key>` (default tenant) or `<tenant-uuid>:<key>`, the caller acts in its key's tenant with partial trust, without keys every request gets 401, and the AgentCard is open only with `a2a.allow_open`. An inbound task is visible only to the key that created it (migration 103; the key ID is `key-` plus 16 hex characters of the key's SHA-256). Inbound A2A prompts go through the quarantine: the task is created first and records the held message (`quarantine_message_id`), Approve replays the prompt only while the task still waits for it (otherwise the message is rejected and the call answers 409), Reject rejects the task, a cancel by the caller withdraws the message, and a prompt its task cannot record is withdrawn. `HandoffService` is constructed and the Go Core handles `handoff.request`: source and target are checked in the request's tenant and project, the handoff is screened by the quarantine, the target agent's configured mode wins (an unknown requested mode is refused), a task and a run are started and tracked like any run, the target gets an inbox message and `handoff.status` carries `run_id` and one of `initiated`, `quarantined`, `rejected`, `failed`, `a2a_delegated`. Each stage (request, approval after the quarantine) is claimed once (`handoff_claims`, migrations 102, 106, 107: an unfinished claim is taken over after an 11-minute lease, a retry reuses the stage's task, permanent start errors refuse at once) and dead-lettered `handoff.request` / `handoff.approved` end the handoff as failed. The same S2-F/S2-G review rounds hardened task and conversation delivery, see the 2026-10-01 entry at the top and [ADR-016](architecture/adr/016-nats-delivery-semantics.md). Follow-ups: [KI-90](#known-issues) to KI-92.
- [x] (2026-09-30) **KI-16 Experience pool has no tenant isolation and cannot be disabled** (medium): The worker always creates one `ExperiencePool` with the zero-UUID tenant and ignores `experience.enabled` / `CODEFORGE_EXPERIENCE_ENABLED` (documented default: off), `confidence_threshold` and `max_entries`, so all experiences land in the zero tenant and similar prompts can be answered from cache without running the agent loop. Evidence: `workers/codeforge/consumer/__init__.py:137`, `workers/codeforge/memory/experience.py:39`, `internal/config/config.go:229-233`. **Fixed (S6):** the worker honours `experience.enabled` (default off), `confidence_threshold` and `max_entries`; every pool statement filters by the conversation's `tenant_id` (no tenant, no cache); the agent loop never reads or writes the cache (a cached final message would claim work that was not done); only the first turn of a simple chat (one text-only user message, `agentic=false`) can be answered from the cache, streamed like a live answer, cost 0; cache errors fall back to the normal chat.

#### Messaging and runtime

- [x] (2026-09-30) **KI-18 NATS delivery is unsafe** (high): Go durable consumers have a 5 min inactivity threshold and deliver from the stream start, so after more than 5 min of Go Core downtime they are recreated and replay up to 30 days of messages (duplicate conversation messages, re-finalized runs). Python work handlers ack only at the end with a 30 s ack wait and unlimited redelivery, so with several workers any run longer than 30 s executes twice, and per-run cancel listeners are never unsubscribed and can exhaust the 200-consumer limit. Evidence: `internal/adapter/nats/nats.go:240-251`, `workers/codeforge/consumer/__init__.py:209-234`, `workers/codeforge/runtime.py:72-96`. **Fixed (S2, [ADR-016](architecture/adr/016-nats-delivery-semantics.md)):** Go and worker ensure one shared durable pull consumer per subject and side (`codeforge-go-*` / `codeforge-py-*`) without inactivity threshold; a new durable starts at new messages, an existing one keeps its position (no replay on recreation); `MaxDeliver` 4, `AckWait` 90 s, in-progress heartbeats every `AckWait/3` while a handler runs. `runs.start`, `conversation.run.start`, `tasks.agent.*` and `benchmark.run.request` are accepted with a confirmed ack (at-most-once; a failure is reported as a failed completion with publish retries, never re-executed); the DLQ copy drops `Nats-*` headers so JetStream dedup cannot swallow it; in-progress acks are lazy and capped (Go: approval timeout + 5 min). Per-run cancel listeners and heartbeats are released by `RuntimeClient.close()`. Gaps: KI-65 (no Go-side watchdog for conversation runs), KI-67. Test fix (2026-10-02): `TestQueue_RestartDoesNotReplayOrLose` published the "while down" message before the server had dropped the stopped subscriber's pull request, so under CI load the message went to that request and came back only after `AckWait`; the test now waits until the durable has no pull requests (`NumWaiting` 0).
- [x] (2026-09-30) **KI-19 Python retry/DLQ path is unreachable** (medium): Retries are counted from a `Retry-Count` header nobody sets, so failed `tasks.agent.*` messages are NAK'd forever and never dead-lettered; invalid payloads are NAK'd without delay in a hot loop, and failures after the dedup mark are acked as duplicates on redelivery. Go dead-letters handler errors, but messages that exhaust `MaxDeliver` through ack timeouts are dropped without a DLQ copy. Evidence: `workers/codeforge/consumer/_base.py:62-70`, `workers/codeforge/consumer/_tasks.py:67-76`, `workers/codeforge/consumer/_base.py:145-160`. **Fixed (S2):** retries use `msg.metadata.num_delivered` (NAK with 2 s delay before the last attempt, then publish to `{subject}.dlq` and ack); invalid payloads go to the DLQ at once and are terminated (`msg.term()`) in every handler; a failed DLQ publish NAKs instead of acking; a failed request is removed from the dedup cache so its redelivery runs. Go uses the same rules (`Retry-Count` header removed). Also fixed: `_publish_error_result` of the base mixin shadowed the conversation one (TypeError killed the conversation loop), the compact handler acked twice.
- [x] (2026-09-30) **KI-20 Schema validation skips run and context subjects** (low): `Validate()` accepts any JSON on `runs.*` and does not check `context.*` / `repomap.*`, although payload structs exist for them. Evidence: `internal/port/messagequeue/validator.go:14-17`, `internal/port/messagequeue/validator.go:122-133`. **Fixed (S2):** every `runs.*`, `context.*` and `repomap.*` subject with a port struct is validated against it (`runs.cancel`, `runs.trajectory.event` stay JSON-only); a `null` payload is rejected like `{}`.
- [x] (2026-09-30) **KI-65 At-most-once work has incomplete Go-side watchdogs** (medium): `conversation.run.start` (and since the S2 review `tasks.agent.*`) is acked on accept (ADR-016), and the worker's wall-clock timeout dies with the worker, so a crashed worker leaves the conversation "running" until the user stops it. The `runs.start` timer does not cover policies without `TimeoutSeconds`, runs in `quality_gate`, or any run after a Go Core restart (in-memory timer); `HeartbeatTimeout` is only evaluated when a tool call arrives. Evidence: ADR-016 section 6, `internal/service/runtime.go` (`StartRun` timer). Found during the KI-18 fix (2026-09-30). `tasks.agent.*`: `ActiveWorkService.ReleaseStaleWork` only resets a task without a row update for 30 min to `pending` (not failed, agent status not reset, a healthy long task is released too). SIGTERM: the worker's `stop()` drains NATS while at-most-once handlers still run, so their completions can be lost (the give-up path fails them, shutdown does not; the grace would have to fit Docker's `stop_grace_period`). A `tasks.cancel` published while the task still waits in NATS is lost, so the task runs later (found in the KI-22 fix). **Partly fixed (S3):** runs stuck in `quality_gate` are failed by the stuck-work watchdog (gate heartbeats, NATS backlog probe); the other parts are in progress (S2 follow-ups). **Fixed (S2):** heartbeats (with `tenant_id`; conversation runs with `turn_id`; backend tasks via `tasks.heartbeat`) are stored (migration 098); the stuck-work watchdog stops lost runs as `timeout`, fails lost conversation turns and lost tasks (agent reset) after `heartbeat_timeout + 2 x heartbeat_interval`; the active conversation turn is persisted; `ReleaseStaleWork` removed; SIGTERM fails accepted work before NATS is drained (`stop_grace_period: 45s`); a `tasks.cancel` for a queued task is remembered (stream-sequence registry). Review round in progress (tasks that are never accepted, unconfirmed accepts on the last delivery, turn checks after a lost-worker end, heartbeat interval sent to the worker).
- [x] (2026-09-30) **KI-66 Worker dedup keys are too coarse** (medium): the worker dedups `repomap-{project_id}`, `retidx-{project_id}`, prompt-evolution and handoff requests by project or source run, so a second legitimate request for the same project is skipped as a duplicate while the key stays in the 10,000-entry cache. Evidence: `workers/codeforge/consumer/_repomap.py`, `_retrieval.py`, `_handoff.py` (`_is_duplicate`). Found during the KI-19 fix (2026-09-30). **Fixed (S2):** at-least-once requests are deduplicated per message (key plus stream sequence): repo map, index, graph, prompt evolution, memory, handoff; at-most-once work keeps its run-ID key.
- [x] (2026-09-30) **KI-67 Worker consumer lifecycle gaps** (low): the worker does not recreate a durable that is deleted while it runs (its fetch loop stops after repeated errors; Go has a health monitor), it creates the `CODEFORGE` stream with default settings when missing (the Go Core should own the stream config), Go keeps heartbeat entries for conversation run IDs forever, and notification subjects still use per-run ephemeral JetStream consumers instead of core NATS. Found during the KI-18 fix (2026-09-30). **Partly fixed (2026-09-30):** a deleted durable is re-attached, the error counter resets on healthy fetches, a loop that gives up fails the worker's unfinished work and exits 1; notification consumers use ack policy `none`. Still open: stream created with default settings by the worker, Go conversation heartbeat entries kept forever, notifications on per-run JetStream consumers instead of core NATS, threads (repo map, retrieval, graph) delay the exit after give-up. **Fixed (S2):** the worker waits up to 120 s for the Go Core to create the stream instead of creating it; conversation heartbeats are stored per active turn, not kept in memory; the worker exits without waiting for indexing threads. Left on purpose: notifications stay on per-run JetStream consumers (the cancel registry needs stream sequences, which core NATS does not have; per-run consumers are released since KI-18).
- [x] (2026-09-30) **KI-21 `runs.start` is a single LLM completion; late approvals are lost** (medium): The run path makes one policy request and one LiteLLM completion (no tool loop, no agent backend, no file changes), so quality gates and delivery run against an unchanged workspace. When a tool call resolves to ask, the worker gives up after 30 s while Go waits 60 s for HITL approval, so approvals given after 30 s are lost. Evidence: `workers/codeforge/executor.py:191-259`, `workers/codeforge/constants.py:24`, `internal/service/runtime_approval.go:25-29`. **Fixed (S2):** `runs.start` runs `AgentLoopExecutor` with the default tools (without skill tools) plus the run's MCP servers in the payload workspace, sharing the conversation path's loop setup (fallback models, routing layer feeding the MAB router, tool guide, local sampling parameters; Claude Code models are excluded); every LLM and tool call is decided by Go; a missing or unusable workspace fails the run before any LLM call; runs send heartbeats; runs no longer use the experience pool. The approval timeout has one source (`config.Runtime.ApprovalTimeout()`), is sent as `approval_timeout_seconds` on `runs.start` and `conversation.run.start`, and the worker waits that long plus 15 s for a decision.
- [x] (2026-09-30) **KI-22 Subjects without a working counterpart** (medium): `tasks.cancel` is published by every backend `Stop()`, but no worker path cancels the running Aider/OpenHands/Goose/OpenCode/Plandex process, and `review.trigger.request` has a Python consumer that Go never publishes to. Evidence: `internal/adapter/aider/backend.go:62-72`, `workers/codeforge/consumer/__init__.py:188`, `internal/port/messagequeue/queue.go:124`. **Fixed (S2):** backend tasks run as their own asyncio task with a per-task `tasks.cancel` listener (one `runtime.listen_for_cancel` helper for runs and tasks); CLI backends start in their own process group, and cancel, timeout and errors stop the whole group (SIGTERM, 5 s, SIGKILL, guarded against PIDs <= 1 and the worker's own group); OpenHands deletes its remote conversation; a `cancelled` result keeps the task cancelled, and task results follow the reported status (a `failed` result without an error message is failed). `StopTask` is tenant- and project-checked. `review.trigger.request/complete` were removed on both sides (Go never published the request; review triggers belong to `ReviewTriggerService`, KI-17). Open: a `tasks.cancel` for a task still queued in NATS is lost (KI-65).
- [x] (2026-09-30) **KI-23 Run and backend-task payloads lack workspace and backend** (medium): `RunStartPayload` and the `tasks.agent.*` payload carry no `workspace_path` or backend, so backend tasks always get `workspace_path ""`, and the production compose has no workspace volume shared by core and worker. Evidence: `internal/port/messagequeue/schemas_run.go:44-62`, `workers/codeforge/consumer/_tasks.py:50`, `docker-compose.prod.yml:157-243`. **Fixed (S2):** `RunStartPayload` carries `workspace_path` and `backend`; `tasks.agent.*` carries `TaskAgentPayload` (`task_id`, `project_id`, `tenant_id`, `agent_id`, `title`, `prompt`, `backend`, `workspace_path`; the worker still accepts the old `id` key); `POST /runs` and agent dispatch answer 400 for a project without a workspace or a task/agent of another project; handoff requests carry the source run's `workspace_path` and `approval_timeout_seconds` (a handoff without a workspace fails at handoff time or is dead-lettered). The prod compose already shares `/data/workspaces` between core and worker (KI-45); a project adopted outside that volume is not visible to the prod worker.
- [x] (2026-09-30) **KI-24 A stopped conversation stays cancelled forever** (high): `StopConversation` stores the conversation ID in `RunStateManager.cancelledConvs` and nothing clears it; conversation runs reuse the conversation ID, so every later tool call in that conversation is denied until the Go Core restarts (a side effect of the 2026-03-19 Bug 2 fix below). Evidence: `internal/service/run_state.go:125-131`, `internal/service/runtime_execution.go:168-170`. **Fixed (S2):** each conversation dispatch records a turn (`turn_id` on `conversation.run.start`, `runs.toolcall.request` and `conversation.run.complete`) before anything is stored or published; tool calls of a stopped turn are denied ("conversation run ended") while the next run's calls are evaluated; per-conversation run state is deleted once empty (completion, failed dispatch, stopped run reporting its end, conversation delete). One run per conversation: a second message while a run is active gets 409 "conversation run in progress" (the chat keeps the text and explains).
- [x] (2026-09-30) **KI-30 Termination, stall and cancel paths leave plans hanging** (medium): The termination-limit, stall-detection and user-cancel paths complete the run without calling `onRunComplete`, so execution-plan steps stay running; the termination path also skips the task/agent reset and run-state cleanup. Evidence: `internal/service/runtime_execution.go:50-58`, `internal/service/runtime_execution.go:328-345`, `internal/service/runtime.go:475`. **Fixed (S2):** one completion path (`finalizeRun`); control-plane stops (`stopRun`: user cancel, timeout, termination limit, budget, stall) mark the run "stopping", tell the worker first (`runs.cancel` on a detached ctx), keep the stored outcome and their own status/reason, then finalize and wake HITL waiters; `onRunComplete` advances plans; a failed or cancelled step ends sequential/parallel plans as failed and skips blocked dependents; a step that cannot start ends the plan; `advancePlan` re-reads the plan under the scheduling lock; `ReplanStep` (still unwired, KI-62) starts a new run for an unsuccessful step and unblocks its dependents; routed debate steps no longer deadlock. Cancels and failed starts do not count as agent failures.

#### Quality gates and delivery

- [x] (2026-09-30) **KI-26 Delivery requires a passed quality gate** (high): `triggerDelivery` is only called after a passed gate, so presets without gates (`trusted-mount-autonomous`, `supervised-ask-all`, `plan-readonly`) never deliver despite `deliver_mode`, and a failed gate without rollback still ends the run `completed`. `HandleRunComplete` has no terminal-state guard, so late or replayed completions overwrite cancelled or timed-out runs. Evidence: `internal/service/runtime_completion.go:172`, `internal/service/runtime_completion.go:184`, `internal/service/runtime_completion.go:15-20`. **Fixed (S3):** delivery runs in `endRun` for every run stored `completed` (with or without gates), before checkpoint cleanup; a failed gate always fails the run and never delivers; `HandleRunComplete` ignores runs waiting in `quality_gate`; only a check that ran and failed rolls back (with `rollback_on_gate_fail`) and counts against the agent; gate audit entries, events and broadcasts come only from the path whose completion write won.
- [x] (2026-09-30) **KI-27 Shadow checkpoints corrupt delivery** (high): Checkpoints are real commits in the workspace that are removed after delivery by `git reset --soft <first checkpoint>^`, so commit-local delivery is erased (changes left staged), branch/PR delivery pushes the checkpoint commits, and patch delivery (`git diff HEAD`) contains only the last edit. Evidence: `internal/service/runtime_execution.go:136-144`, `internal/service/checkpoint.go:139-157`, `internal/service/deliver.go:79`. **Fixed (S3):** checkpoints are working-tree commits built from a private per-run index with `commit-tree`, chained under `refs/codeforge/checkpoints/<run>` (never on a branch; HEAD, branches and the user's index untouched); the base checkpoint records the user's index and the pre-run HEAD (branch, detached or unborn); the ref chain is the durable record (rollback and cleanup work after a restart); rollback restores tree, index and HEAD; patch delivery diffs the base against the working tree into `.git/codeforge/patches/<run>.patch`. Checkpoints use a fixed identity, no signing, no hooks.
- [x] (2026-09-30) **KI-28 Quality gate timeout is ignored** (medium): `runtime.quality_gate_timeout` / `CODEFORGE_QG_TIMEOUT` is never sent to the worker, which uses a fixed 120 s per command and does not kill a timed-out process; runs in `quality_gate` status have no watchdog, so a lost gate result leaves the run there forever. Evidence: `internal/config/loader.go:201`, `workers/codeforge/qualitygate.py:127-129`, `internal/service/runtime_lifecycle.go:33-35`. **Fixed (S3):** `timeout_seconds` on the gate request; the worker runs each command in its own process group and kills the group on timeout; `heartbeat_seconds` makes the worker send `runs.heartbeat` (phase `quality_gate`) and keep the request in progress; the stuck-work watchdog (`internal/service/stuck_work_watchdog.go`, every `runtime.stale_check_interval`) fails a gated run silent for 2 min when the NATS backlog probe shows nothing queued, otherwise after a bounded cap; a partial index serves the sweep (migration 097). `quality_gate_timeout` must be at least 1s.
- [x] (2026-09-30) **KI-29 Quality gates run Go commands only and fail open** (medium): Gates always use the Go defaults (`go test ./...`, `golangci-lint run ./...`) whatever the project language, and a missing test or lint result counts as passed. Evidence: `internal/service/runtime_completion.go:80-82`, `internal/service/runtime_completion.go:155-157`. **Fixed (S3):** gate commands come from the project config `test_command` / `lint_command` (validated on create/update: shlex-parseable, executable on the worker's allowlist, else 400), then from the language whose test runner is set up at the workspace root, then from `runtime.default_test_command` / `default_lint_command` (default now empty); a required check without a command or without a result fails the gate; the worker reports a check that could not run (no/invalid/disallowed command, cannot start, timeout) as a null verdict with an `error`; an unknown profile fails the run.

#### Persistence

- [x] (2026-09-30) **KI-31 No optimistic locking on runs, plans and teams** (medium): Run, plan and team updates have no version check or status predicate, and `HandleToolCallRequest` writes `running` after the up-to-60 s HITL wait, so a run cancelled or timed out meanwhile is moved back to `running`. Evidence: `internal/adapter/postgres/store_run.go:38-53`, `internal/service/runtime_execution.go:148`. **Fixed (S2):** run status writes follow a transition table (`run.SourceStatuses`/`CanTransition`: running from pending/running, quality_gate from running, terminal from any active status, nothing out of a terminal status) enforced in SQL (`status = ANY($n)`), plans and teams refuse writes from terminal states (`domain.ErrConflict`, HTTP 409 for `CancelRun` unless already cancelled); tool-call steps are counted only while running (`CountRunStep`), usage only while running and never lowered (`AddRunUsage`, `RaiseRunUsage`, redelivered results counted once); a call approved after its run ended is denied; failed starts end the run as failed.
- [x] (2026-09-30) **KI-32 Result and plan events are not persisted** (medium): Task result events are appended with agent_id `""` (error only logged) and `plan.*` events with empty agent_id/task_id (error discarded), which the UUID NOT NULL columns reject. Evidence: `internal/service/agent.go:176-181`, `internal/service/orchestrator_consensus.go:402-408`, `internal/adapter/postgres/migrations/004_create_agent_events.sql:4-7`. **Fixed (S2):** migration `091_agent_events_optional_agent_task.sql` makes `agent_id`/`task_id` nullable, `Append` stores NULL for missing IDs and `{}` for a nil payload, result events record the task's agent, append errors go through `logBestEffort`; `UpdateTaskResult` writes the task status with the result.
- [x] (2026-10-01) **KI-33 Teams are never cleaned up** (low): `CleanupTeam` is only called from tests, so teams stay `initializing` forever and their agents are not released. Evidence: `internal/service/pool_manager.go:172-196`. **Fixed (S6):** a team ends with its plan (completed, failed or cancelled; the plan-end callbacks run after the scheduling lock is released), and the watchdog check "ended teams" (also once at startup) ends the teams of plans that ended while the Go Core was down.

#### Worker and observability

- [x] (2026-09-30) **KI-34 Worker health check breaks the production worker** (high): There is no HTTP health endpoint (`health.py` is never imported, `CODEFORGE_WORKER_HEALTH_PORT` is unused), and the replacement sentinel file in `/tmp` cannot be written under the prod compose's `read_only: true` without tmpfs, so the worker crashes at startup and restart-loops while the compose healthcheck only tests importability. Evidence: `workers/codeforge/health.py:9`, `workers/codeforge/consumer/__init__.py:90`, `workers/codeforge/consumer/__init__.py:204`, `docker-compose.prod.yml:210`. **Fixed (S4):** the worker serves `GET /health` (liveness) and `GET /health/ready` (NATS connected, every consumer loop alive, not given up, not stopping) on `CODEFORGE_WORKER_HEALTH_PORT` (default 8081); the sentinel file is gone; an unbindable port or a failing `start()` exits 1; `scripts/worker-healthcheck.py` checks readiness in the Dockerfile and the prod compose (`start_period` 30 s). `main()` awaits its shutdown (NATS drained, final log lines written); a stop request is sticky, so SIGTERM during startup stops the worker before any consumer starts and a second signal always works.
- [x] (2026-09-30) **KI-35 Python log schema differs from Go** (low): Worker logs use structlog's `event`/`timestamp`/lowercase `level` instead of the Go slog schema (`msg`/`time`/`level`), and stdlib loggers (httpx, nats) print plain text to the same stream. Evidence: `workers/codeforge/logger.py:42-57`, `workers/codeforge/logger.py:29`. **Fixed (S4):** one formatter renders structlog and stdlib records (httpx, nats) in the Go slog schema: `time` (UTC, ms), `level` (DEBUG/INFO/WARN/ERROR), `msg`, `service`, `logger`, attributes; tracebacks in an `exception` field, one JSON object per line, async writing kept; URL userinfo is redacted across the whole rendered line (nested values included); nothing prints before logging is set up (dev-key warning and tracing status moved into `main()`).
- [x] (2026-09-30) **KI-36 OTEL export is incomplete** (medium): The Go exporter never calls `WithInsecure()`, so the default TLS credentials win and `CODEFORGE_OTEL_INSECURE=true` cannot reach a plaintext collector such as the dev Jaeger; Python metrics have no MeterProvider, and the worker never injects trace context into published messages. Evidence: `internal/adapter/otel/setup.go:53-63`, `workers/codeforge/tracing/metrics.py:7-43`, `workers/codeforge/tracing/propagation.py:26`. **Fixed (S4):** the Go exporters use `WithInsecure()` when `otel.insecure` is set; the worker default is now false like Go (a worker exporting to a plaintext collector needs `CODEFORGE_OTEL_INSECURE=true`); the worker installs an OTLP MeterProvider (a setup error is logged, not fatal; shutdown bounded and off the event loop) and `TracingJetStreamContext` adds `traceparent` to every worker publish.
- [x] (2026-09-30) **KI-37 Verifier metrics always score 0.0** (medium): `trajectory_verifier` and `logprob_verifier` import `litellm`, which is not a worker dependency, and catch the failure, so benchmark runs using them (and the hybrid pipeline's rank stage) always get 0.0. Evidence: `workers/codeforge/evaluation/evaluators/trajectory_verifier.py:155`, `workers/codeforge/evaluation/evaluators/logprob_verifier.py:89`. **Fixed (S6):** both verifiers call the proxy through the worker's `LiteLLMClient.chat_completion` (with `logprobs` / `top_logprobs`); a failed call or unparseable answer is an evaluation error (`<evaluator>_error`, `EvalDimension.error`) and is left out of every average instead of scoring 0.0. Follow-up: a dedicated error field in the Go result, database and UI.
- [x] (2026-09-30) **KI-38 Tool configuration drift** (low): The capability allowlist names `handoff` but the tool is registered as `handoff_to`, so `api_with_tools` models lose the handoff tool, and the Go settings `agent.builtin_tools` and `agent.tool_output_max_chars` are never read. Evidence: `workers/codeforge/tools/capability.py:86`, `workers/codeforge/tools/handoff.py:24`, `internal/config/config.go:169`, `internal/config/config.go:174`. **Fixed (S6):** the capability allowlist names `handoff_to`; `agent.tool_output_max_chars` is sent with `conversation.run.start` and used for history truncation; `agent.builtin_tools` was removed (mode tool lists choose the tools).

#### Frontend

- [x] (2026-09-30) **KI-39 Live updates are not wired** (medium): Run, plan and agent panels are never refreshed by WebSocket events (only toasts), the project page filters `task.output` on a `project_id` the event never carries so live output stays empty, and `AgentLane` appends every task's output to every lane. Evidence: `frontend/src/features/project/useProjectDetail.ts:120-128`, `frontend/src/features/project/useProjectDetail.ts:144-145`, `frontend/src/features/project/AgentLane.tsx:47-52`. **Fixed (S5):** typed parsers/reducers (`frontend/src/features/project/liveEvents.ts`); live output is filtered by the project's task IDs (a task belongs to one project and its `task.status`/`run.status` event precedes any output; decision recorded in the fix plan); `run.status` carries `agent_id` and `step_count`; RunPanel, PlanPanel (incl. review decisions and debate badges), AgentPanel and WarRoom update on their events; each AgentLane shows only its own task's output and run's tool calls. Follow-ups: KI-74.
- [x] (2026-09-30) **KI-40 API client mismatches** (medium): The UI deletes models with `DELETE /api/v1/llm/models/{id}`, but only `POST /api/v1/llm/models/delete` exists, and type errors hide calls to non-existent methods (`api.mcp.listProjectServers`, `assignToProject`, `unassignFromProject`) that throw when MCP server assignment is used. Evidence: `frontend/src/api/resources/llm.ts:25-26`, `internal/adapter/http/routes.go:421`, `frontend/src/features/project/CompactSettingsPopover.tsx:42`. **Fixed (S5, D12):** `DELETE /api/v1/llm/models/{id}` (admin, audited) replaces `POST /llm/models/delete` (now 405); the model list takes the deployment ID from LiteLLM's `model_info.id`, so Delete is shown. The MCP client methods were restored in S0.
- [x] (2026-09-30) **KI-41 Settings popover wipes the project config** (medium): The compact settings popover saves `{config: {autonomy_level}}` and `ProjectService.Update` replaces the whole config map, dropping `policy_preset`, `detected_languages` and `expansion_prompt`; `autonomy_level` itself is read by no backend code. Evidence: `frontend/src/features/project/CompactSettingsPopover.tsx:107-113`, `internal/service/project.go:183-184`. **Fixed (S5):** `PUT /projects/{id}` merges `config` (a key with a value is set, JSON `null` deletes it, other keys are kept); the dead autonomy control was removed from the popover (autonomy comes from the mode). `CreateProjectModal` still writes the unused `autonomy_level` key (KI-74).
- [x] (2026-09-30) **KI-42 Channel real-time events are never broadcast** (low): `channel.message` / `channel.typing` / `channel.read` are defined but never sent and the channel UI has no WebSocket subscription, so messages from other users, agents or webhooks appear only after a reload. Evidence: `internal/domain/event/broadcast.go:101-103`, `frontend/src/features/channels/ChannelView.tsx:47-75`. **Fixed (S5):** `ChannelService.SendMessage` broadcasts `channel.message` tenant-scoped for messages, thread replies and webhooks; ChannelView appends live (including messages that arrive while the list loads). Found and fixed on the way: no channel message could be stored at all (`channel_messages.tenant_id` NOT NULL was never set), a message could be posted into another tenant's channel, senders could pose as agents or other users (now the authenticated user), a thread parent of another channel/tenant was accepted. `channel.typing`/`channel.read` have no producer; follow-ups: KI-73.

#### Deployment and configuration

- [x] (2026-09-30) **KI-43 `postgres:18-alpine` rejects the data volume path** (high): Dev and prod compose mount the data volume at `/var/lib/postgresql/data`, which the PostgreSQL 18 image refuses (it expects `/var/lib/postgresql`), so PostgreSQL exits before initdb and nothing that depends on it starts. Evidence: `docker-compose.yml:84`, `docker-compose.prod.yml:36`. **Fixed (S4):** both compose files mount the data volume at `/var/lib/postgresql` (PG 18 keeps its cluster in `/var/lib/postgresql/18/docker`); the dev WAL archive moved to `/var/lib/postgresql/archive`. An existing PG <= 17 volume is refused by the PG 18 entrypoint ("there appears to be PostgreSQL data in /var/lib/postgresql"): dump it with the old image and restore, or `pg_upgrade --link` (see dev-setup.md).
- [x] (2026-09-30) **KI-44 Production PostgreSQL enables SSL without certificates** (high): The prod compose sets `ssl=on` with `server.crt` / `server.key` that no script or mount creates, so PostgreSQL refuses to start (audit INFRA-002 still open). Evidence: `docker-compose.prod.yml:56-60`. **Fixed (S4):** `generate-secrets.sh` creates a self-signed certificate (RSA-3072, SAN `postgres`, replaceable by a CA-signed pair); a root entrypoint wrapper copies cert and key (0600) to a postgres-owned tmpfs and fixes the archive volume ownership; clients keep `sslmode=require` (`verify-full` works with the generated cert). Core and worker run with `APP_ENV=production`.
- [x] (2026-09-30) **KI-45 Read-only production core has no writable workspace** (high): The core runs with `read_only: true`, no volume or tmpfs and the relative default `data/workspaces`, so cloning a repository and writing the initial admin password file fail, and no workspace survives container recreation. Evidence: `docker-compose.prod.yml:163`, `internal/config/config.go:487`, `internal/config/config.go:590`. **Fixed (S4):** prod volumes `core_data` at `/data` and `workspaces` at `/data/workspaces` (shared with the worker at the same path), tmpfs `/tmp` for core and worker; images run as UID/GID 10001 and create `/data/workspaces`.
- [x] (2026-09-30) **KI-46 Production secrets handling is broken** (high): Mounted Docker secret files are never read by the Go Core (`secrets.FileProvider` / `Auto()` are test-only) while the prod compose requires every secret as `${VAR:?}` env (visible in `docker inspect`); neither the JWT secret nor `CODEFORGE_INTERNAL_KEY` is passed to core (users are logged out on every restart, worker-to-core calls get 401). `scripts/validate-env.sh` checks `CODEFORGE_JWT_SECRET`, which nothing reads (the real variable is `CODEFORGE_AUTH_JWT_SECRET`). Evidence: `internal/secrets/provider.go:61`, `cmd/codeforge/main.go:294`, `docker-compose.prod.yml:157-178`, `scripts/validate-env.sh:4`. Also: `scripts/generate-secrets.sh` writes base64 values (`/`, `+`, `=`) that `docker-compose.prod.yml` embeds unescaped in the `DATABASE_URL` / `NATS_URL` userinfo, and the prod worker gets no `CODEFORGE_INTERNAL_KEY`. **Fixed (S4):** Go reads `<KEY>_FILE` for secret settings (`secrets.LookupFileEnv`; setting both `KEY` and `KEY_FILE` is a startup error; missing/empty files fail); prod compose passes only `*_FILE` paths (JWT secret, internal key, LLM key encryption secret, DB/NATS URLs, ...), Postgres uses `POSTGRES_PASSWORD_FILE`, NATS a generated auth config, LiteLLM and the worker export their values from the files in an entrypoint wrapper. `generate-secrets.sh` writes hex values, a TLS pair and the derived `database-url`/`nats-url`/`nats-auth.conf` (kept once written), refuses to regenerate secrets whose rotation loses data, reads `.env` like compose; `validate-env.sh` checks the mounted files. New `CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET` (+ `_FILE`) decouples the LLM key encryption from the JWT secret. URL userinfo is redacted in all logs.
- [x] (2026-09-30) **KI-47 Blue-green Traefik routes the frontend to the wrong port** (high): Traefik sends `frontend-blue` / `frontend-green` traffic to port 80 while nginx listens on 8080, so blue-green deployments cannot serve the frontend (audit INFRA-001 still open). Evidence: `docker-compose.blue-green.yml:59`, `docker-compose.blue-green.yml:72`, `frontend/nginx.conf:5`. **Partly fixed (S4):** the Traefik label uses the frontend's port 8080. The overlay still does not work, see KI-70. **Fixed (S6, with KI-70):** the overlay serves the frontend and the core through Traefik.
- [x] (2026-09-30) **KI-48 Image scan pulls a tag that is never pushed** (medium): `docker-build.yml` tags images with the short SHA without prefix, but the scan job pulls `sha-<full sha>`, so the Grype scan fails on every push and image scanning never runs. Evidence: `.github/workflows/docker-build.yml:44`, `.github/workflows/docker-build.yml:207`. **Fixed (S4):** each build job outputs `<lowercased image>@<digest>` and the scan matrix scans exactly that reference (`fail-fast: false`).
- [x] (2026-09-30) **KI-49 Restore script never terminates active connections** (medium): `psql -c` does not substitute `:'dbname'` and the error is discarded, so `dropdb` fails while core, worker or LiteLLM are connected and the restore aborts. Evidence: `scripts/restore-postgres.sh:42-44`. **Fixed (S4):** `restore-postgres.sh` uses `dropdb --force` (PostgreSQL 13+) and aborts visibly on errors.
- [x] (2026-09-30) **KI-50 Devcontainer sets `LITELLM_URL`** (medium): The devcontainer sets `LITELLM_URL`, but Go Core and worker read `LITELLM_BASE_URL` and fall back to `localhost:4000`, which is unreachable from the devcontainer; the 2026-03-19 Bug 3 fix below covered the worker code, not the devcontainer. Evidence: `.devcontainer/devcontainer.json:20`, `internal/config/loader.go:180`, `workers/codeforge/config.py:187`. **Fixed (S4):** the devcontainer sets `LITELLM_BASE_URL`.
- [x] (2026-09-30) **KI-51 Configuration drift** (low): `OLLAMA_BASE_URL` does not change LiteLLM's Ollama routing (`litellm/config.yaml` hardcodes `api_base`), `DOCS_MCP_*` in `.env.example` are ignored by `docker-compose.yml`, `scripts/logs.sh` suggests a non-existent `docs-mcp-server` service, and `codeforge.example.yaml` contradicts the code (bcrypt minimum is 12, not 4; routing is enabled by default). The SMTP port defaults to 0 although documented as 587. Evidence: `litellm/config.yaml:18`, `docker-compose.yml:146-149`, `scripts/logs.sh:27`, `codeforge.example.yaml:105`, `internal/config/config.go:211`. Also: `scripts/resolve-docker-ips.sh` runs `set -euo pipefail` although it is meant to be sourced into an interactive shell; the worker `HistoryConfig.max_context_tokens` default (120000) differs from Go `agent.max_context_tokens` (128000). **Fixed (S4):** LiteLLM reads Ollama's `api_base` from `OLLAMA_API_BASE` (set from `OLLAMA_BASE_URL` in both compose files); docs-mcp uses the `DOCS_MCP_*` variables with the former values as defaults (plus the `host.docker.internal` mapping on Linux); `scripts/logs.sh` lists the real compose services; `codeforge.example.yaml` matches the code (bcrypt 12-31, routing on, `meta_router_model` "", no `dashboard_port`); SMTP port defaults to 587 and is validated when `smtp_host` is set; `resolve-docker-ips.sh` is safe to source; the worker history token default is 128000 like Go. Left over: `benchmark.dashboard_port` is still an unused Go setting; the example's `orchestrator.decompose_model` differs from the code default "".
- [x] (2026-09-30) **KI-59 Config files are not ASCII-only** (low): AGENTS.md (then CLAUDE.md) requires ASCII in config files, but `configs/benchmarks/{agent-coding,basic-coding,tool-use-basic}.yaml`, `configs/model_pricing.yaml`, `internal/service/prompts/system/tool_permissions.yaml` and `scripts/{logs,resolve-docker-ips,test}.sh` contain non-ASCII characters (`codeforge.example.yaml` fixed 2026-09-29). Evidence: `LC_ALL=C grep -P '[^\x00-\x7F]'` on those files. **Fixed (S4):** em dashes replaced in the eight files; prompt golden files regenerated; no non-ASCII left in yaml/yml/sh/toml/conf/.env files.
- [x] (2026-10-01) **KI-70 Blue-green overlay does not work** (medium): `docker-compose.blue-green.yml` uses `extends:` without `file:` (the colored services have no image); extending the prod services would inherit their published ports (frontend :80 collides with Traefik, blue and green both bind :8080) and removing them needs `ports: !reset []`, which the check-yaml pre-commit hook rejects; Traefik is not on the services' networks; nginx proxies to `core`, which does not exist in blue-green; `${ACME_EMAIL}` is not expanded in `traefik.yaml`. Found during the KI-47 fix (2026-09-30). **Fixed (S6):** `docker-compose.blue-green.yml` works. The colors (`core-blue` / `frontend-blue`, `core-green` / `frontend-green`) are compose profiles that extend the prod services (with `file:`) and drop their published ports (`ports: !reset []`, so pre-commit runs `check-yaml --unsafe` on this one file); Traefik (v3.6 or later, configured by command flags, `traefik/traefik.yaml` is gone; `ACME_EMAIL` and `CODEFORGE_DOMAIN` are required) is on the fixed network `codeforge-public`, routes `/api`, `/health`, `/ws`, `/.well-known` and `/a2a` to the core and everything else to the frontend of the running color; each frontend's nginx proxies to its own core (`CORE_UPSTREAM`, default `core:8080`); both cores carry the network alias `core` for the worker. `./scripts/deploy-blue-green.sh [blue|green]` starts the inactive color with `--no-deps`, waits until its core and frontend are healthy and then stops the other color (`DRY_RUN=1` previews). The shared services (postgres, nats, litellm) must already run and be healthy; the script refuses otherwise and never recreates them. Usage: [dev-setup](dev-setup.md), [disaster-recovery](disaster-recovery.md).
- [x] (2026-10-01) **KI-71 Agent tools can read the worker's secrets** (medium): since S4 agent tool subprocesses get a scrubbed environment (`codeforge.subprocess_env.tool_env`), but they still run as the worker's UID and can read `/run/secrets/*` (0644 inside the container) and `/proc/1/environ`, which hold `CODEFORGE_INTERNAL_KEY` (admin on the core API), the database, NATS and LiteLLM credentials. Needs a separate UID for tool processes or the sandbox (KI-13). Found in the S4 review (2026-09-30). Also: agent tool processes can reach NATS (no NATS authentication inside the deployment), so a prompt-injected agent with Bash could publish completions, heartbeats, cancels or tool-call responses (found in the S2 follow-up security review, 2026-09-30). **Fixed (S6, [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md), 2026-10-01; four review rounds and a security-review round):** (1) *Tool user.* The worker container starts as root with only `SETUID`, `SETGID` and `KILL`; `scripts/worker-entrypoint.sh` runs the worker as uid 10001 (group `codeforge-ws` 10010) with those as ambient capabilities. Every process an agent causes (Bash, grep, `git`, quality gates and workspace tests, benchmark test commands, the backend CLIs and their version checks, the Claude Code CLI and its hook, MCP stdio servers) starts only through `workers/codeforge/tool_process.py` as uid/gid 10002 with the workspace group, no capabilities, `no_new_privs` and umask 002: `setpriv` with a fixed environment, then `env -i` as the tool user (an `LD_PRELOAD` of a declared environment never reaches `setpriv`). `CODEFORGE_TOOL_ISOLATION=required` (image and prod) probes this at startup and fails every tool call closed (`ToolIsolationError`); `off` elsewhere. (2) *Secrets.* The worker reads `DATABASE_URL`, `NATS_URL`, `LITELLM_MASTER_KEY` and `CODEFORGE_INTERNAL_KEY` from `*_FILE` paths; compose ignores secret `uid`/`gid`/`mode`, so `/run/secrets` is a tmpfs (uid 10001, mode 0700) with the 0644 files inside, locked to mode 0 once the worker read them. (3) *Workspaces.* Core, worker and tools share them through group 10010 (setgid 2775 root, Go Core umask 002, files 0664, directories 0770); `share_tool_files` (a tool-user `find` pass after each tool process and at the end of every run, conversation run, task and benchmark task) keeps what tools create shared; a versioned one-time walk opens existing workspaces. (4) *NATS.* Every client authenticates: user `core` (publishes every stream subject, owns the stream) and `worker` (publishes only its results, progress and `.dlq` copies; no stream changes, no KV); permissions in `configs/nats/nats-server.conf`, passwords in the secrets `nats-core-pass` and `nats-worker-pass`, image pinned to `nats:2.15-alpine`. *Review rounds:* MCP stdio servers run as the tool user and the Go Core never starts one (the stdio test answers 400); the `setpriv` environment reset; per-service inbox prefixes (`_INBOX_core`, `_INBOX_worker`) and consumer rights bound to the worker's own consumer names; the worker's `NotificationHub` (four shared push consumers, gap and reconnect read-back with batched direct get, never deletes a consumer, `/health/ready` reports `starting` and `not ready`); `handoff.approved` needs an approved, unconsumed quarantine release (migration 111); MCP env and header values are redacted to `***` and kept only for the same transport, URL, command and arguments; MCP links are tenant-scoped (migration 112) and runs resolve their project's servers in their own tenant; a tenant's admins manage, test and assign its MCP servers; assignment audit entries name the server and are written before the change (503 when they cannot be); the stamp file and the workspace walk never follow a symlink. *Operators:* run `scripts/generate-secrets.sh` then `scripts/validate-env.sh` after upgrading, deploy `configs/nats/`; the worker image starts as root (platforms that forbid root containers get failing tool calls); the first start walks existing workspaces once. *Residual risks* (open follow-ups KI-95 to KI-103): all tenants share the tool uid 10002; in-process worker readers still follow symlinks; workspaces are group-writable (the KI-77 `.git/config` window stays); NATS does not check deliveries to a known plain subscription; stdio MCP servers run with the rights of the agents' Bash in that tenant. The NATS part was found in the S2 follow-up security review (2026-09-30).
- [x] (2026-09-30) **KI-72 Claude Code runs bypass the policy layer** (high): `claudecode/*` runs start the Claude Code CLI (`workers/codeforge/claude_code_executor.py`) without permission flags or a policy callback, so no per-tool policy check, mode tool list or HITL approval applies to them. `_make_policy_callback` (maps Claude Code tool calls to `runs.toolcall.request`) was never wired into the SDK options and is unused since the SDK path was removed (S4: the SDK always passes the worker's full environment to the CLI). Wire a policy hook the CLI supports (e.g. a permission prompt tool / hooks calling the Go policy) or restrict claudecode runs to a read-only mode until then. Found in the S4 review (2026-09-30). **Fixed (S6):** every Claude Code tool call goes through a PreToolUse hook (matcher `*`, any error blocks) and a per-run unix socket (private dir, random token) to `runs.toolcall.request`, so mode tool lists, path/command rules and HITL apply; the CLI loads no user/project/local settings (`--setting-sources ""`), no MCP servers (`--strict-mcp-config`), runs under `--permission-mode dontAsk` and is offered only tools with a canonical policy name (`--tools`, `Monitor` = `Bash`; other names are denied; WebFetch/WebSearch not offered); paths go through the loop's mapping relative to the real workspace; prompt on stdin, system prompt via a 0600 file; the CLI runs in its own process group, is stopped on timeout (run time without approval waits) and on cancel, streams output and counts usage of interrupted turns, and falls back to another model only if the turn changed nothing; a capability check (flags in `--help`, probe for hidden options, only successes cached) fails unsupported CLIs closed and hides them from routing; `CODEFORGE_CLAUDECODE_PATH` is honoured. Also found: before the fix every claudecode run failed on current CLIs (`stream-json` requires `--verbose`) and fell back to LiteLLM. Follow-up: a canonical network tool name would be needed to ever offer WebFetch/WebSearch. **Review round (S6-G):** the CLI runs with `CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR=1` so every Bash call starts in the workspace (CLIs without that variable are not covered).
- [x] (2026-10-01) **KI-73 Channel follow-ups** (medium): the webhook entry point always answers 403 (`GetChannel` selects no webhook key and `channels` has no `webhook_key` column; `GenerateWebhookKey` is unused); `ThreadPanel` is not mounted anywhere (ChannelView passes no `onThreadClick`), so threads are shown only as flat messages; `channel.typing` and `channel.read` have no producer or stored read state. Found in the S5 review (2026-09-30). **Fixed (S6):** `POST /channels/{id}/webhook-key` (admins) makes a key, shown once and stored as a SHA-256 hash (migration 108); the webhook is the public route `POST /api/v1/webhooks/channels/{id}` with the `X-Webhook-Key` header (constant-time comparison; 401 without the header, one uniform 403 for a wrong key, an unknown channel or a channel ID that is not a UUID). Read state per user: `POST` / `GET /channels/{id}/read`, the `channel.read` event, `has_webhook_key` and `unread_count` on the channels. `channel.typing` was removed (no producer) and `ThreadPanel` is mounted in `ChannelView`. Follow-up: [KI-89](#known-issues).
- [x] (2026-10-01) **KI-74 Frontend live-update follow-ups** (low): the runtime changes task and agent status on run start/finish without broadcasting `task.status`/`agent.status`, so three consumers refetch on `run.status` instead (duplicate `GET /agents` from AgentPanel and useProjectDetail); PlanPanel refetches undebounced on every `plan.step.status`; RunPanel drops live events of a new run until `api.runs.start` resolves; output in the first ~0.5 s of a run before its lane mounts is not shown; `CreateProjectModal` writes the unused `autonomy_level` config key. Found in the S5 work and review (2026-09-30). **Fixed (S6):** the runtime broadcasts `task.status` and `agent.status` (tenant-scoped) when a run starts and ends, so `AgentPanel` uses the page's agents instead of refetching; `PlanPanel` batches refetches (300 ms); `RunPanel` keeps events that arrive before `api.runs.start` resolves; the War Room collects the output of its lanes itself; `CreateProjectModal` no longer sends `autonomy_level`.
- [x] (2026-09-30) **KI-75 LLM models are global across tenants** (medium): all tenants share one LiteLLM proxy, and an admin (or, for adding, an editor) of any tenant can add or delete models for every tenant (`POST/DELETE /api/v1/llm/models`, audited since S5). Model management should be restricted to a platform-admin role or tenant-scoped. Found in the S5 review (2026-09-30). **Fixed (S6):** `POST/DELETE /api/v1/llm/models` require a platform admin (`middleware.RequirePlatformAdmin`: admin role in the default tenant); everyone keeps read access. The model UI still shows the actions to other users (they get 403), follow-up in progress.
- [x] (2026-09-30) **KI-76 Runtime follow-ups from S2** (medium): plan steps drop `ModeID` (`CreatePlan` and the plan-step store), so debates run without the proponent/moderator modes; the review router's LLM call runs under the global plan scheduling lock; the auto-agent waits for a run's completion only after dispatching (race), and after a wait timeout its next message gets 409 until the run ends; a conversation whose run completion never arrives (e.g. its start went to the DLQ) stays blocked until Stop or a Go restart; if the write that ends a stopped run fails, the worker's completion is not used and the run stays running. Found in the S2 runtime reviews (2026-09-30). **Fixed (S2):** plan steps keep `ModeID`; the review router's LLM call runs outside the plan lock (2 min timeout; the step stays pending until decided); the auto-agent registers its completion waiter before dispatching and stops a run it gives up on; a completion arriving during a stop is kept and used if the stop cannot record the end; Go subscribes to `runs.start.dlq` and `conversation.run.start.dlq` and ends dead-lettered starts. Review round in progress (ping-pong round accounting with the review router, dropped review decisions, stop/completion atomicity).
- [x] (2026-10-01) **KI-78 Artifact validation writes before the run's end is decided** (low): artifact-validation events and audit entries are written before `CompleteRun`, so a run that loses the race to another completion path (stop, watchdog) can show contradictory entries (the gate path was fixed in the S3 review). Found in the S3 review (2026-09-30). **Fixed (S6):** the artifact validation (the result on the run, the `artifact.validation` broadcast, the run event and the `artifact.failed` audit entry) is recorded only by the path whose write ended the run (or moved it into its quality gate); a path that loses the run's end records nothing.

#### Compliance

- [x] (2026-09-30) **KI-52 GDPR retention never runs** (medium): `RetentionService` is never instantiated, so expired sessions, conversations, runs and audit entries are never purged, and `AnonymizeExpiredIPAddresses` uses `UPDATE ... LIMIT`, which PostgreSQL rejects (residual of WT-3 below). Evidence: `internal/service/retention.go:25`, `internal/adapter/postgres/store_audit_log.go:94-99`. **Fixed (S6):** the retention job runs at startup and every `retention.interval` (default 24h, 0 disables it) across all tenants in batches (`WHERE id IN (SELECT ... LIMIT $n)`); sessions, conversations and runs are aged by last activity, audit entries by creation (default 7 years, as the policy says), audit IP addresses are removed after 180 days (`retention.audit_ip_addresses`); deleting expired conversations also removes their task-less agent sessions (the FK check made it fail before); one failing category does not stop the others; periods under 24h are rejected. Agent events and benchmark results are not purged yet (see [data-retention.md](data-retention.md)).
- [x] (2026-09-30) **KI-53 Audit log listing breaks after GDPR erasure** (high): Migration 089 makes `admin_email` nullable and GDPR erasure sets it to NULL, but both listing queries scan it into a `string`, so the tenant's audit log endpoint returns 500 once any user with audit entries has been erased (residual of WT-3 below). Evidence: `internal/adapter/postgres/store_audit_log.go:42`, `internal/adapter/postgres/store_audit_log.go:64`, `internal/adapter/postgres/store_audit_log.go:112`. **Fixed (S6):** `AuditEntry.AdminEmail` is nullable (`*string`); both listings share one scanner; an erased user's entries are listed with `admin_email: null`.
- [x] (2026-09-30) **KI-54 deepeval telemetry is not disabled** (low): deepeval is a runtime dependency and `DEEPEVAL_TELEMETRY_OPT_OUT` is set nowhere, so evaluation runs may send usage telemetry to a third party that the privacy policy does not disclose (audit COMP-014 still open). Evidence: `pyproject.toml:28`, `workers/codeforge/evaluation/metrics.py:11-12`. **Fixed (S6):** the worker forces `DEEPEVAL_TELEMETRY_OPT_OUT=YES`, `CONFIDENT_METRIC_LOGGING_ENABLED=NO`, `CONFIDENT_TRACING_ENABLED=NO`, `DEEPEVAL_UPDATE_WARNING_OPT_IN=0`, `DEEPEVAL_DISABLE_DOTENV=1`, `DEEPEVAL_DISABLE_LEGACY_KEYFILE=1` before any deepeval import (`codeforge/evaluation/_deepeval_env.py`, also `ENV` in `Dockerfile.worker`), so neither telemetry nor Confident AI uploads (which included evaluated inputs and outputs) can be switched on.

#### Unwired features

- [x] (2026-10-01) **KI-17 Contract-first review/refactor (Phase 31) is not wired** (high): `ReviewTriggerService` gets a nil orchestrator, so `POST /projects/{id}/review-refactor` and `/boundaries/analyze` return `{"triggered": true}` but start nothing, and `DiffImpactScorer` has no caller. `RefactorApproval` listens for `refactor.approval_required` while the backend sends `review.approval_required` with another payload, and its approve/reject `fetch()` calls send no Authorization header (also the WT-7 remainder below). Evidence: `cmd/codeforge/main.go:489`, `internal/service/review_trigger.go:25-27`, `frontend/src/features/project/RefactorApproval.tsx:34`, `frontend/src/features/project/RefactorApproval.tsx:45`. **Fixed (S6):** the pipeline runs end to end. `POST /projects/{id}/review-refactor` and `/boundaries/analyze` start a plan and answer 202 `{triggered: true, plan_id}` (409 when the project already has an active review pipeline or the agent belongs to another plan, 400 for a non-git workspace). Artifact validators know `BOUNDARIES.json`, `CONTRACT_REVIEW.md`, `PROPOSAL.md` and `SYNTHESIS.md`. The baseline commit is recorded when the refactorer step starts (`StepPreparer`, outside the scheduling lock) in the Go record `review_pipelines` (migration 100; state `pending`, `refactoring`, `awaiting_decision`, `done`; baseline, result, step, run, impact); the refs `refs/codeforge/review/<plan>` and `refs/codeforge/review-result/<plan>` only keep the commits from garbage collection, the record is what is trusted. `DiffImpactScorer` measures the change and the gate fails closed (tampered ref, missing record, boundary lookup error, unmeasurable change); `StepGate` returns status and apply, and the apply runs under the lock after the step status is stored. Keep (`POST /runs/{id}/approve`) or undo (`/reject`; a path-scoped three-way undo, HEAD moves back only by compare-and-swap) answer `{status, head_restored, message?, restored_paths?}`, also after a failed or cancelled refactoring, and are matched on the review record (they survive run retention); an undo inside the status/apply window answers 409 "try again". `GET /projects/{id}/review/pending` lists the waiting decisions (the dialog loads them on open and on WebSocket reconnect); `RefactorApproval` uses the API client and the `review.approval_required` WebSocket event. A stopped refactoring is measured after its worker confirmed the stop or, after `LostWorkerAfter`, by the watchdog check "undecided review refactorings". The unused `review.*` NATS subject and the `review.>` stream wildcard are removed. Open: [KI-94](#known-issues).
- [ ] **KI-25 `spawn_subagent` starts nothing** (medium): The tool only publishes an `agent.subagent_requested` trajectory event and tells the LLM "Sub-agent spawned"; Go only logs and broadcasts it, so the orchestrating model waits for results that never arrive. Evidence: `workers/codeforge/tools/spawn_subagent.py:110-129`, `internal/service/runtime_subscribers.go:259-282`. **Partly fixed (S6):** the worker no longer registers `spawn_subagent`, so no model is offered a tool that only reports a spawn (D-S3). Still open: Go has to start the sub-agent run and return its result before the tool is offered again.
  - Owner decision (2026-10-02): sub-agents work like Claude Code's Agent tool (fresh context, agent type selects prompt/tools/model, final report as the tool result, parallel calls, depth limit, Go-owned state). Plan: [ki25-subagents-plan.md](plans/ki25-subagents-plan.md) (judged design, security and correctness reviewed); owner decisions open in its section 19. Its migration is renumbered to 122 (113 is S7-C, 120-121 are S7-H).
  - Owner decisions on the plan's open questions (2026-10-03): depth 2, cost caps as shares of the profile budget, transcripts kept per tenant setting, no native Agent tool for Claude Code runs, no MCP tools, worktrees or background sub-agents in v1; approval waits do not count against a sub-agent's deadline and its default wall clock is 3600 s (for weaker and local models); details in the plan's section 19.
- [x] (2026-10-01) **KI-55 GitHub OAuth web flow is never wired** (medium): `NewGitHubOAuthService` has no caller and `Handlers.GitHubOAuth` is never set, so `/api/v1/auth/github` always returns 501. Evidence: `internal/service/github_oauth.go:38`, `internal/adapter/http/handlers_github_oauth.go:10-13`. **Fixed (S3-F):** the web flow is wired. It is enabled only with `github.client_id`, `github.client_secret` (or `GITHUB_CLIENT_SECRET_FILE`) and `github.callback_url` (`GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `GITHUB_CALLBACK_URL`; `client_id` alone keeps the device flow). The callback URL is the only redirect URI sent to GitHub: https (http on loopback), path `/api/v1/auth/github/callback`, no user info, query or fragment. `POST /api/v1/auth/github` (authenticated) stores a single-use state for the caller's tenant, sets an HttpOnly SameSite=Lax state cookie and returns `{url}`; `GET /api/v1/auth/github/callback` (no session) checks cookie and state, consumes the state once, creates the VCS account in the starting user's tenant and redirects to `/settings?github_oauth=connected` or `/settings?github_oauth=failed&reason=<code>`; both answer 501 while the flow is not configured. The retention job purges expired states of abandoned flows on every sweep and the callback refuses expired states.
- [x] (2026-10-01) **KI-56 Webhook-triggered roadmap sync always fails** (medium): GitHub webhooks ask for provider `github` (registered as `github-issues`), Plane gets no `api_token` and GitLab an empty base URL; the webhook still returns 200 and the failure is only logged. Evidence: `internal/service/pm_webhook.go:31-47`, `internal/service/pm_webhook.go:88`, `internal/adapter/githubpm/provider.go:15`. **Fixed (S3-F):** webhooks ask for the registered provider names (`github` -> `github-issues`, `gitlab`, `plane`) and find their project by an exact match (`FindProjectByRepo`: host and full path, case-insensitive, in the webhook's tenant; Plane by the project config keys `plane_workspace` and `plane_project_id`). The webhook answers 202 when the sync started (its outcome arrives as a `pm.sync` event with `status` `completed` or `failed` and an `error`), 404 when no project matches, 400 when the provider cannot sync (not registered or configured, project on another host) and 200 for an event type it ignores. The operator's Plane token goes only to `plane.base_url` (default `https://api.plane.so`, `CODEFORGE_PLANE_BASE_URL`): a project whose `plane_base_url` names another host is not synced. GitLab syncs without a token and the VCS webhooks keep the substring lookup: both fixed in [KI-85](#known-issues) (per-project webhooks with an exact repository match, an own API token per PM integration).
- [x] (2026-10-01) **KI-57 Email HITL provider sends no approval emails** (medium): The email feedback provider is built with nil recipients and a hardcoded localhost callback, and its Approve/Deny GET links point at a POST-only authenticated route, so approval requests by email are silently never delivered. Evidence: `cmd/codeforge/main.go:761-763`, `internal/adapter/email/feedback.go:34-60`. **Fixed (S3-F):** approval emails need `notification.smtp_host`, `smtp_from`, `approval_recipients` (`CODEFORGE_NOTIFICATION_APPROVAL_RECIPIENTS`, bare addresses) and `notification.web_ui_url` (`CODEFORGE_NOTIFICATION_WEB_UI_URL`, absolute http(s) base URL); the provider is registered only when all four are set (the startup log names what is missing). The mail (`html/template`, so agent-controlled text is escaped) shows tool, command, path, profile and arguments preview and links to `<web_ui_url>/approvals/<run>/<call>`. That page (login returns to its `?next=` path) reads `GET /api/v1/runs/{id}/approvals/{callId}` and decides with `POST /api/v1/feedback/{run_id}/{call_id}?decision=allow|deny`, both for admins and editors and scoped to the caller's tenant; the audit entry records tool and deciding user. Only requests of the default tenant are mailed. `runtime.approval_timeout_seconds` (default 60) must be raised for approvals by email.
- [x] (2026-09-30) **KI-58 Agent `create_skill` always fails** (medium): The tool inserts `tenant_id ''` into the UUID NOT NULL `skills.tenant_id` column, which PostgreSQL rejects, so agent-generated skill drafts (Auto-Agent Skills Task 10 below) never persist. Evidence: `workers/codeforge/consumer/_conversation_skill_integration.py:46`, `internal/adapter/postgres/migrations/045_create_skills.sql:4`. **Fixed (S6):** `wire_skill_tools` / `make_skill_save_fn` take the conversation's `tenant_id` (required); drafts are stored with tenant, project, source `agent`, status `draft`; without a tenant the tool reports that the skill was not saved instead of claiming success.
- [x] (2026-10-01) **KI-60 Tiered cache is built and discarded** (low): `main.go` builds the L1/L2 cache and throws it away (`_ = tiered.New(...)`), so no service uses it although docs describe it as active. Evidence: `cmd/codeforge/main.go:185`. **Fixed (S3-F):** the tiered cache is removed: `internal/port/cache`, the `ristretto`, `natskv` and `tiered` adapters, the `cache.*` configuration (`CODEFORGE_CACHE_*`; configuration files that still have the keys load, the keys are ignored) and the `dgraph-io/ristretto/v2` dependency. HTTP idempotency keys keep their own NATS KV bucket (`idempotency.bucket`).
- [x] (2026-09-30) **KI-61 SIGHUP config reload has no effect** (medium): the `ConfigHolder` is created after every service has copied its config and nothing reads it; SIGHUP only really reloads the secrets vault, config changes still need a restart. Evidence: `cmd/codeforge/main.go:1056-1065`. **Fixed (S4, SIGHUP is secrets-only):** the unused `ConfigHolder` is removed; SIGHUP reloads the secrets vault (read per request by the LiteLLM client), re-loads the configuration like startup (`config.ChangedSinceStart`: YAML, env, `*_FILE`, CLI flags) and logs the names of changed settings that need a restart, never values; an invalid config file is logged and the running config stays. Making services hot-reloadable would mean re-reading config in every service (ADR-013) and is not planned.
- [x] (2026-10-01) **KI-79 GDPR residuals** (low): `quarantine_messages.reviewed_by` is free text typed by the reviewer and cannot be matched to an erased user (the handler should record the logged-in user's ID); the in-app privacy page says account data is kept "lifetime + 30 days after deletion", while erasure is immediate and backups expire after about five weeks - the wording needs a decision. Found in the S6 GDPR work (2026-09-30). **Fixed (S6):** the quarantine reviewer is the logged-in user (`reviewed_by_user_id`, migration 109, `reviewed_by_id` in the API; a name in the request body is ignored and the review page no longer asks for one; a caller without a users row, such as auth disabled or the internal service key, is recorded by name only); erasure (GDPR endpoints and account deletion) replaces the reviewer name with "Deleted user" before the user row goes. The privacy page (EN/DE) says account data is erased immediately on request or account deletion and that backups containing it are rotated out after about five weeks; [data-retention](data-retention.md) and the [privacy policy](privacy-policy.md) use the same wording. Found: the page points to a Settings > Privacy screen that does not exist, [KI-93](#known-issues).
- [x] (2026-10-01) **KI-62 Stall re-planning is not wired** (medium): `OrchestratorService.ReplanStep` has no production caller, so the documented MagenticOne stall detection plus re-planning loop never re-plans. Evidence: `internal/service/orchestrator_consensus.go:413`. **Fixed (S6):** a stalled run is recognised by the shared marker `stall detected:` in its error (worker and Go agree on it, contract fixture `internal/domain/run/testdata/stall_contract.json`); its plan step gets a new run up to `runtime.stall_max_retries` times (default 2, 0 = no re-planning) and a ping_pong step re-runs the same round; when the retries are used up the step fails. Open: [KI-94](#known-issues).

#### Follow-ups found in the S3-F to S6-H reviews (2026-10-01)

- [ ] **KI-88 Submodules and nested repositories are refused in workspaces** (medium): a workspace whose index or HEAD has a gitlink (any mode with the type bits `0160000`), that has a nested `.git` (directory or file) outside ignored directories, or whose index has a path with a `.git` component is refused for every Go git operation (checkpoints, rewind, delivery, review baseline, the git providers), because `git add` and `git status` would run git inside the nested repository with its own configuration and attributes. The walk stops at 200,000 entries or depth 64 and refuses the workspace then (fail closed), so very large trees (more than 200,000 entries outside ignored directories) are refused too; the check costs about 30 ms per git operation on a repository of 2,400 files. Linked worktrees are refused as well. Allowing nested repositories would need the same hardening for each nested repository (`internal/git/nested.go`). Found in the security review of the S3-F fix round (2026-10-01).
  - Owner decision (2026-10-02): option 2, submodules and nested repositories are supported read-only and opaque (Go git never enters them; agent writes inside them are refused by policy). Plan: [ki88-opaque-submodules-plan.md](plans/ki88-opaque-submodules-plan.md) (security reviewed); open questions in its last section. Its migration is renumbered to 123 (113 is S7-C, 120-121 are S7-H).
  - Owner decisions on the plan's open questions (2026-10-03): pull requests through the provider REST API (gh removed from the Go Core), delivery refused when a run created a nested repository, no per-tenant git pool share; submodule initialisation in the worker is a follow-up; details in the plan's open questions.
- [x] (2026-10-04) **KI-89 Channel posts and channel creation need a users row** (low): `channel_messages.sender_id` and `channels.created_by` reference `users(id)` (migration 071), but with auth disabled (the all-zero user) and with the internal service key no users row exists, so posting a message or creating a channel as that identity probably fails its foreign key (not tested; the read state and the quarantine reviewer already handle callers without a row). Store the sender as nullable text for such callers, or create a row for them. Found in the S6-H review (2026-10-01). **Fixed (S7-E):** a channel's creator and a message's sender are linked by user ID only when they have a users row; a caller without one (auth disabled, internal service key) is recorded by `sender_type` and `sender_name` without an ID instead of failing on the foreign key (500). The creator of a channel is the authenticated caller (`created_by` in the body is ignored; 401 without a caller). No migration. **Review fixes (S7-E, 2026-10-04):** only the synthetic identities (auth disabled, internal service key; `user.IsAccountless`) post, create channels or review quarantine messages without a user ID. A user whose row is gone (erased or deleted, with a still-valid access token, KI-143) gets 401 and nothing is stored, so no row escapes the erasure (channels, channel messages, quarantine reviews). A channel's project must belong to the caller's tenant (404 otherwise).
- [x] (2026-10-04) **KI-90 No retention for handoff claims and delivery records** (low): `handoff_claims` has no foreign key and no retention rule, so a row stays for every handoff stage ever carried out; `task_result_costs` and `conversation_turn_completions` are removed only with their task or conversation. Add them to the retention job (`docs/data-retention.md`). Also `webhook_deliveries` (KI-85): the delivery claims of a quiet webhook are pruned only on its next delivery (they are removed with the webhook). Found in the S2-G work (2026-10-01). **Fixed (S7-E):** the retention job deletes handoff claims whose stage was done longer ago than `retention.handoff_claims` (default 720h, past the NATS stream's 30-day max age) and webhook delivery claims older than `webhook.delivery_retention` (default 168h), across tenants in batches (migration 123 adds the indexes). Claims never done stay (a redelivery may take them over; KI-141); `conversation_turn_completions` go with their conversation, `task_result_costs` with their task. **Review fixes (S7-E):** `retention.handoff_claims` must be 0 or at least 720h, the queue's message max age (`messagequeue.StreamMaxAge`, shared with the stream config). Migration 123 builds its indexes concurrently (`NO TRANSACTION`, like 096).
- [x] (2026-10-04) **KI-91 Quarantine messages never expire** (low): `quarantine_messages` has `expires_at` and a status `expired` (counted in `GET /quarantine/stats`), but nothing sets that status, so a held inbound A2A task waits until an admin decides or its caller cancels. When expiry is implemented it must reject the held A2A task (as Reject does) and withdraw the message in one step. Found in the S2-G work (2026-10-01). **Fixed (S7-E):** the stuck-work watchdog expires pending quarantine messages past `expires_at` in their tenant (100 per sweep, every `runtime.stale_check_interval`); a held inbound A2A task is rejected with its message in one transaction, as Reject does. Both changes are conditional, so the sweep is idempotent and safe with several replicas and concurrent decisions. An overdue message can no longer be approved (409), even before the sweep has run. The Quarantine page does not update live on `quarantine.resolved` (KI-142). **Review fixes (S7-E):** `quarantine.expiry_hours` must be 1..2562047 (startup error otherwise). The expiry also rejects an A2A task whose executor has not recorded the held message yet (the late record then conflicts on the task version). Approve/Reject of an already-resolved message answers 409 (was 500).
- [x] (2026-10-04) **KI-92 War Room handoff arrows of `initiated` handoffs stay** (low): `MessageFlow` removes the arrow of a settled handoff after 10 s, but the arrow of an `initiated` handoff (the target's run started) is never removed, so arrows pile up in a long session. Found in the S2-G work (2026-10-01). **Fixed (S7-G):** an initiated handoff's War Room arrow follows the target's run (`run_id`) and goes 10 s after a `run.status` ends it (completed, failed, cancelled, timeout); an initiated handoff without a run, and every followed handoff after a WebSocket reconnect, go after 10 s. **Review fix (S7-G):** a handoff whose run already ended (one of the last 200 ended runs the War Room remembers) settles at once.
- [x] (2026-10-04) **KI-93 Privacy page links to screens that do not exist** (low): the in-app privacy page (`privacy.rights.access`, `privacy.rights.erasure`) sends users to Settings > Privacy > Export and > Delete, but the settings page has no such screens; the endpoints exist (`GET /me/export`, `DELETE /me/data`). Build the screens or point the text at the API. Found in the KI-79 work (2026-10-01). **Fixed (S7-G):** Settings > Privacy (`/settings?section=privacy`) exports `GET /me/export` as a JSON file (the export is not kept in the API client's cache) and deletes with `DELETE /me/data` after the user types their email address. The dialog lists what is deleted (account, API/LLM keys, sessions and reset links, channel memberships), what is kept without personal data (audit entries, consent records, channel messages, quarantine reviews), that projects, conversations and runs stay with the organization, and the backup period; the user is then signed out. Consent per purpose is shown and changeable (a granted required purpose cannot be withdrawn). The privacy page's rights of access, erasure and objection link there. Follow-ups: KI-144, KI-145. **Review fixes (S7-G):** the account deletion is sent once (`requestOnce`: no retry, no offline queue), every failure is shown, and the dialog can be closed while the request runs; consent toggles save independently (a failure reverts only its own purpose); every `/api/v1` answer carries `Cache-Control: no-store` (`middleware.NoStore`), so `GET /me/export` stays out of browser and proxy caches.
- [x] (2026-10-06) **KI-94 Review pipeline and stall re-plan leftovers** (low): user edits made while the refactorer runs count as the refactoring; only review pipelines check the agent reservation; `CancelPlan` of a parent plan does not cancel a debate sub-plan that is running; `POST /runs/{id}/approve-partial` answers 501; git-quoted (non-ASCII) paths are not matched by the boundary check; a stall is not added to the prompt of the re-planned run; a debate's proponent and moderator share one stall budget. Found in the S6-F work (2026-10-01). **Partly fixed (S7-F, 2026-10-04):** non-ASCII and quoted paths in the boundary check (`git diff -z`); a re-planned run is told why the earlier one stalled; stall re-plans are counted per step (`plan_steps.stall_replans`, migration 122); `CancelPlan` cancels the plan's debates, and a debate's end leaves a cancelled plan's step alone; `StartPlan` answers 409 when one of its agents works on another running plan of the project (one replica; debate sub-plans are not checked). **Open, owner decisions:** user edits during the refactorer run (the workspace records no writer; options: refuse editor/file-API writes while a refactorer step runs, or warn in the approval dialog), and `approve-partial` (answers 501 on purpose, no caller: remove it or specify per-path keep/undo). **Review fixes (S7-F R2):** a stalled run's completion that reaches two replicas re-plans the step once: `ReplanStalledStep` checks the run and reports re-planned / budget used up / step moved, and on "moved" the step is left alone (it used to be marked failed, or its new run re-planned again). Plan names starting with `debate:` are reserved for debate sub-plans (400). **Owner decision (2026-10-04): **edits during the refactorer run: record editor and file-API writes made while a refactorer step runs and show the files in the approval dialog (no lock). `approve-partial`: remove the route and its OpenAPI entry; partial approval becomes a roadmap idea. **Fixed (S9-D):** editor and file-API writes during a refactoring are recorded (`review_user_edits`, migration 127) and listed in the approval dialog; the pipeline enters `refactoring` before its baseline snapshot (stored by CAS, an unstored baseline fails closed); `approve-partial` and its OpenAPI entry are removed. Still open: only review pipelines check the agent reservation, and debate sub-plans are not checked by `StartPlan`; writes through git pull/checkout and spec sync are not recorded.

#### Follow-ups from the KI-71 work (2026-10-01)

- [x] (2026-10-02) **KI-95 In-process workspace readers follow symlinks** (medium): the worker reads workspace files on an agent's behalf in its own process (repo map, retrieval index and GraphRAG file collectors, the file tools racing a symlink swap), and an agent can plant a symlink that leaves the workspace. `/run/secrets` (locked after startup) and the worker's environment (no secret values since the `*_FILE` change) are covered, other files the worker user can read are not. Skip symlinks that leave the workspace, or open files with no-follow descriptors and check the resolved path (as `share_workspace_root` does). Found in the KI-71 work (2026-10-01). **Fixed (S7-B, three review rounds; the Go Core was worse than the entry said:** most Core readers had no check at all and read with the Core's rights, which include its own `/run/secrets` and `/data`; the worker's file tools already refused a static symlink out, their gaps were the swap race, FIFOs and listings). In-process workspace access goes through one helper per language: Go `internal/workspacefs` (os.Root) and Python `codeforge.workspace_fs` (a descriptor walk with `O_NOFOLLOW`). Absolute paths, `..` above the workspace and symlinks that leave it are refused ("path leaves the workspace"); relative symlinks inside are followed (8 at most per path); absolute symlinks are not followed in-process, even when they point inside (os.Root cannot be told not to follow, so this is the only rule both languages share); opens never block and take regular files only; the workspace directory itself may not be a symlink or FIFO. Walks are iterative and bounded (Go holds at most 64 descriptors; Python at most 32 and goes 128 levels deep, counting and logging what it leaves out). The indexers never enter a symlinked directory; `glob_files` and `list_directory` follow relative directory symlinks inside the workspace (no cycles, at most 8 per path, listed as `-> target`). Covered, worker: the file tools (`read_file` streams, its output capped at 10 MiB, offset and limit reach any line; `edit_file` keeps the file's line endings and refuses ambiguous matches), `list_directory`, `glob_files`, the repo map, retrieval and GraphRAG indexers (skipped entries logged once per run), framework detection (docs prefetch), the benchmark runner (snapshot, setup, which now refuses `..` and absolute `initial_files` paths, and harness) and the filesystem-state evaluator. Covered, Core: the file API (refusals are 400, delete and rename act on a link itself, deleting or renaming the root is refused; names are path-cleaned, so `x/..` counts as the root; errors name only workspace-relative paths), goal discovery (50 KiB), AI goal discovery (docs over 50 KiB are included, cut on a rune boundary and marked `[truncated]`), context scoring, the stack scan, gate detection, workspace health, the task planner listing, roadmap detect and keyword scan, `SyncToSpecFile`, the openspec, speckit, autospec and markdown spec providers (1 MiB), checkpoints (`Store` now takes the workspace and a relative path), index seeding, the `.gitattributes` check (fails closed), the auto-agent test-file check and `writePatch`. A worker test (`test_workspace_fs_scan.py`) fails on direct file access outside the helper (AST scan with an allowlist that gives a reason per site); the Go side has no such scan, so new workspace access must use workspacefs. Residual, not covered by the helpers: hard links (cannot be told apart from regular files; `fs.protected_hardlinks=1`, no cross-mount), subprocess readers (the Core's git [KI-77](#known-issues), LSP [KI-83](#known-issues), svn; the agents' own tools [KI-71](#known-issues)), group-writable tenant directories ([KI-96](#known-issues)); Go escape detection relies on os.Root's unexported error text (pinned by a test). Knowledge-base content and benchmark datasets: fixed (KI-105, KI-107). Details: [SECURITY.md](SECURITY.md#agent-tool-isolation), [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md).
- [x] (2026-10-03) **KI-96 All tenants share one tool UID** (medium): every tool process of every run and tenant is uid 10002, so one agent's processes can read, signal or trace another's, and reach other workspaces (as before KI-71). Needs per-tenant (or per-run) tool UIDs, or the sandbox (KI-13). Found in the KI-71 work (2026-10-01). **Fixed (S7-H, [ADR-018](architecture/adr/018-per-tenant-tool-identities-and-landlock.md); one review round; [plan](plans/ki96-tenant-tool-isolation-plan.md)):** every tool process (Bash, tool commands, quality gates, workspace tests, backend CLIs, the Claude Code CLI and its hook, MCP stdio servers) runs as its tenant's tool UID and GID (20000-29999, `tenants.tool_uid`, migration 120; allocated lazily with `workspace.tool_acls: required`, immutable, bound on the volume in `<root>/.codeforge/uids/`) with no supplementary group, no capabilities and `no_new_privs`, and under Landlock: write access only to its work item's workspace and its tenant's HOME, the system read-only, everything else denied, including `/tmp`, other processes' `/proc` entries and the tenant's other projects; signals and abstract sockets are scoped with ABI 6. Tenant directories are 2770 with `u:T:--x` and a default ACL (`u:T:rwx`, `g:10010:rwx`); the root is 2771. The worker acts only by descriptor in trees a tool can write, verifies root and tenant directories at every launch, and refuses foreign-owned or symlinked ones. The tool environment is passed on a memfd (the KI-71 launcher exposed it in `/proc/<pid>/cmdline`: 150/150 tokens). Tenant HOMEs are on the `tool_homes` volume with a TMPDIR per work item; caches and config live there. An fd-based sharing pass runs as T. Trees from before the upgrade are migrated per tenant (owner-run steps; hard links into other tenants' trees copied; planted ACL entries removed). Leftover processes are reaped by pidfd when the tenant goes idle in a worker, before its last work item's end-of-work steps; the worker's commands that run as a tool UID end within 600 s (migration walks 3600 s), then their process group is killed. Project workspaces are deleted by the worker as T, then the worker removes its own remaining entries (the Go Core's private patch directory) by descriptor. The image's system interpreter has pytest and ruff for tool processes (`workers/tool-requirements.txt`). `/health/ready` gives 503 with the reason. Verified by `workers/tests/test_tenant_isolation_docker.py` on the built image (CI job `tenant-isolation-docker`). Residual: network and IPC (KI-110), metadata within a tenant, shared HOME within a tenant, no quotas, the worker trusted for all tenants; below Landlock ABI 6 a tenant's leftover process can still stop the end-of-work helpers of the tenant's other running work item in that worker, which then wait up to 600 s each (and that subject's message loop on that worker with them). Also: tools see only their own `/proc` entry and cannot use `/tmp`; files from before the upgrade stay owned by 10002 until replaced; Landlock `off` (allowed outside production only) exposes command lines across tenants; Claude Code's Bash inherits the platform's Claude credentials (KI-111); orphaned tool processes stay zombies (KI-113); no tenant erasure or UID reclaim (KI-112).
- [x] (2026-10-02) **KI-97 MCP args and URL userinfo are not redacted** (low): only env and header values are redacted to `***`; command arguments (`--token=...`) and credentials in a URL (`https://user:pass@host`) are returned by the MCP read endpoints. Redact URL userinfo, and point operators to env variables (redacted) instead of arguments for secrets, or document the limit in the UI. Found in the KI-71 review (2026-10-01). **Fixed (S7-A, three review rounds):** every read (list, get, project list, create and update responses) shows the url as one URL value (`secrets.RedactURLField`): the whole userinfo (a token-only user too) and the non-empty values of credential query and fragment parameters become `***` (`https://***@host/sse?api_key=***`; `'`, `(`, `)` and spaces in a secret are covered), and the host shown is always the host the Core connects to (an `@` in the path, query or fragment never changes it). Credential argument values (`--token=***`, `-name=***`, `name=***`, the value after `--api-key`; names per `secrets.IsCredentialName`) become `***`, the flag stays. Keep rule: a url or argument list that carries `***` is accepted only when it equals the value as read as a whole (`stored.Redacted()`) and is then replaced by the stored value as a whole; any other value with `***` is 400 (nothing is guessed from positions or flags), a `***` with nothing stored is 400, and every kept value (env, headers, url, arguments) requires transport, url, command and arguments to equal the stored ones. Errors and logs never quote URL secrets: every error of the Core's connection test is reduced (a `*url.Error` to its operation and cause) and scrubbed of the url, its secret parts and header values; the worker logs a failed MCP connect with the server ID, host and error class and message, without a traceback, scrubbed the same way. UI hint at the args field ("Use env variables for secrets; arguments are visible to all users of the tenant.", EN and DE). Open ([KI-104](#known-issues)): secrets in other argument shapes (`--header=Authorization: Bearer x`, a url with userinfo passed as an argument) stay visible.
- [x] (2026-10-04) **KI-98 MCP UI gaps** (low): the MCP page has no header editor (an edit sends the headers back as read, `***`, while transport and URL stay the same, and drops them when the URL changes), shows no "stored, unchanged" hint for `***` values, and shows the create, edit, delete, test and assign actions to viewers and editors, who get 403. Add the header editor and hint, and hide the admin actions from non-admins. Found in the KI-71 review (2026-10-01). **Fixed (S7-G):** the MCP form has a header editor (sse/streamable_http) and the same key/value editor for env. A `***` (env, header, URL, args) shows "Stored, unchanged" while transport, URL, command and args are unchanged, and "Enter it again" otherwise; the form then refuses to test or save (the Go Core's `KeepRedacted` rule, applied in the UI as a hint only). Viewers and editors see no create, edit, delete, test or assign action (`hasRole("admin")`); the Go Core stays the authority. **Review fixes (S7-G, security):** a stored secret (`***`) is kept only while transport, URL, command, arguments **and every other env variable and header** are unchanged (no key added, changed or removed); before, changing `GITLAB_API_URL` or adding `HTTPS_PROXY` next to a kept token sent the stored token to another host. The Go Core refuses otherwise (400) and the form says "Enter it again". A `***` counts only under a key read as `***`; the form sends a stdio server's stored headers back and shows them.
- [ ] **KI-99 NATS does not check deliveries to a known plain subscription** (low): the server checks neither a push consumer's deliver subject nor a pull request's reply subject against the creator's publish rights, so a consumer can deliver stream messages to any plain subscription whose name its creator knows (a delivery onto a stream subject is refused as a cycle, `$KV.*` and `$JS.API.*` deliveries are neither stored nor executed). The worker cannot learn the Go Core's core inbox names (per-service inbox prefixes), so this is residual only; no mitigation is planned beyond keeping the prefixes private. Found in the KI-71 review (2026-10-01).
- [x] (2026-10-02) **KI-100 SSRF in the MCP connection test** (medium): `POST /mcp/servers/test` and `POST /mcp/servers/{id}/test` connect from the Go Core to the `sse` or `streamable_http` URL of the definition, so a tenant admin can reach internal hosts of the Go Core's network. Apply the private-range blocking of the SSRF protection (SECURITY.md) to the test connection. Found in the KI-71 review (2026-10-01). **Fixed (S7-A, three review rounds; [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md) decision 9):** `netutil.OutboundPolicy` (`internal/netutil/outbound.go`, built on `IsPrivateIP`) decides which addresses an sse or streamable_http url may reach. Always refused: unspecified (0.0.0.0/8, `::`), link-local (169.254.0.0/16, fe80::/10, so 169.254.169.254), cloud metadata (fd00:ec2::254, 100.100.100.200), multicast, reserved (240.0.0.0/4). Private addresses (10/8, 172.16/12, 192.168/16, CGNAT 100.64/10, 198.18/15, 192.0.0.0/24, ULA fc00::/7, site-local fec0::/10, NAT64 local-use 64:ff9b:1::/48) only for the hosts, IPs and CIDRs in `mcp.allowed_private_hosts` (`CODEFORGE_MCP_ALLOWED_PRIVATE_HOSTS`, default empty; invalid entries stop startup); loopback (127.0.0.0/8, `::1`) only by an explicit loopback entry (`localhost` for that name, `127.0.0.1`, `::1`, a loopback CIDR; `0.0.0.0/0` does not open it). IPv4 inside IPv6 (mapped, `::/96`, NAT64 `64:ff9b::/96`) is judged as IPv4. Create, update (only when the transport or url changed, so a server on a now-refused address can still be disabled, renamed or deleted) and both test routes resolve the host and refuse with 400 before connecting. The test client resolves, checks and dials only a checked address on every connection (DNS rebinding, redirects), uses no proxy unless `mcp.use_proxy` (then only the url's host is checked, the address is not pinned) and follows redirects exactly as the worker's MCP SDK does (same method, no userinfo in the Location, the origin of the request just sent, at most 20; otherwise the redirect response is returned and the test fails). The worker applies the same rules (`workers/codeforge/mcp_outbound.py`): `runs.start` and `conversation.run.start` carry `allowed_private_hosts`, `trusted` and `use_proxy` for each sse and streamable_http server; `GuardedTransport` sends every request to a checked address (Host header and TLS server name stay the host's), tries the next checked address when one cannot be connected, bounds each DNS lookup by the request's connect timeout and caches checked addresses per (host, port) for 30 s; a refused server is skipped with a logged reason. `servers_dir` servers are operator config: the worker trusts them with private and loopback addresses (never link-local or metadata) and the Core never checks them; a stored server with an operator server's ID is never trusted. sse and streamable_http urls must be http or https with a host. Open: [KI-104](#known-issues).
- [x] (2026-10-02) **KI-101 `UpsertMCPServerTools` has no tenant filter** (low): `internal/adapter/postgres/store_mcp.go` deletes and inserts `mcp_server_tools` rows by server ID only; `MCPService.UpsertTools` has no production caller yet, but the first one would replace the tools of any server whose ID it passes, another tenant's included. Check the server's tenant in the statement like `ListMCPServerTools` does. Found in the KI-71 review (2026-10-01). **Fixed (S7-A):** the statement first locks the server row by id and tenant (`FOR UPDATE`; no row, another tenant's server included, is `domain.ErrNotFound`), deletes the old rows through the server's tenant (rows written before carry the default tenant, so they are matched through their server) and inserts the new rows with the server's tenant; the lock also serialises concurrent replacements of one server's tools. No migration.
- [ ] **KI-102 Re-run the NATS permission tests before a NATS image upgrade** (low): the worker's rights rely on how the server checks consumer names in JetStream API subjects, on `AllowDirect` batched direct get (nats-server 2.11 or newer; 2.10 does not answer a batch) and on the unchecked deliver subjects (KI-99). `docker-compose.prod.yml` and CI pin `nats:2.15-alpine`; run `workers/tests/test_nats_permissions.py`, `internal/adapter/nats/auth_test.go` and `workers/tests/test_deployment_isolation.py` against a new version (`NATS_SERVER_BIN`) before changing the pin. Found in the KI-71 review (2026-10-01).
- [ ] **KI-103 Cost and gaps of the tool-file sharing pass** (low): `share_tool_files` walks the whole workspace after each tool process (0.17 s on 102,000 entries when nothing changes, 0.51 s when everything does); a very large workspace or many short tool calls make that a measurable cost, and files created by processes that outlive the run-end pass (background servers, daemons) stay private until the next pass in that workspace, so the Go Core cannot read or delete them (rewind, delivery, project deletion). Limit the walk to what changed and share again when the last process of a run exits. Found in the KI-71 review (2026-10-01). Partly addressed by S7-H (KI-96): the per-call pass, now run as the tenant's tool UID, reads ACLs only of entries changed since the call started (the walk still visits the whole workspace, about 0.16 s per 100,000 entries), and a full pass runs at the end of each work item and, after the tenant's leftover processes are killed, on the tenant's other workspaces when its last work item in a worker ends; a targeted pass is Open question 10 of the [KI-96 plan](plans/ki96-tenant-tool-isolation-plan.md).

#### Follow-ups from the S7-A work (2026-10-02)

- [ ] **KI-104 Outbound-policy gaps outside the MCP path** (low): `netutil.SafeTransport` and `IsPrivateIP`, used by the A2A, VCS account and project-git callers (`internal/service/a2a.go`, `vcsaccount.go`, `project_git.go`, `handlers_agent_features.go`), lack CGNAT (100.64/10), multicast and NAT64 (and the other ranges `OutboundPolicy` adds) and have no allowlist; they could adopt `OutboundPolicy`. `OutboundPolicy` and the worker's `GuardedTransport` count 6to4 (2002::/16), Teredo (2001::/32) and Azure's 168.63.129.16 as public. URL query tokens in other (non-MCP) URLs are not covered by the MCP redaction (`RedactURLField`; only logs redact them, via `RedactURL`). MCP residuals: stdio servers and the agents' own Bash and tools can reach internal hosts from the worker's network (the worker's check is defense in depth); with `mcp.use_proxy` the address is not pinned, so DNS rebinding is left to the proxy's egress policy; secrets in other argument shapes (`--header=Authorization: Bearer x`, a url with userinfo as an argument) stay visible. Found in the S7-A work (2026-10-02). The GitLab PM provider uses `OutboundPolicy` (KI-85, `pm.allowed_private_hosts`); the Plane and Gitea PM providers do not yet (KI-108).

#### Follow-ups from the S7-B work (2026-10-02)

- [x] (2026-10-02) **KI-105 Knowledge-base `content_path` reads any file** (high): `content_path` was any absolute path (the only check was "non-empty") and the knowledge-base routes had no role check, so any user of a tenant could make the Go Core read a file such as `/run/secrets/...` into agent context (the context fallback) and the worker index a directory. Found in the S7-B review (2026-10-02). **Fixed (S7-B, three review rounds):** content lives in per-tenant areas `<knowledge.content_root>/<tenant_id>/` (`knowledge.content_root` / `CODEFORGE_KNOWLEDGE_CONTENT_ROOT`, default `data/knowledge`, read by the Core and the worker; prod: a read-only `knowledge` volume at `/data/knowledge` in both). The tenant comes from the request context and must be a canonical UUID. `content_path` is stored relative to the caller's area (`.` is the whole area); an absolute path is accepted only inside it. The area is opened with `workspacefs.OpenBelow` on the content root, so `..` and symlinks never leave it, a FIFO never blocks, only regular files are read, and the context fallback reads at most 8 KiB. Every refusal (outside the area, missing, leads out, not a file or directory, area missing) is the same 400 ("content_path is not available in this tenant's knowledge area (knowledge.content_root/<tenant>)"), so no tenant can probe paths; the real reason is only logged. Creating, updating, deleting and indexing a knowledge base need the admin role (reads are unchanged). Attaching and detaching a knowledge base to a scope require both to belong to the caller's tenant (404 otherwise), the link carries the tenant, and scope listings show only the caller's tenant's knowledge bases; the attach and detach routes themselves still have no role check (any authenticated user of the tenant). A knowledge base whose stored path no longer lies in its tenant's area is skipped before retrieval as well, so an index built earlier is never served from memory (without the knowledge-base service no knowledge-base context is added). `retrieval.index.request` gains `knowledge_path` (omitempty; `workspace_path` stays empty for `kb:` requests, and a project index without a `workspace_path` is refused). The worker requires a canonical `tenant_id`, opens `<root>/<tenant_id>/` first and resolves `knowledge_path` inside it (the content root itself may be a symlink: it is an operator directory); it refuses a `kb:` request with a `workspace_path`, `knowledge_path` on a non-kb project, and an empty, absolute, escaping, symlinked-out or missing path, all with the same single answer. Existing rows pointing elsewhere (an absolute path, or a round-2 root-relative `<tenant>/docs`) are refused when indexed or read and logged once per knowledge base; no migration (the shared dev database has no rows, production none). The dead `internal/port/filesystem` and `internal/adapter/osfs` are removed. Possible follow-up: the UI still shows the create, edit, delete and index buttons to non-admins, who now get 403 (as KI-98 for MCP).
- [x] (2026-10-02) **KI-106 `detect-stack` scans other tenants' workspaces** (low): `POST /detect-stack` checked the path only lexically against the whole workspace root, so a user could scan another tenant's workspace and see its framework names (also the tenant directory, the root itself, and another tenant's workspace through a symlink). Found in the S7-B review (2026-10-02). **Fixed (S7-B):** the rule of Adopt: the path must lie strictly inside the caller's `<workspace_root>/<tenant_id>` and is resolved with `workspacefs.OpenBelow`; outside the workspace root only platform admins, and only strictly inside a `workspace.adopt_roots` directory; everything else is 400.
- [x] (2026-10-02) **KI-107 Benchmark dataset paths read any file** (low, dev mode): a benchmark run's dataset could be any absolute path, which the worker read, and the dataset listing followed symlinks out of the datasets directory. Found in the S7-B review (2026-10-02). **Fixed (S7-B):** a dataset is a name or a path inside `benchmark.datasets_dir` and is stored relative; an absolute path outside it is 400 before the run is stored. The Core resolves and lists datasets through `workspacefs.OpenOperatorDir` (os.Root; `.yml` names resolve now; a symlink out of the directory fails the run with 400; files up to 10 MiB). The worker reads every dataset below its own datasets directory with `WorkspaceRoot.operator_dir` (`read_dataset_text`: an absolute path is accepted only inside that directory, anything else raises `DatasetPathError`; 10 MiB cap) and looks a relative `datasets_dir` up below the working directory, then its parent (in dev the worker runs from `workers/`), then `CODEFORGE_WORKSPACE`; the built-in provider defaults are dataset names.


#### Follow-ups from the S7-C work (2026-10-02)

- [ ] **KI-108 PM providers outside GitLab have no outbound policy** (low): the Plane (`adapter/plane`) and Gitea/Forgejo/Codeberg (`adapter/gitea`) PM providers use plain `http.Client`s, copy response bodies into errors and read answers without a limit. Manual `POST /projects/{id}/roadmap/sync` (admins and editors) takes their `base_url` from `provider_config`. That is blind SSRF to private addresses: the answer is a generic 500 and the body lands only in the Core log. Fix: the GitLab pattern (`SetOutboundPolicy` with `pm.allowed_private_hosts`, errors without bodies, `netutil.SameOriginRedirect`, a size cap). Also: a manual sync to a refused host answers 500, not a 400 that names the refusal; GitLab PM requests no longer use the environment proxy, so a `pm.use_proxy` (like `mcp.use_proxy`) could follow if deployments need one. Found in the S7-C review (2026-10-02).
- [x] (2026-10-06) **KI-109 No UI for webhook management** (low): per-project webhooks (KI-85) are registered, listed, rotated and deleted through the API only (`/api/v1/projects/{id}/webhooks`); the frontend has no screen for them. Found in the S7-C work (2026-10-02). **Fixed (S9-D):** a Webhooks panel on the project page (and a "Manage webhooks" link in the project settings) lists, registers, rotates and deletes webhooks and sets a PM webhook's API token; actions follow the route roles (admins change, editors list); a generated secret is shown once, kept only in memory and never shown in another project (also after a project switch during the request); `ConfirmDialog` gained a busy state and persistent toasts are no longer evicted. Open: KI-224.
- [ ] **Idempotency cache keys (note, low):** `middleware.Idempotency` keys anonymous requests by the raw `Idempotency-Key`. Authenticated keys `<user-id>:<key>` are invalid NATS KV keys, so authenticated requests are never cached. Fixing the separator requires excluding the secret-returning routes (webhook register and rotate) first. Webhook deliveries already bypass the cache. Found in the S7-C review (2026-10-02).

#### Follow-ups from the S7-H work (2026-10-03)

- [ ] **KI-110 Tool processes share the network and local IPC across tenants** (medium): Stage 3 of S7-H. Tool processes of different tenants still reach each other over loopback TCP and every internal service on the worker's network (`core:8080`, `litellm:4000`, postgres, nats; all need credentials), over abstract unix sockets on kernels with Landlock ABI below 6, over SysV IPC and POSIX message queues (the IPC namespace is shared), and through `/dev/shm` (name squatting and the shared 64 MB; files there are 0660 and private to the creating tenant). Fix: an egress proxy with per-tenant allowlists (as Claude Code's sandbox-runtime proxy does); Landlock TCP rules (ABI 4) limiting `connect` to the proxy and the allowed internal ports, and `bind`; the abstract-socket scope required (ABI 6); SysV and POSIX IPC, and names and capacity in `/dev/shm`. Evidence: [KI-96 plan](plans/ki96-tenant-tool-isolation-plan.md) E7 T4c, E10 R1 and R3. Found in the S7-H planning (2026-10-02).
- [ ] **KI-111 Claude Code runs expose the platform's Claude credentials to agent commands** (medium, to verify): the Claude Code CLI gets `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` and `CLAUDE_CODE_OAUTH_TOKEN`, and its Bash tool inherits them, so an agent can read the platform's Claude credential and write it into its workspace (not verified in a CodeForge run). Options: a credential proxy outside the sandbox boundary, or `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1` (to be tested in a container). Found in the S7-H review (2026-10-02).
- [ ] **KI-112 No tenant erasure; tool UIDs of deleted tenants are never reclaimed** (low): there is no tenant deletion, and the tool UID range (20000-29999) allows 10,000 tenants with tool work. When tenant deletion is added, it must refuse while the tenant has work, delete every project workspace through the worker, stop the tenant's tool processes in every worker, remove the HOME's content as the tenant's tool UID and then the HOME and the tenant directory, and record the UID in `retired_tool_uids` (keeping `<root>/.codeforge/uids/<uid>`). A UID may be reused only after a reclaim check (no files, no ACL entries naming it, no processes). See D11 of the [KI-96 plan](plans/ki96-tenant-tool-isolation-plan.md). Found in the S7-H review (2026-10-02).
- [ ] **KI-113 Orphaned tool processes stay zombies** (low): the worker is PID 1 of its container and collects only the tool processes it killed (reaper); an orphan that exits by itself, or a child of a worker helper killed after its timeout (the whole process group is killed, the children are reparented to the worker), stays a zombie until the container restarts. Fix: `init: true` for the worker in compose, or a SIGCHLD collector in the worker. Found in the S7-H work (2026-10-02).
- [ ] **KI-114 Go cannot check which model an LLM call uses** (low): LLM permission requests from the worker carry no model, so Go cannot hold a run or a sub-agent to the model tier its mode or profile allows; only the budget bounds a worker that picks a more expensive model. Fix (owner decision 2026-10-03): the requests carry the model and Go checks it against the allowed tier. Found in the KI-25 planning (2026-10-02).
- [ ] **KI-115 Pending tool-call approvals share one consumer window** (low, to verify): the Go durable on `runs.toolcall.request` allows 100 unacknowledged messages for all tenants, and a call that waits for a human approval stays unacknowledged, so one tenant's pending approvals could stall every tenant's tool calls. Fix: a per-tenant cap on pending approvals. Found in the KI-25 planning (2026-10-02).
- [ ] Opt-in submodule initialisation in the worker (KI-88 follow-up, owner decision 2026-10-03): a new at-most-once subject such as `workspace.submodules.init`, run as the tenant's tool user under policy (ADR-016 treatment, its own plan); after KI-88.
- [ ] Landlock ABI warning (KI-96 follow-up, owner decision 2026-10-03): the production minimum stays ABI 2; below ABI 6 the worker logs a warning and reports it in `/health`, and a setting can require ABI 6.
- [ ] Per-tenant Claude credentials (KI-96 follow-up with KI-111, owner decision 2026-10-03): each tenant stores its own Claude token (encrypted at rest, like the other provider keys) and only its Claude Code runs get it; until then operators pass `CLAUDE_CODE_OAUTH_TOKEN` or `ANTHROPIC_API_KEY`, and the operator's `CLAUDE_CONFIG_DIR` stays out of production tool processes.
- [ ] Background processes per conversation (KI-96 follow-up, owner decision 2026-10-03, own plan first): a tool process belongs to the conversation or run that started it and lives while that work is active, with an idle timeout, instead of ending when its tenant is idle in the worker; a subreaper per conversation also collects orphans (fixes KI-113 for tool processes). Prerequisite for live previews of a running app in the web UI.

#### Found in the README claim audit (2026-10-03)

- [x] (2026-10-05) **KI-116 Production images are never published as `latest`** (medium): `docker-compose.prod.yml` defaults `CORE_IMAGE`, `WORKER_IMAGE` and `FRONTEND_IMAGE` to `:latest`, but `.github/workflows/docker-build.yml` tags only branches, SHAs, semver and `VERSION`, and there are no release tags, so a fresh `up -d` cannot pull the images. Workaround (README): `docker compose -f docker-compose.prod.yml build`, or set the image variables to `:main` / `:0.8.0`. **Owner decision (2026-10-04): **release tags: a `v*` tag on main builds the images as the version and `latest`; the production compose file pins the version from `VERSION` by default (written by `scripts/sync-version.sh`). Releases only on the owner's explicit request. **Fixed (S9-B):** a `v*` tag on main naming `VERSION` builds core, worker and frontend as `<version>`, `<major>.<minor>` and `latest` (the tag must be `v<VERSION>` on a commit of main; prereleases get no `latest`); branch pushes tag the branch name and SHA only (they no longer overwrite the version image); `docker-compose.prod.yml` pins `:<VERSION>`, written by `sync-version.sh`; the check scripts default to it. Owner decision (2026-10-05): branch builds keep only branch and SHA tags; `:<VERSION>` and `latest` come only from release tags. **Owner decision (2026-10-06):** VERSION goes to 0.9.0, because the GHCR tag `0.8.0` very likely holds a March staging build (branch pushes used to tag images with VERSION); the compose pins can then only resolve to an image the release job built. **Review fixes (2026-10-06):** the build jobs have `attestations: write` (every image build had failed at the provenance step since 2026-03-28); a release tag must be a first-parent commit of `main`; `latest` and `<major>.<minor>` move only forward, counting only releases on `main`; VERSION is 0.9.0, so the compose pins resolve only to release images (the GHCR tag `0.8.0` holds an old staging build).
- [x] (2026-10-05) **KI-117 The Go Core image lacks `gh` and `svn`** (medium): the GitHub Issues PM provider (`internal/adapter/githubpm/provider.go`) and PR delivery (`internal/service/deliver.go`) run `gh`, the SVN provider (`internal/adapter/svn/provider.go`) runs `svn`, but `Dockerfile` installs only git, so these features fail in production. PR creation moves to the provider REST API with KI-88 (owner decision 2026-10-03); the GitHub PM provider and SVN need the same decision (REST API, or the binaries in the image). **Owner decision (2026-10-04): **the GitHub PM provider uses the REST API (like pull requests, S1-B); the `svn` binary goes into the Core image and is called only through the hardened SVN adapter. Confirmed in live session 2 (2026-10-04): the github-issues PM preview answers 500 without `gh`. **Fixed (S9-B):** the github-issues provider and PR creation use the GitHub REST API (`internal/adapter/githubapi`); `gh` is gone from the Go Core; operator token `github.token` (default tenant only); PR delivery opens the PR for the project's `repo_url` with the github-api project token or `github.token` (github.com, default tenant) and records why in the `delivery.completed` audit entry when it opens none; GitHub errors answer 400/404 instead of 500; `svn` is in the Core image, called only by the SVN adapter.
- [x] (2026-10-05) **KI-118 Agent backends cannot run on a stock production deployment** (medium): the CLIs of Aider, Goose, OpenCode, Plandex, SWE-agent and Claude Code are not in the worker image, OpenHands defaults to `http://localhost:3000`, and the production worker is on an internal network without internet access, so only the built-in agent loop runs tasks. The same network rule makes external HTTP MCP servers and benchmark dataset downloads unreachable from the worker. SWE-agent has a Python executor but no Go adapter. Fix: an optional worker image (or documented extension) with the backend CLIs, a documented egress path for MCP servers, and a decision on SWE-agent. **Owner decision (2026-10-04): **all backend CLIs go into the standard worker image (not a separate image), with a documented egress path for backends and external MCP servers. **Fixed (S9-B):** Aider 0.86.2, Claude Code 2.1.289, OpenCode 1.18.34 and Goose 1.29.0 are in the worker image (pinned and checksummed, on the tool PATH, run as the tenant tool user, self-updates and analytics off; about +1.3 GB uncompressed); the OpenCode executor's argument bug is fixed; `docker-compose.egress.yml` is the opt-in route out; Plandex, SWE-agent and OpenHands are not shipped (reasons in `workers/tests/test_backend_clis.py`). Open: KI-164, KI-165, KI-166. **Owner decision (2026-10-06):** Claude Code is a build option (`INSTALL_CLAUDE_CODE`), not part of the public images, because its licence does not allow redistribution in them. **Done (2026-10-06):** `Dockerfile.worker` installs the pinned, checksummed Claude Code CLI only with `--build-arg INSTALL_CLAUDE_CODE=true`; the release workflow never sets it; a missing CLI fails with a build hint. `docker-compose.egress.yml` explains what the override opens (host LAN, host ports, cloud metadata) and gives a `DOCKER-USER` example that keeps DNS and deliberately opened hosts reachable.
- [x] (2026-10-05) **KI-119 The first-admin setup is first come, first served** (medium): `POST /api/v1/auth/setup` (`internal/adapter/http/handlers_auth.go`) is public until the first user exists, and `setup_timeout_minutes` is only a countdown in the UI, so on an exposed host anyone who reaches `/setup` first becomes admin. Fix: a one-time setup token (written to the Core's log or a file) required by the endpoint, or setup restricted to loopback. The README tells operators to finish setup before exposing the host. **Owner decision (2026-10-04): **a one-time setup token: the Core creates it on first start, writes it to its log and `data/setup_token`, and `POST /auth/setup` requires it; the env admin (`CODEFORGE_AUTH_ADMIN_PASS`) stays as the alternative. **Fixed (S9-A):** on a start without users in the default tenant the Core creates a one-time setup token, logs it once (INFO, `SETUP TOKEN: ...`) and writes it to `auth.setup_token_file` (default `data/setup_token`, `/data/setup_token` in production, 0600). `POST /auth/setup` requires it as `setup_token` (constant-time compare, bound to the tenant): 403 without or with a wrong token, 409 once users exist; a successful setup deletes it. Each start replaces it; with users none is armed. The env admin and the generated initial password need no token. Setup of other tenants through `X-Tenant-ID` is refused (it was open). The setup page asks for the token and says where to find it.
- [ ] **KI-120 Multi-tenancy has no path in the web UI** (medium, to verify): the frontend never sends a tenant and login uses the request's tenant (`handlers_auth.go`, `middleware/tenant.go`), so users of other tenants cannot log in through the UI, and there is no tenant management screen; tenants exist only through the API. **Owner decision (2026-10-04): **the tenant comes from the e-mail address: e-mail addresses become globally unique and login resolves the tenant from the account (a chooser after login if one address belongs to several tenants), plus a tenant screen for platform admins. Low priority (single-user focus).
- [x] (2026-10-04) **KI-121 Frontend gaps found in the audit** (low): `features/search/SearchPage.tsx` is not routed anywhere (the full-text search API is used only by the agent tool); GDPR export, erasure and consent (`/me/export`, `/me/data`, `/me/consent`) have no UI; `/channels/:id` and `/design-system` are missing from `KNOWN_ROUTES` (`frontend/src/App.tsx`) and render without the app shell and route guard; the roadmap UI never calls the bidirectional `POST /projects/{id}/roadmap/sync`. **Fixed (S7-G):** `/search` (`SearchPage`) is routed in the app shell with a sidebar entry; conversation hits open in their project's chat (`/projects/<id>?conversation=<cid>`). The GDPR self-service UI is Settings > Privacy (export, erasure, consent). `/channels/:id` and `/design-system` render in the app shell behind the route guard (`frontend/src/lib/appRoutes.ts`; a test checks every route of `index.tsx`). The roadmap panel offers "Sync with PM" (`POST /projects/{id}/roadmap/sync`) next to Import from PM: direction pull, push or both; create new, update existing; preview (dry run, default on); and an optional token (`provider_config.token`, `api_token` for Plane). The base URL is not offered (KI-108). Also fixed: the command palette's "models" command went to `/models`, which is not a route (now `/ai`; a test checks every palette target against the router). **Review fixes (S7-G):** the route guard fails closed: every path except the six public pages is guarded, compared lowercase and without empty segments as the router matches (`/SETTINGS`, `/settings/` rendered without the guard before); unknown paths show their 404 inside the shell after sign-in. A linked conversation (`?conversation=`) opens only if it belongs to the page's project. The roadmap sync clears its token on a provider change and after a real sync.
- [ ] **KI-122 GitHub Copilot models are never registered with LiteLLM** (low): `internal/adapter/copilot/register.go` `RegisterWithLiteLLM` has no caller and the Copilot entry in `litellm/config.yaml` is commented out, so the token exchange works but no Copilot model appears; `workers/codeforge/routing/router.py` still lists `github_copilot/*` among its defaults.
- [ ] **KI-123 Configuration leftovers** (low): `.env.example` lists keys for DeepSeek, Cohere, Together AI and Fireworks that have no LiteLLM entry and are passed by neither compose file; `CODEFORGE_AGUI_ENABLED` is parsed but never read; the production nginx (`frontend/nginx.conf`) does not proxy `/a2a` and `/.well-known/agent-card.json` (only the blue-green Traefik setup does).
- [x] (2026-10-05) **KI-124 The dev container does not start a working stack as documented** (low): it sets no `APP_ENV=development` and no `CODEFORGE_INTERNAL_KEY` (worker calls to the Core get 401), takes `LITELLM_MASTER_KEY` from the host environment (empty unless set) and hardcodes the `codeforge_dev` database password; the old README's Codespaces commands ran the worker from the repository root, where the package is not installed. The new README and `docs/dev-setup.md` give the working order. **Fixed (S9-A):** `devcontainer.json` sets `APP_ENV=development`, `LITELLM_MASTER_KEY` and `POSTGRES_PASSWORD` (host value or the dev default), used by `DATABASE_URL` and `docker compose` in `setup.sh`; `.devcontainer/dev-env.sh` gives Core and worker one `CODEFORGE_INTERNAL_KEY`, generated per container. Tested in `workers/tests/test_devcontainer.py`.

#### Live end-to-end testing and the autonomous goal benchmark (owner request 2026-10-03)

- [ ] Reusable live-test environment: the development stack plus a local Ollama model, started by one script (from the README screenshot run), documented in `docs/testing/e2e-setup.md`.
- [ ] A live E2E session at the end of every milestone, logged in [docs/testing/live-e2e-findings.md](testing/live-e2e-findings.md) (session 1: 2026-10-03, README screenshots; session 2: 2026-10-04, KI-147 to KI-160 and benchmark run 1). Reusable stack: `scripts/live-e2e/`.
- [x] (2026-10-05) S9-C: pure-completion models call tools through a text tool protocol (one JSON object per reply, parsed by the worker, JSON-schema grammar where the server supports it, one repair per malformed reply, results fed back as `<tool_result>` text; no NATS, Go or frontend change); plan: [docs/plans/text-tool-protocol-plan.md](plans/text-tool-protocol-plan.md) (owner decision 2026-10-04). Implemented in the worker ([ADR-021](architecture/adr/021-text-tool-protocol.md)); also fixed: stall detection counted retries and nudges.
- [x] (2026-10-06) S9-C review fixes (`efcbe59e..7c7a004e` in the agent branch, three review rounds): a call nested in a broken object never runs; `<think>` stripping no longer reaches into JSON strings (the LLM client keeps `raw_content` for the parser); the parser and the client's think stripping are linear (a crafted reply took up to 54 s before); deep nesting is a repair, not a crash; the stream never shows raw call JSON; an unusable reply is a failed routing outcome; `max_tokens` fits a known context window and is dropped after a context-length 400. Residual edge cases: KI-225. Follow-up: the live display still hides text between think tags anywhere in the stream (display only).
- [x] (2026-10-06) Claude Code MCP servers for working on this repo (owner decision): gopls v0.21.1, Playwright MCP 0.0.83, Context7 and Serena 1.7.0 (read-only trial) in `.mcp.json`; the SessionStart hook installs gopls and pre-warms the npx/uvx caches; details in tech-stack.md and dev-setup.md.
- [x] (2026-10-06) AGENTS.md update (owner decision, all four options): new rules (RequireRole on every route, `context.WithoutCancel` for work after a request, no waiting on agent-writable state, outbound policy and credential scope, cost for every LLM call), corrected facts (0.9.0, shipped backends, ADR 019/020 unassigned, single-test link), the E2E start and dev container details moved to dev-setup, subagent rules (read-only snapshot reviews, finding format, gopls CLI in worktrees, serialized heavy runs, private databases dropped); under 3,000 words.
- [x] (2026-10-06) CI Security Scanning: gitleaks reported synthetic webhook, idempotency and GitHub PM test fixtures (commits of 2026-10-02 and S9-B); their exact values are now in the `.gitleaks.toml` allowlist (checked with gitleaks 8.28 over the PR range: no leaks).
- [x] (2026-10-06) Small coder model (owner decision): `qwen3.5:4b-q4_K_M` replaces `qwen3:4b-instruct` as the live E2E and benchmark default (`LIVE_MODEL`); smoke test and candidates in [testing/model-smoke-test.md](testing/model-smoke-test.md). The worker sends `reasoning_effort: none` with local model calls (`litellm.local_reasoning_effort` / `CODEFORGE_LOCAL_REASONING_EFFORT`; `off` sends nothing). Open: benchmark run 2 with the new model; Granite 4.2 3B and MiniCPM5-2B were not measured; like `top_k`, the parameter also reaches a cloud fallback of a local primary model.
- [ ] S9-C live check (plan section 7 live steps 0-5, idle host; results in testing/live-e2e-findings.md), then tune `max_tokens` 8192 and the one-repair limit.
- [ ] S9-C follow-ups: align or drop the `pure_completion` allowlist (`tools/capability.py`; it applies only without a user prompt and lacks `list_directory`); `ToolDefinition.examples`/`output_format` unused since the full guide was removed; stop sequences or an early stream stop without a grammar; raw-code payloads (JSON tax); read Ollama's `num_ctx`; `transition_to_act` for native models.
- [x] (2026-10-04) Autonomous goal benchmark, preparation: the hidden acceptance suite (80 cases, `testdata/autonomous-goal/mdlinkcheck/acceptance/`), the grader (`grade.py`, `judge_prompt.md`) and a reference solution (kept outside the repository) that scores 100 %; two weak variants score 90 % (acceptance 77.5 %, not a success) and 37 %; a mutation check of 22 spec violations is caught; SPEC.md gained a Details section for the ambiguities found.
- [ ] Autonomous goal benchmark runs ([docs/testing/autonomous-goal-benchmark.md](testing/autonomous-goal-benchmark.md)): the local model first, then, once the owner provides a key as an environment secret, a cloud model.

#### Found while taking the README screenshots with a local model (2026-10-03)

- [x] (2026-10-04) **KI-125 Local models cannot use the agent tools with the shipped setup** (high): four defects together keep an Ollama-only installation from running agents. (1) Tool support is guessed from the model name (`workers/codeforge/tools/capability.py:40-58`): local models without "instruct", "coder" or "qwen3" in the name count as pure completion, get no tools (`agent_loop.py:396-398`), and tool calls written as text are not parsed. (2) Agent runs with routing off and no model in the agent config get no tools: the capability check runs before a model is chosen and an empty name counts as pure completion (`executor.py:183-200`, `capability.py:117-118`). (3) Streamed parallel tool calls are merged by index only (`llm.py:1022-1034`); LiteLLM's `ollama_chat` provider gives every call index 0, so the arguments become `{...}{...}` and the next request fails with a LiteLLM 500. (4) The `ollama/*` route in `litellm/config.yaml` streams tool calls as JSON text; routing Ollama through its OpenAI-compatible `/v1` endpoint (`openai/*` with `api_base`) works. Also, without API keys the wildcard routes list about 990 models from public provider lists. **Fixed (S8-A):** tool capability is decided by an operator override (`litellm.model_capabilities` / `CODEFORGE_MODEL_CAPABILITIES`, `pattern=level`, first match wins), then LiteLLM `/model/info` `supports_function_calling` (cached 60 s), then the name; a run without a model resolves the default model before the check; streamed tool calls with the same index but another id or a new name are separate calls; `ollama/*` goes to Ollama's OpenAI-compatible `/v1` endpoint (`OLLAMA_OPENAI_API_BASE`, derived from `OLLAMA_BASE_URL` in both compose files) with every scenario tag; the Core lists each keyless cloud catalogue route as one entry (`groq/*`) instead of LiteLLM's expansion (593 to 12 rows without keys), and the default model is never a route. Verified end to end with `qwen3:4b-instruct` (`read_file`, `edit_file`, `bash` through LiteLLM). Still open: tool calls written as text by pure-completion models are not parsed (see the product notes in `docs/testing/live-e2e-findings.md`). **Review fixes (S8-A R2, 2026-10-04):** the collapse also hid the models of providers whose key is set (LiteLLM gives every row of a wildcard route the same deployment id, keyed or not), so a cloud-only install had no default model and every chat message failed with "no LLM model configured". The Core and the worker now learn which providers have a key from `litellm.keyed_providers` / `CODEFORGE_LITELLM_KEYED_PROVIDERS` (names only; `docker-compose.prod.yml` derives it from the key variables with `${VAR:+name,}`), plus the key variables of their own environment: a keyed provider's models are listed and can be the default model, a keyless catalogue expansion stays one entry (`groq/*`), and the worker's routing uses the same rule (openrouter, cerebras, chutes and aihubmix were "unknown, assume keyed" there). With no model known the Core dispatches the run without one; the worker resolves it (routing, `CODEFORGE_DEFAULT_MODEL`, its discovery) before it sizes the prompt, or fails the run naming what to configure. No routing stats for `*` routes. A YAML `litellm.model_capabilities` that is not a mapping stops the worker.
- [x] (2026-10-04) **KI-126 A failed tool command loses its output** (medium): `workers/codeforge/loop_helpers.py:75-80` returns only "Error: exit code N", so the traceback of a failing test reaches neither the model nor the chat, and the agent cannot fix what it cannot see. **Fixed (S8-B):** a failed tool's result is the error line, its output and the hint, bounded to `agent.tool_output_max_chars` (head and tail) for the model and the chat tool card; the repeated-error tracker compares the output too, so two different failing commands no longer trigger NON-RETRYABLE; quality gates send `exit code N` and bounded output (`tool_output_max_chars` on `runs.qualitygate.request`), and `run.qualitygate.failed` records the failed checks' output; `search_files` keeps its matches on grep errors. Open: a timed-out bash command loses its partial output (KI-133). **Review fixes (S8-B R2):** `truncate_tool_result` keeps exactly `max_chars` characters (for 1 it returned the whole text) and counts the omitted ones exactly; `agent.tool_output_max_chars` must be 0..80000 (startup error otherwise) and the worker clamps it.
- [x] (2026-10-04) **KI-127 Token usage of streamed OpenAI-compatible replies is never requested** (low): `llm.py:913-948` does not send `stream_options.include_usage`, so tokens stay at 0 for Ollama and other OpenAI-compatible backends on the cost page and in runs. **Fixed (S8-A):** streamed requests send `stream_options.include_usage`, and a usage chunk with an empty choices list is read; token counts arrive for Ollama and other OpenAI-compatible backends.
- [x] (2026-10-04) **KI-128 Bash rules refuse `VAR=value cmd` when the profile has a deny list** (low): `internal/domain/policy/evaluation.go:228` treats a leading variable assignment as not statically analysable, which blocks common commands such as `PYTHONPATH=src python -m pytest`. Fix: parse plain leading assignments (literal values) and check the command after them; keep failing closed for expansions. **Fixed (S8-B):** plain leading assignments and `env NAME=value cmd` are accepted and the command after them is checked; they stay fail closed for non-literal values, `+=`/`[i]=`, quoted names, assignments without a command or after a wrapper, and every variable not on the allow list in `internal/domain/policy/command_env.go`; ADR-015 has the list. Also fixed: `time VAR=x cmd` hid the command from deny lists. **Review fixes (S8-B R2, 2026-10-04):** the variable deny list is replaced by an allow list in `command_env.go`. It takes exact, case-sensitive names: output switches, the locale with no `/` in the value, and `PYTHONPATH`/`NODE_PATH`. Every other name fails closed, lower-case and former names included. Any assignment before `make`/`gmake` fails closed. A wrapper reached through `xargs` fails closed; this was a regression, since `echo x | xargs env` hid the command. `xargs --replace/--eof/--max-lines` take a value only after `=`; before, `xargs --replace curl x` hid curl. More command runners (`ionice`, `taskset`, `script`, ...) are opaque. **Review fixes (S7-F R2, 2026-10-04):** a simple command that sets `PYTHONPATH` or `NODE_PATH` (leading or `env`), or runs under `pipenv run` (which loads the project's `.env`), stays analysable: deny lists, deny/ask rules and a deny-list profile treat it as before, but no allow rule that names commands matches it (command allow list, an allow rule's sub-pattern; Allow-Always refuses with 400), because `PYTHONPATH=. python3 -m json.tool` runs `./sitecustomize.py`. The safe preset now denies `PYTHONPATH=src python -m pytest`. make's options are parsed like getopt: `-E` anywhere in a short cluster, any long option not known exactly (`--ev=` is `--eval=`), and `-f -` fail closed. `poetry run`, `uv run`, `pipenv run`, `pdm run` and `bundle exec` are wrappers: deny lists see the command they run, and allow rules name it (`pytest`, not `poetry run pytest`); an option before it, the run subcommand after an option or another subcommand (`uv tool run`), a computed subcommand and xargs into them fail closed.
- [ ] **KI-129 UI defects found while taking the screenshots** (low): conversation runs count neither on the dashboard nor on the cost page, and the "First agent run" onboarding step and the dashboard activity never update for them (`ProjectDetailPage.tsx:428`); a new project without runs shows as critical (score 35, `internal/domain/dashboard/dashboard.go:44-53`); the approval card always counts down from 60 s, whatever the server's approval timeout (`PermissionRequestCard.tsx:26`, `ChatMessages.tsx:266`); `PolicyPanel.tsx:18-23` misses the `supervised-ask-all` preset and offers to delete it; long model IDs overflow their card (`ModelsPage.tsx`); Feature Map titles are cut off (`featuremap/FeatureCard.tsx:75`); the Routing page keeps stats of removed models; during a run tool cards appear only when the run ends, and the branch badge updates only on reload. **Owner decision (2026-10-04): **(vision) one "agent work" read model across conversation runs, API runs and plans (status, cost, approvals), used by the dashboard, the cost page and a new agent-work page; storage stays as it is. **Owner decisions (2026-10-06):** a chat turn's status, model, cost and tokens are stored with its exactly-once completion claim (`conversation_turn_completions`), and a deleted conversation's turns keep their cost until the cost retention purges them. Plan: [`plans/2026-10-06-agent-work-read-model.md`](plans/2026-10-06-agent-work-read-model.md). **Partly fixed (S9-D UI, one review round, 2026-10-06):** a project without runs in 7 days has health level `unknown` (muted dot, "No runs in the last 7 days"); `GET /policies` returns `presets` (`policy.PresetNames()`, all five), the panel marks them and offers delete only for custom profiles; long model IDs and Feature Map titles wrap with a tooltip; the Routing page strikes through the stats of models that are neither configured, covered by a provider wildcard nor in the registry's `/llm/available` (nothing marked when a list fails to load); live tool cards and the branch badge during a turn (see KI-161), a failed git-status refresh keeps the page (the badge hides until the next refresh). Still open: conversation runs on the dashboard, the cost page and the onboarding step (the agent-work read model, plan below) and the approval card's countdown.
- [ ] **KI-130 Local-only setups run into cloud defaults** (low): the default embedding model is `text-embedding-3-small` (`internal/config/config.go:717`), so retrieval indexing logs an error on every clone without an OpenAI key, and `orchestrator.default_embedding_model` can be set only in YAML (no env var); the frontend `postinstall` downloads 805 file icons from the network (offline installs show broken icons); LiteLLM's health check of the wildcard routes probes `gpt-5-nano` and logs errors. **Partly fixed (S8-A, 2026-10-04):** `CODEFORGE_ORCH_EMBEDDING_MODEL` sets the embedding model; when it is unavailable, retrieval builds a BM25-only index with one warning per model (rebuilt in full once the model works); LiteLLM no longer health-probes the wildcard routes (`disable_background_health_check`, `health_check_skip_disabled_background_models`). Still open: `vscode-icons-js` ships no SVGs, so the frontend `postinstall` downloads 805 icons from the unpinned `master` of the vscode-icons repository; vendor them (about 4.6 MB, MIT) or pin a release tarball with a checksum. Review fix (2026-10-04): only an unusable embedding model (401, 403, 404, or a 400 naming an unknown model) gives a BM25-only index; a rate limit, 5xx or timeout fails the build (`error`) and keeps the existing index. The index status reports `bm25_only` (NATS `retrieval.index.result`, `GET /projects/{id}/index`, `retrieval.status` event); `GET /projects/{id}/index` now uses the snake_case names the frontend reads.
- [x] (2026-10-05) **KI-131 Untagged cloud routes refuse scenario requests** (medium): with routing off, LiteLLM tag filtering answers 401 to a request whose scenario tag (think, plan, review, longContext, background) the route does not carry; the `openai`, `anthropic`, `gemini`, `chutes` and `aihubmix` routes in `litellm/config.yaml` carry no tags, so modes with a scenario fail on those providers. Fix: give every route the scenario tags (as the `ollama/*` route has since S8-A) and test it. Found in S8-A (2026-10-04). **Fixed (S9-A):** every route in `litellm/config.yaml` carries `default` and every scenario tag; `workers/tests/test_litellm_config.py` checks every `model_list` entry.
- [ ] **KI-132 A full-auto project without goals always gets the goal researcher** (low, to verify): the Core's full-auto check sends a conversation in a project with a full-auto preset and no goals or open features to `goal_researcher` (read-only, plan/act), whatever mode the user picked, so a direct coding request does nothing visible. Decide whether that is intended; if so, tell the user in the chat why. Found in S8-A (2026-10-04).
- [ ] **KI-133 A timed-out bash command loses its partial output** (low): on timeout the bash tool cancels `communicate()`, so whatever the command printed before is gone; read the output incrementally with a bound. Follow-up of KI-126, found in S8-B (2026-10-04).
- [ ] **KI-134 `python -i` runs stdin past a deny list** (medium): `python -i script.py` reads program text from stdin after the script, and the interpreter check in `internal/domain/policy/` does not flag `-i`, so a here-doc can run code a deny list meant to block. Treat `-i` (and similar options of other interpreters) as inline code. Found in S8-B (2026-10-04).
- [ ] **KI-135 Program-running options of unmodelled tools** (medium): `sort --compress-program=PROG`, `split --filter=CMD` (runs a shell), `zip -TT`, `ssh`/`scp`/`sftp` `-o ProxyCommand`/`LocalCommand` and `-S`, and `bmake`/`pmake` (BSD make also imports the environment) run programs that deny lists do not see, or under an allow-listed name. Add them to `argumentCode` or `opaqueExecutables` in `internal/domain/policy/`. Found in the S8-B review fixes (2026-10-04).
- [ ] **KI-136 `conversation.run.complete` has no size guard** (low, not verified end to end): it carries every tool message of the run, and `publish_with_retry` does not bound it, so a long run can exceed the 1 MiB NATS max payload (`MaxPayloadError`) and the completion is lost. Found in the S8-B review fixes (2026-10-04).
- [ ] **KI-137 Tenant HOME dotfiles steer allow-listed tools** (medium): the tool `PATH` includes tenant-HOME directories (`.local/bin`, `go/bin`, ...) and that HOME is shared and writable by the tenant's runs, so dotfiles there (`~/.gitconfig`, `~/.config/go/env`) can make an allow-listed git or go run programs. Next to the "tenant HOME is shared" residual risk in SECURITY.md; independent of the assignment parser. Found in the S8-B review fixes (2026-10-04).
- [x] (2026-10-06) **KI-138 The UI does not show a BM25-only index** (low): the index status carries `bm25_only` since the KI-130 review fix, but the frontend `RetrievalIndexStatus` type has no such field and nothing tells the user that semantic search is off. Found in the S8-A review fixes (2026-10-04). **Fixed (S9-D UI):** the project's retrieval panel warns that semantic search is off when the index is `bm25_only` (EN + DE).
- [ ] **KI-139 More Go Core state is per replica** (low): after KI-86, WebSocket broadcasts reach only the clients of the replica that sends them (a permission request raised on one replica is not shown to a user connected to the other), the auto-agent registry (`Start`/`Stop` per project) and in-memory run state (stall trackers, heartbeats, conversation-run tracker) live in one process. Run one Go Core replica outside a blue-green switch. Found in S7-F (2026-10-04).
- [x] (2026-10-04) **KI-140 Package runners run project-defined code before the inner command** (low): `poetry run`, `uv run`, `pdm run` and `bundle exec` are wrappers since the S7-F review, and an allow rule naming the inner command matches; the runner first sets up the project environment (`.venv/bin` executables, so `uv run python3` can be a workspace binary; the Gemfile; `uv run`'s project sync and build hooks). This is consistent with matching `./pytest` by basename; the alternative is to never allow-match runner segments (one line, `runnerLoadsDotenv` for all five runners). Lead decision pending. Found in the S7-F review fixes (2026-10-04). **Owner decision (2026-10-04): **keep the current rule: an allow rule naming the inner command matches it, consistent with matching `./pytest` by basename (the runner's project setup is trusted like `conftest.py`). Closed: ADR-015 and SECURITY.md describe the rule.
- [ ] **KI-141 Handoff claims that are never done are never purged; tasks have no retention category** (low): a stage dead-lettered after a transient failure, or a process that died on its last delivery, leaves a claim without `done_at`, which the KI-90 retention keeps forever (possible fix: mark the claim done in `HandleDeadLetteredHandoff`). Tasks are deleted only with their project. Found in S7-E (2026-10-04).
- [x] (2026-10-06) **KI-142 The Quarantine page does not update live** (low): nothing in the frontend consumes `quarantine.resolved`, so an open page does not show a message that was approved, rejected or expired elsewhere until it reloads. Found in S7-E (2026-10-04). **Fixed (S9-D UI, one review round):** the Quarantine page refetches list and stats on `quarantine.alert` / `quarantine.resolved` of the selected project (or of a shown message, as a withdrawal names no project), an open detail of a message resolved elsewhere shows the new status without decision buttons and its review form closes; the listener checks the project first, reads a failed list as empty and coalesces refetches (one in flight plus one queued; `lib/coalesce.ts`).
- [x] (2026-10-05) **KI-143 Access tokens outlive account deletion, erasure and disabling** (medium): access tokens are stateless JWTs, revocable only by token ID, and no list of a user's tokens exists. A deleted, erased or disabled user (and a role change) keeps the old token's rights until `auth.access_token_expiry` (default 15m); refresh tokens and API keys are deleted with the user. Writes that name the user fail since the S7-E review (401), but reads and other actions work. Fix: a per-user token epoch or issued-at check (or a user-row lookup, cached briefly) in token validation. Found in the S7-E review (2026-10-04). **Owner decision (2026-10-04): **a per-user token epoch: `users.token_epoch` in the JWT, compared with a briefly cached value in token validation; deleting, erasing, disabling a user and a role change raise it. **Fixed (S9-A):** `users.token_epoch` (migration 126) is carried in the access JWT (`epoch`); validation compares it with a value cached for 5 s (at most 10,000 users); tokens without the claim, of missing users, with a stale epoch or whose lookup fails are refused. A role change and disabling raise the epoch; delete and erase remove the row; all drop this replica's cache entry, other replicas follow within 5 s. Re-enabling does not restore old tokens. Access tokens issued before this version are refused (users sign in again). **Review fixes (S9-A, 2026-10-05):** a disabled user's API keys are refused (they work again once the user is enabled); a role change or disable is saved in the same statement as the epoch raise (`UpdateUserInvalidatingTokens`), so a failed save changes nothing and a retry raises; admin password reset, the reset link and a password change raise the epoch in the user's own tenant and delete the user's refresh tokens (a password change signs the user out everywhere; the UI signs in again with the new password); ending a user's sessions also closes their WebSocket connections on this replica (`Hub.DropUser`); an epoch read before a raise is not cached after it (generation counter); disabling a user deletes their refresh tokens.
- [ ] **KI-144 `GET /me/export` exports the whole tenant's sessions, conversations and runs** (medium, privacy): `GDPRService.ExportUserData` walks every project of the tenant, so any user, a viewer included, downloads every conversation with its messages, every agent session and every run of the tenant as "their" data. A data subject export should hold the user's own data (conversations they started, messages they wrote, runs they triggered), which needs user attribution on those rows; until then the export should at least respect the caller's read rights. The Settings > Privacy screen states what the export contains. Found in S7-G (2026-10-04). **Owner decision (2026-10-04): **first restrict the export to the account's own data (account, key metadata, consents, audit entries, own channel messages), then add user attribution (`created_by` / `triggered_by`) to conversations, messages and runs and export exactly the user's own. Low priority: CodeForge targets self-hosters and single users, for whom the tenant's data is their own.
- [ ] **KI-145 The last admin of a tenant can erase their own account** (low): `DELETE /me/data` does not stop the last admin of a tenant, which leaves the tenant without an admin; the UI does not warn either. Refuse it (409) with a hint to hand over the role first. Found in S7-G (2026-10-04). **Owner decision (2026-10-04): **409 with a hint to hand over the admin role when other users are in the tenant; allowed when the admin is the tenant's only user (the default tenant then returns to the setup state, guarded by the KI-119 token). Low priority (single-user focus). Note (S9-A): the setup token is armed only at startup, so a tenant that lost its last user gets setup back after a restart.
- [x] (2026-10-06) **KI-146 The knowledge-base page shows admin actions to non-admins** (low): create, edit, delete and index buttons are shown to viewers and editors, who get 403 since KI-105; hide them as the MCP page does since KI-98. Found in S7-B and S7-G. **Fixed (S9-D UI):** create, index and delete are offered to tenant admins only (`hasRole("admin")`), everyone else is told that admins manage knowledge bases; the card labels are translated.
- [x] (2026-10-05) **KI-147 Conversation message order is lost** (high): `CreateToolMessages` inserts one turn's messages with a single `now()` and `ListMessages` orders only by `created_at` (`internal/adapter/postgres/store_conversation.go:110`), so tool results can come before their calls; `conversation_dispatch.go:252` builds the next turn's LLM history from this list. Evidence: 22 messages with 3 distinct timestamps. Fix: a sequence column as tiebreaker. Found in live session 2 (2026-10-04). **Fixed (S9-0):** migration 124 adds `conversation_messages.seq` (identity; existing rows numbered by `created_at`, then heap position); `ListMessages` (HTTP listing, next turn's history, GDPR export) orders by `seq`; index `(conversation_id, seq)` replaces `idx_conversation_messages_conv`. **Review fix (S9-0):** the backfill orders a tool row directly after the assistant row of the same batch whose `tool_calls` hold its call ID.
- [x] (2026-10-05) **KI-148 The chat loses a running turn and its pending approval on reload** (high): after a reload the approval is not shown and is denied by timeout; tool messages are persisted only when the run ends. Fix: restore the running turn and pending approvals on page load; persist tool results incrementally. Found in live session 2. **Fixed (S9-0):** `GET /api/v1/conversations/{id}/run` returns the running turn (active, turn ID, streamed text, pending approvals with `timeout_seconds`/`expires_at`); the chat restores it on load, and approval cards count down to the Core's deadline (they always counted 60 s before); `GET /conversations/{id}/session` answers 204 without a session (was 404 on every load). Open: tool messages are persisted at turn end only, and conversation turns get no live tool cards (KI-161); approvals on another replica are not listed (KI-139). **Review fixes (S9-0):** the approval card's countdown is display only and never denies (the Core enforces the timeout); it counts from `timeout_seconds` (live card) or the Core-computed `remaining_seconds` (restored card), so browser clock skew has no effect. Cards are de-duplicated by run and call; a conversation switch clears cards and streamed text, and a restore answer that arrives after a run started or finished is dropped. Streamed text is kept only for the run's own tenant.
- [x] (2026-10-05) **KI-149 Validation and not-found errors answer 500** (medium): password complexity (`internal/service/auth.go` 67/101/382/449/501 lack `domain.ErrValidation`), a bad PM sync reference, an unknown consent purpose. Fix: sentinel errors and status tests. Found in live session 2. **Fixed (S9-0):** auth validation, an unknown role, a wrong current password, withdrawing a required consent, an unknown PM provider or direction and a malformed PM reference answer 400; an unknown consent purpose and a PM project unknown to GitLab or Plane answer 404. **Review fix (S9-0):** upstream 404s from GitLab and Plane are logged at warn (status, no body) before answering 404.
- [x] (2026-10-05) **KI-150 Local-only setups get no index** (medium): LiteLLM answers an embedding request without a key with a 500 AuthenticationError, which `_embedding_model_unusable` (`workers/codeforge/retrieval.py`) treats as transient, so the index ends in `error` instead of BM25-only; `/search` shows "No results found." without a reason. Fix: skip embeddings for unkeyed providers (`litellm.keyed_providers`) and show the index state on the search page. Found in live session 2. **Fixed (S9-V):** an embedding answer naming an authentication or API key problem (any status but 429) makes the model unusable, so the index is BM25-only; the embedding model is always called (the worker cannot see LiteLLM's routing, for example an `api_base` alias of a local server); without a key LiteLLM's authentication error makes the index BM25-only, logged once with the answer as reason; `POST /search` returns `indexes` (building, failed, BM25-only) and `/search` names them per project.
- [x] (2026-10-05) **KI-151 `propose_roadmap` has no allow rule in any preset** (medium): unlike `propose_goal`, so it waits for an approval and is denied by the 60 s timeout. Fix: allow it like `propose_goal`. Found in live session 2. **Fixed (S9-0):** `propose_roadmap` is allowed in the three presets that allow `propose_goal`; a test checks every preset decides both tools the same way.
- [x] (2026-10-05) **KI-152 The auto-agent marks a feature done whenever its run ends** (medium): post-verification runs only when the description contains "Tests: test_x.py", which the planner never writes, and the project's `test_command`/`lint_command` are unused. Fix: run the test and lint commands plus a workspace-change check after each feature and feed failures back. Found in benchmark run 1 (2026-10-04). **Fixed (S9-V):** after each feature run the auto-agent checks that the workspace changed (a `workspacefs` digest without `.git` and caches) and runs the test command (`test_command` > the detected stack's default > `runtime.default_test_command`) and, when configured, the lint command (`lint_command` > `runtime.default_lint_command`) in the worker (`conversation.test.request`, quality gate executor); failures with bounded output go back to the agent up to `agent.auto_agent_fix_attempts` (default 2), then the feature is cancelled with the reason. The result is stored on the feature (`features.result`, migration 125) and shown on the feature card; a check that could not run leaves it "not fully verified" (it does not fail the feature, like quality gates). Open: a feature resumed after an interruption takes its change-check baseline at the restart. **Review fixes (S9-V):** a detected test command is used only when its runner is set up (`project.DetectGateCommands`; pytest counts as set up only with a config file); pytest exit 5 (no tests collected) is no verdict; the description's test file fails the feature only on a verdict or when missing; in a git repository the change check digests HEAD and the `git status` change set (`internal/git`; ignored files do not count), otherwise a bounded walk (`workspacefs.WalkDirBounded`, batches of 256) without caches and build output; verification commands the conversation's policy profile does not allow without a person (deny, or ask outside accept-edits/delegate) are skipped with a note.
- [x] (2026-10-05) **KI-153 Implementation turns are offered the planning tools** (medium): `propose_goal` and `propose_roadmap` come from the tool router's `BASE_TOOLS`; in benchmark run 1 the model called them instead of writing code and the turn ended. Fix: offer planning tools only in planning turns; nudge "continue" when a model announces an action without a tool call. Found in benchmark run 1. **Fixed (S9-V):** `conversation.run.start` carries `implementation_turn` (the auto-agent's feature and fix turns; `runs.start` always is one); such turns get no `propose_goal`/`propose_roadmap` (ToolRouter `PLANNING_TOOLS` only with `planning=True`), and a reply that announces an action without a tool call gets one "continue: call the tool now" nudge per turn.
- [ ] **KI-154 Context window mismatch for local models** (low, latent): LiteLLM reports the model card's 262,144 tokens, the worker assumes 32,000 and Ollama runs with `num_ctx` 16,384 (it silently drops old messages). Fix: take the server's context or document `OLLAMA_CONTEXT_LENGTH`. Found in live session 2.
- [ ] **KI-155 The admin password is reset on every start** (low): `syncAdminPassword` (`internal/service/auth.go:351`) sets the env admin password at each start, also in production, silently reverting a password changed in the UI. Fix: set it only when the admin is created (or behind an explicit flag). Found in live session 2.
- [ ] **KI-156 Dead settings** (low): Settings > General "Default LLM Provider", "Default Autonomy Level" and "Auto-Clone" are read by no code. Fix: wire them or remove them. Found in live session 2.
- [x] (2026-10-05) **KI-157 Approving a roadmap card fails silently without a roadmap** (low): a 404 when the project has no roadmap (local projects get none). Fix: create the roadmap on first approval. Found in live session 2. **Fixed (S9-V):** the first milestone of a project without a roadmap creates it (`RoadmapService.EnsureForProject`); the proposal card shows why an approval failed.
- [ ] **KI-158 `CODEFORGE_TOOL_PATH` is ignored with isolation off** (low, dev only): the agent ran `pip install` into the host Python. Found in live session 2.
- [ ] **KI-159 The live chat stays "running" after the run completes** (low, needs a repro). Found in live session 2.
- [ ] **KI-160 No cacheable prompt prefix** (low, performance): every new conversation paid about 2.5 minutes of prefill (9.2k tokens at about 60 tokens/s on CPU). Fix: a stable prompt prefix first, a compact prompt for small models. Found in live session 2.
- [ ] **KI-161 Conversation turns show no live tool cards and persist tool messages only at the end** (medium): the Core broadcasts `agui.tool_call`/`tool_result` only for task runs; for conversation turns it keeps only the trajectory event `agent.tool_called`, so the chat shows tool cards after the turn ends, and a reload during a turn has none to restore. Persisting tool results per call needs a per-call worker message, NATS permissions and a dedup key at completion. Overlaps KI-129. Found in S9-0 (2026-10-05). **Partly fixed (S9-D UI, one review round, 2026-10-06):** the Core broadcasts `agui.tool_call` when it decides a call of the conversation's active turn and `agui.tool_result` when the worker reports it (in the conversation's tenant, loaded under the tenant the worker reports; nothing for another tenant or an unknown conversation); a denied call shows failed with its reason; the agent loop's per-step LLM permission gets no card on either path; diff hunks in the broadcast are capped at 8 KiB with a `truncated` marker; a non-JSON argument preview is shown as text; Claude Code turns forward the hook's `tool_use_id`, a denied call is reported at once and an allowed one when the CLI's `tool_result` arrives (a call without an id gets no card). Still open: persisting tool messages per call (a reload during a turn has none to restore).
- [ ] **KI-162 The frontend does not refresh the access token on a 401** (low): after a role change, a disable or an upgrade (KI-143) open tabs fail until the scheduled refresh (up to about 14 min) or a reload. Fix: refresh once on a 401, then retry the request. Found in S9-A (2026-10-05).
- [ ] **KI-163 `.vscode/launch.json` has a fixed dev internal key and admin password** (low, dev only): consistent between Core and worker, but fixed values in the repository. Use the dev container's generated key. Found in S9-A (2026-10-05).
- [ ] **KI-164 The Plandex backend does not work** (low): its executor passes `tell --yes` and `--model`, which plandex 2.2.1 does not have, and Plandex needs a server and an interactive sign-in; upstream inactive since 2025-10. Fix or drop the backend. Found in S9-B (2026-10-05).
- [ ] **KI-165 Goose is pinned to 1.29.0** (low): the last versioned upstream image; a newer version needs the release archive and its checksum. Found in S9-B.
- [x] (2026-10-06) **KI-166 GitHub Enterprise on a private network is refused** (low): PR delivery and github-api `ListRepos` reach public addresses only, and the github-issues provider supports github.com only (as `gh` did by default). Found in S9-B. **Fixed (S9-B review):** `pm.allowed_private_hosts` also opens private hosts for the github-api provider and github-issues integrations with a `base_url` (own token only); a GitHub Enterprise issues webhook syncs with its own server and token. Still open: PM imports use api.github.com and the operator token.
- [x] (2026-10-06) **KI-167 multidict advisory GHSA-54p9-h82j-f925** (medium): pip-audit in the Security Scanning job flagged multidict 6.7.1 (a transitive dependency of aiohttp and yarl) after the advisory was published; no dependency file had changed. **Fixed:** `poetry.lock` pins multidict 6.9.1; pip-audit is clean and the Python suite passes.
- [x] (2026-10-06) **KI-168 seroval advisories GHSA-p6vx-979v-rg4c, GHSA-jp82-f5mq-hwhp** (medium): `npm audit --omit=dev` in the Security Scanning job flagged seroval 1.5.6 (critical, fixed in 1.6.3), which solid-js 1.9.x pins to `~1.5.4` (no solid-js 1.9 release depends on a fixed seroval). The SPA bundle does not contain seroval (only Solid's SSR code uses it), so the frontend was not exposed. **Fixed:** `frontend/package.json` `overrides` pins seroval and seroval-plugins to `~1.6.8`; npm audit is clean, typecheck, the 754 vitest tests and the build pass. Remove the override once solid-js depends on a fixed seroval.
- [ ] **KI-169 The conversation cost budget is always 0** (medium): the budget check (`internal/service/conversation_agent_prompt.go:114-130`) sums `run.toolcall.result` events, which are never written for conversations, so a conversation's cost limit never triggers. Found while planning the agent-work read model (2026-10-06).
- [ ] **KI-170 Backend task and benchmark costs are outside the cost views** (low): `tasks.cost_usd` and benchmark runs are not part of the agent-work read model, so the cost page does not show them. Found while planning the agent-work read model (2026-10-06).
- [x] (2026-10-06) **KI-171 Viewers can start agent runs and change tenant data** (high): Many mutating routes have no `RequireRole`, and no service checks a role, while new users are viewers by default. A viewer can send an agentic chat message with a mode of its choice (prototyper maps to `trusted-mount-autonomous`, Bash allowed) but cannot stop the run, start goal discovery, decompose or plan-feature with `auto_start`, and write microagents, skills, memories, shared context, routing outcomes, VCS accounts, scope membership, A2A push configs and other members' channel settings, or start LSP servers. Fix: admin/editor by default for every non-GET route with an explicit allowlist, a route-table test, and no chat input for viewers. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-2, R2-1, R3-1, R3-9, R10a-1, R10b-6, R4-10); fix round S10-A. **Fixed (S10-A, one review round):** every non-GET route under `/api/v1` needs the editor or admin role (`editorOrAdmin`) unless it is on the `viewerRoutes` allowlist in `routes.go` (public auth and webhook routes, the caller's own session, account, keys and channel settings, and read-only queries with a body: search, graph search, memory recall, policy evaluate, prompt-section preview); `routes_roles_test.go` walks the router and fails for any mutating route without a role middleware or allowlist entry and for a stale entry, and sends a viewer request to each (403). Viewers get 403 on conversation messages, goal discovery, decompose and plan-feature; `PUT /channels/{id}/members/{uid}` only for the caller's own uid unless admin; the sub-agent search (`/projects/{id}/search/agent`, up to 20 LLM calls on a chosen model) is editor-only. The frontend still shows viewers the chat input and the guarded actions ([KI-229](#known-issues), S10-J).
- [x] (2026-10-06) **KI-172 The audit log is not wired in production** (high): `cmd/codeforge/main.go` mounts the routes without `WithAuditStore`, so no `audit(...)` middleware writes anything, `GET /audit-logs` is 404, the fail-closed audit of MCP assignment and webhooks never blocks, and the GDPR export's audit trail is empty. The login, setup, refresh and password-reset audit entries could never fire because those requests have no user. Fix: wire the store with a wiring test and record auth events in the handlers. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-1); fix round S10-A. **Fixed (S10-A):** `cmd/codeforge/routes.go` (`apiRouteOptions`) wires `WithAuditStore` in production, with a test that walks the router (`GET /audit-logs` mounted, delete routes audited); the auth handlers record `setup`, `login` (`result: success|failure` with the attempted email, never the password), `refresh`, `forgot_password` and `reset_password` through `RecordAuditAs`, a user-less failure under the reserved anonymous actor ID (distinct from the auth-disabled operator ID). On the public auth routes the entry's tenant is the request's tenant (`X-Tenant-ID` fallback), bounded by the auth rate limiter: a conscious choice.
- [x] (2026-10-06) **KI-173 Editors can delete projects through the batch route** (medium): `POST /projects/batch/delete` allows editors, writes no audit entry and accepts duplicate IDs, while `DELETE /projects/{id}` is admin-only and audited. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-3, R3-6, R10a-2); fix round S10-A. **Fixed (S10-A):** `POST /projects/batch/delete` is admin-only, deduplicates IDs (first-seen order, one result per ID) and writes one audit entry per project before its delete; a project whose entry cannot be written is kept and reported (`audit log unavailable`, fail closed, like MCP assignment and webhooks), while `DELETE /projects/{id}` keeps the audit middleware's behaviour.
- [x] (2026-10-06) **KI-174 Tenant admins can list and change every tenant; disabling a tenant has no effect** (medium): `/tenants` GET, GET `/{id}` and PUT `/{id}` need only the admin role and the store queries are unscoped; `TenantService.ValidateExists` has no caller, so a disabled tenant keeps working. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-4, R3-5); fix round S10-A. **Fixed (S10-A, one review round):** `GET /tenants` and `POST /tenants` are platform-admin only; `GET/PUT /tenants/{id}` need the caller's own tenant or a platform admin (`middleware.RequireOwnTenant`), `TenantService.Get/Update` are scoped the same way and the tenant store queries are marked `INTENTIONALLY CROSS-TENANT`; `middleware.EnabledTenant` (after the access log and recoverer) refuses every authenticated request of a disabled or deleted tenant (403, verdicts cached 30 s per tenant, store errors answer 503 and are not cached), the A2A endpoint checks the same, and login and refresh refuse a disabled tenant's users; the default tenant cannot be disabled.
- [x] (2026-10-06) **KI-175 API key scopes are never enforced** (medium): Scopes are validated and stored, but `RequireScope` has no caller, so a key created with `projects:read` has its user's full rights (`docs/project-status.md` claims scopes). Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-8, R3-3); fix round S10-A. **Fixed (S10-A, one review round):** API-key scopes are enforced per route group (`internal/adapter/http/scopes.go`: a documented table mapping the matched chi route to `projects`, `runs`, `agents` or `admin:all`, GET/HEAD and read-only POST queries need `:read`, everything else `:write`; `admin:all` satisfies all); a key without scopes (nil or empty, also keys created before this round with `[]`) keeps its user's full rights; unknown scope names are rejected on creation (400). The check classifies the route chi matched, not the decoded path.
- [x] (2026-10-06) **KI-176 HTTP leftovers: cross-tenant user lookups, A2A push tokens, body limits, 500s** (low): The admin GDPR export reads another tenant's user and user deletes end a foreign user's sessions before answering 404; `GET /a2a/tasks/{id}/push-config` returns tokens to every role; several handlers decode bodies without `MaxBytesReader`; some validation and not-found errors still answer 500 (KI-149 residue). Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-14, R3-8, R1-15, R10a-22, R1-16, R1-21); fix round S10-A. **Fixed (S10-A, one review round):** the admin GDPR export and user deletes act only within the caller's tenant (404 for another tenant's user, no side effect; sessions end only after a successful erasure); `GET /a2a/tasks/{id}/push-config` returns the token to admins only and `has_token` to everyone (the A2A page renders from it); every body-decoding handler uses the shared body limit (413; an empty optional body is accepted, a malformed one answers 400); settings and routing validation errors answer 400 and unknown projects on goals detect/ai-discover 404.
- [ ] **KI-177 Login and password changes write back stale user rows** (medium): Lockout bookkeeping, `ChangePassword` and the reset paths save the whole user row read before bcrypt, so a concurrent admin disable, demotion or forced reset is undone (role, enabled and the old password hash come back), and parallel failed logins lose the lockout count. Fix: targeted atomic updates. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R3-2); fix round S10-B.
- [ ] **KI-178 Session handling: logout fails open, parallel refreshes log users out** (medium): Logout needs a valid access token, so after a 401 the refresh token and cookie stay valid; concurrent `/auth/refresh` calls from several tabs rotate the same token and the losers clear the cookie; notifications, active runs and the offline queue survive logout; network retries re-send POSTs. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R10a-3, R10a-4, R10a-15, R10a-16); fix round S10-B.
- [ ] **KI-179 Auth leftovers** (low): Password reset tokens are never delivered (no mailer) while the login page links to it; unknown and locked accounts answer faster (account oracle) and change-password has no failure limit; expired reset and refresh tokens are purged only in the default tenant or never; emails are case-sensitive; an admin can demote, disable or delete the last admin (extends KI-145); the idempotency middleware has no in-flight reservation, caches errors and secrets and is not bound to the request. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-13, R3-11, R3-15, R3-12, R5-18, R3-16, R10b-4, R1-11); fix round S10-B.
- [ ] **KI-180 Project credentials in the project config are readable by every user** (medium): The SVN password and the github-api, GitLab and Gitea tokens live in plaintext in `project.config`, which `GET /projects` and the MCP server return unchanged to every role. Fix: redact credential keys on read with the keep-on-`***` rule, or store them encrypted outside the config. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R3-4, R6-3); fix round S10-C.
- [ ] **KI-181 `GET /projects/remote-branches` runs `git ls-remote` against any host** (medium): Any authenticated user can make the Core connect to internal http, git or ssh hosts (SSRF, port-scan oracle); no outbound policy applies. KI-104 understates this path. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-17, R3-10, R6-5); fix round S10-C.
- [ ] **KI-182 The built-in MCP server is unauthenticated by default** (medium): `mcp.enabled` serves the default tenant's projects, runs and costs on `:3001` without a key unless `mcp.api_key` is set in YAML; there is no env or `_FILE` setting, the comparison is not constant-time, and the project cost resource returns the global summary. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-10, R6-4); fix round S10-C.
- [ ] **KI-183 Agent memories take the tenant from the request body and recall has no tenant filter** (medium): Go publishes the body's `tenant_id`, the worker defaults it to the default tenant and recall filters by project only, so a user can read and write other tenants' memories and normal stores of other tenants land in the default tenant. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R4-5, R8-4, R9-3); fix round S10-C.
- [ ] **KI-184 Routing state mixes tenants and is poisoned by personal keys** (medium): The worker reports outcomes and loads stats with the internal key (default tenant); the stats cache key has no tenant; a personal key's 401/429 blocks a model for 24 h for everyone; context-window and deadline errors count as billing errors and block a provider for an hour. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-5, R3-7, R9-4, R9-5); fix round S10-C.
- [ ] **KI-185 Custom modes and pipeline templates are global and in memory** (medium): Any tenant's editor can overwrite another tenant's custom mode (prompt prefix, tools, autonomy) and create silently upserts; a restart drops every mode and template created in the UI. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-9, R2-6, R10b-1); fix round S10-C.
- [ ] **KI-186 LSP routes have no tenant check and start/stop races (when `lsp.enabled`)** (medium): Stop, status, diagnostics, hover and symbols use the URL project ID without a tenant check; `StopServers` during `StartServers` panics on a nil map and orphans the server (extends KI-83). Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R1-12, R4-7); fix round S10-C.
- [x] (2026-10-06) **KI-187 A FIFO in the workspace hangs Go Core git calls** (high): `mkfifo .gitignore` (or `.gitattributes`, `info/exclude`, `core.excludesFile`) makes every Core git call on that workspace block forever while holding one of the 5 git pool slots; git commands have no deadline. Agent-writable config keys (`checkout.workers`, `pack.*`, `index.*`) also size Core processes (fork bomb in the Core container). Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R6-1, R6-6); fix round S10-D. **Fixed (S10-D):** every Go Core git process has a deadline (`git.command_timeout` 2m, `git.network_timeout` 10m for network commands) and its own process group, killed as a whole (`ErrGitTimeout`); `OpenRepo` refuses a FIFO, socket or device where git opens a file by name (root and nested ignore/attributes files, `.git/info`, refs, logs, `objects/info`, `objects/pack`, `core.excludesFile`), pre-scans the working tree for such `.gitignore` files (200k entries) and bounds its checks to 20 s; the objects walk is bounded; sizing keys (`checkout.workers`, `index.threads`, `pack.*`, `core.packedGit*`, `core.deltaBaseCacheSize`, `core.bigFileThreshold`) are overridden on the command line; one tenant holds at most `git.max_concurrent - 1` pool slots; `Repo.Command` is gone from the Core API. Residuals: [KI-227](#known-issues).
- [x] (2026-10-06) **KI-188 Delivery pushes where the agent's git config says, and push failures count as delivered** (medium): `git push -u origin <branch>` follows agent-set `remote.origin.push`/`pushurl` (force-push to `main`, or to another repository with the operator's key); a failed push or PR is reported as a completed delivery. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R6-2, R2-5); fix round S10-D. **Fixed (S10-D):** `Repo.PushBranch` pushes `refs/heads/<b>:refs/heads/<b>` without force to the project's `repo_url` (branch name checked with `check-ref-format --branch`), from a private bare repository in the Core's temp directory with alternates to the workspace objects, `--no-follow-tags` and push options cleared, so the workspace's remote and push config is never read; a remote named like the URL is refused; `push.pushoption` and `transfer.bundleuri` in the workspace config refuse network operations. A failed push is a failed delivery (`run.delivery.failed`), a pushed branch without its pull request a partial one (new status `partial`, event `run.delivery.partial`, shown as a warning in Activity and the dashboard timeline). Residuals: [KI-227](#known-issues).
- [x] (2026-10-06) **KI-189 Git and VCS leftovers** (low): The checkout branch reaches git without `--end-of-options`; the re-clone path deletes workspaces in the Core; github-api token clones are not wired although documented; SVN `svn://`, `svn+ssh://` and `http://` URLs are refused by project validation; the SVN `allow_file_urls` gate is project config that editors set. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R6-13, R6-15, R6-11, R6-12, R3-14); fix round S10-D. **Fixed (S10-D), except the github-api token clone:** checkout takes only branch names (`check-ref-format --branch`, then `git switch --end-of-options`; HTTP 400 otherwise); `project.ValidateRepoURL` checks the URL per provider (SVN: http, https, svn, svn+ssh); `file://` SVN repositories are an operator setting (`svn.allow_file_urls`, the project key is ignored); a re-clone discards a workspace only when it is no checkout of the URL (git: `ErrNotRepository` or another origin; SVN: no `.svn` or another URL) and, with `workspace.tool_acls: required`, removes it through the worker (`workspace.delete.request`); the SVN provider checks every host it contacts with the outbound policy (`svn.allowed_private_hosts`). Still open: github-api projects do not clone with the project token ([KI-227](#known-issues)).
- [x] (2026-10-06) **KI-190 Prompt reminders are rendered into every turn unconditionally** (high): None of the 7 reminders has conditions, so every turn tells the model twice that it is in PLAN mode, stuck, over budget, near the context limit and drifting, partly with raw `{{.BudgetPercent}}` templates. Likely a cause of KI-153 and part of KI-160. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R4-1); fix round S10-E. **Fixed (S10-E1, reviewed):** reminders have runtime conditions (four without a data source in Go were removed), are excluded from `Assemble`, and template errors skip the entry; `countStallIterations` uses canonical tool names.
- [x] (2026-10-06) **KI-191 The stall detector aborts normal edit/test loops** (high): Any identical call 3 times among the last 5 counts as a stall, and the escape prompt never clears the window, so edit, pytest, edit, pytest aborts at iteration 9 with `stall detected: repeated None`. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R8-1); fix round S10-E. **Fixed (S10-E1, reviewed):** consecutive repeats and repeated pairs count as a stall; an escape clears the window.
- [x] (2026-10-06) **KI-192 Tool selection and routing use the wrong input and skip the policy** (medium): The tool router never offers MCP, skill or conversation-search tools; routing, tool selection, skills and the docs prefetch use the conversation's first user message; the docs prefetch calls an MCP tool without a policy decision; router and skill selection send the prompt to models the user did not choose, uncosted. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R8-2, R8-9, R8-10, R9-6); fix round S10-E. **Fixed (S10-E1, reviewed):** see features/04 (tool selection). Open: the docs prefetch still asks for approval under supervised presets (one call per turn).
- [x] (2026-10-06) **KI-193 An agentic conversation without a workspace runs file tools in the worker's own directory** (medium): `IsAgentic` returns `req.Agentic` before its workspace check and the worker normalises an empty workspace to its CWD, so `read_file` can read the worker's files, `.env` included. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R8-3); fix round S10-E. **Fixed (S10-E1):** Go refuses agentic turns without a workspace (400); the worker refuses runs and conversation turns whose workspace is not a directory.
- [x] (2026-10-06) **KI-194 Tool processes survive Stop, timeout and abort** (medium): Bash commands are not killed on cancellation; a string or null `timeout` leaves the process unmanaged; benchmark test commands leak on timeout; the CLI backend timeout is per output line. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R8-6, R8-7, R9-8, R8-18); fix round S10-E. **Fixed (S10-E2):** bash, `search_files`' grep, benchmark functional tests and agent-runner test commands run in a process group of their own (`subprocess_utils.communicate_in_group` / `run_tool_shell`) that a timeout, a Stop (the tool executor polls the run's cancel while a tool runs and reports "cancelled: the run was stopped while the tool ran"), the run's wall clock or a worker abort kills as a whole; a group is signalled only while the command has not been reaped (its PID could name another group); a command that finished while a background job holds its pipes returns its exit code and output at the timeout and the job keeps running; the bash `timeout` argument is coerced to whole seconds and clamped to 1..3600 (null, words, bools, inf, NaN: the default 120); the CLI backends' timeout is one deadline for the whole task. Residuals: [KI-228](#known-issues).
- [x] (2026-10-06) **KI-195 Multi-rollout conversations stash user changes and keep the last rollout** (medium): Rollout 0 stashes the user's uncommitted work without popping it, and the workspace ends with the last rollout while the best one is reported (opt-in setting). Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R8-5); fix round S10-E. **Fixed (S10-E1, reviewed):** clean-workspace rollouts with git-tree snapshots, start-branch restore, a stop on outside changes, bounded git calls, and a Go guard against concurrent work on the project. Open: the worker cannot tell a rollout's writes from concurrent writes during that rollout.
- [x] (2026-10-06) **KI-196 Streamed LLM calls cost $0 and mid-stream errors complete the turn** (high): LiteLLM's stream headers carry no cost and the fallback price table is not in the image, so every streamed turn costs 0 and budgets never stop a run; an SSE error frame after the first chunk becomes a normal partial answer; a retried stream repeats text. Model resolution also makes blocking HTTP calls on the event loop. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R9-1, R9-2, R9-12, R9-7); fix round S10-E. **Fixed (S10-E2):** a stream without a cost of its own (LiteLLM sets `x-litellm-response-cost` before a chat-completions stream starts) is priced from the per-token prices of `/model/info` for the model the chunks name (else the requested model), cached prompt tokens at `cache_read_input_token_cost`, attempts that failed after their usage chunk added; a `usage.cost` or the header still wins; the fallback price table ships in the worker image (`/app/configs/model_pricing.yaml`); a non-local model that still costs $0 is logged once; a failed `/model/info` request is not repeated for 60 s. An SSE error frame raises `LLMError` with its status (retry and fallback run instead of a truncated answer); a stream whose text the caller already received is not retried. `resolve_model_async()` reads the model cache on the event loop and refreshes in a thread only when due; the benchmark's routing runs in a thread. Residuals: [KI-228](#known-issues).
- [ ] **KI-197 The A2A protocol path is broken in several places** (high): SDK task saves violate `NOT NULL` (history/artifacts) so tasks cannot be read or cancelled; the worker ignores cancel and Go overwrites terminal states; the client cache ignores lookup errors (cross-tenant use, nil dereference that crashes the Core from a NATS handler); results carry no output; `a2a.task.created` has no DLQ subscriber; discovery and sending bypass the outbound policy (KI-104 is wrong there); remote agent URLs are globally unique. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R5-1, R7-2, R4-9, R4-4, R8-11, R6-10, R4-6, R5-8); fix round S10-F.
- [ ] **KI-198 Cursor pagination is wrong for trajectories, audit and channels** (medium): Cursors compare random UUIDs or only timestamps while rows are ordered by another column, so pages skip and repeat entries; agent event `version` is constant, so replays and exports have no real order; channels and threads show only the newest 50 messages. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R5-2, R5-3, R10a-7, R5-20, R10a-13); fix round S10-F.
- [ ] **KI-199 Inserts accept another tenant's project ID** (medium): Tasks, agents, conversations, roadmaps, boundaries and branch rules are inserted for any project ID; a foreign roadmap blocks the owner's (409) and a boundary upsert silently writes nothing; some `DO UPDATE` clauses lack the tenant predicate. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R5-4); fix round S10-F.
- [ ] **KI-200 Deleted projects and erased users leave data behind** (medium): Agent events, audit trail, graph, skills, microagents, quarantine, A2A tasks, routing outcomes, feedback and prompt scores stay after a project is deleted; `feedback_audit.responder` keeps an erased user's email; erasure is not atomic. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R5-6, R5-5, R5-10); fix round S10-F.
- [ ] **KI-201 System jobs see only the default tenant; review policies never run** (medium): The review cron, benchmark watchdog, suite seeding and token purges run tenant-scoped queries without a tenant; review policies fail on plan foreign keys and the cron adds a pending review every minute; review store writes ignore `RowsAffected`. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R5-7, R4-11, R2-4, R5-15); fix round S10-F.
- [ ] **KI-202 Store leftovers** (low): List queries silently stop at 100 rows (password reset of user 101+ fails); consent purposes are never created; `ClaimTask` sets queued without a dispatch; benchmark updates have no status guard; `CreateToolMessages` swallows an error; MCP and push credentials are stored in plaintext; migration 124 takes a long exclusive lock; a nil filter panics in `ListA2ATasks`. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R5-11, R5-12, R5-13, R5-14, R5-17, R5-9, R5-16, R5-19); fix round S10-F.
- [x] (2026-10-06) **KI-203 Roadmap sync destroys spec files and import loses status** (high): "Sync to file" re-renders the whole file (paragraphs, tables, code blocks and long lines are lost, `[x]` becomes `[ ]`, the fallback overwrites ROADMAP/TODO files); import sets every heading and item to `backlog` and matches by line number; PM import duplicates on every run. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R4-2, R4-3, R4-12); fix round S10-G. **Fixed (S10-G, reviewed):** sync patches only checkbox markers in place (no re-render, no truncate, conflicts give 409), import takes the checkbox status with a three-way merge against the last-seen state (migration 128), matches by line or title, PM import upserts by external ID. Open: legacy features from earlier imports are not cleaned up.
- [x] (2026-10-06) **KI-204 An unknown trust minimum fails open** (medium): `MeetsMinimum` treats an unknown minimum as rank -1, so a typo in `quarantine.min_trust_bypass` disables quarantine and a misspelled `trust_minimum` in a policy rule accepts every level. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R7-1); fix round S10-G. **Fixed (S10-G):** `trust.IsValidLevel`; an unknown `quarantine.min_trust_bypass` or rule `trust_minimum` is rejected at load, and `MeetsMinimum` fails closed.
- [x] (2026-10-06) **KI-205 Branch protection rules are never evaluated** (medium): `CheckBranch`/`CheckMerge` have no production caller, so stored rules protect nothing (commit-local delivery can move `main`). Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R7-3, R2-10); fix round S10-G. **Fixed (S10-G, reviewed):** rules are evaluated before delivery creates, pushes or moves a branch; a rule protects only the branches it matches; `**` patterns. Open: agent-run git commands and branch deletion are not checked.
- [ ] **KI-206 Auto-agent Stop leaves the status at "stopping" for good** (high): All writes after `cancel()` use the cancelled context, so the stored status stays `stopping`, the UI button stays disabled and the interrupted feature is reported failed. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R2-2); fix round S10-H.
- [ ] **KI-207 Phantom running turns and orphaned conversation runs** (medium): A dispatch cancelled by the HTTP request (reload while context is built) or a dead-lettered start leaves the stored turn active forever with no heartbeat (likely the cause of KI-159); the turn is ended before its completion is claimed; deleting a conversation does not cancel its worker run; the frontend never resyncs active runs after a reconnect. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R2-3, R2-14, R2-15, R10a-14); fix round S10-H.
- [ ] **KI-208 Orchestrator and run bookkeeping defects** (medium): A plan step start runs retrieval (up to minutes) under the global orchestrator lock; debate mappings are in memory only; a completion during a stop never confirms the stop; a task's cost is overwritten by each run; a quarantine-blocked `StartRun` leaves the run running. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R2-7, R2-13, R2-12, R2-11, R2-16); fix round S10-H.
- [ ] **KI-209 `/compact` and goal proposals do not work as documented** (medium): The compaction summary is only logged and the worker's history fetch has no internal key (401); a rejected goal proposal stays enabled and decisions sent during a run get 409. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R2-8, R8-13, R2-9, R10b-5); fix round S10-H.
- [x] (2026-10-06) **KI-210 The production WAL archive grows without bound** (high): `archive_mode=on` with `archive_timeout=300` archives a 16 MiB segment every 5 minutes (about 4.5 GiB a day, the model registry writes every 60 s); nothing prunes it, the cleanup script's `find` is wrong, no base backup exists, and the DR runbook names the wrong container and paths. The disk fills within weeks and PostgreSQL stops. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R11-1); fix round S10-I. **Fixed (S10-I, reviewed):** archiving is off unless `POSTGRES_ARCHIVE_MODE=on`; the cleanup script's `find` is fixed; model capability syncs write only on change; the DR runbook names the compose service and `/archive`.
- [x] (2026-10-06) **KI-211 Production shares one login rate-limit bucket across all clients** (medium): Both shipped topologies put nginx or Traefik in front of the Core without `CODEFORGE_TRUSTED_PROXIES` (and the network has no fixed subnet), so every client is keyed by the proxy IP: one client can keep login at 429 for everyone, and audit records store the proxy IP (follow-up to KI-11). Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R11-2, R1-7); fix round S10-I. **Fixed (S10-I, reviewed):** fixed `public` subnet `10.250.240.0/24` (outside Docker's pools) with the trusted-proxy range following it, overridable; upgrade note in dev-setup.
- [x] (2026-10-06) **KI-212 `restore-postgres.sh latest` can drop the database and restore a random file** (medium): Without plain `*.sql.gz` backups (always the case with encryption) `xargs ls -t` lists the current directory; the script then drops the database and `pg_restore` fails. Encrypted backups are neither pruned nor restorable. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R11-3); fix round S10-I. **Fixed (S10-I, reviewed):** `xargs -r`, `*.sql.gz*`, `.gpg` decrypt, `pg_restore --file=/dev/null` before any drop, encrypted backups pruned.
- [x] (2026-10-06) **KI-213 Go Core network and resilience defaults** (medium): The Core listens on all interfaces in dev and live E2E (public admin credentials, isolation off) and prod publishes 8080 over HTTP; the 30 s route timeout kills clones, setups and SSE; the NATS connection gives up after about 2 minutes; LiteLLM completions use a 10 s timeout; circuit breakers trip on 4xx; a listen failure is only logged; a missing explicit config file and unparsable env values are ignored; `auth.enabled=false` is accepted in production; Secure cookies behind an outer TLS proxy; health checks call LiteLLM's live `/health`. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R11-4, R11-10, R1-6, R6-8, R6-7, R6-9, R1-18, R1-19, R1-20, R1-22); fix round S10-I. **Fixed (S10-I, reviewed):** `server.host`, loopback-bound prod port with nginx proxying A2A, git operation deadline outside the route timeout, unlimited NATS reconnects, LiteLLM completion timeout, breakers neutral for caller deadlines and 4xx, listen failure exits, fail-fast config, `server.force_secure_cookies`, `/health/readiness`; concurrent clones of one project serialized.
- [ ] **KI-214 Build, CI and deployment hygiene** (medium): `.dockerignore` patterns apply only at the root (host `node_modules` and `.env` files reach the images); the real-SQL tenant tests and the Claude Code policy test never run in CI; all services use the PostgreSQL superuser; Traefik holds the Docker socket; provider keys are visible in `docker inspect`; two actions are not SHA-pinned; image builds share one cache scope and have no concurrency group; the feature-verification and goimports checks can never fail; core shutdown is killed after 10 s; tool versions drift; `ipc: host` for playwright-mcp; `run-agent-eval.sh` sends invalid JSON; dead alert rules; the README's setup-token command named a service that does not exist (fixed 2026-10-06). Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R11-11, R11-5, R11-6, R11-7, R11-8, R11-9, R11-12, R11-13, R11-14, R11-15, R11-16, R11-17, R11-18, R11-19, R11-20); fix round S10-I. **Partly fixed (S10-I, reviewed):** `.dockerignore` patterns, real-SQL worker tests in CI, gitleaks pinned, failing verification and goimports hooks, stop_grace_period, no `ipc: host`, run-agent-eval JSON, alert rules, version pins, Traefik without Docker access (file provider). Open: PostgreSQL roles per service, provider keys visible in `docker inspect`, the Claude Code policy test in CI. **Also fixed (2026-10-06):** the three image builds use their own GHA cache scope (`scope=core|worker|frontend`) and the workflow has a concurrency group per ref (in order, no cancellation), so a later push never leaves `:main` or `:staging` at an older commit. `actions/attest-build-provenance` is pinned to the v2.4.0 commit (2026-10-06).
- [ ] **KI-215 One failed request replaces the whole app with the error screen** (medium): Errored resources are read without a guard (channel list on every page, dashboard, charts, cost page, channel view, model combobox) and only the app root has an error boundary; chat slash commands use raw `fetch()` and report success on 403/500. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R10a-5, R10a-6); fix round S10-J.
- [ ] **KI-216 Chat UI state defects** (medium): Switching conversations keeps the previous run's tool calls, thinking state and Stop button; rewind always fails and the timeline shows other plans' steps; a failed approval shows Denied and still saves the Allow-Always rule; errors are swallowed (a canvas send loses the text); text starting with `/` is lost; live output grows without bound; chat images cannot be opened. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R10b-9, R10b-7, R10b-16, R10b-15, R10a-21, R10b-10, R10b-20); fix round S10-J.
- [ ] **KI-217 File panel defects** (medium): Chat attachments overwrite workspace files without asking and corrupt binaries; edits typed during a save are marked as saved; the file tree never refreshes after create, delete, rename or upload. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R10b-11, R10b-12, R10b-13); fix round S10-J.
- [ ] **KI-218 Settings and admin page defects** (medium): Editing an MCP server without a description crashes Save and Test and hides refusal reasons; quarantine payloads are shown as base64; the VCS account provider dropdown has a random default and invalid options. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R10b-2, R10b-21, R10b-3, R10b-14); fix round S10-J.
- [ ] **KI-219 Frontend leftovers** (low): The Activity page misreads gate and entity payloads; KPIs show `$NaN` below 0.5; export links send no credentials (401); channel state carries over; batch results are ignored; the offline banner and API dot misreport; approval links point to the current project and the approval page reports every error as not pending; the GET cache is unbounded; edits cannot clear text fields; search keeps old results; onboarding "Add model" cannot work; device-flow polling overlaps; several pages show actions that answer 403; the dashboard activity timeline listens for event types that are never broadcast (addressed by the agent-work read model). Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R10a-9, R10a-8, R10a-10, R10a-11, R10b-8, R10a-12, R10a-17, R10a-18, R10a-20, R10a-23, R10b-17, R10b-18, R10b-19, R10b-22); fix round S10-J.
- [ ] **KI-220 Evaluation and benchmark defects** (low): Evaluation errors still score 0.0 (and GEMMAS returns 1.0 on failure); `tool_correctness` never sees the tool calls; offered metrics are silently dropped; SPARC rewards crashed runs; agent benchmarks always fail on `publish_trajectory_event`; results are not idempotent; benchmark Cancel does not stop the worker and the routing report is dropped; a worker without `APP_ENV=development` drops requests. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R9-9, R9-10, R9-11, R9-18, R8-12, R10a-19, R8-16); fix round S10-K.
- [ ] **KI-221 Prompt evolution, prompt sections and routing helpers are not wired** (low): Prompt-evolution variants collide on a unique key, use model "auto", are never selected and retry with a fresh message ID; prompt editor sections are never applied; model capabilities need the uninstalled `litellm` package; `claudecode/default` can become a LiteLLM fallback; the context reranker drops entries; the mid-loop model switch switches nothing. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R4-8, R4-13, R9-13, R8-19, R9-14, R9-15, R9-16, R9-17); fix round S10-K.
- [ ] **KI-222 Worker and service leftovers** (low): Tool-call results are unbounded (NATS 1 MiB); a timed-out turn loses its record; `glob_files` ignores the `path` the policy checks; `workspace.>` is missing from the Python stream list; a knowledge-base status race; the prompt score cache is unbounded; GEMMAS scores are only logged; SMTP mails have no timeout; retrieval search blocks the event loop; an approved quarantine message is lost when its replay publish fails; subscription device-flow errors never reach the UI and concurrent connects can lose a key. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R8-14, R8-15, R8-17, R7-4, R4-14, R6-14, R9-19, R3-13, R3-17); fix round S10-K.
- [x] (2026-10-06) **KI-223 The owner-run walk runs out of file descriptors on wide trees** (medium): The tool-isolation walk opens a descriptor for every subdirectory before descending, so with the common 1024 soft limit a `node_modules` with more than about 1000 packages is skipped: the sharing pass misses it and the migration stamps the tree migrated with old ACLs. Found in the full code review of 2026-10-06 ([audit](audits/2026-10-06-code-review/README.md), R8-8); fix round S10-E. **Fixed (S10-E2):** the owner-run walk enters a subdirectory only when it gets to it, holds at most 32 directory descriptors (the deepest levels of its path, reopened from the root one component at a time without following symlinks and checked by inode when it climbs back) and is capped at 128 levels (`workspace_fs.MAX_WALK_DEPTH`); its report counts what it could not list, enter or stat (only `FileNotFoundError` is passed over), `census()` returns that report and `exact()` / `unshare_links` fail on it; a migration whose walks skipped anything is not stamped and runs again with the tenant's next work item. A directory of another owner that the legacy walks cannot list (the Go Core's 0700 `.git/codeforge/patches`) is reported as `foreign_unentered` (cap 1000) and the migration's worker step checks it as the owner: an entry of the retired tool user 10002 there, a path outside the tree or a directory the worker cannot list fails the migration.
- [ ] **KI-224 Webhook API gaps found while building the UI** (low): the list has no last-delivery time (`webhook_deliveries.received_at` exists); the API returns only a path and has no setting for the public base URL; there is no endpoint listing the supported providers (the frontend mirrors `internal/domain/webhook/endpoint.go`); Plane registration is circular (Plane shows its secret only after its webhook has a URL, which CodeForge creates only with that secret). Found in S9-D (2026-10-06).
- [ ] **KI-225 Text tool protocol residuals** (low): with malformed JSON from the model, a nested call is still reachable through a multi-line neighbour (`{"path":"x","content":"a"}b",\n "data":\n {"tool":...}}`) or an outer object with unquoted keys (`{name: ..., parameters: {"tool":...}}`); the Go policy still decides the call. Two rare valid replies are refused (a call followed on the same line by prose containing `"word":`, and a Python final answer with a `{'name': ..., 'input': ...}` dict). Found in the S9-C review (2026-10-06).
- [x] (2026-10-06) **KI-226 The SessionStart hook appends PATH on every start** (low): `.claude/hooks/session-start.sh` writes `export PATH="$GOBIN:$PATH"` to `CLAUDE_ENV_FILE` each time it runs, so a long-lived cloud session ends up with `/root/go/bin` about 60 times in PATH. Found while setting up the MCP servers (2026-10-06). **Fixed:** the hook writes one idempotent line (`case ":$PATH:" in ... esac`) that adds `$GOBIN` only when it is missing.
- [ ] **KI-227 S10-D git and SVN residuals** (low): `FetchFrom` and pull still read the workspace config, so an `insteadOf` written after `OpenRepo` can redirect a fetch; the private push repository reads workspace objects by name, so a FIFO swapped in after `OpenRepo` holds a pool slot until `git.network_timeout`; alternates added between the re-check and git's read are followed; a shallow workspace has no `shallow` file in the push repository, so its branch push likely fails (reported as a failed delivery); the SVN host check runs once before svn resolves the name again (DNS rebinding); `runSVN` has no deadline of its own; github-api projects do not clone with the project token although the provider builds token URLs. Found in the S10-D review (2026-10-06).
- [ ] **KI-228 S10-E2 worker residuals** (low): a call that fails for good records no cost (`LLMError` carries none); a finished command whose background job holds the pipes waits out the full timeout before it returns; after a timeout kill `proc.wait()` can hang when a process that left the group with `setsid` holds the pipes; `agent_loop.py`, `consumer/_conversation_routing.py`, `skills/selector.py` and `skills/safety.py` still resolve models through `asyncio.to_thread` on every call; migrations of trees deeper than 128 levels or with another owner's directory the worker cannot list either (root-owned 0700) fail permanently by design. Found in the S10-E2 review and fix round (2026-10-06).
- [ ] **KI-229 S10-A and S10-H frontend leftovers** (low): the chat input and the actions behind the routes guarded in S10-A still render for viewers and answer 403 (S10-J); the audit view has no filter for `result: failure` login entries. Found in the S10-A review (2026-10-06).
- [x] (2026-10-04) **S7-F review, pre-existing fixes:** a run with a user's own provider key falls back only to models of that provider, so the key is never sent to another one; the retrieval and graph routes (`/projects/{id}/index`, `/graph/status`, `/search`, `/search/agent`, `/graph/search`) and `POST /search` with `project_ids` answer 404 for another tenant's project; retrieval scopes accept and change only their own tenant's projects (create, update, add and remove across tenants answer 404; legacy cross-tenant links are ignored).

---

### Completed Phases (0 through 30+)

> All phases below are complete. For implementation details, see git history.
> For phase summaries, see [project-status.md](project-status.md).

#### Phase 0 -- Project Setup (COMPLETED)
- [x] Market research (20+ tools), architecture decisions, devcontainer, linting, documentation structure

#### Phase 1 -- Foundation (COMPLETED)
- [x] Docker Compose (PostgreSQL, NATS, LiteLLM), Go Core REST API (9 endpoints), Python Workers, SolidJS frontend, CI

#### Phase 2 -- MVP Features (COMPLETED)
- [x] Git local provider (clone, status, pull, branches), agent lifecycle with Aider backend, WebSocket live output, LLM provider management

#### Phase 3 -- Reliability & Performance (COMPLETED)
- [x] Hierarchical config, structured JSON logging, circuit breaker, graceful shutdown, idempotency keys, dead letter queue (Python DLQ path unreachable, see [KI-19](#known-issues))
- [x] Event sourcing, tiered cache adapter (ristretto L1 + NATS KV L2; removed again, KI-60), rate limiting (see [KI-11](#known-issues)), DB pool tuning, worker pools

#### Phase 4 -- Agent Execution Engine (COMPLETED)
- [x] Policy layer (first-match-wins, 4 presets, YAML custom policies), runtime step-by-step protocol (open defects: [KI-4..KI-9](#known-issues))
- [x] Docker sandbox execution, stall detection, quality gates, 5 delivery modes, shadow Git checkpoints (sandbox/hybrid gated until tools run in the container, [KI-13](#known-issues); open defects: [KI-26..KI-29](#known-issues))
- [x] Resource limits, secrets vault with SIGHUP reload, multi-tenancy preparation

#### Phase 5 -- Multi-Agent Orchestration (COMPLETED)
- [x] DAG scheduling (sequential, parallel, ping-pong, consensus), Meta-Agent LLM decomposition
- [x] Agent Teams with role-based composition, Context Optimizer with token budget packing
- [x] Modes System: 24 built-in agent specialization modes

#### Phase 6 -- Code-RAG (COMPLETED)
- [x] tree-sitter Repo Map (16+ languages, PageRank), Hybrid Retrieval (BM25S + semantic, RRF fusion)
- [x] Retrieval Sub-Agent with LLM query expansion, GraphRAG with PostgreSQL adjacency-list graph

#### Phase 7 -- Cost & Token Transparency (COMPLETED)
- [x] Real cost extraction from LiteLLM, fallback pricing table, cost aggregation API (5 endpoints)
- [x] WebSocket budget alerts, frontend cost dashboard with project breakdown and daily bars

#### Phase 8 -- Roadmap Foundation, Trajectory, Docker Production (COMPLETED)
- [x] Roadmap/Feature-Map domain model, spec/PM provider ports, 12 REST endpoints
- [x] Trajectory API with cursor pagination, Docker production images, docker-compose.prod.yml (open defects: [KI-43..KI-46](#known-issues))

#### Phase 9A-9E -- Advanced Integrations (COMPLETED)
- [x] 9A: OpenSpec, Markdown, GitHub Issues adapters, spec/PM import
- [x] 9B: SVN provider, Gitea/Forgejo PM adapter, VCS webhooks (GitHub + GitLab), bidirectional PM sync
- [x] 9C: PM webhook processing (provider mapping, exact project match and status codes fixed in [KI-56](#known-issues)), Slack + Discord notification adapters
- [x] 9D: OpenTelemetry stub, A2A protocol stub, AG-UI event protocol, blue-green deployment (see [KI-47](#known-issues))
- [x] 9E: Plane.so PM adapter (full CRUD), full auto-detection engine, Feature-Map visual editor

#### Phase 10 -- Frontend Foundations (COMPLETED)
- [x] JWT auth (HS256, access + refresh), RBAC middleware, API key management
- [x] Signal-based i18n (480+ keys, EN + DE), CSS design tokens, command palette, toast system
- [x] WCAG 2.2 AA conformance, error boundaries, offline detection

#### Phase 11 -- GUI Enhancements (COMPLETED)
- [x] Tab-based ProjectDetailPage, settings page, mode selection UI, step-progress indicators
- [x] Team management, trajectory replay inspector, diff-review, architecture graph visualization

#### Post-Phase 11 -- Security Hardening (COMPLETED)
- [x] 18 audit findings fixed (5 P0, 8 P1, 5 P2): prompt injection defense, secret redaction, audit trail
- [x] Fail-closed quality gates (a missing check result fails the gate since the KI-29 fix, 2026-09-30), JWT standard claims + revocation, API key scopes, account lockout

#### Phase 12A-12K -- Architecture Evolution (COMPLETED)
- [x] 12A: Mode extensions (DeniedTools, DeniedActions, RequiredArtifact, modular prompt templates; tool restrictions are prompt-only, see [KI-10](#known-issues))
- [x] 12B: LLM routing via LiteLLM tag-based scenario routing (6 scenarios)
- [x] 12C: Role evaluation framework (FakeLLM harness, 9-role matrix, 15 fixtures)
- [x] 12D-12F: RAG shared scopes, artifact-gated pipelines, pipeline templates (3 built-in)
- [x] 12G-12K: Workspace management, per-tool token tracking, periodic reviews, project wizard, knowledge bases

#### OWASP Audit Remediation (COMPLETED)
- [x] Two rounds of OWASP Top 10:2025 + WSTG v4.2 (50+ findings across P0-P3)
- [x] Docker hardening, tenant isolation, request body limits, path traversal prevention, CSP headers

#### Phase 13 -- UI/UX Improvements & Chat Interface (COMPLETED)
- [x] Foundation fixes, CRUD completeness (projects, modes editable), settings + account management
- [x] Spec/roadmap detection fix, chat interface with conversation API and AG-UI integration
- [x] Automatic orchestration, Goose/OpenCode/Plandex/OpenHands agent backends

#### Phase 14 -- UX Simplification (COMPLETED)
- [x] Side-by-side project layout, simplified project creation with branch selection
- [x] Roadmap structured parsing with drag-to-reorder, bidirectional sync (UI -> repo files)
- [x] Chat enhancements (streaming, Markdown rendering, tool call cards)

#### Phase 15 -- Protocol Integrations (MCP + LSP) (COMPLETED)
- [x] MCP client in Python Workers (McpWorkbench with BM25 tool recommendation)
- [x] MCP server in Go Core (mcp-go SDK, 4 tools, 2 resources), server registry with DB persistence
- [x] (2026-03-24) MCP parameterized resource templates: `codeforge://projects/{id}`, `codeforge://projects/{id}/costs`
- [x] LSP code intelligence with per-language server lifecycle, tool routing with policy integration

#### Phase 16 -- Frontend Design System Rework (COMPLETED)
- [x] 25 CSS design tokens, 11 primitives, 8 composites, 4 layout components, full page migration (42 files)

#### Phase 17 -- Interactive Agent Loop (COMPLETED)
- [x] LLM tool-calling support, 7 built-in tools (Read, Write, Edit, Bash, Search, Glob, ListDir)
- [x] AgentLoopExecutor with multi-turn tool-use, ConversationHistoryManager with token budget
- [x] HITL approval via WebSocket, AG-UI streaming events, ChatPanel with tool call display

#### Phase 18 -- Live E2E Testing & Blockers (COMPLETED)
- [x] NATS stream subjects bug fix, system prompt self-correction, model auto-discovery
- [x] Runtime conversation policy fix, live testing with real LLM calls, knowledge base system fixes

#### Phase 19 -- Frontend UX Refinements (COMPLETED)
- [x] Resizable roadmap/chat split, collapsible roadmap panel, chat auto-scroll
- [x] Expanded mode prompts with composable prompt system and editor, MCP Streamable HTTP transport

#### Phase 20 -- Benchmark Mode (COMPLETED)
- [x] DeepEval integration (correctness, faithfulness, relevancy, tool correctness metrics)
- [x] OpenTelemetry tracing, GEMMAS collaboration metrics (IDS, UPR)
- [x] Go Core benchmark API (7 endpoints, migration 041), frontend benchmark dashboard

#### Phase 21 -- Intelligent Agent Orchestration (COMPLETED)
- [x] Confidence-based moderator router with structured output, typed agent module schemas
- [x] SVG-based agent flow DAG visualization, moderator agent mode with debate protocol

#### Phase 22 -- Planned Pattern Implementation (COMPLETED)
- [x] All 8 patterns from CLAUDE.md: RouterLLM wiring, Copilot token exchange, composite memory scoring
- [x] Experience pool (@exp_cache), HandoffMessage, Microagents, Skills system, Human Feedback Protocol (defects fixed in [KI-15, KI-16, KI-57](#known-issues))

#### Phase 23 -- Security & Identity Patterns (COMPLETED)
- [x] 23A: Trust annotations (4 levels), auto-stamped on NATS payloads
- [x] 23B: Message quarantine with risk scoring, admin review hold (A2A prompts and handoffs are screened since [KI-15](#known-issues))
- [x] 23C: Persistent agent identity (fingerprint, stats accumulation, inbox)
- [x] 23D: War Room -- live multi-agent collaboration view with swim lanes

#### Phase 24 -- Active Work Visibility (COMPLETED)
- [x] Parallel task deduplication, atomic claim/release with optimistic locking, stale recovery

#### Phase 25 -- Frontend Form Dropdowns (COMPLETED)
- [x] Dynamic dropdown population for agent, policy, and mode selectors, TagInput component

#### Phase 26 -- Benchmark System Redesign (COMPLETED)
- [x] Provider interface pattern, evaluator plugins (LLMJudge, FunctionalTest, SPARC), 3 runner types
- [x] 8 external providers (HumanEval, MBPP, SWE-bench, etc.), multi-compare with radar chart
- [x] NATS bridge, WebSocket live updates, suites CRUD, 132 E2E tests

#### Phase 27 -- A2A Protocol Integration (COMPLETED)
- [x] Full A2A v0.3.0 via a2a-go SDK -- server (inbound tasks) and client (outbound federation)
- [x] AgentCard builder, auth middleware, task lifecycle, remote agent registry, `a2a://` handoff routing (A2A API keys authenticate the A2A routes since [KI-15](#known-issues))

#### Phase 28 -- R2E-Gym / EntroPO Integration (COMPLETED)
- [x] Hybrid verification pipeline (filter->rank), trajectory verifier (5-dimension LLM scoring; the verifiers use the worker's LiteLLM client since [KI-37](#known-issues) and report unusable verdicts as evaluation errors, never as 0.0)
- [x] Multi-rollout test-time scaling (best-of-N), diversity-aware MAB routing (entropy-UCB1)
- [x] DPO/EntroPO trajectory export (JSONL), SWE-GEN synthetic task generation from Git history
- [x] (2026-03-16) Evaluation improvements: logprob verifier, categorical trajectory scoring, longest/shortest selection strategies

#### Phase 29 -- Hybrid Intelligent Model Routing (COMPLETED)
- [x] Three-layer cascade: ComplexityAnalyzer (<1ms) -> MABModelSelector (UCB1) -> LLMMetaRouter
- [x] Task-type complexity boost, model auto-discovery from LiteLLM, wildcard config
- [x] Adaptive retry with exponential backoff, per-provider rate-limit tracking

#### Phase 30 -- Goal Discovery & Adaptive Retry (COMPLETED)
- [x] Auto-detection of project goals from workspace files, priority-based context injection
- [x] LLMClientConfig with env-var-driven retry/timeout, HybridRouter skips exhausted providers
- [x] Goal system redesign: replaced `manage_goals` HTTP-callback tool with `propose_goal` AG-UI event tool (2026-03-09)
- [x] Rewritten goal_researcher mode with GSD questioning methodology (interview-first)
- [x] GoalProposalCard UI for approve/reject of agent-proposed goals
- [x] Context injection: docs/PROJECT.md, REQUIREMENTS.md, STATE.md passed to goal_researcher agent
- [x] Functional options pattern (`AgenticOption`/`WithContextEntries`) for extensible agentic dispatch

#### Unified LLM Path & Global Run Tracking (COMPLETED)
- [x] Simple chat unified with agentic path through NATS dispatch
- [x] ConversationRunProvider for global run state, sidebar indicator, ChatPanel seamless resume

#### OTEL Tracing Rewrite (COMPLETED)
- [x] AgentNeo replaced with OpenTelemetry backend (OTLP gRPC exporter), 6 instrumented services (export gaps, see [KI-36](#known-issues))

#### QA Audit (COMPLETED)
- [x] ~90 new handler tests across P0-P3 tiers, 33 duplicate test names renamed
- [x] P0: Auth (23 tests) + Orchestration (18 tests), P1: Auto-Agent, Files, Roadmap, Agent Features
- [x] P2: Conversation, Cost, Settings, Session, MCP, KB, LLM, P3: Service-layer gaps

#### Comprehensive Code Review (COMPLETED)
- [x] 46 issues found across 10 areas (18 critical, 24 important, 4 medium) -- all fixed
- [x] NATS contract fixes, backend executor implementations, runtime state leak fixes
- [x] Security hardening, benchmark fixes, memory/multi-tenancy, PM sync, orchestration fixes

#### Documentation-Code Reconciliation (COMPLETED)
- [x] Python trust/quarantine layer, A2A protocol expansion, handoff enrichment

#### Benchmark Interactive Testing Guide (COMPLETED)
- [x] (2026-03-15) Added step-by-step interactive E2E testing guide to `docs/dev-setup.md` — covers infrastructure verification, API-level testing (all 3 benchmark types, evaluator combinations, comparison, cost analysis, export), frontend dashboard walkthrough, error scenarios, automated E2E suite, troubleshooting table, custom dataset creation

#### Benchmark Cross-Layer Bug Fixes (COMPLETED)
- [x] 7 bugs fixed: DB migration for rollout fields, cost population, NATS wiring, CSV export

#### Benchmark Validation E2E Bug Fixes (COMPLETED)
- [x] (2026-03-15) **Bug 1 — Score Key Mismatch (Medium):** Evaluator dimension names (`correctness`, `sparc_*`, `trajectory_*`) didn't match metric request names (`llm_judge`, `sparc`, `trajectory_verifier`). Added `aggregate_metric_scores()` with `_DIMENSION_TO_METRIC` mapping (now in `workers/codeforge/consumer/_benchmark_gemmas.py`). 16 tests in `workers/tests/test_score_key_normalization.py`.
- [x] (2026-03-15) **Bug 2 — Stuck "running" Runs (High):** Runs with invalid params stayed `"running"` forever. Fix 2A: `StartRun()` returns error when dataset resolution fails and no suite fallback. Fix 2B: Watchdog goroutine scans every 5 min for runs stuck >15 min. Added `ErrorMessage` field to `Run` struct + DB migration `072`. Files: `internal/service/benchmark.go`, `internal/domain/benchmark/benchmark.go`, `internal/adapter/postgres/store_benchmark.go`, `cmd/codeforge/main.go`. 5 tests in `internal/service/benchmark_test.go`.
- [x] (2026-03-15) **Bug 3 — Invalid Model Silently Succeeds (Medium):** LiteLLM fell back to default model. Added `_validate_model_exists()` checking `/v1/models` endpoint in `workers/codeforge/consumer/_benchmark.py`. 6 tests in `workers/tests/test_model_validation.py`.
- [x] (2026-03-15) **Bug 4 — `model=auto` Without Routing (Low):** `_resolve_effective_llm()` silently passed `"auto"` to LiteLLM. Now raises `ValueError` when router unavailable. 2 tests in `workers/tests/test_model_validation.py`.
- [x] (2026-03-15) **Bug 5 — LLM Judge Context Overflow (Low):** Evaluators exceeded local model context limits. Added `compress_for_context()` head+tail truncation in `workers/codeforge/evaluation/evaluators/prompt_compressor.py`. Enhanced error fallback distinguishes `context_overflow` from `evaluation_failed`. 18 tests in `workers/tests/test_prompt_compressor.py`.
- [x] (2026-03-15) Updated E2E test assertions in `block-3-agent.spec.ts`, `block-4-routing.spec.ts`, `block-5-errors.spec.ts` to verify all fixes
- Findings: `frontend/e2e/benchmark-validation/FINDINGS.md`
- Plan: `docs/plans/2026-03-11-benchmark-findings-fixes-plan.md`

#### Benchmark Validation E2E Round 2 — Bugs 6-10 + External Suite Fixes (COMPLETED)
- [x] (2026-03-15) **Bug 6 — Agent Provider Wrong Kwarg (High):** `datasets_dir=` → `dataset_path=` in `_benchmark.py:405`
- [x] (2026-03-15) **Bug 7 — Watchdog Timeout Too Short (High):** 15min → 2h default, configurable via `CODEFORGE_BENCHMARK_WATCHDOG_TIMEOUT` / `benchmark.watchdog_timeout` (`internal/config/loader.go`); since 2026-03-20 overridden per type (simple 30m, tool_use 1h, agent 4h), the global value only applies to runs without a type
- [x] (2026-03-15) **Bug 8 — RolloutOutcome Missing eval_score (High):** Added `eval_score` field to `RolloutOutcome` dataclass in `multi_rollout.py`
- [x] (2026-03-15) **Bug 9 — Wrong Attribute Name in _convert_rollout_outcome (High):** `outcome.execution.*` → `outcome.result.*` in `_benchmark.py:518-527`
- [x] (2026-03-15) **Bug 10 — Hybrid Pipeline Passed as Regular Pipeline (Medium):** Separated pipeline construction, added `hybrid_pipeline` parameter
- [x] (2026-03-16) **Issue D — External Suite HF API Failures:** Fixed BigCodeBench (config/split swap), CRUXEval (dataset moved to `cruxeval-org/cruxeval` + HF_TOKEN auth), LiveCodeBench (correct dataset + adaptive page size fallback 100→10→1 with timeout handling and broken-row skipping)
- [x] (2026-03-16) Early NATS ack in benchmark handler to prevent stale message redelivery
- [x] (2026-03-16) Documented `HF_TOKEN` and `BENCHMARK_WATCHDOG_TIMEOUT` env vars in `docs/dev-setup.md`
- Results: Phase 3b external suites 4/5 PASS (LiveCodeBench partial due to HF server limitations), Phase 5 API 12/12 PASS, Phase 6 errors 2/5 PASS
- Findings: `frontend/e2e/benchmark-validation/FINDINGS.md`

#### Benchmark E2E Full Run (2026-03-19) — Findings & Recommendations (MOSTLY COMPLETED)

> Report: `docs/testing/benchmark-e2e-report.md`
> Full API + Playwright-MCP UI test. 86/90 passed, 4 deferred (queue timing).

##### REC-1: Parallel Benchmark Run Processing with Dependency Awareness (Critical)

> **Problem:** Python worker processes runs sequentially (`consumer/__init__.py:271` blocks on `await handler(msg)`).
> A single agent run (15-30 min) blocks ALL subsequent runs. During E2E test, Phase 4+6 runs waited >30 min.
> **Root cause:** `_message_loop` fetches `batch=1`, awaits handler inline. Go side supports parallelism
> (`MaxAckPending: 100`) but Python serializes everything. Tasks within a run are also sequential
> (`runners/_base.py:run_tasks()` for-loop).

- [x] (2026-03-20, c42bf5a6) REC-1.1: Add `asyncio.Semaphore` to benchmark handler, spawn runs via `asyncio.create_task()` instead of inline `await`
  - File: `workers/codeforge/consumer/_benchmark.py`
  - Config: `CODEFORGE_BENCHMARK_MAX_PARALLEL` env var (default 3, `workers/codeforge/config.py`)
  - Constraint: Agent `mount` mode runs sharing the same project workspace MUST NOT run in parallel (file corruption risk). Guard with per-project workspace lock or reject parallel mount runs to same project.
  - [ ] Still open: per-project workspace lock (or rejection) for parallel mount-mode agent runs
  - Constraint: LLM rate limits are the real parallelism bottleneck — size semaphore based on provider capacity
  - Note: Each run already has its own `RunResult` — no shared mutable state between runs, safe to parallelize
- [x] (2026-03-20, c42bf5a6) REC-1.2: Add structured error handling for concurrent task failures
  - If a spawned task raises an exception, it must still publish `benchmark.run.result` with `status: "failed"` to NATS
  - Use `asyncio.create_task()` with an `add_done_callback` that catches and publishes errors
- [x] (2026-03-20, c42bf5a6) REC-1.3: Update `_message_loop` to support concurrent handlers — solved inside the handler instead: the benchmark handler acks and returns immediately after spawning the run; `_message_loop` stays sequential
  - File: `workers/codeforge/consumer/__init__.py:271`
  - Current: `await handler(msg)` — blocks loop
  - Change: `asyncio.create_task(handler(msg))` — only for benchmark subject, other subjects (conversation, toolcall) remain sequential for ordering guarantees
- [ ] REC-1.4: Add integration test verifying parallel execution (only unit tests exist: `workers/tests/test_benchmark_parallel.py`)
  - Create 3 simple runs with different datasets simultaneously
  - Assert all 3 complete within ~1x single-run duration (not 3x)
  - Assert results are correct and don't interfere

##### REC-2: Trajectory Endpoint Returns 500 for Running Runs (High)

> **Problem:** `GET /runs/{id}/trajectory` returns HTTP 500 when run has no events yet.
> Frontend LiveFeed logs errors + skips hydration for all visible running runs.
> **Location:** `handlers_roadmap.go:444-455`

- [x] (2026-03-20) REC-2.1: Return empty result instead of 500 when `LoadTrajectory()` or `TrajectoryStats()` errors (run-not-found is not distinguished yet; it also returns 200 empty)
  - File: `internal/adapter/http/handlers_roadmap.go:444-455`
  - Fix: On error, set `page = &eventstore.TrajectoryPage{Events: []event.Event{}}` and `stats = &eventstore.TrajectoryStats{}`
  - Distinguish "run not found" (404) from "no events yet" (200 empty) if needed by checking run existence first

##### REC-3: Training Export Returns Empty Body Instead of Empty Array (Medium)

> **Problem:** JSONL export writes nothing when no pairs exist (for-loop iterates zero times).
> Client gets headers but zero-byte body. JSON format correctly returns `[]`.
> **Location:** `handlers_benchmark.go:336-339`

- [x] (2026-03-20) REC-3.1: Add empty-check before JSONL loop, fall back to `[]` JSON response when no pairs exist
  - File: `internal/adapter/http/handlers_benchmark.go` (`ExportTrainingData`)
- [ ] RLVR export (`ExportRLVRData`, `handlers_benchmark.go`) still writes a zero-byte JSONL body when there are no entries

##### REC-4: Suite Creation Should Auto-Derive Type from Provider (Low)

> **Problem:** `POST /suites` requires explicit `type` field. Provider already implies type.
> **Location:** `benchmark.go:86` — `r.Type.IsValid()` rejects empty type

- [x] (2026-03-20) REC-4.1: Add provider-to-type mapping, auto-derive in `RegisterSuite()` before `Validate()` (`benchmark.ProviderDefaultType()`, used in `internal/service/benchmark_suite.go`)
  - File: `internal/service/benchmark.go` (service layer), `internal/domain/benchmark/benchmark.go` (mapping)
  - Frontend: auto-populate type field in `SuiteManagement.tsx` on provider selection (nice-to-have)

##### REC-5: Configurable Watchdog Timeout per Suite/Type (Low)

> **Problem:** Single global 2h watchdog. Simple runs stuck 2h before cleanup, agent runs on slow models killed prematurely.
> **Location:** `cmd/codeforge/main.go` — watchdog goroutine, `internal/service/benchmark.go`

- [x] (2026-03-20) REC-5.1: Use benchmark type as heuristic timeout (no DB change needed) — `watchdogTimeoutForType()` in `internal/service/benchmark.go`
  - `simple` → 30 min, `tool_use` → 1h, `agent` → 4h
  - Or: add optional `timeout` field to `Suite` domain model + DB migration

---

#### Benchmark E2E — Remaining Bugs (COMPLETED 2026-03-16)

> Discovered during E2E validation Round 2 (Phase 6 error scenarios).
> Reference: `frontend/e2e/benchmark-validation/FINDINGS.md` → "Known Issues (Not Yet Fixed)"
> These bugs affect input validation and error handling — the happy path works, but malformed
> requests don't fail cleanly.

##### Issue A: Invalid Model Name Silently Succeeds (Regression from Bug 3)

> **Severity:** Medium
> **E2E Test:** Phase 6.2 — FAIL (run completes with score=0 instead of failing)
> **Root cause:** `_validate_model_exists()` does exact match against `/v1/models` list, but
> LiteLLM accepts `provider/model-name` format models (e.g. `nonexistent/model-xyz-404`) and
> silently falls back or passes them through. The validation only catches bare model names that
> aren't in the list — prefixed names bypass it.

- [x] (2026-03-16) A.1: Write failing test — model with `provider/name` format not in LiteLLM models list
  - File: `workers/tests/test_model_validation.py`
  - Test: `_validate_model_exists("nonexistent/model-xyz-404", available_models=["lm_studio/qwen3-30b"])` should raise `ValueError`
  - Test: `_validate_model_exists("openai/gpt-4", available_models=["openai/gpt-4"])` should pass (exact match still works)
  - Run: `cd workers && poetry run pytest tests/test_model_validation.py -v`

- [x] (2026-03-16) A.2: Fix `_validate_model_exists()` — add /model/info fallback for provider-prefixed models
  - File: `workers/codeforge/consumer/_benchmark.py` (line 57-71)
  - Current logic: `if model not in available_models: raise ValueError`
  - Problem: LiteLLM `/v1/models` might return `lm_studio/qwen3-30b-a3b` while user sends `lm_studio/nonexistent-model` — both have `lm_studio/` prefix but only one is valid
  - Fix option A (preferred): Keep exact match but also attempt a LiteLLM model info call (`/model/info` endpoint) to confirm the model is actually routable. If the model doesn't resolve, raise `ValueError`.
  - Fix option B (simpler): Extract provider prefix from model name (`model.split("/")[0]`), check that the prefix matches at least one available model's prefix AND the full model name matches. If no exact match, raise `ValueError` with helpful message listing similar models.
  - Key constraint: Must not break `model=auto` (already guarded by early return)

- [x] (2026-03-16) A.3: Write test for edge cases
  - File: `workers/tests/test_model_validation.py`
  - Test: model with valid prefix but invalid name (`lm_studio/nonexistent`) → fails
  - Test: model that is a substring of a valid model (`lm_studio/qwen3`) → fails (no partial match)
  - Test: empty model list (LiteLLM unreachable) → passes (existing skip behavior)
  - Test: model `auto` → passes (existing early return)

##### Issue B: HTTP 500 Instead of 400 for Invalid Requests

> **Severity:** High
> **E2E Tests:** Phase 6.1 (invalid dataset) — WEAK PASS (HTTP 500, should be 400),
>                Phase 6.3 (missing required field) — FAIL (HTTP 500, should be 400)
> **Root cause:** `CreateRunRequest.Validate()` returns plain `fmt.Errorf()` errors, but
> `writeDomainError()` only maps `domain.ErrValidation`-wrapped errors to HTTP 400. Plain
> errors fall through to the `default` case → HTTP 500.

- [x] (2026-03-16) B.1: Write failing Go test — validation errors should return HTTP 400
  - File: `internal/adapter/http/handlers_test.go`
  - Test: `POST /api/v1/benchmarks/runs` with `{"model": "gpt-4", "metrics": ["llm_judge"]}` (missing dataset AND suite_id) → expect HTTP 400 with `"dataset or suite_id is required"`
  - Test: `POST /api/v1/benchmarks/runs` with `{"dataset": "foo", "metrics": ["llm_judge"]}` (missing model) → expect HTTP 400 with `"model is required"`
  - Test: `POST /api/v1/benchmarks/runs` with `{"dataset": "foo", "model": "gpt-4"}` (missing metrics) → expect HTTP 400 with `"at least one metric is required"`
  - Test: `POST /api/v1/benchmarks/runs` with `{"dataset": "foo", "model": "gpt-4", "metrics": ["llm_judge"], "benchmark_type": "invalid"}` → expect HTTP 400 with `"invalid benchmark type"`
  - Run: `cd /workspaces/CodeForge && go test ./internal/adapter/http/ -run TestCreateBenchmarkRun -v`

- [x] (2026-03-16) B.2: Wrap `Validate()` errors with `domain.ErrValidation`
  - File: `internal/domain/benchmark/benchmark.go` (line 175-190, `Validate()` method)
  - Current: `return fmt.Errorf("model is required")`
  - Fix: `return fmt.Errorf("%w: model is required", domain.ErrValidation)`
  - Apply to ALL 5 error returns in `Validate()`:
    1. `dataset or suite_id is required`
    2. `model is required`
    3. `at least one metric is required`
    4. `invalid benchmark type: %q`
    5. `invalid exec mode: %q`
  - Import: `"github.com/CodeForge/internal/domain"` (or wherever `ErrValidation` is defined)
  - This ensures `writeDomainError()` in `internal/adapter/http/helpers.go:114` matches the `errors.Is(err, domain.ErrValidation)` case → HTTP 400

- [x] (2026-03-16) B.3: Verify `StartRun()` dataset-not-found also returns 400 (not 500)
  - File: `internal/service/benchmark.go` (line 192)
  - Current: `return nil, fmt.Errorf("dataset %q not found: %w", run.Dataset, statErr)`
  - This wraps `statErr` (an `os.PathError`) — `writeDomainError()` won't match it → HTTP 500
  - Fix: `return nil, fmt.Errorf("%w: dataset %q not found", domain.ErrValidation, run.Dataset)`
  - Test: `POST /api/v1/benchmarks/runs` with `{"dataset": "nonexistent-xyz", "model": "gpt-4", "metrics": ["llm_judge"]}` → expect HTTP 400 (not 500)

- [x] (2026-03-16) B.4: Run full test suite to verify no regressions
  - Run: `cd /workspaces/CodeForge && go test ./internal/... -count=1`
  - All existing benchmark tests must still pass

##### Issue C: Unknown Evaluator Names Silently Ignored

> **Severity:** Medium
> **E2E Test:** Phase 6.4 — FAIL (run completes with empty scores instead of failing)
> **Root cause:** `_build_evaluators()` in `workers/codeforge/consumer/_benchmark.py:652-697`
> uses a `logger.warning("unknown evaluator, skipping")` for unrecognized names (line 681),
> then falls through to the "no evaluators" fallback which creates a default LLMJudgeEvaluator.
> So requesting `metrics: ["nonexistent_evaluator"]` silently succeeds with a default evaluator.

- [x] (2026-03-16) C.1: Decide on validation strategy (two options):
  - **Option 1 — Go-side validation (preferred):** Add a `ValidMetrics` set in `internal/domain/benchmark/benchmark.go` containing the 4 valid top-level metrics (`llm_judge`, `functional_test`, `sparc`, `trajectory_verifier`) plus the 5 LLM judge sub-metrics (`correctness`, `faithfulness`, `relevance`, `coherence`, `fluency`). Check each element of `req.Metrics` in `Validate()` — reject with HTTP 400 if unknown.
  - **Option 2 — Python-side validation:** Change `_build_evaluators()` to raise `ValueError` instead of logging a warning for unknown names. The existing error handler would then publish `status=failed` with a descriptive error message.
  - **Recommendation:** Option 1 (Go-side) catches errors earlier and returns proper HTTP 400. Option 2 is a safety net. Implement both.

- [x] (2026-03-16) C.2: Write failing Go test — unknown metric names rejected
  - File: `internal/domain/benchmark/benchmark_test.go`
  - Test: `CreateRunRequest{Metrics: []string{"nonexistent_evaluator"}}` → `Validate()` returns error containing `"unknown metric"` and `"nonexistent_evaluator"`
  - Test: `CreateRunRequest{Metrics: []string{"llm_judge", "functional_test"}}` → `Validate()` returns nil (valid)
  - Test: `CreateRunRequest{Metrics: []string{"llm_judge", "invalid"}}` → `Validate()` returns error (one invalid is enough to reject)
  - Run: `cd /workspaces/CodeForge && go test ./internal/domain/benchmark/ -v`

- [x] (2026-03-16) C.3: Add `ValidMetrics` set and check in `Validate()`
  - File: `internal/domain/benchmark/benchmark.go`
  - Add: `var ValidMetrics = map[string]bool{"llm_judge": true, "functional_test": true, "sparc": true, "trajectory_verifier": true, "correctness": true, "faithfulness": true, "relevance": true, "coherence": true, "fluency": true}`
  - In `Validate()`, after the `len(r.Metrics) == 0` check, add:
    ```go
    for _, m := range r.Metrics {
        if !ValidMetrics[m] {
            return fmt.Errorf("%w: unknown metric %q; valid metrics: llm_judge, functional_test, sparc, trajectory_verifier", domain.ErrValidation, m)
        }
    }
    ```

- [x] (2026-03-16) C.4: Python-side safety net — raise instead of warn for unknown evaluators
  - File: `workers/codeforge/consumer/_benchmark.py` (line 681)
  - Current: `logger.warning("unknown evaluator, skipping", evaluator=name)`
  - Fix: `raise ValueError(f"unknown evaluator/metric: {name!r}. Valid: llm_judge, functional_test, sparc, trajectory_verifier, correctness, faithfulness, relevance, coherence, fluency")`
  - This ensures that even if Go-side validation is bypassed (e.g. direct NATS message), the worker fails cleanly
  - Write test in `workers/tests/test_score_key_normalization.py` or new file:
    `_build_evaluators(["nonexistent_evaluator"], "gpt-4")` → raises `ValueError`

- [x] (2026-03-16) C.5: Remove the "no evaluators" fallback default
  - File: `workers/codeforge/consumer/_benchmark.py` (lines 691-697)
  - Current: `if not evaluators:` → creates a default `LLMJudgeEvaluator`
  - This fallback masks validation failures. After C.3+C.4, it should never be reached.
  - Replace with: `if not evaluators: raise ValueError("no valid evaluators after processing metrics list")`
  - Or remove the fallback entirely (the `raise ValueError` in C.4 will already prevent reaching this code)

##### Issue E: LiveCodeBench — Replace HF HTTP API with `datasets` Library

> **Severity:** Low (workaround exists: `max_tasks: 3`)
> **Root cause:** HuggingFace Datasets Server HTTP API (`datasets-server.huggingface.co/rows`)
> can't serve `livecodebench/code_generation` rows reliably — returns 502/504 for large rows
> even at page_size=10. Current adaptive page_size fallback (100→10→1) works but is extremely
> slow (~12h for 880 rows). Some rows return 500 and are skipped entirely.

- [x] (2026-03-16) E.1: Add `datasets` library to Poetry dependencies
  - File: `workers/pyproject.toml`
  - Add: `datasets = "^3.0"` (HuggingFace datasets library)
  - Run: `cd workers && poetry add datasets`
  - Note: This is a large dependency (~100MB with Apache Arrow). Consider making it optional via extras: `[tool.poetry.extras] hf = ["datasets"]`

- [x] (2026-03-16) E.2: Add `download_hf_dataset_parquet()` alternative in cache module
  - File: `workers/codeforge/evaluation/cache.py`
  - New function: `async def download_hf_dataset_parquet(dataset, split, provider_name, filename, base_dir, config)` that:
    1. Checks cache first (same `get_cached_path()` logic)
    2. Uses `datasets.load_dataset(dataset, config, split=split)` for direct Parquet download
    3. Converts to JSONL and saves to cache directory
    4. Handles `HF_TOKEN` authentication via `datasets.login(token=hf_token)` or `HfFolder.save_token()`
  - This bypasses the HTTP rows API entirely — uses HuggingFace Hub direct download

- [x] (2026-03-16) E.3: Update LiveCodeBench provider to use Parquet download
  - File: `workers/codeforge/evaluation/providers/livecodebench.py`
  - Change `_fetch_tasks()` to call `download_hf_dataset_parquet()` instead of `download_hf_dataset()`
  - Keep `download_hf_dataset()` as fallback for providers that work fine with HTTP API (humaneval, mbpp, bigcodebench, cruxeval)

- [x] (2026-03-16) E.4: Write tests for Parquet download path
  - File: `workers/tests/test_cache_parquet.py`
  - Test: mock `datasets.load_dataset()` → verify JSONL file created with correct records
  - Test: cached file exists → skips download
  - Test: `HF_TOKEN` env var propagated to datasets library
  - Run: `cd workers && poetry run pytest tests/test_cache_parquet.py -v`

#### Evaluation System Improvements — R2E-Gym Cherry-Picks + Categorical Verifier (COMPLETED)

> Three targeted improvements to the Phase 26+28 evaluation pipeline. Python-only, no Go/NATS/frontend changes.

- [x] (2026-03-16) **LogprobVerifierEvaluator (new):** Calibrated ranking via P(YES) logprobs with `max_tokens=1`. Softmax normalization `P(YES) = exp(yes_lp) / (exp(yes_lp) + exp(no_lp))`. Falls back to text parsing when provider doesn't support logprobs. Registered in `_build_evaluators()` + `_DIMENSION_TO_METRIC`. Files: `workers/codeforge/evaluation/evaluators/logprob_verifier.py` (new), `workers/tests/test_logprob_verifier.py` (new, 13 tests), `workers/codeforge/consumer/_benchmark.py`.
- [x] (2026-03-16) **Categorical TrajectoryVerifier:** Replaced unreliable float-based scoring (0.0-1.0) with ACHIEVED/PARTIALLY_ACHIEVED/NOT_ACHIEVED categories (based on RocketEval ICLR 2025, Prometheus ICLR 2024). Same 5 dimensions, same interface, case-insensitive parsing with backward compat for floats. `max_tokens` reduced 256->128. Files: `workers/codeforge/evaluation/evaluators/trajectory_verifier.py`, `workers/tests/test_trajectory_verifier.py` (5 new + 2 updated tests).
- [x] (2026-03-16) **Selection strategies (longest/shortest):** Added trajectory-length-based selection for `MultiRolloutRunner` (from R2E-Gym). `_trajectory_length()` helper with 3-tier fallback (trajectory -> step_count -> actual_output). Zero-cost heuristic when hybrid pipeline unavailable. Files: `workers/codeforge/evaluation/runners/multi_rollout.py`, `workers/tests/test_multi_rollout_runner.py` (7 new tests).
- Total: 48 tests (27 new + 21 existing), 7 files changed (+718/-30 lines)

#### Sidebar Restructure (COMPLETED)
- [x] (2026-03-16) Section grouping, page merges, top bar navigation

#### Loading Animation System (COMPLETED)
- [x] (2026-03-16) 5 primitives: Skeleton (text/rect/circle), TypingIndicator (bouncing dots), StreamingCursor (blink cursor), ProgressBar (determinate/indeterminate), PacmanSpinner (branded SVG)
- [x] (2026-03-16) 4 composites: SkeletonText, SkeletonCard (stat/project), SkeletonTable, SkeletonChat
- [x] (2026-03-16) 7 CSS keyframes: cf-shimmer, cf-blink, cf-bounce-dot, cf-progress-slide, cf-pacman-chomp, cf-dot-orbit, cf-fade-in
- [x] (2026-03-16) Skeleton design tokens (--cf-skeleton-base, --cf-skeleton-shine) for light/dark themes, registered in @theme
- [x] (2026-03-16) ChatPanel: TypingIndicator replaces animate-pulse, StreamingCursor replaces static "Streaming..." label
- [x] (2026-03-16) ResourceGuard: optional `skeleton` prop for custom loading states (backward-compatible)
- Note (2026-03-18): ProgressBar, PacmanSpinner, SkeletonText, SkeletonChat, ResourceGuard and the keyframes cf-progress-slide / cf-pacman-chomp / cf-dot-orbit were removed as unused (c124162e); SkeletonCard and SkeletonTable were restored (47705a3f). Skeleton, TypingIndicator and StreamingCursor remain.

#### Benchmark Metric Validation & Detail Card Fix (COMPLETED)
- [x] (2026-03-16) **Go ValidMetrics allowlist gap:** Frontend offers 5 metrics (`correctness`, `tool_correctness`, `faithfulness`, `answer_relevancy`, `contextual_precision`) but Go `ValidMetrics` only had 9 entries — missing `tool_correctness`, `answer_relevancy`, `contextual_precision`. Runs with all metrics failed HTTP 400. Added 3 missing metrics to `internal/domain/benchmark/benchmark.go:180`. 29 Go tests pass.
- [x] (2026-03-16) **SolidJS event delegation bug in benchmark detail card:** Clicking task rows in `BenchmarkRunDetail` collapsed the parent card because SolidJS delegates all `onClick` to `document` — `stopPropagation()` alone doesn't prevent parent SolidJS handlers. Fixed parent card `onClick` in `BenchmarkPage.tsx:346` with `target.closest("table"|"button"|"a")` guard. Added `stopPropagation()` on task row as defense-in-depth.
- [x] (2026-03-16) **Verified via Playwright MCP** with `lm_studio/qwen/qwen3-30b-a3b`: detail card shows summary scores + task results table, all 3 task rows expand/collapse correctly showing Actual Output and Evaluator Scores.

#### Frontend UI Bug Fixes & i18n (COMPLETED)
- [x] (2026-03-15) **BUG-1 (High) — Broken "Go to Chat" Navigation:** `onNavigate("chat")` silently did nothing — `"chat"` was not a valid `LeftTab`. Created unified `handleNavigate()` in `ProjectDetailPage.tsx` that switches `mobileView` to `"chat"` on mobile. Replaced 8 duplicate inline handlers. Fixed in: `GoalsPanel.tsx:202`, `SessionPanel.tsx:116`, `WarRoom.tsx:90`, `OnboardingProgress.tsx:44`.
- [x] (2026-03-15) **BUG-2 (Medium) — Dead RunPanel Code:** `run.toolcall` WS event was a stub comment. `RunPanel.addToolCall`/`updateRunStatus` attached to component function object but never called. Removed dead code — tool calls are rendered via AG-UI events in `ChatPanel`. Files: `ProjectDetailPage.tsx`, `RunPanel.tsx`.
- [x] (2026-03-15) **BUG-3 (Low) — `window.prompt()` for Folder Creation:** Replaced `window.prompt("New folder name:")` with custom Modal dialog consistent with Create/Rename/Delete modals. New state: `showFolderModal`, `newFolderName`, `newFolderPrefix`. File: `FilePanel.tsx`.
- [x] (2026-03-15) **Monaco Theme Sync:** Editor now reactively follows dark/light theme toggle via `createEffect` + `monaco.editor.setTheme()`. File: `CodeEditor.tsx`.
- [x] (2026-03-15) **File Panel Icon Alignment:** Expand/Collapse-all SVG polyline points centered in 16x16 viewBox. File: `FilePanel.tsx`.
- [x] (2026-03-15) **i18n: ~40 hardcoded strings replaced** across `FilePanel.tsx`, `FileContextMenu.tsx`, `GoalProposalCard.tsx`, `KnowledgeBasesPage.tsx`. 28 new keys in `en.ts` + `de.ts` (`files.*`, `common.approve`, `common.reject`, `detail.tab.files`).
- [x] (2026-03-15) **"Allow Always" Policy Persistence:** `PermissionRequestCard.tsx` TODO resolved. Clicking "Allow Always" now approves the current tool call AND adds an `allow` rule to the project's policy profile via `POST /api/v1/policies/allow-always` (in memory; written to disk only when a policy directory is configured, which `cmd/codeforge/main.go` does not do, see [KI-7](#known-issues)). Preset profiles are cloned to `{preset}-custom-{projectId}` on first use. Rule construction: tool name + first word of command as glob pattern (e.g., `Bash/git*`). Idempotent (duplicate rules detected via `HasRuleForSpecifier`). 26 new tests across domain, service, and HTTP layers. Files: `internal/domain/policy/policy.go`, `internal/service/policy.go`, `internal/service/project.go`, `internal/adapter/http/handlers_policy_crud.go`, `internal/adapter/http/routes.go`, `frontend/src/api/resources/settings.ts`, `frontend/src/features/project/PermissionRequestCard.tsx`, `frontend/src/features/project/ChatPanel.tsx`.

#### Benchmark Live Feed (COMPLETED)
- [x] (2026-03-10) Go: `TrajectoryEventPayload` in `internal/domain/event/broadcast_payloads.go` — enriched WS broadcast with cost, tokens, input, output, step fields
- [x] (2026-03-10) Go: Runtime trajectory subscription handler broadcasts enriched payload
- [x] (2026-03-10) TypeScript: `LiveFeedEvent` + `BenchmarkLiveProgress` types in `api/types.ts`
- [x] (2026-03-10) Frontend: `BenchmarkLiveFeed.tsx` — virtualized auto-scrolling feed with `@tanstack/solid-virtual`, feature accordions, progress header, elapsed timer
- [x] (2026-03-10) Integration: Wired into `BenchmarkPage.tsx` for selected running runs
- Design: `docs/specs/2026-03-10-benchmark-live-feed-design.md`
- Plan: `docs/plans/2026-03-10-benchmark-live-feed-plan.md`

#### Benchmark Live Feed — State Persistence & Density Improvements (COMPLETED)
- [x] (2026-03-16) State persistence: Live feed state lifted from `BenchmarkLiveFeed` to `BenchmarkPage` as `Map<runId, LiveFeedState>` — closing/reopening info card no longer loses state
- [x] (2026-03-16) API hydration: Running runs rehydrate from `GET /runs/{id}/trajectory` + `GET /benchmarks/runs/{id}/results` on page load
- [x] (2026-03-16) `BenchmarkLiveFeed` converted to presentational component (receives `LiveFeedState` props)
- [x] (2026-03-16) Pure functions extracted to `liveFeedState.ts` with 18 unit tests: `formatTokens`, `computeEta`, `agentEventToLiveFeedEvent`, `statsFromSummary`, `resultToFeatureEntry`
- [x] (2026-03-16) Inline stats line: avg score, tokens in/out, tool calls, $/task
- [x] (2026-03-16) Mini score bars on feature rows (green/yellow/red color coding)
- [x] (2026-03-16) ETA display when total_tasks known
- [x] (2026-03-16) Indeterminate progress bar fix for unknown total_tasks
- [x] (2026-03-18) Event dedup: backend `sequence_number` on trajectory events (migration 077, Go eventstore, frontend dedup)
- [x] (2026-03-18) WS reconnect gap: `after_sequence` REST param, frontend gap-fill on reconnect
- Spec: `docs/specs/2026-03-16-benchmark-live-feed-density-design.md`
- Plan: `docs/plans/2026-03-16-benchmark-live-feed-improvements-plan.md`

#### Agent-Eval Benchmark Results (2026-03-10)
- [x] (2026-03-10) Ran `/agent-eval mistral/mistral-large-latest` — auto-agent pipeline end-to-end
- Result: 0/300 total score (Grade F) — Mistral model could not produce code within 43 min
- All skeleton files unchanged — agent stalled on spec reading without invoking Write tool
- Infrastructure verified: workspace paths absolute, project seeding works, test suites collect correctly

#### Test Suites (COMPLETED)
- [x] Browser E2E: 17 Playwright tests (health, navigation, projects, costs, models, a11y)
- [x] LLM E2E: 95 API-level tests across 11 spec files
- [x] Benchmark E2E: 132 browser Playwright tests across 12 spec files
- [x] Benchmark Validation E2E: 22 API-level tests across 7 blocks (`frontend/e2e/benchmark-validation/`)
- [x] Backend E2E: 88 pass / 0 fail / 3 skip (97% pass rate)
- [x] Python unit tests: 134 pass as of 2026-03-16 (107 prior + 27 new from evaluation improvements); the suite has since grown to ~140 test files / ~2,400 test functions

#### Chat Enhancements (COMPLETED)
- [x] (2026-03-10) Phase 1: HITL permission UI + `supervised-ask-all` preset + autonomy-to-preset mapping
- [x] (2026-03-10) Phase 2: Inline diff review with DiffPreview component + file content endpoint
- [x] (2026-03-10) Phase 3: Action buttons (copy, retry, apply, view diff) on agent messages
- [x] (2026-03-10) Phase 4: Per-message cost tracking with MessageBadge + CostBreakdown
- [x] (2026-03-10) Phase 5: Smart references with @/#// autocomplete popover + frequency tracker
- [x] (2026-03-10) Phase 6: Slash commands (/compact, /rewind, /clear, /help, /mode, /model)
- [x] (2026-03-10) Phase 7: Conversation full-text search with PostgreSQL FTS (GIN index, ts_rank)
- [x] (2026-03-10) Phase 8: Notification center with browser push, sound, tab badge, AG-UI wiring
- [x] (2026-03-10) Phase 9: Real-time channels with threads, domain model, sidebar integration (channel WS events are never broadcast, see [KI-42](#known-issues))
- [x] (2026-03-10) Phase 10+11: Feature spec + documentation updates
- Feature spec: [docs/features/05-chat-enhancements.md](features/05-chat-enhancements.md)

#### Subscription Provider Integration (COMPLETED)
- [x] (2026-03-10) OAuth device flow adapters: Anthropic (Claude Max) + GitHub Copilot
- [x] (2026-03-10) Atomic EnvWriter service for .env file management
- [x] (2026-03-10) Subscription orchestration service (background polling, token exchange, .env persistence)
- [x] (2026-03-10) HTTP endpoints: list/connect/status/disconnect providers
- [x] (2026-03-10) Python routing: github_copilot in key_filter, router tiers, meta_router tiers
- [x] (2026-03-10) LiteLLM config: extra_headers for github_copilot
- [x] (2026-03-10) Frontend: Subscription Providers section in SettingsPage with device flow UI
- [x] (2026-03-10) Tests: 22 auth adapter tests, 9 envwriter tests, 8 subscription tests, 48 Python routing tests

---

### E2E Playwright Test Findings (2026-03-09)

> 5 findings from interactive end-to-end Playwright MCP test of full agent evaluation workflow.

#### F4: Workspace Path Resolution Bug (Priority: CRITICAL)

> Python worker resolves workspace paths relative to its own CWD (`workers/`) instead of the
> project root. Agent tools write files to `workers/data/workspaces/.../data/workspaces/.../`
> (doubled path). This blocks ALL agent file operations end-to-end.

- [x] F4.1: Add integration test reproducing the bug (2026-03-09) — create a project, seed a file via API, then call `resolve_safe_path("data/workspaces/{tid}/{pid}", "lru_cache.py")` and assert the resolved path matches the actual file on disk
  - File: `workers/tests/test_workspace_path_resolution.py`
  - Run: `cd workers && poetry run pytest tests/test_workspace_path_resolution.py -v`
  - Expected: FAIL (confirms bug exists)

- [x] F4.2: Fix workspace path — Go `NewProjectService` resolves to absolute via `filepath.Abs()` (2026-03-09)
  - File: `workers/codeforge/tools/_base.py:64`
  - Current: `workspace = Path(workspace_path).resolve()` — resolves relative to CWD
  - Fix option A (preferred): Make Go Core send **absolute** paths in NATS payload — change `proj.WorkspacePath` to `filepath.Join(cfg.DataDir, proj.WorkspacePath)` before publishing
    - File: `internal/service/conversation_agent.go:255,433,698`
    - Also update: `internal/service/auto_agent.go` (wherever it publishes workspace_path)
  - Fix option B (fallback): In Python `_base.py`, detect relative paths and resolve against a known `CODEFORGE_ROOT` env var
    - File: `workers/codeforge/tools/_base.py:64`
    - Change: `workspace = Path(workspace_path) if Path(workspace_path).is_absolute() else (Path(os.environ.get("CODEFORGE_ROOT", "/workspaces/CodeForge")) / workspace_path)`

- [x] F4.3: Fix `bash.py` tool CWD — automatically fixed by F4.2 (absolute paths from Go) (2026-03-09)
  - File: `workers/codeforge/tools/bash.py:81`
  - Current: `cwd=workspace_path` (relative → wrong CWD)
  - Fix: Same as F4.2 — if Go sends absolute path, this is automatically fixed

- [x] F4.4: Add Go workspace path tests — 3 tests in `project_workspace_test.go` (2026-03-09)
  - File: `internal/port/messagequeue/contract_test.go` (add assertion)
  - File: `workers/tests/test_nats_contracts.py` (add assertion)
  - Rule: `workspace_path` must start with `/` in all `conversation.run.start` payloads

- [x] F4.5: Integration tests passing (2026-03-09)
  - Run: `cd workers && poetry run pytest tests/test_workspace_path_resolution.py -v`
  - Expected: PASS

- [x] (2026-03-10) F4.6: Re-run agent-eval to verify end-to-end fix — workspace paths are absolute (verified), but Mistral model scored 0/300 (agent stalled, never wrote code in 43 min)
  - Run: `/agent-eval mistral/mistral-large-latest`
  - Result: 0/3 features completed, 1 failed, agent timed out after 43 min
  - Root cause: Mistral free-tier rate limiting + model unable to invoke Write tool correctly
  - Workspace path fix (F4.2) confirmed working — paths are absolute in NATS payloads

#### F3: Routing Does Not Fallback on Provider Billing Errors (Priority: HIGH)

> When Anthropic credits are exhausted, the router selected `anthropic/claude-sonnet-4` and
> failed without trying another provider. Rate tracker only handles 429 (rate limit), not
> 401/402/billing errors.

- [x] F3.1: Add test for billing/auth error classification (2026-03-09)
  - File: `workers/tests/test_routing_error_classification.py` (billing/auth exhaustion and cooldown tests; no standalone `test_routing_rate_tracker.py`)
  - Test: Call `rate_tracker.record_error("anthropic", error_type="billing")` → `is_exhausted("anthropic")` returns `True`
  - Test: Call `rate_tracker.record_error("anthropic", error_type="auth")` → `is_exhausted("anthropic")` returns `True`
  - Run: `cd workers && poetry run pytest tests/test_routing_error_classification.py -v`

- [x] F3.2: Add `record_error()` to `RateLimitTracker` with billing/auth cooldowns (2026-03-09)
  - File: `workers/codeforge/routing/rate_tracker.py`
  - Add: `record_error(provider, error_type)` method that marks provider as exhausted for longer duration (e.g. 1 hour for billing, 5 min for auth)
  - Billing/auth errors should mark provider exhausted with longer cooldown than rate limits

- [x] F3.3: Add `classify_error_type()` in `llm.py` wired into `_with_retry` (2026-03-09)
  - File: `workers/codeforge/llm.py`
  - Parse LiteLLM exceptions: `AuthenticationError` → "auth", `BudgetExceededError` / status 402 → "billing", `RateLimitError` → "rate_limit"
  - Feed classification to rate tracker on failure

- [x] (2026-03-09) F3.4: Add retry-with-fallback in agent loop LLM call path
  - File: `workers/codeforge/agent_loop.py`
  - Wired `classify_error_type()` + `get_tracker().record_error()` into `_try_model_fallback`
  - Provider extracted from model string (e.g., "anthropic" from "anthropic/claude-sonnet-4")

- [x] (2026-03-09) F3.5: Enable routing by default when multiple providers are configured
  - File: `internal/config/config.go` — `Defaults()` now sets `Routing.Enabled: true`
  - Override: `CODEFORGE_ROUTING_ENABLED=false` or YAML `routing.enabled: false`

- [x] (2026-03-10) F3.6: E2E test verifying full routing fallback chain — 6/6 tests pass
  - File: `workers/tests/test_routing_fallback_e2e.py`
  - Tests: billing error classification, fallback chain filtering, agent loop model fallback, all fallbacks exhausted, auth error trigger, rate limit short cooldown
  - Run: `cd workers && poetry run pytest tests/test_routing_fallback_e2e.py -v`

#### F6: Tool-Message Format Incompatibility on Mid-Conversation Model Switch (Priority: HIGH)

> When `_RoutingLLMWrapper` routes to different models per-iteration within the same agent loop
> (e.g. Gemini → Groq), the message history accumulates tool-result messages in one provider's
> format that the new provider rejects. Groq requires `content` on `role:tool` messages, but
> Gemini may omit it.
>
> Error: `'messages.3' : for 'role:tool' the following must be satisfied[('messages.3.content' : property 'content' is missing)]`

- [x] (2026-03-10) F6.1: Add test reproducing tool-message format rejection on model switch — 12 tests
  - File: `workers/tests/test_tool_message_compat.py`
  - Tests: empty content, missing content, None content, missing tool_call_id, mixed messages, routing wrapper sanitization
  - Run: `cd workers && poetry run pytest tests/test_tool_message_compat.py -v`

- [x] (2026-03-10) F6.2: Add `sanitize_tool_messages()` normalizer in `agent_loop.py`
  - File: `workers/codeforge/loop_helpers.py` (moved out of `agent_loop.py` in the 2026-03-24 decomposition)
  - Ensures all `role:tool` messages have `content` (defaults to `""`) and `tool_call_id`
  - Also fixed `_payload_to_dict()` to always include `content` for `role:tool` messages

- [x] (2026-03-10) F6.3: Wire sanitizer into `_RoutingLLMWrapper` and `AgentLoopExecutor`
  - File: `workers/codeforge/consumer/_benchmark.py` — `_RoutingLLMWrapper._sanitize_messages()` called before forwarding
  - File: `workers/codeforge/agent_loop.py` — `sanitize_tool_messages(messages)` called before `chat_completion_stream`

- [x] (2026-03-10) F6.4: Integration test — routing wrapper with tool message sanitization
  - File: `workers/tests/test_tool_message_compat.py` (`TestSanitizeWiredIntoLLMCall`)
  - Verifies `_RoutingLLMWrapper` sanitizes messages before forwarding to real LLM

#### F7: 429 Rate-Limit Should Trigger Immediate Model Fallback (Priority: HIGH)

> The `_with_retry` logic in `llm.py` retries the same rate-limited model 2 times with
> exponential backoff (20s + 58s waits) before failing, instead of immediately switching
> to a fallback model. This wastes ~78+ seconds per task on exhausted providers (e.g.
> Gemini free-tier 20 req/min limit).
>
> Expected: On first 429, immediately try the next fallback model from the routing plan.
> Current: Retries same exhausted model 2x, then fails the entire call.

- [x] (2026-03-10) F7.1: Add test for immediate-fallback-on-429 behavior — 8 tests
  - File: `workers/tests/test_llm_retry_fallback.py`
  - Tests: 429 raises immediately (no retry), 502/503/504 still retried, model fallback on 429, fallback chain exhausted, 429 total time <2s, rate tracker integration
  - Run: `cd workers && poetry run pytest tests/test_llm_retry_fallback.py -v`

- [x] (2026-03-10) F7.2: Remove 429 from `retryable_codes` in `LLMClientConfig`
  - File: `workers/codeforge/llm.py` (line 184)
  - Changed: `retryable_codes` from `(429, 502, 503, 504)` to `(502, 503, 504)`
  - 429 now propagates immediately to agent loop's `_try_model_fallback` which switches models
  - No refactoring of `_with_retry` needed — existing agent loop fallback logic handles everything

- [x] (2026-03-10) F7.3: No additional wiring needed
  - Agent loop's `_handle_llm_error` → `_try_model_fallback` → `_pick_next_fallback` already handles model switching via `LoopConfig.fallback_models`
  - The fix in F7.2 (removing 429 from retryable) is sufficient to trigger this existing path

- [x] (2026-03-10) F7.4: Integration tests verify full fallback chain
  - File: `workers/tests/test_llm_retry_fallback.py` (`TestFallbackChain`, `TestRateLimitTrackerIntegration`)
  - 429 resolves to fallback model in <2s, rate tracker records exhausted providers

#### F1: No File Upload/Create in Project UI (Priority: MEDIUM)

> FilePanel only displays files — no buttons to create, upload, or edit files.
> Users must use the REST API as workaround.

- [x] F1.1: Add i18n keys for file management actions (2026-03-09)
  - File: `frontend/src/i18n/en.ts`
  - Keys: `files.createFile`, `files.uploadFile`, `files.fileName`, `files.fileContent`, `files.createSuccess`, `files.uploadSuccess`, `files.createFailed`
  - File: `frontend/src/i18n/locales/de.ts` (German translations)

- [x] F1.2: Add "Create File" button and modal to FilePanel (2026-03-09)
  - File: `frontend/src/features/project/FilePanel.tsx`
  - Add: Button in the file tree header (+ icon or "New File" text)
  - Add: Modal with `path` (text input) and `content` (textarea) fields
  - Call: `api.files.write(projectId, path, content)` on submit
  - Show: toast on success/error

- [x] (2026-03-09) F1.3: Add "Upload File" button with native file picker
  - File: `frontend/src/features/project/FilePanel.tsx`
  - Upload button with SVG icon in SidebarHeader, hidden `<input type="file">`, FileReader handler
  - Calls `api.files.write()`, shows toast, opens uploaded file in editor

- [x] (2026-03-10) F1.4: Playwright E2E tests for file creation — 4/4 pass
  - File: `frontend/e2e/file-crud.spec.ts`
  - Tests: create file via API + verify in file tree, read back content, overwrite replaces content, special characters in name
  - Run: `cd frontend && npx playwright test file-crud.spec.ts`

#### F2: No Feature Description Field in Create/Edit Modal (Priority: MEDIUM)

> FeatureCardForm only has a title input — no description/body textarea.
> Feature descriptions are critical for agent consumption (contain full problem specs).

- [x] F2.1: Add i18n key `featuremap.descriptionPlaceholder` (2026-03-09)
  - File: `frontend/src/i18n/en.ts`
  - Keys: `featuremap.description`, `featuremap.descriptionPlaceholder`
  - File: `frontend/src/i18n/locales/de.ts`

- [x] F2.2: Description wired into create/update API calls (2026-03-09)
  - File: `frontend/src/api/client.ts` (around line 1170)
  - Check: Does `api.roadmap.createFeature()` accept a `description` field? If not, add it.
  - Check: Does `api.roadmap.updateFeature()` accept a `description` field? If not, add it.

- [x] F2.3: Add description textarea to FeatureCardForm (2026-03-09)
  - File: `frontend/src/features/project/featuremap/FeatureCardForm.tsx`
  - Add: `const [description, setDescription] = createSignal(props.feature?.description ?? "");`
  - Add: `<textarea>` between the title input and status selector
  - Pass: `description` to `createFeature()` and `updateFeature()` calls (lines 48, 41)
  - Style: Match existing form patterns (rounded-cf-sm, border-cf-border, p-2)

- [x] (2026-03-10) F2.4: Playwright E2E tests for feature descriptions — 4/4 pass
  - File: `frontend/e2e/feature-description.spec.ts`
  - Tests: create with description, update persists changes, visible in project UI, empty description allowed
  - Run: `cd frontend && npx playwright test feature-description.spec.ts`

#### F5: Playwright MCP Session Not Recoverable After Container Restart (Priority: LOW)

> After `docker restart codeforge-playwright`, the MCP session ID becomes stale.
> All subsequent browser_* calls return "Session not found".

- [x] F5.1: Document Playwright MCP session limitation (2026-03-09, commit 197557c)
  - File: `docs/dev-setup.md`
  - Add section: "Playwright MCP Container" with note that session is lost on restart
  - Workaround: Restart the Claude Code session (or MCP client) after container restart

- [x] F5.2: Add health check to Playwright Docker service (2026-03-09, commit 197557c)
  - File: `docker-compose.yml`
  - Add: `healthcheck` to `codeforge-playwright` service (test: HTTP GET to :8001/mcp)
  - Ensures container is only "healthy" when MCP server is accepting connections

---

### Auto-Agent Skills System (Phase 31)

> Design: [docs/specs/2026-03-09-auto-agent-skills-design.md](specs/2026-03-09-auto-agent-skills-design.md)
> Plan: [docs/plans/2026-03-09-auto-agent-skills-plan.md](plans/2026-03-09-auto-agent-skills-plan.md)
> Goal: Auto-agent automatically selects and uses relevant skills via LLM, with multi-format import, agent-generated skills, and prompt injection protection.

#### Task 1: DB Migration — Extend skills table (Priority: CRITICAL)
- [x] T1.1: Write migration 067 — add type, source, source_url, format_origin, status, usage_count, content columns (2026-03-09)
  - File: `internal/adapter/postgres/migrations/067_extend_skills.sql`
  - Includes: check constraints, status index, data migration (code → content)
- [x] T1.2: Verify migration applies cleanly (2026-03-09)
- [x] T1.3: Commit (2026-03-09)

#### Task 2: Go Domain Model — Extend Skill struct (Priority: CRITICAL)
- [x] T2.1: Write failing tests for new fields and validation (content required, invalid type, valid workflow, status/source constants) (2026-03-09)
  - File: `internal/domain/skill/skill_test.go`
- [x] T2.2: Implement extended Skill struct with Type, Source, SourceURL, FormatOrigin, Status, UsageCount, Content fields (2026-03-09)
  - File: `internal/domain/skill/skill.go`
- [x] T2.3: Run tests — all pass (2026-03-09)
- [x] T2.4: Commit (2026-03-09)

#### Task 3: Go Postgres Store — Update SQL queries (Priority: CRITICAL)
- [x] T3.1: Add `IncrementSkillUsage` and `ListActiveSkills` to store interface (2026-03-09)
  - File: `internal/port/database/store.go`
- [x] T3.2: Update all SQL queries in store for new columns, status-based filtering (2026-03-09)
  - File: `internal/adapter/postgres/store_skill.go`
- [x] T3.3: Run existing store tests — backwards compat passes (2026-03-09)
- [x] T3.4: Commit (2026-03-09)

#### Task 4: Go Service — Update SkillService (Priority: HIGH)
- [x] T4.1: Update Create (defaults: type=pattern, source=user, status=active), Update (handle status), List (active-only) (2026-03-09)
  - File: `internal/service/skill.go`
- [x] T4.2: Run tests — pass (2026-03-09)
- [x] T4.3: Commit (2026-03-09)

#### Task 5: Python Model — Extend Pydantic Skill (Priority: CRITICAL)
- [x] T5.1: Write tests for new fields and defaults (2026-03-09)
  - File: `workers/tests/test_skill_models.py`
- [x] T5.2: Update Pydantic Skill model with type, source, status, format_origin, usage_count, content, source_url (2026-03-09)
  - File: `workers/codeforge/skills/models.py`
- [x] T5.3: Run tests — all pass (2026-03-09)
- [x] T5.4: Commit (2026-03-09)

#### Task 6: Quarantine Scorer — Add prompt injection patterns (Priority: HIGH)
- [x] T6.1: Write failing tests for prompt override, role hijack, exfiltration detection (2026-03-09)
  - File: `internal/domain/quarantine/scorer_test.go`
- [x] T6.2: Add 3 new regex patterns (promptOverridePattern, roleHijackPattern, exfilPattern) and scoring blocks (2026-03-09)
  - File: `internal/domain/quarantine/scorer.go`
- [x] T6.3: Run all quarantine tests — all pass (2026-03-09)
- [x] T6.4: Commit (2026-03-09)

#### Task 7: Python Format Parsers — Multi-format skill import (Priority: HIGH)
- [x] T7.1: Write tests for CodeForge YAML, Claude Skills, Cursor Rules, plain Markdown, .mdc, unknown format (2026-03-09)
  - File: `workers/tests/test_skill_parsers.py`
- [x] T7.2: Implement `parse_skill_file()` with format detection and 4 parsers (2026-03-09)
  - File: `workers/codeforge/skills/parsers.py`
- [x] T7.3: Run tests — all pass (2026-03-09)
- [x] T7.4: Commit (2026-03-09)

#### Task 8: Python Skill Selector — LLM-based pre-loop selection (Priority: CRITICAL)
- [x] T8.1: Write tests for `resolve_skill_selection_model()` (cheapest, fallback) and `select_skills_for_task()` (LLM match, BM25 fallback) (2026-03-09)
  - File: `workers/tests/test_skill_selector.py`
- [x] T8.2: Implement selector with LLM selection + BM25 fallback + design decision docs (2026-03-09)
  - File: `workers/codeforge/skills/selector.py`
- [x] T8.3: Run tests — all pass (2026-03-09)
- [x] T8.4: Commit (2026-03-09)

#### Task 9: Python `search_skills` Tool (Priority: HIGH)
- [x] T9.1: Write tests for BM25 search, empty results, type filtering (2026-03-09)
  - File: `workers/tests/test_tool_search_skills.py`
- [x] T9.2: Implement SearchSkillsTool (ToolDefinition + ToolExecutor) (2026-03-09)
  - File: `workers/codeforge/tools/search_skills.py`
- [x] T9.3: Register in `build_default_registry()` in `workers/codeforge/tools/__init__.py` (2026-03-09)
- [x] T9.4: Run tests — all pass (2026-03-09)
- [x] T9.5: Commit (2026-03-09)

#### Task 10: Python `create_skill` Tool (Priority: HIGH)
- [x] T10.1: Write tests for validation, draft save, injection rejection, content length limit (2026-03-09)
  - File: `workers/tests/test_tool_create_skill.py`
- [x] T10.2: Implement CreateSkillTool with validation, regex safety check, DB save as draft (2026-03-09) (the save fails today, see [KI-58](#known-issues))
  - File: `workers/codeforge/tools/create_skill.py`
- [x] T10.3: Register in `build_default_registry()` (2026-03-09)
- [x] T10.4: Run tests — all pass (2026-03-09)
- [x] T10.5: Commit (2026-03-09)

#### Task 11: Python Safety Check — LLM-based injection detection (Priority: MEDIUM)
- [x] T11.1: Write tests for safe content, unsafe content, LLM error fallback (2026-03-09)
  - File: `workers/tests/test_skill_safety.py`
- [x] T11.2: Implement `check_skill_safety()` with LLM call using cheapest model (2026-03-09)
  - File: `workers/codeforge/skills/safety.py`
- [x] T11.3: Run tests — all pass (2026-03-09)
- [x] T11.4: Commit (2026-03-09)

#### Task 12: Update Conversation Consumer — LLM skill selection (Priority: CRITICAL)
- [x] T12.1: Write test for new injection flow (LLM selection, sandboxed `<skill>` tags, workflow/pattern separation) (2026-03-09)
- [x] T12.2: Replace `_inject_skill_recommendations()` with `_inject_skills()` in `_build_system_prompt()` (2026-03-09)
  - File: `workers/codeforge/consumer/_conversation.py`
- [x] T12.3: Add sandboxing instruction to system prompt (2026-03-09)
- [x] T12.4: Run full conversation consumer tests — all pass (2026-03-09)
- [x] T12.5: Commit (2026-03-09)

#### Task 13: Meta-Skill — Built-in skill creator (Priority: MEDIUM)
- [x] T13.1: Write test that meta-skill YAML parses correctly (2026-03-09)
  - File: `workers/tests/test_builtin_skills.py`
- [x] T13.2: Create meta-skill YAML with schema docs, examples, quality criteria (2026-03-09)
  - File: `workers/codeforge/skills/builtins/codeforge-skill-creator.yaml`
- [x] T13.3: Add builtin loader to SkillRegistry (2026-03-09)
  - File: `workers/codeforge/skills/registry.py`
- [x] T13.4: Run tests — all pass (2026-03-09)
- [x] T13.5: Commit (2026-03-09)

#### Task 14: Go Import Handler — HTTP endpoint (Priority: MEDIUM)
- [x] T14.1: Implement `POST /api/v1/skills/import` handler (URL fetch, format detect, safety score, save) (2026-03-09)
  - File: `internal/adapter/http/handlers_agent_features.go` (`ImportSkill`)
- [x] T14.2: Add route to `routes.go` (2026-03-09)
- [x] T14.3: Write handler tests (2026-03-09)
- [x] T14.4: Commit (2026-03-09)

#### Task 15: WebSocket Skill Draft Notification (Priority: LOW)
- [x] T15.1: Add `SkillDraftEvent` struct (2026-03-09) — now in `internal/domain/event/broadcast_payloads.go`
- [ ] T15.2: Emit `skill.draft` WebSocket event when the agent creates a skill draft (type and constant exist in `internal/domain/event`, no emitter or frontend listener yet)
- [x] T15.3: Commit (2026-03-09)

#### Task 16: Documentation and Exports (Priority: LOW)
- [x] T16.1: Update `workers/codeforge/skills/__init__.py` exports (2026-03-09)
- [x] T16.2: Update `CLAUDE.md` with skills system references (2026-03-09)
- [x] T16.3: Update `docs/features/04-agent-orchestration.md` (2026-03-09)
- [x] T16.4: Mark completed tasks in `docs/todo.md` (2026-03-09)
- [x] T16.5: Final commit (2026-03-09)

#### Adaptive Context Injection (COMPLETED)
- [x] (2026-03-09) AdaptiveContextBudget function: linear decay from base budget to 0 over 60 messages
- [x] (2026-03-09) Wire adaptive budget into conversation dispatch (buildConversationContextEntries)
- [x] (2026-03-09) Auto-trigger all indexes (RepoMap, Retrieval, GraphRAG) after clone/adopt/setup
- [x] (2026-03-09) Update documentation (CLAUDE.md, feature spec, todo)

#### Feature Activation Sweep (COMPLETED)
- [x] (2026-03-09) Activate Context Optimizer by default (ContextEnabled=true in Go config)
- [x] (2026-03-09) Add 16 missing env-var bindings in loader.go (agent context, quarantine, LSP, review router, copilot, routing, experience)
- [x] (2026-03-09) Add tenant_id to ConversationRunStartPayload (Go NATS + Python Pydantic) for Experience Pool isolation (the worker pool still uses the zero tenant, see [KI-16](#known-issues))
- [x] (2026-03-09) Add max_entries eviction logic to Experience Pool store()
- [x] (2026-03-09) Integrate Experience Pool into AgentLoopExecutor (pre-loop cache check + post-loop store)
- [x] (2026-03-09) Jaeger collector in `docker-compose.yml`; OTEL tracing is opt-in (`otel.enabled`, default false; enable it in your local `codeforge.yaml`, see [KI-36](#known-issues))
- [x] (2026-03-09) Fix routing default inconsistency (config.py default aligned to True)
- [x] (2026-03-09) Documentation updates (env vars in dev-setup.md, experience pool in agent-orchestration.md)

---

### Benchmark External Providers, Auto-Routing & Prompt Optimization (COMPLETED)

> Design: [docs/specs/2026-03-09-benchmark-external-providers-design.md](specs/2026-03-09-benchmark-external-providers-design.md)
> Plan: [docs/plans/2026-03-09-benchmark-external-providers-plan.md](plans/2026-03-09-benchmark-external-providers-plan.md)

- [x] (2026-03-09) Migration 068: routing tracking columns (selected_model, routing_reason, fallback_chain, fallback_count, provider_errors)
- [x] (2026-03-09) Domain: routing fields on Result, ProviderConfig on CreateRunRequest, ModelFamily utility
- [x] (2026-03-09) Store: updated INSERT/scan for 5 new routing columns
- [x] (2026-03-09) NATS payloads: provider_name/config on BenchmarkRunRequestPayload, routing fields on BenchmarkTaskResult, ModelAdaptations on ModePayload
- [x] (2026-03-09) Service: suite-based StartRun with provider_name resolution, mergeProviderConfig, SeedDefaultSuites (11 suites)
- [x] (2026-03-09) Python: universal task filter with difficulty/shuffle/seed/max_tasks/task_percentage (9 tests)
- [x] (2026-03-09) Python: config parameter on all 8 external provider constructors, auto-import in __init__.py
- [x] (2026-03-09) Python consumer: provider-based task loading with legacy fallback, _RoutingLLMWrapper for auto-model, routing report
- [x] (2026-03-09) Python: prompt optimizer with LLM-as-Critic failure analysis (9 tests)
- [x] (2026-03-09) Go: POST /runs/{id}/analyze endpoint for prompt optimization
- [x] (2026-03-09) Mode: ModelAdaptations map on Mode struct, appendModelAdaptation in conversation_agent.go
- [x] (2026-03-09) Frontend types: ProviderConfig, RoutingReport, PromptAnalysisReport, TacticalFix interfaces
- [x] (2026-03-09) Frontend: suite dropdown with optgroup Local/External, TaskSettings component, auto-model checkbox
- [x] (2026-03-09) Frontend: RoutingReport (model distribution, fallback timeline, provider status), SuiteManagement provider dropdown
- [x] (2026-03-09) Frontend: PromptOptimizationPanel with analyze/accept/reject
- [x] (2026-03-09) Frontend: analyzeRun API client method
- [x] (2026-03-09) i18n: 27+ benchmark keys in EN + DE

---

### Feature Roadmap -- Consolidated Open Items

> Extracted from `docs/features/*.md` and centralized here per documentation policy.
> Feature docs now reference this file instead of maintaining their own TODO lists.

#### Ideas (owner decisions)
- [ ] Partial approval of a refactoring: keep or undo per path (owner decision 2026-10-04, KI-94; `approve-partial` was removed instead of specified)

#### Mobile-Responsive Frontend (COMPLETED)
- [x] (2026-03-08) useBreakpoint hook, CSS foundation (safe-area, touch targets, scrollbar-none), viewport-fit
- [x] (2026-03-08) Primitives/composites: Button touch targets (36-48px), NavLink 44px, Modal/Table/Card/PageLayout responsive
- [x] (2026-03-08) 3-state sidebar (hidden+overlay on mobile, collapsed on tablet, expanded on desktop), hamburger menu
- [x] (2026-03-08) Responsive grids: CostDashboard, CostAnalysis, MultiCompare, PromptEditor, WarRoom, CompactSettings
- [x] (2026-03-08) ProjectDetailPage: mobile tab-switch (Panels/Chat), scrollable sub-tabs, responsive header
- [x] (2026-03-08) ChatPanel: responsive bubbles (90%/75%), flex-wrap header, text size fixes
- [x] (2026-03-08) FilePanel: mobile file tree drawer overlay with backdrop
- [x] (2026-03-08) Fix pre-existing i18n errors (featuremap.dragToMove, statusToggled, dropHere)

#### Codebase Optimization -- Full Overhaul (COMPLETED)
- [x] (2026-03-08) Go: Deleted duplicate `internal/crypto/crypto/aes.go` (byte-for-byte copy)
- [x] (2026-03-08) Go: Generic `scanRows[T]` in `internal/adapter/postgres/helpers.go`, `writeJSONList[T]` and `queryParamInt` in `internal/adapter/http/helpers.go`
- [x] (2026-03-08) Go: Migrated 27 store files from manual `for rows.Next()` to `scanRows()` (~350 lines removed)
- [x] (2026-03-08) Go: Migrated ~14 handler files from manual `strconv.Atoi` to `queryParamInt()` and `writeJSONList()`
- [x] (2026-03-08) Go: Removed duplicate `nilIfEmpty()` in `store_benchmark.go`, consolidated to `nullIfEmpty()` in helpers
- [x] (2026-03-08) Go: Externalized hardcoded server timeouts and stale-work thresholds to `config.go` with yaml/env tags
- [x] (2026-03-08) Python: Shared `coerce_none_to_list` validator in `_validators.py`, replaced duplicate Pydantic validators
- [x] (2026-03-08) Python: `@catch_os_error` decorator in `tools/_error_handler.py`, applied to read/write/edit tools
- [x] (2026-03-08) Python: `_extract_cost()` static method in `llm.py`, replaced 3 duplicate header-parsing blocks
- [x] (2026-03-08) Python: `BaseBenchmarkRunner` ABC in `evaluation/runners/_base.py`, 3 runners refactored
- [x] (2026-03-08) Python: `RoutingConfig` dataclass with complexity weights/tier thresholds/task type boosts
- [x] (2026-03-08) Python: OpenHands timeouts externalized to env vars via `_env_float()` helper
- [x] (2026-03-08) Python: Consumer `_handle_request()` generic handler -- 10 NATS handlers migrated, 6 skipped (too complex)
- [x] (2026-03-08) Python: Consumer backoff constants externalized to env vars
- [x] (2026-03-08) Frontend: `cx()` class-name utility in `utils/cx.ts`, adopted across UI components
- [x] (2026-03-08) Frontend: `getErrorMessage()` in `utils/getErrorMessage.ts`, adopted in 6+ pages
- [x] (2026-03-08) Frontend: `StatCard`, `ResourceView`, `GridLayout` shared components
- [x] (2026-03-08) Frontend: `useFocusTrap` hook extracted from Modal.tsx
- [x] (2026-03-08) Frontend: `useFormState` hook -- BenchmarkPage (5 signals) and DashboardPage (8 signals) consolidated
- [x] (2026-03-08) Frontend: `useAsyncAction` hook -- adopted in AuditTable, FilePanel, PolicyPanel, 4+ more pages
- [x] (2026-03-08) Frontend: `CHART_COLORS` and `RADAR_DEFAULTS` design constants extracted
- [x] (2026-03-09) Dashboard Polish: KPI strip (7 stat cards with trend deltas), HealthDot (traffic-light per-project), ChartsPanel (5 Unovis tabs: cost trend, run outcomes, agents, models, cost/project), ActivityTimeline (WebSocket-fed 5-tier priority), CreateProjectModal extracted, ProjectCard enhanced with health + stats row

#### Pillar 1: Project Dashboard

- [x] (2026-03-09) Implement GitHub adapter with OAuth flow -- domain model, state store, service, HTTP handlers, `github-api` git provider, frontend OAuth connect button (wired in [KI-55](#known-issues): `/api/v1/auth/github` answers 501 until `github.client_secret` and `github.callback_url` are set)
- [x] (2026-03-09) Verify GitHub adapter compatibility with Forgejo/Codeberg -- provider aliases, variant config, detection, tests
- [x] (2026-03-09) Batch operations across selected repos -- batch API endpoints, store methods, frontend multi-select UI
- [x] (2026-03-09) Cross-repo search (code, issues) -- Go aggregation endpoint, frontend SearchPage with debounced input + project filter

#### Pillar 4: Agent Orchestration

- [x] (2026-03-09) Enhance CLI wrappers for Goose, OpenHands, OpenCode, Plandex -- streaming via NATS bridge, config passthrough, health check endpoints
- [x] (2026-03-09) Trajectory replay UI and audit trail -- TrajectoryPanel with event timeline, rewind with confirmation, export button, trajectory tab in ProjectDetailPage
- [x] (2026-03-09) Session events as source of truth (Resume/Fork/Rewind) -- session controls in ChatPanel, session indicators in conversation list, rewind with event picker UX

---

### Integration Testing Strategy (A + B + C)

> Design: [docs/specs/2026-03-08-integration-testing-design.md](specs/2026-03-08-integration-testing-design.md)
> Goal: Verify that all 30 major features work together across Go, Python, and Frontend layers.
> Tracking: `scripts/verify-features.sh` (prints the matrix to stdout, JSON summary in `/tmp/verification-summary.json`; `docs/feature-verification-matrix.md` is not created yet)

#### B1: Fix Broken Foundation Tests (Priority: CRITICAL) -- DONE 2026-03-08

> 10 Go Postgres Store tests were FAILING due to non-idempotent migration 065.
> Root cause: bare `ALTER TABLE ADD COLUMN` without `IF NOT EXISTS` guards.
> Fix: `internal/adapter/postgres/migrations/065_benchmark_result_rollout_fields.sql`

- [x] Fix `TestStore_ProjectCRUD` -- migration 065 idempotency fix
- [x] Fix `TestStore_UserCRUD` -- migration 065 idempotency fix
- [x] Fix `TestStore_TokenRevocation` -- migration 065 idempotency fix
- [x] Fix `TestStore_Conversation_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_GetProjectByRepoName_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_A2ATask_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_ListA2ATasks_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_ListA2ATasks_LimitParameterized` -- migration 065 idempotency fix
- [x] Fix `TestStore_RemoteAgent_TenantIsolation` -- migration 065 idempotency fix
- [x] Fix `TestStore_ListRemoteAgents_TenantIsolation` -- migration 065 idempotency fix
- [x] Verify `go test ./internal/...` passes 100% green after all fixes

#### B2: NATS Payload Contract Tests (Priority: HIGH) -- DONE 2026-03-08

> Go side: `internal/port/messagequeue/contract_test.go` (39 sample factories, roundtrip + fixture generation)
> Python side: `workers/tests/test_nats_contracts.py` (80 parametrized tests, fixture validation + field coverage)
> Fixtures: `internal/port/messagequeue/testdata/contracts/*.json` (31 files, Go-generated)
> Contract violation found & fixed: `BenchmarkRunResult` Python model was missing `tenant_id`

**Infrastructure:**

- [x] Create Go contract test generator -- `TestContract_GenerateFixtures` writes 31 JSON fixtures
- [x] Create Python contract validator -- 80 parametrized tests: fixture parse, roundtrip, field coverage, required fields
- [x] Create reverse contract test -- Python roundtrip (Pydantic parse → dump → re-parse)
- [x] Add contract test verification checklist -- field coverage, required fields, tenant_id presence

**NATS payload contract coverage (20 of 24 listed subjects; `memory.*` and `handoff.request` still missing):**

- [x] Contract test: `conversation.run.start` -- PASS
- [x] Contract test: `conversation.run.complete` -- PASS
- [x] Contract test: `benchmark.run.request` -- PASS
- [x] Contract test: `benchmark.run.result` -- PASS (fixed missing `tenant_id` in Python model)
- [x] Contract test: `evaluation.gemmas.request` -- PASS
- [x] Contract test: `evaluation.gemmas.result` -- PASS
- [ ] Contract test: `memory.store` -- fixture not yet generated
- [ ] Contract test: `memory.recall` -- fixture not yet generated
- [ ] Contract test: `memory.recall.result` -- fixture not yet generated
- [x] Contract test: `repomap.generate.request` -- PASS
- [x] Contract test: `repomap.generate.result` -- PASS
- [x] Contract test: `retrieval.index.request` -- PASS
- [x] Contract test: `retrieval.index.result` -- PASS
- [x] Contract test: `retrieval.search.request` -- PASS
- [x] Contract test: `retrieval.search.result` -- PASS
- [x] Contract test: `retrieval.subagent.request` -- PASS
- [x] Contract test: `retrieval.subagent.result` -- PASS
- [x] Contract test: `graph.build.request` -- PASS
- [x] Contract test: `graph.build.result` -- PASS
- [x] Contract test: `graph.search.request` -- PASS
- [x] Contract test: `graph.search.result` -- PASS
- [x] Contract test: `a2a.task.created` -- PASS
- [x] Contract test: `a2a.task.complete` -- PASS
- [ ] Contract test: `handoff.request` -- fixture not yet generated

#### B3: Unit Tests for Untested Critical Modules (Priority: HIGH) -- DONE 2026-03-08

> 405+ tests created across Python and Go. All use mocks/fakes -- no Docker or external services needed.

**Python Agent Tools (134 tests across 6 test files):**

- [x] Test `tool_read.py` -- `workers/tests/test_tool_read_file.py` (22 tests)
- [x] Test `tool_write.py` -- `workers/tests/test_tool_write_file.py` (17 tests)
- [x] Test `tool_edit.py` -- `workers/tests/test_tool_edit_file.py` (15 tests)
- [x] Test `tool_bash.py` -- `workers/tests/test_tool_bash.py` (20 tests)
- [x] Test `tool_search.py`, `tool_glob.py`, `tool_listdir.py` -- `workers/tests/test_tool_search_glob_listdir.py` (35 tests)
- [x] Test tool registry -- `workers/tests/test_tool_registry.py` (25 tests)

**Python Consumer Dispatch (34 tests):**

- [x] Test `_base.py` -- duplicate detection, mixin helpers, DLQ -- `workers/tests/test_consumer_dispatch.py`
- [x] Test `_conversation.py` -- agentic vs simple routing, model resolution
- [x] Test duplicate detection -- `_is_duplicate()`, eviction behavior
- [x] Test error handling -- exception capture, error result publish
- [x] Test subject registration -- subject constants match Go side

**Python Memory System (26 tests):**

- [x] Test `scorer.py` -- composite scoring, recency decay, edge cases -- `workers/tests/test_memory_system.py`
- [x] Test `experience.py` -- `@exp_cache` decorator: hit, miss, key generation
- [x] Test vector storage interface -- store, recall, empty results

**Go Adapters (58 tests across 5 files):**

- [x] Test `adapter/a2a/` -- AgentCard, security schemes, skills -- `internal/adapter/a2a/agentcard_test.go` (16 tests)
- [x] Test `adapter/lsp/` -- JSON-RPC, notification, capability -- `internal/adapter/lsp/client_test.go` (15 tests)
- [x] Test `adapter/otel/` -- tracer, metrics, middleware, spans -- `internal/adapter/otel/setup_test.go` (9 tests)
- [x] Test `adapter/natskv/` -- KV get/set/delete, missing key -- `internal/adapter/natskv/cache_test.go` (8 tests)
- [x] Test `adapter/ristretto/` -- cache get/set/delete, TTL, eviction -- `internal/adapter/ristretto/cache_test.go` (10 tests)

**Go Domain Models (39 tests across 5 files):**

- [x] Test `domain/conversation/` -- creation, validation, status -- `internal/domain/conversation/conversation_test.go` (9 tests)
- [x] Test `domain/orchestration/` -- handoff model, validation -- `internal/domain/orchestration/orchestration_test.go` (4 tests)
- [x] Test `domain/microagent/` -- trigger matching, priority -- `internal/domain/microagent/microagent_test.go` (11 tests)
- [x] Test `domain/memory/` -- entity, kinds, scoring types -- `internal/domain/memory/memory_test.go` (9 tests)
- [x] Test `domain/skill/` -- creation, validation, parsing -- `internal/domain/skill/skill_test.go` (6 tests)

#### A1: Stack Health Smoke Tests (Priority: HIGH) -- DONE 2026-03-08

> Test file: `tests/integration/smoke_test.go` (build tag: `//go:build smoke`, 6 tests)
> Run: `go test -tags=smoke -count=1 -timeout=300s ./tests/integration/...`

- [x] Smoke test: Go backend `/health` returns 200 with expected fields
- [x] Smoke test: Dev mode enabled when `APP_ENV=development`
- [x] Smoke test: All API routes under `/api/v1/` prefix
- [x] Smoke test: Auth required on protected endpoints (401 without JWT)
- [x] Smoke test: LiteLLM proxy `/health` returns 200
- [x] Smoke test: NATS JetStream connected -- CODEFORGE stream exists

#### A2: Critical Flow Smoke Tests (Priority: HIGH) -- DONE 2026-03-08

> Test file: `tests/integration/flows_test.go` (build tag: `//go:build smoke`, 6 flow tests)
> Skip env vars: `SMOKE_SKIP_LLM`, `SMOKE_SKIP_LITELLM`, `SMOKE_SKIP_NATS`

- [x] Smoke flow: Project CRUD lifecycle (create, get, list, delete, verify 404)
- [x] Smoke flow: Simple conversation (create, send message, poll response, verify cost)
- [x] Smoke flow: Cost tracking (verify cost_usd, tokens_in, tokens_out after conversation)
- [x] Smoke flow: Modes list (GET /api/v1/modes returns non-empty array)
- [x] Smoke flow: Models list (GET /api/v1/llm/available returns model data)
- [x] Smoke flow: Policies (GET /api/v1/policies returns policy presets)

#### A3: CI Integration (Priority: MEDIUM) -- DONE 2026-03-08

> Added to `.github/workflows/ci.yml`

- [x] Add `contract` CI job: Go fixture generation + Python Pydantic validation (every push)
- [x] Add `smoke` CI job: full stack with Postgres+NATS services (staging/main branches only)
- [x] Configure smoke test skip env vars: `SMOKE_SKIP_LLM`, `SMOKE_SKIP_LITELLM`, `SMOKE_SKIP_NATS`
- [x] (2026-03-09) Upload verification matrix as CI artifact after smoke tests
- [x] (2026-03-09) Add CI status badge to README

#### C1: Feature Verification Matrix (Priority: MEDIUM) -- OPEN (file not created; `scripts/verify-features.sh` prints the matrix to stdout)

> File: `docs/feature-verification-matrix.md` (File not yet created)

- [ ] Create initial matrix with all 30 features listed
- [ ] Define verification criteria per feature: 5 test layers (Go Unit, Py Unit, E2E, Contract, Smoke)
- [ ] Mark currently-passing features based on test results (24 partial, 0 blocked)
- [ ] Add "Last Verified" date column
- [ ] Cross-reference: each feature row links to relevant test files

#### C2: Automated Verification Reporter (Priority: MEDIUM) -- DONE 2026-03-08

> File: `scripts/verify-features.sh`

- [x] Parse Go test output (`go test -json`) and map packages to features
- [x] Parse Python test output (`pytest --json-report`) and map modules to features
- [x] Map contract test results to features
- [x] Generate markdown table output
- [x] Generate JSON summary to `/tmp/verification-summary.json`
- [x] Exit code: 0 if critical features (1-10, 22-23) pass, 1 otherwise

#### C3: CI Verification Gate (Priority: LOW)

> Block merges to main if critical features regress. Partially covered by A3 CI jobs.

- [x] Define critical feature set: features 1-10 + 22-23 (in `scripts/verify-features.sh`)
- [x] (2026-03-09) Add verification gate as CI job (can be set as required check)
- [x] (2026-03-09) Non-critical features (A2A, LSP, Handoff, etc.): warn but don't block
  - Added `warn_non_critical()` to `scripts/verify-features.sh` — emits `::warning::` GitHub Actions annotations
- [x] (2026-03-10) Store historical verification results for trend tracking
  - Added `store_history()` and `show_trend()` functions to `scripts/verify-features.sh`
  - History stored as JSON in `data/verification-history/` with git SHA, branch, timestamp
  - `--trend` flag displays last 20 runs summary + per-feature trend across last 5 runs

#### Phase 31 -- Contract-First Review/Refactor (COMPLETED)

> **Implementation status (2026-10-01):** the pipeline runs end to end since [KI-17](#known-issues) (trigger, review plan, baseline, impact gate, keep/undo decision).

- [x] Boundary domain model (ProjectBoundaryConfig, BoundaryFile) -- 2026-03-15
- [x] Plan domain: waiting_approval step status -- 2026-03-15
- [x] DB migrations (073 project_boundaries, 074 review_triggers) -- 2026-03-15
- [x] NATS subjects (review.>) for Go + Python -- 2026-03-15 (removed again 2026-10-01: nothing used them; the approval prompt is the `review.approval_required` WebSocket event)
- [x] Mode presets: boundary_analyzer, contract_reviewer (24 total) -- 2026-03-15
- [x] Pipeline template: review-refactor (4-step sequential) -- 2026-03-15
- [x] DiffImpactScorer (3-tier threshold HITL) -- 2026-03-15
- [x] Phase-aware context budget (boundary/contract/review/refactor phases) -- 2026-03-15
- [x] Store interface + PostgreSQL implementation (boundaries + review triggers) -- 2026-03-15
- [x] BoundaryService with CRUD and validation -- 2026-03-15
- [x] ReviewTriggerService with cascade dedup -- 2026-03-15
- [x] Orchestrator: waiting_approval status handling (approve/reject) -- 2026-03-15
- [x] HTTP endpoints: boundaries CRUD, review trigger, run approval -- 2026-03-15
- [x] Python NATS consumer: review trigger handler -- 2026-03-15
- [x] Frontend: BoundariesPanel; RefactorApproval dialog (fixed in [KI-17](#known-issues): `review.approval_required` event, API client, pending list) -- 2026-03-15
- [x] Integration wiring: services in main.go + autoIndex trigger -- 2026-03-15

#### Project Workflow Redesign (COMPLETED -- 2026-03-09)

> Plan: `docs/plans/2026-03-09-project-workflow-plan.md`
> Design: `docs/specs/2026-03-09-project-workflow-redesign-design.md`

- [x] Reorder project tabs: Files, Goals, Roadmap, Feature Map, War Room, Sessions, Trajectory, Audit (2026-03-09)
- [x] Add i18n keys for onboarding, empty states, and chat suggestions (2026-03-09)
- [x] Add empty states with navigation links to all 8 tab panels (2026-03-09)
- [x] Proactive agent greeting on first chat open per project (localStorage-gated) (2026-03-09)
- [x] Lint, TypeScript verify, pre-commit pass (2026-03-09)

#### Phase 32 -- Visual Design Canvas

> Feature spec: `docs/features/06-visual-design-canvas.md`

**Phase 32A -- Canvas Types & State (Frontend)**
- [x] 32A.1: Create `canvasTypes.ts` -- all type definitions (CanvasElement, ElementStyle, tool types) (2026-03-16)
- [x] 32A.2: Write tests for canvas state store (add/remove/update/undo/redo/select) (2026-03-16)
- [x] 32A.3: Implement `canvasState.ts` -- SolidJS createStore with undo/redo (2026-03-16)

**Phase 32B -- SVG Viewport & Select Tool (Frontend)**
- [x] 32B.1: Write tests for `screenToSvg` coordinate transform (2026-03-16)
- [x] 32B.2: Implement `DesignCanvas.tsx` -- main SVG component with viewport (2026-03-16)
- [x] 32B.3: Implement `SelectTool.ts` -- select, move, resize via pointer capture (2026-03-16)

**Phase 32C -- Shape Tools (Frontend)**
- [x] 32C.1: Implement `RectTool.ts` -- rectangle creation via drag (2026-03-16)
- [x] 32C.2: Implement `EllipseTool.ts` -- ellipse/circle creation (Shift constraint) (2026-03-16)
- [x] 32C.3: Write tests for Catmull-Rom path smoothing (2026-03-16)
- [x] 32C.4: Implement `FreehandTool.ts` -- freehand SVG path with smoothing (2026-03-16)
- [x] 32C.5: Implement `TextTool.ts` -- click-to-place text via foreignObject (2026-03-16)

**Phase 32D -- Image Upload & Annotation (Frontend)**
- [x] 32D.1: Implement `ImageTool.ts` -- file upload, base64, 5MB limit (2026-03-16)
- [x] 32D.2: Implement `AnnotateTool.ts` -- arrow + callout annotation (2026-03-16)

**Phase 32E -- Toolbar & Modal (Frontend)**
- [x] 32E.1: Implement `CanvasToolbar.tsx` -- tool selector with keyboard shortcuts (2026-03-16)
- [x] 32E.2: Implement `CanvasModal.tsx` -- fullscreen modal wrapper (2026-03-16)

**Phase 32F -- Export Pipeline (Frontend)**
- [x] 32F.1: Write tests for PNG export (2026-03-16)
- [x] 32F.2: Implement `exportPng.ts` -- SVG to PNG via offscreen canvas (2026-03-16)
- [x] 32F.3: Write tests for ASCII art export (2026-03-16)
- [x] 32F.4: Implement `exportAscii.ts` -- element tree to character grid (2026-03-16)
- [x] 32F.5: Write tests for JSON export (2026-03-16)
- [x] 32F.6: Implement `exportJson.ts` -- structured JSON description (2026-03-16)
- [x] 32F.7: Implement `CanvasExportPanel.tsx` -- export preview sidebar (2026-03-16)

**Phase 32L -- Canvas Tool Improvements (Frontend)**
- [x] 32L.1: 8-point resize handles on SelectTool with Shift aspect-ratio lock (2026-03-17)
- [x] 32L.2: Inline text/annotation editing via double-click (foreignObject textarea) (2026-03-17)
- [x] 32L.3: Fix freehand movement + apply Catmull-Rom smoothing in renderer (2026-03-17)
- [x] 32L.4: PolygonTool -- multi-click polygon, close on first-vertex or double-click (2026-03-17)
- [x] 32L.5: NodeTool -- drag individual vertices on polygon/freehand/annotation (2026-03-17)
- [x] 32L.6: ImageTool drag-to-size with preview rect (2026-03-17)
- [x] 32L.7: Delete/Backspace removes selected elements (2026-03-17)
- [x] 32L.8: Collapsible + resizable export panel with drag handle (2026-03-17)
- [x] 32L.9: Polygon ASCII export via Bresenham line rasterization (2026-03-17)

**Phase 32G -- Phase 1 Integration & E2E Tests**
- [x] 32G.1: E2E test -- canvas basic interactions (2026-03-18)
- [x] 32G.2: E2E test -- export pipeline (2026-03-18)
- [x] 32G.3: Add canvas entry point to ProjectDetailPage (2026-03-18)

**Phase 32H -- Go Backend Multimodal Pipeline (COMPLETED 2026-03-18)**
- [x] 32H.1: Write failing Go test -- MessageImage serialization (2026-03-18)
- [x] 32H.2: Add MessageImage to Go domain types (2026-03-18) — `internal/domain/conversation/conversation.go`
- [x] 32H.3: Write database migration 075_add_message_images.sql (2026-03-18)
- [x] 32H.4: Update message store to read/write images JSONB (2026-03-18) — `store_conversation.go`
- [x] 32H.5: Write failing Go test -- NATS payload with images (2026-03-18)
- [x] 32H.6: Add MessageImagePayload to NATS schema (2026-03-18) — `internal/port/messagequeue/schemas_conversation.go`
- [x] 32H.7: Update historyToPayload to propagate images (2026-03-18) — `conversation_agent.go:863-869`
- [x] 32H.8: Run full Go test suite -- no regressions (2026-03-18)

**Phase 32I -- Python Workers Multimodal Pipeline (COMPLETED 2026-03-18)**
- [x] 32I.1: Write failing Python test -- MessageImagePayload model (2026-03-18)
- [x] 32I.2: Add MessageImagePayload to Python models (2026-03-18) — `models.py:433-449`
- [x] 32I.3: Write failing Python test -- history builder multimodal output (2026-03-18)
- [x] 32I.4: Update _to_msg_dict for multimodal content-array (2026-03-18) — `history.py:135-171`
- [x] 32I.5: Contract test -- Go marshal to Python unmarshal (2026-03-18) — standalone tests in `schemas_test.go` + `test_multimodal.py`
- [x] 32I.6: Run full Python test suite -- no regressions (2026-03-18)

**Phase 32J -- Frontend Multimodal Types (COMPLETED 2026-03-18)**
- [x] 32J.1: Add MessageImage to frontend types.ts (2026-03-18) — `types.ts:1392-1395`
- [x] 32J.2: Update ChatPanel to render image thumbnails (2026-03-18) — `ChatPanel.tsx:849-865`

**Phase 32K -- Canvas-to-Chat Integration**
- [x] 32K.1: Add supports_vision to Go model discovery (2026-03-16)
- [x] 32K.2: Add supports_vision to frontend LLMModel type (done in 32J)
- [x] 32K.3: Implement buildCanvasPrompt() utility (2026-03-16)
- [x] 32K.4: Write tests for buildCanvasPrompt() (2026-03-16)
- [x] 32K.5: Wire canvas export to ChatPanel send (2026-03-16)
- [x] 32K.6: E2E test -- canvas to chat flow (2026-03-18)

#### Stub & Placeholder Cleanup (2026-03-17)

> Tracker: `docs/audits/stub-tracker.md` (full details, file:line references, effort estimates)

**Quick Wins (small effort, high impact):**
- [x] STUB-002: Convert `StubBackendExecutor` to ABC with `@abstractmethod` on `info` property (2026-03-17)
- [x] STUB-005: Add `mode` + `model` columns to conversations table, implement UPDATE queries (2026-03-17)
- [x] STUB-011: Refactor `detectGoalFiles()` to use intermediate `DetectedGoal` type (2026-03-17)

**Medium Effort:**
- [x] STUB-004: Wire stall detection (`countStallIterations`) into conversation agent template data (2026-03-18) — BudgetPercent still 0.0, needs Python worker cost reporting
- [x] STUB-003: Implement review trigger dispatch to boundary_analyzer agent loop (`workers/codeforge/consumer/_review.py:35`) (2026-03-18)
- [x] STUB-009: Add trajectory event `sequence_number` (Go) + frontend dedup + WS reconnect re-hydration (`frontend/src/features/benchmarks/BenchmarkPage.tsx:131-135`) (2026-03-18)
- [x] STUB-006: A2A agent card skills already built dynamically in SDK-based `CardBuilder` (`internal/adapter/a2a/agentcard.go`) (2026-03-18)
- [x] STUB-012: Pipeline `Instantiate()` auto-generates TaskID/AgentID UUIDs when nil (2026-03-18)

**Large Effort (phase-level):**
- [x] STUB-001: A2A Python consumer mixin (`workers/codeforge/consumer/_a2a.py`) + PostgreSQL persistence already in SDK implementation (2026-03-18) — Cleanup: delete dead code `internal/port/a2a/`
- [x] STUB-010: SWE-agent backend adapter implementation (`workers/codeforge/backends/sweagent.py`) (2026-03-17)

**Small Effort (new from 2026-03-18 scan):**
- [x] STUB-024: Wire re-run benchmark button onClick in PromptOptimizationPanel (2026-03-18)
- [x] STUB-025: Remove deprecated `activeTool` prop from CanvasModal interface (2026-03-18)
- [x] Cleanup: Delete dead code `internal/port/a2a/` (zero imports, merge artifact) (2026-03-18)
- [x] STUB-004 (remaining): Wire BudgetPercent from event store accumulated cost (2026-03-18)

**No Action Needed (intentional designs):**
- STUB-007/008: GitHub OAuth + Subscription 501 gates (feature-gated by design)
- STUB-013/014/015: Intentional no-ops (_BenchmarkRuntime, StubBackendExecutor.cancel, useCRUDForm fallback)
- STUB-016-023, STUB-026: Documentation TODOs and intentional config exclusions

**Modular Prompt System (Phases A-F)**
- [x] Phase A: Domain types (PromptEntry, AssemblyContext, Category, Conditions) + YAML loader + tests (2026-03-17)
- [x] Phase B: PromptAssembler + PromptLibraryService + wiring into buildSystemPrompt + tests (2026-03-17)
- [x] Phase C: Write 56 embedded YAML prompt library files across 12 categories (2026-03-17)
- [x] Phase D: Migrate 24 mode PromptPrefix strings to YAML library files (2026-03-17)
- [x] Phase E: System reminders in NATS payload and prompt pipeline (2026-03-17)
- [x] Phase F: Wire assembler at app startup, add PromptsFS() accessor, integration tests (2026-03-17)

#### Codebase-Wide Lint Cleanup (2026-03-18)

- [x] Go: Fix all 50 golangci-lint issues (errcheck, gocritic hugeParam/rangeValCopy/appendCombine, gosec G118 nolint) (2026-03-18)
- [x] TypeScript: Fix all 21 ESLint issues (18 SolidJS reactivity warnings, 3 config parsing errors) (2026-03-18)
- [x] Config: golangci-lint exclusions for frontend/node_modules + G117/G118 test rules (2026-03-18)
- [x] Audit: Reviewed ~220+ linter suppression comments across Go/Python/TypeScript -- all justified (2026-03-18)

---

### Quality & Performance Improvements (2026-03-18)

> Spec: `docs/specs/2026-03-18-quality-performance-improvements-design.md`
> Based on: Current LLM-for-coding research (2025/26), codebase analysis, identified gaps
> 12 measures, 90+ atomic TODOs, 5 implementation phases

#### Phase 1 — Quick Wins (COMPLETED 2026-03-18)

**A1: Stall Detection + Escape** -- COMPLETED (2026-03-18)
- [x] A1.1-A1.4: Write 22 stall detection tests (identical calls, args hash, escape injection, double-stall abort, edge cases)
- [x] A1.5: Implement `StallDetector` class (~50 lines, deque-based sliding window) — now in `workers/codeforge/stall_detection.py`
- [x] A1.6: Integrate into `AgentLoopExecutor.run()` with `_check_stall()` helper (cyclomatic complexity managed)
- [x] A1.7: Publish `trajectory.stall_detected` event via `_runtime.publish_trajectory_event()`
- [x] A1.8: 60/60 tests pass (22 new + 33 existing agent loop + 5 related)

**B3: Adaptive Context Budget Based on Task Complexity** -- COMPLETED (2026-03-18)
- [x] B3.1-B3.3: Write 17 tests (9 budget mapping + 8 complexity classifier including edge cases)
- [x] B3.4: Implement `ClassifyComplexity()` in `internal/service/complexity.go` (240 lines, 7 heuristics + task-type boost)
- [x] B3.5: Implement `ComplexityBudget()` in `internal/service/context_budget.go` (composes with PhaseAware + Adaptive)
- [x] B3.6: NATS payload integration deferred to Phase 2 (Go-side functions ready)
- [x] B3.7: All Go service tests pass, golangci-lint clean

**C2: Confidence-Based Early Stopping for Multi-Rollout** -- COMPLETED (2026-03-18)
- [x] C2.1-C2.5: Write 15 early stopping tests (quorum, threshold, exit_code, rollout_count<=3, clusters, best selection)
- [x] C2.6: Implement `EarlyStopChecker` class in `workers/codeforge/evaluation/runners/early_stopping.py`
- [x] C2.7: Integrate into `MultiRolloutRunner.run()` with `MultiRolloutMetadata` dataclass
- [x] C2.8: Add `CODEFORGE_EARLY_STOP_THRESHOLD` / `CODEFORGE_EARLY_STOP_QUORUM` env vars
- [x] C2.9: 35/35 tests pass (15 new + 20 existing runner tests)

#### Phase 2 — Core Quality (A1 helps validate, ~8h total)

**A3: Plan/Act Mode Toggle (~5h)** -- DONE 2026-03-18
- [x] A3.1-A3.5: Write plan/act tests (tool restriction, phase transition, max iterations, autonomy, routing tags) -- 29 Python + 8 Go tests
- [x] A3.6: Add `plan_act_enabled` to NATS payload (`internal/port/messagequeue/schemas_conversation.go` + `models.py`)
- [x] A3.7: Set `plan_act_enabled` based on `modeAutonomy >= 4` in dispatcher (`conversation_agent.go`)
- [x] A3.8: Implement `PlanActController` class in `workers/codeforge/plan_act.py`
- [x] A3.9: Integrate into `run()` and `_do_llm_iteration()` in `agent_loop.py`
- [x] A3.10-A3.11: Add `CODEFORGE_PLAN_ACT_MAX_ITERATIONS` env var (default 10) + all tests passing

**B2: Semantic Deduplication of Context Candidates (~3h)** ✅ 2026-03-18
- [x] B2.1-B2.3b: Write dedup tests (overlapping lines, cross-file, no-dupes, simhash edge cases) — 26 tests in `dedup_test.go`
- [x] B2.4: Implement `simhash64()` + `hammingDistance()` in `internal/service/dedup.go`
- [x] B2.5: Implement `deduplicateCandidates()` in `dedup.go`
- [x] B2.6: Integrate into `assembleAndPack()` in `context_optimizer.go`
- [x] B2.7: Run regression tests — all 11 context optimizer tests pass

#### Phase 3 — Context Intelligence (COMPLETED)

**B1: LLM-Based Re-Ranking of Retrieval Results**
- [x] (2026-03-18) B1.1-B1.4: Write reranker tests (reorder, prompt format, fallback, routing tags)
- [x] (2026-03-18) B1.5: Add NATS subjects in `queue.go` + `_subjects.py`
- [x] (2026-03-18) B1.6: Implement `ContextReranker` in `workers/codeforge/context_reranker.py`
- [x] (2026-03-18) B1.7-B1.7b: Add NATS handler (`ContextHandlerMixin`) + Go integration test
- [x] (2026-03-18) B1.8: Integrate into Go `ContextOptimizerService` (syncWaiter + `assembleAndPack()`)
- [x] (2026-03-18) B1.9-B1.11: Add config (`CODEFORGE_CONTEXT_RERANK_ENABLED/MODEL`) + verify JetStream + run tests

**A2: Conversation Summarization at Context Exhaustion**
- [x] (2026-03-18) A2.1-A2.4: Write summarization tests (threshold, tail preservation, prompt, routing tags)
- [x] (2026-03-18) A2.5: Implement `ConversationSummarizer` class in `workers/codeforge/history.py`
- [x] (2026-03-18) A2.6: Implement `_summarize_history()` async method
- [x] (2026-03-18) A2.7: Add `async summarize_if_needed()` pre-processing step in `_conversation.py`
- [x] (2026-03-18) A2.8: Add `CODEFORGE_SUMMARIZE_THRESHOLD` env var + wire into NATS payload
- [x] (2026-03-18) A2.9: Run regression tests (16 new tests pass, no regressions)

#### Phase 4 — Advanced Features (COMPLETED 2026-03-18)

**C1: Routing Transparency + Mid-Loop Model Switching (COMPLETED 2026-03-18)**
- [x] (2026-03-18) C1.1-C1.4: Write routing transparency + quality signal + model switch tests — 7 new tests in `test_routing_transparency.py`
- [x] (2026-03-18) C1.5: `route_with_metadata()` on `HybridRouter` — `workers/codeforge/routing/router.py`, returns `RoutingMetadata`
- [x] (2026-03-18) C1.6: `IterationQualityTracker` class — now in `workers/codeforge/quality_tracking.py`, integrated into `run()`
- [x] (2026-03-18) C1.7: Wired `route_with_metadata()` into agent loop via `RoutingResult.routing_metadata` → `LoopConfig` → `_publish_routing_decision()` trajectory event
- [x] (2026-03-18) C1.8: 132/132 tests pass (23 routing + 33 agent loop + 67 routing/fallback + 34 consumer dispatch), zero regressions

**A4: Inference-Time Scaling for Conversations (COMPLETED 2026-03-18)**
- [x] (2026-03-18) A4.1-A4.5b: 27 rollout tests in `test_conversation_rollout.py` (single, multi, selection, early stopping, cost, non-git fallback, snapshot/restore, clamping, trajectory metadata)
- [x] (2026-03-18) A4.6: `rollout_count` in Go NATS payload (`internal/port/messagequeue/schemas_conversation.go`) + Python `ConversationRunStartMessage` (`models.py:488`)
- [x] (2026-03-18) A4.7: `Agent.ConversationRolloutCount` config (`config.go:151`), env var `CODEFORGE_AGENT_CONVERSATION_ROLLOUT_COUNT`, default 1
- [x] (2026-03-18) A4.8: `ConversationRolloutExecutor` (`agent_loop.py`) with `EarlyStopChecker`, non-git fallback
- [x] (2026-03-18) A4.9: `_snapshot_workspace()` + `_restore_workspace()` (`agent_loop.py`) via git stash
- [x] (2026-03-18) A4.10: NATS consumer dispatch wired — `rollout_count > 1` triggers `ConversationRolloutExecutor` in `_conversation.py:279-290`, clamped `max(1, min(count, 8))`
- [x] (2026-03-18) A4.11-A4.12: `trajectory.rollout_complete` event with rollout_count, selected_index, scores, early_stopped; NATS contract fixture updated; 128/128 tests pass

#### Phase 5 — Ecosystem (COMPLETED)

**C3: New Benchmark Providers — DPAI Arena + Terminal-Bench (~6h)**
- [x] (2026-03-18) C3.1-C3.2: Research dataset formats and access methods
- [x] (2026-03-18) C3.3-C3.4: Write provider load tests (20 DPAI Arena + 21 Terminal-Bench + 15 Filesystem State = 56 tests)
- [x] (2026-03-18) C3.5: Implement `DPAIArenaProvider` (BenchmarkType.SIMPLE, HuggingFace DPAI/arena dataset)
- [x] (2026-03-18) C3.6: Implement `TerminalBenchProvider` (BenchmarkType.AGENT, filesystem state verification)
- [x] (2026-03-18) C3.7: Add `FilesystemStateEvaluator` for Terminal-Bench (expected files, content match, missing files)
- [x] (2026-03-18) C3.8-C3.9: Update Go `defaultSuites` + all tests pass

**C4: RLVR Training Pipeline Export (~8h)**
- [x] (2026-03-18) C4.1-C4.4: Write RLVR export tests (19 Python + 13 Go service + 4 Go handler = 36 tests)
- [x] (2026-03-18) C4.5: Implement `compute_rlvr_reward()` (weighted avg, functional_test 2x, clamped [0,1])
- [x] (2026-03-18) C4.6: Implement `format_rlvr_entry()` formatter + `RLVRExporter` class
- [x] (2026-03-18) C4.7: Implement `ExportRLVRDataset()` in Go service + `ComputeRLVRReward()`
- [x] (2026-03-18) C4.8-C4.10: Add `GET /api/v1/benchmarks/runs/{id}/export/rlvr` endpoint (JSONL + JSON)
- [x] (2026-03-18) C4.11: Full test suite passes (87 Python, all Go packages green)

#### Claude Code Integration

- [x] Claude Code as routing target with execution branch (2026-03-18)
  - ClaudeCodeExecutor with SDK + CLI fallback
  - Policy enforcement via can_use_tool callback
  - COMPLEXITY_DEFAULTS: claudecode/default in COMPLEX + REASONING
  - Availability detection with caching
- [ ] Claude Code: E2E manual test with live CLI
- [x] (2026-03-19) Claude Code: model selection override (claudecode/claude-sonnet-4) <!-- audit: implemented in claude_code_executor.py + conversation routing -->
- [x] Claude Code: read CODEFORGE_CLAUDECODE_MAX_TURNS, _TIMEOUT, _TIERS from env (2026-03-18)
- [x] Claude Code: update docs/dev-setup.md with new env vars (2026-03-18)

#### UX/UI Audit Implementation (COMPLETED 2026-03-18)

> 17 atomic tasks across 3 layers implementing UX/UI improvements.

**Layer 1 — Quick Wins (6 tasks):**
- [x] (2026-03-18) Q1: Added anvil SVG favicon (`frontend/public/favicon.svg`)
- [x] (2026-03-18) Q2: Per-page document titles (17 pages)
- [x] (2026-03-18) Q3: Fixed Prompts Preview button variant (ghost -> secondary)
- [x] (2026-03-18) Q4: Debounced WebSocket reconnect banner (2s delay + 3s initial suppress)
- [x] (2026-03-18) Q5: Abbreviated KPI labels for mobile viewport
- [x] (2026-03-18) Q6: Hover effects + click-to-navigate on project cards

**Layer 2 — Medium-Term (7 tasks):**
- [x] (2026-03-18) M1: SVG empty state illustrations for 6 pages (MCP, Knowledge, Benchmarks, Prompts, Activity, Costs)
- [x] (2026-03-18) M2: Page transition fade-in animations (PageTransition component)
- [x] (2026-03-18) M3: Skeleton loaders replacing "Loading..." text on AI Config, Costs, Settings
- [x] (2026-03-18) M4: Sticky section navigation on Settings page (9 sections, IntersectionObserver)
- [x] (2026-03-18) M5: Project Detail graceful degradation (per-panel ErrorBoundary)
- [x] (2026-03-18) M6: Anvil brand mark in sidebar header (CodeForgeLogo component)
- [x] (2026-03-18) M7: Collapsible model cards on AI Config page (expand/collapse all)

**Layer 3 — Strategic (4 tasks):**
- [x] (2026-03-18) S1: Typography system -- Outfit (display) + Source Sans 3 (body), self-hosted woff2 in `frontend/public/fonts/`
- [x] (2026-03-18) S2: Micro-interactions -- button press, card hover lift, tab animation, KPI count-up, toast slide-in, modal fade+scale
- [x] (2026-03-18) S3: Living design system page at `/design-system` (dev-mode only) + `frontend/src/ui/DESIGN-SYSTEM.md`
- [x] (2026-03-18) S4: 3-step onboarding wizard for first-time users (Connect Code -> Configure AI -> Create Project)

---

#### Bugs Found During Autonomous Goal-to-Program Test (2026-03-19)

> Discovered during S1 testplan execution (`docs/testing/autonomous-goal-to-program-testplan.md`).
> These are product bugs blocking autonomous agent execution end-to-end.

**Bug 1 — Model Router ignores LiteLLM health status (Priority: HIGH) — FIXED 2026-03-19**
- [x] (2026-03-19) `model_resolver.py` now queries `/health` endpoint and prefers healthy models over alphabetically first
- [x] (2026-03-19) `key_filter.py` now accepts models known healthy from LiteLLM `/health` even without API key (covers local LM Studio via `openai/container`)
- [x] (2026-03-19) `_conversation.py:144` explicit `model` from NATS payload now takes precedence over routing; routing only applies when no explicit model set
- [x] (2026-03-19) Health-aware model selection added via `_fetch_healthy_models()` in `model_resolver.py`

**Bug 2 — NATS JetStream backlog from cancelled conversations blocks new runs (Priority: CRITICAL) — FIXED 2026-03-19**
- [x] (2026-03-19) `handleConversationToolCall` now fast-rejects tool calls for cancelled conversation runs via `RunStateManager.cancelledConvs` (`internal/service/run_state.go`); the flag is cleared when the next run starts (KI-24, fixed 2026-09-30)
- [x] (2026-03-19) `StopConversation` HTTP handler calls `Runtime.MarkConversationRunCancelled()` which also cleans up HITL approval channels
- [ ] Consider per-conversation NATS subjects or consumer groups to prevent cross-conversation blocking (future improvement)

**Bug 3 — Worker env var naming inconsistency (Priority: LOW) — FIXED 2026-03-19**
- [x] (2026-03-19) `LITELLM_URL` vs `LITELLM_BASE_URL` — code reads `LITELLM_BASE_URL`, documented in testplan (the devcontainer still sets `LITELLM_URL`, see [KI-50](#known-issues))
- [x] (2026-03-19) `_conversation.py` inline env reads replaced with `self._litellm_url` from constructor
- [x] (2026-03-19) `_benchmark.py` inline env reads replaced with `WorkerSettings().litellm_url`

**Bug 4 — Onboarding agent blocks project setup (Priority: MEDIUM) — FIXED 2026-03-19**
- [x] (2026-03-19) `goal_researcher` mode autonomy changed from 2 (semi-auto) to 4 (full-auto) — tool calls auto-approve
- [ ] HITL approval cards for `list_directory`/`glob_files` tools are not visible in chat UI (tool calls time out silently after 60s) — separate UI bug
- [x] (2026-03-19) Zombie NATS messages from cancelled conversations now fast-rejected (see Bug 2 fix)

**Bug 5 — WSL2 Docker port mapping unreachable from host (Priority: LOW, environment-specific) — FIXED 2026-03-19**
- [x] (2026-03-19) Documented in testplan Phase 0 — container IPs must be used instead of localhost
- [ ] Add `dev-setup.md` section for WSL2-specific Docker networking workarounds
- [x] (2026-03-19) Created `scripts/resolve-docker-ips.sh` helper that exports correct env vars

#### Multi-Language Autonomous Testplan (2026-03-22)

> Design spec: `docs/specs/2026-03-22-autonomous-multi-language-testplan-design.md`
> Testplan: `docs/testing/autonomous-multi-language-testplan.md`
> Implementation plan: `docs/plans/2026-03-22-autonomous-multi-language-testplan.md`

- [x] (2026-03-22) Research: AI coding agent benchmarks + showcase demos (SWE-bench, Commit0, DevBench, Devin, MetaGPT, Codex, etc.)
- [x] (2026-03-22) Design spec: 10 phases, 2 modes (A: Weather Dashboard, D: Free Choice), 4-tier verification
- [x] (2026-03-22) Implementation plan: 11 tasks, 28 steps
- [x] (2026-03-22) Executable runbook: 11 phases with Playwright-MCP commands, decision trees, report template
- [x] (2026-03-22) First test run: Mode A (Weather Dashboard) with local model (`lm_studio/qwen/qwen3-30b-a3b`) — `docs/testing/2026-03-22-multi-language-autonomous-report.md`, `docs/testing/2026-03-23-multi-language-autonomous-report.md`
- [x] (2026-03-23) First test run: Mode A with cloud model (`groq/llama-3.1-8b-instant`, not Claude/GPT) — `docs/testing/2026-03-23-run4b-multi-language-report.md`
- [ ] First test run: Mode D (Free Choice)

#### Universal Audit Remediation (2026-03-23)

> Audit report: `docs/audits/2026-03-23-universal-audit-report.md`
> 11 commits, 57 files changed, +3287 / -863 lines

**Resolved Findings:**
- [x] (2026-03-23) **F-002 (CRITICAL):** Hexagonal architecture violation -- event types moved from `adapter/ws/events.go` to `internal/domain/event/` (broadcast.go, broadcast_payloads.go, agui.go). OTEL span helpers moved from `adapter/otel/spans.go` to `internal/telemetry/spans.go`. LSP service decoupled via `port/codeintel/provider.go` interface + `adapter/lsp/noop.go` fallback
- [x] (2026-03-23) **F-008 (HIGH):** Silenced database errors -- 38 `_ = s.store.*` calls in service layer replaced with `logBestEffort` helper (`internal/service/log_best_effort.go`) that logs non-fatal errors with structured context
- [x] (2026-03-23) **F-010 (HIGH):** Test coverage gaps -- 2384 LOC new tests: `runtime_execution_test.go` (903 LOC), `runtime_lifecycle_test.go` (904 LOC), `test_conversation_handler.py` (577 LOC)
- [x] (2026-03-23) **F-033 (MEDIUM):** Handler struct concrete types -- `Handlers.LiteLLM *litellm.Client` replaced with `Handlers.LLM llm.Provider`, `Handlers.Copilot *copilot.Client` replaced with `Handlers.TokenExchanger tokenexchange.Exchanger`
- [x] (2026-03-23) **F-034 (MEDIUM):** Event types in adapter layer -- moved to `internal/domain/event/` (55 event constants + 49 payload structs)

**Remaining Findings — Remediation Work Plans (2026-03-27):**

> Audit: the F-IDs below come from the 45-finding version of 2026-03-27 (commit 43372899); the current
> `docs/audits/2026-03-27-universal-audit-report.md` is a 77-finding re-run with SEC-/QUAL-/ARCH-/INFRA-/COMP- IDs.
> Work plans: `docs/plans/2026-03-27-audit-remediation-workplans.md` (22 worktrees; WT-11..WT-22 are tracked there).
> Still-open findings re-verified on 2026-09-29 are tracked as [Known Issues](#known-issues): INFRA-001 → KI-47,
> INFRA-002 → KI-44, SEC-003/SEC-004/INFRA-007/INFRA-008 → KI-14, SEC-008 (WT-12) → KI-12, COMP-014 → KI-54.

**Tier 1 — CRITICAL (parallel):**
- [x] (2026-03-27, 0de61dbf) **WT-1 `fix/config-secrets`:** F-001/F-003/F-012/F-013/F-018 — `json:"-"` on 10 config fields, JWT blocklist, clear codeforge.yaml secrets, sslmode staging rejection
- [x] (2026-03-27, 15a2efaf) **WT-2 `fix/nats-trajectory-duplicate`:** F-002 — remove duplicate trajectory subscription, add missing `roadmap_proposed`/`subagent_requested` handlers

**Tier 2 — HIGH (parallel after Tier 1):**
- [x] (2026-03-27, 80cbf266) **WT-3 `fix/gdpr-compliance`:** F-008/F-025/F-026/F-027 — audit log anonymization (ADR-009), GDPR service tests, IP retention 180d (residual defects KI-52, KI-53 fixed 2026-09-30)
- [x] (2026-03-27, deb9863c) **WT-4 `fix/error-handling`:** F-007/F-020 — dashboard swallowed errors (logBestEffort), dead model resolution
- [ ] **WT-5 `refactor/hexagonal-handlers`** (partial): F-005/F-015/F-016 — AllowAlways extracted to PolicyService (2026-03-27, a024c503); remaining: move `os.ReadFile` / `os.MkdirAll` / `os.Remove` out of `handlers_goals.go`, `handlers_policy_crud.go` and `PolicyService.AllowAlways` behind the filesystem port
- [x] (2026-03-27) **WT-6 `test/auth-token`:** F-009 — 11 auth token lifecycle tests in `internal/service/auth_token_test.go`
- [ ] Add refresh-token reuse-detection and concurrent-refresh race tests (planned in WT-6, not written)
- [ ] **WT-7 `fix/frontend-compliance`** (partial): F-010/F-036/F-041 — AGPL source link and aria-labels done (2026-03-27, 1875625d); remaining: route the `fetch()` call in `features/chat/commandExecutor.ts` through the API client (`features/project/RefactorApproval.tsx` uses it since [KI-17](#known-issues))

**Tier 3 — MEDIUM (after WT-5):**
- [x] (2026-03-27, 892da44e) **WT-8 `fix/infra-hardening`:** F-004/F-014/F-031/F-040 — JetStream retention limits, Prometheus alerts, backup encryption
- [ ] **WT-9 `refactor/type-safety`:** F-019/F-021/F-022/F-037/F-038/F-039 — Python Protocols, Go response structs, flatten nesting

**Tier 4 — BACKLOG (after WT-2):**
- [ ] **WT-10 `refactor/god-objects`:** F-006/F-017 — Store ISP decomposition, RuntimeService/ConversationService split

---

#### Frontend Feature Pages & Prompt Evolution (COMPLETED 2026-03-23)

- [x] (2026-03-23) **MicroagentsPage:** UI for managing YAML+Markdown trigger-driven microagents -- `frontend/src/features/microagents/MicroagentsPage.tsx`
- [x] (2026-03-23) **QuarantinePage:** Admin review UI for Phase 23B message quarantine (risk scores, evaluate/approve/reject) -- `frontend/src/features/quarantine/QuarantinePage.tsx`
- [x] (2026-03-23) **A2APage:** Frontend for Phase 27 A2A v0.3.0 agent federation (remote agents, task history, AgentCard details) -- `frontend/src/features/a2a/A2APage.tsx`
- [x] (2026-03-23) **RoutingStatsPage:** Live statistics for Phase 29 hybrid routing (model distribution, fallback events, MAB UCB1 scores) -- `frontend/src/features/routing/RoutingStatsPage.tsx`
- [x] (2026-03-23) **Prompt Evolution:** LLM-driven prompt improvement pipeline -- reflect/mutate cycles via NATS. Files: `frontend/src/features/prompts/EvolutionTab.tsx`, `internal/service/prompt_evolution.go`, `workers/codeforge/consumer/_prompt_evolution.py`, migration 078

---

#### Frontend Cleanup (2026-03-24)

- [x] (2026-03-24) **FIX-106:** Extracted inline SVG icons from ChatPanel.tsx into `frontend/src/ui/icons/ChatIcons.tsx` (AttachIcon, CanvasIcon)
- [x] (2026-03-24) **useCRUDForm fix:** Replaced silent `async () => {}` fallback with dev-mode console.warn in `frontend/src/hooks/useCRUDForm.ts`

#### Security Hardening (2026-03-24)

- [x] (2026-03-24) **FIX-093:** Add `force_secure_cookies` config flag to `Server` struct -- unconditionally set `Secure=true` on cookies for TLS-terminating proxy deployments. Refactored `isSecureRequest` into `isSecureRequestWithConfig` + `isSecureCookie` method on Handlers. Files: `internal/config/config.go`, `internal/adapter/http/handlers_auth.go`, `internal/adapter/http/handlers.go`
- [x] (2026-03-24) **FIX-096:** Add per-user rate limiting keyed on JWT user ID -- composite key `userID:IP` for authenticated requests, IP-only fallback for unauthenticated (the IP part was spoofable until 2026-09-29, [KI-11](#known-issues)). Auth middleware injects user ID into context at all 5 auth paths. Files: `internal/middleware/ratelimit.go`, `internal/middleware/auth.go`

#### v2 API Migration Design (2026-03-24)

- [x] (2026-03-24) v2 API migration design document (`docs/specs/v2-api-migration-design.md`) -- FIX-061/063/095/098/100

#### Playwright-MCP Frontend Testing & Bugfixes (2026-03-24)

> Full test report: `docs/testing/2026-03-24-autonomous-goal-to-program-report.md`

- [x] (2026-03-24) **S2 Autonomous Test Run:** Executed S2 (Medium — Build Your Own `cut` Tool) via agentic chat with `lm_studio/qwen/qwen3-30b-a3b`. 18 tool calls, 1 git commit, package structure correct. Result: PARTIAL (file read scope bug in agent output). Report: `docs/testing/2026-03-24-autonomous-goal-to-program-report.md`
- [x] (2026-03-24) **FIX: migration 077** — `CREATE SEQUENCE agent_events_seq_number_seq` fails on restart after partial migration. Added `IF NOT EXISTS` to both `ADD COLUMN` and `CREATE SEQUENCE`. File: `internal/adapter/postgres/migrations/077_add_agent_events_sequence_number.sql`
- [x] (2026-03-24) **Playwright-MCP full frontend test** — tested 20/22 routes with real browser interactions (login, CRUD, forms, tabs, theme, locale). Found and fixed 7 bugs:
  - `App.tsx`: missing `</NavLink>` closing tag for A2A nav link
  - `api/client.ts`, `api/resources/index.ts`, `api/types.ts`: 3 leftover merge conflict markers (`>>>>>>> feat/frontend-prompt-evolution`)
  - `api/types.ts`: missing closing `}` for `QuarantineReviewRequest` interface
  - `ui/layout/NavIcons.tsx`: `MicroagentsIcon` missing `</svg>`, `);`, `}` — `RoutingIcon` function started inside it
  - `features/project/FilePanel.tsx`: `useFileTree()` called outside `FileTreeProvider` scope — wrapped component in provider

#### Project Detail UX Improvements (2026-03-24)

> UX test report: `docs/testing/2026-03-24-project-detail-ux-report.md`
> UX testplan: `docs/testing/project-detail-ux-testplan.md`

- [x] (2026-03-24) **Project Detail UX Test:** 37 features tested across 8 phases via Playwright-MCP. 35 PASS, 2 FAIL (Pull/Auto-Agent errors). Report: `docs/testing/2026-03-24-project-detail-ux-report.md`
- [x] (2026-03-24) **FIX: Pull button** returns "internal server error" on local projects → now returns 400 with "no remote configured — add a remote with 'git remote add origin <url>'" (`internal/service/project.go`)
- [x] (2026-03-24) **FIX: Auto-Agent** returns 404 when no roadmap exists → now returns 400 with "create a roadmap with features before starting auto-agent" (`internal/service/autoagent.go`)
- [x] (2026-03-24) **FIX: Goal delete confirmation** — added `useConfirm` dialog before goal deletion to prevent accidental data loss (`features/project/GoalsPanel.tsx`)
- [x] (2026-03-24) **Grouped Panel Selector** — replaced native `<select>` (SolidJS strips `<optgroup>`) with custom dropdown using Portal + inline styles. 4 groups (Planning/Execution/Intelligence/Governance), 14 panels with one-line descriptions (`features/project/ProjectDetailPage.tsx`)

#### AI Pipeline E2E Test & Event-Flow Fixes (2026-03-24)

- [x] (2026-03-24) **E2E AI Goal Discovery Test:** LLM reads codebase via `read_file`, proposes 3 goals via `propose_goal` tool (Add Math Ops, Testing, Documentation). Verified with `lm_studio/qwen/qwen3-30b-a3b`.
- [x] (2026-03-24) **BUG: AI Discover without explicit model** selects weak model (`groq/llama-3.1-8b`) → produces empty responses. With explicit tool-capable model, works correctly.
- [x] (2026-03-24) **FIX: Goal auto-persist broken** — `runtimeSvc.SetGoalService(goalSvc)` was never called in `main.go`, so `s.goalSvc == nil` and auto-persist was silently skipped. Added the missing wiring.
- [x] (2026-03-24) **FIX: Trajectory event UUID error** — `AgentEvent.AgentID` and `TaskID` were empty strings for conversation-based runs, causing `invalid input syntax for type uuid` on INSERT. Fixed by using `RunID` as fallback (`internal/service/runtime.go`).
- [x] (2026-03-24) **FIX: AI Discover model selection** — Handler now accepts optional `model` field in request body, passed via `WithModel()`. Prevents auto-selection of weak models that can't use tools (`internal/adapter/http/handlers_goals.go`).
- [x] (2026-03-24) **FIX: GoalProposalCard wiring** — `onAIDiscoverStarted` callback connected: GoalsPanel → ProjectDetailPage → ChatPanel `switchToConversation` signal. Chat auto-switches to goal-discovery conversation so AG-UI events match.
- [x] (2026-03-24) **FIX: Panel dropdown clicks** — SolidJS `onClick` (delegated) races with `document.addEventListener("click")` in Portal. Changed to `on:click` (native, direct) which fires before delegation. Options now register clicks correctly.
