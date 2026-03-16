"""Tests for LogprobVerifierEvaluator — calibrated ranking via P(YES) logprobs.

Tests cover: single dimension output, high/low/equal confidence logprobs,
text fallback (YES/NO/ambiguous), LLM exception, empty trajectory,
stage/name properties, missing YES/NO tokens, case-insensitive matching.
"""

from __future__ import annotations

import math
from unittest.mock import AsyncMock, patch

import pytest

from codeforge.evaluation.evaluators.logprob_verifier import LogprobVerifierEvaluator
from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec, TrajectoryMessage


def _task() -> TaskSpec:
    return TaskSpec(id="t1", name="Test task", input="Implement feature X")


def _result() -> ExecutionResult:
    return ExecutionResult(
        actual_output="Done",
        files_changed=["src/main.py"],
        trajectory=[
            TrajectoryMessage(role="user", content="Do the thing"),
            TrajectoryMessage(role="assistant", content="I'll do it"),
        ],
    )


def _result_empty_trajectory() -> ExecutionResult:
    return ExecutionResult(actual_output="Done", trajectory=[])


def _mock_logprob_response(yes_logprob: float, no_logprob: float) -> AsyncMock:
    """Create a mock LLM response with logprobs for YES and NO tokens."""
    response = AsyncMock()
    response.choices = [AsyncMock()]
    response.choices[0].message.content = "YES" if yes_logprob > no_logprob else "NO"

    top_logprobs = []
    # YES token
    yes_token = AsyncMock()
    yes_token.token = "YES"  # noqa: S105
    yes_token.logprob = yes_logprob
    top_logprobs.append(yes_token)
    # NO token
    no_token = AsyncMock()
    no_token.token = "NO"  # noqa: S105
    no_token.logprob = no_logprob
    top_logprobs.append(no_token)

    logprobs_content_item = AsyncMock()
    logprobs_content_item.top_logprobs = top_logprobs
    response.choices[0].logprobs = AsyncMock()
    response.choices[0].logprobs.content = [logprobs_content_item]

    return response


def _mock_text_response(text: str) -> AsyncMock:
    """Create a mock LLM response without logprobs (text fallback)."""
    response = AsyncMock()
    response.choices = [AsyncMock()]
    response.choices[0].message.content = text
    response.choices[0].logprobs = None
    return response


