"""Built-in tool: handoff_to — allows agents to initiate handoffs to other agents."""

from __future__ import annotations

from typing import Any

import structlog

logger = structlog.get_logger()


HANDOFF_TOOL_DEF = {
    "type": "function",
    "function": {
        "name": "handoff_to",
        "description": "Hand off the current task to a specialist agent with context and artifacts.",
        "parameters": {
            "type": "object",
            "properties": {
                "target_agent_id": {
                    "type": "string",
                    "description": "ID of the target agent to hand off to.",
                },
                "target_mode": {
                    "type": "string",
                    "description": "Mode ID for the target agent (e.g., 'coder', 'reviewer').",
                },
                "context": {
                    "type": "string",
                    "description": "Context message for the target agent explaining what to do.",
                },
                "artifacts": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "List of artifact paths or IDs to pass to the target.",
                },
            },
            "required": ["target_agent_id", "context"],
        },
    },
}


async def execute_handoff(
    run_id: str,
    arguments: dict[str, Any],
    nats_publish: Any,
) -> str:
    """Execute a handoff_to tool call by publishing to the handoff NATS subject."""
    import json

    target = arguments.get("target_agent_id", "")
    context_msg = arguments.get("context", "")
    target_mode = arguments.get("target_mode", "")
    artifacts = arguments.get("artifacts", [])

    if not target or not context_msg:
        return "Error: target_agent_id and context are required"

    payload = {
        "source_run_id": run_id,
        "target_agent_id": target,
        "target_mode_id": target_mode,
        "context": context_msg,
        "artifacts": artifacts,
    }

    await nats_publish("handoff.request", json.dumps(payload).encode())

    logger.info(
        "handoff initiated",
        run_id=run_id,
        target=target,
        mode=target_mode,
    )

    return f"Handoff to {target} initiated successfully."
