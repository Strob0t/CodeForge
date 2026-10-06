"""Tests for the StallDetector in the agent loop (Measure A1)."""

from __future__ import annotations

import hashlib
import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

from codeforge.agent_loop import AgentLoopExecutor, LoopConfig, StallDetector
from codeforge.llm import ChatCompletionResponse, ToolCallPart
from codeforge.models import ToolCallDecision
from codeforge.tools import ToolRegistry
from codeforge.tools._base import ToolDefinition, ToolResult

if TYPE_CHECKING:
    from pathlib import Path

ESCAPE_PROMPT = (
    "<SYSTEM: You are repeating the same action without progress. "
    "Stop and try a fundamentally different approach. If you were reading, "
    "start writing. If you were searching, use what you found.>"
)


def _args_hash(args: dict) -> str:
    """Compute the same hash the StallDetector uses internally."""
    raw = json.dumps(args, sort_keys=True)[:200]
    return hashlib.sha256(raw.encode()).hexdigest()


# ---- Test 1: Empty history returns False ----


class TestDetectStallEmpty:
    def test_empty_history_not_stalled(self) -> None:
        detector = StallDetector()
        assert detector.is_stalled() is False

    def test_empty_history_no_repeated_action(self) -> None:
        detector = StallDetector()
        assert detector.get_repeated_action() is None


# ---- Test 2: Single entry returns False ----


class TestDetectStallSingleEntry:
    def test_single_entry_not_stalled(self) -> None:
        detector = StallDetector()
        detector.record("Read", {"file": "main.py"})
        assert detector.is_stalled() is False


# ---- Test 3: 3 identical consecutive tool calls -> is_stalled() True ----


class TestThreeIdenticalCalls:
    def test_three_identical_calls_stalled(self) -> None:
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is True

    def test_three_identical_calls_repeated_action(self) -> None:
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.get_repeated_action() == "Read"


# ---- Test 4: 5 different tool calls -> is_stalled() False ----


class TestFiveDifferentCalls:
    def test_five_different_calls_not_stalled(self) -> None:
        detector = StallDetector()
        detector.record("Read", {"file": "a.py"})
        detector.record("Write", {"file": "b.py"})
        detector.record("Edit", {"file": "c.py"})
        detector.record("Search", {"query": "hello"})
        detector.record("Glob", {"pattern": "*.py"})
        assert detector.is_stalled() is False


# ---- Test 5: Same tool with different args (different hash) -> not stalled ----


class TestSameToolDifferentArgs:
    def test_same_tool_different_args_not_stalled(self) -> None:
        detector = StallDetector()
        detector.record("Read", {"file": "a.py"})
        detector.record("Read", {"file": "b.py"})
        detector.record("Read", {"file": "c.py"})
        detector.record("Read", {"file": "d.py"})
        detector.record("Read", {"file": "e.py"})
        assert detector.is_stalled() is False


# ---- Test 6: Same tool with same args hash -> is_stalled() True ----


class TestSameToolSameArgs:
    def test_same_tool_same_args_stalled(self) -> None:
        detector = StallDetector()
        args = {"query": "find the bug"}
        detector.record("Search", args)
        detector.record("Search", args)
        detector.record("Search", args)
        assert detector.is_stalled() is True

    def test_three_of_five_identical_with_calls_between_is_no_stall(self) -> None:
        """Only consecutive identical calls count (KI-191): other calls in between are progress."""
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Write", {"file": "other.py"})
        detector.record("Read", args)
        detector.record("Edit", {"file": "x.py"})
        detector.record("Read", args)
        assert detector.is_stalled() is False


# ---- Test 7: After stall detected, escape prompt should be injectable ----


class TestEscapePromptInjection:
    def test_escape_prompt_content(self) -> None:
        """Verify the escape prompt constant matches the expected text."""
        from codeforge.stall_detection import STALL_ESCAPE_PROMPT

        assert "repeating the same action" in STALL_ESCAPE_PROMPT
        assert "fundamentally different approach" in STALL_ESCAPE_PROMPT

    def test_record_escape_increments(self) -> None:
        detector = StallDetector()
        assert detector.should_abort() is False
        detector.record_escape()
        assert detector.should_abort() is False  # only 1 escape so far

    def test_escape_resets_not_stalled(self) -> None:
        """After escape + new different calls, stall should clear."""
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is True
        detector.record_escape()
        # Now record different calls to move the window
        detector.record("Write", {"file": "output.py"})
        detector.record("Edit", {"file": "other.py"})
        detector.record("Search", {"query": "test"})
        assert detector.is_stalled() is False


# ---- Test 8: Double stall -> should_abort() True ----


class TestDoubleStallAbort:
    def test_third_stall_after_two_escapes_aborts(self) -> None:
        detector = StallDetector()
        args = {"file": "main.py"}
        for _ in range(2):
            for _ in range(3):
                detector.record("Read", args)
            assert detector.is_stalled() is True
            assert detector.should_abort() is False
            detector.record_escape()
            assert detector.is_stalled() is False, "the escape clears the window"

        for _ in range(3):
            detector.record("Read", args)
        assert detector.should_abort() is True

    def test_no_abort_when_the_agent_recovers_after_two_escapes(self) -> None:
        detector = StallDetector()
        detector.record_escape()
        detector.record_escape()
        detector.record("Edit", {"file": "a.py"})
        assert detector.should_abort() is False

    def test_single_escape_no_abort(self) -> None:
        detector = StallDetector()
        detector.record_escape()
        assert detector.should_abort() is False


