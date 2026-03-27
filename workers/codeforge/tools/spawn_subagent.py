"""Agent tool for spawning sub-agents to handle research, debate, and implementation."""

from __future__ import annotations

import logging
import uuid
from typing import Any

from ._base import ToolDefinition, ToolExample, ToolResult

logger = logging.getLogger(__name__)

_VALID_ROLES = {"researcher", "implementer", "reviewer", "debater"}
_VALID_MODEL_TIERS = {"weak", "mid", "strong"}

SPAWN_SUBAGENT_DEFINITION = ToolDefinition(
    name="spawn_subagent",
    description=(
        "Spawn a sub-agent to handle a delegated task. The sub-agent runs "
        "independently with its own context and reports results back. Use this "
        "to parallelize research, delegate implementation, or run structured debates."
    ),
    parameters={
        "type": "object",
        "properties": {
            "role": {
                "type": "string",
                "enum": ["researcher", "implementer", "reviewer", "debater"],
                "description": "The role of the sub-agent.",
            },
            "task": {
                "type": "string",
                "description": "A clear description of what the sub-agent should accomplish.",
            },
            "context": {
                "type": "string",
                "description": "Optional background context to pass to the sub-agent.",
            },
            "model_tier": {
                "type": "string",
                "enum": ["weak", "mid", "strong"],
                "description": "LLM tier for the sub-agent (default: mid).",
            },
        },
        "required": ["role", "task"],
    },
    when_to_use=(
        "Use this tool when a task benefits from delegation: research that can "
        "run in parallel, implementation of an isolated component, code review "
        "by a separate perspective, or a structured debate between viewpoints."
    ),
    output_format="Confirmation that the sub-agent was spawned, including its ID and role.",
    common_mistakes=[
        "Spawning a sub-agent for trivial tasks that are faster to do directly.",
        "Not providing enough context for the sub-agent to work independently.",
        "Using 'strong' tier for simple research that 'weak' can handle.",
    ],
    examples=[
        ToolExample(
            description="Spawn a researcher to investigate authentication patterns",
            tool_call_json=(
                '{"role": "researcher", "task": "Investigate OAuth2 vs session-based auth '
                'for our SPA", "context": "We use SolidJS frontend with Go backend"}'
            ),
            expected_result="Sub-agent spawned: researcher abc12345 — Investigate OAuth2 vs session-based auth for our SPA",
        ),
        ToolExample(
            description="Spawn a debater to argue for a specific approach",
            tool_call_json=(
                '{"role": "debater", "task": "Argue for using PostgreSQL over MongoDB '
                'for our event store", "model_tier": "strong"}'
            ),
            expected_result="Sub-agent spawned: debater def67890 — Argue for using PostgreSQL over MongoDB for our event store",
        ),
    ],
)


class SpawnSubagentExecutor:
    """Emit a subagent_requested trajectory event via the runtime."""

    def __init__(self, runtime: object) -> None:
        self._runtime = runtime

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        role = arguments.get("role", "")
        if not role or role not in _VALID_ROLES:
            return ToolResult(
                output="",
                error=f"Invalid role: {role!r}. Must be one of: {', '.join(sorted(_VALID_ROLES))}.",
                success=False,
            )

        task = arguments.get("task", "")
        if not task:
            return ToolResult(
                output="",
                error="task is required and must not be empty.",
                success=False,
            )

        model_tier = arguments.get("model_tier", "mid")
        if model_tier not in _VALID_MODEL_TIERS:
            return ToolResult(
                output="",
                error=f"Invalid model_tier: {model_tier!r}. Must be one of: {', '.join(sorted(_VALID_MODEL_TIERS))}.",
                success=False,
            )

        context = arguments.get("context", "")
        subagent_id = str(uuid.uuid4())[:8]

        event_data = {
            "event_type": "agent.subagent_requested",
            "data": {
                "subagent_id": subagent_id,
                "role": role,
                "task": task,
                "context": context,
                "model_tier": model_tier,
            },
        }

        await self._runtime.publish_trajectory_event(event_data)

        logger.info("sub-agent spawned: %s %s — %s", role, subagent_id, task)
        return ToolResult(
            output=f"Sub-agent spawned: {role} {subagent_id} — {task}"
        )
