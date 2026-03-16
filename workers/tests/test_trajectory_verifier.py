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
        """Well-formed trajectory -> 5 named EvalDimension scores."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": "ACHIEVED", "approach_efficiency": "PARTIALLY_ACHIEVED", '
            '"code_quality": "ACHIEVED", "error_recovery": "NOT_ACHIEVED", "completeness": "ACHIEVED"}'
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
        by_name = {d.name: d.score for d in dims}
        assert by_name["trajectory_solution_quality"] == 1.0
        assert by_name["trajectory_approach_efficiency"] == 0.5
        assert by_name["trajectory_error_recovery"] == 0.0

    @pytest.mark.asyncio
    async def test_empty_trajectory_returns_zero_scores(self) -> None:
        """Empty trajectory -> all 5 dimensions with score 0.0."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": "NOT_ACHIEVED", "approach_efficiency": "NOT_ACHIEVED", '
            '"code_quality": "NOT_ACHIEVED", "error_recovery": "NOT_ACHIEVED", "completeness": "NOT_ACHIEVED"}'
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
    async def test_unknown_category_maps_to_zero(self) -> None:
        """Unknown category string maps to 0.0."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": "UNKNOWN", "approach_efficiency": "MAYBE", '
            '"code_quality": "ACHIEVED", "error_recovery": "ACHIEVED", "completeness": "ACHIEVED"}'
        )

        evaluator = TrajectoryVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_with_trajectory())

        by_name = {d.name: d.score for d in dims}
        assert by_name["trajectory_solution_quality"] == 0.0  # UNKNOWN -> 0.0
        assert by_name["trajectory_approach_efficiency"] == 0.0  # MAYBE -> 0.0
        assert by_name["trajectory_code_quality"] == 1.0  # ACHIEVED -> 1.0

    @pytest.mark.asyncio
    async def test_achieved_maps_to_1(self) -> None:
        """ACHIEVED -> 1.0."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": "ACHIEVED", "approach_efficiency": "ACHIEVED", '
            '"code_quality": "ACHIEVED", "error_recovery": "ACHIEVED", "completeness": "ACHIEVED"}'
        )
        evaluator = TrajectoryVerifierEvaluator(model="test-model")
        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_with_trajectory())
        for d in dims:
            assert d.score == 1.0

    @pytest.mark.asyncio
    async def test_partially_maps_to_half(self) -> None:
        """PARTIALLY_ACHIEVED -> 0.5."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": "PARTIALLY_ACHIEVED", "approach_efficiency": "PARTIALLY_ACHIEVED", '
            '"code_quality": "PARTIALLY_ACHIEVED", "error_recovery": "PARTIALLY_ACHIEVED", '
            '"completeness": "PARTIALLY_ACHIEVED"}'
        )
        evaluator = TrajectoryVerifierEvaluator(model="test-model")
        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_with_trajectory())
        for d in dims:
            assert d.score == 0.5

    @pytest.mark.asyncio
    async def test_not_achieved_maps_to_zero(self) -> None:
        """NOT_ACHIEVED -> 0.0."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": "NOT_ACHIEVED", "approach_efficiency": "NOT_ACHIEVED", '
            '"code_quality": "NOT_ACHIEVED", "error_recovery": "NOT_ACHIEVED", '
            '"completeness": "NOT_ACHIEVED"}'
        )
        evaluator = TrajectoryVerifierEvaluator(model="test-model")
        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_with_trajectory())
        for d in dims:
            assert d.score == 0.0

    @pytest.mark.asyncio
    async def test_case_insensitive_categories(self) -> None:
        """Category matching is case-insensitive."""
        mock_response = AsyncMock()
        mock_response.choices = [AsyncMock()]
        mock_response.choices[0].message.content = (
            '{"solution_quality": "achieved", "approach_efficiency": "Achieved", '
            '"code_quality": "ACHIEVED", "error_recovery": "partially_achieved", '
            '"completeness": "Partially_Achieved"}'
        )
        evaluator = TrajectoryVerifierEvaluator(model="test-model")
        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_with_trajectory())
        by_name = {d.name: d.score for d in dims}
        assert by_name["trajectory_solution_quality"] == 1.0
        assert by_name["trajectory_approach_efficiency"] == 1.0
        assert by_name["trajectory_code_quality"] == 1.0
        assert by_name["trajectory_error_recovery"] == 0.5
        assert by_name["trajectory_completeness"] == 0.5

    def test_prompt_contains_category_definitions(self) -> None:
        """Prompt includes ACHIEVED / PARTIALLY_ACHIEVED / NOT_ACHIEVED."""
        from codeforge.evaluation.evaluators.trajectory_verifier import _VERIFIER_PROMPT

        assert "ACHIEVED" in _VERIFIER_PROMPT
        assert "PARTIALLY_ACHIEVED" in _VERIFIER_PROMPT
        assert "NOT_ACHIEVED" in _VERIFIER_PROMPT
