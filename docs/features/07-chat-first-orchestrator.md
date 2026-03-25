# Chat-First Orchestrator (Phase 33)

## Overview

Transforms CodeForge from a panel-first project manager into a chat-first orchestrator where the LLM drives 80% of the workflow.

## Architecture

The chat is the single orchestration hub. The Orchestrator behavior prompt (`internal/service/prompts/behavior/chat_first_orchestration.yaml`) makes the LLM proactively:
1. **Extract goals** from natural conversation using `propose_goal` tool
2. **Generate roadmaps** with atomic work steps using `propose_roadmap` tool
3. **Spawn sub-agents** for research/debate/implementation using `spawn_subagent` tool
4. **Suggest next actions** proactively after each user message

## New Components

### Backend (Go)
- `AGUIRoadmapProposal` event type (`internal/domain/event/agui.go`)
- Trajectory event routing for `agent.roadmap_proposed` and `agent.subagent_requested` (`internal/service/runtime.go`)

### Python Tools
- `propose_roadmap` — propose milestones and atomic work steps with complexity/model-tier mapping
- `spawn_subagent` — delegate tasks to sub-agents (researcher, implementer, reviewer, debater)

### Frontend
- `RoadmapProposalCard` — inline approval card for roadmap proposals in chat
- `PanelChatLink` — "Discuss in Chat" deep-link button for panel items
- `ConsolidatedPlanView` — combined Goals + Roadmap + FeatureMap view
- Panel consolidation: 14+ panels reduced to 4 views (Plan/Execute/Code/Govern)
- Deep-link callback: panels can pre-fill chat input with item references

## AG-UI Event Flow

```
User describes project in chat
  -> LLM detects goals -> propose_goal tool -> agent.goal_proposed trajectory event
  -> Go backend -> agui.goal_proposal WS broadcast -> GoalProposalCard in chat
  -> User approves -> saved to DB

LLM offers roadmap generation
  -> propose_roadmap tool -> agent.roadmap_proposed trajectory event
  -> Go backend -> agui.roadmap_proposal WS broadcast -> RoadmapProposalCard in chat
  -> User approves milestones/steps -> saved to Roadmap

Panel deep-links
  -> User clicks "Discuss" on goal/step in panel
  -> PanelChatLink fires onSendChatMessage callback
  -> ProjectDetailPage sets prefillMessage signal
  -> ChatPanel populates input with [type:ID] reference
```

## Atomic Step Design (from MAKER/ADaPT research)

Each roadmap step must be:
- One file, one function, one test at a time
- Independently verifiable (clear success/failure criteria)
- Executable by a weak LLM (7B-30B params) with only local context
- Complexity-mapped to model tier: trivial/simple -> weak, medium -> mid, complex -> strong

## Panel Consolidation

| Old (14+ panels) | New (4 views) |
|---|---|
| Goals, Roadmap, FeatureMap, Tasks, Plans | **Plan** (ConsolidatedPlanView) |
| WarRoom, Agents, Sessions, Trajectory | **Execute** |
| Files, RepoMap, Retrieval, Boundaries | **Code** |
| Policy, Audit | **Govern** |

## Research References

- MAKER (arXiv:2511.09030) — Microagent atomic decomposition
- ADaPT (NAACL 2024) — Recursive decomposition adapting to LLM capability
- COPE (arXiv:2506.11578) — Strong-plans/weak-executes with confidence escalation
- Squad (GitHub Blog) — Thin coordinator routing to specialists
- IntentFlow (arXiv:2507.22134) — Structured intent persistence
