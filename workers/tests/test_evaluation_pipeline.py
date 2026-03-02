"""Tests for Phase 26B — Evaluation Pipeline & Evaluator Plugins."""

from __future__ import annotations

import asyncio

import pytest

from codeforge.evaluation.evaluators.base import Evaluator
from codeforge.evaluation.evaluators.functional_test import FunctionalTestEvaluator
from codeforge.evaluation.evaluators.llm_judge import SUPPORTED_METRICS, LLMJudgeEvaluator
from codeforge.evaluation.evaluators.sparc import (
    SPARCEvaluator,
)
from codeforge.evaluation.pipeline import EvaluationPipeline
from codeforge.evaluation.providers.base import EvalDimension, EvalScore, ExecutionResult, TaskSpec

# -- Fixtures --


@pytest.fixture
def simple_task() -> TaskSpec:
    return TaskSpec(
        id="t1",
        name="FizzBuzz",
        input="Write a FizzBuzz function",
        expected_output="def fizzbuzz(): ...",
    )


@pytest.fixture
def simple_result() -> ExecutionResult:
    return ExecutionResult(
        actual_output="def fizzbuzz(n): return 'fizz' if n % 3 == 0 else str(n)",
        cost_usd=0.05,
        tokens_in=100,
        tokens_out=50,
        duration_ms=2000,
        step_count=3,
    )


@pytest.fixture
def agent_result() -> ExecutionResult:
    return ExecutionResult(
        actual_output="Fixed the bug in auth module",
        files_changed=["auth.py", "test_auth.py"],
        test_output="2 passed, 0 failed",
        cost_usd=1.50,
        tokens_in=5000,
        tokens_out=2000,
        duration_ms=60000,
        step_count=12,
    )


# -- Stub evaluator for pipeline tests --


class StubEvaluator:
    """Test evaluator that returns fixed scores."""

    def __init__(self, eval_name: str = "stub", scores: list[EvalDimension] | None = None) -> None:
        self._name = eval_name
        self._scores = scores or [EvalDimension(name="stub_metric", score=0.85)]

    @property
    def name(self) -> str:
        return self._name

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        return self._scores


class FailingEvaluator:
    """Test evaluator that always raises."""

    @property
    def name(self) -> str:
        return "failing"

    async def evaluate(self, task: TaskSpec, result: ExecutionResult) -> list[EvalDimension]:
        raise RuntimeError("evaluation exploded")


# -- Evaluator Protocol Tests --


class TestEvaluatorProtocol:
    def test_stub_satisfies_protocol(self) -> None:
        assert isinstance(StubEvaluator(), Evaluator)

    def test_llm_judge_satisfies_protocol(self) -> None:
        evaluator = LLMJudgeEvaluator()
        assert isinstance(evaluator, Evaluator)
        assert evaluator.name == "llm_judge"

    def test_functional_test_satisfies_protocol(self) -> None:
        evaluator = FunctionalTestEvaluator()
        assert isinstance(evaluator, Evaluator)
        assert evaluator.name == "functional_test"

    def test_sparc_satisfies_protocol(self) -> None:
        evaluator = SPARCEvaluator()
        assert isinstance(evaluator, Evaluator)
        assert evaluator.name == "sparc"


# -- SPARC Evaluator Tests --


