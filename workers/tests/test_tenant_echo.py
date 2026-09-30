"""Workers echo the tenant of each request in the messages they send back (KI-12).

The Go core scopes WebSocket events (and store writes) to the tenant carried in
worker messages and drops events without one, so every message a worker sends
on behalf of a run or request must carry that request's ``tenant_id``.
"""

from __future__ import annotations

import json
import uuid
from collections import OrderedDict
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest
from pydantic import BaseModel

from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._benchmark import _build_progress_callbacks
from codeforge.consumer._quality_gate import QualityGateHandlerMixin
from codeforge.consumer._repomap import RepoMapHandlerMixin
from codeforge.consumer._runs import RunHandlerMixin
from codeforge.consumer._subjects import (
    SUBJECT_OUTPUT,
    SUBJECT_QG_RESULT,
    SUBJECT_REPOMAP_RESULT,
    SUBJECT_RESULT,
)
from codeforge.consumer._tasks import TaskHandlerMixin
from codeforge.models import (
    GraphBuildRequest,
    GraphBuildResult,
    QualityGateRequest,
    QualityGateResult,
    RepoMapRequest,
    RepoMapResult,
    RetrievalIndexRequest,
    RetrievalIndexResult,
    RunStartMessage,
    TerminationConfig,
)
from codeforge.nats_subjects import (
    SUBJECT_AGENT_OUTPUT,
    SUBJECT_BENCHMARK_TASK_PROGRESS,
    SUBJECT_BENCHMARK_TASK_STARTED,
    SUBJECT_RUN_COMPLETE,
    SUBJECT_RUN_OUTPUT,
    SUBJECT_TOOLCALL_REQUEST,
    SUBJECT_TOOLCALL_RESULT,
    SUBJECT_TRAJECTORY_EVENT,
)
from codeforge.runtime import RuntimeClient

TENANT = "aaaaaaaa-0000-0000-0000-000000000001"


