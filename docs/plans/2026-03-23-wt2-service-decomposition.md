# WT-2: Service God Object Decomposition — Implementation Plan (PRIORITY)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose the 3 largest God Objects: BenchmarkService (36 methods, 1077 LOC), ConversationService (42 methods, 1568 LOC), AuthService (24 methods, 761 LOC). Also unify ContextBudget into a strategy interface.

**Architecture:** Each God Object is split into focused sub-services. The original service becomes a thin orchestrator that composes the sub-services. Setter injection pattern is preserved for backward compatibility during transition. Each sub-service is independently testable.

**Tech Stack:** Go 1.25, existing port interfaces, NATS, PostgreSQL

**Best Practice:** SRP — each service should have one reason to change. The existing TODO in `benchmark.go:27` confirms this decomposition is overdue. Use composition: the orchestrator delegates to sub-services, not inheritance.

---

## Task 1: Decompose BenchmarkService (36 methods -> 4 sub-services)

**Files:**
- Create: `internal/service/benchmark_suite.go`
- Create: `internal/service/benchmark_run.go`
- Create: `internal/service/benchmark_result.go`
- Create: `internal/service/benchmark_watchdog.go`
- Modify: `internal/service/benchmark.go` (becomes orchestrator)
- Modify: `cmd/codeforge/main.go`

### Sub-Service 1: BenchmarkSuiteService (suite CRUD + datasets)

- [ ] **Step 1: Create BenchmarkSuiteService**

```go
// internal/service/benchmark_suite.go
package service

type BenchmarkSuiteService struct {
    store       database.Store
    datasetsDir string
}

func NewBenchmarkSuiteService(store database.Store, datasetsDir string) *BenchmarkSuiteService {
    return &BenchmarkSuiteService{store: store, datasetsDir: datasetsDir}
}
```

- [ ] **Step 2: Move methods**

Move from benchmark.go:
- `SeedDefaultSuites`
- `RegisterSuite`
- `GetSuite`
- `ListSuites`
- `UpdateSuite`
- `DeleteSuite`
- `ListDatasets`

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/service/...
```

### Sub-Service 2: BenchmarkRunManager (run lifecycle)

- [ ] **Step 4: Create BenchmarkRunManager**

```go
// internal/service/benchmark_run.go
package service

type BenchmarkRunManager struct {
    store      database.Store
    queue      messagequeue.Queue
    hub        broadcast.Broadcaster
    routingSvc *RoutingService
    suiteSvc   *BenchmarkSuiteService
}
```

- [ ] **Step 5: Move methods**

Move: `CreateRun`, `StartRun`, `GetRun`, `ListRuns`, `ListRunsFiltered`, `UpdateRun`, `DeleteRun`, `resolveDatasetPath` (private helper)

### Sub-Service 3: BenchmarkResultAggregator (results + compare + export)

- [ ] **Step 6: Create BenchmarkResultAggregator**

```go
// internal/service/benchmark_result.go
package service

type BenchmarkResultAggregator struct {
    store database.Store
}
```

- [ ] **Step 7: Move methods**

Move: `ListResults`, `Compare`, `CompareMulti`, `CostAnalysis`, `Leaderboard`, `ExportTrainingPairs`, `ExportRLVRDataset`
Move helpers: `sortLeaderboard`, `resultToTrainingEntry`, `ComputeRLVRReward`, `avgFromMap`, `avgScoreFromJSON`

### Sub-Service 4: BenchmarkWatchdog

- [ ] **Step 8: Create BenchmarkWatchdog**

```go
// internal/service/benchmark_watchdog.go
package service

