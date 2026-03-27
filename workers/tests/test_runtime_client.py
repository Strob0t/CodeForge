"""Tests for RuntimeClient NATS protocol interactions.

Validates that RuntimeClient publishes to the correct NATS subjects
and handles responses properly, using AsyncMock for JetStreamContext.
"""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

from codeforge.models import TerminationConfig
from codeforge.nats_subjects import (
    SUBJECT_AGENT_OUTPUT,
    SUBJECT_RUN_COMPLETE,
    SUBJECT_RUN_OUTPUT,
    SUBJECT_TOOLCALL_REQUEST,
    SUBJECT_TOOLCALL_RESULT,
    SUBJECT_TRAJECTORY_EVENT,
)
from codeforge.runtime import RuntimeClient


def _make_js_mock() -> MagicMock:
    """Create a mock JetStreamContext with publish as AsyncMock."""
    js = MagicMock()
    js.publish = AsyncMock()
    js.subscribe = AsyncMock()
    return js


def _make_client(js: MagicMock | None = None) -> RuntimeClient:
    """Create a RuntimeClient with a mock JetStream."""
    if js is None:
        js = _make_js_mock()
    return RuntimeClient(
        js=js,
        run_id="run-123",
        task_id="task-456",
        project_id="proj-789",
        termination=TerminationConfig(max_steps=50, timeout_seconds=600, max_cost=5.0),
    )


# ---------------------------------------------------------------------------
# send_output
# ---------------------------------------------------------------------------


async def test_send_output_publishes_to_run_output_subject() -> None:
    """send_output publishes a JSON payload to the runs.output subject."""
    js = _make_js_mock()
    client = _make_client(js)

    await client.send_output("hello world")

    # send_output publishes to both SUBJECT_RUN_OUTPUT and SUBJECT_AGENT_OUTPUT
    assert js.publish.call_count == 2

    # First call is to runs.output
    first_call = js.publish.call_args_list[0]
    subject = first_call.args[0]
    payload = json.loads(first_call.args[1])

    assert subject == SUBJECT_RUN_OUTPUT
    assert payload["run_id"] == "run-123"
    assert payload["task_id"] == "task-456"
    assert payload["line"] == "hello world"
    assert payload["stream"] == "stdout"


async def test_send_output_mirrors_to_agent_output() -> None:
    """send_output also publishes to agents.output for WS broadcast."""
    js = _make_js_mock()
    client = _make_client(js)

    await client.send_output("test line", stream="stderr")

    # Second call is to agents.output
    second_call = js.publish.call_args_list[1]
    subject = second_call.args[0]
    payload = json.loads(second_call.args[1])

    assert subject == SUBJECT_AGENT_OUTPUT
    assert payload["task_id"] == "task-456"
    assert payload["line"] == "test line"
    assert payload["stream"] == "stderr"


# ---------------------------------------------------------------------------
# report_tool_result
# ---------------------------------------------------------------------------


async def test_report_tool_result_publishes_to_toolcall_result() -> None:
    """report_tool_result publishes to runs.toolcall.result with correct payload."""
    js = _make_js_mock()
    client = _make_client(js)

    await client.report_tool_result(
        call_id="call-1",
        tool="bash",
        success=True,
        output="command output",
        cost_usd=0.01,
        tokens_in=100,
        tokens_out=50,
        model="test-model",
    )

    js.publish.assert_called_once()
    subject = js.publish.call_args.args[0]
    payload = json.loads(js.publish.call_args.args[1])

    assert subject == SUBJECT_TOOLCALL_RESULT
    assert payload["run_id"] == "run-123"
    assert payload["call_id"] == "call-1"
    assert payload["tool"] == "bash"
    assert payload["success"] is True
    assert payload["output"] == "command output"
    assert payload["cost_usd"] == 0.01
    assert payload["tokens_in"] == 100
    assert payload["tokens_out"] == 50
    assert payload["model"] == "test-model"


async def test_report_tool_result_increments_step_count() -> None:
    """Each report_tool_result call increments the internal step counter."""
    js = _make_js_mock()
    client = _make_client(js)

    assert client.step_count == 0
    await client.report_tool_result(call_id="c1", tool="t1", success=True)
    assert client.step_count == 1
    await client.report_tool_result(call_id="c2", tool="t2", success=True)
    assert client.step_count == 2


async def test_report_tool_result_accumulates_cost() -> None:
    """Costs are accumulated across multiple report_tool_result calls."""
    js = _make_js_mock()
    client = _make_client(js)

    assert client.total_cost == 0.0
    await client.report_tool_result(call_id="c1", tool="t1", success=True, cost_usd=0.01)
    await client.report_tool_result(call_id="c2", tool="t2", success=True, cost_usd=0.02)
    assert abs(client.total_cost - 0.03) < 1e-9


async def test_report_tool_result_includes_diff_when_provided() -> None:
    """When diff is provided, it is included in the published payload."""
    js = _make_js_mock()
    client = _make_client(js)
    diff = {"file": "test.py", "hunks": [{"old_start": 1, "new_start": 1}]}

    await client.report_tool_result(call_id="c1", tool="edit", success=True, diff=diff)

    payload = json.loads(js.publish.call_args.args[1])
    assert payload["diff"] == diff


