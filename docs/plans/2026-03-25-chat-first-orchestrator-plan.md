# Chat-First Orchestrator — Implementation Plan

> **For agentic workers:** Use the executing-plans workflow to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform CodeForge from a panel-first project manager into a chat-first orchestrator where the LLM drives 80% of the workflow — goal extraction, roadmap generation, task decomposition, and sub-agent coordination — while graphical panels serve as read-views with adjustment capabilities and deep-links back into the chat.

**Architecture:** Inspired by MAKER (atomic microagent decomposition), ADaPT (recursive decomposition adapting to executor capability), COPE (strong-plans/weak-executes with confidence escalation), Squad (thin coordinator routing to specialists), IntentFlow (structured intent persistence across conversation turns), and GitHub Spec Kit (conversation -> spec -> plan -> execute). The chat becomes the single orchestration hub. A new AG-UI event (`roadmap_proposal`) streams structured proposals to the frontend. A new Orchestrator behavior prompt makes the LLM proactively detect goals, generate roadmaps, and suggest next actions. Panel items deep-link back to chat for discussion. Panel navigation consolidates from 14+ panels into 4 contextual views.

**Tech Stack:** SolidJS (frontend), Go (backend services), Python (agent loop + tools), NATS JetStream (messaging), AG-UI (streaming events), PostgreSQL (persistence)