type BenchmarkWatchdog struct {
    store database.Store
}
```

- [ ] **Step 9: Move methods**

Move: `RunWatchdogOnce`, `StartWatchdog`

### Orchestrator: BenchmarkService (thin wrapper)

- [ ] **Step 10: Reduce benchmark.go to orchestrator**

```go
type BenchmarkService struct {
    Suites   *BenchmarkSuiteService
    Runs     *BenchmarkRunManager
    Results  *BenchmarkResultAggregator
    Watchdog *BenchmarkWatchdog
    queue    messagequeue.Queue
    hub      broadcast.Broadcaster
}
```

Keep NATS handlers in orchestrator: `HandleBenchmarkRunResult`, `HandleBenchmarkTaskStarted`, `HandleBenchmarkTaskProgress`, `StartResultSubscriber` — these dispatch to sub-services.

- [ ] **Step 11: Update main.go wiring**

```go
suiteSvc := service.NewBenchmarkSuiteService(store, cfg.Benchmark.DatasetsDir)
runMgr := service.NewBenchmarkRunManager(store, suiteSvc)
resultAgg := service.NewBenchmarkResultAggregator(store)
watchdog := service.NewBenchmarkWatchdog(store)
benchmarkSvc := service.NewBenchmarkService(suiteSvc, runMgr, resultAgg, watchdog)
benchmarkSvc.SetQueue(queue)
benchmarkSvc.SetHub(hub)
benchmarkSvc.SetRoutingService(routingSvc)
```

- [ ] **Step 12: Update handler references**

In `handlers_benchmark.go`, update calls like `h.Benchmarks.ListSuites` to `h.Benchmarks.Suites.ListSuites` etc.

- [ ] **Step 13: Run tests + lint**

```bash
go test ./internal/service/... ./internal/adapter/http/... -count=1
golangci-lint run ./internal/...
```

- [ ] **Step 14: Commit**

```bash
git add internal/service/benchmark*.go cmd/codeforge/main.go internal/adapter/http/handlers_benchmark*.go
git commit -m "refactor: decompose BenchmarkService into 4 focused sub-services (F-025/F-027)"
```

---

## Task 2: Decompose ConversationService (42 methods -> 3 sub-services)

**Files:**
- Create: `internal/service/conversation_messages.go`
- Create: `internal/service/conversation_prompt.go`
- Modify: `internal/service/conversation.go` (CRUD only)
- Modify: `internal/service/conversation_agent.go` (agentic orchestrator)
- Modify: `cmd/codeforge/main.go`

### Sub-Service 1: ConversationMessageService

- [ ] **Step 1: Create ConversationMessageService**

```go
// internal/service/conversation_messages.go
package service

type ConversationMessageService struct {
    store database.Store
    queue messagequeue.Queue
    hub   broadcast.Broadcaster
}
```

- [ ] **Step 2: Move message-related methods from conversation.go**

Move: `ListMessages`, `SearchMessages`, `ClearConversation`, `CompactConversation`, `HandleCompactComplete`, `StartCompactSubscriber`

### Sub-Service 2: PromptAssemblyService (system prompt + context)

- [ ] **Step 3: Create PromptAssemblyService**

```go
// internal/service/conversation_prompt.go
package service

type PromptAssemblyService struct {
    store           database.Store
    contextOpt      *ContextOptimizerService
    goalSvc         *GoalDiscoveryService
    modeSvc         *ModeService
    promptAssembler *PromptAssembler
    events          eventstore.Store
    agentCfg        *config.Agent
}
```

- [ ] **Step 4: Move prompt/context methods from conversation_agent.go**

Move: `buildSystemPrompt`, `buildConversationContextEntries`, `evaluateReminders`, `computeBudget`, `historyToPayload`, `resolveProviderAPIKey`
Move standalone functions: `ExtractModelFamily`, `appendModelAdaptation`, `detectStackSummary`

### Keeper: ConversationService (CRUD) + AgenticFlowService (orchestration)

- [ ] **Step 5: Simplify conversation.go to CRUD only**

Keep: `Create`, `Get`, `ListByProject`, `Delete`, `SendMessage` (simple non-agentic), `SetMode`, `SetModel`

- [ ] **Step 6: conversation_agent.go becomes AgenticFlowService**

Rename conceptually (can keep same file). Inject `ConversationMessageService` and `PromptAssemblyService`:
- `SendMessageAgentic` calls `promptSvc.buildSystemPrompt()` instead of `cs.buildSystemPrompt()`
- `HandleConversationRunComplete` calls `msgSvc.` for message storage

- [ ] **Step 7: Update main.go wiring**

```go
msgSvc := service.NewConversationMessageService(store, queue, hub)
promptSvc := service.NewPromptAssemblyService(store, contextOptSvc, goalSvc, modeSvc, assembler, eventStore, &cfg.Agent)
conversationSvc := service.NewConversationService(store, hub, conversationModel, modeSvc)
conversationSvc.SetMessageService(msgSvc)
conversationSvc.SetPromptService(promptSvc)
```

- [ ] **Step 8: Run tests + lint**

```bash
go test ./internal/service/... -count=1
golangci-lint run ./internal/...
```

- [ ] **Step 9: Commit**

```bash
git add internal/service/conversation*.go cmd/codeforge/main.go
git commit -m "refactor: decompose ConversationService into CRUD + Messages + Prompt + Agentic (F-025/F-031)"
```

---

## Task 3: Decompose AuthService (24 methods -> 3 sub-services)

**Files:**
- Create: `internal/service/auth_token.go`
- Create: `internal/service/auth_apikey.go`
- Modify: `internal/service/auth.go` (keep login/register + compose sub-services)
- Modify: `cmd/codeforge/main.go`

### Sub-Service 1: TokenManager

- [ ] **Step 1: Create TokenManager**

```go
// internal/service/auth_token.go
package service

