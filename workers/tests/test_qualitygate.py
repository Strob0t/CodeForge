"""Tests for the quality gate executor."""

from __future__ import annotations

import asyncio
from collections import OrderedDict
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.models import QualityGateRequest, QualityGateResult
from codeforge.qualitygate import QualityGateExecutor

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
    with patch(_SPAWN, return_value=hanging):
        result = await short_executor.execute(request)

    assert result.tests_passed is False
    assert "timed out" in result.test_output


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