class TestSPARCEvaluator:
    def test_steps_score_zero_steps(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(step_count=0)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        steps_dim = next(d for d in dims if d.name == "sparc_steps")
        assert steps_dim.score == 1.0

    def test_steps_score_half_max(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator(max_steps=50)
        result = ExecutionResult(step_count=25)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        steps_dim = next(d for d in dims if d.name == "sparc_steps")
        assert steps_dim.score == 0.5

    def test_steps_score_over_max(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator(max_steps=10)
        result = ExecutionResult(step_count=20)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        steps_dim = next(d for d in dims if d.name == "sparc_steps")
        assert steps_dim.score == 0.0

    def test_time_score(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator(max_duration_ms=100_000)
        result = ExecutionResult(duration_ms=50_000)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        time_dim = next(d for d in dims if d.name == "sparc_time")
        assert time_dim.score == 0.5

    def test_cost_score(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator(max_cost_usd=10.0)
        result = ExecutionResult(cost_usd=2.0)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        cost_dim = next(d for d in dims if d.name == "sparc_cost")
        assert cost_dim.score == 0.8

    def test_complexity_simple(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(step_count=3)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        comp_dim = next(d for d in dims if d.name == "sparc_complexity")
        assert comp_dim.score == 1.0
        assert comp_dim.details["category"] == "simple"

    def test_complexity_medium(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(step_count=10)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        comp_dim = next(d for d in dims if d.name == "sparc_complexity")
        assert comp_dim.score == 0.75
        assert comp_dim.details["category"] == "medium"

    def test_complexity_complex(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(step_count=20)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        comp_dim = next(d for d in dims if d.name == "sparc_complexity")
        assert comp_dim.score == 0.5

    def test_complexity_very_complex(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(step_count=50)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        comp_dim = next(d for d in dims if d.name == "sparc_complexity")
        assert comp_dim.score == 0.25
        assert comp_dim.details["category"] == "very_complex"

    def test_code_quality_clean(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(files_changed=["main.py"], test_output="1 passed")
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        quality_dim = next(d for d in dims if d.name == "sparc_code_quality")
        assert quality_dim.score == 1.0

    def test_code_quality_many_files(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(files_changed=[f"file{i}.py" for i in range(12)])
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        quality_dim = next(d for d in dims if d.name == "sparc_code_quality")
        assert quality_dim.score == 0.7  # 1.0 - 0.3

    def test_code_quality_warnings(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(test_output="2 passed, 1 warning")
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        quality_dim = next(d for d in dims if d.name == "sparc_code_quality")
        assert quality_dim.score == 0.9  # 1.0 - 0.1

    def test_security_clean(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(actual_output="print('hello')")
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        sec_dim = next(d for d in dims if d.name == "sparc_security")
        assert sec_dim.score == 1.0

    def test_security_hardcoded_secret(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(actual_output='API_KEY = "sk-abcdefghijklmnopqrstuvwxyz1234567890"')
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        sec_dim = next(d for d in dims if d.name == "sparc_security")
        assert sec_dim.score <= 0.5
        assert "hardcoded_secret" in sec_dim.details

    def test_security_unsafe_command(self, simple_task: TaskSpec) -> None:
        evaluator = SPARCEvaluator()
        result = ExecutionResult(actual_output="run: rm -rf /important")
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, result))
        sec_dim = next(d for d in dims if d.name == "sparc_security")
        assert sec_dim.score <= 0.5
        assert "unsafe_command" in sec_dim.details

    def test_all_dimensions_returned(self, simple_task: TaskSpec, simple_result: ExecutionResult) -> None:
        evaluator = SPARCEvaluator()
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, simple_result))
        names = {d.name for d in dims}
        assert names == {
            "sparc_steps",
            "sparc_time",
            "sparc_cost",
            "sparc_complexity",
            "sparc_code_quality",
            "sparc_security",
        }


# -- Functional Test Evaluator Tests --


class TestFunctionalTestEvaluator:
    def test_no_command(self, simple_task: TaskSpec, simple_result: ExecutionResult) -> None:
        evaluator = FunctionalTestEvaluator()
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, simple_result))
        assert len(dims) == 1
        assert dims[0].score == 0.0
        assert "no test_command" in dims[0].details.get("error", "")

    def test_passing_command(self) -> None:
        task = TaskSpec(id="t1", name="test", input="test", test_command="true")
        result = ExecutionResult()
        evaluator = FunctionalTestEvaluator()
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(task, result))
        assert len(dims) == 1
        assert dims[0].score == 1.0
        assert dims[0].details["exit_code"] == "0"

    def test_failing_command(self) -> None:
        task = TaskSpec(id="t1", name="test", input="test", test_command="false")
        result = ExecutionResult()
        evaluator = FunctionalTestEvaluator()
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(task, result))
        assert len(dims) == 1
        assert dims[0].score == 0.0

    def test_command_with_output(self) -> None:
        task = TaskSpec(id="t1", name="test", input="test", test_command="echo 'hello world'")
        result = ExecutionResult()
        evaluator = FunctionalTestEvaluator()
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(task, result))
        assert "hello world" in dims[0].details.get("output", "")

    def test_timeout(self) -> None:
        task = TaskSpec(id="t1", name="test", input="test", test_command="sleep 10")
        result = ExecutionResult()
        evaluator = FunctionalTestEvaluator(timeout=1)
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(task, result))
        assert dims[0].score == 0.0
        assert "timeout" in dims[0].details.get("error", "")


# -- LLM Judge Evaluator Tests --


class TestLLMJudgeEvaluator:
    def test_default_metrics(self) -> None:
        evaluator = LLMJudgeEvaluator()
        assert evaluator._metrics == ["correctness"]

    def test_custom_metrics(self) -> None:
        evaluator = LLMJudgeEvaluator(metrics=["correctness", "faithfulness"])
        assert evaluator._metrics == ["correctness", "faithfulness"]

    def test_supported_metrics_set(self) -> None:
        assert "correctness" in SUPPORTED_METRICS
        assert "tool_correctness" in SUPPORTED_METRICS
        assert "faithfulness" in SUPPORTED_METRICS
        assert "answer_relevancy" in SUPPORTED_METRICS
        assert "nonexistent" not in SUPPORTED_METRICS

    def test_unsupported_metric_skipped(self, simple_task: TaskSpec, simple_result: ExecutionResult) -> None:
        evaluator = LLMJudgeEvaluator(metrics=["nonexistent_metric"])
        dims = asyncio.get_event_loop().run_until_complete(evaluator.evaluate(simple_task, simple_result))
        # Should skip unsupported metrics and return empty.
        assert len(dims) == 0


# -- EvaluationPipeline Tests --


class TestEvaluationPipeline:
    def test_requires_evaluators(self) -> None:
        with pytest.raises(ValueError, match="at least one evaluator"):
            EvaluationPipeline([])

    def test_single_evaluator(self, simple_task: TaskSpec, simple_result: ExecutionResult) -> None:
        pipeline = EvaluationPipeline([StubEvaluator()])
        score = asyncio.get_event_loop().run_until_complete(pipeline.evaluate(simple_task, simple_result))
        assert isinstance(score, EvalScore)
        assert len(score.dimensions) == 1
        assert score.dimensions[0].score == 0.85

    def test_multiple_evaluators(self, simple_task: TaskSpec, simple_result: ExecutionResult) -> None:
        e1 = StubEvaluator("eval_a", [EvalDimension(name="metric_a", score=0.9)])
        e2 = StubEvaluator("eval_b", [EvalDimension(name="metric_b", score=0.7)])
        pipeline = EvaluationPipeline([e1, e2])
        score = asyncio.get_event_loop().run_until_complete(pipeline.evaluate(simple_task, simple_result))
        assert len(score.dimensions) == 2
        names = {d.name for d in score.dimensions}
        assert names == {"metric_a", "metric_b"}

    def test_evaluator_names(self) -> None:
        pipeline = EvaluationPipeline([StubEvaluator("a"), StubEvaluator("b")])
        assert pipeline.evaluator_names == ["a", "b"]

    def test_failing_evaluator_handled(self, simple_task: TaskSpec, simple_result: ExecutionResult) -> None:
        pipeline = EvaluationPipeline([FailingEvaluator(), StubEvaluator()])
        score = asyncio.get_event_loop().run_until_complete(pipeline.evaluate(simple_task, simple_result))
        # Should have error dimension from failing + stub dimension.
        assert len(score.dimensions) == 2
        error_dim = next(d for d in score.dimensions if "error" in d.name)
        assert error_dim.score == 0.0

    def test_cost_per_score_point(self, simple_task: TaskSpec) -> None:
        result = ExecutionResult(cost_usd=1.0, tokens_in=100, tokens_out=50)
        pipeline = EvaluationPipeline([StubEvaluator(scores=[EvalDimension(name="m", score=0.5)])])
        score = asyncio.get_event_loop().run_until_complete(pipeline.evaluate(simple_task, result))
        # cost_per_point = 1.0 / 0.5 = 2.0
        assert score.cost_per_score_point == 2.0

    def test_token_efficiency(self, simple_task: TaskSpec) -> None:
        result = ExecutionResult(cost_usd=1.0, tokens_in=100, tokens_out=100)
        pipeline = EvaluationPipeline([StubEvaluator(scores=[EvalDimension(name="m", score=0.5)])])
        score = asyncio.get_event_loop().run_until_complete(pipeline.evaluate(simple_task, result))
        # token_efficiency = 0.5 / 200 = 0.0025
        assert score.token_efficiency == 0.0025

    def test_zero_score_no_division_error(self, simple_task: TaskSpec) -> None:
        result = ExecutionResult(cost_usd=1.0, tokens_in=0, tokens_out=0)
        pipeline = EvaluationPipeline([StubEvaluator(scores=[EvalDimension(name="m", score=0.0)])])
        score = asyncio.get_event_loop().run_until_complete(pipeline.evaluate(simple_task, result))
        assert score.cost_per_score_point == 0.0
        assert score.token_efficiency == 0.0

    def test_evaluate_batch(self, simple_task: TaskSpec, simple_result: ExecutionResult) -> None:
        pipeline = EvaluationPipeline([StubEvaluator()])
        pairs = [(simple_task, simple_result), (simple_task, simple_result)]
        scores = asyncio.get_event_loop().run_until_complete(pipeline.evaluate_batch(pairs))
        assert len(scores) == 2
        assert all(isinstance(s, EvalScore) for s in scores)


# -- EvalScore Tests --


class TestEvalScore:
    def test_average_score_empty(self) -> None:
        score = EvalScore()
        assert score.average_score() == 0.0

    def test_average_score_single(self) -> None:
        score = EvalScore(dimensions=[EvalDimension(name="m", score=0.8)])
        assert score.average_score() == 0.8

    def test_average_score_multiple(self) -> None:
        score = EvalScore(
            dimensions=[
                EvalDimension(name="a", score=0.6),
                EvalDimension(name="b", score=0.8),
            ]
        )
        assert score.average_score() == pytest.approx(0.7)


# -- Integration: SPARC + Pipeline --


class TestSPARCPipelineIntegration:
    def test_sparc_in_pipeline(self, simple_task: TaskSpec, agent_result: ExecutionResult) -> None:
        pipeline = EvaluationPipeline([SPARCEvaluator()])
        score = asyncio.get_event_loop().run_until_complete(pipeline.evaluate(simple_task, agent_result))
        assert len(score.dimensions) == 6
        assert score.average_score() > 0

    def test_functional_plus_sparc(self) -> None:
        task = TaskSpec(id="t1", name="test", input="test", test_command="true")
        result = ExecutionResult(step_count=5, duration_ms=10000, cost_usd=0.50)
        pipeline = EvaluationPipeline([FunctionalTestEvaluator(), SPARCEvaluator()])
        score = asyncio.get_event_loop().run_until_complete(pipeline.evaluate(task, result))
        # 1 from functional + 6 from SPARC = 7
        assert len(score.dimensions) == 7
        func_dim = next(d for d in score.dimensions if d.name == "functional_test")
        assert func_dim.score == 1.0
