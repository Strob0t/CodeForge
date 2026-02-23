# CodeForge — E2E Vision Test Plan

> **Purpose:** Validate that all 4 CodeForge pillars work together end-to-end with real LLM calls.
> **Scope:** All 4 pillars (Project Dashboard, Roadmap, Multi-LLM, Agent Orchestration) + cross-pillar integration.
> **Date:** 2026-02-23

## Overview

After completing HTTP endpoint QA testing (131 PASS, 0 FAIL), this E2E test validates the actual *vision*:
Can CodeForge manage projects, track roadmaps, route LLM calls, and orchestrate AI agents as one integrated system?

### Infrastructure Requirements

| Service | Status | Access |
|---------|--------|--------|
| Go Core | Running | `http://localhost:8080` |
| PostgreSQL | Docker (healthy) | `localhost:5432` |
| NATS JetStream | Docker (healthy) | `localhost:4222` / `localhost:8222` (monitoring) |
| LiteLLM | Docker (healthy) | `http://codeforge-litellm:4000` (internal) |
| Python Worker | Manual start | `python -m codeforge.consumer` |

### Verified LLM Models

| Provider | Model | Status |
|----------|-------|--------|
| Groq | `groq/llama-3.3-70b-versatile` | Working |
| Mistral | `mistral/mistral-large-latest` | Working |
| Gemini | `gemini-2.0-flash` | Quota exceeded |
| OpenAI | `gpt-4o-mini` | No API key |

---

## Test Phases

### Phase 0: Infrastructure Bootstrap

| # | Test | Endpoint | Expected |
|---|------|----------|----------|
| 0.1 | Go Core liveness | `GET /health` | `{"status":"ok"}` |
| 0.2 | Dependency readiness | `GET /health/ready` | postgres=up, nats=up, litellm=up |
| 0.3 | LLM connectivity | LiteLLM `/v1/chat/completions` | 200 with completion |
| 0.4 | NATS health | `GET localhost:8222/healthz` | `ok` |

### Phase 1: Pillar 1 — Project Dashboard

| # | Test | Endpoint | Expected |
|---|------|----------|----------|
| 1.1 | Create project | `POST /api/v1/projects` | 201, project ID |
| 1.2 | Clone repository | `POST /api/v1/projects/{id}/clone` | 200, workspace path |
| 1.3 | Verify workspace | `GET /api/v1/projects/{id}/workspace` | `exists: true` |
| 1.4 | Detect stack | `GET /api/v1/projects/{id}/detect-stack` | Go, Python, TypeScript detected |
| 1.5 | Git status | `GET /api/v1/projects/{id}/git/status` | 200, branch + commit hash |
| 1.6 | Git branches | `GET /api/v1/projects/{id}/git/branches` | main + staging in list |

### Phase 2: Pillar 2 — Roadmap/Feature-Map

| # | Test | Endpoint | Expected |
|---|------|----------|----------|
| 2.1 | Create roadmap | `POST /api/v1/projects/{id}/roadmap` | 201/200, roadmap data |
| 2.2 | Create milestone | `POST /api/v1/projects/{id}/roadmap/milestones` | 201, milestone ID |
| 2.3 | Create feature | `POST /api/v1/milestones/{id}/features` | 201, feature ID |
| 2.4 | AI view (JSON) | `GET /api/v1/projects/{id}/roadmap/ai?format=json` | 200, parseable JSON |
| 2.5 | AI view (YAML) | `GET /api/v1/projects/{id}/roadmap/ai?format=yaml` | 200, parseable YAML |
| 2.6 | AI view (Markdown) | `GET /api/v1/projects/{id}/roadmap/ai?format=markdown` | 200, valid markdown |
| 2.7 | Spec detection | `POST /api/v1/projects/{id}/roadmap/detect` | 200, detection result |
| 2.8 | Spec import | `POST /api/v1/projects/{id}/roadmap/import` | 200, counts returned |
| 2.9 | GitHub PM import | `POST /api/v1/projects/{id}/roadmap/import/pm` | 200, features imported |
| 2.10 | Bidirectional sync (dry-run) | `POST /api/v1/projects/{id}/roadmap/sync` | 200, sync counts |

