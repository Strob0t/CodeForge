"""Tests verifying that tracing decorators don't break instrumented modules.

The decorators should be transparent no-op wrappers when AgentNeo is not
installed (which is the case in the test environment).
"""

from __future__ import annotations

import sys
from types import ModuleType
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.tracing.setup import _NoOpTracer

# --- Mock deepeval module tree (required by evaluation/litellm_judge imports) ---
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


def test_executor_module_imports() -> None:
    """executor.py should import without errors even with NoOp tracer."""
    import codeforge.executor as executor_mod

    assert hasattr(executor_mod, "AgentExecutor")
    assert hasattr(executor_mod, "_tracer")


def test_agent_loop_module_imports() -> None:
    """agent_loop.py should import without errors even with NoOp tracer."""
    import codeforge.agent_loop as loop_mod

    assert hasattr(loop_mod, "AgentLoopExecutor")
    assert hasattr(loop_mod, "_tracer")


def test_mcp_workbench_module_imports() -> None:
    """mcp_workbench.py should import without errors even with NoOp tracer."""
    import codeforge.mcp_workbench as workbench_mod

    assert hasattr(workbench_mod, "McpWorkbench")
    assert hasattr(workbench_mod, "_tracer")


@pytest.mark.asyncio
async def test_noop_tracer_on_async_method() -> None:
    """NoOp tracer decorators should not break async functions."""
    noop = _NoOpTracer()

    @noop.trace_agent("test-agent")
    async def my_async_func() -> str:
        return "hello"

    result = await my_async_func()
    assert result == "hello"


@pytest.mark.asyncio
async def test_executor_execute_with_tracer() -> None:
    """AgentExecutor.execute should work normally with tracing decorators."""
    from codeforge.executor import AgentExecutor
    from codeforge.models import TaskMessage

    mock_llm = MagicMock()
    mock_llm.completion = AsyncMock(
        return_value=MagicMock(
            content="test output",
            tokens_in=10,
            tokens_out=5,
            model="fake",
            cost_usd=0.001,
        )
    )

    executor = AgentExecutor(llm=mock_llm)
    task = TaskMessage(
        id="t-1",
        project_id="p-1",
        title="test task",
        prompt="do something",
    )
    result = await executor.execute(task)
    assert result.output == "test output"
    assert result.status.value == "completed"
