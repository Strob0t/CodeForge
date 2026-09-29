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

> **Known gaps (2026-09-29):** policy/tool enforcement (KI-4..KI-10), cross-tenant WebSocket broadcasts (KI-12), no sandbox isolation (KI-13), experience pool not tenant-scoped (KI-16), dev compose exposes PostgreSQL/NATS (KI-14), production secrets (KI-46), GDPR retention job never runs (KI-52) - see [Known Issues](todo.md#known-issues).

## Secret Management

### Development

Secrets are loaded from environment variables (`.env` file, gitignored).
The default dev key `sk-codeforge-dev` is used for LiteLLM in development only.

### Production

Target design (planned): secrets are stored as Docker Secrets and mounted at `/run/secrets/`.

> **Implementation status (2026-09-29):** `docker-compose.prod.yml` mounts the files from `generate-secrets.sh` under `/run/secrets/`, but also requires every secret as an environment variable (`${VAR:?}`) and passes them to the containers as env vars / connection URLs (visible in `docker inspect`). The Go Core reads secrets from env only; the Python worker reads only `LITELLM_MASTER_KEY` from its secret file. Neither the JWT secret nor `CODEFORGE_INTERNAL_KEY` is passed to core. See [Known Issues](todo.md#known-issues) KI-46.

1. **Generate:** `./scripts/generate-secrets.sh ./secrets`
2. **Export (required today):** `POSTGRES_PASSWORD`, `LITELLM_MASTER_KEY`, `NATS_USER`, `NATS_PASS` (e.g. from the files in `./secrets`) - otherwise compose aborts with `POSTGRES_PASSWORD is required`
3. **Deploy:** `docker compose -f docker-compose.prod.yml up -d`
4. **Rotate:** Update the secret file and the exported variable, recreate the affected service

### Hierarchy (highest priority first)

Target design (planned); today only the Python worker applies step 1, and only for `LITELLM_MASTER_KEY`:

1. Docker Secrets (`/run/secrets/*`) -- production, file-based, not visible in `docker inspect`
2. Environment variables -- development and CI, or fallback when secrets files are missing
3. Config file defaults (codeforge.yaml) -- NEVER for actual secret values

### Implementation

| Layer | Module | Pattern |
|-------|--------|---------|
| Go Core | `internal/secrets/` | Env only today: `Vault` + `EnvLoader("LITELLM_MASTER_KEY")` (`cmd/codeforge/main.go`); `Provider` interface with `FileProvider` (Docker Secrets) and `Auto()` selector exists in `provider.go` but is not wired in (tests only) |
| Python Worker | `workers/codeforge/secrets.py` | `get_secret()`: file-first, env var fallback; used for `LITELLM_MASTER_KEY` only (`DATABASE_URL`/`NATS_URL` come from env) |
| Docker | `docker-compose.prod.yml` | Top-level `secrets:` block, per-service mounts; the same values are also required as env vars |

### Managed Secrets

| Secret | File Name | Services |
|--------|-----------|----------|
| `LITELLM_MASTER_KEY` | `litellm-master-key` | core, worker, litellm |
| `POSTGRES_PASSWORD` | `postgres-password` | core, worker, litellm |
| `NATS_USER` | `nats-user` | core, worker |
| `NATS_PASS` | `nats-pass` | core, worker |

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
