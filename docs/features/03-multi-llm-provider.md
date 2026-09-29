# Feature: Multi-LLM Provider (Pillar 3)

> Status: Implemented -- Foundation (Phase 1-2), Cost Transparency (Phase 7), Intelligent Routing (Phase 29), LLM Retry & Rate-Limit Awareness (Phase 30)
> Priority: Phase 1 (Foundation) + Phase 2 (MVP) completed; Phase 7 (Cost), Phase 29 and Phase 30 completed
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
| LiteLLM Config Manager | Go Core | Lists/adds/deletes model deployments via the LiteLLM admin API (`internal/adapter/litellm/`); `litellm/config.yaml` is a static file. Planned: generating the config from the DB, key CRUD. |
| User-Key Mapping | Go Core | Encrypted per-user provider API keys (`/api/v1/llm-keys`, `internal/domain/llmkey/`), injected into conversation runs. Planned: mapping CodeForge users to LiteLLM Virtual Keys. |
| **Scenario Router** | Python Workers | Mode scenario (set in Go mode presets, forwarded in the run payload) to LiteLLM tag via `resolve_scenario()` in `workers/codeforge/llm.py`. |
| Cost Dashboard | Frontend | Aggregates run costs stored by CodeForge (from LiteLLM's `x-litellm-response-cost`) per project/model/tool/day (`internal/service/cost.go`). Planned: LiteLLM Spend API queries, per-user/agent views. |
| Local Model Discovery | Go Core | Discovers LiteLLM and Ollama models (`GET /api/v1/llm/discover`); they are served through the `ollama/*` and `lm_studio/*` wildcards instead of being added to the LiteLLM config. |
| Copilot Token Exchange | Go Core | GitHub OAuth to Copilot bearer token for free model access. |
| **Subscription Connect** | Go Core | OAuth device flow for Claude Max + GitHub Copilot. Produces API keys stored in `.env`. |

### Scenario-Based Routing

Requests without a scenario tag route to **all models** (no tag filtering). Specific scenarios restrict routing to tagged models only.

| Scenario | Use Case | Typical Models |
|---|---|---|
| *(none)* | General coding (no tag sent) | All models eligible |
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
# docker-compose.yml (dev, abridged)
services:
  litellm:
    image: docker.litellm.ai/berriai/litellm:main-stable   # prod: ghcr.io/berriai/litellm:v1.63.2 (docker-compose.prod.yml)
    ports:
      - "4000:4000"
    volumes:
      - ${HOST_PROJECT_PATH:-.}/litellm:/app/data
    command: ["--config", "/app/data/config.yaml", "--port", "4000"]
    environment:
      LITELLM_MASTER_KEY: ${LITELLM_MASTER_KEY:-sk-codeforge-dev}
      DATABASE_URL: postgresql://codeforge:${POSTGRES_PASSWORD:-codeforge_dev}@postgres:5432/codeforge
      # plus provider API keys (OPENAI_API_KEY, ANTHROPIC_API_KEY, ...), OLLAMA_API_BASE, LM_STUDIO_API_BASE
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "python3", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:4000/health/liveliness')"]
```

### Completed (Phase 1)

- [x] LiteLLM service in `docker-compose.yml` with health check, depends_on, shared PostgreSQL.
- [x] Initial `litellm/config.yaml` with provider configuration.
- [x] Health check integration from Go Core (`/health/ready` pings LiteLLM).
- [x] Basic LLM call through the proxy (Python workers use `httpx` against LiteLLM's OpenAI-compatible API; no LiteLLM SDK).

### Completed (Phase 2)

- [x] LiteLLM Config Manager via admin API (`internal/adapter/litellm/`).
- [x] Frontend: Provider configuration UI (ModelsPage -- add/delete models, health status). Deleting fails: the UI sends `DELETE /llm/models/{id}`, the backend only has `POST /llm/models/delete` (see [Known Issues](../todo.md#known-issues) KI-40).

### Completed (Phase 7 -- Cost and Token Transparency)

- [x] Real cost calculation in Python workers (`x-litellm-response-cost` header + fallback pricing table).
- [x] Token persistence in database (migration 015: `tokens_in`, `tokens_out`, `model` on runs).
- [x] Cost aggregation API (5 endpoints: global, per-project, per-model, daily, recent runs).
- [x] WS budget alerts (80% and 90% thresholds with dedup).
- [x] Frontend: CostDashboardPage (global totals, project breakdown), ProjectCostSection (model/daily/runs).
- [x] Frontend: RunPanel token + model display in active run and history.

### Completed (Phase 29 -- Intelligent Routing)

Replaces manual tag-based routing with a three-layer intelligent cascade.

**Architecture:** Python HybridRouter selects exact model name -> LiteLLM routes directly via provider wildcards. No manual tag assignment needed.

**Three-Layer Cascade:**

| Layer | Name | Mechanism | Latency |
|-------|------|-----------|---------|
| 1 | ComplexityAnalyzer | Rule-based prompt analysis (7 dimensions + task-type boost) | <1ms |
| 2 | MABModelSelector | UCB1 bandit learning from benchmark + usage data, entropy-aware diversity | <1ms (cached) |
| 3 | LLMMetaRouter | Small LLM classifies edge cases / cold start | ~500ms |

**Complexity Tiers:** SIMPLE -> MEDIUM -> COMPLEX -> REASONING (weighted sum of 7 dimension scores + task-type boost)

**Task-Type Boost (29K):** Task types inferred from keyword patterns (REVIEW, DEBUG, REFACTOR, PLAN, QA, CODE, CHAT) receive an inherent complexity boost that shifts tier classification upward. For example, "Review this code" (REVIEW, +0.25) routes to a more capable model than "Hello" (CHAT, +0.0) even when both prompts have similar surface-level dimension scores. Boosts: PLAN/REVIEW +0.25, DEBUG/REFACTOR +0.20, QA +0.15, CODE +0.10, CHAT +0.0.

**Dimension Weights:** code_presence 0.20, reasoning_markers 0.20, technical_terms 0.15, prompt_length 0.10, multi_step 0.15, context_requirements 0.10, output_complexity 0.10.

**Model Auto-Discovery:** When no explicit model is configured, the system auto-discovers available models from LiteLLM's `/v1/models` endpoint. Python workers use `model_resolver.py` (cached, 60s TTL). Go Core uses `ModelRegistry.BestModel()`. Priority: explicit config > env var > auto-discovery.

**Fallback:** If all layers fail or routing disabled, tag-based routing via `resolve_scenario()` still works.

**LiteLLM Config:** Provider-level wildcards replace the former 38 individual model entries, plus an explicit `openai/container` alias for LM Studio (see `litellm/config.yaml`):
```yaml
model_list:
  - model_name: "ollama/*"       # Local Ollama models
  - model_name: "lm_studio/*"    # Local LM Studio models
  - model_name: "openai/container"
  - model_name: "openai/*"
  - model_name: "anthropic/*"
  - model_name: "gemini/*"
  - model_name: "groq/*"
  - model_name: "mistral/*"
  - model_name: "openrouter/*"
  - model_name: "cerebras/*"
  - model_name: "chutes/*"
  - model_name: "aihubmix/*"
```

**Config:** Intelligent routing is enabled by default; set `CODEFORGE_ROUTING_ENABLED=false` to fall back to tag-based routing via `resolve_scenario()`.

**Supporting Components:**

| Component | File | Purpose |
|-----------|------|---------|
| **Model Blocklist** | `workers/codeforge/routing/blocklist.py` | TTL-based model blocklist for temporarily disabling failing models |
| **Rate Limit Tracker** | `workers/codeforge/routing/rate_tracker.py` | Per-provider rate limit tracking for intelligent request routing |

- [x] Python routing package: `workers/codeforge/routing/` (11 modules, ~280 tests in `workers/tests/test_routing_*.py`)
- [x] Integration: `resolve_model_with_routing()` in llm.py, conversation handler, executor
- [x] LiteLLM wildcard config: provider-level wildcard entries (11 providers + `openai/container`) replace 38 individual models
- [x] Task-type complexity boost: inherent task difficulty (PLAN/REVIEW/REFACTOR etc.) shifts tier classification (29K)
- [x] Model auto-discovery: `model_resolver.py` (Python, cached 60s TTL) + `ModelRegistry.BestModel()` (Go) — no hardcoded model defaults
- [x] NATS runtime fix: `DeliverPolicy.NEW` prevents 30s timeout from replaying old JetStream messages

### Completed (Phase 30 -- LLM Retry & Rate-Limit Awareness)

Automatic retry with exponential backoff for transient LLM provider failures, plus per-provider rate-limit tracking to skip exhausted providers during routing fallback.

**Retry Behaviour:** `LiteLLMClient._with_retry()` wraps all three HTTP methods (`completion`, `chat_completion`, `chat_completion_stream`) with configurable retries on 408/500/502/503/504 (429 is handled by fallback logic, not retry). Backoff respects `Retry-After` hints from provider error bodies when available, otherwise uses exponential backoff (`base^(attempt+1)`, capped at `backoff_max`).

**Rate-Limit Tracking:** After every LLM response, `x-ratelimit-remaining-requests`, `x-ratelimit-limit-requests`, and `x-ratelimit-reset-requests` headers are parsed and fed into a `RateLimitTracker` singleton. The tracker maintains per-provider state with automatic recovery after the reset window elapses.

**Rate-Aware Routing:** `HybridRouter._complexity_fallback()` queries the tracker before selecting a model. If a provider's quota is exhausted, all its models are skipped in the preference list. The last-resort fallback (first available model) is not filtered to prevent total routing failure.

**Config (all optional, defaults are production-ready):**

| Variable | Default | Description |
|---|---|---|
| `CODEFORGE_LLM_MAX_RETRIES` | `2` | Max retry attempts per LLM call |
| `CODEFORGE_LLM_BACKOFF_BASE` | `2.0` | Exponential backoff base (seconds) |
| `CODEFORGE_LLM_BACKOFF_MAX` | `60.0` | Maximum backoff cap (seconds) |
| `CODEFORGE_LLM_CONNECT_TIMEOUT` | `10.0` | HTTP connect timeout (seconds) |
| `CODEFORGE_LLM_READ_TIMEOUT` | `300.0` | HTTP read timeout (seconds) |

- [x] `LLMClientConfig` dataclass + `load_llm_client_config()` env-var loader
- [x] `_with_retry()` async retry wrapper in LiteLLMClient (all 3 methods)
- [x] `RateLimitTracker` (`workers/codeforge/routing/rate_tracker.py`) — per-provider state
- [x] Rate-aware `HybridRouter._complexity_fallback()` skips exhausted providers
- [x] Agent loop cleanup: removed 40-line inline retry, consolidated to LLM client
- [x] LiteLLM proxy retry reduced 2 -> 1 (app-level retry handles escalation)
- [x] 64 tests across 4 test files

### Open Items

> **Task tracking:** See [docs/todo.md](../todo.md) for current open items related to Multi-LLM Provider.