**Research Sources:**
- [MAKER: Solving a Million-Step LLM Task with Zero Errors](https://arxiv.org/abs/2511.09030) — Microagent atomic decomposition + voting error correction
- [ADaPT: As-Needed Decomposition and Planning](https://aclanthology.org/2024.findings-naacl.264/) — Recursive decomposition adapting to executor LLM capability
- [COPE: Efficient LLM Collaboration via Planning](https://arxiv.org/html/2506.11578) — Strong model plans, weak model executes, confidence-based escalation
- [TDP: Task-Decoupled Planning](https://arxiv.org/html/2601.07577v1) — DAG-based sub-task decomposition with isolated replanning
- [IntentFlow: Interactive Intent Communication with LLMs](https://arxiv.org/html/2507.22134v1) — Structured intent persistence across conversation turns
- [Squad: Coordinated AI Agents](https://github.blog/ai-and-ml/github-copilot/how-squad-runs-coordinated-ai-agents-inside-your-repository/) — Thin coordinator, specialist routing, independent reviewer protocol
- [ComposioHQ/agent-orchestrator](https://github.com/ComposioHQ/agent-orchestrator) — Git-worktree-isolated parallel agents, event-driven coordination
- [ChatDev 2.0](https://github.com/OpenBMB/ChatDev) — Chat chain as orchestration mechanism, phase-based agent communication
- [VS Code Agents / Flowbaby](https://github.com/groupzer0/vs-code-agents) — Pipeline Roadmap->Plan->Implement->Review->QA, document-driven handoff
- [GitHub Spec Kit](https://github.blog/ai-and-ml/generative-ai/spec-driven-development-with-ai-get-started-with-a-new-open-source-toolkit/) — Conversation generates spec, /plan generates implementation plan

---

## File Structure

### New Files

| File | Responsibility |
|---|---|
| `workers/codeforge/tools/propose_roadmap.py` | Agent tool to propose milestones + atomic work steps via AG-UI |
| `workers/codeforge/tools/spawn_subagent.py` | Agent tool to spawn sub-agents for research/debate/implementation |
| `internal/service/prompts/behavior/chat_first_orchestration.yaml` | Orchestrator behavior prompt — injected for all project conversations |
| `frontend/src/features/project/RoadmapProposalCard.tsx` | Inline roadmap proposal approval card (chat) |
| `frontend/src/features/project/PanelChatLink.tsx` | Reusable "Discuss in Chat" deep-link component |
| `frontend/src/features/project/ConsolidatedPlanView.tsx` | Combined Goals + Roadmap + FeatureMap view |

### Modified Files

| File | Change |
|---|---|
| `internal/domain/event/agui.go` | Add `AGUIRoadmapProposal` event type + struct |
| `internal/service/runtime.go:752-785` | Add `agent.roadmap_proposed` trajectory event routing (next to existing `goal_proposed`) |
| `internal/service/conversation_prompt.go:76+` | Ensure orchestrator behavior prompt is loaded by `PromptAssemblyService` |
| `frontend/src/api/websocket.ts` | Add `agui.roadmap_proposal` event type + interface |
| `frontend/src/features/project/useChatAGUI.ts` | Handle `agui.roadmap_proposal` event, expose `roadmapProposals` signal |
| `frontend/src/features/project/chatPanelTypes.ts` | Add `RoadmapProposalState` type |
| `frontend/src/features/project/ChatMessages.tsx` | Render `RoadmapProposalCard` inline |
| `frontend/src/features/project/ChatPanel.tsx` | Pass roadmapProposals to ChatMessages, add deep-link message handler |
| `frontend/src/features/project/GoalsPanel.tsx` | Add "Discuss" button per goal, de-emphasize manual CRUD |
| `frontend/src/features/project/RoadmapPanel.tsx` | Add "Discuss" button per step, de-emphasize manual creation |
| `frontend/src/features/project/FeatureMapPanel.tsx` | Add "Discuss" button per feature card |
| `frontend/src/features/project/ProjectDetailPage.tsx` | Consolidate panel groups, wire deep-link callback |
| `workers/codeforge/tools/propose_goal.py` | Update description for proactive use |
| `workers/codeforge/consumer/_conversation_skill_integration.py` | Add `register_propose_roadmap_tool`, `register_spawn_subagent_tool` |
| `workers/codeforge/consumer/_conversation.py` | Import and call new registration functions |

**Note:** `propose_plan.py` and `PlanProposalCard.tsx` are deferred to a follow-up plan. This plan focuses on the core chat-first loop: goals -> roadmap -> sub-agents.

---

## Phase A: Orchestrator Persona & Auto Goal Extraction

**Objective:** Make the default chat agent a "Project Orchestrator" that proactively detects goals from natural conversation and uses `propose_goal` automatically — no separate "AI Discover" button needed.

### Task 1: Create Orchestrator Behavior Prompt

**Files:**
- Create: `internal/service/prompts/behavior/chat_first_orchestration.yaml`

The prompt follows the established schema used by `PromptAssembler` (see existing files in `internal/service/prompts/`): `id`, `category`, `name`, `priority`, `sort_order`, `conditions`, `content`.

- [ ] **Step 1: Write the orchestrator behavior prompt YAML**

```yaml
id: behavior.chat_first_orchestration
category: behavior
name: Chat-First Orchestration
priority: 80
sort_order: 5
conditions: {}
content: |
  ## Project Orchestrator Behavior

  You drive the entire development workflow through conversation. You are proactive, not reactive.

  ### 1. Goal Extraction (always active)
  As the user describes what they want to build, you MUST detect goals embedded
  in their language. When you identify a goal, immediately use the `propose_goal`
  tool to propose it — do NOT wait for the user to ask.
  - Vision statements -> kind: "vision"
  - Functional requirements -> kind: "requirement"
  - Technical constraints -> kind: "constraint"
  - Current state descriptions -> kind: "state"
  - Background context -> kind: "context"

  ### 2. Roadmap Generation (after goals are established)
  Once 2+ goals are approved, proactively offer: "Shall I generate a roadmap
  with atomic work steps from your goals?"
  When confirmed, use `propose_roadmap` to propose milestones with atomic steps.
  Each step MUST be small enough for a weak LLM to execute independently:
  - One file, one function, one test at a time
  - Clear input/output per step
  - No implicit dependencies between steps in different milestones

  ### 3. Sub-Agent Coordination (during execution)
  When a step requires research, debate, or specialized expertise,
  use `spawn_subagent` to delegate. Sub-agents can:
  - Research: gather information, analyze code, read documentation
  - Debate: two agents argue pros/cons, you synthesize
  - Implement: execute atomic work steps

  ### 4. Proactive Suggestions
  After each user message, consider suggesting the next logical action:
  - No goals yet? -> "Let me understand your project. What are you building and why?"
  - Goals exist, no roadmap? -> "Ready to generate a roadmap from your goals?"
  - Roadmap exists, not started? -> "Shall I start implementing? Which steps first?"

  ### 5. Deep-Link Awareness
  When a message contains a reference like `[goal:ID]` or `[roadmap-step:ID]`,
  load that item's context and discuss it specifically.

  ### 6. Atomic Step Design (from MAKER/ADaPT research)
  When decomposing work into steps:
  - Each step = one microagent decision (read one file, write one function, run one test)
  - Steps are independently verifiable (clear success/failure criteria)
  - A weak LLM (7B-30B params) must be able to execute each step with only local context
  - If a step requires understanding >3 files simultaneously, decompose further

  ### 7. Confidence-Based Escalation (from COPE research)
  When assigning steps to agents:
  - Simple steps (file read, single-function edit) -> route to cheapest available model
  - Medium steps (multi-function, needs context) -> route to mid-tier model
  - Complex steps (architecture decisions, cross-cutting) -> route to strongest model
```

- [ ] **Step 2: Verify YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('internal/service/prompts/behavior/chat_first_orchestration.yaml'))" && echo "OK"`
Expected: `OK`

- [ ] **Step 3: Verify prompt is loaded by PromptAssembler**

The `PromptAssembler` uses `//go:embed prompts` to embed the entire `prompts/` directory. Since `behavior/` already exists as a category, the new file will be auto-discovered. Verify:

Run: `go test ./internal/service/ -run TestPromptAssembler -v -count=1`
Expected: Existing tests PASS (new prompt loaded but not breaking anything, since `conditions: {}` means it applies broadly).

- [ ] **Step 4: Commit**

```bash
git add internal/service/prompts/behavior/chat_first_orchestration.yaml
git commit -m "feat: add chat-first orchestration behavior prompt"
```

### Task 2: Verify Orchestrator Prompt Reaches System Prompt

**Files:**
- Modify: `internal/service/conversation_prompt.go` (verify BuildSystemPrompt includes behavior prompts)
- Test: `internal/service/conversation_prompt_test.go`

- [ ] **Step 1: Read BuildSystemPrompt in conversation_prompt.go**

Read: `internal/service/conversation_prompt.go` lines 76-end to understand how prompts are assembled.

- [ ] **Step 2: Write test for orchestrator prompt presence**

```go
func TestBuildSystemPrompt_IncludesChatFirstOrchestration(t *testing.T) {
    svc := newTestPromptAssemblyService(t)
    ctx := tenantctx.WithTenant(context.Background(), testTenantID)
    prompt := svc.BuildSystemPrompt(ctx, testProjectID)
    assert.Contains(t, prompt, "Goal Extraction")
    assert.Contains(t, prompt, "propose_goal")
    assert.Contains(t, prompt, "Roadmap Generation")
    assert.Contains(t, prompt, "propose_roadmap")
    assert.Contains(t, prompt, "Atomic Step Design")
}
```

- [ ] **Step 3: Run test**

Run: `go test ./internal/service/ -run TestBuildSystemPrompt_IncludesChatFirstOrchestration -v`
Expected: If the `PromptAssembler` auto-loads `behavior/` prompts with empty conditions, this should PASS. If it fails, the conditions may need adjustment (e.g., adding a `default: true` flag), or `BuildSystemPrompt` may need to explicitly include behavior-category prompts. Fix accordingly.

- [ ] **Step 4: Commit**

```bash
git add internal/service/conversation_prompt.go internal/service/conversation_prompt_test.go
git commit -m "test: verify chat-first orchestration prompt reaches system prompt"
```

### Task 3: Make propose_goal Proactive

**Files:**
- Modify: `workers/codeforge/tools/propose_goal.py:13-18`

- [ ] **Step 1: Update propose_goal description for proactive use**

In `propose_goal.py`, update `PROPOSE_GOAL_DEFINITION` at lines 13-18:

```python
PROPOSE_GOAL_DEFINITION = ToolDefinition(
    name="propose_goal",
    description=(
        "Propose a project goal for user review. Use this PROACTIVELY "
        "whenever you detect a goal, requirement, constraint, or context "
        "in the user's message. Do NOT wait for explicit instructions."
    ),
    # ... keep parameters unchanged ...
    when_to_use=(
        "Use this tool whenever you detect goal-like intent in the conversation: "
        "vision statements, requirements, constraints, current state, or context. "
        "Propose goals one at a time as you detect them. The user will approve or reject."
    ),
    # ... keep rest unchanged ...
)
```

- [ ] **Step 2: Verify propose_goal is already unconditionally registered**

Confirm in `_conversation.py:264` that `register_propose_goal_tool(registry, runtime)` is called for all conversation runs, not just AI Discover mode. If conditional, make unconditional.

- [ ] **Step 3: Run existing tests**

Run: `cd workers && python -m pytest tests/ -k propose_goal -v`
Expected: All existing tests PASS

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/tools/propose_goal.py
git commit -m "feat: make propose_goal proactive — always available, no explicit trigger needed"
```

---

## Phase B: Roadmap Proposal Tool & AG-UI Event

**Objective:** Enable the orchestrator to propose roadmaps with atomic work steps directly in the chat, with inline approval cards.

### Task 4: Add AG-UI Roadmap Proposal Event (Go Backend)

**Files:**
- Modify: `internal/domain/event/agui.go`

- [ ] **Step 1: Read current agui.go**

Read: `internal/domain/event/agui.go`

- [ ] **Step 2: Add constant after `AGUIActionSuggestion` (line 90)**

```go
AGUIRoadmapProposal = "agui.roadmap_proposal"
```

- [ ] **Step 3: Add event struct after `AGUIGoalProposalEvent` (line 119)**

```go
// AGUIRoadmapProposalEvent carries a proposed milestone or work step.
type AGUIRoadmapProposalEvent struct {
	RunID                string `json:"run_id"`
	ProposalID           string `json:"proposal_id"`
	Action               string `json:"action"` // "create_milestone", "create_step"
	MilestoneTitle       string `json:"milestone_title"`
	MilestoneDescription string `json:"milestone_description,omitempty"`
	MilestoneSortOrder   int    `json:"milestone_sort_order,omitempty"`
	StepTitle            string `json:"step_title,omitempty"`
	StepDescription      string `json:"step_description,omitempty"`
	StepSortOrder        int    `json:"step_sort_order,omitempty"`
	StepComplexity       string `json:"step_complexity,omitempty"` // trivial, simple, medium, complex
	StepModelTier        string `json:"step_model_tier,omitempty"` // weak, mid, strong
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/domain/event/`
Expected: Success (exit 0)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/event/agui.go
git commit -m "feat: add AGUIRoadmapProposal event type and struct"
```

### Task 5: Route Trajectory Event to AG-UI WebSocket (Go Backend)

**Files:**
- Modify: `internal/service/runtime.go:785` (insert after goal_proposed block)
- Test: `internal/service/runtime_test.go`

- [ ] **Step 1: Write test for roadmap event routing**

```go
func TestTrajectoryEvent_RoadmapProposed_BroadcastsAGUI(t *testing.T) {
	// Setup mock hub and services
	hub := &mockBroadcastHub{}
	svc := &RuntimeService{hub: hub, goalSvc: nil}

	// Simulate trajectory event with agent.roadmap_proposed
	payload := trajectoryPayload{
		RunID:     "run-1",
		ProjectID: "proj-1",
		EventType: "agent.roadmap_proposed",
	}
	data := []byte(`{"data":{"proposal_id":"p1","action":"create_milestone","milestone_title":"Auth System","milestone_description":"JWT auth"}}`)

	// Call the handler
	svc.handleTrajectoryEvent(context.Background(), payload, data)

	// Assert broadcast
	require.Equal(t, 1, hub.broadcastCount)
	require.Equal(t, event.AGUIRoadmapProposal, hub.lastEventType)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestTrajectoryEvent_RoadmapProposed -v`
Expected: FAIL (handler not implemented yet)

- [ ] **Step 3: Add roadmap_proposed handler in runtime.go**

Insert after the `agent.goal_proposed` block (line 785), before `return nil`:

```go
// Roadmap proposal events get a dedicated AG-UI broadcast.
if payload.EventType == "agent.roadmap_proposed" {
	var proposal struct {
		Data struct {
			ProposalID           string `json:"proposal_id"`
			Action               string `json:"action"`
			MilestoneTitle       string `json:"milestone_title"`
			MilestoneDescription string `json:"milestone_description"`
			MilestoneSortOrder   int    `json:"milestone_sort_order"`
			StepTitle            string `json:"step_title"`
			StepDescription      string `json:"step_description"`
			StepSortOrder        int    `json:"step_sort_order"`
			StepComplexity       string `json:"step_complexity"`
			StepModelTier        string `json:"step_model_tier"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &proposal); err == nil {
		s.hub.BroadcastEvent(msgCtx, event.AGUIRoadmapProposal, event.AGUIRoadmapProposalEvent{
			RunID:                payload.RunID,
			ProposalID:           proposal.Data.ProposalID,
			Action:               proposal.Data.Action,
			MilestoneTitle:       proposal.Data.MilestoneTitle,
			MilestoneDescription: proposal.Data.MilestoneDescription,
			MilestoneSortOrder:   proposal.Data.MilestoneSortOrder,
			StepTitle:            proposal.Data.StepTitle,
			StepDescription:      proposal.Data.StepDescription,
			StepSortOrder:        proposal.Data.StepSortOrder,
			StepComplexity:       proposal.Data.StepComplexity,
			StepModelTier:        proposal.Data.StepModelTier,
		})
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestTrajectoryEvent_RoadmapProposed -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/runtime.go internal/service/runtime_test.go
git commit -m "feat: route agent.roadmap_proposed trajectory events to agui.roadmap_proposal WS broadcasts"
```

### Task 6: Create propose_roadmap Python Tool

**Files:**
- Create: `workers/codeforge/tools/propose_roadmap.py`
- Test: `workers/tests/tools/test_propose_roadmap.py`

- [ ] **Step 1: Write the propose_roadmap tool**

```python
"""Agent tool for proposing roadmap milestones and atomic work steps via AG-UI events."""

from __future__ import annotations

import logging
import uuid
from typing import Any

from ._base import ToolDefinition, ToolExample, ToolResult

logger = logging.getLogger(__name__)

PROPOSE_ROADMAP_DEFINITION = ToolDefinition(
    name="propose_roadmap",
    description=(
        "Propose a roadmap milestone or atomic work step for user review. "
        "Use this after goals are established to build the implementation roadmap. "
        "Propose milestones first, then atomic steps within each milestone."
    ),
    parameters={
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["create_milestone", "create_step"],
                "description": "Whether to propose a milestone or a work step.",
            },
            "milestone_title": {
                "type": "string",
                "description": "Milestone title. Required for create_milestone, used as parent reference for create_step.",
            },
            "milestone_description": {
                "type": "string",
                "description": "Milestone description (only for create_milestone).",
            },
            "step_title": {
                "type": "string",
                "description": "Atomic work step title (only for create_step).",
            },
            "step_description": {
                "type": "string",
                "description": "What this step does. Must be atomic: one file, one function, one test.",
            },
            "sort_order": {
                "type": "integer",
                "description": "Position within milestone (0-based). Steps execute in this order.",
            },
            "complexity": {
                "type": "string",
                "enum": ["trivial", "simple", "medium", "complex"],
                "description": "Step complexity. Determines which model tier executes it.",
            },
        },
        "required": ["action", "milestone_title"],
    },
    when_to_use=(
        "Use after 2+ goals are approved to build the implementation roadmap. "
        "First propose milestones (high-level phases), then propose atomic steps "
        "within each milestone. Each step must be independently executable by a "
        "weak LLM (7B-30B params) with only local context."
    ),
    output_format="Confirmation that the roadmap item was proposed for user review.",
    common_mistakes=[
        "Proposing steps before milestones.",
        "Steps that are too large (touching multiple files or functions).",
        "Missing milestone_title when action is create_step.",
    ],
    examples=[
        ToolExample(
            description="Propose a milestone",
            tool_call_json=(
                '{"action": "create_milestone", "milestone_title": "Authentication System",'
                ' "milestone_description": "JWT-based auth with login, register, and token refresh"}'
            ),
            expected_result="Roadmap milestone proposed: Authentication System",
        ),
        ToolExample(
            description="Propose an atomic work step",
            tool_call_json=(
                '{"action": "create_step", "milestone_title": "Authentication System",'
                ' "step_title": "Create User model with password hashing",'
                ' "step_description": "Create models/user.py with User dataclass, bcrypt hash_password and verify_password methods. Write test_user.py with 3 tests.",'
                ' "sort_order": 0, "complexity": "simple"}'
            ),
            expected_result="Roadmap step proposed: Create User model with password hashing",
        ),
    ],
)

_MODEL_TIER_MAP = {"trivial": "weak", "simple": "weak", "medium": "mid", "complex": "strong"}


class ProposeRoadmapExecutor:
    """Emit a roadmap_proposal AG-UI event via the runtime trajectory stream."""

    def __init__(self, runtime: object) -> None:
        self._runtime = runtime

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        action = arguments.get("action", "")
        milestone_title = arguments.get("milestone_title", "")

        if not milestone_title:
            return ToolResult(output="", error="milestone_title is required.", success=False)

        if action == "create_milestone":
            event_data = {
                "event_type": "agent.roadmap_proposed",
                "data": {
                    "proposal_id": str(uuid.uuid4()),
                    "action": "create_milestone",
                    "milestone_title": milestone_title,
                    "milestone_description": arguments.get("milestone_description", ""),
                    "milestone_sort_order": arguments.get("sort_order", 0),
                },
            }
            await self._runtime.publish_trajectory_event(event_data)
            logger.info("roadmap milestone proposed: %s", milestone_title)
            return ToolResult(output=f"Roadmap milestone proposed: {milestone_title}")

        if action == "create_step":
            step_title = arguments.get("step_title", "")
            if not step_title:
                return ToolResult(output="", error="step_title is required for create_step.", success=False)

            complexity = arguments.get("complexity", "simple")
            event_data = {
                "event_type": "agent.roadmap_proposed",
                "data": {
                    "proposal_id": str(uuid.uuid4()),
                    "action": "create_step",
                    "milestone_title": milestone_title,
                    "step_title": step_title,
                    "step_description": arguments.get("step_description", ""),
                    "step_sort_order": arguments.get("sort_order", 0),
                    "step_complexity": complexity,
                    "step_model_tier": _MODEL_TIER_MAP.get(complexity, "mid"),
                },
            }
            await self._runtime.publish_trajectory_event(event_data)
            logger.info("roadmap step proposed: %s -> %s", milestone_title, step_title)
            return ToolResult(output=f"Roadmap step proposed: {step_title}")

        return ToolResult(output="", error=f"Unknown action: {action}. Use create_milestone or create_step.", success=False)
```

- [ ] **Step 2: Write unit test**

```python
# workers/tests/tools/test_propose_roadmap.py
import pytest
from unittest.mock import AsyncMock
from codeforge.tools.propose_roadmap import ProposeRoadmapExecutor

@pytest.mark.asyncio
async def test_propose_milestone():
    runtime = AsyncMock()
    executor = ProposeRoadmapExecutor(runtime)
    result = await executor.execute(
        {"action": "create_milestone", "milestone_title": "Auth", "milestone_description": "JWT auth"},
        "/workspace",
    )
    assert result.success is not False
    assert "Auth" in result.output
    runtime.publish_trajectory_event.assert_called_once()
    event = runtime.publish_trajectory_event.call_args[0][0]
    assert event["data"]["action"] == "create_milestone"

@pytest.mark.asyncio
async def test_propose_step_maps_complexity_to_model_tier():
    runtime = AsyncMock()
    executor = ProposeRoadmapExecutor(runtime)
    result = await executor.execute(
        {"action": "create_step", "milestone_title": "Auth", "step_title": "User model", "complexity": "simple"},
        "/workspace",
    )
    assert "User model" in result.output
    event = runtime.publish_trajectory_event.call_args[0][0]
    assert event["data"]["step_model_tier"] == "weak"

@pytest.mark.asyncio
async def test_propose_step_complex_routes_to_strong():
    runtime = AsyncMock()
    executor = ProposeRoadmapExecutor(runtime)
    await executor.execute(
        {"action": "create_step", "milestone_title": "Auth", "step_title": "Auth arch", "complexity": "complex"},
        "/workspace",
    )
    event = runtime.publish_trajectory_event.call_args[0][0]
    assert event["data"]["step_model_tier"] == "strong"

@pytest.mark.asyncio
async def test_missing_milestone_title():
    runtime = AsyncMock()
    executor = ProposeRoadmapExecutor(runtime)
    result = await executor.execute({"action": "create_milestone"}, "/workspace")
    assert result.success is False

@pytest.mark.asyncio
async def test_create_step_missing_step_title():
    runtime = AsyncMock()
    executor = ProposeRoadmapExecutor(runtime)
    result = await executor.execute(
        {"action": "create_step", "milestone_title": "Auth"},
        "/workspace",
    )
    assert result.success is False
```

- [ ] **Step 3: Run tests**

Run: `cd workers && python -m pytest tests/tools/test_propose_roadmap.py -v`
Expected: 5 PASS

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/tools/propose_roadmap.py workers/tests/tools/test_propose_roadmap.py
git commit -m "feat: add propose_roadmap tool for chat-driven roadmap generation"
```

### Task 7: Register propose_roadmap Tool

**Files:**
- Modify: `workers/codeforge/consumer/_conversation_skill_integration.py` (add registration function)
- Modify: `workers/codeforge/consumer/_conversation.py` (import and call)

- [ ] **Step 1: Add register_propose_roadmap_tool in _conversation_skill_integration.py**

After `register_propose_goal_tool` (line 99), add:

```python
def register_propose_roadmap_tool(registry: object, runtime: object) -> None:
    """Register the propose_roadmap tool for agent-driven roadmap proposals."""
    from codeforge.tools.propose_roadmap import PROPOSE_ROADMAP_DEFINITION, ProposeRoadmapExecutor

    registry.register(PROPOSE_ROADMAP_DEFINITION, ProposeRoadmapExecutor(runtime))
```

- [ ] **Step 2: Import and call in _conversation.py**

In `_conversation.py`, add to imports (line 20-22):

```python
from codeforge.consumer._conversation_skill_integration import (
    register_handoff_tool,
    register_propose_goal_tool,
    register_propose_roadmap_tool,
    wire_skill_tools,
)
```

Add call after `register_propose_goal_tool(registry, runtime)` (line 264):

```python
register_propose_roadmap_tool(registry, runtime)
```

- [ ] **Step 3: Verify import works**

Run: `cd workers && python -c "from codeforge.consumer._conversation_skill_integration import register_propose_roadmap_tool; print('OK')"`
Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/consumer/_conversation_skill_integration.py workers/codeforge/consumer/_conversation.py
git commit -m "feat: register propose_roadmap in conversation handler"
```

---

## Phase C: Frontend — AG-UI Events & Proposal Cards

**Objective:** Handle roadmap proposal AG-UI events and render inline approval cards matching the GoalProposalCard design pattern (design system tokens, Badge/Button primitives, useI18n).

### Task 8: Add AG-UI Types for Roadmap Proposals

**Files:**
- Modify: `frontend/src/api/websocket.ts`
- Modify: `frontend/src/features/project/chatPanelTypes.ts`

- [ ] **Step 1: Read current websocket.ts**

Read: `frontend/src/api/websocket.ts`

- [ ] **Step 2: Add event type in AGUIEventMap (around line 48-59)**

```typescript
"agui.roadmap_proposal": AGUIRoadmapProposal;
```

- [ ] **Step 3: Add AGUIRoadmapProposal interface (after AGUIActionSuggestion)**

```typescript
export interface AGUIRoadmapProposal {
  run_id: string;
  proposal_id: string;
  action: "create_milestone" | "create_step";
  milestone_title: string;
  milestone_description?: string;
  milestone_sort_order?: number;
  step_title?: string;
  step_description?: string;
  step_sort_order?: number;
  step_complexity?: "trivial" | "simple" | "medium" | "complex";
  step_model_tier?: "weak" | "mid" | "strong";
}
```

- [ ] **Step 4: Add RoadmapProposalState in chatPanelTypes.ts**

```typescript
export interface RoadmapProposalState {
  proposalId: string;
  action: "create_milestone" | "create_step";
  milestoneTitle: string;
  milestoneDescription?: string;
  stepTitle?: string;
  stepDescription?: string;
  stepComplexity?: string;
  stepModelTier?: string;
  status: "pending" | "approved" | "rejected";
}
```

- [ ] **Step 5: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/websocket.ts frontend/src/features/project/chatPanelTypes.ts
git commit -m "feat: add AGUIRoadmapProposal TypeScript types"
```

### Task 9: Handle Roadmap Proposal Events in useChatAGUI

**Files:**
- Modify: `frontend/src/features/project/useChatAGUI.ts`

- [ ] **Step 1: Read current useChatAGUI.ts**

Read: `frontend/src/features/project/useChatAGUI.ts`

- [ ] **Step 2: Add roadmapProposals signal**

After `goalProposals` signal declaration:

```typescript
const [roadmapProposals, setRoadmapProposals] = createSignal<RoadmapProposalState[]>([]);
```

- [ ] **Step 3: Add event handler after goal_proposal handler (around line 252)**

```typescript
onAGUIEvent("agui.roadmap_proposal", (ev) => {
  if (ev.run_id !== activeRunId()) return;
  setRoadmapProposals((prev) => [
    ...prev,
    {
      proposalId: ev.proposal_id,
      action: ev.action,
      milestoneTitle: ev.milestone_title,
      milestoneDescription: ev.milestone_description,
      stepTitle: ev.step_title,
      stepDescription: ev.step_description,
      stepComplexity: ev.step_complexity,
      stepModelTier: ev.step_model_tier,
      status: "pending",
    },
  ]);
});
```

- [ ] **Step 4: Clear roadmapProposals on run_started (add to existing handler around line 84-97)**

```typescript
setRoadmapProposals([]);
```

- [ ] **Step 5: Return roadmapProposals from the hook**

Add `roadmapProposals` to the return object.

- [ ] **Step 6: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
git add frontend/src/features/project/useChatAGUI.ts
git commit -m "feat: handle agui.roadmap_proposal events in useChatAGUI"
```

### Task 10: Create RoadmapProposalCard Component

**Files:**
- Create: `frontend/src/features/project/RoadmapProposalCard.tsx`

This follows the GoalProposalCard pattern exactly: uses `cf-*` design tokens, `Badge`/`Button` from `~/ui`, `useI18n()` for text.

- [ ] **Step 1: Write the component**

```typescript
import { createSignal, Show } from "solid-js";

import { api } from "~/api/client";
import type { AGUIRoadmapProposal } from "~/api/websocket";
import { useI18n } from "~/i18n";
import { Badge, Button } from "~/ui";
import type { BadgeVariant } from "~/ui/primitives/Badge";

interface Props {
  proposal: AGUIRoadmapProposal;
  projectId: string;
  onApprove: (title: string) => void;
  onReject: (title: string) => void;
}

const COMPLEXITY_VARIANTS: Record<string, BadgeVariant> = {
  trivial: "success",
  simple: "info",
  medium: "warning",
  complex: "danger",
};

const CONTENT_PREVIEW_LIMIT = 300;

export default function RoadmapProposalCard(props: Props) {
  const { t } = useI18n();
  const [status, setStatus] = createSignal<"pending" | "approved" | "rejected">("pending");
  const [saving, setSaving] = createSignal(false);

  const isMilestone = () => props.proposal.action === "create_milestone";
  const title = () => isMilestone() ? props.proposal.milestone_title : props.proposal.step_title ?? "";
  const description = () =>
    isMilestone() ? props.proposal.milestone_description : props.proposal.step_description;

  const descriptionPreview = (): string => {
    const raw = description() ?? "";
    if (raw.length <= CONTENT_PREVIEW_LIMIT) return raw;
    return raw.slice(0, CONTENT_PREVIEW_LIMIT) + "...";
  };

  const handleApprove = async (): Promise<void> => {
    setSaving(true);
    try {
      if (isMilestone()) {
        await api.roadmap.createMilestone(props.projectId, {
          title: props.proposal.milestone_title,
          description: props.proposal.milestone_description,
        });
      } else {
        const roadmap = await api.roadmap.get(props.projectId);
        const milestone = roadmap?.milestones?.find(
          (m: { title: string }) => m.title === props.proposal.milestone_title,
        );
        if (milestone) {
          await api.roadmap.createFeature(milestone.id, {
            title: props.proposal.step_title ?? "",
            description: props.proposal.step_description,
          });
        }
      }
      setStatus("approved");
      props.onApprove(title());
    } catch {
      setSaving(false);
    }
  };

  const handleReject = (): void => {
    setStatus("rejected");
    props.onReject(title());
  };

  const cardBorder = (): string => {
    switch (status()) {
      case "approved":
        return "border-cf-success-border bg-cf-success-bg/30";
      case "rejected":
        return "border-cf-danger-border/30 bg-cf-danger-bg/20 opacity-60";
      default:
        return "border-cf-border bg-cf-bg-secondary";
    }
  };

  return (
    <div class={`rounded-cf-sm border p-3 my-2 ${cardBorder()}`}>
      <div class="flex items-center gap-2 mb-2">
        <Badge variant={isMilestone() ? "neutral" : "info"} pill>
          {isMilestone() ? "Milestone" : "Work Step"}
        </Badge>
        <Show when={!isMilestone() && props.proposal.step_complexity}>
          <Badge variant={COMPLEXITY_VARIANTS[props.proposal.step_complexity!] ?? "neutral"} pill>
            {props.proposal.step_complexity}
          </Badge>
        </Show>
        <Show when={!isMilestone() && props.proposal.step_model_tier}>
          <span class="text-xs text-cf-text-tertiary">{props.proposal.step_model_tier} model</span>
        </Show>
      </div>

      <h4 class="text-sm font-semibold text-cf-text-primary mb-1">{title()}</h4>

      <Show when={description()}>
        <p class="text-xs text-cf-text-secondary whitespace-pre-wrap mb-3 line-clamp-4">
          {descriptionPreview()}
        </p>
      </Show>

      <Show
        when={status() === "pending"}
        fallback={
          <span
            class={`text-xs font-medium ${
              status() === "approved" ? "text-cf-success-fg" : "text-cf-danger-fg"
            }`}
          >
            {status() === "approved" ? "\u2713 Approved" : "\u2717 Rejected"}
          </span>
        }
      >
        <div class="flex items-center gap-2">
          <Button
            variant="primary"
            size="xs"
            loading={saving()}
            disabled={saving()}
            onClick={handleApprove}
          >
            {saving() ? t("common.saving") : t("common.approve")}
          </Button>
          <Button variant="secondary" size="xs" onClick={handleReject}>
            {t("common.reject")}
          </Button>
        </div>
      </Show>
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/project/RoadmapProposalCard.tsx
git commit -m "feat: add RoadmapProposalCard for inline roadmap proposal approval"
```

### Task 11: Wire Proposal Cards into ChatMessages

**Files:**
- Modify: `frontend/src/features/project/ChatMessages.tsx`
- Modify: `frontend/src/features/project/ChatPanel.tsx`

- [ ] **Step 1: Read current ChatMessages.tsx**

Read: `frontend/src/features/project/ChatMessages.tsx` — find where `GoalProposalCard` is rendered.

- [ ] **Step 2: Add RoadmapProposalCard rendering after GoalProposalCard loop**

```tsx
<For each={props.roadmapProposals}>
  {(proposal) => (
    <RoadmapProposalCard
      proposal={proposal}
      projectId={props.projectId}
      onApprove={(title) => props.sendChatMessage?.(`Approved roadmap item: ${title}`)}
      onReject={(title) => props.sendChatMessage?.(`Rejected roadmap item: ${title}`)}
    />
  )}
</For>
```

- [ ] **Step 3: Add roadmapProposals to ChatMessages props interface**

- [ ] **Step 4: Pass roadmapProposals from ChatPanel to ChatMessages**

In ChatPanel.tsx, pass `roadmapProposals={agui.roadmapProposals()}` to `<ChatMessages>`.

- [ ] **Step 5: Verify frontend builds**

Run: `cd frontend && npm run build`
Expected: Build succeeds

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/project/ChatMessages.tsx frontend/src/features/project/ChatPanel.tsx
git commit -m "feat: render RoadmapProposalCard inline in chat messages"
```

---

## Phase D: Panel -> Chat Deep-Linking

**Objective:** Every goal, roadmap step, and feature gets a "Discuss in Chat" button that pre-fills the chat input with item context.

### Task 12: Create PanelChatLink Component

**Files:**
- Create: `frontend/src/features/project/PanelChatLink.tsx`

- [ ] **Step 1: Write the component**

```typescript
import type { Component } from "solid-js";

import { Button } from "~/ui";

interface Props {
  type: "goal" | "roadmap-step" | "feature" | "milestone";
  id: string;
  title: string;
  context?: string;
  onDiscuss: (message: string) => void;
}

const PanelChatLink: Component<Props> = (props) => {
  const handleClick = () => {
    const ref = `[${props.type}:${props.id}]`;
    const contextLine = props.context ? `\n\nContext: ${props.context}` : "";
    props.onDiscuss(`Let's discuss ${props.type} "${props.title}" ${ref}${contextLine}`);
  };

  return (
    <Button variant="ghost" size="xs" onClick={handleClick} title={`Discuss "${props.title}" in chat`}>
      Discuss
    </Button>
  );
};

export default PanelChatLink;
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/features/project/PanelChatLink.tsx
git commit -m "feat: add PanelChatLink component for panel-to-chat deep-linking"
```

### Task 13: Add Deep-Links to GoalsPanel

**Files:**
- Modify: `frontend/src/features/project/GoalsPanel.tsx`

- [ ] **Step 1: Read current GoalsPanel.tsx**

Read: `frontend/src/features/project/GoalsPanel.tsx`

- [ ] **Step 2: Add PanelChatLink to each goal item**

In the goal list rendering, add a `<PanelChatLink>` next to each goal's delete/toggle actions:

```tsx
<PanelChatLink
  type="goal"
  id={g.id}
  title={g.title}
  context={g.content?.substring(0, 200)}
  onDiscuss={(msg) => props.onSendChatMessage?.(msg)}
/>
```

- [ ] **Step 3: Add onSendChatMessage to GoalsPanel props interface**

```typescript
interface Props {
  projectId: string;
  onAIDiscoverStarted?: (conversationId: string) => void;
  onNavigate?: (target: string) => void;
  onSendChatMessage?: (msg: string) => void; // NEW
}
```

- [ ] **Step 4: Add hint text de-emphasizing manual creation**

Above the manual goal form, add:

```tsx
<p class="text-xs text-cf-text-tertiary mb-2">
  Goals are extracted automatically from your chat conversation.
</p>
```

- [ ] **Step 5: Verify frontend builds**

Run: `cd frontend && npm run build`
Expected: Build succeeds

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/project/GoalsPanel.tsx
git commit -m "feat: add Discuss deep-links to GoalsPanel, de-emphasize manual creation"
```

### Task 14: Add Deep-Links to RoadmapPanel

**Files:**
- Modify: `frontend/src/features/project/RoadmapPanel.tsx`

- [ ] **Step 1: Read current RoadmapPanel.tsx**

Read: `frontend/src/features/project/RoadmapPanel.tsx`

- [ ] **Step 2: Add PanelChatLink to milestones and features**

Add `<PanelChatLink type="milestone" ...>` next to each milestone title.
Add `<PanelChatLink type="roadmap-step" ...>` next to each feature/step.

- [ ] **Step 3: Add onSendChatMessage prop**

Same pattern as GoalsPanel.

- [ ] **Step 4: Add hint text**

```tsx
<p class="text-xs text-cf-text-tertiary mb-2">
  Roadmap is generated from your goals via chat. Drag to reorder, click Discuss to refine.
</p>
```

- [ ] **Step 5: Verify frontend builds**

Run: `cd frontend && npm run build`
Expected: Build succeeds

- [ ] **Step 6: Commit**

```bash
git add frontend/src/features/project/RoadmapPanel.tsx
git commit -m "feat: add Discuss deep-links to RoadmapPanel"
```

### Task 15: Add Deep-Links to FeatureMapPanel

**Files:**
- Modify: `frontend/src/features/project/FeatureMapPanel.tsx`

- [ ] **Step 1: Read current FeatureMapPanel.tsx**

Read: `frontend/src/features/project/FeatureMapPanel.tsx`

- [ ] **Step 2: Add PanelChatLink to each FeatureCard**

Pass `onSendChatMessage` through to `FeatureCard` component, add `<PanelChatLink type="feature">`.

- [ ] **Step 3: Verify frontend builds**

Run: `cd frontend && npm run build`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add frontend/src/features/project/FeatureMapPanel.tsx
git commit -m "feat: add Discuss deep-links to FeatureMapPanel"
```

### Task 16: Wire Deep-Link Callback Through ProjectDetailPage

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx`
- Modify: `frontend/src/features/project/ChatPanel.tsx`

- [ ] **Step 1: Read ProjectDetailPage panel rendering**

Read: `frontend/src/features/project/ProjectDetailPage.tsx` — find where GoalsPanel, RoadmapPanel, FeatureMapPanel are rendered.

- [ ] **Step 2: Add sendChatMessage signal and callback**

```typescript
const [prefillMessage, setPrefillMessage] = createSignal("");

const handleSendChatMessage = (msg: string) => {
  setPrefillMessage(msg);
  // On mobile, switch to chat tab
  if (isMobile()) setMobileTab("chat");
};
```

- [ ] **Step 3: Pass callback to all panel components**

```tsx
<GoalsPanel projectId={...} onSendChatMessage={handleSendChatMessage} ... />
<RoadmapPanel projectId={...} onSendChatMessage={handleSendChatMessage} ... />
<FeatureMapPanel projectId={...} onSendChatMessage={handleSendChatMessage} ... />
```

- [ ] **Step 4: Pass prefillMessage to ChatPanel**

```tsx
<ChatPanel projectId={...} prefillMessage={prefillMessage()} ... />
```

- [ ] **Step 5: Handle prefillMessage in ChatPanel**

In ChatPanel, add an effect that populates the input when prefillMessage changes:

```typescript
createEffect(() => {
  const msg = props.prefillMessage;
  if (msg) {
    setInput(msg);
    inputRef?.focus();
  }
});
```

- [ ] **Step 6: Verify frontend builds**

Run: `cd frontend && npm run build`
Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
git add frontend/src/features/project/ProjectDetailPage.tsx frontend/src/features/project/ChatPanel.tsx
git commit -m "feat: wire deep-link messages from panels to chat input"
```

---

## Phase E: Panel Consolidation

**Objective:** Reduce 14+ panels into 4 focused views: Plan, Execute, Code, Govern.

### Task 17: Create ConsolidatedPlanView

**Files:**
- Create: `frontend/src/features/project/ConsolidatedPlanView.tsx`

- [ ] **Step 1: Write the combined Plan view**

Combines GoalsPanel + RoadmapPanel + FeatureMapPanel as collapsible sections. Each section renders the existing panel component with its full functionality — no `compact` prop needed.

```typescript
import { type Component, createSignal, Show } from "solid-js";

import GoalsPanel from "./GoalsPanel";
import RoadmapPanel from "./RoadmapPanel";
import FeatureMapPanel from "./FeatureMapPanel";

interface Props {
  projectId: string;
  onSendChatMessage?: (msg: string) => void;
  onAIDiscoverStarted?: (conversationId: string) => void;
  onNavigate?: (target: string) => void;
  onError?: (err: Error) => void;
}

const ConsolidatedPlanView: Component<Props> = (props) => {
  const [goalsOpen, setGoalsOpen] = createSignal(true);
  const [roadmapOpen, setRoadmapOpen] = createSignal(true);
  const [featuremapOpen, setFeaturemapOpen] = createSignal(false);

  const SectionHeader: Component<{ label: string; open: boolean; onToggle: () => void }> = (s) => (
    <button
      class="w-full flex items-center justify-between px-3 py-2 text-sm font-medium text-cf-text-primary hover:bg-cf-bg-tertiary border-b border-cf-border"
      onClick={s.onToggle}
    >
      <span>{s.label}</span>
      <span class="text-xs text-cf-text-tertiary">{s.open ? "\u25B4" : "\u25BE"}</span>
    </button>
  );

  return (
    <div class="flex flex-col h-full overflow-y-auto">
      <section>
        <SectionHeader label="Goals" open={goalsOpen()} onToggle={() => setGoalsOpen(!goalsOpen())} />
        <Show when={goalsOpen()}>
          <div class="p-2">
            <GoalsPanel
              projectId={props.projectId}
              onAIDiscoverStarted={props.onAIDiscoverStarted}
              onNavigate={props.onNavigate}
              onSendChatMessage={props.onSendChatMessage}
            />
          </div>
        </Show>
      </section>

      <section>
        <SectionHeader label="Roadmap" open={roadmapOpen()} onToggle={() => setRoadmapOpen(!roadmapOpen())} />
        <Show when={roadmapOpen()}>
          <div class="p-2">
            <RoadmapPanel
              projectId={props.projectId}
              onError={props.onError ?? (() => {})}
              onNavigate={props.onNavigate}
              onSendChatMessage={props.onSendChatMessage}
            />
          </div>
        </Show>
      </section>

      <section>
        <SectionHeader label="Feature Map" open={featuremapOpen()} onToggle={() => setFeaturemapOpen(!featuremapOpen())} />
        <Show when={featuremapOpen()}>
          <div class="p-2">
            <FeatureMapPanel projectId={props.projectId} />
          </div>
        </Show>
      </section>
    </div>
  );
};

export default ConsolidatedPlanView;
```

- [ ] **Step 2: Verify TypeScript compilation**

Run: `cd frontend && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/features/project/ConsolidatedPlanView.tsx
git commit -m "feat: add ConsolidatedPlanView combining Goals + Roadmap + FeatureMap"
```

### Task 18: Simplify Panel Groups in ProjectDetailPage

**Files:**
- Modify: `frontend/src/features/project/ProjectDetailPage.tsx:62-109` (PANEL_GROUPS)

- [ ] **Step 1: Read current PANEL_GROUPS**

Read: `frontend/src/features/project/ProjectDetailPage.tsx` lines 62-109.

- [ ] **Step 2: Replace with consolidated groups**

```typescript
const PANEL_GROUPS = [
  {
    label: "Workflow",
    items: [
      { id: "plan", label: "Plan", description: "Goals, Roadmap & Features" },
      { id: "execute", label: "Execute", description: "War Room, Runs & Trajectory" },
    ],
  },
  {
    label: "Tools",
    items: [
      { id: "code", label: "Code", description: "Files, RepoMap & Search" },
      { id: "govern", label: "Govern", description: "Policy & Audit" },
    ],
  },
];
```

- [ ] **Step 3: Update panel rendering switch**

Map consolidated IDs to components:
- `"plan"` -> `<ConsolidatedPlanView>` (Goals + Roadmap + FeatureMap)
- `"execute"` -> existing WarRoom (or wrap WarRoom + RunPanel + TrajectoryPanel with tabs)
- `"code"` -> existing FilePanel (default) with tab access to RepoMap, Retrieval, Boundaries
- `"govern"` -> existing PolicyPanel with tab access to AuditPanel

**Important:** Keep individual panel IDs working for backwards-compatible deep navigation (e.g., `onNavigate("goals")` still switches to `"plan"` view and scrolls to Goals section).

- [ ] **Step 4: Verify frontend builds**

Run: `cd frontend && npm run build`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/project/ProjectDetailPage.tsx
git commit -m "feat: consolidate 14 panels into 4 views (Plan/Execute/Code/Govern)"
```

---

## Phase F: Sub-Agent Spawning (Foundation)

**Objective:** Enable the orchestrator to spawn sub-agents for research, debate, and implementation.

### Task 19: Create spawn_subagent Python Tool

**Files:**
- Create: `workers/codeforge/tools/spawn_subagent.py`
- Test: `workers/tests/tools/test_spawn_subagent.py`

- [ ] **Step 1: Write the tool**

```python
"""Agent tool for spawning sub-agents to handle research, debate, or implementation."""

from __future__ import annotations

import logging
import uuid
from typing import Any

from ._base import ToolDefinition, ToolExample, ToolResult

logger = logging.getLogger(__name__)

SPAWN_SUBAGENT_DEFINITION = ToolDefinition(
    name="spawn_subagent",
    description=(
        "Spawn a sub-agent to handle a specific task: research, debate, or implementation. "
        "The sub-agent runs as a separate conversation with its own context and model. "
        "Results are reported back to the orchestrator conversation."
    ),
    parameters={
        "type": "object",
        "properties": {
            "role": {
                "type": "string",
                "enum": ["researcher", "implementer", "reviewer", "debater"],
                "description": "Sub-agent role determining its system prompt and tools.",
            },
            "task": {
                "type": "string",
                "description": "Specific task for the sub-agent to accomplish.",
            },
            "context": {
                "type": "string",
                "description": "Additional context from the orchestrator conversation.",
            },
            "model_tier": {
                "type": "string",
                "enum": ["weak", "mid", "strong"],
                "description": "Model tier for this sub-agent. Defaults to mid.",
            },
        },
        "required": ["role", "task"],
    },
    when_to_use=(
        "Use when a task requires specialized focus: research needs search, "
        "implementation needs file editing, review needs code analysis, "
        "debate needs two agents arguing pros/cons."
    ),
    output_format="Sub-agent spawn confirmation with ID for tracking.",
    common_mistakes=[
        "Spawning a sub-agent for trivial tasks the orchestrator can handle directly.",
        "Not providing enough context for the sub-agent to work independently.",
        "Spawning too many sub-agents simultaneously (max 3 concurrent recommended).",
    ],
    examples=[
        ToolExample(
            description="Spawn a researcher",
            tool_call_json=(
                '{"role": "researcher", "task": "Research JWT vs session auth for Go backend. '
                'Compare security, performance, complexity. Report findings.", "model_tier": "mid"}'
            ),
            expected_result="Sub-agent spawned: researcher-abc123. Monitoring for results.",
        ),
    ],
)


class SpawnSubagentExecutor:
    """Request sub-agent spawning via trajectory event."""

    def __init__(self, runtime: object) -> None:
        self._runtime = runtime

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        role = arguments.get("role", "")
        task = arguments.get("task", "")

        if not role or not task:
            return ToolResult(output="", error="role and task are required.", success=False)

        subagent_id = str(uuid.uuid4())[:8]

        event_data = {
            "event_type": "agent.subagent_requested",
            "data": {
                "subagent_id": subagent_id,
                "role": role,
                "task": task,
                "context": arguments.get("context", ""),
                "model_tier": arguments.get("model_tier", "mid"),
            },
        }

        await self._runtime.publish_trajectory_event(event_data)

        logger.info("sub-agent requested: %s (%s)", role, subagent_id)
        return ToolResult(output=f"Sub-agent spawned: {role}-{subagent_id}. Monitoring for results.")
```

- [ ] **Step 2: Write unit tests**

```python
# workers/tests/tools/test_spawn_subagent.py
import pytest
from unittest.mock import AsyncMock
from codeforge.tools.spawn_subagent import SpawnSubagentExecutor

@pytest.mark.asyncio
async def test_spawn_researcher():
    runtime = AsyncMock()
    executor = SpawnSubagentExecutor(runtime)
    result = await executor.execute(
        {"role": "researcher", "task": "Investigate auth patterns"},
        "/workspace",
    )
    assert "researcher" in result.output
    runtime.publish_trajectory_event.assert_called_once()
    event = runtime.publish_trajectory_event.call_args[0][0]
    assert event["data"]["role"] == "researcher"
    assert event["data"]["model_tier"] == "mid"

@pytest.mark.asyncio
async def test_spawn_with_explicit_tier():
    runtime = AsyncMock()
    executor = SpawnSubagentExecutor(runtime)
    await executor.execute(
        {"role": "implementer", "task": "Build user model", "model_tier": "weak"},
        "/workspace",
    )
    event = runtime.publish_trajectory_event.call_args[0][0]
    assert event["data"]["model_tier"] == "weak"

@pytest.mark.asyncio
async def test_missing_role():
    runtime = AsyncMock()
    executor = SpawnSubagentExecutor(runtime)
    result = await executor.execute({"task": "do something"}, "/workspace")
    assert result.success is False

@pytest.mark.asyncio
async def test_missing_task():
    runtime = AsyncMock()
    executor = SpawnSubagentExecutor(runtime)
    result = await executor.execute({"role": "researcher"}, "/workspace")
    assert result.success is False
```

- [ ] **Step 3: Run tests**

Run: `cd workers && python -m pytest tests/tools/test_spawn_subagent.py -v`
Expected: 4 PASS

- [ ] **Step 4: Commit**

```bash
git add workers/codeforge/tools/spawn_subagent.py workers/tests/tools/test_spawn_subagent.py
git commit -m "feat: add spawn_subagent tool for orchestrator to delegate to sub-agents"
```

### Task 20: Register spawn_subagent and Handle in Backend

**Files:**
- Modify: `workers/codeforge/consumer/_conversation_skill_integration.py`
- Modify: `workers/codeforge/consumer/_conversation.py`
- Modify: `internal/service/runtime.go` (trajectory event handler)

- [ ] **Step 1: Add register_spawn_subagent_tool**

In `_conversation_skill_integration.py`, add:

```python
def register_spawn_subagent_tool(registry: object, runtime: object) -> None:
    """Register the spawn_subagent tool for sub-agent delegation."""
    from codeforge.tools.spawn_subagent import SPAWN_SUBAGENT_DEFINITION, SpawnSubagentExecutor

    registry.register(SPAWN_SUBAGENT_DEFINITION, SpawnSubagentExecutor(runtime))
```

- [ ] **Step 2: Import and call in _conversation.py**

Add `register_spawn_subagent_tool` to imports and call it after `register_propose_roadmap_tool`.

- [ ] **Step 3: Add Go handler for agent.subagent_requested in runtime.go**

After the `agent.roadmap_proposed` block, add handler that:
1. Creates a new conversation via `ConversationService`
2. Sends the task as first user message with sub-agent system prompt
3. Starts agentic run with the specified model tier
4. Broadcasts a `text_message` to parent: "Sub-agent {role} started: {task}"

```go
if payload.EventType == "agent.subagent_requested" {
	var req struct {
		Data struct {
			SubagentID string `json:"subagent_id"`
			Role       string `json:"role"`
			Task       string `json:"task"`
			Context    string `json:"context"`
			ModelTier  string `json:"model_tier"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &req); err == nil && s.convSvc != nil {
		go s.spawnSubagent(msgCtx, payload.ProjectID, payload.RunID, req.Data)
	}
}
```

- [ ] **Step 4: Implement spawnSubagent method**

```go
func (s *RuntimeService) spawnSubagent(ctx context.Context, projectID, parentRunID string, data subagentRequest) {
	conv, err := s.convSvc.Create(ctx, projectID, nil)
	if err != nil {
		slog.Warn("subagent: create conversation failed", "error", err)
		return
	}

	modeID := data.Role // Maps to existing modes: researcher, implementer, reviewer
	content := fmt.Sprintf("[Sub-agent task from orchestrator]\n\n%s", data.Task)
	if data.Context != "" {
		content += fmt.Sprintf("\n\nContext:\n%s", data.Context)
	}

	if err := s.convSvc.SendMessageAgenticWithMode(ctx, conv.ID, content, modeID); err != nil {
		slog.Warn("subagent: send message failed", "error", err, "conversation_id", conv.ID)
	}
}
```

- [ ] **Step 5: Write test**

```go
func TestTrajectoryEvent_SubagentRequested_CreatesConversation(t *testing.T) {
	// Setup mock convSvc
	// Send agent.subagent_requested
	// Assert convSvc.Create called, SendMessageAgenticWithMode called
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/service/ -run TestTrajectoryEvent_SubagentRequested -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add workers/codeforge/consumer/_conversation_skill_integration.py workers/codeforge/consumer/_conversation.py internal/service/runtime.go internal/service/runtime_test.go
git commit -m "feat: wire spawn_subagent tool and handle sub-agent requests in Go backend"
```

---

## Phase G: Documentation

### Task 21: Update Documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/todo.md`
- Create: `docs/features/07-chat-first-orchestrator.md`
- Create: `docs/testing/2026-03-25-chat-first-orchestrator-testplan.md`

- [ ] **Step 1: Write feature documentation**

Document: chat-first orchestrator flow, orchestrator persona, new AG-UI events, new tools (propose_roadmap, spawn_subagent), deep-link mechanism, panel consolidation.

- [ ] **Step 2: Update CLAUDE.md**

Add to "Agentic Conversation Loop" section:
- Chat-first orchestrator behavior prompt (`behavior/chat_first_orchestration.yaml`)
- New tools: `propose_roadmap`, `spawn_subagent`
- New AG-UI event: `roadmap_proposal`
- Panel consolidation: 14 -> 4 views (Plan/Execute/Code/Govern)
- Deep-link: `PanelChatLink` component, `[type:ID]` reference format

- [ ] **Step 3: Write testplan**

Test scenarios (Playwright-MCP browser):
1. Auto Goal Extraction: type project description -> GoalProposalCards appear without "AI Discover"
2. Goal Approval: approve 2+ goals -> appear in GoalsPanel -> orchestrator suggests roadmap
3. Roadmap Generation: confirm -> RoadmapProposalCards with milestones + atomic steps -> approve -> RoadmapPanel updates
4. Deep-Link: click "Discuss" on goal -> chat input fills with `[goal:ID]` reference
5. Panel Consolidation: verify 4 panel groups in dropdown
6. Sub-Agent: verify spawn creates visible activity in War Room

- [ ] **Step 4: Update docs/todo.md**

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md docs/todo.md docs/features/07-chat-first-orchestrator.md docs/testing/2026-03-25-chat-first-orchestrator-testplan.md
git commit -m "docs: add chat-first orchestrator documentation and testplan"
```

---

## Execution Order & Dependencies

```
Phase A (Tasks 1-3)    Orchestrator persona + proactive goal extraction
  |
Phase B (Tasks 4-7)    Roadmap proposal tool + AG-UI event + Go routing
  |                    (depends on A for orchestrator prompt to reference propose_roadmap)
  |
Phase F (Tasks 19-20)  Sub-agent spawning
  |                    (depends on A for orchestrator prompt; can run parallel with B)
  |
Phase C (Tasks 8-11)   Frontend: AG-UI handling + proposal cards
  |                    (depends on B for event types + Go routing to be complete)
  |
Phase D (Tasks 12-16)  Deep-linking: panels -> chat
  |                    (independent of B/C; can start after A)
  |
Phase E (Tasks 17-18)  Panel consolidation
  |                    (depends on D for deep-link props being wired)
  |
Phase G (Task 21)      Documentation
                       (depends on all above)
```

**Parallelizable:**
- B + F can run in parallel (both depend only on A)
- D can start in parallel with B/C/F (independent)

**Estimated commits:** 21 tasks, ~21 atomic commits.

**Deferred to follow-up plan:**
- `propose_plan` tool + `PlanProposalCard` (execution plan proposals in chat)
- `ConsolidatedExecutionView` (combined WarRoom + Runs + Trajectory)
- Frontend component tests (Vitest/Testing Library)
