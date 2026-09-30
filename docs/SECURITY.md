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
- **Secrets:** Environment variables (development and, currently, production), never hardcoded; file-based Docker Secrets are only partly wired (see [Secret Management](#secret-management))
- **SSRF Protection:** Private IP range blocking (IPv4 + IPv6)

> **Known gaps (2026-09-29):** policy profiles not tenant-scoped (KI-68), tool-call approvals not tenant-checked (KI-63), experience pool not tenant-scoped (KI-16), agent tools can read the worker's secrets (KI-71), GDPR retention job never runs (KI-52) - see [Known Issues](todo.md#known-issues).

## Secret Management

### Development

Secrets are loaded from environment variables (`.env` file, gitignored).
The default dev key `sk-codeforge-dev` is used for LiteLLM in development only.

### Production

Secrets are Docker secret files mounted at `/run/secrets/` (implemented 2026-09-30, KI-46). The core receives only
`*_FILE` paths (nothing secret in `docker inspect`); PostgreSQL uses `POSTGRES_PASSWORD_FILE`, NATS a generated auth
config, LiteLLM and the worker export their values from the files in an entrypoint wrapper.

1. **Generate:** `./scripts/generate-secrets.sh` (hex values, PostgreSQL TLS pair, derived `database-url`, `nats-url`, `nats-auth.conf`)
2. **Validate:** `./scripts/validate-env.sh` (checks the mounted files)
3. **Deploy:** `docker compose -f docker-compose.prod.yml up -d`
4. **Rotate:** per secret, see the header of `scripts/generate-secrets.sh` and [dev-setup.md](dev-setup.md#secret-management); the JWT secret, the LLM key encryption secret, the LiteLLM master key and the PostgreSQL password cannot be rotated by regenerating the file (data loss or a password mismatch), the script refuses to

Agent tool subprocesses (bash, search, quality gates, git, benchmark commands, CLI backends, the Claude Code CLI) run
with an allowlisted environment (`workers/codeforge/subprocess_env.py`): no `CODEFORGE_*`, database, NATS or LiteLLM
credentials. They still run as the worker's UID and can read `/run/secrets/*` and `/proc/1/environ` (KI-71); real
isolation needs the sandbox execution mode (KI-13). URL userinfo is redacted in all Go and worker logs.

### Hierarchy (highest priority first)

1. Docker Secrets (`/run/secrets/*`, referenced by `<KEY>_FILE`) -- production, file-based, not visible in `docker inspect`; setting both `KEY` and `KEY_FILE` is a startup error
2. Environment variables -- development and CI
3. Config file defaults (codeforge.yaml) -- NEVER for actual secret values

### Implementation

| Layer | Module | Pattern |
|-------|--------|---------|
| Go Core | `internal/secrets/`, `internal/config/loader.go` | `LookupFileEnv`: `<KEY>_FILE` for every secret setting (`loadSecretFiles` after the env layer); the SIGHUP-reloaded `EnvLoader` honours `_FILE` too; `RedactURL` in the log handler |
| Python Worker | `workers/codeforge/secrets.py`, prod entrypoint | `get_secret()` file-first for `LITELLM_MASTER_KEY`; the prod entrypoint exports `DATABASE_URL`, `NATS_URL`, `CODEFORGE_INTERNAL_KEY` from the files; tool subprocesses get `tool_env()` |
| Docker | `docker-compose.prod.yml` | Top-level `secrets:` block, per-service mounts, `*_FILE` environment only |

### Managed Secrets

| Secret | File Name | Services |
|--------|-----------|----------|
| `LITELLM_MASTER_KEY` | `litellm-master-key` | core, worker, litellm |
| `POSTGRES_PASSWORD` | `postgres-password` | postgres (initdb only) |
| PostgreSQL TLS pair | `postgres-tls.crt`, `postgres-tls.key` | postgres |
| `DATABASE_URL` (derived) | `database-url` | core, worker, litellm |
| `NATS_USER` / `NATS_PASS` | `nats-user`, `nats-pass` (inputs) | nats via `nats-auth.conf` (derived) |
| `NATS_URL` (derived) | `nats-url` | core, worker |
| `CODEFORGE_AUTH_JWT_SECRET` | `codeforge-auth-jwt-secret` | core |
| `CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET` | `codeforge-auth-llm-key-encryption-secret` | core |
| `CODEFORGE_INTERNAL_KEY` | `codeforge-internal-key` | core, worker |

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
