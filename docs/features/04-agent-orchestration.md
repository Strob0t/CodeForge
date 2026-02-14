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
