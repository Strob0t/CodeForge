# ADR-010: A2A Protocol Adoption for Agent Federation

> **Status:** accepted
> **Date:** 2026-02-28
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge orchestrates multiple AI coding agents that need to communicate with each other and with external agent systems. As the ecosystem matures, agents hosted by different organizations need to discover, negotiate with, and delegate tasks to each other. The system needed a standardized protocol for agent-to-agent communication that supports:

- Task delegation between heterogeneous agents (different runtimes, languages, capabilities)
- Agent discovery via machine-readable capability cards
- Secure cross-boundary communication with authentication and authorization
- Task lifecycle management (create, execute, cancel, monitor)
- Streaming results back to the requesting agent

CodeForge already uses MCP (Model Context Protocol) for tool integration, but MCP is designed for LLM-to-tool communication, not agent-to-agent coordination. A separate protocol was needed for federation.

### Decision

**Adopt A2A v0.3.0** (Agent-to-Agent Protocol, Linux Foundation, Apache 2.0) as the federation protocol for inter-agent communication.

CodeForge implements A2A in dual mode:
- **Server:** Exposes CodeForge agents as A2A-compatible services for inbound task requests
- **Client:** Connects to external A2A agents via the remote agent registry for outbound delegation

#### Key Implementation Details

- **SDK:** `a2a-go` for Go Core integration (official reference implementation)
- **Transport:** JSON-RPC 2.0 over HTTPS with SSE for streaming and push notifications
- **Discovery:** AgentCard builder generates machine-readable capability descriptions
- **Auth:** API Key, Bearer/JWT, OAuth 2.0, OIDC, mTLS, JWS (configurable per remote agent)
- **Routing:** `a2a://` prefix in handoff messages triggers A2A dispatch instead of internal NATS routing
- **Multi-tenant:** `/{tenant}/` path prefix isolates agent registries per tenant
- **Types:** Protobuf `lf.a2a.v1` definitions with 11 RPCs and 8 task states

#### Integration Architecture

```
Internal agents  -->  NATS (internal routing)
External agents  -->  A2A (federation routing)
HandoffMessage with a2a:// prefix --> A2A client --> remote agent
HandoffMessage without prefix     --> NATS      --> local agent
```

### Consequences

#### Positive

- Standards-based: A2A is a Linux Foundation project with broad industry adoption (Google, Salesforce, SAP)
- Complementary to MCP: "MCP for tools, A2A for agents" is a clean separation of concerns
- Multi-language SDKs (Go/Python/JS/Java/.NET) enable future polyglot federation
- AgentCard provides automatic capability negotiation without manual configuration
- Task lifecycle (8 states) maps naturally to CodeForge's existing run lifecycle

#### Negative

- Protocol is still pre-1.0 (v0.3.0-draft); breaking changes are possible
- A2A adds network latency compared to NATS for local agent communication. Mitigation: A2A is only used for cross-boundary federation; internal agents always use NATS
- Additional attack surface from accepting inbound A2A requests. Mitigation: auth middleware, SafeTransport for SSRF prevention, trust annotations on incoming messages

#### Neutral

- A2A and NATS coexist: the `a2a://` routing prefix makes the boundary explicit in code
- AgentCard generation is automatic from Mode definitions, requiring no extra configuration
- Remote agent registry is stored in PostgreSQL with tenant isolation

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Custom federation protocol | Full control, tailored to CodeForge | No ecosystem, no SDKs, maintenance burden, interop impossible | Reinventing the wheel contradicts project principles |
| AMP (Agent Messaging Protocol) | Ed25519 signatures, trust, 23 message blocks | Pre-draft (v0.1.2), single author, no SDK ecosystem, no industry backing | Too immature; patterns adopted for Phase 23 trust system instead |
| MCP for everything | Already integrated, one protocol | MCP is tool-oriented (request/response), not agent-oriented (task lifecycle, streaming, delegation) | Wrong abstraction level for agent-to-agent coordination |
| gRPC direct | Type-safe, fast, bidirectional streaming | Tight coupling, no discovery, no task lifecycle semantics, firewall-unfriendly | A2A provides the semantics; raw gRPC is just a transport |

### References

- [A2A Protocol Specification](https://google.github.io/A2A/) -- v0.3.0
- `internal/adapter/a2a/` -- Go A2A server and client implementation
- `internal/domain/orchestration/handoff.go` -- HandoffMessage with `a2a://` routing
- `internal/service/handoff.go` -- Handoff dispatch logic (NATS vs A2A)
- `docs/features/04-agent-orchestration.md` -- A2A integration section
