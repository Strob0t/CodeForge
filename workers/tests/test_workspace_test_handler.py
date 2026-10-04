"""Tests for the auto-agent's workspace test run in the worker (KI-81).

The Go Core no longer runs pytest itself: it publishes conversation.test.request
and the worker runs the test file with its tool environment, in its own process
group, bounded by the request's timeout, and publishes conversation.test.result.
"""

from __future__ import annotations

import os
from collections import OrderedDict
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.models import WorkspaceTestRequest, WorkspaceTestResult
from codeforge.nats_subjects import SUBJECT_CONVERSATION_TEST_REQUEST, SUBJECT_CONVERSATION_TEST_RESULT

if TYPE_CHECKING:
    from pathlib import Path

_SPAWN = "codeforge.qualitygate.asyncio.create_subprocess_exec"


def _proc(output: str, returncode: int) -> MagicMock:
    proc = MagicMock()
    proc.communicate = AsyncMock(return_value=(output.encode(), None))
    proc.returncode = returncode
    return proc


@pytest.fixture
def consumer(monkeypatch: pytest.MonkeyPatch) -> TaskConsumer:
    monkeypatch.setattr(ConsumerBaseMixin, "_processed_ids", OrderedDict())
    return TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")


def _request(workspace: Path, test_file: str = "test_feature.py", **kw: object) -> WorkspaceTestRequest:
    return WorkspaceTestRequest(
        request_id="req-1",
        tenant_id="tenant-1",
        project_id="proj-1",
        conversation_id="conv-1",
        workspace_path=str(workspace),
        test_file=test_file,
        timeout_seconds=kw.pop("timeout_seconds", 30),  # type: ignore[arg-type]
    )


async def _handle(consumer: TaskConsumer, request: WorkspaceTestRequest) -> tuple[WorkspaceTestResult, MagicMock]:
    msg = MagicMock()
    msg.data = request.model_dump_json().encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.term = AsyncMock()
    msg.in_progress = AsyncMock()
    msg.is_acked = False
    consumer._js = AsyncMock()
    await consumer._handle_workspace_test(msg)
    calls = [c for c in consumer._js.publish.call_args_list if c.args[0] == SUBJECT_CONVERSATION_TEST_RESULT]
    assert len(calls) == 1, consumer._js.publish.call_args_list
    return WorkspaceTestResult.model_validate_json(calls[0].args[1]), msg


def test_subject_is_consumed(consumer: TaskConsumer) -> None:
    """The worker subscribes to the request subject."""
    assert hasattr(consumer, "_handle_workspace_test")
    assert SUBJECT_CONVERSATION_TEST_REQUEST == "conversation.test.request"


@pytest.mark.parametrize(("returncode", "want"), [(0, True), (1, False)])
async def test_runs_pytest_in_the_workspace(
    consumer: TaskConsumer, tmp_path: Path, monkeypatch: pytest.MonkeyPatch, returncode: int, want: bool
) -> None:
    monkeypatch.setenv("LITELLM_MASTER_KEY", "sk-secret")
    (tmp_path / "test_feature.py").write_text("def test_x(): pass\n")
    with patch(_SPAWN, return_value=_proc("=== 3 passed ===", returncode)) as spawn:
        result, msg = await _handle(consumer, _request(tmp_path))

    assert spawn.call_args.args == ("python", "-m", "pytest", "test_feature.py", "-v", "--tb=short")
    assert spawn.call_args.kwargs["cwd"] == str(tmp_path)
    assert spawn.call_args.kwargs["start_new_session"] is True
    env = spawn.call_args.kwargs["env"]
    assert "LITELLM_MASTER_KEY" not in env
    assert result.passed is want
    assert "3 passed" in result.output
    assert result.request_id == "req-1"
    assert result.tenant_id == "tenant-1"
    assert result.conversation_id == "conv-1"
    msg.ack.assert_called_once()


@pytest.mark.parametrize(
    "test_file",
    ["../test_x.py", "test_x.py; rm -rf /", "sub/test_x.py", "conftest.py", "test_missing.py", "test_link.py"],
)
async def test_refuses_files_it_must_not_run(consumer: TaskConsumer, tmp_path: Path, test_file: str) -> None:
    outside = tmp_path.parent / "outside_test.py"
    outside.write_text("def test_x(): pass\n")
    os.symlink(outside, tmp_path / "test_link.py")
    with patch(_SPAWN) as spawn:
        result, msg = await _handle(consumer, _request(tmp_path, test_file=test_file))

    spawn.assert_not_called()
    assert result.passed is None
    assert result.error
    msg.ack.assert_called_once()


async def test_timeout_reports_no_verdict(consumer: TaskConsumer, tmp_path: Path) -> None:
    (tmp_path / "test_feature.py").write_text("import time\ndef test_x(): time.sleep(60)\n")
    request = _request(tmp_path, timeout_seconds=1)
    with patch(
        "codeforge.qualitygate.QualityGateExecutor.run_command",
        new=AsyncMock(return_value=(None, "command timed out after 1s")),
    ) as run:
        result, _ = await _handle(consumer, request)

    assert run.await_args.args[3] == 1  # the request's timeout
    assert result.passed is None
    assert "timed out" in result.error


async def test_duplicate_request_runs_once(consumer: TaskConsumer, tmp_path: Path) -> None:
    (tmp_path / "test_feature.py").write_text("def test_x(): pass\n")
    with patch(_SPAWN, return_value=_proc("1 passed", 0)) as spawn:
        await _handle(consumer, _request(tmp_path))
        msg = MagicMock()
        msg.data = _request(tmp_path).model_dump_json().encode()
        msg.ack = AsyncMock()
        await consumer._handle_workspace_test(msg)

    assert spawn.call_count == 1
    msg.ack.assert_called_once()


async def test_large_output_is_capped_to_its_tail(consumer: TaskConsumer, tmp_path: Path) -> None:
    """S3-F review C7: output above the NATS max payload made the publish fail
    (and the tests re-run up to MaxDeliver times). The result keeps the tail,
    about 64 KiB, behind a truncation marker."""
    (tmp_path / "test_feature.py").write_text("def test_x(): pass\n")
    head = "early line é\n" * 50_000  # ~650 KB, multi-byte characters
    tail = "FAILED test_feature.py::test_x - assert 1 == 2\n=== 1 failed ===\n"
    with patch(_SPAWN, return_value=_proc(head + tail, 1)):
        result, msg = await _handle(consumer, _request(tmp_path))

    assert result.passed is False
    assert len(result.output.encode()) <= 64 * 1024 + 200
    assert result.output.endswith(tail)
    assert "truncated" in result.output.splitlines()[0]
    msg.ack.assert_awaited_once()


async def test_small_output_is_kept_whole(consumer: TaskConsumer, tmp_path: Path) -> None:
    (tmp_path / "test_feature.py").write_text("def test_x(): pass\n")
    with patch(_SPAWN, return_value=_proc("=== 3 passed ===", 0)):
        result, _ = await _handle(consumer, _request(tmp_path))
    assert result.output == "=== 3 passed ==="
