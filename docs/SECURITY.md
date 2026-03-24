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
- **Secrets:** Environment variables only, never hardcoded
- **SSRF Protection:** Private IP range blocking (IPv4 + IPv6)

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
