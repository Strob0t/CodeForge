# Worktree 2: fix/security-defaults — Unsichere Defaults härten

**Branch:** `fix/security-defaults`
**Priority:** SOFORT
**Scope:** 5 findings (F-SEC-002, F-SEC-003, F-SEC-005, F-SEC-012, F-COM-007)
**Estimated effort:** Small (1-2 days)

## Research Summary

- CISA Secure by Design Pledge: eliminate default passwords, fail-closed defaults
- NIST SP 800-53 SA-8(23): deny-unless-authorized
- OWASP JWT: minimum 64-char HMAC secrets
- Go 1.25 stdlib `crypto/hkdf.Key` for key derivation (RFC 5869)
- Stanford study: secure defaults work because they shift the burden from users

## Steps

### 1. F-SEC-002: JWT Secret — Enforce in all non-dev environments

**File:** `internal/config/loader.go` (validate function, ~line 381)

```go
if cfg.Auth.Enabled && cfg.Auth.JWTSecret == "codeforge-dev-jwt-secret-change-in-production" {
    if cfg.AppEnv != "development" {
        return errors.New("FATAL: default JWT secret only allowed in APP_ENV=development")
    }
    slog.Warn("using default JWT secret — acceptable only for local development")
}
if cfg.Auth.Enabled && len(cfg.Auth.JWTSecret) < 32 {
    return fmt.Errorf("auth.jwt_secret must be at least 32 characters (got %d)", len(cfg.Auth.JWTSecret))
}
```

### 2. F-SEC-005: Admin Password — Reject defaults in non-dev

**File:** `internal/config/loader.go` (~line 391-396)

- Change `slog.Warn` to `return errors.New(...)` when `cfg.AppEnv != "development"`
- Expand blocklist: `changeme123`, `admin`, `password`, `change_me_on_first_boot`
- Keep auto-generate path as recommended default

### 3. F-SEC-003: PostgreSQL SSL — Change default to sslmode=prefer

**File:** `internal/config/config.go:437`

```go
DSN: "postgres://codeforge:codeforge_dev@localhost:5432/codeforge?sslmode=prefer",
```

Add production validation:
```go
if cfg.AppEnv == "production" && strings.Contains(cfg.Postgres.DSN, "sslmode=disable") {
    return errors.New("postgres.dsn must not use sslmode=disable in production")
}
```

### 4. F-SEC-012: Auth disabled — Change example config

**File:** `codeforge.example.yaml:99`

- Change `enabled: false` to `enabled: true`
- Add comment: `# Set to false only for local development without authentication`

### 5. F-COM-007: LLM Key Encryption — SHA-256 → HKDF

**File:** `internal/crypto/aes.go:15-18`

Replace:
```go
func DeriveKey(jwtSecret string) []byte {
    h := sha256.Sum256([]byte(jwtSecret))
    return h[:]
}
```

With:
```go
func DeriveKey(secret string, salt []byte, info string) ([]byte, error) {
    return hkdf.Key(sha256.New, []byte(secret), salt, info, 32)
}
```

**Migration path:**
1. Add `key_version` column to `llm_keys` and `vcs_accounts` (default 1)
2. New function: `DeriveKeyV2(secret, salt, info)` using HKDF
3. On startup or via migration command: decrypt with old key, re-encrypt with new, set `key_version=2`
4. Update `DeriveKey` callers to use info strings: `"codeforge/llmkey/v1"`, `"codeforge/vcsaccount/v1"`
5. Generate and persist 32-byte random salt in config/DB at first startup

## Verification

- `APP_ENV=staging go run ./cmd/codeforge/` with default JWT secret → must fail
- `APP_ENV=development go run ./cmd/codeforge/` with defaults → must warn but start
- `APP_ENV=production` with `sslmode=disable` → must fail
- Existing encrypted LLM keys can be decrypted after migration
- All existing tests pass

## Sources

- [CISA Secure by Design Pledge](https://www.cisa.gov/securebydesign/pledge)
- [NIST SP 800-53 SA-8(23)](https://csf.tools/reference/nist-sp-800-53/r5/sa/sa-8/sa-8-23/)
- [OWASP JWT Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_for_Java_Cheat_Sheet.html)
- [RFC 5869 — HKDF](https://datatracker.ietf.org/doc/html/rfc5869)
- [Go crypto/hkdf docs](https://pkg.go.dev/crypto/hkdf)
- [Trail of Bits: Key Derivation Best Practices](https://blog.trailofbits.com/2025/01/28/best-practices-for-key-derivation/)
- [SoK: Safe and Secure Defaults (ScienceDirect 2025)](https://www.sciencedirect.com/science/article/pii/S2214212625000274)
