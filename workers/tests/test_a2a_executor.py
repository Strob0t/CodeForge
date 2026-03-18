"""Tests for AgentExecutor.execute_a2a_task -- A2A-specific task execution.

TDD RED phase: These tests define the expected behavior of execute_a2a_task
before it is implemented.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.executor import AgentExecutor
from codeforge.models import TaskResult, TaskStatus


@pytest.fixture
def fake_llm() -> MagicMock:
    llm = MagicMock()
    llm.completion = AsyncMock(
        return_value=MagicMock(
            content="Hello world!",
            tokens_in=10,
            tokens_out=5,
            cost_usd=0.001,
            model="gpt-4o",
        ),
    )
    return llm


@pytest.fixture
def executor(fake_llm: MagicMock) -> AgentExecutor:
    return AgentExecutor(llm=fake_llm)


# ---------------------------------------------------------------------------
# Happy path
# ---------------------------------------------------------------------------


async def test_execute_a2a_task_returns_task_result(executor: AgentExecutor) -> None:
    """execute_a2a_task should return a TaskResult on success."""
    result = await executor.execute_a2a_task(
        task_id="a2a-1",
        skill_id="code-task",
        prompt="Write hello world in Python",
    )
    assert isinstance(result, TaskResult)
    assert result.task_id == "a2a-1"
    assert result.status == TaskStatus.COMPLETED
    assert result.output != ""


async def test_execute_a2a_task_passes_prompt_to_llm(
    executor: AgentExecutor,
    fake_llm: MagicMock,
) -> None:
    """execute_a2a_task should pass the prompt to the LLM."""
    await executor.execute_a2a_task(
        task_id="a2a-2",
        skill_id="code-task",
        prompt="Explain recursion",
    )
    fake_llm.completion.assert_called_once()
    call_kwargs = fake_llm.completion.call_args.kwargs
    assert "Explain recursion" in call_kwargs.get("prompt", "")


async def test_execute_a2a_task_includes_skill_in_system(
    executor: AgentExecutor,
    fake_llm: MagicMock,
) -> None:
    """execute_a2a_task should include the skill_id in the system prompt."""
    await executor.execute_a2a_task(
        task_id="a2a-3",
        skill_id="decompose",
        prompt="Break down login feature",
    )
    call_kwargs = fake_llm.completion.call_args.kwargs
    system_prompt = call_kwargs.get("system", "")
    assert "decompose" in system_prompt.lower()


async def test_execute_a2a_task_tracks_cost(executor: AgentExecutor) -> None:
    """execute_a2a_task should populate cost_usd from LLM response."""
    result = await executor.execute_a2a_task(
        task_id="a2a-4",
        skill_id="code-task",
        prompt="Write a test",
    )
    assert result.cost_usd >= 0.0


async def test_execute_a2a_task_tracks_tokens(executor: AgentExecutor) -> None:
    """execute_a2a_task should populate token counts."""
    result = await executor.execute_a2a_task(
        task_id="a2a-5",
        skill_id="code-task",
        prompt="Write a function",
    )
    assert result.tokens_in > 0
    assert result.tokens_out > 0


# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------


async def test_execute_a2a_task_handles_llm_error(
    executor: AgentExecutor,
    fake_llm: MagicMock,
) -> None:
    """execute_a2a_task should return FAILED status when LLM raises."""
    fake_llm.completion = AsyncMock(side_effect=RuntimeError("LLM timeout"))
    result = await executor.execute_a2a_task(
        task_id="a2a-err-1",
        skill_id="code-task",
        prompt="This will fail",
    )
    assert result.status == TaskStatus.FAILED
    assert "LLM timeout" in result.error


async def test_execute_a2a_task_empty_prompt(executor: AgentExecutor) -> None:
    """execute_a2a_task should handle empty prompt gracefully."""
    result = await executor.execute_a2a_task(
        task_id="a2a-empty",
        skill_id="code-task",
        prompt="",
    )
    # Should still succeed (LLM will get empty prompt)
    assert isinstance(result, TaskResult)


async def test_execute_a2a_task_empty_skill_id(executor: AgentExecutor) -> None:
    """execute_a2a_task should handle empty skill_id gracefully."""
    result = await executor.execute_a2a_task(
        task_id="a2a-no-skill",
        skill_id="",
        prompt="Do something",
    )
    assert isinstance(result, TaskResult)
    assert result.status == TaskStatus.COMPLETED
