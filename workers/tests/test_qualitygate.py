"""Tests for the quality gate executor."""

from __future__ import annotations

import asyncio
import os
import shlex
import signal
import sys
import time
from collections import OrderedDict
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.models import QualityGateRequest, QualityGateResult
from codeforge.qualitygate import QualityGateExecutor

if TYPE_CHECKING:
    from pathlib import Path

_SPAWN = "codeforge.qualitygate.asyncio.create_subprocess_exec"


def _proc(output: str, returncode: int) -> MagicMock:
    """Fake finished subprocess, so the tests need no gate tool binaries installed."""
    proc = MagicMock()
    proc.communicate = AsyncMock(return_value=(output.encode(), None))
    proc.returncode = returncode
    return proc


@pytest.fixture
def executor() -> QualityGateExecutor:
    """Create a QualityGateExecutor with short timeout for tests."""
    return QualityGateExecutor(timeout_seconds=5)


@pytest.fixture
def consumer(monkeypatch: pytest.MonkeyPatch) -> TaskConsumer:
    """Create a TaskConsumer for testing, with an empty class-level dedup cache."""
    monkeypatch.setattr(ConsumerBaseMixin, "_processed_ids", OrderedDict())
    return TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")


async def test_execute_tests_pass(executor: QualityGateExecutor) -> None:
    """Execute should report tests_passed=True when command exits 0."""
    request = QualityGateRequest(
        run_id="run-1",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=True,
        run_lint=False,
        test_command="pytest -q 'tests/unit'",
    )
    with patch(_SPAWN, return_value=_proc("tests pass", 0)) as spawn:
        result = await executor.execute(request)

    assert spawn.call_args.args == ("pytest", "-q", "tests/unit")
    assert spawn.call_args.kwargs["cwd"] == "/tmp"
    assert result.run_id == "run-1"
    assert result.tests_passed is True
    assert result.lint_passed is None


async def test_execute_tests_fail(executor: QualityGateExecutor) -> None:
    """Execute should report tests_passed=False when command exits non-zero."""
    request = QualityGateRequest(
        run_id="run-2",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=True,
        run_lint=False,
        test_command="pytest",
    )
    with patch(_SPAWN, return_value=_proc("1 failed", 1)):
        result = await executor.execute(request)

    assert result.tests_passed is False
    assert "1 failed" in result.test_output


async def test_execute_rejects_command_not_on_allowlist(executor: QualityGateExecutor) -> None:
    """Commands whose executable is not on the allowlist must never be spawned."""
    request = QualityGateRequest(
        run_id="run-2b",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=True,
        run_lint=False,
        test_command="echo 'tests pass'",
    )
    with patch(_SPAWN) as spawn:
        result = await executor.execute(request)

    spawn.assert_not_called()
    assert result.tests_passed is False
    assert "not allowed" in result.test_output


async def test_execute_lint_pass(executor: QualityGateExecutor) -> None:
    """Execute should report lint_passed=True when lint command exits 0."""
    request = QualityGateRequest(
        run_id="run-3",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=False,
        run_lint=True,
        lint_command="ruff check .",
    )
    with patch(_SPAWN, return_value=_proc("All checks passed!", 0)):
        result = await executor.execute(request)

    assert result.lint_passed is True
    assert result.tests_passed is None


async def test_execute_combined(executor: QualityGateExecutor) -> None:
    """Execute should run both tests and lint when both are requested."""
    request = QualityGateRequest(
        run_id="run-4",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=True,
        run_lint=True,
        test_command="pytest",
        lint_command="ruff check .",
    )
    with patch(_SPAWN, side_effect=[_proc("tests ok", 0), _proc("lint ok", 0)]):
        result = await executor.execute(request)

    assert result.tests_passed is True
    assert result.lint_passed is True
    assert "tests ok" in result.test_output
    assert "lint ok" in result.lint_output


async def test_execute_timeout(executor: QualityGateExecutor) -> None:
    """Execute should handle command timeout gracefully."""
    short_executor = QualityGateExecutor(timeout_seconds=1)
    request = QualityGateRequest(
        run_id="run-5",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=True,
        run_lint=False,
        test_command="pytest",
    )

    async def never_finishes() -> tuple[bytes, None]:
        await asyncio.Event().wait()
        return b"", None

    hanging = _proc("", 0)
    hanging.communicate = never_finishes
    hanging.pid = object()  # a fake process: its "group" must only reach the patched killpg
    hanging.wait = AsyncMock(return_value=-9)
    with patch(_SPAWN, return_value=hanging) as spawn, patch("codeforge.qualitygate.os.killpg") as killpg:
        result = await short_executor.execute(request)

    assert result.tests_passed is False
    assert "timed out" in result.test_output
    assert spawn.call_args.kwargs["start_new_session"] is True
    killpg.assert_called_once_with(hanging.pid, signal.SIGKILL)
    hanging.wait.assert_awaited_once()


