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

**LiteLLM Config:** Simplified from 38 individual model entries to 6 provider-level wildcards:
```yaml
model_list:
  - model_name: "openai/*"     # All OpenAI models
  - model_name: "anthropic/*"  # All Anthropic models
  - model_name: "groq/*"       # All Groq models
  - model_name: "gemini/*"     # All Google Gemini models
  - model_name: "ollama/*"     # Local Ollama models
  - model_name: "mistral/*"    # All Mistral AI models
```

**Config:** Set `CODEFORGE_ROUTING_ENABLED=true` to activate intelligent routing.

- [x] Python routing package: `workers/codeforge/routing/` (7 modules, 164 tests)
- [x] Integration: `resolve_model_with_routing()` in llm.py, conversation handler, executor
- [x] LiteLLM wildcard config: 6 provider entries replace 38 individual models
- [x] Task-type complexity boost: inherent task difficulty (PLAN/REVIEW/REFACTOR etc.) shifts tier classification (29K)
- [x] Model auto-discovery: `model_resolver.py` (Python, cached 60s TTL) + `ModelRegistry.BestModel()` (Go) — no hardcoded model defaults
- [x] NATS runtime fix: `DeliverPolicy.NEW` prevents 30s timeout from replaying old JetStream messages

### Completed (Phase 30 -- LLM Retry & Rate-Limit Awareness)

Automatic retry with exponential backoff for transient LLM provider failures, plus per-provider rate-limit tracking to skip exhausted providers during routing fallback.

**Retry Behaviour:** `LiteLLMClient._with_retry()` wraps all three HTTP methods (`completion`, `chat_completion`, `chat_completion_stream`) with configurable retries on 429/502/503/504. Backoff respects `Retry-After` hints from provider error bodies when available, otherwise uses exponential backoff (`base^(attempt+1)`, capped at `backoff_max`).

**Rate-Limit Tracking:** After every LLM response, `x-ratelimit-remaining-requests`, `x-ratelimit-limit-requests`, and `x-ratelimit-reset-requests` headers are parsed and fed into a `RateLimitTracker` singleton. The tracker maintains per-provider state with automatic recovery after the reset window elapses.

**Rate-Aware Routing:** `HybridRouter._complexity_fallback()` queries the tracker before selecting a model. If a provider's quota is exhausted, all its models are skipped in the preference list. The last-resort fallback (first available model) is not filtered to prevent total routing failure.

**Config (all optional, defaults are production-ready):**

| Variable | Default | Description |
|---|---|---|
| `CODEFORGE_LLM_MAX_RETRIES` | `2` | Max retry attempts per LLM call |
| `CODEFORGE_LLM_BACKOFF_BASE` | `2.0` | Exponential backoff base (seconds) |
| `CODEFORGE_LLM_BACKOFF_MAX` | `60.0` | Maximum backoff cap (seconds) |
| `CODEFORGE_LLM_TIMEOUT` | `120.0` | HTTP request timeout (seconds) |

- [x] `LLMClientConfig` dataclass + `load_llm_client_config()` env-var loader
- [x] `_with_retry()` async retry wrapper in LiteLLMClient (all 3 methods)
- [x] `RateLimitTracker` (`workers/codeforge/routing/rate_tracker.py`) — per-provider state
- [x] Rate-aware `HybridRouter._complexity_fallback()` skips exhausted providers
- [x] Agent loop cleanup: removed 40-line inline retry, consolidated to LLM client
- [x] LiteLLM proxy retry reduced 2 -> 1 (app-level retry handles escalation)
- [x] 64 tests across 4 test files

### TODOs (Phase 9+)

Tracked in [todo.md](../todo.md) under Phase 9+.

- [ ] User-Key Mapping (secure storage, virtual keys per CodeForge user).
- [x] Scenario Router (Phase 29 -- intelligent routing replaces manual tag mapping).
- [ ] Local Model Discovery (Ollama, LM Studio auto-detection).
- [ ] Copilot Token Exchange (GitHub OAuth to Copilot bearer token).
- [ ] Distributed tracing (OpenTelemetry full implementation).
