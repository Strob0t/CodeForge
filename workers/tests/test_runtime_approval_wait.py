"""The worker waits for a tool call decision at least as long as Go waits for a HITL approval (KI-21).

A call the policy resolves to "ask" is answered only after a human decided or
Go's approval timeout expired. The worker used to give up after a fixed 30 s
while Go waited 60 s, so approvals given in between were lost. Go now sends its
timeout with the run start; the tests scale the times down.
"""

from __future__ import annotations

import asyncio
import json

import pytest

from codeforge.constants import APPROVAL_RESPONSE_MARGIN_SECONDS, DEFAULT_APPROVAL_TIMEOUT_SECONDS
from codeforge.models import TerminationConfig
from codeforge.runtime import RuntimeClient, policy_response_timeout
from tests.jetstream_fakes import RecordingJetStream


@pytest.mark.parametrize(
    ("approval_timeout", "expected"),
    [
        (0, DEFAULT_APPROVAL_TIMEOUT_SECONDS + APPROVAL_RESPONSE_MARGIN_SECONDS),
        (-1, DEFAULT_APPROVAL_TIMEOUT_SECONDS + APPROVAL_RESPONSE_MARGIN_SECONDS),
        (1, 1 + APPROVAL_RESPONSE_MARGIN_SECONDS),
        (300, 300 + APPROVAL_RESPONSE_MARGIN_SECONDS),
    ],
)
def test_policy_wait_outlasts_the_go_approval_timeout(approval_timeout: int, expected: float) -> None:
    assert policy_response_timeout(approval_timeout) == expected


def test_default_wait_outlasts_the_go_default() -> None:
    """Without a timeout from Go (older core) the worker still outwaits Go's default of 60 s."""
    assert DEFAULT_APPROVAL_TIMEOUT_SECONDS == 60
    assert APPROVAL_RESPONSE_MARGIN_SECONDS > 0
    assert policy_response_timeout(0) > 60


def _runtime(js: RecordingJetStream, approval_timeout: float) -> RuntimeClient:
    return RuntimeClient(
        js=js,  # type: ignore[arg-type]
        run_id="run-1",
        task_id="task-1",
        project_id="proj-1",
        termination=TerminationConfig(),
        approval_timeout_seconds=approval_timeout,
    )


async def _answer_later(js: RecordingJetStream, delay: float, decision: str) -> None:
    """Answer the pending tool call after *delay* seconds, as Go does once the human decided."""
    while not js.published:
        await asyncio.sleep(0.005)
    request = json.loads(js.published[0][1])
    await asyncio.sleep(delay)
    response = {"run_id": request["run_id"], "call_id": request["call_id"], "decision": decision}
    js.subscriptions[0].deliver(json.dumps(response).encode())


async def test_late_approval_reaches_the_worker(monkeypatch: pytest.MonkeyPatch) -> None:
    """Go approval timeout 0.4 s (scaled 60 s): an approval after 0.3 s (past the old 30 s) is used."""
    monkeypatch.setattr("codeforge.runtime.APPROVAL_RESPONSE_MARGIN_SECONDS", 0.1)
    js = RecordingJetStream()
    runtime = _runtime(js, approval_timeout=0.4)

    answer = asyncio.create_task(_answer_later(js, 0.3, "allow"))
    decision = await runtime.request_tool_call(tool="bash", command="rm -rf build")
    await answer

    assert decision.decision == "allow"


async def test_no_decision_within_the_go_timeout_denies(monkeypatch: pytest.MonkeyPatch) -> None:
    """Go denies at its timeout; if even that answer is lost, the worker denies after timeout + margin."""
    monkeypatch.setattr("codeforge.runtime.APPROVAL_RESPONSE_MARGIN_SECONDS", 0.05)
    js = RecordingJetStream()
    runtime = _runtime(js, approval_timeout=0.1)

    loop = asyncio.get_running_loop()
    started = loop.time()
    decision = await runtime.request_tool_call(tool="bash", command="rm -rf build")
    waited = loop.time() - started

    assert decision.decision == "deny"
    assert "timeout" in decision.reason
    assert waited >= 0.15
    assert js.subscriptions[0].unsubscribed
