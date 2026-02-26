"""Tests for BenchmarkRunner (Phase 20A).

deepeval is not installed in the dev/test environment, so we mock the entire
module tree before importing production code that depends on it.
"""

from __future__ import annotations

import sys
from types import ModuleType
from unittest.mock import AsyncMock, MagicMock, patch

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
_deepeval_test_case.LLMTestCaseParams = MagicMock()
_deepeval_test_case.ToolCall = MagicMock()
_deepeval_models.DeepEvalBaseLLM = type("DeepEvalBaseLLM", (), {})

sys.modules.setdefault("deepeval", _deepeval)
sys.modules.setdefault("deepeval.metrics", _deepeval_metrics)
sys.modules.setdefault("deepeval.test_case", _deepeval_test_case)
sys.modules.setdefault("deepeval.models", _deepeval_models)

from codeforge.evaluation.datasets import BenchmarkDataset, BenchmarkTask  # noqa: E402
from codeforge.evaluation.runner import BenchmarkRunner  # noqa: E402


def _make_dataset(num_tasks: int = 2) -> BenchmarkDataset:
    """Build a small benchmark dataset for testing."""
    tasks = [
        BenchmarkTask(
            id=f"task-{i}",
            name=f"Test Task {i}",
            input=f"Solve problem {i}",
            expected_output=f"Solution {i}",
            expected_tools=[],
            context=[],
        )
        for i in range(num_tasks)
    ]
    return BenchmarkDataset(name="test-dataset", description="unit test", tasks=tasks)


class _FakeResponse:
    """Mimics ChatCompletionResponse with .content and .tool_calls."""

    def __init__(self, content: str, tool_calls: list[object] | None = None) -> None:
        self.content = content
        self.tool_calls = tool_calls or []


def _make_fake_llm(responses: list[dict[str, str]] | None = None) -> MagicMock:
    """Build a mock LiteLLMClient with pre-programmed chat_completion() responses."""
    llm = MagicMock()
    if responses is None:
        responses = [{"content": "fake answer"}]

    call_count = 0

    async def _chat_completion(**kwargs: object) -> _FakeResponse:
        nonlocal call_count
        resp = responses[call_count % len(responses)]
        call_count += 1
        return _FakeResponse(
            content=resp.get("content", ""),
            tool_calls=resp.get("tool_calls", []),
        )

    llm.chat_completion = AsyncMock(side_effect=_chat_completion)
    return llm


def _make_fake_judge() -> MagicMock:
    """Build a mock LiteLLMJudge that returns fixed scores."""
    judge = MagicMock()
    judge.get_model_name.return_value = "fake-judge"
    judge.load_model.return_value = "fake-judge"
    return judge


@pytest.mark.asyncio
async def test_run_basic_dataset() -> None:
    """Runner should execute all tasks and return results with scores."""
    llm = _make_fake_llm([{"content": "answer 0"}, {"content": "answer 1"}])
    judge = _make_fake_judge()

    with patch("codeforge.evaluation.runner.evaluate_correctness", new_callable=AsyncMock) as mock_corr:
        mock_corr.return_value = 0.85
        runner = BenchmarkRunner(llm=llm, model="test/model", metrics=["correctness"], judge=judge)
        results = await runner.run(_make_dataset(2))

    assert len(results) == 2
    assert results[0].task_id == "task-0"
    assert results[0].task_name == "Test Task 0"
    assert results[0].scores["correctness"] == 0.85
    assert results[1].task_id == "task-1"
    assert llm.chat_completion.call_count == 2


@pytest.mark.asyncio
async def test_run_failed_execution_returns_empty_output() -> None:
    """If LLM call fails, output should be empty string and scores still computed."""
    llm = MagicMock()
    llm.chat_completion = AsyncMock(side_effect=RuntimeError("LLM down"))
    judge = _make_fake_judge()

    with patch("codeforge.evaluation.runner.evaluate_correctness", new_callable=AsyncMock) as mock_corr:
        mock_corr.return_value = 0.0
        runner = BenchmarkRunner(llm=llm, model="test/model", metrics=["correctness"], judge=judge)
        results = await runner.run(_make_dataset(1))

    assert len(results) == 1
    assert results[0].actual_output == ""
    assert results[0].scores["correctness"] == 0.0


@pytest.mark.asyncio
async def test_run_unknown_metric_returns_zero() -> None:
    """Unknown metric names should score 0.0 without raising."""
    llm = _make_fake_llm()
    judge = _make_fake_judge()

    runner = BenchmarkRunner(llm=llm, model="test/model", metrics=["nonexistent_metric"], judge=judge)
    results = await runner.run(_make_dataset(1))

    assert len(results) == 1
    assert results[0].scores["nonexistent_metric"] == 0.0


@pytest.mark.asyncio
async def test_run_duration_is_positive() -> None:
    """Duration should always be >= 0 for any task execution."""
    llm = _make_fake_llm()
    judge = _make_fake_judge()

    with patch("codeforge.evaluation.runner.evaluate_correctness", new_callable=AsyncMock) as mock_corr:
        mock_corr.return_value = 1.0
        runner = BenchmarkRunner(llm=llm, model="test/model", metrics=["correctness"], judge=judge)
        results = await runner.run(_make_dataset(1))

    assert results[0].duration_ms >= 0


class _FakeToolCall:
    """Mimics ToolCallPart from llm module."""

    def __init__(self, name: str, arguments: str) -> None:
        self.id = "call-1"
        self.name = name
        self.arguments = arguments


@pytest.mark.asyncio
async def test_tool_correctness_receives_actual_tools() -> None:
    """Tool calls from the LLM response should flow through to the metric evaluator."""
    tool_calls = [_FakeToolCall(name="read_file", arguments='{"path": "main.py"}')]
    llm = MagicMock()
    llm.chat_completion = AsyncMock(
        return_value=_FakeResponse(content="done", tool_calls=tool_calls),
    )
    judge = _make_fake_judge()

    with patch("codeforge.evaluation.runner.evaluate_tool_correctness", new_callable=AsyncMock) as mock_tc:
        mock_tc.return_value = 0.9
        runner = BenchmarkRunner(llm=llm, model="test/model", metrics=["tool_correctness"], judge=judge)
        results = await runner.run(_make_dataset(1))

    assert len(results) == 1
    assert results[0].scores["tool_correctness"] == 0.9
    assert results[0].tool_calls == [{"name": "read_file", "args": '{"path": "main.py"}'}]
    # Verify the metric received actual tool data, not empty list
    call_kwargs = mock_tc.call_args
    assert call_kwargs.kwargs["actual_tools"] == [{"name": "read_file", "args": '{"path": "main.py"}'}]
