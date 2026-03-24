"""Helper functions for the agentic loop.

Message building, tool result formatting, sanitization, plan/act helpers,
model switching, and schema resolution.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from codeforge.models import (
    ConversationMessagePayload,
    ConversationToolCallFunction,
    ConversationToolCallPayload,
)

if TYPE_CHECKING:
    from codeforge.llm import ChatCompletionResponse, ToolCallPart
    from codeforge.plan_act import PlanActController
    from codeforge.quality_tracking import IterationQualityTracker
    from codeforge.tools._base import ToolResult as _ToolResultType

logger = logging.getLogger(__name__)

_PLAN_ACT_MARKER = "\n\nYou are in "


class ToolErrorTracker:
    """Tracks per-tool error counts to prevent retry loops.

    When a tool produces the same error twice (after normalization), it is
    marked as NON-RETRYABLE and the agent receives a block message telling
    it to stop retrying and move on.
    """

    __slots__ = ("_counts", "_max_identical")

    def __init__(self, max_identical: int = 2) -> None:
        self._counts: dict[tuple[str, str], int] = {}  # (tool, error_sig) -> count
        self._max_identical = max_identical

    def record_error(self, tool_name: str, error: str) -> bool:
        """Record an error. Returns True if this is NON-RETRYABLE (exceeded max)."""
        sig = self._normalize_error(error)
        key = (tool_name, sig)
        self._counts[key] = self._counts.get(key, 0) + 1
        return self._counts[key] >= self._max_identical

    def get_block_message(self, tool_name: str) -> str:
        """Return a message telling the agent this tool is blocked."""
        return (
            f"[NON-RETRYABLE] Tool '{tool_name}' has failed {self._max_identical} times "
            f"with the same error. Do NOT retry. Continue with your main task "
            f"using read_file, write_file, or bash."
        )

    @staticmethod
    def _normalize_error(error: str) -> str:
        """Strip variable parts (line numbers, paths, UUIDs) for comparison."""
        import re

        # Strip UUIDs before numbers so hex digits are caught.
        s = re.sub(r"[0-9a-f]{8}-[0-9a-f]{4}", "UUID", error)
        s = re.sub(r"\d+", "N", s)
        return s[:200]


def build_tool_result_text(
    result: _ToolResultType,
    tool_name: str,
    error_tracker: ToolErrorTracker | None,
) -> str:
    """Build the result text for a tool call, including correction hints and error tracking."""
    if result.success:
        return result.output
    if not result.error:
        return "Tool returned an error"
    correction = build_correction_hint(tool_name, result.error)
    text = f"Error: {result.error}\n\n{correction}" if correction else f"Error: {result.error}"
    # Track repeated errors and block if NON-RETRYABLE (M5).
    if error_tracker is not None and error_tracker.record_error(tool_name, result.error):
        text = error_tracker.get_block_message(tool_name)
    return text


def build_correction_hint(tool_name: str, error: str) -> str:
    """Generate a correction hint for common tool argument errors.

    Returns an empty string if no specific hint applies.
    """
    error_lower = error.lower()

    if "not found" in error_lower or "no such file" in error_lower:
        return (
            f"Hint: The file or path was not found. Use list_directory or glob_files "
            f"to verify the correct path before retrying {tool_name}."
        )

    if "path traversal" in error_lower:
        return "Hint: Use paths relative to the workspace root. Do not use absolute paths or '..'."

    if tool_name == "edit_file":
        if "not found in file" in error_lower:
            return (
                "Hint: The old_text was not found. Use read_file to view the current file "
                "content and copy the exact text (including whitespace and indentation)."
            )
        if "found" in error_lower and "times" in error_lower:
            return (
                "Hint: The old_text matches multiple locations. Include more surrounding "
                "context lines in old_text to make it unique."
            )

    if "timed out" in error_lower:
        return "Hint: The command timed out. Try increasing the timeout parameter or breaking it into smaller steps."

    if "permission denied" in error_lower:
        return "Hint: Permission denied. Check if the file exists and is writable."

    # Generic argument error patterns.
    if "missing" in error_lower and ("required" in error_lower or "argument" in error_lower):
        return f"Hint: A required argument is missing. Check the {tool_name} tool definition for required parameters."

    return ""


def build_assistant_message(response: ChatCompletionResponse) -> ConversationMessagePayload:
    """Build a ConversationMessagePayload for an assistant message with tool_calls."""
    return ConversationMessagePayload(
        role="assistant",
        content=response.content,
        tool_calls=[
            ConversationToolCallPayload(
                id=tc.id,
                type="function",
                function=ConversationToolCallFunction(name=tc.name, arguments=tc.arguments),
            )
            for tc in response.tool_calls
        ],
    )


def build_tool_result_message(tc: ToolCallPart, content: str) -> ConversationMessagePayload:
    """Build a ConversationMessagePayload for a tool result."""
    return ConversationMessagePayload(
        role="tool",
        content=content,
        tool_call_id=tc.id,
        name=tc.name,
    )


def payload_to_dict(msg: ConversationMessagePayload) -> dict[str, object]:
    """Convert a ConversationMessagePayload to an OpenAI-compatible message dict."""
    d: dict[str, object] = {"role": msg.role}
    if msg.role == "tool":
        # Tool messages MUST always include 'content' — some providers (e.g. Groq)
        # reject messages with role:tool missing the content field.
        d["content"] = msg.content or ""
    elif msg.content:
        d["content"] = msg.content
    if msg.tool_calls:
        d["tool_calls"] = [
            {
                "id": tc.id,
                "type": tc.type,
                "function": {"name": tc.function.name, "arguments": tc.function.arguments},
            }
            for tc in msg.tool_calls
        ]
    if msg.tool_call_id:
        d["tool_call_id"] = msg.tool_call_id
    if msg.name:
        d["name"] = msg.name
    return d


def sanitize_tool_messages(messages: list[dict[str, object]]) -> list[dict[str, object]]:
    """Normalize tool messages for cross-provider compatibility.

    Ensures every ``role:tool`` message has a non-empty ``content`` field and
    a ``tool_call_id``.  Some providers (e.g. Groq) reject tool messages that
    are missing these fields, even though other providers (e.g. Gemini) may
    omit them.
    """
    for msg in messages:
        if msg.get("role") != "tool":
            continue
        if "content" not in msg or msg["content"] is None:
            msg["content"] = ""
        if "tool_call_id" not in msg or not msg["tool_call_id"]:
            msg["tool_call_id"] = f"_sanitized_{id(msg)}"
    return messages


def init_plan_act(cfg: object, messages: list[dict[str, object]]) -> PlanActController:
    """Initialize the Plan/Act controller and inject the system prompt suffix."""
    from codeforge.plan_act import PlanActController, get_max_plan_iterations

    plan_act = PlanActController(
        enabled=cfg.plan_act_enabled,
        max_plan_iterations=get_max_plan_iterations(),
        extra_plan_tools=cfg.extra_plan_tools,
    )
    suffix = plan_act.get_system_suffix()
    if suffix:
        append_system_suffix(messages, suffix)
    return plan_act


def check_model_switch(quality_tracker: IterationQualityTracker, cfg: object) -> None:
    """Bump complexity tier and log if quality tracker recommends a model switch (C1)."""
    if not quality_tracker.should_switch() or not cfg.routing_layer:
        return
    from codeforge.routing.models import ComplexityTier

    old_tier = cfg.complexity_tier
    new_tier = quality_tracker.bump_tier(ComplexityTier(old_tier) if old_tier else ComplexityTier.SIMPLE)
    quality_tracker.register_switch()
    cfg.complexity_tier = str(new_tier)
    logger.info(
        "mid-loop model switch: tier %s -> %s (switch #%d)",
        old_tier,
        new_tier,
        quality_tracker.switch_count,
    )


def check_plan_act_transition(plan_act: PlanActController, messages: list[dict[str, object]]) -> None:
    """Auto-transition from plan to act when max plan iterations reached."""
    if plan_act.tick_and_should_transition():
        plan_act.transition_to_act()
        logger.info("plan/act auto-transition to act phase after %d plan iterations", plan_act.plan_iterations)
        update_system_suffix(messages, plan_act.get_system_suffix())


def append_system_suffix(messages: list[dict[str, object]], suffix: str) -> None:
    """Append plan/act suffix to the first system message."""
    for msg in messages:
        if msg.get("role") == "system":
            content = msg.get("content", "")
            msg["content"] = str(content) + suffix if content else suffix
            return


def update_system_suffix(messages: list[dict[str, object]], new_suffix: str) -> None:
    """Replace plan/act suffix on the first system message (phase transition)."""
    for msg in messages:
        if msg.get("role") == "system":
            content = str(msg.get("content", ""))
            idx = content.find(_PLAN_ACT_MARKER)
            if idx >= 0:
                content = content[:idx]
            msg["content"] = content + new_suffix
            return


def resolve_schema(name: str) -> type | None:
    """Resolve a schema name to the corresponding Pydantic model class from codeforge.schemas."""
    import codeforge.schemas as _schemas_mod

    return getattr(_schemas_mod, name, None)
