# ADR-008: Benchmark Evaluation Framework

> **Status:** accepted
> **Date:** 2026-02-25
> **Deciders:** CodeForge team

### Context

CodeForge orchestrates multiple AI coding agents across different LLM providers and models. To measure and compare agent quality beyond basic "does it work" checks, we need a structured evaluation framework that can:

1. Run standardized coding tasks against any configured LLM model
2. Score outputs using LLM-as-judge metrics (correctness, faithfulness, tool usage)
3. Trace agent execution for observability (tool selection, goal decomposition, plan adaptability)
4. Measure multi-agent collaboration quality when DAG orchestration is active
5. Compare results across runs, models, and configurations

This framework must be dev-mode only (no production overhead), integrate with the existing NATS-based worker architecture, and produce actionable scores stored in PostgreSQL.

### Decision

We adopt a three-pillar evaluation stack, all running in the Python worker:

#### DeepEval (Primary Metrics)

- LLM-as-judge evaluation using `GEval`, `FaithfulnessMetric`, and `AnswerRelevancyMetric`
- Custom `LiteLLMJudge` wrapper routes judge calls through the existing LiteLLM Proxy
- `BenchmarkRunner` loads YAML datasets, executes tasks, and evaluates with configured metrics
- Results persisted to PostgreSQL via Go Core API

#### AgentNeo (Tracing & Observability)

- Optional dependency (`agentneo`) for dev-mode agent execution tracing
- `TracingManager` with graceful degradation to `_NoOpTracer` when not installed
- Three metric wrappers: tool selection accuracy, goal decomposition, plan adaptability
- Optional React dashboard on configurable port (default: 3100)

#### GEMMAS-Inspired Collaboration Metrics

- Custom implementation of Information Diversity Score (IDS) and Unnecessary Path Ratio (UPR)
- Based on the GEMMAS paper ([arxiv.org/abs/2507.13190](https://arxiv.org/abs/2507.13190)), adapted for our DAG orchestration model
- TF-IDF cosine similarity (scikit-learn) for content diversity measurement
- `CollaborationDAG` builder with spatial and temporal adjacency matrices

#### Go Core API

CRUD endpoints for benchmark runs/results behind a `DevModeOnly` middleware that checks `APP_ENV=development`. Migration 041 adds `benchmark_runs` and `benchmark_results` tables.

#### Frontend

Unified `BenchmarkPage` with run management, dataset selection, results inspection, and run comparison.

### Consequences

#### Positive

- Standardized quality measurement across all LLM models and configurations
- LLM-as-judge via LiteLLM means any configured model can serve as evaluator
- Dev-mode gate ensures zero production overhead
- YAML-based datasets are easy to create and version-control
- Collaboration metrics prepare for multi-agent DAG workflows
- AgentNeo tracing is optional and degrades gracefully

#### Negative

- LLM-as-judge has inherent cost overhead (judge model calls per evaluation)
- DeepEval and scikit-learn add ~100MB to the Python worker image
- AgentNeo is an early-stage project with limited documentation
- GEMMAS metrics are our own implementation (no reference code from the paper)
- Benchmark runs are only available in dev mode, not for production monitoring

#### Neutral

- Benchmark datasets use YAML format (consistent with project-wide YAML preference)
- Results are stored in PostgreSQL alongside application data (same migration system)
- NATS `benchmark.>` subjects follow existing subject naming convention

### Alternatives Considered

| Alternative | Pros | Cons | Why Not |
|---|---|---|---|
| SPARC-Bench | Purpose-built for coding agents | Tightly coupled to Roo Code, not open-source | Vendor lock-in, no reuse possible |
| REALM-Bench | Multi-step agent evaluation | No OSS license, logistics-domain focused | Wrong domain, unavailable |
| RAGAS | Mature RAG evaluation framework | RAG-only, no agent/tool metrics | Too narrow for agent orchestration |
| Custom from scratch | Full control, minimal deps | Significant development effort, no LLM-as-judge baseline | DeepEval provides validated metrics |
| MLflow + custom metrics | Mature experiment tracking | Heavy infrastructure, not agent-focused | Over-engineered for dev-mode benchmarks |

### References

- DeepEval documentation: [docs.confident-ai.com](https://docs.confident-ai.com/)
- AgentNeo repository: [github.com/raga-ai-hub/agentneo](https://github.com/raga-ai-hub/agentneo)
- GEMMAS paper: [arxiv.org/abs/2507.13190](https://arxiv.org/abs/2507.13190)
- LiteLLM documentation: [docs.litellm.ai](https://docs.litellm.ai/)
- ADR-006: Agent Execution Approach C (Go control plane + Python runtime)
