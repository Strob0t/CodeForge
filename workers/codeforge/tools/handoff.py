"""Built-in tool: handoff_to -- allows agents to initiate handoffs to other agents."""

from __future__ import annotations

import json
import uuid
from typing import Any

import structlog

from codeforge.consumer._subjects import SUBJECT_HANDOFF_REQUEST

logger = structlog.get_logger()

MAX_HANDOFF_HOPS = 10


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
                "plan_id": {
                    "type": "string",
                    "description": "Associated plan ID for workflow tracking.",
                },
                "step_id": {
                    "type": "string",
                    "description": "Associated step ID within the plan.",
                },
                "metadata": {
                    "type": "object",
                    "description": "Additional key-value metadata to pass along.",
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
    target = arguments.get("target_agent_id", "")
    context_msg = arguments.get("context", "")
    target_mode = arguments.get("target_mode", "")
    artifacts: list[str] = arguments.get("artifacts", [])
    plan_id = arguments.get("plan_id", "")
    step_id = arguments.get("step_id", "")
    metadata: dict[str, str] = dict(arguments.get("metadata", {}))

    if not target or not context_msg:
        return "Error: target_agent_id and context are required"

    # Auto-generate chain tracking
    if "handoff_chain_id" not in metadata:
        metadata["handoff_chain_id"] = str(uuid.uuid4())
        metadata["handoff_hop"] = "0"
    else:
        hop = int(metadata.get("handoff_hop", "0")) + 1
        if hop > MAX_HANDOFF_HOPS:
            return (
                f"Error: handoff chain exceeded maximum of {MAX_HANDOFF_HOPS} hops"
                " (possible cycle)"
            )
        metadata["handoff_hop"] = str(hop)

    payload = {
        "source_run_id": run_id,
        "target_agent_id": target,
        "target_mode_id": target_mode,
        "context": context_msg,
        "artifacts": artifacts,
        "plan_id": plan_id,
        "step_id": step_id,
        "metadata": metadata,
    }

    await nats_publish(SUBJECT_HANDOFF_REQUEST, json.dumps(payload).encode())

    logger.info(
        "handoff initiated",
        run_id=run_id,
        target=target,
        mode=target_mode,
        plan_id=plan_id,
        chain_id=metadata.get("handoff_chain_id", ""),
    )

    return f"Handoff to {target} initiated successfully."
