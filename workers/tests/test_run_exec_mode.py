"""The worker runs every tool as a local process, so it must refuse runs that ask
for sandbox or hybrid isolation instead of silently running them unisolated (KI-13).

Go rejects these runs at start; this guard covers run starts that bypass Go
(handoffs, messages queued before an upgrade).
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer import TaskConsumer
from codeforge.models import RunStartMessage
from codeforge.nats_subjects import SUBJECT_RUN_COMPLETE


@pytest.fixture
def consumer() -> TaskConsumer:
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    consumer._js = AsyncMock()
    consumer._executor = MagicMock()
    consumer._executor.execute_with_runtime = AsyncMock()
    return consumer


def _run_start_msg(exec_mode: str) -> MagicMock:
    run_msg = RunStartMessage(
        run_id=f"run-{exec_mode or 'empty'}",
        task_id="task-1",
        project_id="proj-1",
        agent_id="agent-1",
        prompt="run the tests",
        exec_mode=exec_mode,
    )
    msg = MagicMock()
    msg.data = run_msg.model_dump_json().encode()
    msg.headers = None
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.ack_sync = AsyncMock()
    return msg


def _published(consumer: TaskConsumer, subject: str) -> list[dict[str, object]]:
    assert consumer._js is not None
    return [json.loads(call.args[1]) for call in consumer._js.publish.call_args_list if call.args[0] == subject]


@pytest.mark.parametrize("exec_mode", ["sandbox", "hybrid", "Sandbox", " hybrid", "docker"])
async def test_run_start_refuses_exec_modes_without_isolation(consumer: TaskConsumer, exec_mode: str) -> None:
    msg = _run_start_msg(exec_mode)

    await consumer._handle_run_start(msg)

    consumer._executor.execute_with_runtime.assert_not_called()
    completions = _published(consumer, SUBJECT_RUN_COMPLETE)
    assert len(completions) == 1
    assert completions[0]["status"] == "failed"
    assert "not available yet: tools would run without isolation (KI-13)" in str(completions[0]["error"])
    # The run is accepted (acked) and then rejected; a nak would redeliver it forever.
    msg.ack_sync.assert_awaited_once()  # accepted with a confirmed ack (ADR-016)
    msg.nak.assert_not_called()


@pytest.mark.parametrize("exec_mode", ["mount", ""])
async def test_run_start_executes_mount_runs(consumer: TaskConsumer, exec_mode: str) -> None:
    msg = _run_start_msg(exec_mode)

    await consumer._handle_run_start(msg)

    consumer._executor.execute_with_runtime.assert_called_once()
    assert _published(consumer, SUBJECT_RUN_COMPLETE) == []
    msg.ack_sync.assert_awaited_once()  # accepted with a confirmed ack (ADR-016)
