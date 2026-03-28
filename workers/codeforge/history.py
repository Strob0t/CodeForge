"""Conversation history manager for agentic loops.

Assembles system prompt + conversation history within a token budget,
using a head-and-tail strategy to preserve the most relevant context.
"""

from __future__ import annotations

import base64
import logging
from dataclasses import dataclass
from typing import TYPE_CHECKING

from codeforge.constants import CHARS_PER_TOKEN
from codeforge.models import ConversationMessagePayload

if TYPE_CHECKING:
    from codeforge.llm import LiteLLMClient
    from codeforge.models import ContextEntry

logger = logging.getLogger(__name__)

# Maximum characters for tool result output before truncation.
DEFAULT_TOOL_OUTPUT_MAX_CHARS = 10_000


def estimate_tokens(text: str) -> int:
    """Fast token estimate using 4-chars-per-token heuristic."""
    return max(1, len(text) // CHARS_PER_TOKEN)


def estimate_messages_tokens(messages: list[dict[str, object]]) -> int:
    """Estimate total tokens across a list of OpenAI-format messages."""
    total = 0
    for msg in messages:
        content = msg.get("content", "")
        if isinstance(content, str):
            total += estimate_tokens(content)
        elif isinstance(content, list):
            for part in content:
                if isinstance(part, dict) and "text" in part:
                    total += estimate_tokens(str(part["text"]))
        # Tool calls add ~50 tokens each
        tool_calls = msg.get("tool_calls")
        if tool_calls and isinstance(tool_calls, list):
            total += len(tool_calls) * 50
    return total


def trim_messages_to_budget(
    messages: list[dict[str, object]],
    context_limit: int,
    keep_recent: int = 10,
) -> list[dict[str, object]]:
    """Trim messages to fit within context budget.

    Keeps the system prompt (first message) and the most recent messages.
    Removes older messages from the middle when the budget is exceeded.
    Only triggers when estimated token usage exceeds 80% of the limit.
    """
    current_tokens = estimate_messages_tokens(messages)
    threshold = int(context_limit * 0.80)

    if current_tokens <= threshold:
        return messages

    # Keep system prompt + last N messages, drop middle
    if len(messages) <= keep_recent + 1:
        return messages

    system = messages[:1] if messages and messages[0].get("role") == "system" else []
    tail = messages[-keep_recent:]

    # Try including some head messages if budget allows
    head_candidates = messages[len(system) : len(messages) - keep_recent]
    budget = context_limit - estimate_messages_tokens(system) - estimate_messages_tokens(tail)

    kept_head: list[dict[str, object]] = []
    for msg in head_candidates:
        msg_tokens = estimate_messages_tokens([msg])
        if msg_tokens > budget:
            break
        kept_head.append(msg)
        budget -= msg_tokens

    trimmed = system + kept_head + tail
    new_tokens = estimate_messages_tokens(trimmed)
    dropped = len(messages) - len(trimmed)
    if dropped > 0:
        logger.info(
            "proactive context trim: dropped %d messages (%d -> %d tokens, limit=%d)",
            dropped,
            current_tokens,
            new_tokens,
            context_limit,
        )
    return trimmed


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
        """Convert a ConversationMessagePayload to an OpenAI message dict.

        When the message has images and the role is "user", produces the
        OpenAI content-array format with text and image_url parts.
        """
        d: dict[str, object] = {"role": msg.role}

        if msg.content:
            content = msg.content
            # Truncate tool results (role="tool") that are too long.
            if msg.role == "tool":
                content = truncate_tool_result(content, self._config.tool_output_max_chars)

            # If images present AND role is "user", use content-array format.
            if msg.images and msg.role == "user":
                valid_images = self._filter_valid_images(msg.images)
                if valid_images:
                    content_parts: list[dict[str, object]] = [{"type": "text", "text": content}]
                    content_parts.extend(
                        {
                            "type": "image_url",
                            "image_url": {"url": f"data:{img.media_type};base64,{img.data}"},
                        }
                        for img in valid_images
                    )
                    d["content"] = content_parts
                else:
                    d["content"] = content
            else:
                d["content"] = content
        elif msg.images and msg.role == "user":
            # Images without text content.
            valid_images = self._filter_valid_images(msg.images)
            if valid_images:
                content_parts = [
                    {
                        "type": "image_url",
                        "image_url": {"url": f"data:{img.media_type};base64,{img.data}"},
                    }
                    for img in valid_images
                ]
                d["content"] = content_parts

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

    @staticmethod
    def _filter_valid_images(images: list[object]) -> list[object]:
        """Filter out images with invalid base64 data."""
        valid: list[object] = []
        for img in images:
            try:
                base64.b64decode(img.data, validate=True)
            except Exception as exc:
                logger.warning(
                    "skipping image with invalid base64 data: %s",
                    exc,
                    extra={"image_id": getattr(img, "id", "unknown")},
                    exc_info=True,
                )
                continue
            valid.append(img)
        return valid

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
        elif isinstance(content, list):
            # Content-array format (multimodal).
            for part in content:
                if isinstance(part, dict):
                    if part.get("type") == "text":
                        total += estimate_tokens(str(part.get("text", "")))
                    elif part.get("type") == "image_url":
                        total += 1000  # ~1000 tokens per image
        tool_calls = msg.get("tool_calls")
        if isinstance(tool_calls, list):
            for tc in tool_calls:
                if isinstance(tc, dict):
                    func = tc.get("function", {})
                    total += estimate_tokens(str(func.get("name", "")))
                    total += estimate_tokens(str(func.get("arguments", "")))
        return max(1, total)


_SUMMARIZE_SYSTEM = (
    "You are a conversation summarizer for a coding assistant session. "
    "Summarize the conversation concisely. Preserve: key decisions made, "
    "code changes performed, files read/modified, errors encountered, "
    "and current task state. Remove redundant back-and-forth. "
    "Output a single cohesive summary paragraph."
)

_SUMMARIZE_MAX_CHARS = 50_000


class ConversationSummarizer:
    """Summarizes older conversation history when context is exhausted.

    When ``summarize_if_needed()`` is called and the history length exceeds
    the threshold, the head (older) messages are summarized via LLM and
    replaced with a single summary message. The tail (recent) messages
    are always preserved intact.
    """

    def __init__(
        self,
        llm: LiteLLMClient,  # also accepts FakeLLM (duck-typed)
        threshold: int = 60,
        min_recent: int = 20,
    ) -> None:
        self._llm = llm
        self._threshold = threshold
        self._min_recent = min_recent

    async def summarize_if_needed(
        self,
        history: list[ConversationMessagePayload],
    ) -> list[ConversationMessagePayload]:
        """Summarize head of history if length exceeds threshold.

        Returns the (possibly shortened) message list. If summarization
        fails, returns the original history unchanged.
        """
        if len(history) <= self._threshold:
            return history

        # Split: head (to summarize) + tail (to preserve)
        tail_start = max(0, len(history) - self._min_recent)
        head = history[:tail_start]
        tail = history[tail_start:]

        if not head:
            return history

        try:
            summary_text = await self._summarize_history(head)
        except Exception as exc:
            logger.warning("conversation summarization failed, keeping original history", exc_info=True, error=str(exc))
            return history

        summary_msg = ConversationMessagePayload(
            role="system",
            content=f"[Conversation Summary]\n{summary_text}",
        )

        return [summary_msg, *tail]

    async def _summarize_history(
        self,
        messages: list[ConversationMessagePayload],
    ) -> str:
        """Call LLM to summarize a list of messages."""
        lines: list[str] = []
        for msg in messages:
            content = msg.content or ""
            lines.append(f"{msg.role}: {content}")

        full_text = "\n".join(lines)
        if len(full_text) > _SUMMARIZE_MAX_CHARS:
            full_text = full_text[:_SUMMARIZE_MAX_CHARS] + "\n... (truncated)"

        resp = await self._llm.completion(
            prompt=full_text,
            system=_SUMMARIZE_SYSTEM,
            temperature=0.1,
            tags=["background"],
        )
        return resp.content
