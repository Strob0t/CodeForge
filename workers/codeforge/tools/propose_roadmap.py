"""Agent tool for proposing roadmap milestones and atomic work steps via AG-UI events."""

from __future__ import annotations

import logging
import uuid
from typing import Any

from ._base import ToolDefinition, ToolExample, ToolResult

logger = logging.getLogger(__name__)

_COMPLEXITY_TO_MODEL_TIER: dict[str, str] = {
    "trivial": "weak",
    "simple": "weak",
    "medium": "mid",
    "complex": "strong",
}

PROPOSE_ROADMAP_DEFINITION = ToolDefinition(
    name="propose_roadmap",
    description=(
        "Propose a roadmap milestone or an atomic work step within a milestone. "
        "Use this PROACTIVELY after goals are approved to break down the project "
        "into milestones and concrete steps. Each step gets a model tier based on complexity."
    ),
    parameters={
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["create_milestone", "create_step"],
                "description": "Whether to create a milestone or a step within a milestone.",
            },
            "milestone_title": {
                "type": "string",
                "description": "Title of the milestone (required for both actions).",
            },
            "milestone_description": {
                "type": "string",
                "description": "Description of the milestone (used with create_milestone).",
            },
            "step_title": {
                "type": "string",
                "description": "Title of the work step (required for create_step).",
            },
            "step_description": {
                "type": "string",
                "description": "Detailed description of the work step.",
            },
            "complexity": {
                "type": "string",
                "enum": ["trivial", "simple", "medium", "complex"],
                "description": "Step complexity. Maps to model tier: trivial/simple->weak, medium->mid, complex->strong.",
            },
        },
        "required": ["action", "milestone_title"],
    },
    when_to_use=(
        "Use this tool after goals are approved to decompose the project into "
        "milestones and atomic work steps. Create milestones first, then add steps "
        "to each milestone. The user will review and approve the roadmap."
    ),
    output_format="Confirmation that the milestone or step was proposed for user review.",
    common_mistakes=[
        "Creating steps before creating the parent milestone.",
        "Not specifying complexity for steps (defaults to medium).",
        "Proposing steps that are too large to be atomic.",
    ],
    examples=[
        ToolExample(
            description="Propose a milestone for authentication",
            tool_call_json=(
                '{"action": "create_milestone", "milestone_title": "Authentication",'
                ' "milestone_description": "JWT-based auth with login, signup, and token refresh"}'
            ),
            expected_result="Milestone proposed for review: Authentication",
        ),
        ToolExample(
            description="Propose a simple step within a milestone",
            tool_call_json=(
                '{"action": "create_step", "milestone_title": "Authentication",'
                ' "step_title": "User model", "step_description": "Create User SQLAlchemy model",'
                ' "complexity": "simple"}'
            ),
            expected_result="Step proposed for review: User model (tier: weak)",
        ),
    ],
)

_VALID_ACTIONS = {"create_milestone", "create_step"}


class ProposeRoadmapExecutor:
    """Emit a roadmap_proposed AG-UI event via the runtime trajectory stream."""

    def __init__(self, runtime: object) -> None:
        self._runtime = runtime

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        action = arguments.get("action", "")
        if action not in _VALID_ACTIONS:
            return ToolResult(
                output="",
                error=f"Unknown action: {action}. Use create_milestone or create_step.",
                success=False,
            )

        milestone_title = arguments.get("milestone_title", "")
        if not milestone_title:
            return ToolResult(output="", error="milestone_title is required.", success=False)

        if action == "create_step":
            step_title = arguments.get("step_title", "")
            if not step_title:
                return ToolResult(output="", error="create_step requires step_title.", success=False)

        proposal_id = str(uuid.uuid4())
        milestone_description = arguments.get("milestone_description", "")

        event_data: dict[str, Any] = {
            "event_type": "agent.roadmap_proposed",
            "data": {
                "proposal_id": proposal_id,
                "action": action,
                "milestone_title": milestone_title,
                "milestone_description": milestone_description,
            },
        }

        if action == "create_step":
            step_title = arguments.get("step_title", "")
            step_description = arguments.get("step_description", "")
            complexity = arguments.get("complexity", "medium")
            model_tier = _COMPLEXITY_TO_MODEL_TIER.get(complexity, "mid")

            event_data["data"]["step_title"] = step_title
            event_data["data"]["step_description"] = step_description
            event_data["data"]["complexity"] = complexity
            event_data["data"]["step_model_tier"] = model_tier

        await self._runtime.publish_trajectory_event(event_data)

        if action == "create_milestone":
            logger.info("milestone proposed: %s", milestone_title)
            return ToolResult(output=f"Milestone proposed for review: {milestone_title}")

        step_title = arguments.get("step_title", "")
        model_tier = event_data["data"]["step_model_tier"]
        logger.info("step proposed: %s (tier: %s)", step_title, model_tier)
        return ToolResult(output=f"Step proposed for review: {step_title} (tier: {model_tier})")
