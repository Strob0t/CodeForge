# Worktree L: Low-Priority Cleanup — Atomic Plan

> **Branch:** `chore/low-priority-cleanup`
> **Effort:** Opportunistic | **Findings:** 8 | **Risk:** Minimal

---

## Task L1: Use go:embed for VERSION File (Q-010)

**File:** `internal/version/version.go:22`

- [ ] Replace multi-path search with `//go:embed`:
```go
import _ "embed"

//go:embed VERSION
var embeddedVersion string
```
Note: This requires VERSION file to be accessible at build time. The current approach works with `go run` from any directory — `go:embed` requires the file relative to the package. Since VERSION is in root and this package is in `internal/version/`, this may not work directly. **Evaluate feasibility before changing.**

- [ ] If not feasible: add comment documenting why the multi-path search exists

**Commit:** `refactor: simplify VERSION file resolution (Q-010)` or `docs: document VERSION path search rationale (Q-010)`

---

## Task L2: Add Circular Import CI Check (A-011)

**File:** `.github/workflows/ci.yml`

- [ ] Add step to `test-go` job:
```yaml
- name: Check circular imports
  run: go vet ./... 2>&1 | grep -i "import cycle" && exit 1 || true
```

**Commit:** `ci: add circular import detection to Go test job (A-011)`

---

## Task L3: Improve CSP Nonce and img-src (S-011)

**File:** `internal/adapter/http/middleware.go:33`

- [ ] Remove `data:` from `img-src` if not needed:
```go
"img-src 'self';"
```
- [ ] If `data:` is needed (e.g., for inline images), document why

**Commit:** `fix: tighten CSP img-src directive (S-011)`

---

## Task L4: Fix Deferred Close Pattern (Q-009)

**File:** `internal/adapter/speckit/provider.go:99`

- [ ] Change from:
```go
defer f.Close() //nolint:errcheck
```
To:
```go
defer func() { _ = f.Close() }()
```

**Commit:** `fix: explicit error discard on deferred file close (Q-009)`

---

## Task L5: Document Python Worker Architecture (A-014)

**File:** Create or update `workers/codeforge/README.md`

- [ ] Document the Python worker module structure
- [ ] Note: not using hexagonal arch (intentional — workers are thin execution layer)

**Commit:** `docs: document Python worker architecture rationale (A-014)`

---

## Remaining (Minimal Impact, Optional)

| ID | Title | Action |
|---|---|---|
| S-006 | Path traversal mitigated | No action — already mitigated |
| Q-016 | Unused Handlers fields | Tracked by Worktree D (decomposition) |
| I-023 | Nginx health check | Verify `/health` returns minimal info (already does) |

---

## Verification

- [ ] `go build ./cmd/codeforge/`
- [ ] `go test ./... -count=1`
