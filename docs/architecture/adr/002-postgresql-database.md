# ADR-002: PostgreSQL 18 as Primary Database

> **Status:** accepted (updated 2026-02-26: upgraded from PG 17 to PG 18)
> **Date:** 2026-02-14
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge needs a persistent database for:
- Project and repository metadata
- Agent task definitions, runs, and results
- Audit logs and trajectory recording
- Cost tracking and budget management
- User settings and API key storage

Additionally, LiteLLM Proxy requires PostgreSQL for spend tracking, virtual keys, teams, and budgets (~15 tables with `LiteLLM_` prefix). This makes PostgreSQL a hard dependency regardless of the application database choice.

The goal is to minimize tech stack complexity: use as few different technologies as possible while keeping the ability to scale when needed.

### Decision

**PostgreSQL 18** is the single database for CodeForge, shared with LiteLLM via schema separation.

#### Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Database | PostgreSQL 18 (Alpine) | Single DB for app + LiteLLM, JSONB, LISTEN/NOTIFY, UUIDv7, Async I/O |
| Go Driver | pgx v5 | Go-native, best performance, direct PG feature access |
| Migrations | goose | Simple SQL files (up/down), embeddable in Go binary |
| Python Driver | psycopg3 | Sync+async, Row Factories for Pydantic models |

#### Key Reasons

- LiteLLM hard dependency: PostgreSQL is already required, so adding a second DB (SQLite, etc.) creates unnecessary complexity
- Shared instance, separated schemas: LiteLLM uses `?schema=litellm` in its connection string; CodeForge uses `public` schema (one container, clean isolation)
- Simplicity over tooling: pgx v5 directly (no ORM, no code generator) + goose for migrations (plain SQL files) gives a minimal toolchain
- JSONB: Flexible storage for agent configs, task metadata, LLM responses without schema migrations for every new field
- LISTEN/NOTIFY: Real-time push for WebSocket (agent status, cost updates) without additional infrastructure
- Scaling path: Read replicas, PgBouncer, partitioning available when needed, not built upfront

#### Rejected Tooling (Simplicity Principle)

| Tool | Why Rejected |
|------|-------------|
| sqlc (code generator) | Adds build step and tooling complexity; pgx v5 is sufficient with manual SQL |
| Atlas (migration framework) | Powerful but complex (declarative schemas, HCL, drift detection); goose is simpler |
| GORM / Ent (ORMs) | Reflection overhead, struct tags leak into domain layer, poor fit for Hexagonal Architecture |
| DuckDB (future OLAP) | YAGNI: add only when PG aggregations actually become slow |
| pgvector (embeddings) | YAGNI: PG extension activatable in Phase 3 if needed |

#### Configuration

```yaml
# docker-compose.yml
services:
  postgres:
    image: postgres:18-alpine
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: codeforge
      POSTGRES_USER: codeforge
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - ./data/postgres:/var/lib/postgresql/data
    shm_size: 256mb
    command: >
      postgres
        -c shared_buffers=128MB
        -c effective_cache_size=384MB
        -c max_connections=100
        -c lc_collate=C.UTF-8
        -c lc_ctype=C.UTF-8
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U codeforge"]
      interval: 5s
      timeout: 3s
      retries: 5
```

#### LiteLLM Shared Instance

```yaml
# LiteLLM connects to the same PostgreSQL instance
general_settings:
  database_url: "postgresql://codeforge:${POSTGRES_PASSWORD}@postgres:5432/codeforge?schema=litellm"
```

#### Client Libraries

| Layer | Library | Notes |
|---|---|---|
| Go Core | `pgx` v5 + `pgxpool` | Native PG protocol, connection pooling, LISTEN/NOTIFY, JSONB |
| Go Migrations | `goose` | SQL files in `migrations/`, embeddable via Go API |
| Python Workers | `psycopg3` | Sync+async, Row Factories, small connection pool (5-10) |

#### Connection Budget

| Consumer | Pool Size | Notes |
|----------|-----------|-------|
| Go Core (pgxpool) | ~20 | Main application queries |
| Go Core (dedicated) | 1 | LISTEN/NOTIFY for WebSocket push |
| LiteLLM Proxy | ~15 | Prisma default, configurable |
| Python Workers | ~10 | Read-heavy, task metadata |
| Total | ~46 | Well within `max_connections=100` |

### Consequences

#### Positive

- Single database for everything: one backup strategy, one migration tool, one monitoring target
- Cross-schema queries enable Cost Dashboard to join CodeForge tasks with LiteLLM spend data
- LISTEN/NOTIFY eliminates need for additional pub/sub infrastructure for UI push
- JSONB avoids rigid schema for agent-specific configuration
- PostgreSQL 18 async I/O (`io_uring`) improves sequential scans, vacuum, and bitmap heap scan performance in containers
- Native `uuidv7()` for timestamp-ordered UUIDs (better B-tree locality than random UUIDs)
- `pg_upgrade --jobs` enables parallel major version upgrades

#### Negative

- PostgreSQL requires a running server (unlike embedded SQLite), meaning no database without Docker
- Shared instance is a single point of failure for dev (acceptable; production can use managed PG)
- LiteLLM runs Prisma migrations on startup, requiring schema separation to avoid conflicts

#### Neutral

- NATS JetStream KV (already in stack) handles ephemeral state: heartbeats, task locks, runtime status
- LISTEN/NOTIFY payload limited to 8000 bytes, so send event IDs not full data
- `C.UTF-8` collation prevents index corruption on Docker base image upgrades

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| SQLite | Zero config, embedded, fast reads | Single writer, no LISTEN/NOTIFY, "two-database problem" with LiteLLM | PostgreSQL already required; SQLite adds a second DB system |
| MySQL | Widely used, good tooling | No technical advantage over PG; LiteLLM dropped MySQL support ("hard to maintain, led to bugs") | LiteLLM explicitly recommends PostgreSQL |
| CockroachDB | Distributed, horizontally scalable | ~2GB RAM minimum, overkill for dev tool | No need for distributed database |
| SurrealDB | Multi-model (document + graph + relational) | Immature, small community, limited Go ecosystem | Too risky for production use |

### References

- [PostgreSQL 18 Release Notes](https://www.postgresql.org/docs/release/18.0/)
- [What's New in PostgreSQL 18](https://www.bytebase.com/blog/what-is-new-in-postgres-18/)
- [pgx -- PostgreSQL Driver for Go](https://github.com/jackc/pgx)
- [goose -- Database Migration Tool](https://github.com/pressly/goose)
- [psycopg3 -- PostgreSQL Driver for Python](https://www.psycopg.org/)
- [LiteLLM -- What is stored in the DB](https://docs.litellm.ai/docs/proxy/db_info)
- [LiteLLM -- Schema Configuration](https://github.com/BerriAI/litellm/discussions/5503)
- [PostgreSQL LISTEN/NOTIFY](https://www.postgresql.org/docs/18/sql-notify.html)
