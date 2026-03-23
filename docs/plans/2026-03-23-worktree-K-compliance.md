# Worktree K: Compliance & Documentation — Atomic Plan

> **Branch:** `feat/compliance-and-docs`
> **Effort:** ~1d | **Findings:** 11 | **Risk:** Low-Medium (new endpoints, schema changes)

---

## Task K1: Fix GitHub Token in Clone URL (C-002)

**File:** `internal/adapter/github/provider.go:69`

The `x-access-token:TOKEN@host` pattern is standard Git HTTPS auth and is used by GitHub Actions, GitLab CI, etc. The real risk is token leakage in logs/process listings. Add documentation and masking.

- [ ] Add comment documenting the security consideration:
```go
// CloneURL returns a token-authenticated HTTPS clone URL.
// NOTE: Token is embedded in URL (standard git auth). Ensure this URL
// is never logged or displayed to users. Use credential helpers for
// interactive use.
```
- [ ] Verify: `go build ./cmd/codeforge/`

**Commit:** `docs: document security consideration for GitHub clone URL token (C-002)`

---

## Task K2: Verify CASCADE DELETE Constraints (C-003)

**File:** Check `internal/adapter/postgres/migrations/035_fix_cascade_deletes.sql`

Research shows CASCADE constraints are properly defined for `runs`, `plan_steps`. Check conversation_messages:

- [ ] Search migrations for `conversation_messages` FK constraint
- [ ] If missing: create migration `087_conversation_messages_cascade.sql`:
```sql
ALTER TABLE conversation_messages DROP CONSTRAINT IF EXISTS conversation_messages_conversation_id_fkey;
ALTER TABLE conversation_messages ADD CONSTRAINT conversation_messages_conversation_id_fkey
    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE;
```
- [ ] Verify: migration applies cleanly

**Commit:** `fix: add CASCADE DELETE for conversation_messages FK (C-003)`

---

## Task K3: Add CSRF Rationale to Architecture Docs (C-007)

**File:** `docs/architecture.md`

- [ ] Add section under existing Security content:
```markdown
### CSRF Protection

CodeForge does not use explicit CSRF tokens because:
1. API-only backend — no HTML forms, no cookie-based auth for mutations
2. Authentication uses Bearer tokens via Authorization header
3. Refresh tokens use HttpOnly + SameSite=Lax cookies (immune to CSRF)
4. CORS restricts cross-origin requests to the configured origin

Reference: OWASP CSRF Prevention Cheat Sheet — "Token-based authentication
(Bearer/JWT) is inherently CSRF-resistant when tokens are not auto-attached
by the browser."
```

**Commit:** `docs: add CSRF protection rationale to architecture docs (C-007)`

---

## Task K4: Create CHANGELOG.md (C-015)

**File:** Create `CHANGELOG.md` in project root

- [ ] Create with initial content covering v0.8.0:
```markdown
# Changelog

All notable changes to CodeForge are documented in this file.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html)

## [0.8.0] - 2026-03-23

### Added
- Universal audit prompt and automated codebase auditing
- Visual Design Canvas with SVG tools and multimodal pipeline
- Contract-First Review/Refactor pipeline
- Benchmark and Evaluation system (Phase 26+28)
- Security and Trust annotations (Phase 23)
- Real-Time Channels for team collaboration

### Security
- HSTS header added to all responses
- Docker images pinned to specific versions
- cap_drop ALL applied to production containers
- Pre-deploy env validation script

### Fixed
- Bare error returns wrapped with context (6 files)
- Python exception handlers now log error details
- WebSocket endpoint rate-limited
```

**Commit:** `docs: create CHANGELOG.md (C-015)`

---

## Task K5: Fix Image Alt Text (C-006)

**File:** `frontend/src/features/project/FileTree.tsx:201,283`

- [ ] Line 201: Change `alt=""` to `alt={props.entry.is_dir ? "folder" : "file"}`
- [ ] Line 283: Same change
- [ ] Verify: `cd frontend && npx tsc --noEmit`

**Commit:** `fix: add semantic alt text to FileTree icons (C-006)`

---

## Task K6: Add Startup Warning for Default Passwords (S-010, C-014)

**File:** `internal/config/config.go` or `cmd/codeforge/main.go`

- [ ] After config loading, add validation:
```go
if cfg.AppEnv != "development" {
    if cfg.Database.DSN != "" && strings.Contains(cfg.Database.DSN, "codeforge_dev") {
        slog.Warn("production detected with default database password — change POSTGRES_PASSWORD")
    }
}
```
- [ ] Verify: `go build ./cmd/codeforge/`

**Commit:** `fix: warn on default database password in production (S-010, C-014)`

---

## Task K7: Separate LLM Key Encryption from JWT Secret (I-021)

**File:** `cmd/codeforge/main.go` (search for `crypto.DeriveKey`)

- [ ] Add new config field `LLMKeyEncryptionSecret` to Auth config:
```go
LLMKeyEncryptionSecret string `yaml:"llm_key_encryption_secret"`
```
- [ ] In main.go, change from:
```go
llmKeyEncKey := crypto.DeriveKey(cfg.Auth.JWTSecret)
```
To:
```go
llmKeySecret := cfg.Auth.LLMKeyEncryptionSecret
if llmKeySecret == "" {
    llmKeySecret = cfg.Auth.JWTSecret // fallback for backwards compatibility
    slog.Warn("LLM key encryption using JWT secret — set AUTH_LLM_KEY_ENCRYPTION_SECRET for production")
}
llmKeyEncKey := crypto.DeriveKey(llmKeySecret)
```
- [ ] Add env mapping in config loader: `AUTH_LLM_KEY_ENCRYPTION_SECRET`
- [ ] Verify: `go build ./cmd/codeforge/`

**Commit:** `fix: separate LLM key encryption secret from JWT secret (I-021)`

---

## Verification

- [ ] `go build ./cmd/codeforge/`
- [ ] `go test ./... -count=1 -timeout=120s`
- [ ] `cd frontend && npx tsc --noEmit`
