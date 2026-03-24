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
- **CSRF:** Not applicable (API-only, Bearer token auth)
- **Security Headers:** CSP, HSTS, X-Frame-Options, X-Content-Type-Options
- **Secrets:** Docker Secrets (production) / environment variables (development), never hardcoded
- **SSRF Protection:** Private IP range blocking (IPv4 + IPv6)

## Secret Management

### Development

Secrets are loaded from environment variables (`.env` file, gitignored).
The default dev key `sk-codeforge-dev` is used for LiteLLM in development only.

### Production

Secrets are stored as Docker Secrets and mounted at `/run/secrets/`:

1. **Generate:** `./scripts/generate-secrets.sh ./secrets`
2. **Deploy:** `docker compose -f docker-compose.prod.yml up -d`
3. **Rotate:** Update secret file, recreate the affected service

### Hierarchy (highest priority first)

1. Docker Secrets (`/run/secrets/*`) -- production, file-based, not visible in `docker inspect`
2. Environment variables -- development and CI, or fallback when secrets files are missing
3. Config file defaults (codeforge.yaml) -- NEVER for actual secret values

### Implementation

| Layer | Module | Pattern |
|-------|--------|---------|
| Go Core | `internal/secrets/provider.go` | `Provider` interface: `FileProvider` (Docker Secrets) with env var fallback, `Auto()` selector |
| Python Worker | `workers/codeforge/secrets.py` | `get_secret()`: file-first, env var fallback |
| Docker | `docker-compose.prod.yml` | Top-level `secrets:` block, per-service mounts |

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
