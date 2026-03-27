# ADR-012: Hybrid Routing Cascade for Model Selection

> **Status:** accepted
> **Date:** 2026-03-10
> **Deciders:** Project lead + Claude Code analysis

### Context

CodeForge supports multiple LLM providers (OpenAI, Anthropic, local models via Ollama/LM Studio) through LiteLLM. As the number of available models grew, the system needed intelligent model selection that:

- Routes tasks to the most cost-effective model capable of handling the complexity
- Learns from task outcomes to improve routing over time
- Works immediately for new deployments without historical data (cold-start)
- Handles provider outages and rate limits gracefully
- Operates with sub-second latency to avoid blocking the agent loop
- Falls back safely when routing components fail

A single routing strategy cannot satisfy all these requirements: rule-based systems are fast but cannot learn, learning systems need data to bootstrap, and LLM-based routers are accurate but slow.

### Decision

**Three-layer routing cascade** with deterministic fallback, where each layer handles a specific failure mode of the layers above it.

#### Cascade Order

```
Request --> (1) ComplexityAnalyzer --> (2) MABModelSelector --> (3) LLMMetaRouter --> (4) Complexity Defaults
            rule-based, <1ms         UCB1 learning            LLM cold-start       final fallback
            always runs              primary router           only when MAB          static mapping
                                                              has no data
```

1. **ComplexityAnalyzer** (rule-based, always runs first): Classifies task complexity using token count, tool requirements, code vs. prose, and task-type boost factors. Runs in <1ms. Output: complexity score (low/medium/high/critical) attached to every request.

2. **MABModelSelector** (UCB1 multi-armed bandit, primary router): Uses exploration-exploitation to learn which models perform best for each complexity tier. Entropy-enhanced UCB1 from Phase 28 promotes diversity in early exploration. Requires historical data to be effective.

3. **LLMMetaRouter** (cold-start fallback): An LLM call that analyzes the task and selects the best model. Only invoked when MABModelSelector has insufficient data (cold-start). Expensive (~$0.01 per routing decision) but accurate.

4. **Complexity Defaults** (final fallback): Static mapping from complexity tier to a known-good model. Used when all other layers fail (network issues, no healthy models in MAB, LLMMetaRouter error).

#### Configuration

```yaml
# Enabled by default
CODEFORGE_ROUTING_ENABLED: true

# LiteLLM uses provider wildcards
# HybridRouter picks the exact model within each provider
litellm_config:
  model_list:
    - model_name: "openai/*"
    - model_name: "anthropic/*"
```

#### Key Behaviors

- **Model auto-discovery:** Healthy models fetched from LiteLLM `/v1/models` every 60s
- **Rate-limit awareness:** Per-provider rate-limit tracking from response headers; exhausted providers are skipped
- **Adaptive retry:** Exponential backoff with provider rotation on failure
- **Explicit model override:** When a user or mode specifies an exact model, the router is bypassed entirely
- **Scenario tags:** When routing is disabled (`CODEFORGE_ROUTING_ENABLED=false`), LiteLLM tag-based scenario routing (default/background/think/longContext/review/plan) is used as a simpler alternative

### Consequences

#### Positive

- Graceful degradation: Each cascade layer handles a specific failure mode, so the system always produces a model selection
- Fast path: 90%+ of requests are handled by ComplexityAnalyzer + MAB in <5ms total
- Self-improving: MAB learns from task outcomes without manual tuning
- Cost optimization: Simpler tasks are routed to cheaper models, reducing average cost per task
- Diversity: Entropy-enhanced UCB1 prevents the MAB from prematurely converging on a single model

#### Negative

- Complexity: Three routing layers plus fallback is more complex than a single strategy. Mitigation: each layer is isolated in its own module with clear interfaces; the cascade is a simple ordered list
- MAB cold-start: New deployments route through LLMMetaRouter (slow, costly) until MAB accumulates data. Mitigation: LLMMetaRouter is only called once per complexity-tier-model combination; results are cached
- LLMMetaRouter cost: Each cold-start routing decision costs ~$0.01. Mitigation: only invoked when MAB has no data; typically <50 calls total during bootstrap

#### Neutral

- Routing is transparent to the agent loop: it receives a model name regardless of which cascade layer selected it
- Disabled routing (`CODEFORGE_ROUTING_ENABLED=false`) falls back to LiteLLM's native scenario tags, which is the behavior that existed before Phase 29
- Model auto-discovery cache (60s TTL) means new models are available within a minute of being added to LiteLLM

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| LiteLLM native routing only | Zero custom code, tag-based scenarios | No learning, no cost optimization, static mapping | Does not improve over time; cannot adapt to model performance differences |
| LLM-based router for every request | Most accurate selection | ~200ms latency + ~$0.01 per request, single point of failure | Too slow and expensive for the agent loop hot path (50+ tool calls per conversation) |
| Pure MAB (no cascade) | Simple, learns well | No cold-start strategy, no sub-millisecond complexity classification | MAB needs data; fails silently on new deployments with no history |
| Embedding-based router (e.g., RouterLLM) | Fast inference, learned representations | Requires training data, model maintenance, additional dependencies | MAB achieves similar results without a separate ML model; simpler to operate |
| User-configured model per mode | Explicit, no routing logic | Requires manual tuning, no adaptation, bad UX for new users | Per-mode model is supported as an override but should not be the only mechanism |

### References

- `workers/codeforge/routing/` -- Python routing package (ComplexityAnalyzer, MABModelSelector, LLMMetaRouter, HybridRouter)
- `internal/service/conversation.go` -- Go Core routing integration and model override logic
- `workers/codeforge/consumer.py` -- Consumer-side routing invocation
- `docs/features/03-multi-llm-provider.md` -- Multi-LLM provider specification
