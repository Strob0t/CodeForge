# Changelog

All notable changes to CodeForge are documented in this file.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

## [Unreleased]

### Added
- Self-service GDPR endpoints: `/me/export` (GET) and `/me/data` (DELETE) for user data portability and erasure without admin role (Art. 15, 17, 20)
- Entropy-based JWT secret validation in production (rejects secrets with < 3.0 bits/char Shannon entropy)
- Auto-generation of cryptographically random JWT secret on first boot
- Docker secrets file support via `_FILE` env var suffix pattern
- `queryParamIntClamped` helper for bounded API parameter validation
- WebSocket connection limiting in nginx (limit_conn 50)
- Canvas keyboard navigation: Tab cycling, Delete/Backspace removal, Escape deselect (WCAG 2.1.1)
- `extractErrorMessage()` frontend utility replacing 52 inline error extractions

### Changed
- RBAC middleware added to ~30 previously unprotected write endpoints
- Audit logging added to password change, API key CRUD, branch protection CRUD
- Grype severity cutoff lowered from `critical` to `high`
- CI tool versions pinned: poetry==2.1.1, govulncheck@v1.1.4, pip-audit==2.9.0
- Worker HEALTHCHECK start-period increased from 15s to 30s
- nginx client_max_body_size reduced from 100M to 20M
- WebSocket proxy_read_timeout reduced from 86400 to 300
- PII (email) removed from INFO-level production logs in auth service
- Retention service now iterates all tenants for cleanup
- Agent loop methods annotated with typed IterationOutcome union

### Fixed
- Path traversal bypass in Python tool resolution (startswith -> is_relative_to)
- SSRF in A2A webhook, project info, VCS account HTTP clients (added SafeTransport)
- Channel webhook key validated with constant-time comparison (was presence-only check)
- Workspace adoption restricted to workspace root (was unrestricted absolute paths)
- DetectStackByPath restricted to workspace root
- Hardcoded JWT secret in active config bypassing loader blocklist
- 9 unbounded SQL queries now have LIMIT clauses
- Silenced JSON unmarshal errors in store layer replaced with slog.Warn
- Duplicate buildSystemPrompt (~125 LOC) removed
- Dead code (writeValidationError) removed
- Custom min64 replaced with Go 1.21+ built-in min

### Security
- F-ARC-011/012: bypass-approvals and branch protection CRUD now require admin role
- F-SEC-001: Python tool path traversal fix (CWE-22)
- F-SEC-002: Webhook HMAC validation (CWE-287)
- F-SEC-005/007/012: SSRF protection via SafeTransport (CWE-918)
- F-INF-001: Hardcoded secrets removed from config, entropy validation added
- F-COM-003: PII removed from production logs (GDPR Art. 5(1)(c))

## [0.8.0] - 2026-03-23

### Added
- Visual Design Canvas with SVG tools and multimodal pipeline (Phase 32)
- Contract-First Review/Refactor pipeline (Phase 31)
- Benchmark and Evaluation system (Phase 26+28)
- Security and Trust annotations (Phase 23)
- Real-Time Channels for team collaboration
- Chat enhancements: HITL permission UI, inline diff review, action buttons
- Hybrid routing with ComplexityAnalyzer and MAB model selection
- Agentic conversation loop with 7 built-in tools
- A2A protocol support (v0.3.0)
- MCP server with project/cost resources

### Security
- HSTS header on all responses
- Docker images pinned to specific versions
- cap_drop ALL on production containers
- Pre-deploy environment validation script
- Trust annotations with 4 levels on NATS messages

### Fixed
- Python exception handlers now log error details
- Bare Go error returns wrapped with context
- WebSocket endpoint rate-limited
- PostgreSQL archive command error handling
