# Feature: Agent Orchestration (Pillar 4)

> **Status:** Design phase
> **Priority:** Phase 2 (MVP basic) + Phase 3 (Advanced multi-agent)
> **Architecture reference:** [architecture.md](../architecture.md) — "Agent Execution", "Worker Modules", "Modes System"

## Overview

Coordination of various AI coding agents through a unified orchestration layer.
Agents are swappable backends that run in configurable execution modes with
safety controls and quality assurance.

## Agent Backends

| Agent | Adapter | Priority | Type |
|---|---|---|---|
| Aider | `adapter/aider/` | Existing | Full-featured (own tools) |
| OpenHands | `adapter/openhands/` | Existing | Full-featured (own tools) |
| SWE-agent | `adapter/sweagent/` | Existing | ReAct loop, ACI |
| Goose | `adapter/goose/` | Priority 1 | MCP-native, Rust |
| OpenCode | `adapter/opencode/` | Priority 1 | Go, LSP-aware |
| Plandex | `adapter/plandex/` | Priority 1 | Planning-first, diff sandbox |

All backends implement the `agentbackend.Backend` interface with capability declarations.

## Execution Modes

| Mode | Security | Speed | Use Case |
|---|---|---|---|
| **Sandbox** | High (isolated container) | Medium | Untrusted agents, batch jobs |
| **Mount** | Low (direct file access) | High | Trusted agents, local dev |
| **Hybrid** | Medium (controlled access) | Medium | Review workflows, CI-like |

## Agent Workflow

```
Plan → Approve → Execute → Review → Deliver
```

Each step is individually configurable. Autonomy level determines who approves.

## Autonomy Spectrum (5 Levels)

| Level | Name | Who Approves | Use Case |
|---|---|---|---|
| 1 | `supervised` | User at every step | Learning, critical codebases |
| 2 | `semi-auto` | User for destructive actions | Everyday development |
| 3 | `auto-edit` | User only for terminal/deploy | Experienced users |
| 4 | `full-auto` | Safety rules | Batch jobs, delegated tasks |
| 5 | `headless` | Safety rules, no UI | CI/CD, cron jobs, API |

## Safety Layer (8 Components)

1. **Budget Limiter** — hard stop on cost exceeded
2. **Command Safety Evaluator** — blocklist + regex matching
3. **Branch Isolation** — never on main, always feature branch
4. **Test/Lint Gate** — deliver only when tests + lint pass
5. **Max Steps** — infinite loop detection
6. **Rollback** — automatic on failure (Shadow Git)
7. **Path Blocklist** — sensitive files protected
8. **Stall Detection** — re-planning or abort

## Quality Layer (4 Tiers)

1. **Action Sampling** (light) — N responses, select best
2. **RetryAgent + Reviewer** (medium) — retry + score/chooser evaluation
3. **LLM Guardrail Agent** (medium) — dedicated agent checks output
4. **Multi-Agent Debate** (heavy) — Pro/Con/Moderator

## Modes System

YAML-configurable agent specializations:
- **Built-in:** architect, coder, reviewer, debugger, tester, lint-fixer, planner, researcher
- **Custom:** user-defined in `.codeforge/modes/`
- **Composition:** pipelines and DAG workflows

## Worker Modules

| Module | Purpose |
|---|---|
| Context (GraphRAG) | Vector search + graph DB + web fallback |
| Quality | Debate, reviewer, sampler, guardrail |
| Routing | Task-based model routing via LiteLLM |
| Safety | Command evaluation, blocklists, policies |
| Execution | Sandbox/mount management, tool provisioning |
| Memory | Composite scoring, context strategies, experience pool |
| History | Context window optimization pipeline |
| Events | Event bus for observability |
| Orchestration | DAG flow, termination conditions, handoff, planning loop |
| Hooks | Agent/environment lifecycle observer |
| Trajectory | Recording, replay, audit trail |
| HITL | Human feedback provider protocol |

## Policy System

