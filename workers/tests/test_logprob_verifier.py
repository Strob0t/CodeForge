"""Tests for LogprobVerifierEvaluator — calibrated ranking via P(YES) logprobs.

Tests cover: single dimension output, high/low/equal confidence logprobs,
text fallback (YES/NO/ambiguous), LLM exception, empty trajectory,
stage/name properties, missing YES/NO tokens, case-insensitive matching.
"""

from __future__ import annotations

import math
from unittest.mock import patch

import pytest

from codeforge.evaluation.evaluators.base import EvaluatorError
from codeforge.evaluation.evaluators.logprob_verifier import LogprobVerifierEvaluator
from codeforge.evaluation.providers.base import ExecutionResult, TaskSpec, TrajectoryMessage
from codeforge.llm import ChatCompletionResponse, TokenLogprob


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


def _response(content: str, top_logprobs: list[TokenLogprob] | None = None) -> ChatCompletionResponse:
    """A verifier answer as LiteLLMClient.chat_completion returns it."""
    return ChatCompletionResponse(
        content=content,
        tool_calls=[],
        finish_reason="stop",
        tokens_in=10,
        tokens_out=1,
        model="test-model",
        top_logprobs=top_logprobs or [],
    )


def _mock_logprob_response(yes_logprob: float, no_logprob: float) -> ChatCompletionResponse:
    """A response with logprobs for the YES and NO tokens."""
    return _response(
        "YES" if yes_logprob > no_logprob else "NO",
        [TokenLogprob("YES", yes_logprob), TokenLogprob("NO", no_logprob)],
    )


def _mock_text_response(text: str) -> ChatCompletionResponse:
    """A response without logprobs (text fallback)."""
    return _response(text)


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
    @pytest.mark.parametrize("content", ["Maybe", "", "   "])
    async def test_unusable_answer_is_an_evaluation_error(self, content: str) -> None:
        """No YES/NO logprobs and no YES/NO text -> EvaluatorError, not a 0.5 score (S6-G review, 7)."""
        mock_response = _mock_text_response(content)
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with (
            patch.object(evaluator, "_call_verifier", return_value=mock_response),
            pytest.raises(EvaluatorError, match="no usable answer"),
        ):
            await evaluator.evaluate(_task(), _result())

    @pytest.mark.asyncio
    async def test_llm_exception_is_an_evaluation_error(self) -> None:
        """LLM call raises -> EvaluatorError, not a 0.0 score (KI-37)."""
        evaluator = LogprobVerifierEvaluator(model="test-model")

        with (
            patch.object(evaluator, "_call_verifier", side_effect=RuntimeError("API down")),
            pytest.raises(EvaluatorError, match="API down"),
        ):
            await evaluator.evaluate(_task(), _result())

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
        response = _response("YES", [TokenLogprob("MAYBE", -0.5)])

        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=response):
            dims = await evaluator.evaluate(_task(), _result())

        assert dims[0].score == 1.0
        assert dims[0].details["method"] == "text_fallback"

    @pytest.mark.asyncio
    async def test_case_insensitive_matching(self) -> None:
        """Logprobs with lowercase 'yes'/'no' tokens are recognized correctly."""
        response = _response("yes", [TokenLogprob("yes", -0.1), TokenLogprob("no", -3.0)])

        evaluator = LogprobVerifierEvaluator(model="test-model")

        with patch.object(evaluator, "_call_verifier", return_value=response):
            dims = await evaluator.evaluate(_task(), _result())

        # Should recognize lowercase tokens and use logprob method
        assert dims[0].details["method"] == "logprob"
        # yes=-0.1 should give high confidence
        expected = math.exp(-0.1) / (math.exp(-0.1) + math.exp(-3.0))
        assert dims[0].score == pytest.approx(expected, abs=0.01)

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("content", "score"),
        [
            ("Yes.", 1.0),
            ("yes", 1.0),
            ("**YES**", 1.0),
            ("  Yes, the task is solved.", 1.0),
            ("Y", 1.0),
            ("No, because the tests fail", 0.0),
            ("no!", 0.0),
            ("n.", 0.0),
        ],
    )
    async def test_text_fallback_reads_a_leading_yes_or_no(self, content: str, score: float) -> None:
        """A leading yes/no word counts, whatever punctuation and text surround it (S6-G re-review 4)."""
        evaluator = LogprobVerifierEvaluator(model="test-model")
        with patch.object(evaluator, "_call_verifier", return_value=_mock_text_response(content)):
            dims = await evaluator.evaluate(_task(), _result())
        assert dims[0].score == score
        assert dims[0].details["method"] == "text_fallback"

    @pytest.mark.asyncio
    @pytest.mark.parametrize("content", ["Nope", "N/A", "Not sure", "Yesterday", "Maybe yes", "..."])
    async def test_text_fallback_rejects_other_answers(self, content: str) -> None:
        evaluator = LogprobVerifierEvaluator(model="test-model")
        with (
            patch.object(evaluator, "_call_verifier", return_value=_mock_text_response(content)),
            pytest.raises(EvaluatorError, match="no usable answer"),
        ):
            await evaluator.evaluate(_task(), _result())
