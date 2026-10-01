"""Built-in tool: handoff_to -- allows agents to initiate handoffs to other agents."""

from __future__ import annotations

import json
import uuid
from typing import TYPE_CHECKING, Any

import structlog

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

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


def _string_metadata(raw: object) -> dict[str, str] | None:
    """The handoff metadata with string values, or None if it is no object.

    The Go Core reads metadata as a map of strings; the LLM supplies a
    free-form object, so a non-string value is sent JSON-encoded (S2-G fix, 4).
    """
    if raw is None:
        return {}
    if not isinstance(raw, dict):
        return None
    return {str(key): value if isinstance(value, str) else json.dumps(value) for key, value in raw.items()}


async def execute_handoff(
    run_id: str,
    arguments: dict[str, Any],
    nats_publish: Callable[[str, bytes], Awaitable[object]],
    tenant_id: str = "",
    project_id: str = "",
    *,
    workspace_path: str,
    approval_timeout_seconds: int = 0,
) -> str:
    """Execute a handoff_to tool call by publishing a handoff request to the Go Core.

    The Go Core checks the request (trust, quarantine, the target agent in the
    source's tenant and project), creates the target agent's task and starts
    its run (KI-15). tenant_id and project_id are the source run's: the
    handoff run belongs to the same tenant and project. workspace_path and
    approval_timeout_seconds are informational: the Go Core takes them from
    the project and its config.
    """
    target = arguments.get("target_agent_id", "")
    context_msg = arguments.get("context", "")
    target_mode = arguments.get("target_mode", "")
    artifacts: list[str] = arguments.get("artifacts", [])
    plan_id = arguments.get("plan_id", "")
    step_id = arguments.get("step_id", "")
    metadata = _string_metadata(arguments.get("metadata"))

    if not target or not context_msg:
        return "Error: target_agent_id and context are required"
    if metadata is None:
        return "Error: metadata must be an object of key-value pairs"
    if not workspace_path.strip():
        # The handoff run would fail without a workspace; refuse it here.
        return "Error: handoff not possible: this run has no workspace to hand over"

    # Auto-generate chain tracking
    if "handoff_chain_id" not in metadata:
        metadata["handoff_chain_id"] = str(uuid.uuid4())
        metadata["handoff_hop"] = "0"
    else:
        try:
            hop = int(metadata.get("handoff_hop", "0")) + 1
        except ValueError:
            return "Error: metadata.handoff_hop must be a whole number"
        if hop > MAX_HANDOFF_HOPS:
            return f"Error: handoff chain exceeded maximum of {MAX_HANDOFF_HOPS} hops (possible cycle)"
        metadata["handoff_hop"] = str(hop)

    payload = {
        "tenant_id": tenant_id,
        "project_id": project_id,
        "source_run_id": run_id,
        "target_agent_id": target,
        "target_mode_id": target_mode,
        "context": context_msg,
        "artifacts": artifacts,
        "plan_id": plan_id,
        "step_id": step_id,
        "metadata": metadata,
        "workspace_path": workspace_path,
        "approval_timeout_seconds": approval_timeout_seconds,
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
