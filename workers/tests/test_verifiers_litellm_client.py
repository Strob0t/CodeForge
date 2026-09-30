"""The trajectory and logprob verifiers call the LiteLLM proxy through the worker's client (KI-37).

They imported the ``litellm`` package, which is not a worker dependency, and
turned the ImportError into a 0.0 score, so every run scored 0.0. Now they use
``LiteLLMClient.chat_completion`` and a failure is an evaluation error: the
evaluator raises, the pipeline records ``<evaluator>_error`` with the cause,
and the error does not count as a 0.0 score in the averages.
"""

from __future__ import annotations

import json
import sys
from typing import TYPE_CHECKING

import httpx
import pytest

from codeforge.evaluation.evaluators.base import EvaluatorError
from codeforge.evaluation.evaluators.logprob_verifier import LogprobVerifierEvaluator
from codeforge.evaluation.evaluators.trajectory_verifier import TrajectoryVerifierEvaluator
from codeforge.evaluation.pipeline import EvaluationPipeline
from codeforge.evaluation.providers.base import EvalDimension, EvalScore, ExecutionResult, TaskSpec, TrajectoryMessage
from codeforge.llm import ChatCompletionResponse, LiteLLMClient, LLMError, TokenLogprob

if TYPE_CHECKING:
    from collections.abc import Callable


def _task() -> TaskSpec:
    return TaskSpec(id="t1", name="Fix bug", input="Fix the login bug", expected_output="login works")


def _result() -> ExecutionResult:
    return ExecutionResult(
        actual_output="fixed",
        files_changed=["login.py"],
        trajectory=[
            TrajectoryMessage(role="user", content="fix it"),
            TrajectoryMessage(role="assistant", content="done"),
        ],
    )


class FakeLLM:
    """Stands in for LiteLLMClient: records chat_completion calls and returns a scripted answer."""

    def __init__(self, answer: ChatCompletionResponse | Exception) -> None:
        self.answer = answer
        self.calls: list[dict[str, object]] = []

    async def chat_completion(self, **kwargs: object) -> ChatCompletionResponse:
        self.calls.append(kwargs)
        if isinstance(self.answer, Exception):
            raise self.answer
        return self.answer


def _answer(content: str, top_logprobs: list[TokenLogprob] | None = None) -> ChatCompletionResponse:
    return ChatCompletionResponse(
        content=content,
        tool_calls=[],
        finish_reason="stop",
        tokens_in=10,
        tokens_out=1,
        model="judge",
        top_logprobs=top_logprobs or [],
    )


@pytest.fixture
def no_litellm_package(monkeypatch: pytest.MonkeyPatch) -> None:
    """The litellm package is not installed in the worker: importing it fails."""
    monkeypatch.setitem(sys.modules, "litellm", None)


ALL_ACHIEVED = json.dumps(
    {
        "solution_quality": "ACHIEVED",
        "approach_efficiency": "PARTIALLY_ACHIEVED",
        "code_quality": "ACHIEVED",
        "error_recovery": "NOT_ACHIEVED",
        "completeness": "ACHIEVED",
    }
)


@pytest.mark.usefixtures("no_litellm_package")
async def test_trajectory_verifier_scores_through_the_proxy_client() -> None:
    llm = FakeLLM(_answer(ALL_ACHIEVED))
    dims = await TrajectoryVerifierEvaluator(model="judge-model", llm=llm).evaluate(_task(), _result())  # type: ignore[arg-type]

    assert {d.name: d.score for d in dims} == {
        "trajectory_solution_quality": 1.0,
        "trajectory_approach_efficiency": 0.5,
        "trajectory_code_quality": 1.0,
        "trajectory_error_recovery": 0.0,
        "trajectory_completeness": 1.0,
    }
    assert llm.calls[0]["model"] == "judge-model"
    assert llm.calls[0]["temperature"] == 0.0


@pytest.mark.usefixtures("no_litellm_package")
async def test_logprob_verifier_scores_through_the_proxy_client() -> None:
    llm = FakeLLM(_answer("YES", [TokenLogprob("YES", -0.1), TokenLogprob("NO", -2.5)]))
    dims = await LogprobVerifierEvaluator(model="judge-model", llm=llm).evaluate(_task(), _result())  # type: ignore[arg-type]

    assert len(dims) == 1
    assert dims[0].name == "logprob_verification"
    assert dims[0].score == pytest.approx(0.917, abs=0.01)
    assert dims[0].details["method"] == "logprob"
    call = llm.calls[0]
    assert call["model"] == "judge-model"
    assert call["max_tokens"] == 1
    assert call["logprobs"] is True
    assert call["top_logprobs"] == 20


