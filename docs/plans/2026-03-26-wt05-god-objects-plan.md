# Worktree 5: refactor/god-objects — Handlers, main.go, StartSubscribers

**Branch:** `refactor/god-objects`
**Priority:** Mittel
**Scope:** 4 findings (F-ARC-002, F-ARC-003, F-ARC-015, F-ARC-010)
**Estimated effort:** Large (1-2 weeks)

## Research Summary

- Chi idiomatic pattern: per-domain handler structs with `Routes()` method + `Mount()`
- Pure DI composition root: extract phase functions, no framework needed
- `oklog/run.Group` for managing goroutine lifecycles
- Dave Cheney SOLID Go Design: consumer-defined interfaces

## Steps

### 1. F-ARC-002: Decompose Handlers (77 fields → handler groups)

**Pattern:** Per-domain handler struct + `Routes() chi.Router`

Already started: `ProjectHandlers`, `AgentHandlers`, `TaskHandlers`, `RunHandlers`, `PolicyHandlers`, `UtilityHandlers`. Remaining ~60 fields need extraction.

**Next groups to extract (by dependency cluster):**
1. `ConversationHandlers` (Conversation, Message, Search, Channel services)
2. `BenchmarkHandlers` (Benchmark, Evaluation services)
3. `ReviewHandlers` (Review, Boundary, ReviewTrigger services)
4. `RoadmapHandlers` (Roadmap, PM services)
5. `LLMHandlers` (LLMKey, Model, Routing services)
6. `IntelligenceHandlers` (Context, RepoMap, Retrieval, Graph, Skill services)
7. `SecurityHandlers` (Auth, GDPR, Tenant, VCSAccount services)
8. `OrchestrationHandlers` (Orchestrator, MetaAgent, Mode, AutoAgent services)
9. `DevToolsHandlers` (LSP, MCP, File services)

**Per group:** Create struct with only needed deps, define consumer-side interfaces, move methods, add `Routes()`, mount in `MountRoutes()`.

### 2. F-ARC-015: Decompose MountRoutes (608 LOC)

Extract per-domain mounting into `Routes()` methods on handler groups (follows naturally from step 1). `MountRoutes()` becomes a thin orchestrator calling `api.Mount("/conversations", convHandlers.Routes())` etc.

### 3. F-ARC-003: Decompose main.go:run() (1046 LOC)

**Extract these phases:**

```go
func run() error {
    cfg, logCloser, err := loadConfig()           // ~30 lines
    defer logCloser()

    infra, cleanup, err := initInfra(cfg)          // ~60 lines (PG, NATS, Cache, Git, OTEL)
    defer cleanup()

    services, err := initServices(cfg, infra)      // ~400 lines → sub-phases
    handlers := buildHandlers(cfg, services)        // ~100 lines
    router := buildRouter(cfg, handlers)            // ~85 lines
    mcpSrv := startMCPServer(cfg, infra.store)     // ~20 lines

    return serve(ctx, cfg, router, infra, mcpSrv)  // ~100 lines (signals, shutdown)
}
```

**Sub-phases for `initServices`:**
- `initCoreServices(cfg, infra)` — Project, Agent, Task, Run, Policy, Mode
- `initConversationServices(cfg, infra, core)` — Conversation, Prompt, History
- `initIntelligenceServices(cfg, infra)` — Context, RepoMap, Retrieval, Graph
- `initOrchestrationServices(cfg, infra, core, conv)` — Orchestrator, MetaAgent, Runtime

**Use intermediate structs:**
```go
type infrastructure struct {
    pool    *pgxpool.Pool
    queue   *cfnats.Queue
    store   *postgres.Store
    hub     *ws.Hub
    metrics cfmetrics.Recorder
    // ...
}
```

### 4. F-ARC-010: Decompose StartSubscribers (223 LOC)

Extract each subscriber's handler into a named method:
- `handleRunComplete(ctx, msg)`
- `handleTrajectoryEvent(ctx, msg)` — currently 114 lines inline
- `handleHeartbeat(ctx, msg)`
- etc.

## Migration Order

1. **StartSubscribers** first — smallest, self-contained
2. **MountRoutes + Handler groups** — one group per PR
3. **main.go phases** — bottom-up: `initInfra()` → `buildRouter()` → `buildHandlers()` → `initServices()`

## Verification

- `go build ./...` at every step
- All handler tests pass (test one group at a time)
- `run()` function under 50 lines at completion

## Sources

- [Chi: todos-resource example](https://github.com/go-chi/chi/blob/master/_examples/todos-resource/main.go)
- [Maragu.dk: Structuring HTTP handlers in Go](https://www.maragu.dk/blog/structuring-and-testing-http-handlers-in-go)
- [Rost Glukhov: DI in Go](https://www.glukhov.org/post/2025/12/dependency-injection-in-go/)
- [Dave Cheney: SOLID Go Design](https://dave.cheney.net/2016/08/20/solid-go-design)
