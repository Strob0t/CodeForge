"""Tests for TrajectoryVerifierEvaluator — full trajectory quality assessment.

Tests cover: well-formed trajectory, empty trajectory, truncation,
LLM parse failure, stage property, and multi-dimension output.
"""

from __future__ import annotations

from unittest.mock import AsyncMock, patch

import pytest

from codeforge.evaluation.evaluators.trajectory_verifier import (
    TrajectoryVerifierEvaluator,
    _format_trajectory,
)
from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec, TrajectoryMessage


def _task() -> TaskSpec:
    return TaskSpec(id="t1", name="Fix login bug", input="The login form fails on empty email")


def _result_with_trajectory() -> ExecutionResult:
    return ExecutionResult(
        actual_output="Fixed the validation check",
        files_changed=["src/auth/login.py"],
        test_output="2 passed",
        exit_code=0,
        cost_usd=0.03,
        tokens_in=500,
        tokens_out=200,
        step_count=3,
        trajectory=[
            TrajectoryMessage(role="user", content="Fix the login form bug"),
            TrajectoryMessage(role="assistant", content="Let me read the login code"),
            TrajectoryMessage(role="assistant", tool_name="read_file", tool_args='{"path": "src/auth/login.py"}'),
            TrajectoryMessage(
                role="tool", content="def validate_email(email):\n    return True", tool_name="read_file"
            ),
            TrajectoryMessage(role="assistant", content="I see the issue. The validation always returns True."),
            TrajectoryMessage(role="assistant", tool_name="edit_file", tool_args='{"path": "src/auth/login.py"}'),
            TrajectoryMessage(role="tool", content="File edited successfully", tool_name="edit_file"),
            TrajectoryMessage(role="assistant", content="Fixed the validation check"),
        ],
    )


def _result_empty_trajectory() -> ExecutionResult:
    return ExecutionResult(actual_output="some output", trajectory=[])


# ---------------------------------------------------------------------------
# Tests: format_trajectory
# ---------------------------------------------------------------------------


class TestFormatTrajectory:
    def test_formats_all_roles(self) -> None:
        result = _result_with_trajectory()
        text = _format_trajectory(_task(), result)

        assert "[USER]" in text
        assert "[ASSISTANT]" in text
        assert "[TOOL CALL] read_file" in text
        assert "[TOOL RESULT: read_file]" in text

    def test_empty_trajectory(self) -> None:
        result = _result_empty_trajectory()
        text = _format_trajectory(_task(), result)

        assert text == ""

    def test_truncates_long_content(self) -> None:
        long_content = "x" * 2000
        result = ExecutionResult(
            trajectory=[TrajectoryMessage(role="assistant", content=long_content)],
        )
        text = _format_trajectory(_task(), result)

        # Content should be truncated to 500 chars per message
        assert len(text) < 2000


# ---------------------------------------------------------------------------
# Tests: evaluate
# ---------------------------------------------------------------------------


class TestTrajectoryVerifierEvaluator:
    @pytest.mark.asyncio
    async def test_returns_five_dimensions(self) -> None:
        """Well-formed trajectory → 5 named EvalDimension scores."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": 0.9, "approach_efficiency": 0.8, '
            '"code_quality": 0.85, "error_recovery": 0.7, "completeness": 0.95}'
        )

        evaluator = TrajectoryVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_with_trajectory())

        assert len(dims) == 5
        names = {d.name for d in dims}
        assert names == {
            "trajectory_solution_quality",
            "trajectory_approach_efficiency",
            "trajectory_code_quality",
            "trajectory_error_recovery",
            "trajectory_completeness",
        }
        # Check scores match
        by_name = {d.name: d.score for d in dims}
        assert by_name["trajectory_solution_quality"] == 0.9
        assert by_name["trajectory_approach_efficiency"] == 0.8

    @pytest.mark.asyncio
    async def test_empty_trajectory_returns_zero_scores(self) -> None:
        """Empty trajectory → all 5 dimensions with score 0.0."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": 0.0, "approach_efficiency": 0.0, '
            '"code_quality": 0.0, "error_recovery": 0.0, "completeness": 0.0}'
        )

        evaluator = TrajectoryVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_empty_trajectory())

        assert len(dims) == 5
        for d in dims:
            assert d.score == 0.0

    @pytest.mark.asyncio
    async def test_llm_parse_failure_returns_error_dimension(self) -> None:
        """LLM returns unparseable response → single error dimension."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = "I cannot evaluate this task properly."

        evaluator = TrajectoryVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_with_trajectory())

        assert len(dims) == 1
        assert dims[0].name == "trajectory_quality"
        assert dims[0].score == 0.0
        assert "error" in dims[0].details

    @pytest.mark.asyncio
    async def test_llm_call_exception_returns_error_dimension(self) -> None:
        """LLM call raises exception → single error dimension."""
        evaluator = TrajectoryVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", side_effect=RuntimeError("API down")):
            dims = await evaluator.evaluate(_task(), _result_with_trajectory())

        assert len(dims) == 1
        assert dims[0].name == "trajectory_quality"
        assert dims[0].score == 0.0

    def test_stage_is_rank(self) -> None:
        """Trajectory verifier is a Stage 2 (rank) evaluator."""
        evaluator = TrajectoryVerifierEvaluator()
        assert evaluator.stage == "rank"

    def test_name(self) -> None:
        evaluator = TrajectoryVerifierEvaluator()
        assert evaluator.name == "trajectory_verifier"

    @pytest.mark.asyncio
    async def test_scores_clamped_to_valid_range(self) -> None:
        """Scores outside 0-1 range are clamped."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": 1.5, "approach_efficiency": -0.3, '
            '"code_quality": 0.5, "error_recovery": 0.5, "completeness": 0.5}'
        )

        evaluator = TrajectoryVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_with_trajectory())

        by_name = {d.name: d.score for d in dims}
        assert by_name["trajectory_solution_quality"] == 1.0  # clamped from 1.5
        assert by_name["trajectory_approach_efficiency"] == 0.0  # clamped from -0.3