# ---- Test 9: After should_abort(), error payload contains details ----


class TestAbortErrorPayload:
    def test_abort_error_payload(self) -> None:
        detector = StallDetector()
        args = {"file": "main.py"}
        detector.record_escape()
        detector.record_escape()
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.should_abort() is True

        payload = detector.get_abort_info()
        assert payload["repeated_action"] == "Read"
        assert payload["escape_count"] == 2

    def test_abort_info_when_not_stalled(self) -> None:
        detector = StallDetector()
        payload = detector.get_abort_info()
        assert payload["repeated_action"] is None
        assert payload["escape_count"] == 0


# ---- Edge cases ----


class TestStallDetectorEdgeCases:
    def test_window_slides_correctly(self) -> None:
        """Old entries should fall out of the sliding window."""
        detector = StallDetector(window_size=5, stall_threshold=3)
        args = {"file": "main.py"}
        # Fill window with 3 identical
        detector.record("Read", args)
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is True
        # Push 3 different to shift window
        detector.record("Write", {"file": "a.py"})
        detector.record("Edit", {"file": "b.py"})
        detector.record("Search", {"query": "c"})
        # Now window has [Read, Read, Write, Edit, Search] -> only 2 Read
        # Wait - deque maxlen=5, so after 6 entries: [Read, Read, Write, Edit, Search]
        # Actually: we recorded 6 items with maxlen=5, so window is
        # [Read, Read, Write, Edit, Search] which has 2 Reads
        # But actually: the deque drops the oldest, so after:
        # record 1: [Read]
        # record 2: [Read, Read]
        # record 3: [Read, Read, Read]
        # record 4: [Read, Read, Read, Write]
        # record 5: [Read, Read, Read, Write, Edit]
        # record 6: [Read, Read, Write, Edit, Search]  (first Read dropped)
        # 2 Reads in window of 5 -> not stalled
        assert detector.is_stalled() is False

    def test_empty_args(self) -> None:
        """Empty args dict should be handled."""
        detector = StallDetector()
        detector.record("Read", {})
        detector.record("Read", {})
        detector.record("Read", {})
        assert detector.is_stalled() is True

    def test_custom_thresholds(self) -> None:
        """Custom window_size and stall_threshold work."""
        detector = StallDetector(window_size=3, stall_threshold=2)
        args = {"file": "main.py"}
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is True

    def test_exactly_at_threshold(self) -> None:
        """Exactly threshold consecutive identical calls trigger a stall."""
        detector = StallDetector(window_size=5, stall_threshold=3)
        args = {"file": "x.py"}
        detector.record("Write", {"a": 1})
        detector.record("Read", args)
        detector.record("Read", args)
        assert detector.is_stalled() is False
        detector.record("Read", args)
        assert detector.is_stalled() is True

    def test_interleaved_identical_calls_are_no_stall(self) -> None:
        """3 of 5 identical with other calls in between is progress, not a loop (KI-191)."""
        detector = StallDetector(window_size=5, stall_threshold=3)
        args = {"file": "x.py"}
        detector.record("Read", args)
        detector.record("Write", {"a": 1})
        detector.record("Read", args)
        detector.record("Write", {"b": 2})
        detector.record("Read", args)
        assert detector.is_stalled() is False

    def test_below_threshold(self) -> None:
        """Just below threshold should not trigger."""
        detector = StallDetector(window_size=5, stall_threshold=3)
        args = {"file": "x.py"}
        detector.record("Read", args)
        detector.record("Write", {"a": 1})
        detector.record("Read", args)
        detector.record("Write", {"b": 2})
        detector.record("Edit", {"c": 3})
        # 2 of 5 are identical Read(x.py) -> not stalled
        assert detector.is_stalled() is False

    def test_args_hash_truncation(self) -> None:
        """Args longer than 200 chars should be truncated before hashing."""
        detector = StallDetector()
        long_val = "x" * 500
        args = {"content": long_val}
        detector.record("Write", args)
        detector.record("Write", args)
        detector.record("Write", args)
        assert detector.is_stalled() is True


# ---- KI-191: the agent loop's use of the detector ----


def _drive(calls: list[tuple[str, dict[str, object]]]) -> tuple[int, int, str | None]:
    """Feed calls the way the agent loop does: check (abort / escape), then record.

    Returns (escapes, abort_iteration or -1, abort error).
    """
    from codeforge.stall_detection import stall_error

    detector = StallDetector()
    escapes = 0
    for iteration, (name, args) in enumerate(calls):
        if detector.should_abort():
            info = detector.get_abort_info()
            return escapes, iteration, stall_error(info["repeated_action"], info["escape_count"])
        if detector.is_stalled():
            detector.record_escape()
            escapes += 1
        detector.record(name, args)
    return escapes, -1, None


