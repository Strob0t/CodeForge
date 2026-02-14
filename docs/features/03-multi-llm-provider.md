# Feature: Multi-LLM Provider (Pillar 3)

> **Status:** Design phase
> **Priority:** Phase 1 (Foundation) + Phase 2 (MVP)
> **Architecture reference:** [architecture.md](../architecture.md) — "LLM Integration: LiteLLM Proxy as Sidecar"

## Overview

Multi-provider LLM integration through LiteLLM Proxy as a Docker sidecar.
**No custom LLM provider interface** — all LLM communication goes through
LiteLLM's OpenAI-compatible API on port 4000.

## Architecture Decision

CodeForge does not build its own LLM abstraction layer. LiteLLM Proxy handles:
- 127+ provider integrations (OpenAI, Anthropic, Ollama, Bedrock, etc.)
- 6 routing strategies (latency, cost, usage, least-busy, shuffle, tag-based)
- Budget management, rate limiting, cost tracking
- Streaming normalization, tool calling, structured output

## What LiteLLM Provides (not built by us)

| Capability | Mechanism |
|---|---|
| Provider abstraction | 127+ providers, unified OpenAI-compatible API |
| Routing | Tag-based routing (scenario → model deployment) |
| Fallbacks | Chains with cooldown (60s default) |
| Cost tracking | Per call, per model, per key (36,000+ pricing entries) |
| Budgets | Per key, per team, per user limits |
| Caching | In-memory, Redis, semantic (Qdrant) |
| Observability | 42+ integrations (Prometheus, Langfuse, etc.) |

## What CodeForge Builds (Custom)

| Component | Layer | Description |
|---|---|---|
| **LiteLLM Config Manager** | Go Core | Generates `litellm_config.yaml` from DB. CRUD for models/deployments/keys. |
| **User-Key Mapping** | Go Core | CodeForge user → LiteLLM Virtual Keys. Secure key storage. |
| **Scenario Router** | Go Core | Task type → LiteLLM tag. Routes tasks to appropriate models. |
| **Cost Dashboard** | Frontend | Queries LiteLLM Spend API. Visualization per project/user/agent. |
| **Local Model Discovery** | Go Core | Auto-discover Ollama/LM Studio models, add to LiteLLM config. |
| **Copilot Token Exchange** | Go Core | GitHub OAuth → Copilot bearer token for free model access. |

## Scenario-Based Routing

| Scenario | Use Case | Typical Models |
|---|---|---|
| `default` | General coding | Claude Sonnet, GPT-4o |
| `background` | Batch, indexing, embedding | GPT-4o-mini, DeepSeek, local |
| `think` | Architecture, complex logic | Claude Opus, o3 |
| `longContext` | Input > 60K tokens | Gemini Pro (1M context) |
| `review` | Code review, quality check | Claude Sonnet |
| `plan` | Feature planning, design | Claude Opus |

## LLM Capability Levels

| Level | Example | What CodeForge Provides |
|---|---|---|
| Full-featured Agents | Claude Code, Aider, OpenHands | Orchestration only |
| API with Tool Support | OpenAI, Claude API, Gemini | Context Layer + Routing + Tool Definitions |
| Pure Completion | Ollama, LM Studio | Everything: Context, Tools, Prompts, Quality Layer |

## Docker Compose Configuration (Planned)

```yaml
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
    depends_on:
      - postgres
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:4000/health/liveliness"]
```

## TODOs

Tracked in [todo.md](../todo.md) under Phase 1 and Phase 2.

### Phase 1
- [ ] Add litellm service to `docker-compose.yml`
- [ ] Create initial `litellm_config.yaml` (at least one provider)
- [ ] Health check integration from Go Core
- [ ] Verify basic LLM call through proxy

### Phase 2
- [ ] LiteLLM Config Manager (Go Core)
- [ ] User-Key Mapping (secure storage, virtual keys)
- [ ] Scenario Router (task → tag mapping)
- [ ] Local Model Discovery (Ollama, LM Studio)
- [ ] Copilot Token Exchange
- [ ] Frontend: Provider configuration UI
- [ ] Frontend: Cost Dashboard (LiteLLM Spend API)
