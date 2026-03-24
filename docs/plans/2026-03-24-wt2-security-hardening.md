# WT2: Security Hardening — Per-User Rate Limiting & Cookie Config

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-user rate limiting keyed on JWT user ID for authenticated endpoints and a `force_secure_cookies` config flag for hardened deployments.

**Architecture:** Extend the existing `RateLimiter` to support composite keys (IP + optional user ID). Add `ForceSecureCookies` bool to config. Both are backward-compatible — existing IP-only behavior is preserved when no user context is present.

**Tech Stack:** Go 1.25, chi v5, JWT middleware

**Best Practices Applied:**
- Token bucket per-user keyed on JWT `sub` claim (industry standard)
- Composite key `userID:IP` prevents both per-user abuse and shared-IP exhaustion
- `force_secure_cookies` overrides `isSecureRequest()` for TLS-terminated proxies
- Standard `X-RateLimit-*` headers preserved

---

### Task 1: Add `ForceSecureCookies` config field

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/adapter/http/handlers_auth.go`

- [ ] **Step 1: Write the failing test**

Create: `internal/adapter/http/handlers_auth_test.go`

```go
package http

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestIsSecureRequest(t *testing.T) {
	tests := []struct {
		name   string
		tls    bool
		header string
		force  bool
		want   bool
	}{
		{"plain HTTP", false, "", false, false},
		{"direct TLS", true, "", false, true},
		{"proxy HTTPS header", false, "https", false, true},
		{"force overrides plain", false, "", true, true},
		{"force with proxy header", false, "https", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/", nil)
			if tt.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if tt.header != "" {
				r.Header.Set("X-Forwarded-Proto", tt.header)
			}
			got := isSecureRequestWithConfig(r, tt.force)
			if got != tt.want {
				t.Errorf("isSecureRequestWithConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/adapter/http && go test -run TestIsSecureRequest -v`
Expected: FAIL — `isSecureRequestWithConfig` undefined

- [ ] **Step 3: Add config field and refactor isSecureRequest**

In `internal/config/config.go`, find the `Server` or top-level struct (where HTTP config lives) and add:

```go
ForceSecureCookies bool `yaml:"force_secure_cookies"` // Unconditionally set Secure=true on cookies (default: false)
```

In `internal/adapter/http/handlers_auth.go`, refactor:

```go
// isSecureRequestWithConfig returns true when cookies should use Secure flag.
// If forceSecure is true, always returns true (for hardened deployments
// behind TLS-terminating proxies).
func isSecureRequestWithConfig(r *http.Request, forceSecure bool) bool {
	if forceSecure {
		return true
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
```

Update the existing `isSecureRequest` to delegate:
```go
func (h *Handlers) isSecureCookie(r *http.Request) bool {
	return isSecureRequestWithConfig(r, h.cfg.ForceSecureCookies)
}
```

Update all call sites of `isSecureRequest(r)` to use `h.isSecureCookie(r)`.

Remove the TODO comment at line 25-26.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/adapter/http && go test -run TestIsSecureRequest -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/adapter/http/handlers_auth.go internal/adapter/http/handlers_auth_test.go
git commit -m "feat(FIX-093): add force_secure_cookies config flag for hardened deployments"
```

---

### Task 2: Add per-user rate limiting

**Files:**
- Modify: `internal/middleware/ratelimit.go`
- Modify: `internal/middleware/ratelimit_test.go` (if exists, else create)

- [ ] **Step 1: Write the failing test**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiter_PerUserKey(t *testing.T) {
	rl := NewRateLimiter(1, 1) // 1 req/s, burst 1

	handler := rl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request from user-A on IP 1.2.3.4 — should pass
	r1 := httptest.NewRequest("GET", "/", nil)
	r1.RemoteAddr = "1.2.3.4:1234"
	r1 = r1.WithContext(withUserID(r1.Context(), "user-A"))
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", w1.Code)
	}

	// Second request from user-B on same IP — should pass (different user)
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "1.2.3.4:1234"
	r2 = r2.WithContext(withUserID(r2.Context(), "user-B"))
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("different user same IP: got %d, want 200", w2.Code)
	}

	// Third request from user-A again on same IP — should be rate limited
	r3 := httptest.NewRequest("GET", "/", nil)
	r3.RemoteAddr = "1.2.3.4:1234"
	r3 = r3.WithContext(withUserID(r3.Context(), "user-A"))
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, r3)
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("repeat user-A: got %d, want 429", w3.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/middleware && go test -run TestRateLimiter_PerUserKey -v`
Expected: FAIL — `withUserID` undefined

- [ ] **Step 3: Implement per-user rate limiting key**

In `internal/middleware/ratelimit.go`:

1. Add a context key type and helper:
```go
type rateLimitUserKey struct{}

// withUserID attaches a user ID to the context for rate limiting.
// Used by auth middleware after JWT validation.
func withUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, rateLimitUserKey{}, userID)
}

// userIDFromContext extracts the user ID from context, if present.
func userIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(rateLimitUserKey{}).(string); ok {
		return v
	}
	return ""
}
```

2. Update `Handler` to use composite key:
```go
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := rateLimitKey(r)
		remaining, retryAfter, allowed := rl.allow(key)
		// ... rest unchanged, just replace ip with key ...
	})
}

// rateLimitKey returns a composite key: "userID:IP" if authenticated, "IP" otherwise.
func rateLimitKey(r *http.Request) string {
	ip := realIP(r)
	if userID := userIDFromContext(r.Context()); userID != "" {
		return userID + ":" + ip
	}
	return ip
}
```

3. Update the `allow` method signature: change parameter name from `ip` to `key` (no logic change needed — it already uses string keys).

Remove the TODO comment at lines 145-148.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/middleware && go test -run TestRateLimiter_PerUserKey -v`
Expected: PASS

- [ ] **Step 5: Run all middleware tests**

Run: `cd internal/middleware && go test ./... -v`
Expected: All PASS (existing IP-based tests still work since unauthenticated requests use IP-only key)

- [ ] **Step 6: Wire userID into context from auth middleware**

In the existing auth middleware (grep for JWT user ID extraction), add:
```go
ctx = withUserID(ctx, claims.Subject)  // or claims.UserID, depending on claim name
```

Check `internal/middleware/auth.go` for the exact claim field name and add the context injection after successful JWT validation.

- [ ] **Step 7: Commit**

```bash
git add internal/middleware/ratelimit.go internal/middleware/ratelimit_test.go internal/middleware/auth.go
git commit -m "feat(FIX-096): add per-user rate limiting keyed on JWT user ID"
```

---

### Task 3: Update docs/todo.md

**Files:**
- Modify: `docs/todo.md`

- [ ] **Step 1: Mark FIX-093 and FIX-096 as completed**

Find both entries and mark `[x]` with date `2026-03-24`.

- [ ] **Step 2: Commit**

```bash
git add docs/todo.md
git commit -m "docs: mark FIX-093 and FIX-096 as completed"
```