class TestAgentLoopPatterns:
    def test_edit_test_loop_never_stalls(self) -> None:
        """edit (new args each time) / pytest (same command) is the loop's own verify pattern."""
        calls: list[tuple[str, dict[str, object]]] = []
        for i in range(20):
            calls.append(("edit_file", {"file_path": "a.py", "old_text": f"v{i}", "new_text": f"v{i + 1}"}))
            calls.append(("bash", {"command": "python -m pytest -q"}))
        escapes, aborted_at, error = _drive(calls)
        assert escapes == 0
        assert aborted_at == -1, error

    def test_repeat_loop_escapes_twice_then_aborts(self) -> None:
        same = ("bash", {"command": "python -m pytest -q"})
        escapes, aborted_at, error = _drive([same] * 30)
        assert escapes == 2
        assert aborted_at == 9, "3 calls per stall, 2 escapes, abort on the third stall"
        assert error == "stall detected: repeated bash after 2 escape attempts"

    def test_recovery_after_an_escape_is_not_aborted(self) -> None:
        same = ("read_file", {"file_path": "a.py"})
        calls = [same] * 3 + [("edit_file", {"file_path": "a.py", "n": i}) for i in range(10)]
        escapes, aborted_at, _ = _drive(calls)
        assert escapes == 1
        assert aborted_at == -1

    def test_escape_clears_the_window(self) -> None:
        detector = StallDetector()
        for _ in range(3):
            detector.record("read_file", {"file_path": "a.py"})
        assert detector.get_recent_tool_names() == ["read_file"] * 3
        detector.record_escape()
        assert detector.is_stalled() is False
        assert detector.get_recent_tool_names() == []
        detector.record("read_file", {"file_path": "a.py"})
        detector.record("read_file", {"file_path": "a.py"})
        assert detector.is_stalled() is False, "two repeats after the escape are below the threshold"


# ---- KI-191 through the agent loop ----


class _OkTool:
    async def execute(self, arguments: dict[str, object], workspace_path: str) -> ToolResult:
        return ToolResult(output="ok")


class _ScriptedNativeLLM:
    """Returns one native tool call per request, then a final answer."""

    def __init__(self, calls: list[tuple[str, dict[str, object]]]) -> None:
        self._calls = list(calls)
        self.requests = 0

    async def chat_completion_stream(self, **kwargs: object) -> ChatCompletionResponse:
        self.requests += 1
        if not self._calls:
            return ChatCompletionResponse(
                content="Done.", tool_calls=[], finish_reason="stop", model="m", tokens_in=1, tokens_out=1
            )
        name, args = self._calls.pop(0)
        part = ToolCallPart(id=f"call-{self.requests}", name=name, arguments=json.dumps(args))
        return ChatCompletionResponse(
            content="", tool_calls=[part], finish_reason="tool_calls", model="m", tokens_in=1, tokens_out=1
        )


async def _run_loop(
    calls: list[tuple[str, dict[str, object]]], workspace: str
) -> tuple[object, list[dict[str, object]]]:
    registry = ToolRegistry()
    for name in ("edit_file", "bash", "read_file"):
        registry.register(ToolDefinition(name=name, description=name, parameters={"type": "object"}), _OkTool())
    runtime = MagicMock()
    runtime.run_id = "run-1"
    runtime.project_id = "proj-1"
    runtime.is_cancelled = False
    runtime.send_output = AsyncMock()
    runtime.report_tool_result = AsyncMock()
    runtime.publish_trajectory_event = AsyncMock()
    runtime.request_tool_call = AsyncMock(return_value=ToolCallDecision(call_id="c", decision="allow"))
    messages: list[dict[str, object]] = [
        {"role": "system", "content": "You are a coder."},
        {"role": "user", "content": "Fix the bug"},
    ]
    executor = AgentLoopExecutor(
        llm=_ScriptedNativeLLM(calls),  # type: ignore[arg-type]
        tool_registry=registry,
        runtime=runtime,
        workspace_path=workspace,
    )
    result = await executor.run(messages, LoopConfig(model="m", max_iterations=60))
    return result, messages


async def test_loop_edit_test_cycle_completes(tmp_path: Path) -> None:
    calls: list[tuple[str, dict[str, object]]] = []
    for i in range(12):
        calls.append(("edit_file", {"file_path": "a.py", "old_text": f"v{i}", "new_text": f"v{i + 1}"}))
        calls.append(("bash", {"command": "python -m pytest -q"}))
    result, messages = await _run_loop(calls, str(tmp_path))
    assert not result.error  # type: ignore[attr-defined]
    assert result.final_content == "Done."  # type: ignore[attr-defined]
    assert not any("without progress" in str(m.get("content")) for m in messages)


async def test_loop_repeat_cycle_aborts_with_the_repeated_tool(tmp_path: Path) -> None:
    same = ("bash", {"command": "python -m pytest -q"})
    result, _ = await _run_loop([same] * 30, str(tmp_path))
    assert result.error == "stall detected: repeated bash after 2 escape attempts"  # type: ignore[attr-defined]
