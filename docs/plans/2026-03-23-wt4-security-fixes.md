# WT-4: Security Fixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix IPv6 SSRF gap, CORS wildcard logic flaw, missing RBAC on write endpoints, and document WebSocket token risk.

**Architecture:** Targeted fixes in netutil (SSRF), HTTP middleware (CORS), and route registration (RBAC). Each fix includes tests.

**Tech Stack:** Go 1.25, `net` stdlib, chi middleware, `internal/middleware`

**Best Practice:**
- SSRF: OWASP recommends checking ALL address families (IPv4 + IPv6 + mapped). Use `net.IP.To4()` to detect IPv4-mapped IPv6.
- CORS: Default-deny. Only allow wildcard when explicitly opted in.
- RBAC: Apply authorization at the routing level, not in handlers. Principle of least privilege.

---

### Task 1: Add IPv6 Ranges to SSRF Protection

**Files:**
- Modify: `internal/netutil/ssrf.go:12-27`
- Create: `internal/netutil/ssrf_test.go`

- [ ] **Step 1: Write tests for IPv6 SSRF protection**

```go
// internal/netutil/ssrf_test.go
package netutil

import (
    "net"
    "testing"
)

func TestIsPrivateIP(t *testing.T) {
    tests := []struct {
        name    string
        ip      string
        private bool
    }{
        // IPv4
        {"IPv4 private 10.x", "10.0.0.1", true},
        {"IPv4 private 172.16.x", "172.16.0.1", true},
        {"IPv4 private 192.168.x", "192.168.1.1", true},
        {"IPv4 loopback", "127.0.0.1", true},
        {"IPv4 link-local", "169.254.1.1", true},
        {"IPv4 public", "8.8.8.8", false},
        // IPv6
        {"IPv6 loopback", "::1", true},
        {"IPv6 link-local", "fe80::1", true},
        {"IPv6 ULA", "fc00::1", true},
        {"IPv6 ULA fd", "fd12:3456::1", true},
        {"IPv6 mapped loopback", "::ffff:127.0.0.1", true},
        {"IPv6 mapped private", "::ffff:10.0.0.1", true},
        {"IPv6 mapped public", "::ffff:8.8.8.8", false},
        {"IPv6 public", "2001:4860:4860::8888", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ip := net.ParseIP(tt.ip)
            if ip == nil {
                t.Fatalf("failed to parse IP %q", tt.ip)
            }
            got := IsPrivateIP(ip)
            if got != tt.private {
                t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
            }
        })
    }
}
```

- [ ] **Step 2: Run test to verify IPv6 cases fail**

```bash
go test ./internal/netutil/ -run TestIsPrivateIP -v
```
Expected: IPv6 ULA and mapped-loopback tests FAIL.

- [ ] **Step 3: Add IPv6 private ranges to IsPrivateIP**

```go
func IsPrivateIP(ip net.IP) bool {
    // Check IPv4-mapped IPv6 addresses (::ffff:x.x.x.x)
    if mapped := ip.To4(); mapped != nil {
        ip = mapped
    }

    privateRanges := []net.IPNet{
        // IPv4
        {IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
        {IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},
        {IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},
        {IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
        {IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)},
        {IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(32, 32)},
        // IPv6
        {IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},   // ULA
        {IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)},  // Link-local
    }
    for _, r := range privateRanges {
        if r.Contains(ip) {
            return true
        }
    }
    return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
```

- [ ] **Step 4: Run tests — all should pass**

```bash
go test ./internal/netutil/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/netutil/
git commit -m "fix: add IPv6 private ranges to SSRF protection (CWE-918)"
```

---

### Task 2: Fix CORS Wildcard Logic

**Files:**
- Modify: `internal/adapter/http/middleware.go:44-52`

- [ ] **Step 1: Write test for CORS behavior**

Create or update test that verifies:
- `APP_ENV=""` + `origin="*"` -> wildcard DENIED (not allowed)
- `APP_ENV="development"` + `origin="*"` -> wildcard allowed
- `APP_ENV="production"` + `origin="*"` -> wildcard DENIED

- [ ] **Step 2: Fix CORS logic — invert the condition**

Change line 46 from:
```go
if appEnv != "" && appEnv != "development" {
```
to:
```go
if appEnv != "development" {
```

This means: wildcard is ONLY allowed when `APP_ENV` is explicitly `"development"`. Any other value (empty, production, staging) rejects wildcard.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/adapter/http/... -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/http/middleware.go
git commit -m "fix: CORS wildcard only allowed with explicit APP_ENV=development (CWE-942)"
```

---

### Task 3: Add RequireRole Middleware to Write Endpoints

**Files:**
- Modify: `internal/adapter/http/routes.go`

- [ ] **Step 1: Identify all unprotected write endpoints**

These endpoints need `RequireRole(user.RoleAdmin, user.RoleEditor)`:
```
POST /llm/models                    -> AddLLMModel
POST /llm/models/delete             -> DeleteLLMModel
POST /policies                      -> CreatePolicyProfile
DELETE /policies/{name}             -> DeletePolicyProfile
POST /modes                         -> CreateMode
PUT /modes/{id}                     -> UpdateMode
DELETE /modes/{id}                  -> DeleteMode
POST /pipelines                     -> RegisterPipeline
POST /pipelines/{id}/instantiate    -> InstantiatePipeline
POST /projects/{id}/tasks           -> CreateTask
POST /tasks/{id}/claim              -> ClaimTask
POST /runs                          -> StartRun
POST /runs/{id}/cancel              -> CancelRun
POST /projects/{id}/plans           -> CreatePlan
POST /plans/{id}/start              -> StartPlan
POST /plans/{id}/cancel             -> CancelPlan
POST /scopes                        -> CreateScope
PUT /scopes/{id}                    -> UpdateScope
DELETE /scopes/{id}                 -> DeleteScope
```

Admin-only (destructive):
```
POST /llm/models/delete             -> Admin only
DELETE /policies/{name}             -> Admin only
DELETE /modes/{id}                  -> Admin only
DELETE /scopes/{id}                 -> Admin only
```

- [ ] **Step 2: Add RequireRole to each endpoint**

Wrap each route with the appropriate middleware:
```go
r.With(middleware.RequireRole(user.RoleAdmin, user.RoleEditor)).Post("/llm/models", h.AddLLMModel)
r.With(middleware.RequireRole(user.RoleAdmin)).Post("/llm/models/delete", h.DeleteLLMModel)
// ... etc
```

- [ ] **Step 3: Run existing E2E tests to verify no regressions**

```bash
go test ./internal/adapter/http/... -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/http/routes.go
git commit -m "fix: add RequireRole middleware to unprotected write endpoints (OWASP A01)"
```

---

### Task 4: Document WebSocket Token Risk

**Files:**
- Modify: `internal/middleware/auth.go` (add comment)

- [ ] **Step 1: Add accepted-risk documentation**

At the WebSocket auth section (lines 97-124), ensure the comment documents:
- Why token is in URL (browser limitation)
- Mitigations in place (HTTPS, short-lived tokens)
- What would change this assessment (longer token lifetimes, non-HTTPS)

- [ ] **Step 2: Commit**

```bash
git add internal/middleware/auth.go
git commit -m "docs: document WebSocket token-in-URL as accepted risk (CWE-598)"
```
