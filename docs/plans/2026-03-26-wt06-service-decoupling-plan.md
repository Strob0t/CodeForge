# Worktree 6: refactor/service-decoupling — Interface Segregation in Services

**Branch:** `refactor/service-decoupling`
**Priority:** Mittel
**Scope:** 3 findings (F-ARC-004, F-ARC-013, F-ARC-012)
**Estimated effort:** Medium (1 week)

## Research Summary

- Go idiom: "Accept interfaces, return structs" (Dave Cheney, Go Code Review Comments)
- Consumer-defined interfaces: define at consumer, not producer
- Setter → constructor injection: eliminates nil-pointer panics from missed setters
- Break circular deps with interfaces or callback functions

## Steps

### 1. F-ARC-004: Replace concrete service pointers with consumer-defined interfaces

**Target services (most setters):**
- `ConversationService` (20+ fields, 16 setters)
- `RuntimeService` (18+ fields, 10+ setters)
- `ContextOptimizerService` (6 setters)

**Pattern per service:**

```go
// In conversation.go (the consumer):
type modelResolver interface {
    BestModel() string
    ResolveModel(requested string) string
}
type policyEvaluator interface {
    Evaluate(ctx context.Context, req policy.EvalRequest) policy.Decision
}

type ConversationService struct {
    models modelResolver      // was: *ModelRegistry
    policy policyEvaluator    // was: *PolicyService
}
```

**Migration (per field, per commit):**
1. Define interface next to consumer
2. Change field type from `*XxxService` to interface
3. Setter still works: `s.models = r` (concrete type satisfies interface)
4. Later: consolidate setters into constructor `Deps` struct

### 2. F-ARC-013: Service config isolation

Replace `*config.Runtime` (with 20+ fields) with service-specific config:

```go
type RuntimeConfig struct {
    MaxSteps      int
    DefaultModel  string
    ContextTokens int
}
```

Follow existing pattern from `SandboxConfig` in `sandbox.go`.

### 3. F-ARC-012: Typed broadcast payloads

Replace `BroadcastEvent(ctx, eventType string, payload any)` with:

```go
type Payload interface{ isBroadcastPayload() }
func BroadcastEvent(ctx context.Context, eventType string, payload Payload)
```

Or use the existing typed event structs in `event/` package and enforce their use.

## Migration Order

1. Start with `RuntimeService` (most consumed, clearest dependencies)
2. Then `ConversationService`
3. Then smaller services
4. Config isolation last (lower priority)

## Sources

- [rednafi: Interface Segregation in Go](https://rednafi.com/go/interface_segregation/)
- [ByteGoblin: Accept Interfaces Return Structs](https://bytegoblin.io/blog/accept-interfaces-return-structs-in-go)
- [devtrovert: Define Interfaces in Consumer Package](https://blog.devtrovert.com/p/go-ep2-define-interfaces-in-the-consumer)
- [Martin Fowler: Inversion of Control and DI](https://martinfowler.com/articles/injection.html)
