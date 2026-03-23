"""Tests for tasks.cancel wiring (Task 1 of NATS remaining wiring).

Verifies:
- SUBJECT_TASK_CANCEL constant exists and matches Go subject
- RuntimeClient cancel listener recognizes task_id field
- _runs.py passes tasks.cancel as extra_subject
"""

from __future__ import annotations

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.consumer._subjects import SUBJECT_TASK_CANCEL
from codeforge.models import TerminationConfig
from codeforge.runtime import RuntimeClient


class TestSubjectTaskCancel:
    """SUBJECT_TASK_CANCEL must exist and match the Go side."""

    def test_constant_value(self) -> None:
        assert SUBJECT_TASK_CANCEL == "tasks.cancel"

    def test_constant_is_string(self) -> None:
        assert isinstance(SUBJECT_TASK_CANCEL, str)


class TestRuntimeClientCancelByTaskID:
    """RuntimeClient._listen_sub must match on task_id as well as run_id."""

    @pytest.fixture
    def mock_js(self) -> AsyncMock:
        js = AsyncMock()
        return js

    @pytest.fixture
    def runtime(self, mock_js: AsyncMock) -> RuntimeClient:
        return RuntimeClient(
            js=mock_js,
            run_id="run-abc",
            task_id="task-xyz",
            project_id="proj-1",
            termination=TerminationConfig(max_steps=50, timeout_seconds=600, max_cost=5.0),
        )

    async def test_cancel_by_run_id(self, runtime: RuntimeClient, mock_js: AsyncMock) -> None:
        """Cancel message with matching run_id sets _cancelled."""

        def make_sub() -> AsyncMock:
            sub = AsyncMock()
            call_count = 0

            async def next_msg_side_effect(timeout: float = 1.0) -> MagicMock:
                nonlocal call_count
                call_count += 1
                if call_count == 1:
                    msg = MagicMock()
                    msg.data = json.dumps({"run_id": "run-abc"}).encode()
                    return msg
                # Break the loop so the test completes.
                raise Exception("done")

            sub.next_msg = next_msg_side_effect
            return sub

        mock_js.subscribe.side_effect = lambda *a, **kw: make_sub()

        await runtime.start_cancel_listener()
        await asyncio.sleep(0.2)

        assert runtime.is_cancelled is True

    async def test_cancel_by_task_id(self, runtime: RuntimeClient, mock_js: AsyncMock) -> None:
        """Cancel message with matching task_id (no run_id) sets _cancelled."""

        def make_sub() -> AsyncMock:
            sub = AsyncMock()
            call_count = 0

            async def next_msg_side_effect(timeout: float = 1.0) -> MagicMock:
                nonlocal call_count
                call_count += 1
                if call_count == 1:
                    msg = MagicMock()
                    msg.data = json.dumps({"task_id": "task-xyz"}).encode()
                    return msg
                raise Exception("done")

            sub.next_msg = next_msg_side_effect
            return sub

        mock_js.subscribe.side_effect = lambda *a, **kw: make_sub()

        await runtime.start_cancel_listener()
        await asyncio.sleep(0.2)

        assert runtime.is_cancelled is True

    async def test_cancel_non_matching_ids_ignored(self, runtime: RuntimeClient, mock_js: AsyncMock) -> None:
        """Cancel message with non-matching IDs does not set _cancelled."""

        def make_sub() -> AsyncMock:
            sub = AsyncMock()
            call_count = 0

            async def next_msg_side_effect(timeout: float = 1.0) -> MagicMock:
                nonlocal call_count
                call_count += 1
                if call_count == 1:
                    msg = MagicMock()
                    msg.data = json.dumps({"run_id": "other", "task_id": "other"}).encode()
                    return msg
                # Break out of the loop.
                raise Exception("done")

            sub.next_msg = next_msg_side_effect
            return sub

        mock_js.subscribe.side_effect = lambda *a, **kw: make_sub()

        await runtime.start_cancel_listener()
        await asyncio.sleep(0.2)

        assert runtime.is_cancelled is False


class TestRunsHandlerExtraSubjects:
    """RunHandlerMixin must pass tasks.cancel as extra_subjects."""

    async def test_start_cancel_listener_called_with_tasks_cancel(self) -> None:
        """_do_run_start should call start_cancel_listener with tasks.cancel in extra_subjects."""
        from codeforge.consumer._runs import RunHandlerMixin
        from codeforge.consumer._subjects import SUBJECT_TASK_CANCEL
        from codeforge.models import RunStartMessage, TerminationConfig

        # Build a minimal RunStartMessage
        run_msg = RunStartMessage(
            run_id="run-1",
            task_id="task-1",
            project_id="proj-1",
            agent_id="agent-1",
            prompt="do something",
            policy_profile="default",
            exec_mode="sandbox",
            termination=TerminationConfig(max_steps=50, timeout_seconds=600, max_cost=5.0),
        )

        # Create handler with mock dependencies
        handler = type("Handler", (RunHandlerMixin,), {})()
        mock_js = AsyncMock()
        mock_js.subscribe = AsyncMock(return_value=AsyncMock())
        handler._js = mock_js

        mock_executor = AsyncMock()
        handler._executor = mock_executor
        handler._processed_ids = set()  # type: ignore[assignment]

        # Patch RuntimeClient to capture the extra_subjects call
        import codeforge.consumer._runs as runs_module

        captured_extra = []

        class MockRuntime:
            def __init__(self, **kwargs: object) -> None:
                self.run_id = kwargs["run_id"]
                self.task_id = kwargs["task_id"]

            async def start_cancel_listener(self, extra_subjects: list[str] | None = None) -> None:
                captured_extra.extend(extra_subjects or [])

        original_rc = runs_module.RuntimeClient
        runs_module.RuntimeClient = MockRuntime  # type: ignore[assignment,misc]
        try:
            import structlog

            log = structlog.get_logger().bind(test=True)
            await handler._do_run_start(run_msg, log)
        finally:
            runs_module.RuntimeClient = original_rc  # type: ignore[assignment,misc]

        assert SUBJECT_TASK_CANCEL in captured_extra
