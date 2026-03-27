# ADR-013: Service Layer Config Sub-Struct Imports (Accepted)

> **Status:** accepted
> **Date:** 2026-03-27
> **Deciders:** Project lead + Claude Code audit analysis

### Context

An architecture audit (finding F-035) identified that 19+ service files in `internal/service/` import `internal/config` to reference config sub-struct types (e.g., `config.Auth`, `config.Runtime`, `config.Orchestrator`).

In strict hexagonal architecture, the config package is an outer-layer concern (infrastructure/adapter layer), while services belong to the inner layer (domain/application). Importing an outer-layer package from the inner layer technically violates the dependency rule that says inner layers must not depend on outer layers.

The question is whether this import represents a real architectural violation that should be refactored, or a pragmatic trade-off that can be explicitly accepted.

### Decision

**Accepted as a pragmatic trade-off.** The config sub-struct imports in the service layer are permitted under the following conditions:

1. **Config sub-structs are value types.** They contain only primitive fields and nested value types. They perform no I/O, have no side effects, and hold no references to external systems.

2. **Services receive sub-structs via constructors.** Every service accepts its config as a constructor parameter (e.g., `NewAuthService(store, *config.Auth)`). Services never call `config.Load()` or `config.LoadWithCLI()` directly. The wiring happens in `cmd/codeforge/main.go`, which is the outer layer.

3. **The config package contains only type definitions used across layers.** It defines struct shapes and defaults -- it is not an adapter, does not implement ports, and does not perform runtime operations beyond initial loading (which is only invoked from `main.go`).

This pattern is consistent with how the Go standard library treats types like `http.Server` (a config struct passed into services) and aligns with the project's principle that "practicality beats purity" (PEP 20).

### Invariant

**Services MUST NOT import config loader functions, only type definitions.**

Specifically:
- Allowed: `config.Auth`, `config.Runtime`, `config.Orchestrator`, etc. (sub-struct types)
- Forbidden: `config.Load()`, `config.LoadWithCLI()`, `config.Defaults()` (loader/factory functions)

If a service needs to reload or modify configuration at runtime, it must go through a port interface, not import the config loader.

### Consequences

#### Positive

- **Simplicity:** No indirection layer between config types and their consumers. Developers can follow the type from constructor to definition in one jump.
- **No boilerplate:** Avoids duplicating every config sub-struct in a separate `types` or `domain` package, which would add maintenance burden with zero behavioral benefit.
- **Constructor injection is clean:** `NewFoo(cfg *config.Foo)` is immediately clear about what configuration the service needs.

#### Negative

- **Technically an outer-layer import in inner layer:** A strict hexagonal purist would flag this. The import creates a compile-time dependency from service to config package.
- **Precedent risk:** Developers might extend the pattern to import config loader functions, violating the invariant. Code review and linting must enforce the boundary.

#### Neutral

- **Acceptable given current clean usage pattern:** All 47 files that import config follow the constructor-injection pattern consistently. No file calls `config.Load()`.
- **If config sub-structs grow complex** (e.g., methods with side effects, interfaces), this decision should be revisited.

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Shared types package (`internal/configtypes/`) | Pure hexagonal compliance; config types in a layer-neutral package | Duplicates definitions; two packages to keep in sync; adds indirection for no behavioral change | Overhead exceeds benefit for value-only structs |
| Service-specific option types | Each service defines its own config struct; no cross-layer import | Massive duplication (47+ services); wiring in main.go becomes a mapping exercise; drift risk between service options and actual config | Violates DRY; maintenance cost too high |
| Functional options pattern (`WithTimeout(d)`) | Idiomatic Go; no struct import needed | Verbose for services with 5+ config fields; loses struct documentation; harder to test default values | Appropriate for libraries, not internal services with known config shapes |
| Accept and document (this decision) | Zero code change; explicitly recorded; invariant enforced via review | Does not satisfy strict hexagonal purists | **Chosen** -- pragmatic, documented, with clear guardrails |

### References

- Audit finding: F-035 (universal audit report, 2026-03-27)
- `internal/config/config.go` -- Config struct and sub-struct definitions
- `internal/config/loader.go` -- Load/LoadWithCLI (outer-layer only)
- `internal/service/auth.go` -- Example: `NewAuthService(store, *config.Auth)`
- `internal/service/runtime.go` -- Example: constructor receives config sub-struct
- ADR-003: Hierarchical Configuration System
