"""Stall detection for the agentic loop.

Detects when the agent repeats the same tool calls and provides
escape/abort signals to break out of repetitive patterns.
"""

from __future__ import annotations

import hashlib
import json
from collections import Counter, deque

STALL_ESCAPE_PROMPT = (
    "<SYSTEM: You are repeating the same action without progress. "
    "Stop and try a fundamentally different approach. If you were reading, "
    "start writing. If you were searching, use what you found.>"
)


class StallDetector:
    """Detect when the agent repeats the same tool call and force escape.

    Maintains a sliding window of recent ``(tool_name, args_hash)`` tuples.
    If *stall_threshold* or more entries in the last *window_size* are
    identical, the agent is considered stalled.  After two escape attempts
    the detector signals that the loop should abort.
    """

    def __init__(self, window_size: int = 5, stall_threshold: int = 3) -> None:
        self._window: deque[tuple[str, str]] = deque(maxlen=window_size)
        self._threshold = stall_threshold
        self._escape_count = 0

    @staticmethod
    def _hash_args(name: str, args: dict[str, object]) -> str:
        raw = json.dumps(args, sort_keys=True, default=str)
        return hashlib.sha256(f"{name}:{raw}".encode()).hexdigest()

    def record(self, tool_name: str, args: dict[str, object]) -> None:
        """Append a tool call to the sliding window."""
        self._window.append((tool_name, self._hash_args(tool_name, args)))

    def is_stalled(self) -> bool:
        """Return True if >= threshold entries in the window are identical."""
        if len(self._window) < self._threshold:
            return False
        counts = Counter(self._window)
        return counts.most_common(1)[0][1] >= self._threshold

    def get_repeated_action(self) -> str | None:
        """Return the tool name of the most-repeated action, or None."""
        if not self._window:
            return None
        counts = Counter(self._window)
        entry, count = counts.most_common(1)[0]
        if count >= self._threshold:
            return entry[0]  # tool_name from (tool_name, args_hash)
        return None

    def record_escape(self) -> None:
        """Record that an escape prompt was injected."""
        self._escape_count += 1

    def should_abort(self) -> bool:
        """Return True if the loop should abort (>= 2 escape attempts)."""
        return self._escape_count >= 2

    def get_abort_info(self) -> dict[str, object]:
        """Return structured info about the stall for error reporting."""
        return {
            "repeated_action": self.get_repeated_action(),
            "escape_count": self._escape_count,
        }

    def get_recent_tool_names(self) -> list[str]:
        """Return tool names from the sliding window (most recent last)."""
        return [name for name, _ in self._window]

    def get_contextual_escape_prompt(self, recent_tools: list[str]) -> str:
        """Generate a context-aware escape prompt based on what the agent has been doing."""
        if not recent_tools:
            return "You MUST use tools to make progress. Call list_directory or read_file to start."

        reads = sum(1 for t in recent_tools if t in ("read_file", "search_files", "glob_files", "list_directory"))
        writes = sum(1 for t in recent_tools if t in ("write_file", "edit_file"))
        bashes = sum(1 for t in recent_tools if t == "bash")

        if reads > writes and reads > bashes:
            return (
                "You have been exploring the codebase. You have enough context now "
                "-- start writing or editing code to make progress."
            )
        if writes > 0 and bashes == 0:
            return (
                "You have been writing code but not testing it. Run the tests or "
                "verify your changes with bash before continuing."
            )
        if bashes > reads:
            return (
                "Your bash commands are not producing the expected results. Read the "
                "error output carefully, then try a different approach."
            )
        return (
            "You are repeating actions without progress. Step back and try a "
            "fundamentally different approach to solve this task."
        )