class TestLogprobVerifierEvaluator:
    @pytest.mark.asyncio
    async def test_returns_single_dimension(self) -> None:
        """Logprob verifier returns exactly 1 EvalDimension named 'logprob_verification'."""
        mock_response = _mock_logprob_response(yes_logprob=-0.5, no_logprob=-1.0)
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result())

        assert len(dims) == 1
        assert dims[0].name == "logprob_verification"

    @pytest.mark.asyncio
    async def test_high_confidence_yes(self) -> None:
        """YES=-0.01, NO=-5.0 -> score close to 1.0 (high confidence YES)."""
        mock_response = _mock_logprob_response(yes_logprob=-0.01, no_logprob=-5.0)
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result())

        assert dims[0].score == pytest.approx(0.993, abs=0.01)

    @pytest.mark.asyncio
    async def test_high_confidence_no(self) -> None:
        """YES=-5.0, NO=-0.01 -> score close to 0.0 (high confidence NO)."""
        mock_response = _mock_logprob_response(yes_logprob=-5.0, no_logprob=-0.01)
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result())

        assert dims[0].score == pytest.approx(0.007, abs=0.01)

    @pytest.mark.asyncio
    async def test_equal_confidence(self) -> None:
        """YES=-1.0, NO=-1.0 -> score = 0.5 (equal confidence)."""
        mock_response = _mock_logprob_response(yes_logprob=-1.0, no_logprob=-1.0)
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result())

        assert dims[0].score == 0.5

    @pytest.mark.asyncio
    async def test_fallback_text_yes(self) -> None:
        """logprobs=None, content='YES' -> score=1.0, method=text_fallback."""
        mock_response = _mock_text_response("YES")
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result())

        assert dims[0].score == 1.0
        assert dims[0].details["method"] == "text_fallback"

    @pytest.mark.asyncio
    async def test_fallback_text_no(self) -> None:
        """logprobs=None, content='NO' -> score=0.0, method=text_fallback."""
        mock_response = _mock_text_response("NO")
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result())

        assert dims[0].score == 0.0
        assert dims[0].details["method"] == "text_fallback"

    @pytest.mark.asyncio
    async def test_fallback_text_ambiguous(self) -> None:
        """logprobs=None, content='Maybe' -> score=0.5, method=text_fallback."""
        mock_response = _mock_text_response("Maybe")
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result())

        assert dims[0].score == 0.5
        assert dims[0].details["method"] == "text_fallback"

    @pytest.mark.asyncio
    async def test_llm_exception_returns_error(self) -> None:
        """LLM call raises exception -> score=0.0, 'error' in details."""
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", side_effect=RuntimeError("API down")):
            dims = await evaluator.evaluate(_task(), _result())

        assert len(dims) == 1
        assert dims[0].score == 0.0
        assert "error" in dims[0].details

    @pytest.mark.asyncio
    async def test_empty_trajectory(self) -> None:
        """Empty trajectory still produces 1 dimension result."""
        mock_response = _mock_logprob_response(yes_logprob=-0.5, no_logprob=-1.0)
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=mock_response):
            dims = await evaluator.evaluate(_task(), _result_empty_trajectory())

        assert len(dims) == 1
        assert dims[0].name == "logprob_verification"

    def test_stage_is_rank(self) -> None:
        """Logprob verifier is a Stage 2 (rank) evaluator."""
        evaluator = LogprobVerifierEvaluator()
        assert evaluator.stage == "rank"

    def test_name(self) -> None:
        evaluator = LogprobVerifierEvaluator()
        assert evaluator.name == "logprob_verifier"

    @pytest.mark.asyncio
    async def test_missing_yes_no_tokens(self) -> None:
        """Logprobs with only unrelated tokens -> falls back to text parsing."""
        response = AsyncMock()
        response.choices = [AsyncMock()]
        response.choices[0].message.content = "YES"

        # Create logprobs with only an unrelated token
        maybe_token = AsyncMock()
        maybe_token.token = "MAYBE"  # noqa: S105
        maybe_token.logprob = -0.5

        logprobs_content_item = AsyncMock()
        logprobs_content_item.top_logprobs = [maybe_token]
        response.choices[0].logprobs = AsyncMock()
        response.choices[0].logprobs.content = [logprobs_content_item]

        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=response):
            dims = await evaluator.evaluate(_task(), _result())

        assert dims[0].score == 1.0
        assert dims[0].details["method"] == "text_fallback"

    @pytest.mark.asyncio
    async def test_case_insensitive_matching(self) -> None:
        """Logprobs with lowercase 'yes'/'no' tokens are recognized correctly."""
        response = AsyncMock()
        response.choices = [AsyncMock()]
        response.choices[0].message.content = "yes"

        # Lowercase tokens
        yes_token = AsyncMock()
        yes_token.token = "yes"  # noqa: S105
        yes_token.logprob = -0.1

        no_token = AsyncMock()
        no_token.token = "no"  # noqa: S105
        no_token.logprob = -3.0

        logprobs_content_item = AsyncMock()
        logprobs_content_item.top_logprobs = [yes_token, no_token]
        response.choices[0].logprobs = AsyncMock()
        response.choices[0].logprobs.content = [logprobs_content_item]

        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=response):
            dims = await evaluator.evaluate(_task(), _result())

        # Should recognize lowercase tokens and use logprob method
        assert dims[0].details["method"] == "logprob"
        # yes=-0.1 should give high confidence
        expected = math.exp(-0.1) / (math.exp(-0.1) + math.exp(-3.0))
        assert dims[0].score == pytest.approx(expected, abs=0.01)
