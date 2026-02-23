# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 0.1.x   | Yes                |

## Reporting a Vulnerability

If you discover a security vulnerability in CodeForge, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please email security concerns to the maintainers directly. Include:

1. A description of the vulnerability
2. Steps to reproduce the issue
3. Potential impact assessment
4. Suggested fix (if any)

We will acknowledge receipt within 48 hours and aim to provide a fix or mitigation plan within 7 days for critical issues.

## Security Practices

### Authentication

- JWT HS256 with fail-closed token revocation
- bcrypt password hashing (configurable cost)
- Refresh token rotation with atomic swap
- API key support with SHA-256 hashing (prefix-only storage)
- Forced password change for seeded admin accounts

### Authorization

- Role-based access control (admin, user, viewer)
- Multi-tenant isolation via tenant_id on all queries
- Policy layer with first-match-wins rule evaluation
- Path normalization to prevent traversal bypasses

### Transport Security

- HTTP security headers (X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy)
- CORS with configurable allowed origins
- Refresh tokens in httpOnly, Secure, SameSite=Strict cookies
- Webhook signature verification (HMAC-SHA256 with constant-time comparison)

### Infrastructure

- Rate limiting with per-IP token bucket
- Idempotency keys for mutating operations
- Circuit breakers on external service calls
- Docker sandbox isolation for agent execution
- No secrets in logs (structured logging with field filtering)

## Dependency Management

- Go: `go.sum` for integrity verification
- Python: Poetry with `poetry.lock`
- Frontend: `package-lock.json` with `npm ci`
- All dependencies pinned to specific versions in CI
