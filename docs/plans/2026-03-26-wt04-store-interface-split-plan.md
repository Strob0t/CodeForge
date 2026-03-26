# Worktree 4: refactor/store-interface-split — God Interface aufbrechen

**Branch:** `refactor/store-interface-split`
**Priority:** Hoch
**Scope:** 1 finding (F-ARC-001) — grösster Einzelrefactor
**Estimated effort:** Large (1-2 weeks)

## Research Summary

- [Mad Devs: Refactoring 130+ method interface](https://maddevs.io/blog/effective-refactoring-of-heavy-database-interface/) — step-by-step split by aggregate
- [rednafi: Revisiting Interface Segregation in Go](https://rednafi.com/go/interface_segregation/) — consumer-side interfaces
- DDD Repository Pattern: one interface per aggregate root (Three Dots Labs)
- Go embedding: composite `Store` interface embeds all role interfaces — zero-breakage refactor
- Partial mocks via struct embedding (Charles Kaminer pattern)

## Current State

- `internal/port/database/store.go`: 303 methods, 33 domain package imports
- `internal/adapter/postgres/Store`: already split into ~35 `store_*.go` files (good)
- 67 service files with 133 references to `database.Store`

## Migration Strategy (4 Phases, Incremental, Zero-Breakage)

### Phase 1: Define role interfaces (no consumers change)

Create new files in `internal/port/database/`:

| File | Interface | Methods | Domain Imports |
|---|---|---|---|
| `store_project.go` | `ProjectStore` | ~8 | project |
| `store_conversation.go` | `ConversationStore` | ~12 | conversation |
| `store_run.go` | `RunStore` | ~6 | run |
| `store_task.go` | `TaskStore` | ~5 | task |
| `store_agent.go` | `AgentStore` | ~5 | agent, resource |
| `store_plan.go` | `PlanStore` | ~9 | plan |
| `store_context.go` | `ContextStore` | ~10 | context |
| `store_cost.go` | `CostStore` | ~7 | cost, run |
| `store_benchmark.go` | `BenchmarkStore` | ~12 | benchmark |
| `store_user.go` | `UserStore` | ~6 | user |
| `store_auth_token.go` | `AuthTokenStore` | ~12 | user |
| `store_review.go` | `ReviewStore` | ~13 | review |
| `store_roadmap.go` | `RoadmapStore` | ~12 | roadmap |
| `store_mcp.go` | `MCPStore` | ~12 | mcp |
| `store_a2a.go` | `A2AStore` | ~15 | a2a |
| ... | ... | ... | ... |

In `store.go`, replace inline methods with embeddings:
```go
type Store interface {
    ProjectStore
    ConversationStore
    RunStore
    TaskStore
    AgentStore
    // ... all role interfaces
}
```

**Risk: ZERO.** The `Store` interface has the same method set. All existing code compiles unchanged.

### Phase 2: Update mocks/fakes to use embedding

- Any mock struct implementing `Store` can now embed role interfaces for partial mocking
- Update one test file at a time

### Phase 3: Narrow service dependencies (one service per commit)

Example:
```go
// Before:
func NewCostService(store database.Store) *CostService

// After:
func NewCostService(store database.CostStore) *CostService
```

The concrete `*postgres.Store` still satisfies `CostStore` implicitly. Wiring in `main.go` unchanged.

**Do one service per commit.** Each commit changes one service file and its tests.

### Phase 4: Move types out of store.go

- `A2ATaskFilter`, `A2APushConfig`, `AuditEntry` → respective domain packages or `port/database/types.go`

## Pitfalls to Avoid

1. **Don't split into separate packages** — keep all role interfaces in `port/database/` package (different `.go` files). Avoids import cycles.
2. **Don't over-segregate** — keep 5-15 methods per interface, grouped by domain aggregate.
3. **Don't big-bang** — one aggregate at a time.
4. **Keep composite `Store`** — `main.go` and integration tests need the full interface.

## Verification

- `go build ./...` passes at every phase
- All existing tests pass
- No new `database.Store` references added (grep check)
- Each service depends only on the role interfaces it uses (Phase 3)

## Sources

- [Mad Devs: Refactoring Heavy DB Interface](https://maddevs.io/blog/effective-refactoring-of-heavy-database-interface/)
- [rednafi: Interface Segregation in Go](https://rednafi.com/go/interface_segregation/)
- [Three Dots Labs: Repository Pattern in Go](https://threedots.tech/post/repository-pattern-in-go/)
- [Charles Kaminer: Breaking Down Large Interfaces with Embedding](https://medium.com/@cjkaminer/golang-breaking-down-large-interfaces-with-embedding-82a92bdcb02b)
- [Eli Bendersky: Embedding in Go Part 3](https://eli.thegreenplace.net/2020/embedding-in-go-part-3-interfaces-in-structs/)
- [Scott Hannen: Splitting Large Interfaces Is Easy](https://scotthannen.org/blog/2020/10/26/split-up-big-interfaces.html)
