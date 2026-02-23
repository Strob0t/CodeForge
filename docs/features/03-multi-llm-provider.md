# Feature: Multi-LLM Provider (Pillar 3)

> Status: Foundation implemented (Phase 1-2), Cost Transparency (Phase 7)
> Priority: Phase 1 (Foundation) + Phase 2 (MVP) completed; Phase 7 (Cost) completed
> Architecture reference: [architecture.md](../architecture.md) -- "LLM Integration: LiteLLM Proxy as Sidecar"

### Purpose

Multi-provider LLM integration through **LiteLLM** Proxy as a Docker sidecar. CodeForge does not build a custom LLM provider interface. All LLM communication goes through LiteLLM's OpenAI-compatible API on port 4000.

### Architecture Decision

CodeForge does not build its own LLM abstraction layer. LiteLLM Proxy handles 127+ provider integrations (OpenAI, Anthropic, Ollama, Bedrock, etc.), 6 routing strategies (latency, cost, usage, least-busy, shuffle, tag-based), budget management, rate limiting, cost tracking, streaming normalization, tool calling, and structured output.

### What LiteLLM Provides (Not Built By Us)

| Capability | Mechanism |
|---|---|
| Provider abstraction | 127+ providers, unified OpenAI-compatible API |
| Routing | Tag-based routing (scenario to model deployment) |
| Fallbacks | Chains with cooldown (60s default) |
| Cost tracking | Per call, per model, per key (36,000+ pricing entries) |
| Budgets | Per key, per team, per user limits |
| Caching | In-memory, Redis, semantic (Qdrant) |
| Observability | 42+ integrations (Prometheus, Langfuse, etc.) |

### What CodeForge Builds (Custom)

| Component | Layer | Description |
|---|---|---|
| LiteLLM Config Manager | Go Core | Generates `litellm_config.yaml` from DB. CRUD for models/deployments/keys. |
| User-Key Mapping | Go Core | CodeForge user to LiteLLM Virtual Keys. Secure key storage. |
| **Scenario Router** | Go Core | Task type to LiteLLM tag. Routes tasks to appropriate models. |
| Cost Dashboard | Frontend | Queries LiteLLM Spend API. Visualization per project/user/agent. |
| Local Model Discovery | Go Core | Auto-discover Ollama/LM Studio models, add to LiteLLM config. |
| Copilot Token Exchange | Go Core | GitHub OAuth to Copilot bearer token for free model access. |

### Scenario-Based Routing

| Scenario | Use Case | Typical Models |
|---|---|---|
| `default` | General coding | Claude Sonnet, GPT-4o |
| `background` | Batch, indexing, embedding | GPT-4o-mini, DeepSeek, local |
| `think` | Architecture, complex logic | Claude Opus, o3 |
| `longContext` | Input > 60K tokens | Gemini Pro (1M context) |
| `review` | Code review, quality check | Claude Sonnet |
| `plan` | Feature planning, design | Claude Opus |

### LLM Capability Levels

| Level | Example | What CodeForge Provides |
|---|---|---|
| Full-featured Agents | Claude Code, Aider, OpenHands | Orchestration only |
| API with Tool Support | OpenAI, Claude API, Gemini | Context Layer + Routing + Tool Definitions |
| Pure Completion | Ollama, LM Studio | Everything: Context, Tools, Prompts, Quality Layer |

### Docker Compose Configuration

LiteLLM Proxy runs as a Docker sidecar in both dev and production environments.

```yaml
# docker-compose.yml (dev)
services:
  litellm:
    image: docker.litellm.ai/berriai/litellm:main-stable
    ports:
      - "4000:4000"
    volumes:
      - ./litellm_config.yaml:/app/config.yaml
    command: ["--config", "/app/config.yaml", "--port", "4000"]
    environment:
      - LITELLM_MASTER_KEY=${LITELLM_MASTER_KEY}
      - DATABASE_URL=postgresql://codeforge:${POSTGRES_PASSWORD}@postgres:5432/codeforge?schema=litellm
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:4000/health/liveliness"]
```

### Completed (Phase 1)

- [x] LiteLLM service in `docker-compose.yml` with health check, depends_on, shared PostgreSQL.
- [x] Initial `litellm_config.yaml` with provider configuration.
- [x] Health check integration from Go Core (`/health/ready` pings LiteLLM).
- [x] Basic LLM call through proxy (Python workers via `litellm` client library).

### Completed (Phase 2)

- [x] LiteLLM Config Manager via admin API (`internal/adapter/litellm/`).
- [x] Frontend: Provider configuration UI (ModelsPage -- add/delete models, health status).

### Completed (Phase 7 -- Cost and Token Transparency)

- [x] Real cost calculation in Python workers (`x-litellm-response-cost` header + fallback pricing table).
- [x] Token persistence in database (migration 015: `tokens_in`, `tokens_out`, `model` on runs).
- [x] Cost aggregation API (5 endpoints: global, per-project, per-model, daily, recent runs).
- [x] WS budget alerts (80% and 90% thresholds with dedup).
- [x] Frontend: CostDashboardPage (global totals, project breakdown), ProjectCostSection (model/daily/runs).
- [x] Frontend: RunPanel token + model display in active run and history.

### TODOs (Phase 9+)

Tracked in [todo.md](../todo.md) under Phase 9+.

- [ ] User-Key Mapping (secure storage, virtual keys per CodeForge user).
- [ ] Scenario Router (task type to LiteLLM tag mapping).
- [ ] Local Model Discovery (Ollama, LM Studio auto-detection).
- [ ] Copilot Token Exchange (GitHub OAuth to Copilot bearer token).
- [ ] Distributed tracing (OpenTelemetry full implementation).
