# ADR-014: Store Interface Segregation Plan

> **Status:** accepted
> **Date:** 2026-03-27
> **Deciders:** Architecture audit remediation

### Context

The `database.Store` interface composes 34 sub-interfaces (~278 methods total). Every service constructor accepts this composite interface, giving each service access to all 278 methods regardless of what it actually uses. This violates the Interface Segregation Principle (ISP) and makes it impossible to understand a service's data dependencies from its constructor signature.

The sub-interfaces themselves (ProjectStore, RunStore, AuditStore, etc.) are already well-decomposed into cohesive groups of 2-15 methods each. The problem is only at the composition level.

### Decision

1. **The composite `Store` interface is used only at the composition root** (`cmd/codeforge/main.go`) for wiring the concrete PostgreSQL implementation to services.
2. **New services must accept only the sub-interfaces they need**, not the composite `Store`. Example: `NewGDPRService(store interface{ database.UserStore; database.AuditStore })`.
3. **Existing services will be migrated incrementally** — each service's constructor signature is updated to accept only its required sub-interfaces when that service is next modified for other reasons. No big-bang refactor.
4. **Consumer-defined interfaces** (defined in the service file, not in the port package) are the preferred pattern for cross-cutting needs that span 2-3 sub-interfaces.

### Consequences

#### Positive

- Each service's data dependencies are explicit in its constructor signature
- Mock stores in tests only need to implement the methods the service uses (simpler, faster tests)
- IDE autocompletion shows only relevant methods
- Easier to reason about service boundaries for future microservice extraction

#### Negative

- More verbose constructor signatures for services that need many sub-interfaces
- Migration is incremental — both patterns coexist during transition
- Some services genuinely need 5+ sub-interfaces; their constructors will be longer

#### Neutral

- The composite `Store` interface remains available for the composition root
- No behavioral change — all services receive the same concrete implementation at runtime
- The 34 existing sub-interfaces remain unchanged

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Delete composite Store entirely | Forces ISP immediately | Big-bang refactor touching 40+ files and all mocks; high risk | Too risky for a single PR |
| Use Google Wire for DI | Automated wiring, compile-time safety | New dependency, learning curve, overkill at current scale | Manual DI is simpler and sufficient |
| Keep composite Store as-is | Zero effort | ISP violation persists, mocks bloated | Accepted as technical debt |
| Domain-specific repositories | Full hexagonal purity | Requires new repository layer between service and store | Over-engineering for current scale |

### References

- Dave Cheney: SOLID Go Design — Interface Segregation
- Go idiom: "accept interfaces, return structs"
- Audit finding F-006: database.Store god interface (278 methods)
- Audit finding F-017: RuntimeService (54 methods), ConversationService (53 methods)