def _process_gone(pid: int) -> bool:
    """True when pid no longer runs (exited, or a zombie nobody reaped yet)."""
    try:
        with open(f"/proc/{pid}/stat") as f:
            state = f.read().rsplit(")", 1)[1].split()[0]
    except FileNotFoundError:
        return True
    return state in {"Z", "X"}


async def test_execute_timeout_kills_the_whole_process_group(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """A command that outlives the request's timeout is killed with everything it started (KI-28).

    The command starts a grandchild that inherits its output pipe and ignores
    the command's end; only killing the process group ends it, and until then
    the gate could not even read the command's output to its end.
    """
    monkeypatch.setenv("PATH", f"{os.path.dirname(sys.executable)}{os.pathsep}{os.environ['PATH']}")
    pid_file = tmp_path / "grandchild.pid"
    script = (
        "import subprocess, sys, time; "
        "p = subprocess.Popen([sys.executable, '-c', 'import signal, time; signal.signal(signal.SIGTERM, signal.SIG_IGN); time.sleep(60)']); "
        f"open({str(pid_file)!r}, 'w').write(str(p.pid)); "
        "time.sleep(60)"
    )
    request = QualityGateRequest(
        run_id="run-timeout",
        project_id="proj-1",
        workspace_path=str(tmp_path),
        run_tests=True,
        test_command=f"python -c {shlex.quote(script)}",
        timeout_seconds=1,
    )
    executor = QualityGateExecutor(timeout_seconds=30)  # the request's timeout wins

    started = time.monotonic()
    result = await executor.execute(request)
    elapsed = time.monotonic() - started

    assert result.tests_passed is False
    assert "timed out after 1s" in result.test_output
    assert elapsed < 15, f"the gate took {elapsed:.1f}s, the request's 1s timeout was not applied"
    grandchild = int(pid_file.read_text())
    deadline = time.monotonic() + 5
    while not _process_gone(grandchild) and time.monotonic() < deadline:
        await asyncio.sleep(0.05)
    assert _process_gone(grandchild), "the command's grandchild survived the timeout"


async def test_execute_without_request_timeout_uses_the_default(executor: QualityGateExecutor) -> None:
    """A request without timeout_seconds (an older Go Core) keeps the worker's default."""
    request = QualityGateRequest(
        run_id="run-default",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=True,
        test_command="pytest",
    )
    assert request.timeout_seconds == 0
    with (
        patch("codeforge.qualitygate.asyncio.wait_for", wraps=asyncio.wait_for) as wait_for,
        patch(_SPAWN, return_value=_proc("ok", 0)),
    ):
        result = await executor.execute(request)

    assert result.tests_passed is True
    assert wait_for.call_args.kwargs["timeout"] == 5  # the fixture executor's default


async def test_execute_no_commands(executor: QualityGateExecutor) -> None:
    """Execute should skip when neither tests nor lint is requested."""
    request = QualityGateRequest(
        run_id="run-6",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=False,
        run_lint=False,
    )
    result = await executor.execute(request)

    assert result.tests_passed is None
    assert result.lint_passed is None


@pytest.mark.parametrize(
    ("run_tests", "run_lint", "test_command", "lint_command"),
    [
        (True, False, "", ""),
        (False, True, "", ""),
        (True, True, "", "ruff check ."),
        (True, True, "pytest", "   "),
    ],
)
async def test_execute_requested_check_without_command_fails(
    executor: QualityGateExecutor, run_tests: bool, run_lint: bool, test_command: str, lint_command: str
) -> None:
    """A requested check without a command fails instead of being skipped (KI-29)."""
    request = QualityGateRequest(
        run_id="run-7",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=run_tests,
        run_lint=run_lint,
        test_command=test_command,
        lint_command=lint_command,
    )
    with patch(_SPAWN, return_value=_proc("ok", 0)):
        result = await executor.execute(request)

    for requested, command, passed, output in (
        (run_tests, test_command, result.tests_passed, result.test_output),
        (run_lint, lint_command, result.lint_passed, result.lint_output),
    ):
        if not requested:
            assert passed is None
        elif command.strip():
            assert passed is True
        else:
            assert passed is False
            assert "no command" in output


async def test_handle_quality_gate_message(consumer: TaskConsumer) -> None:
    """Consumer should parse quality gate request and publish result."""
    request = QualityGateRequest(
        run_id="run-qg",
        project_id="proj-1",
        workspace_path="/tmp",
        run_tests=True,
        run_lint=False,
        test_command="pytest",
    )

    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    consumer._js = AsyncMock()

    with patch(_SPAWN, return_value=_proc("pass", 0)):
        await consumer._handle_quality_gate(msg)

    # Should publish result
    consumer._js.publish.assert_called_once()
    call_args = consumer._js.publish.call_args
    assert call_args.args[0] == "runs.qualitygate.result"

    result = QualityGateResult.model_validate_json(call_args.args[1])
    assert result.run_id == "run-qg"
    assert result.tests_passed is True

    msg.ack.assert_called_once()
    msg.nak.assert_not_called()
