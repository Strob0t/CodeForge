"""Tests for the runtime client (step-by-step execution protocol)."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.models import (
    RunStartMessage,
    TerminationConfig,
    ToolCallDecision,
)
from codeforge.runtime import (
    SUBJECT_RUN_COMPLETE,
    SUBJECT_RUN_OUTPUT,
    SUBJECT_TOOLCALL_REQUEST,
    SUBJECT_TOOLCALL_RESULT,
    RuntimeClient,
)


@pytest.fixture
def mock_js() -> AsyncMock:
    """Create a mock JetStream context."""
    js = AsyncMock()
    return js


@pytest.fixture
def runtime(mock_js: AsyncMock) -> RuntimeClient:
    """Create a RuntimeClient for testing."""
    return RuntimeClient(
        js=mock_js,
        run_id="run-1",
        task_id="task-1",
        project_id="proj-1",
        termination=TerminationConfig(max_steps=50, timeout_seconds=600, max_cost=5.0),
    )


async def test_request_tool_call_publishes(runtime: RuntimeClient, mock_js: AsyncMock) -> None:
    """request_tool_call should publish a request to NATS."""
    # Mock the subscribe to return a response
    sub = AsyncMock()
    response_data = json.dumps(
        {
            "run_id": "run-1",
            "call_id": "",  # Will be overwritten
            "decision": "allow",
            "reason": "",
        }
    ).encode()

    async def next_msg_side_effect(timeout: float = 1.0) -> MagicMock:
        msg = MagicMock()
        # Get the call_id from the published request
        publish_call = mock_js.publish.call_args
        if publish_call:
            req_data = json.loads(publish_call.args[1])
            msg.data = json.dumps(
                {
                    "run_id": "run-1",
                    "call_id": req_data["call_id"],
                    "decision": "allow",
                    "reason": "",
                }
            ).encode()
        else:
            msg.data = response_data
        return msg

    sub.next_msg = next_msg_side_effect
    sub.unsubscribe = AsyncMock()
    mock_js.subscribe.return_value = sub

    decision = await runtime.request_tool_call(tool="Read", path="main.go")

    assert decision.decision == "allow"
    assert decision.call_id != ""

    # Verify request was published
    mock_js.publish.assert_called_once()
    call_args = mock_js.publish.call_args
    assert call_args.args[0] == SUBJECT_TOOLCALL_REQUEST
    req = json.loads(call_args.args[1])
    assert req["tool"] == "Read"
    assert req["path"] == "main.go"


async def test_request_tool_call_denied(runtime: RuntimeClient, mock_js: AsyncMock) -> None:
    """request_tool_call should return deny when policy denies."""
    sub = AsyncMock()

    async def next_msg_side_effect(timeout: float = 1.0) -> MagicMock:
        publish_call = mock_js.publish.call_args
        req_data = json.loads(publish_call.args[1])
        msg = MagicMock()
        msg.data = json.dumps(
            {
                "run_id": "run-1",
                "call_id": req_data["call_id"],
                "decision": "deny",
                "reason": "not allowed",
            }
        ).encode()
        return msg

    sub.next_msg = next_msg_side_effect
    sub.unsubscribe = AsyncMock()
    mock_js.subscribe.return_value = sub

    decision = await runtime.request_tool_call(tool="Bash", command="rm -rf /")

    assert decision.decision == "deny"
    assert decision.reason == "not allowed"


async def test_request_tool_call_cancelled(runtime: RuntimeClient, mock_js: AsyncMock) -> None:
    """request_tool_call should immediately return deny when cancelled."""
    runtime._cancelled = True

    decision = await runtime.request_tool_call(tool="Read", path="file.go")

    assert decision.decision == "deny"
    assert "cancelled" in decision.reason
    mock_js.publish.assert_not_called()


async def test_report_tool_result(runtime: RuntimeClient, mock_js: AsyncMock) -> None:
    """report_tool_result should publish result and update counters."""
    await runtime.report_tool_result(
        call_id="call-1",
        tool="Read",
        success=True,
        output="file contents",
        cost_usd=0.005,
    )

    assert runtime.step_count == 1
    assert runtime.total_cost == pytest.approx(0.005)

    mock_js.publish.assert_called_once()
    call_args = mock_js.publish.call_args
    assert call_args.args[0] == SUBJECT_TOOLCALL_RESULT
    result = json.loads(call_args.args[1])
    assert result["call_id"] == "call-1"
    assert result["success"] is True
    assert result["cost_usd"] == pytest.approx(0.005)


async def test_report_tool_result_accumulates(runtime: RuntimeClient, mock_js: AsyncMock) -> None:
    """Multiple report_tool_result calls should accumulate steps and cost."""
    await runtime.report_tool_result(call_id="c1", tool="Edit", success=True, cost_usd=0.01)
    await runtime.report_tool_result(call_id="c2", tool="Write", success=True, cost_usd=0.02)
    await runtime.report_tool_result(call_id="c3", tool="Bash", success=False, error="oops", cost_usd=0.005)

    assert runtime.step_count == 3
    assert runtime.total_cost == pytest.approx(0.035)


async def test_complete_run(runtime: RuntimeClient, mock_js: AsyncMock) -> None:
    """complete_run should publish a completion message."""
    runtime._step_count = 5
    runtime._total_cost = 0.05

    await runtime.complete_run(status="completed", output="all done")

    mock_js.publish.assert_called_once()
    call_args = mock_js.publish.call_args
    assert call_args.args[0] == SUBJECT_RUN_COMPLETE
    data = json.loads(call_args.args[1])
    assert data["run_id"] == "run-1"
    assert data["task_id"] == "task-1"
    assert data["status"] == "completed"
    assert data["output"] == "all done"
    assert data["step_count"] == 5
    assert data["cost_usd"] == pytest.approx(0.05)


async def test_send_output(runtime: RuntimeClient, mock_js: AsyncMock) -> None:
    """send_output should publish a streaming output line."""
    await runtime.send_output("Hello, world!", stream="stdout")

    mock_js.publish.assert_called_once()
    call_args = mock_js.publish.call_args
    assert call_args.args[0] == SUBJECT_RUN_OUTPUT
    data = json.loads(call_args.args[1])
    assert data["run_id"] == "run-1"
    assert data["task_id"] == "task-1"
    assert data["line"] == "Hello, world!"
    assert data["stream"] == "stdout"


def test_run_start_message_parsing() -> None:
    """RunStartMessage should parse JSON correctly."""
    raw = json.dumps(
        {
            "run_id": "r-1",
            "task_id": "t-1",
            "project_id": "p-1",
            "agent_id": "a-1",
            "prompt": "Fix the bug",
            "policy_profile": "headless-safe-sandbox",
            "exec_mode": "mount",
            "config": {"model": "gpt-4"},
            "termination": {"max_steps": 100, "timeout_seconds": 300, "max_cost": 10.0},
        }
    )
    msg = RunStartMessage.model_validate_json(raw)

    assert msg.run_id == "r-1"
    assert msg.prompt == "Fix the bug"
    assert msg.termination.max_steps == 100
    assert msg.termination.max_cost == pytest.approx(10.0)
    assert msg.config["model"] == "gpt-4"


def test_tool_call_decision_parsing() -> None:
    """ToolCallDecision should parse correctly."""
    decision = ToolCallDecision(call_id="c-1", decision="allow", reason="")

    assert decision.decision == "allow"
    assert decision.call_id == "c-1"
