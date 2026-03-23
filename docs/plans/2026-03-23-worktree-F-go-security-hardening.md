# Worktree F: Go Security Hardening — Atomic Plan

> **Branch:** `fix/go-security-hardening`
> **Effort:** ~2h | **Findings:** 9 | **Risk:** Low (no API changes)

---

## Task F1: Add HSTS Header (S-009)

**File:** `internal/adapter/http/middleware.go:28-33`

- [ ] Add after line 33 (after CSP header):
```go
w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
```
- [ ] Verify: `go build ./cmd/codeforge/`

**Commit:** `fix: add HSTS header to SecurityHeaders middleware (S-009)`

---

## Task F2: Remove Email from Debug Log (C-001)

**File:** `internal/adapter/http/handlers_auth.go:43`

- [ ] Change line 43 from:
```go
slog.Debug("login failed", "email", req.Email, "error", err)
```
To:
```go
slog.Debug("login failed", "error", err)
```
- [ ] Verify: `go build ./cmd/codeforge/`

**Commit:** `fix: remove PII (email) from login debug log (C-001)`

---

## Task F3: Add MaxHeaderBytes to HTTP Server (I-016)

**File:** `cmd/codeforge/main.go:906-913`

- [ ] Add `MaxHeaderBytes: 1 << 13,` (8192) to `http.Server` struct after `IdleTimeout`:
```go
srv := &http.Server{
    Addr:              addr,
    Handler:           r,
    ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
    ReadTimeout:       cfg.Server.ReadTimeout,
    WriteTimeout:      cfg.Server.WriteTimeout,
    IdleTimeout:       cfg.Server.IdleTimeout,
    MaxHeaderBytes:    1 << 13, // 8 KiB
}
```
- [ ] Verify: `go build ./cmd/codeforge/`

**Commit:** `fix: set MaxHeaderBytes=8KiB on HTTP server (I-016)`

---

## Task F4: Rate-limit WebSocket Upgrade (I-020)

**File:** `cmd/codeforge/main.go:820`

The `/ws` route is registered at top-level before the rate-limited group. Move it into the rate-limited group or apply rate limiter inline.

- [ ] Change line 820 from:
```go
r.Get("/ws", hub.HandleWS)
```
To:
```go
r.Group(func(wsGroup chi.Router) {
    wsGroup.Use(rateLimiter.Handler)
    wsGroup.Get("/ws", hub.HandleWS)
})
```
- [ ] Verify: `go build ./cmd/codeforge/`
- [ ] Verify: `go test ./cmd/codeforge/... -count=1`

**Commit:** `fix: apply rate limiter to WebSocket upgrade endpoint (I-020)`

---

## Task F5: Wrap Bare Error Returns (Q-007)

6 files, mechanical change: `return err` -> `return fmt.Errorf("context: %w", err)`

- [ ] `internal/git/pool.go:34`: `return fmt.Errorf("acquire semaphore: %w", err)`
- [ ] `internal/resilience/breaker.go:58`: `return fmt.Errorf("circuit breaker: %w", err)`
- [ ] `internal/service/vcsaccount.go:98-99`: `return fmt.Errorf("get vcs account: %w", err)`
- [ ] `internal/adapter/tiered/cache.go:53-54`: `return fmt.Errorf("l1 cache set: %w", err)`
- [ ] `internal/adapter/tiered/cache.go:61-62`: `return fmt.Errorf("l1 cache delete: %w", err)`
- [ ] `internal/domain/user/user.go:80-81`: `return fmt.Errorf("password complexity: %w", err)`
- [ ] Add `"fmt"` import where missing
- [ ] Verify: `go build ./...`
- [ ] Verify: `go test ./internal/git/... ./internal/resilience/... ./internal/service/... ./internal/adapter/tiered/... ./internal/domain/user/... -count=1`

**Commit:** `fix: wrap bare error returns with context (Q-007, 6 files)`

---

## Task F6: Harden SQL Error Handling (S-005)

**File:** `internal/adapter/http/helpers.go:136-138`

- [ ] Change the default case from:
```go
default:
    slog.Error("unhandled domain error", "error", err)
    writeError(w, http.StatusInternalServerError, "internal server error")
```
To:
```go
default:
    slog.Error("unhandled domain error", "error", err.Error())
    writeError(w, http.StatusInternalServerError, "internal server error")
```
Note: `err.Error()` ensures only the string message is logged, not the full error chain which may include SQL details. This is a minimal change — the real fix (using typed errors instead of string matching) is tracked separately.

- [ ] Verify: `go build ./cmd/codeforge/`

**Commit:** `fix: log error message string instead of full error chain (S-005)`

---

## Verification

After all tasks:
- [ ] `go build ./cmd/codeforge/`
- [ ] `go vet ./...`
- [ ] `go test ./... -count=1 -timeout=120s` (full test suite)
