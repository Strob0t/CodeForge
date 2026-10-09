"""Stall detection for the agentic loop.

Detects when the agent repeats the same tool calls and provides
escape/abort signals to break out of repetitive patterns.
"""

from __future__ import annotations

import hashlib
import json
from collections import deque

# Every stall error starts with it; the Go Core re-plans a plan step whose run
# failed with such an error (run.StallMarker, contract:
# internal/domain/run/testdata/stall_contract.json).
STALL_ERROR_MARKER = "stall detected:"


def stall_error(repeated_action: object, escape_count: object) -> str:
    """Return the run error of an agent loop aborted for a stall."""
    return f"{STALL_ERROR_MARKER} repeated {repeated_action} after {escape_count} escape attempts"


STALL_ESCAPE_PROMPT = (
    "<SYSTEM: You are repeating the same action without progress. "
    "Stop and try a fundamentally different approach. If you were reading, "
    "start writing. If you were searching, use what you found.>"
)


class StallDetector:
    """Detect when the agent repeats the same tool call and force escape.

    A stall is *stall_threshold* identical ``(tool_name, args)`` calls in a
    row, with no other call in between (KI-191): an edit/test loop repeats
    its test command, but with an edit in between, which is progress. A pair
    of two different calls repeated *stall_threshold* times in a row (the
    same edit and the same test, again and again) is a stall too. An escape
    prompt clears the window, so only a new run of repeats stalls again; a
    stall after *max_escapes* escape prompts aborts the loop. The window
    keeps the last *window_size* calls for the contextual escape prompt.
    """

    def __init__(self, window_size: int = 5, stall_threshold: int = 3, max_escapes: int = 2) -> None:
        self._window: deque[tuple[str, str]] = deque(maxlen=window_size)
        self._threshold = stall_threshold
        self._max_escapes = max_escapes
        self._escape_count = 0
        # Length of the run of identical calls at the end of the window.
        self._repeats = 0
        # Length of the run of calls equal to the call two before them.
        self._cycle = 0

    @staticmethod
    def _hash_args(name: str, args: dict[str, object]) -> str:
        raw = json.dumps(args, sort_keys=True, default=str)
        return hashlib.sha256(f"{name}:{raw}".encode()).hexdigest()

    def record(self, tool_name: str, args: dict[str, object]) -> None:
        """Append a tool call to the window and extend or restart the run of repeats."""
        entry = (tool_name, self._hash_args(tool_name, args))
        self._repeats = self._repeats + 1 if self._window and self._window[-1] == entry else 1
        self._cycle = self._cycle + 1 if len(self._window) >= 2 and self._window[-2] == entry else 0
        self._window.append(entry)

    def _pair_repeats(self) -> int:
        """Return how often the last two (different) calls repeat as a pair at the end of the window."""
        if len(self._window) < 2 or self._window[-1] == self._window[-2]:
            return 0
        return self._cycle // 2 + 1

    def is_stalled(self) -> bool:
        """Return True if the last calls repeat one call or one pair of calls *stall_threshold* times."""
        return self._repeats >= self._threshold or self._pair_repeats() >= self._threshold

    def get_repeated_action(self) -> str | None:
        """Return the tool name of the repeated call (or both names of a pair) while stalled, or None."""
        if self._repeats >= self._threshold:
            return self._window[-1][0]  # tool_name from (tool_name, args_hash)
        if self._pair_repeats() >= self._threshold:
            return f"{self._window[-2][0]}, {self._window[-1][0]}"
        return None

    def record_escape(self) -> None:
        """Record an injected escape prompt and clear the window for a fresh start."""
        self._escape_count += 1
        self._window.clear()
        self._repeats = 0
        self._cycle = 0

    def should_abort(self) -> bool:
        """Return True if the agent stalls again after *max_escapes* escape prompts."""
        return self._escape_count >= self._max_escapes and self.is_stalled()

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
