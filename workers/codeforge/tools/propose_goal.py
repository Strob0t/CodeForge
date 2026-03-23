"""Agent tool for proposing project goals via AG-UI events."""

from __future__ import annotations

import logging
import uuid
from typing import Any

from ._base import ToolDefinition, ToolExample, ToolResult

logger = logging.getLogger(__name__)

PROPOSE_GOAL_DEFINITION = ToolDefinition(
    name="propose_goal",
    description=(
        "Propose a project goal for user review. The goal is NOT created "
        "until the user approves it in the UI. Use this after understanding "
        "the project through exploration and interview."
    ),
    parameters={
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["create", "update", "delete"],
                "description": "The proposal action.",
            },
            "kind": {
                "type": "string",
                "enum": ["vision", "requirement", "constraint", "state", "context"],
                "description": "Goal category.",
            },
            "title": {
                "type": "string",
                "description": "Goal title.",
            },
            "content": {
                "type": "string",
                "description": "Goal content in markdown.",
            },
            "priority": {
                "type": "integer",
                "description": "Priority 0-100, higher = more important (default 90).",
            },
            "goal_id": {
                "type": "string",
                "description": "Existing goal ID (required for update and delete).",
            },
        },
        "required": ["action", "kind", "title", "content"],
    },
    when_to_use=(
        "Use this tool to propose a project goal after exploring the codebase "
        "and interviewing the user. The user must approve before the goal is saved."
    ),
    output_format="Confirmation that the goal was proposed for user review.",
    common_mistakes=[
        "Proposing goals before understanding the project (skip Phase 1 and 2).",
        "Proposing all goals at once instead of one at a time.",
        "Missing goal_id when action is update or delete.",
    ],
    examples=[
        ToolExample(
            description="Propose a backend development goal",
            tool_call_json=(
                '{"action": "create", "kind": "requirement", "title": "Python FastAPI Backend",'
                ' "content": "Create REST API with weather data endpoints, caching, and CORS",'
                ' "priority": 1}'
            ),
            expected_result="Goal proposed for review: Python FastAPI Backend",
        ),
        ToolExample(
            description="Propose a testing goal",
            tool_call_json=(
                '{"action": "create", "kind": "requirement", "title": "Test Coverage",'
                ' "content": "Write pytest tests for backend and vitest tests for frontend",'
                ' "priority": 2}'
            ),
            expected_result="Goal proposed for review: Test Coverage",
        ),
    ],
)

_VALID_ACTIONS = {"create", "update", "delete"}


class ProposeGoalExecutor:
    """Emit a goal_proposal AG-UI event via the runtime trajectory stream."""

    def __init__(self, runtime: object) -> None:
        self._runtime = runtime

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        action = arguments.get("action", "")
        if action not in _VALID_ACTIONS:
            return ToolResult(
                output="", error=f"Unknown action: {action}. Use create, update, or delete.", success=False
            )

        title = arguments.get("title", "")
        content = arguments.get("content", "")

        if action == "create" and (not title or not content):
            return ToolResult(output="", error="create requires title and content.", success=False)

        if action in ("update", "delete") and not arguments.get("goal_id"):
            return ToolResult(output="", error=f"{action} requires goal_id.", success=False)

        proposal_id = str(uuid.uuid4())
        priority = arguments.get("priority", 90)

        event_data = {
            "event_type": "agent.goal_proposed",
            "data": {
                "proposal_id": proposal_id,
                "action": action,
                "kind": arguments.get("kind", ""),
                "title": title,
                "content": content,
                "priority": priority,
                "goal_id": arguments.get("goal_id"),
            },
        }

        await self._runtime.publish_trajectory_event(event_data)

        logger.info("goal proposed: %s (%s)", title, action)
        return ToolResult(output=f"Goal proposed for review: {title}")
