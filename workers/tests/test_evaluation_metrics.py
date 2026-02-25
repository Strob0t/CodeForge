"""Tests for DeepEval metric wrappers (Phase 20A).

deepeval is not installed in the dev/test environment, so we mock the entire
module tree before importing production code that depends on it.
"""

from __future__ import annotations

import sys
from types import ModuleType
from unittest.mock import AsyncMock, MagicMock

import pytest

# --- Mock deepeval module tree before importing production code ---
_deepeval = ModuleType("deepeval")
_deepeval_metrics = ModuleType("deepeval.metrics")
_deepeval_test_case = ModuleType("deepeval.test_case")
_deepeval_models = ModuleType("deepeval.models")

_deepeval_metrics.GEval = MagicMock()
_deepeval_metrics.FaithfulnessMetric = MagicMock()
_deepeval_metrics.AnswerRelevancyMetric = MagicMock()
_deepeval_test_case.LLMTestCase = MagicMock()
_deepeval_test_case.ToolCall = MagicMock()
_deepeval_models.DeepEvalBaseLLM = type("DeepEvalBaseLLM", (), {})

sys.modules.setdefault("deepeval", _deepeval)
sys.modules.setdefault("deepeval.metrics", _deepeval_metrics)
sys.modules.setdefault("deepeval.test_case", _deepeval_test_case)
sys.modules.setdefault("deepeval.models", _deepeval_models)

import codeforge.evaluation.metrics as metrics_mod  # noqa: E402
from codeforge.evaluation.metrics import _build_judge  # noqa: E402


def _make_mock_judge() -> MagicMock:
    """Build a mock LiteLLMJudge."""
    judge = MagicMock()
    judge.get_model_name.return_value = "mock-judge"
    judge.load_model.return_value = "mock-judge"
    return judge


def _make_metric_mock(score: float) -> MagicMock:
    """Build a mock metric instance with AsyncMock a_measure."""
    m = MagicMock()
    m.score = score
    m.a_measure = AsyncMock()
    return m


@pytest.mark.asyncio
async def test_evaluate_correctness() -> None:
    """Correctness metric should create GEval metric and return score."""
    mock_metric = _make_metric_mock(0.9)
    original = metrics_mod.GEval
    metrics_mod.GEval = MagicMock(return_value=mock_metric)
    try:
        score = await metrics_mod.evaluate_correctness(
            user_input="Write fizzbuzz",
            actual_output="def fizzbuzz(): ...",
            expected_output="def fizzbuzz(n): ...",
            judge=_make_mock_judge(),
        )
        assert score == 0.9
        metrics_mod.GEval.assert_called_once()
        mock_metric.a_measure.assert_awaited_once()
    finally:
        metrics_mod.GEval = original


@pytest.mark.asyncio
async def test_evaluate_tool_correctness() -> None:
    """Tool correctness metric should encode tool calls in the test case."""
    mock_metric = _make_metric_mock(0.75)
    original = metrics_mod.GEval
    metrics_mod.GEval = MagicMock(return_value=mock_metric)
    try:
        score = await metrics_mod.evaluate_tool_correctness(
            user_input="List files",
            actual_output="file1.py file2.py",
            expected_tools=[{"name": "ls", "args": "."}],
            actual_tools=[{"name": "ls", "args": "."}],
            judge=_make_mock_judge(),
        )
        assert score == 0.75
        mock_metric.a_measure.assert_awaited_once()
    finally:
        metrics_mod.GEval = original


@pytest.mark.asyncio
async def test_evaluate_faithfulness() -> None:
    """Faithfulness metric should use retrieval context."""
    mock_metric = _make_metric_mock(0.8)
    original = metrics_mod.FaithfulnessMetric
    metrics_mod.FaithfulnessMetric = MagicMock(return_value=mock_metric)
    try:
        score = await metrics_mod.evaluate_faithfulness(
            user_input="What does function X do?",
            actual_output="Function X sorts a list",
            retrieval_context=["Function X: sorts a list using quicksort"],
            judge=_make_mock_judge(),
        )
        assert score == 0.8
        mock_metric.a_measure.assert_awaited_once()
    finally:
        metrics_mod.FaithfulnessMetric = original


@pytest.mark.asyncio
async def test_evaluate_answer_relevancy() -> None:
    """Answer relevancy metric should use input and output."""
    mock_metric = _make_metric_mock(0.95)
    original = metrics_mod.AnswerRelevancyMetric
    metrics_mod.AnswerRelevancyMetric = MagicMock(return_value=mock_metric)
    try:
        score = await metrics_mod.evaluate_answer_relevancy(
            user_input="How to create a list in Python?",
            actual_output="Use square brackets: my_list = []",
            judge=_make_mock_judge(),
        )
        assert score == 0.95
        mock_metric.a_measure.assert_awaited_once()
    finally:
        metrics_mod.AnswerRelevancyMetric = original


def test_build_judge_returns_provided() -> None:
    """_build_judge should return the provided judge without creating a new one."""
    judge = _make_mock_judge()
    result = _build_judge(judge)
    assert result is judge
