# ADR-005: Docker-Native Logging (No External Monitoring Stack)

> **Status:** accepted
> **Date:** 2026-02-17
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge runs as a set of Docker Compose services (Go Core, Python Workers, PostgreSQL, NATS, LiteLLM). Observability is essential for debugging agent execution, tracking costs, and diagnosing failures.

The typical approach for containerized services is a full monitoring stack: ELK (Elasticsearch + Logstash + Kibana), Loki + Grafana, or Datadog. These add significant infrastructure complexity, memory overhead, and operational burden. This overhead is especially problematic for a development tool that primarily runs on a single developer's machine.

Requirements:
- Structured JSON logs from all services for machine parsing
- Log rotation to prevent disk exhaustion
- Easy access to logs for debugging (no external UI required)
- Correlation across services via request ID
- Minimal infrastructure footprint

### Decision

**Docker-native logging with JSON-file driver and structured JSON output.** No external monitoring stack.

#### Docker Compose Configuration

```yaml
x-logging: &default-logging
  driver: json-file
  options:
    max-size: "10m"
    max-file: "3"
```

Applied to all 5 services via YAML anchor. Each service gets at most 30 MB of log storage (3 files x 10 MB), automatically rotated by Docker.

#### Structured JSON Output

All services write structured JSON to stdout/stderr:

| Service | Mechanism | Format |
|---|---|---|
| Go Core | `slog.JSONHandler` (via AsyncHandler) | `{"time":"...","level":"INFO","service":"codeforge","msg":"...","request_id":"..."}` |
| Python Workers | `structlog.JSONRenderer` (via QueueHandler) | `{"timestamp":"...","level":"info","service":"codeforge-worker","event":"...","request_id":"..."}` |
| PostgreSQL | Native | Unstructured (acceptable, DB logs are rarely parsed) |
| NATS | Native | Structured (NATS natively logs JSON in some modes) |
| LiteLLM | Native | Structured JSON (LiteLLM uses Python logging) |

#### Log Access

```bash
# All services
docker compose logs -f

# Single service
docker compose logs -f codeforge

# Filter by level (via jq)
docker compose logs codeforge | jq 'select(.level == "ERROR")'

# Filter by request ID
docker compose logs | jq 'select(.request_id == "abc-123")'

# Helper script: scripts/logs.sh
./scripts/logs.sh tail              # Follow all logs
./scripts/logs.sh errors            # Only ERROR level
./scripts/logs.sh service codeforge # Single service
./scripts/logs.sh request abc-123   # By request ID
```

#### Request ID Propagation

- Go middleware (`internal/middleware/requestid.go`) generates UUIDs for incoming HTTP requests
- Request ID injected into `slog` logger context (`internal/logger/context.go`)
- NATS messages carry `X-Request-ID` header (Go to Python to Go)
- Python workers extract request ID from NATS headers and bind to structlog context
- Enables tracing a single request across Go Core, NATS, Python Worker, NATS, and back to Go Core

### Consequences

#### Positive

- Zero additional infrastructure: no Elasticsearch, no Prometheus, no Grafana
- Docker handles rotation automatically with no logrotate configuration needed
- `docker compose logs` is the single entry point for all debugging
- Structured JSON enables `jq` filtering without any tooling setup
- Request ID correlation works across all services without distributed tracing infrastructure
- `scripts/logs.sh` provides convenient shortcuts for common queries

#### Negative

- No log aggregation UI, so debugging requires terminal and `jq`. Mitigation: acceptable for a development tool; production deployments can add Loki/Grafana.
- No alerting on error patterns. Mitigation: agent errors surface in the WebSocket events / frontend UI.
- Log retention is limited (30 MB per service), so long-running sessions may lose old logs. Mitigation: increase `max-size`/`max-file` in docker-compose override for production.
- No metrics collection (request latency, error rates, etc.). Mitigation: OpenTelemetry integration planned for Phase 3+ (deferred, not removed).

#### Neutral

- Docker's `json-file` driver is the default, so no special Docker configuration is needed
- Production deployments can switch to `fluentd` or `loki` driver without code changes
- The structured JSON format is compatible with any future log aggregation system

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| ELK Stack (Elasticsearch + Logstash + Kibana) | Full-text search, dashboards, alerting | 4+ GB RAM, 3 additional services, complex configuration | Massive overkill for a dev tool; 10x the resource footprint of CodeForge itself |
| Loki + Grafana | Lightweight log aggregation, good with Docker | 2 additional services, needs Promtail/Docker driver, 500MB+ RAM | Still too heavy for default setup; good option for production override |
| Datadog / New Relic / etc. | Managed, zero ops | External service, costs money, data leaves the machine | Not appropriate for a self-hosted dev tool |
| File-based logging (no Docker driver) | Full control over rotation | Must manage rotation ourselves, loses Docker log integration | Docker's json-file driver already does this correctly |

### References

- [Docker Logging Drivers](https://docs.docker.com/config/containers/logging/configure/)
- `docker-compose.yml` -- `x-logging` anchor definition
- `scripts/logs.sh` -- Log access helper script
- `internal/middleware/requestid.go` -- Request ID middleware
- `internal/logger/context.go` -- Request ID in logger context