async def test_report_tool_result_omits_diff_when_none() -> None:
    """When diff is None, it is not included in the published payload."""
    js = _make_js_mock()
    client = _make_client(js)

    await client.report_tool_result(call_id="c1", tool="read", success=True)

    payload = json.loads(js.publish.call_args.args[1])
    assert "diff" not in payload


# ---------------------------------------------------------------------------
# request_tool_call
# ---------------------------------------------------------------------------


async def test_request_tool_call_publishes_request_and_returns_decision() -> None:
    """request_tool_call publishes to runs.toolcall.request and waits for response."""
    js = _make_js_mock()

    # Mock the subscription to return a matching response
    mock_sub = AsyncMock()
    # We need call_id to match dynamically, so we use side_effect
    published_call_id = None

    async def _capture_publish(subject, data):
        nonlocal published_call_id
        if subject == SUBJECT_TOOLCALL_REQUEST:
            published_call_id = json.loads(data)["call_id"]
            # Update the mock subscription response with the correct call_id
            mock_sub.next_msg.return_value = MagicMock(
                data=json.dumps({"call_id": published_call_id, "decision": "allow", "reason": ""}).encode()
            )

    js.publish = AsyncMock(side_effect=_capture_publish)
    js.subscribe = AsyncMock(return_value=mock_sub)
    mock_sub.unsubscribe = AsyncMock()

    client = _make_client(js)
    decision = await client.request_tool_call(tool="bash", command="ls -la")

    assert decision.decision == "allow"
    assert decision.call_id is not None
    assert decision.call_id != ""


async def test_request_tool_call_when_cancelled_returns_deny() -> None:
    """When the run is cancelled, request_tool_call immediately returns deny."""
    js = _make_js_mock()
    client = _make_client(js)
    client._cancelled = True

    decision = await client.request_tool_call(tool="bash", command="ls")

    assert decision.decision == "deny"
    assert "cancelled" in decision.reason
    # No NATS publish should have been attempted
    js.publish.assert_not_called()


# ---------------------------------------------------------------------------
# publish_trajectory_event
# ---------------------------------------------------------------------------


async def test_publish_trajectory_event_correct_subject() -> None:
    """publish_trajectory_event publishes to runs.trajectory.event."""
    js = _make_js_mock()
    client = _make_client(js)

    event = {"event_type": "agent.step_done", "model": "test-model"}
    await client.publish_trajectory_event(event)

    js.publish.assert_called_once()
    subject = js.publish.call_args.args[0]
    payload = json.loads(js.publish.call_args.args[1])

    assert subject == SUBJECT_TRAJECTORY_EVENT
    assert payload["event_type"] == "agent.step_done"
    assert payload["run_id"] == "run-123"
    assert payload["project_id"] == "proj-789"


async def test_publish_trajectory_event_adds_run_and_project_id() -> None:
    """The event is enriched with run_id and project_id from the client."""
    js = _make_js_mock()
    client = _make_client(js)

    await client.publish_trajectory_event({"event_type": "custom"})

    payload = json.loads(js.publish.call_args.args[1])
    assert payload["run_id"] == "run-123"
    assert payload["project_id"] == "proj-789"


async def test_publish_trajectory_event_swallows_publish_errors() -> None:
    """Errors during trajectory event publishing are silently caught (best-effort)."""
    js = _make_js_mock()
    js.publish = AsyncMock(side_effect=Exception("NATS down"))
    client = _make_client(js)

    # Should not raise
    await client.publish_trajectory_event({"event_type": "test"})


# ---------------------------------------------------------------------------
# complete_run
# ---------------------------------------------------------------------------


async def test_complete_run_publishes_to_run_complete() -> None:
    """complete_run publishes to runs.complete with accumulated metrics."""
    js = _make_js_mock()
    client = _make_client(js)

    # Simulate some work
    await client.report_tool_result(call_id="c1", tool="t1", success=True, cost_usd=0.05, tokens_in=200, tokens_out=100)

    await client.complete_run(status="completed", output="result text")

    # Last publish should be to SUBJECT_RUN_COMPLETE
    last_call = js.publish.call_args_list[-1]
    subject = last_call.args[0]
    payload = json.loads(last_call.args[1])

    assert subject == SUBJECT_RUN_COMPLETE
    assert payload["run_id"] == "run-123"
    assert payload["task_id"] == "task-456"
    assert payload["project_id"] == "proj-789"
    assert payload["status"] == "completed"
    assert payload["output"] == "result text"
    assert payload["cost_usd"] == 0.05
    assert payload["step_count"] == 1
    assert payload["tokens_in"] == 200
    assert payload["tokens_out"] == 100


async def test_complete_run_with_error_status() -> None:
    """complete_run can report an error status."""
    js = _make_js_mock()
    client = _make_client(js)

    await client.complete_run(status="failed", error="something went wrong")

    last_call = js.publish.call_args_list[-1]
    payload = json.loads(last_call.args[1])

    assert payload["status"] == "failed"
    assert payload["error"] == "something went wrong"


# ---------------------------------------------------------------------------
# is_cancelled property
# ---------------------------------------------------------------------------


async def test_is_cancelled_defaults_to_false() -> None:
    """A fresh RuntimeClient is not cancelled."""
    client = _make_client()
    assert client.is_cancelled is False


async def test_is_cancelled_reflects_internal_state() -> None:
    """Setting _cancelled changes the is_cancelled property."""
    client = _make_client()
    client._cancelled = True
    assert client.is_cancelled is True
