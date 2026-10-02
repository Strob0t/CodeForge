# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Email:** security@codeforge.dev (or create a private GitHub Security Advisory)
2. **Do NOT** open a public issue for security vulnerabilities
3. Include: description, reproduction steps, impact assessment
4. We will acknowledge within 48 hours and provide a fix timeline within 7 days

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.8.x   | Yes       |
| < 0.8   | No        |

## Security Measures

- **Authentication:** JWT with bcrypt password hashing (configurable cost)
- **Authorization:** Role-based access control (Admin, Editor, Viewer)
- **Tenant Isolation:** All database queries scoped by tenant_id
- **Rate Limiting:** Auth endpoints rate-limited, account lockout after 5 failures
- **CSRF:** No CSRF tokens; API calls use Bearer tokens, and the refresh-token cookie (HttpOnly, SameSite=Lax, path `/api/v1/auth`) relies on SameSite for CSRF mitigation
- **Security Headers:** CSP, HSTS, X-Frame-Options, X-Content-Type-Options
- **Secrets:** Environment variables in development, Docker secret files in production, never hardcoded (see [Secret Management](#secret-management))
- **Agent tool isolation:** processes an agent causes run as an unprivileged tool user (uid 10002) with no capabilities, separate from the worker that holds the secrets (see [Agent Tool Isolation](#agent-tool-isolation), [ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md))
- **NATS authentication:** every client authenticates as `core` or `worker` with its own password and per-subject permissions in production (see [NATS Authentication](#nats-authentication))
- **SSRF Protection:** Private IP range blocking (IPv4 + IPv6)

> **Known gaps (2026-10-01):** all tenants share one tool UID (KI-96), in-process worker readers follow symlinks out of a workspace (KI-95), MCP command arguments and URL userinfo are not redacted (KI-97), the MCP connection test is open to SSRF (KI-100), LSP servers started by the Go Core are not isolated (KI-83), the SVN password is passed on the command line (KI-87); see [Residual Risks](#residual-risks-of-tool-isolation-and-nats-authentication) and [Known Issues](todo.md#known-issues).

## Secret Management

### Development

Secrets are loaded from environment variables (`.env` file, gitignored).
The default dev key `sk-codeforge-dev` is used for LiteLLM in development only.

### Production

Secrets are Docker secret files mounted at `/run/secrets/` (implemented 2026-09-30, KI-46). The core receives only
`*_FILE` paths (nothing secret in `docker inspect`); the worker reads `*_FILE` natively too (since KI-71 no secret is in its
environment); PostgreSQL uses `POSTGRES_PASSWORD_FILE`, NATS reads the users' passwords from a generated file its config
includes, LiteLLM exports its values from the files in an entrypoint wrapper.

1. **Generate:** `./scripts/generate-secrets.sh` (hex values, PostgreSQL TLS pair, derived `database-url`, `nats-core-url`, `nats-worker-url`, `nats-passwords.conf`); after an upgrade from before KI-71 run it again, Compose fails until the new NATS files exist
2. **Validate:** `./scripts/validate-env.sh` (checks the mounted files)
3. **Deploy:** `docker compose -f docker-compose.prod.yml up -d`
4. **Rotate:** per secret, see the header of `scripts/generate-secrets.sh` and [dev-setup.md](dev-setup.md#secret-management); the JWT secret, the LLM key encryption secret, the LiteLLM master key and the PostgreSQL password cannot be rotated by regenerating the file (data loss or a password mismatch), the script refuses to

Agent tool subprocesses (bash, search, quality gates, git, benchmark commands and providers, CLI backends, the Claude
Code CLI) run with an allowlisted environment (`workers/codeforge/subprocess_env.py`): basics (PATH, HOME, locale,
TERM, TZ, temp dirs), proxy and CA-bundle variables, toolchain settings (GOPATH/GOPROXY/GOFLAGS/..., XDG dirs,
`npm_config_*`, pip index URLs, NODE_OPTIONS, CI); any allowed name containing KEY, TOKEN, SECRET, PASSWORD, PASSWD,
AUTH or CREDENTIAL is dropped; never `CODEFORGE_*`, `LITELLM_*`, `DATABASE_URL` or `NATS_URL`. CLI backends
additionally receive the provider key/endpoint list `PROVIDER_ENV` and their own prefix (`AIDER_*`, `GOOSE_*`,
`OPENCODE_*`, `PLANDEX_*`, `SWE_AGENT_*`); anything else goes through the backend's `extra_env`. Claude Code always
runs through the CLI (the SDK passes the full worker environment); it has no per-tool policy check yet (KI-72).
Credentials embedded in proxy or index URLs are passed as-is. With `CODEFORGE_TOOL_ISOLATION=required` (image and production) they run as the tool user and cannot read `/run/secrets/*` or the worker's environment (see [Agent Tool Isolation](#agent-tool-isolation)); isolation between runs and tenants needs per-tenant UIDs (KI-96) or the sandbox execution mode (KI-13). URL userinfo is redacted in all Go and worker logs (Go `secrets.RedactURL` in the log handler, including error and Stringer values; worker structlog processor and a filter on stdlib records; an `@` in a URL path or query is redacted as well).

### Hierarchy (highest priority first)

1. Docker Secrets (`/run/secrets/*`, referenced by `<KEY>_FILE`) -- production, file-based, not visible in `docker inspect`; setting both `KEY` and `KEY_FILE` is a startup error
2. Environment variables -- development and CI
3. Config file defaults (codeforge.yaml) -- NEVER for actual secret values

### Implementation

| Layer | Module | Pattern |
|-------|--------|---------|
| Go Core | `internal/secrets/`, `internal/config/loader.go` | `LookupFileEnv`: `<KEY>_FILE` for every secret setting (`loadSecretFiles` after the env layer); the SIGHUP-reloaded `EnvLoader` honours `_FILE` too; `RedactURL` in the log handler |
| Python Worker | `workers/codeforge/config.py`, `workers/codeforge/secrets.py` | `<KEY>_FILE` for `DATABASE_URL`, `NATS_URL`, `LITELLM_MASTER_KEY`, `CODEFORGE_INTERNAL_KEY` (both forms set is an error; files are read once, then the secrets directory is locked); tool subprocesses get `tool_env()` and run as the tool user |
| Docker | `docker-compose.prod.yml` | Top-level `secrets:` block, per-service mounts, `*_FILE` environment only |

### Managed Secrets

| Secret | File Name | Services |
|--------|-----------|----------|
| `LITELLM_MASTER_KEY` | `litellm-master-key` | core, worker, litellm |
| `POSTGRES_PASSWORD` | `postgres-password` | postgres (initdb only) |
| PostgreSQL TLS pair | `postgres-tls.crt`, `postgres-tls.key` | postgres |
| `DATABASE_URL` (derived) | `database-url` | core, worker, litellm |
| NATS user `core` password | `nats-core-pass` (input) | nats via `nats-passwords.conf` (derived) |
| NATS user `worker` password | `nats-worker-pass` (input) | nats via `nats-passwords.conf` (derived) |
| `NATS_URL` of the core (derived) | `nats-core-url` | core (mounted as `/run/secrets/nats-url`) |
| `NATS_URL` of the worker (derived) | `nats-worker-url` | worker (mounted as `/run/secrets/nats-url`) |
| NATS users and passwords (derived) | `nats-passwords.conf` | nats (included by `configs/nats/nats-server.conf`) |
| `CODEFORGE_AUTH_JWT_SECRET` | `codeforge-auth-jwt-secret` | core |
| `CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET` | `codeforge-auth-llm-key-encryption-secret` | core |
| `CODEFORGE_INTERNAL_KEY` | `codeforge-internal-key` | core, worker |

## Agent Tool Isolation

Agent tools run commands an LLM chose and code an agent wrote. Since KI-71 ([ADR-017](architecture/adr/017-tool-isolation-and-nats-authentication.md)) they do not run as the user that holds the secrets.

| Process | User | Rights |
|---|---|---|
| Go Core | uid 10001 (`codeforge`), group `codeforge-ws` 10010 | no capabilities, `no-new-privileges`, umask 002 |
| Worker | uid 10001, group `codeforge-ws` 10010 | the container starts as root with only `SETUID`, `SETGID` and `KILL`; the entrypoint (`scripts/worker-entrypoint.sh`) runs the worker as uid 10001 and keeps those three as ambient capabilities; it never runs as root |
| Tool processes | uid and gid 10002 (`codeforge-tool`), supplementary group 10010 only | no capabilities (inheritable, permitted, effective, ambient), `no_new_privs`, umask 002, other file descriptors closed |

- **Launcher.** Every process an agent causes (Bash, grep, `git`, quality gates and workspace tests, benchmark test commands, the backend CLIs and their version checks, the Claude Code CLI and its hook, MCP stdio servers) starts only through `workers/codeforge/tool_process.py`: `setpriv` with a fixed environment (`PATH` only, because `setpriv` holds the worker's capabilities and the loader would honour `LD_PRELOAD`), then `env -i` as the tool user sets the command's environment (`tool_env()` plus, for MCP servers, their declared variables without the worker's credentials and code-loading names such as `LD_*`, `PYTHON*`, `NODE_OPTIONS`). A test fails when any other module starts a process.
- **Fail closed.** `CODEFORGE_TOOL_ISOLATION=required` in the image and in production, `off` elsewhere; an unknown value counts as `required`. At startup a probe checks that a tool process has the expected uid, gid, groups, no capabilities, `no_new_privs`, umask, and cannot read `/proc/<worker pid>/environ` or `/run/secrets`. A failed probe in `required` mode makes every tool call fail with `ToolIsolationError`; nothing runs as the worker user. The effective mode is logged once at startup. A platform that forbids root containers therefore gets failing tool calls, not unisolated ones.
- **Secrets directory.** Compose ignores `uid`, `gid` and `mode` of file secrets (verified), so the secret files stay 0644. The worker mounts `/run/secrets` as a tmpfs with `uid=10001,mode=0700` and the files inside, so only uid 10001 can enter; after reading its secrets the worker sets the directory to mode 0, so a symlink planted in a workspace cannot make the worker's own file tools read a secret (settings keep the values they read; even `docker exec` cannot read `/run/secrets` afterwards). No secret is in the worker's environment (`*_FILE` paths only).
- **Tool environment.** Allowlisted variables only (see above), `HOME=/home/codeforge-tool` (a tmpfs, mode 0700, noexec, lost on restart), `USER=codeforge-tool`. `NATS_URL`, `*_URL_FILE` and `CODEFORGE_*` never reach it.
- **Workspaces.** `/data/workspaces` is `10001:10010` with mode 2775 (setgid). The Go Core, worker and tools share it through the group (umask 002; the Go Core creates files 0664 and directories 0770). `share_tool_files` (a `find -P` pass as the tool user after each tool process and at the end of every run, conversation run, task and benchmark task) moves what the tool user created into the group and opens it to it, never following a symlink. A one-time walk shares workspaces that existed before the upgrade; it uses directory descriptors and `O_NOFOLLOW`, so a tool process running meanwhile cannot redirect it, and its version stamp file (`.codeforge-workspace-sharing`, writable by the tool user) is read without following a symlink or blocking and replaced, never written through; forging or deleting it only skips or repeats the walk. Git calls of the Go Core stay hardened as in KI-77.
- **Verification.** `scripts/check-tool-isolation.sh` reproduces the production container settings and prints a tool process's credentials and what it can reach.

## NATS Authentication

The production NATS server (`configs/nats/nats-server.conf`, image `nats:2.15-alpine`) requires credentials. The development compose runs without authentication.

- **Users.** `core` (the Go Core: owns the `CODEFORGE` stream and the KV buckets, publishes every stream subject) and `worker` (publishes only its 31 result, output and heartbeat subjects and the `.dlq` copies of the 24 subjects it consumes; no stream changes, no KV, none of the Go Core's consumers). A compromised worker, or a prompt-injected agent, cannot forge what only the Go Core sends: run starts, cancels, tool-call decisions, task dispatches. Tool processes cannot read the worker's NATS URL file, so they cannot connect at all.
- **Passwords.** `nats-core-pass` and `nats-worker-pass` (hex, generated); `nats-core-url`, `nats-worker-url` and `nats-passwords.conf` are derived. `validate-env.sh` checks that the URLs and the password file agree and that the two passwords differ. URLs are redacted in logs. Rotate: delete `nats-core-pass` and/or `nats-worker-pass`, run `generate-secrets.sh` (the URLs and the password file follow), then recreate `nats`, `core` and `worker`.
- **Inboxes and consumers.** The server checks neither the deliver subject of a push consumer nor the reply subject of a pull request against the creator's publish rights, so a consumer can deliver stream messages to any plain subscription whose name its creator knows. Each service therefore has its own inbox prefix (`_INBOX_core`, `_INBOX_worker`) and may subscribe only to it, so the worker cannot learn the Go Core's inbox names; and the worker may create and use only consumers named in its permissions (its 24 durables by explicit name and four notification consumers, `codeforge-py-notify-<subject>`), never the Go Core's. A new subject or worker subscription needs an entry in the config.
- **Notifications.** Cancels and tool-call decisions reach runs through the worker's `NotificationHub`; a listener first reads the subject back from its start sequence with batched direct gets (`$JS.API.DIRECT.GET.CODEFORGE`; the stream has `AllowDirect`; nats-server 2.11 or newer). A read-back that fails or exceeds 100,000 messages fails the run or task.
- **Approved handoffs.** `handoff.approved` starts a run, so the Go Core carries a message out only when it is exactly the payload of an approved, not yet consumed quarantine message of its tenant (`quarantine_messages.consumed_at`, migration 111); a message that merely arrives on the subject is dropped with a warning.
- **Upgrades.** The worker's rights depend on how the server checks consumer names in API subjects. Re-run `workers/tests/test_nats_permissions.py`, `internal/adapter/nats/auth_test.go` and `workers/tests/test_deployment_isolation.py` against a new server version (`NATS_SERVER_BIN`) before changing the pin (KI-102).

## MCP Servers

- **Where they run.** Stdio MCP servers run in the worker as the tool user (the same launcher and environment as any tool process); the Go Core never starts a stdio server, and its connection test answers 400 for them. `sse` and `streamable_http` servers are connected from the worker at run time and tested from the Go Core (KI-100: no SSRF filter yet).
- **Secrets.** Env and header values are redacted to `***` in every response (list, get, project list, create, update), for every role; empty values stay empty. An update that sends `***` back keeps the stored value, only while transport, URL, command and arguments are unchanged (otherwise 400, so a stored secret is never sent to another destination). `***` on create, or for a key with nothing stored, is 400. Command arguments and URL userinfo are not redacted (KI-97): put secrets in env variables or headers.
- **Tenancy.** MCP servers and their project links belong to a tenant (migration 112); assigning a server to a project of another tenant, or unassigning across tenants, answers 404, and a run resolves its project's servers in its own tenant. A tenant's admins create, change, delete, test and assign the MCP servers of their tenant; every user of the tenant reads them (redacted). A stdio server has the rights the agents' Bash tool has in that tenant's runs, which is why tenant admins, not only platform admins, may define it; tenants still share the tool UID (KI-96).
- **Audit.** Create, update and delete are audited by the route; assign and unassign write one entry (`assign`/`unassign`, resource `mcp_server`, resource ID the server the handler decoded, details `{"project_id": ...}`) before the change, and the request is refused with 503 when the entry cannot be written (an audit log outage blocks assignment on purpose). A request refused before that is audited afterwards with its HTTP status and an empty resource ID.

## Residual Risks of Tool Isolation and NATS Authentication

| Risk | Follow-up |
|---|---|
| All runs and tenants share the tool uid 10002: tool processes can read, signal or trace each other and reach other workspaces (as before KI-71) | KI-96 (per-tenant UIDs), KI-13 (sandbox) |
| In-process worker readers (repo map, retrieval index, GraphRAG collectors, file tools racing a symlink swap) still follow symlinks out of a workspace; `/run/secrets` and the worker's environment are covered, other files the worker can read are not | KI-95 |
| Workspaces are group-writable, so an agent can still rewrite `.git/config`; the time-of-check/time-of-use window of KI-77 stays open | KI-77 |
| NATS does not permission-check deliveries to a known plain subscription; the worker cannot learn the core's inbox names, so this is residual only | KI-99 |
| The worker keeps `SETUID`, `SETGID` and `KILL` for its lifetime; tool processes keep the bounding set `e0`, harmless with `no_new_privs` and empty permitted sets | none (dropping it needs `CAP_SETPCAP`) |
| The worker can read any stream message through direct get (its own durables could already filter any subject); a read-back depends on the stream's retention (30 days, 5 million messages) | none |
| Files created by processes that outlive the run-end sharing pass stay private until the next pass in that workspace; the pass costs a walk per tool process | KI-103 |
| MCP args and URL userinfo are not redacted; the MCP connection test can reach internal hosts; `UpsertMCPServerTools` has no tenant filter | KI-97, KI-100, KI-101 |
| LSP language servers started by the Go Core are not isolated | KI-83 |
| An audit log outage blocks MCP assign and unassign (fail closed, intended) | none |

## GDPR Compliance

- User data export: `POST /api/v1/users/{id}/export`
- User data deletion: `DELETE /api/v1/users/{id}/data`
- Data retention policy: `docs/data-retention.md`
- LLM data processing: User code may be sent to configured LLM providers

## Dependency Management

- Go: `go mod tidy` + `govulncheck`
- Python: Poetry + Ruff security checks
- Frontend: npm audit
- Pre-commit hooks enforce linting and secret detection
