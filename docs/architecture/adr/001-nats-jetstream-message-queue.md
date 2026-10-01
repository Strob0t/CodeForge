# ADR-001: NATS JetStream as Message Queue

> **Status:** accepted; delivery, acknowledgement and retry semantics refined by [ADR-016](016-nats-delivery-semantics.md) (2026-09-30)
> **Date:** 2026-02-14
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge needs a message queue between the Go Core Service and Python AI Workers for:
- Job dispatch (Go to Python): Task assignments to agent workers
- Result streaming (Python to Go): Status updates, logs, agent results
- Fan-out: Real-time updates to multiple WebSocket clients
- Scaling: Multiple Python workers consuming from the same queue
- Reliability: At-least-once delivery, no lost tasks

The two main candidates were NATS JetStream and Redis Streams.

### Decision

**NATS JetStream** is the message queue for CodeForge.

Go Core and Python Workers communicate exclusively through NATS:
- Go Core publishes tasks to subjects like `tasks.agent.aider`, `tasks.agent.openhands`
- Python Workers subscribe as JetStream consumers with explicit acknowledgment
- Result streaming via `tasks.result` (shared subject; the per-task-id pattern `results.{task_id}` was superseded)
- Real-time fan-out for WebSocket updates via standard NATS pub/sub

> **Note (Phase 17+):** The primary subjects have evolved to `conversation.run.start/complete` and `runs.start` for the agentic conversation loop. The `tasks.agent.*` pattern above reflects the original design; both co-exist in the current codebase.

#### Key Reasons

- Go-native: NATS is written in Go; `nats.go` is the reference client, making it an ideal fit for the Go Core
- Purpose-built messaging: Designed for microservice communication, not a data structure server with messaging bolted on
- Subject-based routing: Natural mapping to agent backends (`tasks.agent.{backend}`) and task types
- Request-Reply: First-class pattern for synchronous task dispatch when needed
- JetStream persistence: At-least-once delivery, consumer groups, replay, and built-in KV store
- Lightweight: ~20MB binary, minimal RAM, millisecond startup
- LiteLLM does NOT require Redis: LiteLLM uses in-memory caching for single-instance deployments (Redis is only needed for multi-instance production at >1000 RPS)

#### Configuration

```yaml
# docker-compose.yml
services:
  nats:
    image: nats:2-alpine
    ports:
      - "4222:4222"   # Client connections
      - "8222:8222"   # HTTP monitoring
    command: ["--jetstream", "--store_dir", "/data", "-m", "8222"]
    volumes:
      - nats_data:/data
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8222/healthz"]
```

#### Client Libraries

| Layer | Library | Notes |
|---|---|---|
| Go Core | `nats.go` + `nats.go/jetstream` | Official, reference implementation |
| Python Workers | `nats-py` | Official, asyncio-native |

> **Implementation status (2026-09-29):** One JetStream stream `CODEFORGE` (30-day `MaxAge`, `internal/adapter/nats/nats.go`) carries all subjects. Every Go subscription is a shared durable consumer (deliver group `codeforge-go`), including notification subjects such as `tasks.output`; there are no plain core-NATS subscriptions for fan-out yet. JetStream KV holds the HTTP idempotency keys. Delivery defects (durable consumers replay the stream after Go Core downtime, long Python runs are redelivered, cancel listeners leak): see [Known Issues](../../todo.md#known-issues) KI-18; the Python retry/DLQ path is unreachable: KI-19.

> **Update (2026-10-01):** the L2 cache bucket is gone: the tiered cache (ristretto L1 + NATS KV L2) was never used by a service and was removed with its `cache.*` settings (KI-60). The delivery defects named above were fixed in S2 ([ADR-016](016-nats-delivery-semantics.md)). No `review.*` subject remains, so the stream no longer lists `review.>`.

### Consequences

#### Positive

- Single-purpose tool: NATS does messaging extremely well
- Subject-based routing eliminates manual routing logic
- JetStream KV can serve as lightweight state store (agent sessions, config cache)
- Built-in monitoring on port 8222 (no extra tooling needed)
- Excellent clustering support (RAFT consensus) if scaling is needed later
- Throughput far exceeds our needs (~11M msgs/sec core, ~1M persistent)

#### Negative

- One more service in Docker Compose (but lightweight at ~20MB)
- Less general knowledge than Redis (fewer Stack Overflow answers, though NATS docs are excellent)
- If Redis is ever needed for LiteLLM multi-instance, there would be two infrastructure services

#### Neutral

- JetStream KV is not a replacement for PostgreSQL, it handles lightweight ephemeral state only
- NATS Pub/Sub (non-persistent) can be used alongside JetStream for fire-and-forget events

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Redis Streams | Broad ecosystem, multi-purpose (cache + queue + pub/sub), good client libraries | Not a purpose-built message queue, no subject-based routing, no request-reply pattern, heavier memory footprint | LiteLLM doesn't require Redis for single-instance, so "already in the stack" argument doesn't apply |
| RabbitMQ | Mature, feature-rich, AMQP standard | Heavy (Erlang VM), complex configuration, overkill for our use case | Too heavyweight for a containerized dev tool |
| Direct gRPC | No middleware, type-safe, fast | No persistence, no fan-out, tight coupling, no consumer groups | Doesn't support our scaling or reliability requirements |

### References

- [NATS Documentation](https://docs.nats.io/)
- [JetStream Documentation](https://docs.nats.io/nats-concepts/jetstream)
- [nats.go Client](https://github.com/nats-io/nats.go)
- [nats-py Client](https://github.com/nats-io/nats.py)
- [LiteLLM Caching Docs](https://docs.litellm.ai/docs/caching) (Redis optional for single-instance)
