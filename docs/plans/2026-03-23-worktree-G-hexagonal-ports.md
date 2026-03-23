# Worktree G: Hexagonal Port Interfaces — Atomic Plan

> **Branch:** `refactor/hexagonal-port-interfaces`
> **Effort:** ~1d | **Findings:** 7 | **Risk:** Medium (import path changes)

---

## Task G1: Create LLM Port Interface (A-001, A-007, A-010)

**Create:** `internal/port/llm/provider.go`

```go
package llm

import "context"

// Provider abstracts LLM operations for the service layer.
// Implemented by internal/adapter/litellm.Client.
type Provider interface {
    ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error)
    ChatCompletionStream(ctx context.Context, req ChatCompletionRequest, onChunk func(StreamChunk)) (*ChatCompletionResponse, error)
    ListModels(ctx context.Context) ([]Model, error)
    Health(ctx context.Context) (bool, error)
    HealthDetailed(ctx context.Context) (*HealthStatusReport, error)
}
```

- [ ] Create `internal/port/llm/provider.go` with `Provider` interface
- [ ] Create `internal/port/llm/types.go` — copy/move type definitions (`Model`, `ChatCompletionRequest`, `ChatCompletionResponse`, `StreamChunk`, `ChatMessage`, `ToolDefinition`, `ToolCall`, `ToolCallFunction`, `ToolFunction`, `HealthStatusReport`, `ModelEndpoint`, `ModelHealth`) from `adapter/litellm/client.go`. Keep originals as type aliases pointing to port types.
- [ ] In `internal/adapter/litellm/client.go`: add type aliases `type Model = llm.Model` etc. so existing code continues to compile
- [ ] Verify: `go build ./...`

**Commit:** `refactor: create port/llm interface for LLM abstraction (A-001)`

---

## Task G2: Migrate Services from adapter/litellm to port/llm (A-010)

**Files:** `service/meta_agent.go`, `service/routing.go`, `service/review_router.go`, `service/model_registry.go`

For each file:
- [ ] Change import from `"github.com/Strob0t/CodeForge/internal/adapter/litellm"` to `"github.com/Strob0t/CodeForge/internal/port/llm"`
- [ ] Change field type from `*litellm.Client` to `llm.Provider`
- [ ] Update constructor to accept `llm.Provider` instead of `*litellm.Client`
- [ ] Update `cmd/codeforge/main.go` wiring to pass `litellmClient` (which satisfies `llm.Provider`)
- [ ] Verify: `go build ./...`
- [ ] Verify: `go test ./internal/service/... -count=1`

**Commit:** `refactor: migrate 4 services from adapter/litellm to port/llm (A-010)`

---

## Task G3: Create Metrics Port Interface (A-002)

**Create:** `internal/port/metrics/recorder.go`

```go
package metrics

import "context"

// Recorder abstracts observability metrics for the service layer.
// Implemented by internal/adapter/otel.Metrics.
type Recorder interface {
    RecordRunStarted(ctx context.Context, projectID, model string)
    RecordRunCompleted(ctx context.Context, projectID, model string)
    RecordRunFailed(ctx context.Context, projectID, model, reason string)
    RecordToolCall(ctx context.Context, projectID, toolName string)
    RecordRunDuration(ctx context.Context, projectID string, seconds float64)
    RecordRunCost(ctx context.Context, projectID string, cost float64)
}
```

- [ ] Create `internal/port/metrics/recorder.go`
- [ ] Add wrapper methods to `internal/adapter/otel/metrics.go` that satisfy the `Recorder` interface (currently fields are accessed directly — wrap in methods)
- [ ] Verify: `go build ./...`

**Commit:** `refactor: create port/metrics interface for observability (A-002)`

---

## Task G4: Migrate Services from adapter/otel to port/metrics

**Files:** `service/conversation.go`, `service/runtime.go`, `service/runtime_lifecycle.go`, `service/runtime_execution.go`

For each file:
- [ ] Change import from `cfotel "github.com/Strob0t/CodeForge/internal/adapter/otel"` to `"github.com/Strob0t/CodeForge/internal/port/metrics"`
- [ ] Change field type from `*cfotel.Metrics` to `metrics.Recorder`
- [ ] Replace direct field access (`m.RunsStarted.Add(...)`) with method calls (`m.RecordRunStarted(...)`)
- [ ] Update constructors and `cmd/codeforge/main.go` wiring
- [ ] Verify: `go build ./...`
- [ ] Verify: `go test ./internal/service/... -count=1`

**Commit:** `refactor: migrate 4 services from adapter/otel to port/metrics (A-002)`

---

## Task G5: Move Auth Types to Port Layer (A-003)

**Create:** `internal/port/subscription/provider.go`

- [ ] Move `SubscriptionProvider` interface, `DeviceCode`, `Token` structs, and error constants from `internal/adapter/auth/provider.go` to `internal/port/subscription/provider.go`
- [ ] In `internal/adapter/auth/provider.go`: add type aliases pointing to port types
- [ ] In `internal/service/subscription.go`: change import to `"github.com/Strob0t/CodeForge/internal/port/subscription"`
- [ ] Verify: `go build ./...`
- [ ] Verify: `go test ./internal/service/... -count=1`

**Commit:** `refactor: move auth provider types to port/subscription (A-003)`

---

## Verification

- [ ] `go build ./cmd/codeforge/`
- [ ] `go vet ./...`
- [ ] `go test ./... -count=1 -timeout=120s`
- [ ] Grep: `grep -r "adapter/litellm" internal/service/` should return 0 results (only test files allowed)
- [ ] Grep: `grep -r "adapter/otel" internal/service/` should return 0 results
