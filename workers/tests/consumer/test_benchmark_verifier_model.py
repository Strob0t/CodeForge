"""The benchmark LLM verifiers get a concrete model (S6-G review, item 6).

The verifiers were built before the run's LLM was resolved, so a run with
model "auto" sent the literal "auto" to the proxy for every verification.
They are now built after the resolution, with a concrete model: the run's
model, or for "auto" the resolved default model. They call the worker's own
client, not the routing wrapper, so verification calls are neither routed by
the task prompt nor recorded in the run's routing log.
"""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest
import structlog

from codeforge.consumer import TaskConsumer
from codeforge.models import BenchmarkRunRequest
from tests.jetstream_fakes import RecordingJetStream


@pytest.fixture
def consumer() -> TaskConsumer:
    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    worker._js = RecordingJetStream()  # type: ignore[assignment]
    return worker


@pytest.mark.parametrize(("model", "want"), [("auto", "openai/resolved-default"), ("openai/gpt-4o", "openai/gpt-4o")])
async def test_verifiers_get_a_concrete_model(
    consumer: TaskConsumer, monkeypatch: pytest.MonkeyPatch, model: str, want: str
) -> None:
    order: list[str] = []
    routing_wrapper = object()

    async def resolve(req: object, log: object) -> object:
        order.append("resolve")
        return routing_wrapper

    def build(names: list[str], verifier_model: str, llm: object = None) -> list:
        order.append("build")
        build.args = (verifier_model, llm)  # type: ignore[attr-defined]
        raise RuntimeError("stop after building")

    monkeypatch.setattr(consumer, "_resolve_effective_llm", resolve)
    monkeypatch.setattr("codeforge.consumer._benchmark._build_evaluators", build)
    monkeypatch.setattr("codeforge.model_resolver.resolve_model", MagicMock(return_value="openai/resolved-default"))
    request = BenchmarkRunRequest(run_id="b1", dataset_path="/d.yaml", model=model, evaluators=["trajectory_verifier"])

    await consumer._execute_benchmark_run(request, structlog.get_logger())

    assert order == ["resolve", "build"]
    verifier_model, llm = build.args  # type: ignore[attr-defined]
    assert verifier_model == want
    assert llm is consumer._llm
