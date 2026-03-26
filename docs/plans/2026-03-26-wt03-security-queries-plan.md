# Worktree 3: fix/security-queries — Query- und Auth-Lücken

**Branch:** `fix/security-queries`
**Priority:** Hoch
**Scope:** 3 findings (F-SEC-006, F-SEC-007, F-SEC-008)
**Estimated effort:** Small (1 day)

## Steps

### 1. F-SEC-006: LIKE Wildcard Injection — Escape meta-characters

**File:** `internal/adapter/postgres/store_project.go:43`

Replace LIKE with `position()` / `strpos()`:
```go
`SELECT ... FROM projects WHERE position($1 IN repo_url) > 0 AND tenant_id = $2 LIMIT 1`
```

Or escape LIKE meta-characters before query:
```go
escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(repoName)
// ... WHERE repo_url LIKE '%' || $1 || '%' ESCAPE '\\' AND tenant_id = $2
```

**Ref:** CWE-943

### 2. F-SEC-007: Setup Race Condition — Atomic check-and-create

**File:** `internal/adapter/http/handlers_auth.go:311-347`

Use database advisory lock or atomic INSERT:
```go
// Option A: Advisory lock
tx.Exec(ctx, "SELECT pg_advisory_xact_lock(42)")
// check + create within same transaction

// Option B: Atomic CTE
INSERT INTO users (...)
SELECT ... WHERE NOT EXISTS (SELECT 1 FROM users WHERE role = 'admin' AND tenant_id = $N)
RETURNING id
```

**Ref:** CWE-367

### 3. F-SEC-008: Modular Bias in Random Password — Use crypto/rand.Int

**File:** `internal/crypto/crypto.go:44`

Replace `b[i] = charset[int(b[i])%len(charset)]` with:
```go
idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
result[i] = charset[idx.Int64()]
```

Also fix forced character class placement (lines 47-49): use `rand.Int` for position and character selection instead of modular arithmetic at fixed indices.

**Ref:** CWE-330

## Verification

- `GetProjectByRepoName` with `%` input returns empty, not all projects
- Concurrent `POST /auth/setup` requests create exactly one admin
- Statistical test: generate 100K passwords, verify character frequency distribution is uniform (chi-squared test)
