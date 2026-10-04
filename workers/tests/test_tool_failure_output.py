"""A failed tool command keeps its output (KI-126).

A command that exits non-zero used to reach the model and the chat tool card
as "Error: exit code N" only: build_tool_result_text dropped the output, so
the traceback of a failing test was never seen. The result text is now the
error line followed by the command's output, and the loop bounds it to
agent.tool_output_max_chars keeping head and tail, like successful output.
Repeated identical failures keep their output too.
"""

from __future__ import annotations

from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock, patch

from codeforge.agent_loop import _LoopState
from codeforge.llm import ToolCallPart
from codeforge.loop_helpers import ToolErrorTracker, build_tool_result_text
from codeforge.models import ToolCallDecision
from codeforge.tool_executor import ToolExecutor
from codeforge.tools._base import ToolResult
from codeforge.tools.bash import BashTool
from codeforge.tools.search_files import SearchFilesTool

if TYPE_CHECKING:
    from pathlib import Path

    import pytest

_TRACEBACK = 'Traceback (most recent call last):\n  File "t.py", line 3, in <module>\nValueError: boom-detail'


def _failed(output: str, error: str = "exit code 1") -> ToolResult:
    return ToolResult(output=output, error=error, success=False)


def test_failed_command_keeps_its_output() -> None:
    result = _failed(f"collected 1 item\n--- stderr ---\n{_TRACEBACK}")
    # Exactly the error line and the output: nothing else enters the text.
    assert build_tool_result_text(result, "bash", ToolErrorTracker()) == f"Error: exit code 1\n\n{result.output}"


def test_failure_without_output_is_the_error_line() -> None:
    assert build_tool_result_text(_failed(""), "bash", None) == "Error: exit code 1"


def test_failure_without_error_keeps_its_output() -> None:
    assert build_tool_result_text(_failed("partial output", error=""), "bash", None) == (
        "Tool returned an error\n\npartial output"
    )


def test_correction_hint_follows_the_output() -> None:
    text = build_tool_result_text(_failed("ls: cannot access 'x'", error="No such file or directory"), "bash", None)
    assert text.startswith("Error: No such file or directory\n\nls: cannot access 'x'\n\nHint:")


def test_successful_output_is_unchanged() -> None:
    assert build_tool_result_text(ToolResult(output="ok\n"), "bash", ToolErrorTracker()) == "ok\n"


def test_different_failures_of_commands_are_no_repeated_error() -> None:
    """Every failing command reports "exit code 1": the output tells them apart."""
    tracker = ToolErrorTracker(max_identical=2)
    build_tool_result_text(_failed("FAILED test_a.py::test_a - assert 1 == 2"), "bash", tracker)
    text = build_tool_result_text(_failed("FAILED test_b.py::test_b - KeyError: 'x'"), "bash", tracker)
    assert "NON-RETRYABLE" not in text
    assert "KeyError: 'x'" in text


def test_identical_failure_keeps_its_output_with_the_block_note() -> None:
    tracker = ToolErrorTracker(max_identical=2)
    output = "FAILED test_a.py::test_a - assert 1 == 2\n1 failed in 0.12s"
    build_tool_result_text(_failed(output), "bash", tracker)
    text = build_tool_result_text(_failed(output.replace("0.12s", "0.15s")), "bash", tracker)
    assert text.startswith("Error: exit code 1\n\nFAILED test_a.py::test_a")
    assert text.endswith(tracker.get_block_message("bash"))


def test_repeated_error_without_output_is_still_replaced() -> None:
    tracker = ToolErrorTracker(max_identical=2)
    build_tool_result_text(_failed("", error="old_text not found in file"), "edit_file", tracker)
    text = build_tool_result_text(_failed("", error="old_text not found in file"), "edit_file", tracker)
    assert text == tracker.get_block_message("edit_file")


async def test_loop_bounds_a_failed_result_head_and_tail() -> None:
    """The text goes to the model and, as the stored tool message, to the chat tool card."""
    output = "HEAD-LINE\n" + "x\n" * 20_000 + _TRACEBACK
    registry = MagicMock()
    registry.execute = AsyncMock(return_value=_failed(output))
    runtime = AsyncMock()
    runtime.request_tool_call = AsyncMock(return_value=ToolCallDecision(call_id="c1", decision="allow"))
    state = _LoopState(tool_output_max_chars=2000)
    messages: list[dict[str, object]] = []

    executor = ToolExecutor(registry, runtime, "/tmp/ws", inject_state=False)
    await executor.execute(ToolCallPart(id="t1", name="bash", arguments='{"command": "pytest"}'), messages, state)

    content = str(messages[-1]["content"])
    assert content.startswith("Error: exit code 1\n\nHEAD-LINE")
    assert content.endswith("ValueError: boom-detail")
    assert "characters omitted" in content
    assert len(content) < 2100
    assert state.tool_messages[-1].content == content
    runtime.report_tool_result.assert_awaited_once()
    assert runtime.report_tool_result.call_args.kwargs["error"] == "exit code 1"
    assert runtime.report_tool_result.call_args.kwargs["success"] is False


async def test_real_bash_failure_reaches_the_result_text(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """End to end through the bash tool; worker secrets stay out (tool environment)."""
    monkeypatch.setenv("LITELLM_MASTER_KEY", "sk-worker-secret")
    command = (
        "echo 'collected 1 item'; echo 'Traceback (most recent call last):' >&2; "
        "echo 'ValueError: boom-detail' >&2; env; exit 3"
    )
    result = await BashTool().execute({"command": command}, str(tmp_path))

    text = build_tool_result_text(result, "bash", ToolErrorTracker())
    assert text.startswith("Error: exit code 3\n\ncollected 1 item")
    assert "--- stderr ---\nTraceback (most recent call last):\nValueError: boom-detail" in text
    assert "sk-worker-secret" not in text


async def test_search_files_error_keeps_the_matches(tmp_path: Path) -> None:
    """grep exits 2 when a file cannot be read; the matches it found are kept."""
    proc = MagicMock()
    proc.communicate = AsyncMock(return_value=(b"a.py:1:needle\n", b"grep: b.py: Permission denied\n"))
    proc.returncode = 2
    with patch("codeforge.tools.search_files.start_tool_process", AsyncMock(return_value=proc)):
        result = await SearchFilesTool().execute({"pattern": "needle"}, str(tmp_path))

    assert result.success is False
    assert result.error == "grep: b.py: Permission denied"
    assert result.output == "a.py:1:needle"
