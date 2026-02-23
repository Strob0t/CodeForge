# ADR-004: Async Logging with Buffered Channel + Worker Pool

> **Status:** accepted
> **Date:** 2026-02-17
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge's Go Core handles concurrent HTTP requests, WebSocket connections, NATS message processing, and agent lifecycle management. Synchronous logging (`slog.Handler` writing to stdout) blocks the calling goroutine on every log call. Under load, this creates back-pressure from I/O that slows down request processing.

The Python Workers face the same issue: synchronous logging in an asyncio event loop blocks the entire loop during I/O writes.

Requirements:
- Logging must never block the hot path (HTTP handlers, NATS subscribers)
- Log records must not be silently lost under normal operation
- Graceful shutdown must drain buffered logs before exit
- The solution must use Go's standard `log/slog` (no external logging libraries)

### Decision

**Async logging via buffered channel + worker pool**, implemented as a standard `slog.Handler` wrapper.

#### Go Core (`internal/logger/async.go`)

```text
Caller goroutine                    Worker goroutines (N=4)
     |                                     |
     | Handle(record)                      |
     |-----> ch (buffered, cap=10,000) --->| inner.Handle(record)
     |       select: send or drop          | (writes JSON to stdout)
     |                                     |
     | (non-blocking return)               |
```

Key design:
- `AsyncHandler` wraps any `slog.Handler` with a `chan slog.Record` (capacity 10,000)
- `Handle()` uses `select` with default case: if channel is full, record is dropped and an `atomic.Int64` counter increments
- 4 worker goroutines drain the channel and call `inner.Handle()` (JSON to stdout)
- `Close()` closes the channel, workers drain remaining records, `sync.WaitGroup` ensures completion
- `WithAttrs()` and `WithGroup()` return new `AsyncHandler` instances sharing the same channel, workers, and drop counter
- Factory: `logger.New(cfg)` returns `(*slog.Logger, Closer)` (async when `cfg.Async == true`, sync otherwise)

#### Python Workers (`workers/codeforge/logger.py`)

Mirrors the Go approach using Python stdlib:
- `queue.Queue(maxsize=10_000)` as the buffer
- `logging.handlers.QueueHandler` for non-blocking enqueue
- `logging.handlers.QueueListener` with a background thread for draining
- `stop_logging()` function for graceful shutdown

#### Shared Log Schema

Both Go and Python emit structured JSON with a common schema:

```json
{"time": "...", "level": "INFO", "service": "codeforge", "msg": "...", "request_id": "..."}
```

### Consequences

#### Positive

- Hot path never blocks on log I/O because `Handle()` is bounded by channel send (nanoseconds)
- Standard `slog.Handler` interface means no custom logger API, works with `slog.Info()`, `slog.Error()`, etc.
- Graceful shutdown drains all buffered records, preventing log loss during normal operation
- Drop counter enables monitoring: if `dropped > 0`, the system is under extreme load
- Same pattern in Go and Python provides a consistent mental model across the stack

#### Negative

- Under extreme load (>10,000 records buffered), log records are dropped silently. Mitigation: drop counter is queryable; buffer size is generous for typical workloads.
- Workers add 4 goroutines to the process (negligible resource cost).
- Log ordering across goroutines is not strictly guaranteed (records may interleave between workers). Mitigation: each record has a timestamp; log aggregators sort by time.

#### Neutral

- Async mode is opt-in via `cfg.Async` (defaults to `true` in production config)
- Sync mode available for development/debugging where log ordering matters more than throughput

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Synchronous logging (slog default) | Simple, ordered, no drops | Blocks callers on I/O, measurable latency impact under load | Unacceptable for a service handling concurrent agents + WebSocket + NATS |
| zerolog / zap (external libraries) | Battle-tested async modes, allocation-free logging | External dependency (violates minimal-dep principle), zap has 3 deps | `slog` + async wrapper achieves the same result with zero dependencies |
| Ring buffer (overwrite oldest) | Never drops new records | Complex implementation, loses oldest context which may be important | Channel + drop-newest is simpler and acceptable for our workload |
| Unbounded channel | Never drops | Unbounded memory growth under sustained load | Memory safety is more important than guaranteed delivery for log records |

### References

- `internal/logger/async.go` -- AsyncHandler implementation
- `internal/logger/logger.go` -- Factory function (`New`)
- `internal/logger/context.go` -- Request ID propagation
- `internal/logger/async_test.go` -- 4 test functions
- `workers/codeforge/logger.py` -- Python async logging
- `workers/tests/test_logger.py` -- 2 Python test functions