type TokenManager struct {
    store  database.Store
    secret []byte
    cfg    *config.Auth
}
```

- [ ] **Step 2: Move token methods**

Move: `RefreshTokens`, `Logout`, `RevokeAccessToken`, `ValidateAccessToken`, `StartTokenCleanup`
Move private: `signJWT`, `verifyJWT`, `base64URLEncode`, `base64URLDecode`

### Sub-Service 2: APIKeyManager

- [ ] **Step 3: Create APIKeyManager**

```go
// internal/service/auth_apikey.go
package service

type APIKeyManager struct {
    store database.Store
}
```

- [ ] **Step 4: Move API key methods**

Move: `CreateAPIKey`, `ListAPIKeys`, `DeleteAPIKey`, `ValidateAPIKey`

### Keeper: AuthService (login + register + user CRUD + compose)

- [ ] **Step 5: Update AuthService to compose sub-services**

```go
type AuthService struct {
    store   database.Store
    cfg     *config.Auth
    tokens  *TokenManager
    apiKeys *APIKeyManager
}
```

Keep in auth.go: `Register`, `Login`, `ListUsers`, `GetUser`, `UpdateUser`, `DeleteUser`, `ChangePassword`, `RequestPasswordReset`, `ConfirmPasswordReset`, `GetSetupStatus`, `BootstrapAdmin`, `SeedDefaultAdmin`, `AdminResetPassword`

Delegate: `auth.ValidateAccessToken()` -> `auth.tokens.ValidateAccessToken()`

- [ ] **Step 6: Update callers**

`middleware/auth.go` calls `authSvc.ValidateAccessToken()` — this stays working via delegation method on AuthService.

- [ ] **Step 7: Run tests + lint + commit**

```bash
go test ./internal/service/... ./internal/middleware/... -count=1
git add internal/service/auth*.go cmd/codeforge/main.go
git commit -m "refactor: decompose AuthService into Auth + TokenManager + APIKeyManager (F-025)"
```

---

## Task 4: Create BudgetCalculator Strategy Interface

**Files:**
- Modify: `internal/service/context_budget.go`

- [ ] **Step 1: Add BudgetCalculator interface**

```go
type BudgetCalculator interface {
    Calculate(baseBudget int, ctx BudgetContext) int
}

type BudgetContext struct {
    ModeID   string
    Tier     string
    History  []messagequeue.ConversationMessagePayload
}
```

- [ ] **Step 2: Wrap existing functions as implementations**

```go
type PhaseAwareBudget struct{}
func (PhaseAwareBudget) Calculate(base int, ctx BudgetContext) int {
    return PhaseAwareContextBudget(base, ctx.ModeID)
}

type ComplexityBasedBudget struct{}
func (ComplexityBasedBudget) Calculate(base int, ctx BudgetContext) int {
    return ComplexityBudget(ctx.Tier, base)
}

type AdaptiveBudget struct{}
func (AdaptiveBudget) Calculate(base int, ctx BudgetContext) int {
    return AdaptiveContextBudget(base, ctx.History)
}
```

Keep the existing exported functions for backward compatibility.

- [ ] **Step 3: Commit**

```bash
git add internal/service/context_budget.go
git commit -m "refactor: add BudgetCalculator strategy interface (F-033)"
```