@pytest.fixture(autouse=True)
def _fresh_dedup_cache(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(ConsumerBaseMixin, "_processed_ids", OrderedDict())


def _js() -> MagicMock:
    js = MagicMock()
    js.publish = AsyncMock()
    js.subscribe = AsyncMock()
    return js


def _published(js: MagicMock) -> list[tuple[str, dict]]:
    return [(c.args[0], json.loads(c.args[1])) for c in js.publish.call_args_list]


def _msg(payload: dict) -> MagicMock:
    msg = MagicMock()
    msg.data = json.dumps(payload).encode()
    msg.headers = {}
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    msg.ack_sync = AsyncMock()
    return msg


def _runtime(js: MagicMock, tenant_id: str = TENANT) -> RuntimeClient:
    return RuntimeClient(
        js=js,
        run_id="run-1",
        task_id="task-1",
        project_id="proj-1",
        termination=TerminationConfig(),
        tenant_id=tenant_id,
    )


# --- RuntimeClient ---


async def test_runtime_client_echoes_tenant_on_every_run_message() -> None:
    js = _js()
    runtime = _runtime(js)

    await runtime.send_output("line")
    await runtime.publish_trajectory_event({"event_type": "agent.step_done"})
    await runtime.report_tool_result(call_id="call-1", tool="read_file", success=True)
    await runtime.complete_run(status="completed")

    published = _published(js)
    assert {subject for subject, _ in published} == {
        SUBJECT_RUN_OUTPUT,
        SUBJECT_AGENT_OUTPUT,
        SUBJECT_TRAJECTORY_EVENT,
        SUBJECT_TOOLCALL_RESULT,
        SUBJECT_RUN_COMPLETE,
    }
    for subject, payload in published:
        assert payload["tenant_id"] == TENANT, subject


async def test_runtime_client_echoes_tenant_on_tool_call_request() -> None:
    js = _js()
    runtime = _runtime(js)

    async def respond(timeout: float = 1.0) -> MagicMock:
        request = json.loads(js.publish.call_args.args[1])
        reply = MagicMock()
        reply.data = json.dumps({"call_id": request["call_id"], "decision": "allow"}).encode()
        return reply

    sub = AsyncMock()
    sub.next_msg = respond
    js.subscribe.return_value = sub

    decision = await runtime.request_tool_call(tool="read_file", path="main.go")

    assert decision.decision == "allow"
    subject, payload = _published(js)[0]
    assert subject == SUBJECT_TOOLCALL_REQUEST
    assert payload["tenant_id"] == TENANT


async def test_runtime_client_without_tenant_sends_empty_tenant() -> None:
    js = _js()
    runtime = RuntimeClient(js=js, run_id="r", task_id="t", project_id="p", termination=TerminationConfig())

    await runtime.send_output("line")

    for _, payload in _published(js):
        assert payload["tenant_id"] == ""


async def test_run_start_passes_tenant_to_runtime_client() -> None:
    handler = RunHandlerMixin()
    handler._js = _js()
    handler._executor = SimpleNamespace(execute_with_runtime=AsyncMock())
    run_msg = RunStartMessage(
        run_id=str(uuid.uuid4()),
        task_id="task-1",
        tenant_id=TENANT,
        project_id="proj-1",
        agent_id="agent-1",
        prompt="do it",
    )

    await handler._do_run_start(run_msg, MagicMock())

    runtime = handler._executor.execute_with_runtime.call_args.args[1]
    assert runtime.tenant_id == TENANT


# --- Request/result handlers (_handle_request) ---


def _repomap_result() -> RepoMapResult:
    return RepoMapResult(project_id="p", map_text="m", token_count=1, file_count=1, symbol_count=1, languages=["go"])


class _RepoMapHandler(RepoMapHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js = _js()
        self._repomap_generator = MagicMock()
        self._repomap_generator.generate = AsyncMock(return_value=_repomap_result())


class _QualityGateHandler(QualityGateHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js = _js()
        self._gate_executor = MagicMock()
        self._gate_executor.execute = AsyncMock(return_value=QualityGateResult(run_id="run-1", tests_passed=True))


async def test_repomap_result_echoes_request_tenant() -> None:
    handler = _RepoMapHandler()
    request = RepoMapRequest(project_id=str(uuid.uuid4()), workspace_path="/ws", tenant_id=TENANT)

    await handler._handle_repomap(_msg(request.model_dump()))

    subject, payload = _published(handler._js)[0]
    assert subject == SUBJECT_REPOMAP_RESULT
    assert payload["tenant_id"] == TENANT


async def test_quality_gate_result_echoes_request_tenant() -> None:
    handler = _QualityGateHandler()
    request = QualityGateRequest(run_id=str(uuid.uuid4()), project_id="p", workspace_path="/ws", tenant_id=TENANT)

    await handler._handle_quality_gate(_msg(request.model_dump()))

    subject, payload = _published(handler._js)[0]
    assert subject == SUBJECT_QG_RESULT
    assert payload["tenant_id"] == TENANT


class _NoTenantResult(BaseModel):
    project_id: str = ""


class _EchoHandler(ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js = _js()


async def test_handle_request_leaves_results_without_tenant_field_alone() -> None:
    handler = _EchoHandler()
    request = RepoMapRequest(project_id=str(uuid.uuid4()), workspace_path="/ws", tenant_id=TENANT)

    async def produce(_req: RepoMapRequest, _log: object) -> _NoTenantResult:
        return _NoTenantResult(project_id="p")

    await handler._handle_request(
        msg=_msg(request.model_dump()),
        request_model=RepoMapRequest,
        dedup_key=lambda r: r.project_id,
        handler=produce,
        result_subject="some.result",
    )

    _, payload = _published(handler._js)[0]
    assert payload == {"project_id": "p"}


async def test_handle_request_keeps_a_tenant_the_handler_set() -> None:
    handler = _EchoHandler()
    request = RepoMapRequest(project_id=str(uuid.uuid4()), workspace_path="/ws", tenant_id=TENANT)

    async def produce(_req: RepoMapRequest, _log: object) -> RepoMapResult:
        return _repomap_result().model_copy(update={"tenant_id": "other-tenant"})

    await handler._handle_request(
        msg=_msg(request.model_dump()),
        request_model=RepoMapRequest,
        dedup_key=lambda r: r.project_id,
        handler=produce,
        result_subject="some.result",
    )

    _, payload = _published(handler._js)[0]
    assert payload["tenant_id"] == "other-tenant"


@pytest.mark.parametrize(
    ("request_model", "result_model"),
    [
        (RepoMapRequest, RepoMapResult),
        (RetrievalIndexRequest, RetrievalIndexResult),
        (GraphBuildRequest, GraphBuildResult),
        (QualityGateRequest, QualityGateResult),
    ],
)
def test_request_and_result_models_carry_tenant(request_model: type[BaseModel], result_model: type[BaseModel]) -> None:
    assert "tenant_id" in request_model.model_fields
    assert "tenant_id" in result_model.model_fields
    assert result_model.model_fields["tenant_id"].default == ""


# --- Backend tasks (tasks.agent.*) ---


class _TaskHandler(TaskHandlerMixin, ConsumerBaseMixin):
    def __init__(self) -> None:
        self._js = _js()

        async def execute(**kwargs: object) -> SimpleNamespace:
            await kwargs["on_output"]("working")  # type: ignore[operator]
            return SimpleNamespace(status="completed", output="done", error="")

        self._backend_router = SimpleNamespace(execute=execute)


async def test_backend_task_output_and_result_echo_tenant() -> None:
    handler = _TaskHandler()
    task = {"id": str(uuid.uuid4()), "tenant_id": TENANT, "project_id": "proj-1", "title": "t", "prompt": "p"}
    msg = _msg(task)
    msg.subject = "tasks.agent.aider"

    await handler._handle_message(msg)

    published = _published(handler._js)
    assert {subject for subject, _ in published} == {SUBJECT_OUTPUT, SUBJECT_RESULT}
    for subject, payload in published:
        assert payload["tenant_id"] == TENANT, subject
    result = next(payload for subject, payload in published if subject == SUBJECT_RESULT)
    assert result["project_id"] == "proj-1"


# --- Benchmark progress ---


async def test_benchmark_progress_echoes_tenant() -> None:
    js = _js()
    on_start, on_complete = _build_progress_callbacks(js, "run-1", TENANT)
    task = SimpleNamespace(id="t-1", name="task one")
    result = SimpleNamespace(cost_usd=0.1, scores={"correctness": 1.0})

    await on_start(task, 0, 2)
    await on_complete(task, result, 0, 2)

    published = _published(js)
    assert [subject for subject, _ in published] == [SUBJECT_BENCHMARK_TASK_STARTED, SUBJECT_BENCHMARK_TASK_PROGRESS]
    for subject, payload in published:
        assert payload["tenant_id"] == TENANT, subject
