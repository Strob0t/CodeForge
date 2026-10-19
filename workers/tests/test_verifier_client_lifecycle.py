"""A verifier without an injected client creates one per evaluator and closes it (S6-G review, item 11).

verifier_client(None) created a new LiteLLMClient for every evaluation and
never closed it, leaking an HTTP connection pool per call.
"""

from __future__ import annotations

import pytest

from codeforge.evaluation.evaluators.logprob_verifier import LogprobVerifierEvaluator
from codeforge.evaluation.evaluators.trajectory_verifier import TrajectoryVerifierEvaluator
from codeforge.evaluation.pipeline import EvaluationPipeline
from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec
from codeforge.llm import ChatCompletionResponse


def _response(content: str) -> ChatCompletionResponse:
    return ChatCompletionResponse(
        content=content, tool_calls=[], finish_reason="stop", tokens_in=1, tokens_out=1, model="m"
    )


class _FakeClient:
    created: list[_FakeClient] = []  # noqa: RUF012 - test bookkeeping

    def __init__(self, base_url: str = "", api_key: str = "") -> None:
        self.closed = 0
        self.calls = 0
        _FakeClient.created.append(self)

    async def chat_completion(self, **kwargs: object) -> ChatCompletionResponse:
        self.calls += 1
        if kwargs.get("max_tokens") == 1:
            return _response("YES")
        return _response(
            '{"solution_quality": "ACHIEVED", "approach_efficiency": "ACHIEVED", '
            '"code_quality": "ACHIEVED", "error_recovery": "ACHIEVED", "completeness": "ACHIEVED"}'
        )

    async def close(self) -> None:
        self.closed += 1


@pytest.fixture(autouse=True)
def fake_client(monkeypatch: pytest.MonkeyPatch) -> None:
    _FakeClient.created = []
    monkeypatch.setattr("codeforge.llm.LiteLLMClient", _FakeClient)


@pytest.mark.parametrize("evaluator_cls", [TrajectoryVerifierEvaluator, LogprobVerifierEvaluator])
async def test_one_owned_client_per_evaluator_closed_with_the_pipeline(evaluator_cls: type) -> None:
    evaluator = evaluator_cls(model="m")
    pipeline = EvaluationPipeline([evaluator])
    task = TaskSpec(id="t", name="t", input="do it")

    for _ in range(3):
        await pipeline.evaluate(task, ExecutionResult(actual_output="done"))

    assert len(_FakeClient.created) == 1
    assert _FakeClient.created[0].calls == 3
    await pipeline.aclose()
    assert _FakeClient.created[0].closed == 1
    await pipeline.aclose()  # idempotent
    assert _FakeClient.created[0].closed == 1


async def test_an_injected_client_is_used_and_never_closed() -> None:
    injected = _FakeClient()
    evaluator = TrajectoryVerifierEvaluator(model="m", llm=injected)  # type: ignore[arg-type]
    await evaluator.evaluate(TaskSpec(id="t", name="t", input="x"), ExecutionResult(actual_output="done"))
    await evaluator.aclose()
    assert injected.calls == 1
    assert injected.closed == 0
    assert _FakeClient.created == [injected]