@pytest.mark.parametrize(
    "make",
    [
        lambda: TrajectoryVerifierEvaluator(llm=FakeLLM(LLMError(502, "judge", "bad gateway"))),  # type: ignore[arg-type]
        lambda: TrajectoryVerifierEvaluator(llm=FakeLLM(_answer("I cannot evaluate this."))),  # type: ignore[arg-type]
        lambda: LogprobVerifierEvaluator(llm=FakeLLM(LLMError(502, "judge", "bad gateway"))),  # type: ignore[arg-type]
    ],
    ids=["trajectory call fails", "trajectory answer unparseable", "logprob call fails"],
)
async def test_verifier_failure_is_an_evaluation_error(make: Callable[[], object]) -> None:
    evaluator = make()
    with pytest.raises(EvaluatorError):
        await evaluator.evaluate(_task(), _result())  # type: ignore[attr-defined]


async def test_pipeline_records_the_error_and_leaves_it_out_of_the_average() -> None:
    class Fixed:
        name = "fixed"
        stage = "rank"

        async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
            return [EvalDimension(name="fixed_score", score=0.8)]

    failing = TrajectoryVerifierEvaluator(llm=FakeLLM(LLMError(502, "judge", "bad gateway")))  # type: ignore[arg-type]
    score = await EvaluationPipeline([Fixed(), failing]).evaluate(_task(), _result())  # type: ignore[list-item]

    errors = [d for d in score.dimensions if d.error]
    assert [d.name for d in errors] == ["trajectory_verifier_error"]
    assert "502" in errors[0].error
    assert errors[0].details["error"] == errors[0].error
    assert score.average_score() == pytest.approx(0.8), "an error is not a 0.0 score"


def test_average_without_scored_dimensions_is_zero() -> None:
    score = EvalScore(dimensions=[EvalDimension(name="x_error", score=0.0, error="boom")])
    assert score.average_score() == 0.0


def _client(handler: Callable[[httpx.Request], httpx.Response]) -> LiteLLMClient:
    client = LiteLLMClient(base_url="http://litellm.test")
    client._client = httpx.AsyncClient(base_url="http://litellm.test", transport=httpx.MockTransport(handler))
    return client


async def test_client_requests_and_parses_top_logprobs() -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen.update(json.loads(request.content))
        return httpx.Response(
            200,
            json={
                "choices": [
                    {
                        "message": {"role": "assistant", "content": "YES"},
                        "finish_reason": "stop",
                        "logprobs": {
                            "content": [
                                {
                                    "token": "YES",
                                    "logprob": -0.1,
                                    "top_logprobs": [
                                        {"token": "YES", "logprob": -0.1},
                                        {"token": "NO", "logprob": -2.5},
                                    ],
                                }
                            ]
                        },
                    }
                ],
                "usage": {"prompt_tokens": 5, "completion_tokens": 1},
            },
        )

    resp = await _client(handler).chat_completion(
        messages=[{"role": "user", "content": "ok?"}], model="judge", max_tokens=1, logprobs=True, top_logprobs=20
    )

    assert seen["logprobs"] is True
    assert seen["top_logprobs"] == 20
    assert resp.top_logprobs == [TokenLogprob("YES", -0.1), TokenLogprob("NO", -2.5)]


async def test_client_sends_no_logprobs_unless_asked() -> None:
    seen: dict[str, object] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen.update(json.loads(request.content))
        return httpx.Response(200, json={"choices": [{"message": {"content": "hi"}, "finish_reason": "stop"}]})

    resp = await _client(handler).chat_completion(messages=[{"role": "user", "content": "hi"}], model="m")

    assert "logprobs" not in seen
    assert "top_logprobs" not in seen
    assert resp.top_logprobs == []


def test_benchmark_evaluators_share_the_worker_client() -> None:
    from codeforge.consumer._benchmark import _build_evaluators

    llm = FakeLLM(_answer("YES"))
    evaluators = _build_evaluators(["trajectory_verifier", "logprob_verifier"], "judge-model", llm=llm)  # type: ignore[arg-type]

    assert [e._llm for e in evaluators] == [llm, llm]
