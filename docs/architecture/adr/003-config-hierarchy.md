# ADR-003: Hierarchical Configuration System

> **Status:** accepted
> **Date:** 2026-02-17
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge is a multi-layer system (Go Core, Python Workers, Frontend) that needs consistent configuration across environments:

- Development: Defaults must work out-of-the-box inside the devcontainer
- Staging/Production: Operators override settings via environment variables (12-factor app)
- Custom deployments: Power users want a YAML config file for complex setups (e.g., custom policy directories, sandbox resource limits, orchestrator tuning)

The configuration surface is large: HTTP server, PostgreSQL, NATS, LiteLLM, logging, circuit breaker, rate limiting, cache (L1+L2), idempotency, policy engine, sandbox defaults, orchestrator settings. A single mechanism (e.g., only env vars) would be unwieldy. Multiple mechanisms need a clear precedence order.

### Decision

**Four-tier configuration hierarchy:** defaults < YAML < environment variables < CLI flags.

#### Go Core (`internal/config/`)

```text
Defaults()                    (lowest priority -- always valid)
    |
loadYAML(&cfg, "codeforge.yaml")   (optional file -- graceful skip if missing)
    |
loadEnv(&cfg)                 (non-empty env vars override YAML and defaults)
    |
applyCLI(&cfg, flags)         (highest priority -- CLI flags override everything)
    |
validate(&cfg)                (required fields, min values, constraints)
```

Implementation details:
- `config.go`: Typed `Config` struct with nested sections (Server, Postgres, NATS, LiteLLM, Logging, Breaker, Rate, Git, Policy, Runtime (includes nested Sandbox), Orchestrator, Cache, Idempotency, Webhook, Notification, OTEL, A2A, AGUI, MCP, LSP, Auth, Workspace, Agent, Benchmark, Copilot, Experience, Limits, Quarantine, Routing)
- `loader.go`: `Load()` / `LoadWithCLI()` function applies the four tiers sequentially, then validates
- `Defaults()` returns a fully populated Config so the system runs with zero configuration
- YAML file (`codeforge.yaml`) is optional; missing file returns nil (not an error)
- Env var helpers (`setString`, `setInt`, `setDuration`, etc.) skip empty values and ignore parse errors
- `validate()` checks required fields (port, DSN, NATS URL) and constraints (MaxConns >= 1)

#### Python Workers (`workers/codeforge/config.py`)

- `WorkerSettings` class uses the same defaults < YAML < env hierarchy as Go Core (via `load_yaml_config()` which reads `codeforge.yaml`)
- Sensible defaults for all fields
- Workers load YAML sections (nats, litellm, logging, routing, trust) and allow env var overrides on top

#### Prefix Convention

| Scope | Env Prefix | Examples |
|---|---|---|
| Go Core | `CODEFORGE_*` | `CODEFORGE_PORT`, `CODEFORGE_LOG_LEVEL` |
| External services | Provider standard | `DATABASE_URL`, `NATS_URL` |
| LiteLLM | `LITELLM_*` | `LITELLM_BASE_URL`, `LITELLM_MASTER_KEY` |
| Python Workers | `CODEFORGE_WORKER_*` | `CODEFORGE_WORKER_LOG_LEVEL` |

### Consequences

#### Positive

- Zero-config startup: `go run ./cmd/codeforge/` works with defaults alone (assuming Docker Compose services are running)
- 12-factor compatible: environment variables for containerized deployments
- YAML file for complex configurations (sandbox limits, orchestrator tuning, policy directories) with inline comments
- Single `Load()` call prevents scattered config loading across the codebase
- Validation catches misconfiguration at startup, not at first use

#### Negative

- Four sources of truth can be confusing for debugging ("where did this value come from?"). Mitigation: startup logs could print effective config with source annotations (deferred).
- YAML file path is hardcoded to `codeforge.yaml` in working directory. Mitigation: add CLI flag or env var for config file path (deferred).
- No hot-reload (SIGHUP) support yet, requiring service restart for config changes.

#### Neutral

- Python workers use YAML + env hierarchy (matching Go Core), loading sections relevant to worker operation
- `codeforge.example.yaml` serves as documentation for all available settings

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| Env vars only (12-factor strict) | Simple, well-understood | Unwieldy with 50+ settings, no comments, no grouping | Too many settings for env-only; YAML adds structure |
| Viper (Go config library) | Feature-rich (watch, remote, multi-format) | 15+ transitive dependencies, reflection-heavy, magic behavior | Violates minimal-dependency principle; `yaml.v3` + `os.Getenv` is sufficient |
| TOML instead of YAML | Stricter syntax, comments, no indentation issues | Less common in Go ecosystem, YAML already used for LiteLLM/modes/tools | YAML is the project-wide config format (ADR: YAML for all config) |
| CLI flags | Standard Go pattern | Adds flag parsing complexity, not container-friendly | Env vars are the standard for containerized services |

### References

- [The Twelve-Factor App -- Config](https://12factor.net/config)
- `internal/config/config.go` -- Config struct definitions
- `internal/config/loader.go` -- Load/LoadWithCLI function with four-tier merge
- `internal/config/loader_test.go` -- 25 test functions
- `codeforge.example.yaml` -- Full example configuration