The policy layer governs agent permissions, quality gates, and termination conditions.

### Backend
- **Domain:** `internal/domain/policy/` — PolicyProfile, PermissionRule, ToolSpecifier, QualityGate, TerminationCondition
- **Presets (4):** plan-readonly, headless-safe-sandbox, headless-permissive-sandbox, trusted-mount-autonomous
- **Service:** `internal/service/policy.go` — first-match-wins rule evaluation, CRUD (SaveProfile, DeleteProfile)
- **Loader:** `internal/domain/policy/loader.go` — YAML file loading + SaveToFile for custom profiles
- **REST API:** GET/POST /policies, GET/DELETE /policies/{name}, POST /policies/{name}/evaluate

### Frontend (PolicyPanel)
- **Component:** `frontend/src/features/project/PolicyPanel.tsx`
- **3 views:** List (presets + custom), Detail (summary + rules table + evaluate tester), Editor (create/clone)
- **Evaluate tester:** test a tool call against a policy and see the decision (allow/deny/ask)
- **Types:** `PolicyProfile`, `PermissionRule`, `PolicyQualityGate`, `TerminationCondition`, `ResourceLimits`

### Deferred
- Scope levels: global (user) → project → run/session (override)
- "Effective Permission Preview" — show which rule matched and why
- Run-level policy overrides

## Retrieval Sub-Agent (Phase 6C)

LLM-guided multi-query retrieval that improves context quality for agents working on complex tasks.

### Architecture

```
Go Core (RetrievalService)
  |
  | NATS: retrieval.subagent.request
  v
Python Worker (RetrievalSubAgent)
  |-- 1. LLM query expansion (task prompt -> N focused queries)
  |-- 2. Parallel hybrid searches (existing HybridRetriever.search() x N)
  |-- 3. Deduplication (by filepath+start_line)
  |-- 4. LLM re-ranking (top candidates scored for relevance)
  |-- 5. Return top-K results
  |
  | NATS: retrieval.subagent.result
  v
Go Core (handles result, delivers to waiter or HTTP handler)
```

### Backend
- **Python:** `RetrievalSubAgent` in `workers/codeforge/retrieval.py` — composes `HybridRetriever` + `LiteLLMClient`
- **Go Service:** `SubAgentSearchSync()` / `HandleSubAgentSearchResult()` in `internal/service/retrieval.go`
- **Context Optimizer:** `fetchRetrievalEntries()` tries sub-agent first, falls back to single-shot search
- **REST API:** `POST /api/v1/projects/{id}/search/agent`
- **Config:** `SubAgentModel`, `SubAgentMaxQueries`, `SubAgentRerank` in `config.Orchestrator`

### Frontend (RetrievalPanel)
- Standard/Agent toggle button next to search bar
- Agent mode shows expanded queries as tags + total candidates count
- Component: `frontend/src/features/project/RetrievalPanel.tsx`

### Deferred
- Configurable expansion prompts per project
- Streaming results (partial results as queries complete)
- Cost tracking for sub-agent LLM calls

## TODOs

Tracked in [todo.md](../todo.md) under Phase 1, Phase 2, and Phase 3.

### Phase 1
- [ ] `agentbackend.Backend` interface definition
- [ ] Agent backend registry (`port/agentbackend/registry.go`)
- [ ] Basic queue consumer (Python worker)
- [ ] Minimal agent execution framework

### Phase 2
- [ ] Aider backend adapter
- [ ] Simple task → single agent execution
- [ ] Mount mode implementation
- [ ] Basic safety evaluator
- [ ] Frontend: Agent Monitor (live logs, status)
- [ ] Frontend: Task submission form

### Phase 3
- [ ] Sandbox mode (Docker-in-Docker)
- [ ] Multi-agent orchestration (pipelines, DAGs)
- [ ] Quality Layer implementation
- [ ] Modes System (YAML config loader)
- [ ] Trajectory recording and replay
- [ ] Additional backends (OpenHands, Goose, OpenCode, Plandex)
