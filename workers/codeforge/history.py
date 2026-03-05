"""Conversation history manager for agentic loops.

Assembles system prompt + conversation history within a token budget,
using a head-and-tail strategy to preserve the most relevant context.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import TYPE_CHECKING

from codeforge.constants import CHARS_PER_TOKEN

if TYPE_CHECKING:
    from codeforge.models import ContextEntry, ConversationMessagePayload

logger = logging.getLogger(__name__)

# Maximum characters for tool result output before truncation.
DEFAULT_TOOL_OUTPUT_MAX_CHARS = 10_000


def estimate_tokens(text: str) -> int:
    """Fast token estimate using 4-chars-per-token heuristic."""
    return max(1, len(text) // CHARS_PER_TOKEN)


def truncate_tool_result(text: str, max_chars: int = DEFAULT_TOOL_OUTPUT_MAX_CHARS) -> str:
    """Truncate long tool results, keeping head + tail.

    Returns the original text if it fits within *max_chars*.
    Otherwise keeps the first half and last half with an
    ellipsis separator indicating how many characters were omitted.
    """
    if len(text) <= max_chars:
        return text
    half = max_chars // 2
    omitted = len(text) - max_chars
    return f"{text[:half]}\n\n... ({omitted} characters omitted) ...\n\n{text[-half:]}"


@dataclass
class HistoryConfig:
    """Configuration for history assembly."""

    max_context_tokens: int = 120_000
    tool_output_max_chars: int = DEFAULT_TOOL_OUTPUT_MAX_CHARS
    # Minimum number of recent messages to always include (including tool messages).
    min_recent_messages: int = 20


class ConversationHistoryManager:
    """Builds the messages array for LLM calls within a token budget.

    Strategy (head-and-tail):
    1. System prompt is always included first.
    2. Context entries (repo map, retrieval results, diagnostics) are injected
       into the system prompt.
    3. Recent messages are always included (up to ``min_recent_messages``).
    4. Older messages are included from the beginning until the budget is exhausted.
    5. Long tool results are truncated.
    """

    def __init__(self, config: HistoryConfig | None = None) -> None:
        self._config = config or HistoryConfig()

    def build_messages(
        self,
        system_prompt: str,
        history: list[ConversationMessagePayload],
        context_entries: list[ContextEntry] | None = None,
    ) -> list[dict[str, object]]:
        """Assemble the final messages list for the LLM.

        Returns a list of dicts in OpenAI message format:
        ``[{"role": ..., "content": ..., "tool_calls": ..., ...}]``
        """
        # Build system message with context entries injected.
        system_content = self._build_system_content(system_prompt, context_entries)
        system_msg: dict[str, object] = {"role": "system", "content": system_content}
        system_tokens = estimate_tokens(system_content)

        budget = self._config.max_context_tokens - system_tokens
        if budget <= 0:
            logger.warning("system prompt alone exceeds token budget")
            return [system_msg]

        # Convert history to message dicts, truncating tool results.
        all_msgs = [self._to_msg_dict(m) for m in history]

        # Split into "tail" (always included) and "head" (included if budget allows).
        min_recent = min(self._config.min_recent_messages, len(all_msgs))
        tail = all_msgs[-min_recent:] if min_recent > 0 else []
        head = all_msgs[: len(all_msgs) - min_recent] if min_recent > 0 else all_msgs

        # Calculate tail token cost.
        tail_tokens = sum(self._msg_tokens(m) for m in tail)
        remaining = budget - tail_tokens

        # Include as many head messages as possible.
        included_head: list[dict[str, object]] = []
        for msg in head:
            msg_tokens = self._msg_tokens(msg)
            if msg_tokens > remaining:
                break
            included_head.append(msg)
            remaining -= msg_tokens

        result = [system_msg, *included_head, *tail]
        result = self._sanitize_tool_pairing(result)
        total_tokens = system_tokens + sum(self._msg_tokens(m) for m in included_head) + tail_tokens
        logger.debug(
            "history assembled: %d messages, ~%d tokens (budget %d)",
            len(result),
            total_tokens,
            self._config.max_context_tokens,
        )
        return result

    def _build_system_content(self, base_prompt: str, context_entries: list[ContextEntry] | None) -> str:
        """Inject context entries into the system prompt."""
        if not context_entries:
            return base_prompt

        sections: list[str] = [base_prompt]
        for entry in context_entries:
            if entry.content:
                label = entry.kind.capitalize() if entry.kind else "Context"
                sections.append(f"\n\n## {label}\n{entry.content}")
        return "".join(sections)

    def _to_msg_dict(self, msg: ConversationMessagePayload) -> dict[str, object]:
        """Convert a ConversationMessagePayload to an OpenAI message dict."""
        d: dict[str, object] = {"role": msg.role}

        if msg.content:
            content = msg.content
            # Truncate tool results (role="tool") that are too long.
            if msg.role == "tool":
                content = truncate_tool_result(content, self._config.tool_output_max_chars)
            d["content"] = content

        if msg.tool_calls:
            d["tool_calls"] = [
                {
                    "id": tc.id,
                    "type": tc.type,
                    "function": {
                        "name": tc.function.name,
                        "arguments": tc.function.arguments,
                    },
                }
                for tc in msg.tool_calls
            ]

        if msg.tool_call_id:
            d["tool_call_id"] = msg.tool_call_id

        if msg.name:
            d["name"] = msg.name

        return d

    def _sanitize_tool_pairing(self, messages: list[dict[str, object]]) -> list[dict[str, object]]:
        """Ensure strict tool-call/result pairing, dedup, and ordering.

        Strict providers like Mistral reject messages where:
        - tool_calls count != tool result count
        - tool results appear before their assistant message
        - duplicate tool results exist for the same tool_call_id

        This sanitizer fixes all three cases by rebuilding the message list
        with correct structure: each assistant(tool_calls) is immediately
        followed by exactly one tool result per tool_call_id.
        """
        original_len = len(messages)

        # Index: tool_call_id → first tool result message seen.
        first_result: dict[str, dict[str, object]] = {}
        for m in messages:
            if m.get("role") == "tool":
                tcid = m.get("tool_call_id")
                if isinstance(tcid, str) and tcid and tcid not in first_result:
                    first_result[tcid] = m

        # Track which tool_call_ids have been emitted (dedup across
        # duplicate assistant messages that share the same tool_call IDs).
        emitted_call_ids: set[str] = set()

        result: list[dict[str, object]] = []
        for m in messages:
            if m.get("role") == "tool":
                # Skip all inline tool results — they'll be re-inserted
                # in correct position after their assistant message.
                continue

            if m.get("role") == "assistant" and m.get("tool_calls"):
                tcs = m["tool_calls"]
                if not isinstance(tcs, list):
                    result.append(m)
                    continue

                # Keep only tool_calls that (a) have a matching result and
                # (b) haven't been emitted by a previous duplicate assistant msg.
                kept = [
                    tc
                    for tc in tcs
                    if isinstance(tc, dict) and tc.get("id") in first_result and tc.get("id") not in emitted_call_ids
                ]
                if not kept:
                    # All tool_calls already emitted or no results — drop
                    # the tool_calls key but keep the message if it has content.
                    stripped = {k: v for k, v in m.items() if k != "tool_calls"}
                    if stripped.get("content"):
                        result.append(stripped)
                    continue

                m["tool_calls"] = kept
                result.append(m)

                # Insert the matching tool results immediately after.
                for tc in kept:
                    tc_id = tc["id"]
                    emitted_call_ids.add(tc_id)
                    result.append(first_result[tc_id])
            else:
                result.append(m)

        removed = original_len - len(result)
        if removed:
            logger.warning("sanitize_tool_pairing fixed %d messages (dedup/reorder)", removed)

        return result

    def _msg_tokens(self, msg: dict[str, object]) -> int:
        """Estimate token count for a single message dict."""
        total = 0
        content = msg.get("content", "")
        if isinstance(content, str):
            total += estimate_tokens(content)
        tool_calls = msg.get("tool_calls")
        if isinstance(tool_calls, list):
            for tc in tool_calls:
                if isinstance(tc, dict):
                    func = tc.get("function", {})
                    total += estimate_tokens(str(func.get("name", "")))
                    total += estimate_tokens(str(func.get("arguments", "")))
        return max(1, total)
