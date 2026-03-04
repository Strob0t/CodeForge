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

    max_context_tokens: int = 128_000
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
        """Ensure every tool_call has a matching tool result and vice versa.

        Strict providers like Mistral reject messages where the count of
        tool_calls does not match the count of tool result messages.  This
        can happen when head-and-tail truncation splits a tool-call group
        or when the DB contains incomplete sequences.
        """
        # 1. Collect tool_call_ids present in tool result messages.
        result_ids: set[str] = set()
        for m in messages:
            if m.get("role") == "tool":
                tcid = m.get("tool_call_id")
                if isinstance(tcid, str) and tcid:
                    result_ids.add(tcid)

        # 2. Filter assistant tool_calls to only those with matching results.
        call_ids: set[str] = set()
        for m in messages:
            if m.get("role") != "assistant" or not m.get("tool_calls"):
                continue
            tcs = m["tool_calls"]
            if not isinstance(tcs, list):
                continue
            kept = [tc for tc in tcs if isinstance(tc, dict) and tc.get("id") in result_ids]
            if kept:
                m["tool_calls"] = kept
                call_ids.update(tc["id"] for tc in kept)
            else:
                del m["tool_calls"]

        # 3. Remove orphaned tool result messages.
        original_len = len(messages)
        messages = [m for m in messages if not (m.get("role") == "tool" and m.get("tool_call_id") not in call_ids)]

        removed = original_len - len(messages)
        if removed:
            logger.warning("sanitize_tool_pairing removed %d orphaned messages", removed)

        return messages

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