### Phase 3: Pillar 3 — Multi-LLM Provider

| # | Test | Endpoint | Expected |
|---|------|----------|----------|
| 3.1 | List models | `GET /api/v1/llm/models` | 200, multiple models |
| 3.2 | LLM health | `GET /api/v1/llm/health` | 200, health status |
| 3.3 | Direct LLM call (Groq) | LiteLLM completions | 200, completion text |
| 3.4 | Direct LLM call (Mistral) | LiteLLM completions | 200, completion text |
| 3.5 | Provider registry | `GET /api/v1/providers/agent` | Lists agent backends |
| 3.6 | Global costs | `GET /api/v1/costs` | 200, cost data |

### Phase 4: Pillar 4 — Agent Orchestration

| # | Test | Endpoint | Expected |
|---|------|----------|----------|
| 4.1 | Create agent | `POST /api/v1/projects/{id}/agents` | 201, agent ID |
| 4.2 | Create task | `POST /api/v1/projects/{id}/tasks` | 201, task ID |
| 4.3 | Policy evaluate (allow) | `POST /api/v1/policies/{name}/evaluate` | `allow` |
| 4.4 | Policy evaluate (deny) | `POST /api/v1/policies/{name}/evaluate` | `deny` |
| 4.5 | List modes | `GET /api/v1/modes` | 200, multiple modes |
| 4.6 | Get coder mode | `GET /api/v1/modes/coder` | 200, tools include Read/Write/Edit |
| 4.7 | Get reviewer mode | `GET /api/v1/modes/reviewer` | 200, no Write/Edit/Bash tools |
| 4.8 | Start run | `POST /api/v1/runs` | 200/201, run ID |
| 4.9 | Poll run status | `GET /api/v1/runs/{id}` | Status transitions |
| 4.10 | Run events | `GET /api/v1/runs/{id}/events` | Event list |

### Phase 5: Cross-Pillar Integration

| # | Test | Endpoint | Expected |
|---|------|----------|----------|
| 5.1 | Build repomap | `POST /api/v1/projects/{id}/repomap` | 200/202, job started |
| 5.2 | Build index | `POST /api/v1/projects/{id}/index` | 200/202, indexing started |
| 5.3 | Code search | `POST /api/v1/projects/{id}/search` | Results with scores |
| 5.4 | Create scope | `POST /api/v1/scopes` | 201, scope ID |
| 5.5 | Scope search | `POST /api/v1/scopes/{id}/search` | Results returned |
| 5.6 | Create review policy | `POST /api/v1/projects/{id}/review-policies` | 201, policy ID |
| 5.7 | List review policies | `GET /api/v1/projects/{id}/review-policies` | Policy in list |
| 5.8 | Execution plan CRUD | `POST /api/v1/projects/{id}/plans` | 201, plan created |
| 5.9 | Project costs | `GET /api/v1/projects/{id}/costs` | Cost data |
| 5.10 | Cost by model | `GET /api/v1/projects/{id}/costs/by-model` | Breakdown data |

### Phase 6: WebSocket Live Events

| # | Test | Method | Expected |
|---|------|--------|----------|
| 6.1 | WS connection | `ws://localhost:8080/ws` | Connection established |
| 6.2 | Receive events during run | Monitor WS | `run.status`, `task.status` events |

---

## Deliverables

1. **This document** — `docs/e2e-test-plan.md`
2. **Executable script** — `/tmp/e2e-vision-test.sh` (automated PASS/FAIL)

## Execution

```bash
chmod +x /tmp/e2e-vision-test.sh
/tmp/e2e-vision-test.sh
```

Results: PASS/FAIL/SKIP counts per phase, total cost summary, recommendations.
